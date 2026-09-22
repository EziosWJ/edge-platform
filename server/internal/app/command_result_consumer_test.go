package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/command"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

type commandResultStore struct {
	projectErr  error
	disposition command.ResultDisposition
}

func (s commandResultStore) Create(context.Context, command.Command, command.Delivery, audit.Event) (command.Command, error) {
	return command.Command{}, nil
}
func (s commandResultStore) Detail(context.Context, string) (command.Command, error) {
	return command.Command{}, command.ErrNotFound
}
func (s commandResultStore) Page(context.Context, command.CommandPageQuery) (command.CommandPage, error) {
	return command.CommandPage{}, nil
}
func (s commandResultStore) ProjectResult(context.Context, command.ResultObservation) error {
	return s.projectErr
}
func (s commandResultStore) ProjectResultDecision(context.Context, command.ResultObservation) (command.ResultProjection, error) {
	return command.ResultProjection{Disposition: s.disposition}, s.projectErr
}

func testCommandResultService(t *testing.T, projectErr error) *command.Service {
	t.Helper()
	service, err := command.NewService(commandResultStore{projectErr: projectErr}, resultDeviceReader{}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type resultObserver struct{ counts map[string]int }

func (o *resultObserver) Observe(category string) { o.counts[category]++ }

type resultDeviceReader struct{}

func (resultDeviceReader) Detail(context.Context, string) (device.Device, error) {
	return device.Device{}, nil
}

func testResultMessage(data string) mqtt.CommandResultMessage {
	deviceID := "source"
	return mqtt.CommandResultMessage{
		IngressMetadata: mqtt.IngressMetadata{
			EdgeID: "edge", DeviceID: &deviceID, ReceivedAt: time.Date(2026, 9, 22, 4, 0, 0, 0, time.UTC),
		},
		Data: []byte(data),
	}
}

const validResultData = `{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"SUCCEEDED","receivedAt":"2026-09-10T04:59:00Z","startedAt":null,"completedAt":"2026-09-10T05:00:00Z","result":{"ok":true},"error":null}`

func TestCommandResultConsumerAcknowledgesPermanentContractAndUnknownFailures(t *testing.T) {
	consumer := newCommandResultConsumer(testCommandResultService(t, command.ErrUnknownCommand))
	if got := consumer.Consume(context.Background(), testResultMessage(validResultData)); got != mqtt.OutcomeRejected {
		t.Fatalf("unknown command outcome = %q", got)
	}
	for _, data := range []string{
		`{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"NOPE","receivedAt":"2026-09-10T04:59:00Z","startedAt":null,"completedAt":null,"result":null,"error":null}`,
		`{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"SUCCEEDED","receivedAt":"2026-09-10T04:59:00Z","startedAt":null,"completedAt":null,"result":null}`,
	} {
		if got := consumer.Consume(context.Background(), testResultMessage(data)); got != mqtt.OutcomeRejected {
			t.Errorf("invalid data outcome = %q", got)
		}
	}
	for _, permanent := range []error{command.ErrResultRouteMismatch, command.ErrResultNameMismatch} {
		consumer := newCommandResultConsumer(testCommandResultService(t, permanent))
		if got := consumer.Consume(context.Background(), testResultMessage(validResultData)); got != mqtt.OutcomeRejected {
			t.Errorf("permanent %v outcome = %q", permanent, got)
		}
	}
}

func TestCommandResultConsumerAcceptsAfterProjection(t *testing.T) {
	consumer := newCommandResultConsumer(testCommandResultService(t, nil))
	if got := consumer.Consume(context.Background(), testResultMessage(validResultData)); got != mqtt.OutcomeAccepted {
		t.Fatalf("valid result outcome = %q", got)
	}
}

func TestCommandResultConsumerDoesNotAckPersistenceFailure(t *testing.T) {
	consumer := newCommandResultConsumer(testCommandResultService(t, errors.New("database unavailable")))
	if got := consumer.Consume(context.Background(), testResultMessage(validResultData)); got != mqtt.OutcomeRetry {
		t.Fatalf("persistence failure outcome = %q, want retry", got)
	}
}

func TestCommandResultConsumerAcknowledgesAndClassifiesConflict(t *testing.T) {
	observer := &resultObserver{counts: map[string]int{}}
	service := testCommandResultService(t, nil)
	// The test store is intentionally replaced by a classified store through
	// the service fixture in order to observe the projection disposition seam.
	service, err := command.NewService(commandResultStore{disposition: command.ResultConflict}, resultDeviceReader{}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	consumer := newCommandResultConsumerWithObserver(service, observer)
	if got := consumer.Consume(context.Background(), testResultMessage(validResultData)); got != mqtt.OutcomeAccepted {
		t.Fatalf("conflict outcome = %q, want accepted", got)
	}
	if observer.counts[string(command.ResultConflict)] != 1 {
		t.Fatalf("observer counts = %+v", observer.counts)
	}
}

func TestCommandResultConsumerAcknowledgesAndClassifiesDuplicate(t *testing.T) {
	observer := &resultObserver{counts: map[string]int{}}
	service, err := command.NewService(commandResultStore{disposition: command.ResultDuplicate}, resultDeviceReader{}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	consumer := newCommandResultConsumerWithObserver(service, observer)
	if got := consumer.Consume(context.Background(), testResultMessage(validResultData)); got != mqtt.OutcomeAccepted {
		t.Fatalf("duplicate outcome = %q, want accepted", got)
	}
	if observer.counts[string(command.ResultDuplicate)] != 1 {
		t.Fatalf("observer counts = %+v", observer.counts)
	}
}
