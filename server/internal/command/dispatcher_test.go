package command

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/device"
)

type recordingCommandPublisher struct {
	mu         sync.Mutex
	deliveries []Delivery
	err        error
	hook       func(Delivery)
}

func (p *recordingCommandPublisher) PublishCommand(_ context.Context, delivery Delivery) error {
	p.mu.Lock()
	p.deliveries = append(p.deliveries, Delivery{CommandID: delivery.CommandID, Topic: delivery.Topic, Payload: append([]byte(nil), delivery.Payload...)})
	hook := p.hook
	p.mu.Unlock()
	if hook != nil {
		hook(delivery)
	}
	return p.err
}

func (p *recordingCommandPublisher) calls() []Delivery {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Delivery(nil), p.deliveries...)
}

func newDispatcherFixture(t *testing.T, commandID string) (*Repository, *Service, *Dispatcher, *recordingCommandPublisher, Command, Delivery) {
	t.Helper()
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	service, err := NewService(repository, serviceDeviceReader{value: device.Device{EdgeID: "edge", SourceDeviceID: "source"}}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	publisher := &recordingCommandPublisher{}
	dispatcher, err := NewDispatcher(service, publisher)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC)
	command, delivery := testCommand(commandID, "hash")
	command.IssuedAt, command.ExpiresAt = at, at.Add(time.Minute)
	delivery.Payload, delivery.Topic, err = buildPayload("edge", command.EdgeID, command.SourceDeviceID, command.CommandID, command.Name, []byte(command.Args), command.IssuedAt, command.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	delivery.ExpiresAt = command.ExpiresAt
	if _, err := repository.Create(context.Background(), command, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	dispatcher.now = func() time.Time { return at }
	return repository, service, dispatcher, publisher, command, delivery
}

func TestDispatcherPublishesFrozenDeliveryAndPUBACKDoesNotChangeCommand(t *testing.T) {
	repository, _, dispatcher, publisher, command, delivery := newDispatcherFixture(t, "55555555-5555-4555-8555-555555555555")
	attempted, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || !attempted {
		t.Fatalf("DispatchOnce() = attempted %v, err %v", attempted, err)
	}
	calls := publisher.calls()
	if len(calls) != 1 || calls[0].CommandID != command.CommandID || calls[0].Topic != delivery.Topic || !bytes.Equal(calls[0].Payload, delivery.Payload) {
		t.Fatalf("published delivery = %+v, want frozen topic/payload", calls)
	}
	current, err := repository.Detail(context.Background(), command.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusPending {
		t.Fatalf("publish changed command status to %s", current.Status)
	}
	var stored Delivery
	if err := repository.db.Where("command_id = ?", command.CommandID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.AttemptCount != 1 || stored.LastAttemptAt == nil || stored.LastError != nil {
		t.Fatalf("transport fact = %+v", stored)
	}
}

func TestDispatcherPublishFailureKeepsDeliveryAndRecordsError(t *testing.T) {
	repository, _, dispatcher, publisher, command, _ := newDispatcherFixture(t, "66666666-6666-4666-8666-666666666666")
	at := time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC)
	publisher.err = errors.New("broker unavailable")
	if attempted, err := dispatcher.DispatchOnce(context.Background()); !attempted || !errors.Is(err, publisher.err) {
		t.Fatalf("DispatchOnce() = attempted %v, err %v", attempted, err)
	}
	var stored Delivery
	if err := repository.db.Where("command_id = ?", command.CommandID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.AttemptCount != 1 || stored.LastError == nil || *stored.LastError != publisher.err.Error() {
		t.Fatalf("failed transport fact = %+v", stored)
	}
	dispatcher.now = func() time.Time { return at.Add(time.Second - time.Nanosecond) }
	if attempted, err := dispatcher.DispatchOnce(context.Background()); attempted || err != nil {
		t.Fatalf("delivery retried before next_attempt_at: attempted %v, err %v", attempted, err)
	}
	dispatcher.now = func() time.Time { return at.Add(time.Second) }
	if attempted, err := dispatcher.DispatchOnce(context.Background()); !attempted || !errors.Is(err, publisher.err) {
		t.Fatalf("delivery was not retried at next_attempt_at: attempted %v, err %v", attempted, err)
	}
	current, err := repository.Detail(context.Background(), command.CommandID)
	if err != nil || current.Status != CommandStatusPending {
		t.Fatalf("failure changed command = %+v, err=%v", current, err)
	}
}

func TestDispatcherUsesFixedBackoffSequence(t *testing.T) {
	repository, _, dispatcher, publisher, command, _ := newDispatcherFixture(t, "99999999-9999-4999-8999-999999999999")
	base := time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC)
	now := base
	dispatcher.now = func() time.Time { return now }
	wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second}
	for attempt, wantDelay := range wantDelays {
		if attempted, err := dispatcher.DispatchOnce(context.Background()); !attempted || err != nil {
			t.Fatalf("attempt %d = attempted %v, err %v", attempt+1, attempted, err)
		}
		var stored Delivery
		if err := repository.db.Where("command_id = ?", command.CommandID).Take(&stored).Error; err != nil {
			t.Fatal(err)
		}
		wantNext := now.Add(wantDelay)
		if stored.AttemptCount != attempt+1 || stored.NextAttemptAt == nil || !stored.NextAttemptAt.Equal(wantNext) {
			t.Fatalf("attempt %d delivery = %+v, want next %v", attempt+1, stored, wantNext)
		}
		now = wantNext
	}
	if len(publisher.calls()) != len(wantDelays) {
		t.Fatalf("publish count = %d, want %d", len(publisher.calls()), len(wantDelays))
	}
}

func TestDispatcherRestartResumesSameDeliveryBytesAndCommandTimes(t *testing.T) {
	repository, service, dispatcher, publisher, command, delivery := newDispatcherFixture(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	base := time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC)
	dispatcher.now = func() time.Time { return base }
	if attempted, err := dispatcher.DispatchOnce(context.Background()); !attempted || err != nil {
		t.Fatal(err)
	}
	first := publisher.calls()[0]
	restartedPublisher := &recordingCommandPublisher{}
	restarted, err := NewDispatcher(service, restartedPublisher)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return base.Add(time.Second) }
	if attempted, err := restarted.DispatchOnce(context.Background()); !attempted || err != nil {
		t.Fatalf("restart DispatchOnce() = attempted %v, err %v", attempted, err)
	}
	second := restartedPublisher.calls()[0]
	if first.Topic != delivery.Topic || second.Topic != first.Topic || !bytes.Equal(first.Payload, second.Payload) {
		t.Fatalf("restart changed frozen delivery: first=%+v second=%+v", first, second)
	}
	current, err := repository.Detail(context.Background(), command.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.IssuedAt.Equal(command.IssuedAt) || !current.ExpiresAt.Equal(command.ExpiresAt) || current.CommandID != command.CommandID {
		t.Fatalf("restart changed command identity/times: %+v", current)
	}
}

func TestDispatcherStopsAfterCommittedCommandResult(t *testing.T) {
	repository, _, dispatcher, publisher, command, _ := newDispatcherFixture(t, "77777777-7777-4777-8777-777777777777")
	observation := resultObservation(CommandStatusSucceeded)
	observation.CommandID = command.CommandID
	if err := repository.ProjectResult(context.Background(), observation); err != nil {
		t.Fatal(err)
	}
	if attempted, err := dispatcher.DispatchOnce(context.Background()); attempted || err != nil {
		t.Fatalf("DispatchOnce after result = attempted %v, err %v", attempted, err)
	}
	if len(publisher.calls()) != 0 {
		t.Fatal("dispatcher published after result committed")
	}
	var deliveries int64
	if err := repository.db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("delivery rows after result = %d", deliveries)
	}
	if expired, err := repository.ExpirePendingDeliveries(context.Background(), command.ExpiresAt.Add(time.Hour)); err != nil || expired != 0 {
		t.Fatalf("expiry after committed result = %d, err=%v", expired, err)
	}
}

func TestDeliveryExpiryLeavesPendingForLateResult(t *testing.T) {
	repository, _, _, _, command, _ := newDispatcherFixture(t, "cccccccc-cccc-4ccc-8ccc-cccccccccccc")
	now := command.ExpiresAt
	expired, err := repository.ExpirePendingDeliveries(context.Background(), now)
	if err != nil || expired != 1 {
		t.Fatalf("ExpirePendingDeliveries() = %d, err=%v", expired, err)
	}
	current, err := repository.Detail(context.Background(), command.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusPending || current.DeliveryExpiredAt == nil || !current.DeliveryExpiredAt.Equal(now) {
		t.Fatalf("expiry fabricated command state: %+v", current)
	}
	var deliveries int64
	if err := repository.db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("delivery rows after expiry = %d", deliveries)
	}
	observation := resultObservation(CommandStatusSucceeded)
	observation.CommandID = command.CommandID
	if err := repository.ProjectResult(context.Background(), observation); err != nil {
		t.Fatal(err)
	}
	current, err = repository.Detail(context.Background(), command.CommandID)
	if err != nil || current.Status != CommandStatusSucceeded {
		t.Fatalf("late result after expiry = %+v, err=%v", current, err)
	}
}

func TestResultWinsDuringInFlightPublishWithoutSecondDelivery(t *testing.T) {
	repository, service, dispatcher, _, command, _ := newDispatcherFixture(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	publisher := &recordingCommandPublisher{}
	publisher.hook = func(Delivery) {
		observation := resultObservation(CommandStatusSucceeded)
		observation.CommandID = command.CommandID
		if err := repository.ProjectResult(context.Background(), observation); err != nil {
			t.Fatalf("result projection during publish: %v", err)
		}
	}
	dispatcher, err := NewDispatcher(service, publisher)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.now = func() time.Time { return command.IssuedAt }
	if attempted, err := dispatcher.DispatchOnce(context.Background()); !attempted || err != nil {
		t.Fatalf("in-flight dispatch = attempted %v, err %v", attempted, err)
	}
	current, err := repository.Detail(context.Background(), command.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusSucceeded {
		t.Fatalf("in-flight result status = %s", current.Status)
	}
	var deliveries int64
	if err := repository.db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 || len(publisher.calls()) != 1 {
		t.Fatalf("in-flight result created/recreated delivery: rows=%d publishes=%d", deliveries, len(publisher.calls()))
	}
}

func TestDispatcherLifecycleStopsWithoutLeakingWorker(t *testing.T) {
	_, _, dispatcher, _, _, _ := newDispatcherFixture(t, "88888888-8888-4888-8888-888888888888")
	dispatcher.interval = time.Millisecond
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}
