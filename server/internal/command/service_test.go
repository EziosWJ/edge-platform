package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/device"
)

type serviceDeviceReader struct{ value device.Device }

func (r serviceDeviceReader) Detail(context.Context, string) (device.Device, error) {
	return r.value, nil
}

type serviceStore struct {
	created   Command
	delivery  Delivery
	event     audit.Event
	createErr error
}

func (s *serviceStore) Create(_ context.Context, command Command, delivery Delivery, event audit.Event) (Command, error) {
	if s.createErr != nil {
		return Command{}, s.createErr
	}
	s.created, s.delivery, s.event = command, delivery, event
	return command, nil
}

func (s *serviceStore) Detail(context.Context, string) (Command, error) {
	if s.created.CommandID == "" {
		return Command{}, ErrNotFound
	}
	return s.created, nil
}

func (s *serviceStore) Page(context.Context, CommandPageQuery) (CommandPage, error) {
	if s.created.CommandID == "" {
		return CommandPage{Records: []CommandView{}}, nil
	}
	return CommandPage{Records: []CommandView{s.created.View()}, Total: 1}, nil
}

func TestCanonicalizeArgsSortsObjectsPreservesArraysAndNumbers(t *testing.T) {
	got, err := canonicalizeArgs([]byte(`{"z":1.0000000000000000000000001,"nested":{"b":2,"a":90071992547409931234567890},"array":[{"z":3,"a":4},1e+100]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"array":[{"a":4,"z":3},1e+100],"nested":{"a":90071992547409931234567890,"b":2},"z":1.0000000000000000000000001}`
	if string(got) != want {
		t.Fatalf("canonical args = %s, want %s", got, want)
	}
}

func TestCreateFreezesRoutePayloadAndDefaultTTL(t *testing.T) {
	store := &serviceStore{}
	service, err := NewService(store, serviceDeviceReader{value: device.Device{DeviceID: "cloud-device", EdgeID: "edge-1", SourceDeviceID: "source-1"}}, "factory")
	if err != nil {
		t.Fatal(err)
	}
	issued := time.Date(2026, 9, 22, 1, 2, 3, 456000000, time.UTC)
	service.now = func() time.Time { return issued }
	commandID := "11111111-1111-4111-8111-111111111111"
	view, err := service.Create(context.Background(), 7, CreateInput{
		CommandID: commandID, DeviceID: "cloud-device", Name: "close",
		Args: []byte(`{"b":2,"a":90071992547409931234567890}`),
	}, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != CommandStatusPending || !view.ExpiresAt.Equal(issued.Add(30*time.Second)) {
		t.Fatalf("view = %+v", view)
	}
	if string(view.Args) != `{"a":90071992547409931234567890,"b":2}` {
		t.Fatalf("view args = %s", view.Args)
	}
	if store.created.EdgeID != "edge-1" || store.created.SourceDeviceID != "source-1" || store.created.Topic != "factory/edge-1/device/source-1/command" {
		t.Fatalf("frozen route = %+v", store.created)
	}
	if len(store.delivery.Payload) == 0 || len(store.delivery.Payload) > MaxPayloadBytes || store.delivery.Topic != store.created.Topic {
		t.Fatalf("delivery = topic %q payload=%d", store.delivery.Topic, len(store.delivery.Payload))
	}
	var payload struct {
		Schema    string          `json:"schema"`
		CommandID string          `json:"commandId"`
		DeviceID  string          `json:"deviceId"`
		Name      string          `json:"name"`
		Args      json.RawMessage `json:"args"`
		IssuedAt  string          `json:"issuedAt"`
		ExpiresAt string          `json:"expiresAt"`
	}
	if err := json.Unmarshal(store.delivery.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Schema != "device-command/v1" || payload.CommandID != commandID || payload.DeviceID != "source-1" || payload.Name != "close" || string(payload.Args) != `{"a":90071992547409931234567890,"b":2}` || payload.IssuedAt != issued.Format(time.RFC3339Nano) || payload.ExpiresAt != issued.Add(30*time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("payload = %+v", payload)
	}
	if string(store.delivery.Payload) == "" || strings.Contains(string(store.delivery.Payload), `"messageId"`) || strings.Contains(string(store.delivery.Payload), `"data"`) {
		t.Fatalf("payload uses the old envelope shape: %s", store.delivery.Payload)
	}
	if store.event.Action != "command.create" || store.event.Resource != "command" || store.event.Metadata.ActorID != 7 {
		t.Fatalf("audit = %+v", store.event)
	}
}

func TestCreateRejectsNonObjectOrInvalidTTL(t *testing.T) {
	service, err := NewService(&serviceStore{}, serviceDeviceReader{value: device.Device{EdgeID: "edge", SourceDeviceID: "source"}}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []CreateInput{
		{CommandID: "11111111-1111-4111-8111-111111111111", DeviceID: "device", Name: "x", Args: []byte(`[]`)},
		{CommandID: "11111111-1111-4111-8111-111111111111", DeviceID: "device", Name: "x", Args: []byte(`{}`), TTLSeconds: intPointer(0)},
	} {
		if _, err := service.Create(context.Background(), 1, input, audit.Event{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("Create(%+v) error = %v, want ErrInvalid", input, err)
		}
	}
}

func TestCommandPageAcceptsAllBusinessStatusesWithoutChangingResultValidation(t *testing.T) {
	service, err := NewService(&serviceStore{}, serviceDeviceReader{value: device.Device{EdgeID: "edge", SourceDeviceID: "source"}}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{
		CommandStatusPending,
		CommandStatusAccepted,
		CommandStatusRejected,
		CommandStatusExpired,
		CommandStatusSucceeded,
		CommandStatusFailed,
	} {
		if _, err := service.Page(context.Background(), CommandPageQuery{Page: 1, PageSize: 10, Status: &status}); err != nil {
			t.Errorf("query status %q error = %v", status, err)
		}
	}
	if validResultStatus(CommandStatusPending) {
		t.Fatal("PENDING must remain invalid for result ingress")
	}
}

func intPointer(value int) *int { return &value }
