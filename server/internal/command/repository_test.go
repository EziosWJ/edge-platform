package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openCommandTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "command.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;
CREATE TABLE sys_user (id INTEGER PRIMARY KEY);
CREATE TABLE command (
 command_id TEXT PRIMARY KEY, device_id TEXT NOT NULL, edge_id TEXT NOT NULL,
 source_device_id TEXT NOT NULL, topic TEXT NOT NULL, name TEXT NOT NULL,
 args JSON NOT NULL, requested_by INTEGER NOT NULL, request_hash TEXT NOT NULL,
 issued_at DATETIME NOT NULL, expires_at DATETIME NOT NULL, status TEXT NOT NULL,
 delivery_expired_at DATETIME, edge_received_at DATETIME, started_at DATETIME,
 completed_at DATETIME, result_received_at DATETIME, result JSON,
 error_type TEXT, error_message TEXT,
 FOREIGN KEY (requested_by) REFERENCES sys_user(id));
CREATE TABLE command_delivery (
 command_id TEXT PRIMARY KEY, topic TEXT NOT NULL, payload BLOB NOT NULL,
 expires_at DATETIME NOT NULL, attempt_count INTEGER NOT NULL,
 last_attempt_at DATETIME, last_error TEXT, next_attempt_at DATETIME,
 FOREIGN KEY (command_id) REFERENCES command(command_id));
CREATE TABLE sys_oper_log (
 id INTEGER PRIMARY KEY AUTOINCREMENT, module_name TEXT NOT NULL,
 operation_type TEXT NOT NULL, request_id TEXT, request_method TEXT,
 request_url TEXT, operator_id INTEGER, operator_ip TEXT, user_agent TEXT,
 operation_status TEXT NOT NULL, operation_time DATETIME, create_time DATETIME);
