package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

type deviceConsumerStore struct {
	observations []device.Observation
	err          error
}

func (s *deviceConsumerStore) Observe(_ context.Context, observation device.Observation) (device.Device, error) {
	s.observations = append(s.observations, observation)
	if s.err != nil {
		return device.Device{}, s.err
	}
	return device.Device{DeviceID: "cloud-device-01", EdgeID: observation.EdgeID, SourceDeviceID: observation.SourceDeviceID}, nil
}

func (s *deviceConsumerStore) Page(context.Context, device.PageQuery) (device.Page, error) {
	return device.Page{}, nil
}

func (s *deviceConsumerStore) Detail(context.Context, string) (device.Device, error) {
	return device.Device{}, device.ErrNotFound
}

func TestDeviceStatusConsumerStrictlyProjectsValidPayload(t *testing.T) {
	store := &deviceConsumerStore{}
	service, err := device.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	consumer := newDeviceStatusConsumer(service)
	receivedAt := time.Date(2026, 9, 21, 2, 3, 4, 0, time.UTC)
	sourceDeviceID := "source-01"
	valid := mqtt.DeviceStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", DeviceID: &sourceDeviceID, ReceivedAt: receivedAt, Retained: false},
		Data:            []byte(`{"status":"ONLINE","lastAttemptAt":"2026-09-21T02:03:03+08:00","lastSuccessAt":"2026-09-21T02:03:02Z","error":""}`),
	}
	if got := consumer.Consume(context.Background(), valid); got != mqtt.OutcomeAccepted {
		t.Fatalf("valid outcome = %q, want accepted", got)
	}
	if len(store.observations) != 1 {
		t.Fatalf("observations = %+v", store.observations)
	}
	observation := store.observations[0]
	if observation.EdgeID != "edge-01" || observation.SourceDeviceID != sourceDeviceID || observation.ReceivedAt != receivedAt || observation.Snapshot.CommunicationError != nil {
		t.Fatalf("domain observation = %+v", observation)
	}

	for _, data := range []string{
		`{"status":"ONLINE","lastAttemptAt":null,"error":null}`,
		`{"status":"UNKNOWN","lastAttemptAt":null,"lastSuccessAt":null,"error":null}`,
		`{"status":"ONLINE","lastAttemptAt":1,"lastSuccessAt":null,"error":null}`,
	} {
		invalid := valid
		invalid.Data = []byte(data)
		if got := consumer.Consume(context.Background(), invalid); got != mqtt.OutcomeRejected {
			t.Errorf("invalid payload %s outcome = %q, want rejected", data, got)
		}
	}
}

func TestDeviceStatusConsumerAcknowledgesUnknownParentForReplayCoordinator(t *testing.T) {
	store := &deviceConsumerStore{err: device.ErrParentNotFound}
	service, err := device.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	consumer := newDeviceStatusConsumer(service)
	sourceDeviceID := "source-01"
	message := mqtt.DeviceStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{EdgeID: "unknown-edge", DeviceID: &sourceDeviceID, ReceivedAt: time.Now()},
		Data:            []byte(`{"status":"ONLINE","lastAttemptAt":null,"lastSuccessAt":null,"error":null}`),
	}
	if got := consumer.Consume(context.Background(), message); got != mqtt.OutcomeAccepted {
		t.Fatalf("unknown parent outcome = %q, want accepted", got)
	}
}

func TestDeviceStatusConsumerRetriesInfrastructureFailures(t *testing.T) {
	store := &deviceConsumerStore{err: errors.New("database unavailable")}
	service, err := device.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	consumer := newDeviceStatusConsumer(service)
	sourceDeviceID := "source-01"
	message := mqtt.DeviceStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", DeviceID: &sourceDeviceID, ReceivedAt: time.Now()},
		Data:            []byte(`{"status":"ONLINE","lastAttemptAt":null,"lastSuccessAt":null,"error":null}`),
	}
	if got := consumer.Consume(context.Background(), message); got != mqtt.OutcomeRetry {
		t.Fatalf("infrastructure failure outcome = %q, want retry", got)
	}
}

func TestDeviceStatusConsumerRejectsMissingSourceIdentity(t *testing.T) {
	store := &deviceConsumerStore{}
	service, err := device.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	consumer := newDeviceStatusConsumer(service)
	message := mqtt.DeviceStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: time.Now()},
		Data:            []byte(`{"status":"ONLINE","lastAttemptAt":null,"lastSuccessAt":null,"error":null}`),
	}
	if got := consumer.Consume(context.Background(), message); got != mqtt.OutcomeRejected {
		t.Fatalf("missing source identity outcome = %q, want rejected", got)
	}
}