INSERT INTO sys_user (id) VALUES (7);`).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	return db
}

func testCommand(id, hash string) (Command, Delivery) {
	at := time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)
	return Command{CommandID: id, DeviceID: "device", EdgeID: "edge", SourceDeviceID: "source", Topic: "edge/edge/device/source/command", Name: "close", Args: JSONDocument(`{"a":1}`), RequestedBy: 7, RequestHash: hash, IssuedAt: at, ExpiresAt: at.Add(30 * time.Second), Status: CommandStatusPending}, Delivery{CommandID: id, Topic: "edge/edge/device/source/command", Payload: []byte(`{"schema":"device-command/v1"}`), ExpiresAt: at.Add(30 * time.Second)}
}

func TestRepositorySameCommandIsIdempotentAndConflictsAreAtomic(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("11111111-1111-4111-8111-111111111111", "hash-a")
	event := audit.Event{Action: "command.create", Resource: "command", Metadata: audit.Metadata{ActorID: 7}}
	created, err := repository.Create(context.Background(), first, delivery, event)
	if err != nil {
		t.Fatal(err)
	}
	if created.CommandID != first.CommandID {
		t.Fatalf("created = %+v", created)
	}
	retried, err := repository.Create(context.Background(), first, delivery, event)
	if err != nil || retried.CommandID != first.CommandID {
		t.Fatalf("retry = %+v, err=%v", retried, err)
	}
	conflict := first
	conflict.RequestHash = "hash-b"
	if _, err := repository.Create(context.Background(), conflict, delivery, event); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting retry error = %v, want ErrConflict", err)
	}
	var commands, deliveries, audits int64
	if err := db.Table("command").Count(&commands).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("sys_oper_log").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if commands != 1 || deliveries != 1 || audits != 1 {
		t.Fatalf("counts = command %d delivery %d audit %d", commands, deliveries, audits)
	}
}

func TestRepositoryPageFiltersOrdersAndProjectsOnlyCloudFields(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	issued := time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)
	first, firstDelivery := testCommand("88888888-8888-4888-8888-888888888888", "hash-1")
	first.IssuedAt, first.ExpiresAt = issued, issued.Add(time.Minute)
	firstDelivery.ExpiresAt = first.ExpiresAt
	if _, err := repository.Create(context.Background(), first, firstDelivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	second, secondDelivery := testCommand("77777777-7777-4777-8777-777777777777", "hash-2")
	second.RequestedBy = 7
	second.DeviceID, second.Name, second.Status = "other-device", "open", CommandStatusSucceeded
	second.IssuedAt, second.ExpiresAt = issued, issued.Add(time.Minute)
	secondDelivery.ExpiresAt = second.ExpiresAt
	if _, err := repository.Create(context.Background(), second, secondDelivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	status := CommandStatusPending
	page, err := repository.Page(context.Background(), CommandPageQuery{Page: 1, PageSize: 10, Status: &status, RequestedBy: int64Pointer(7)})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Records) != 1 || page.Records[0].CommandID != first.CommandID {
		t.Fatalf("filtered page = %+v", page)
	}
	all, err := repository.Page(context.Background(), CommandPageQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Records) != 2 || all.Records[0].CommandID != second.CommandID || all.Records[1].CommandID != first.CommandID {
		t.Fatalf("issued_at/command_id order = %+v", all.Records)
	}
	encoded, err := json.Marshal(all.Records[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"topic", "payload", "requestHash"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("public command projection exposed %q: %s", forbidden, encoded)
		}
	}
}

func int64Pointer(value int64) *int64 { return &value }

func TestRepositoryAuditFailureRollsBackCommandAndDelivery(t *testing.T) {
	db := openCommandTestDB(t)
	if err := db.Exec("DROP TABLE sys_oper_log").Error; err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(db)
	command, delivery := testCommand("22222222-2222-4222-8222-222222222222", "hash")
	if _, err := repository.Create(context.Background(), command, delivery, audit.Event{Action: "command.create", Resource: "command"}); err == nil {
		t.Fatal("Create succeeded without audit table")
	}
	var commands, deliveries int64
	if err := db.Table("command").Count(&commands).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if commands != 0 || deliveries != 0 {
		t.Fatalf("failed transaction left rows: command=%d delivery=%d", commands, deliveries)
	}
}

func TestServiceConcurrentSameCommandIDConvergesToOneAcceptedRecord(t *testing.T) {
	db := openCommandTestDB(t)
	service, err := NewService(NewRepository(db), serviceDeviceReader{value: device.Device{EdgeID: "edge", SourceDeviceID: "source"}}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	issued := time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return issued }
	input := CreateInput{
		CommandID: "33333333-3333-4333-8333-333333333333",
		DeviceID:  "device",
		Name:      "close",
		Args:      []byte(`{"nested":{"b":2,"a":1},"large":90071992547409931234567890}`),
	}
	const workers = 12
	start := make(chan struct{})
	views := make(chan CommandView, workers)
	errorsCh := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			view, err := service.Create(context.Background(), 7, input, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}})
			if err != nil {
				errorsCh <- err
				return
			}
			views <- view
		}()
	}
	close(start)
	wait.Wait()
	close(views)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent create error: %v", err)
		}
	}
	var first CommandView
	for view := range views {
		if first.CommandID == "" {
			first = view
			continue
		}
		if view.CommandID != first.CommandID || !view.IssuedAt.Equal(first.IssuedAt) || !view.ExpiresAt.Equal(first.ExpiresAt) || string(view.Args) != string(first.Args) {
			t.Fatalf("concurrent views diverged: first=%+v current=%+v", first, view)
		}
	}
	if first.CommandID == "" {
		t.Fatal("concurrent create returned no successful view")
	}

	if _, err := service.Create(context.Background(), 8, input, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 8}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("different actor error = %v, want ErrConflict", err)
	}
	changed := input
	changed.Name = "open"
	if _, err := service.Create(context.Background(), 7, changed, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("different request error = %v, want ErrConflict", err)
	}

	var commands, deliveries, audits int64
	if err := db.Table("command").Count(&commands).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("sys_oper_log").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if commands != 1 || deliveries != 1 || audits != 1 {
		t.Fatalf("concurrent counts = command %d delivery %d audit %d", commands, deliveries, audits)
	}
	var frozenPayload string
	if err := db.Table("command_delivery").Select("payload").Where("command_id = ?", first.CommandID).Scan(&frozenPayload).Error; err != nil {
		t.Fatal(err)
	}
	expectedPayload, _, err := buildPayload("edge", "edge", "source", first.CommandID, "close", first.Args, first.IssuedAt, first.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(frozenPayload), expectedPayload) {
		t.Fatalf("delivery payload changed: got %s want %s", frozenPayload, expectedPayload)
	}
}
