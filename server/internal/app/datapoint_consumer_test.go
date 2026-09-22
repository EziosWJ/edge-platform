package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

type datapointConsumerStore struct{ err error }

func (s datapointConsumerStore) Create(context.Context, datapoint.CreateInput, audit.Event) (datapoint.DataPoint, error) {
	return datapoint.DataPoint{}, nil
}
func (s datapointConsumerStore) Page(context.Context, datapoint.PageQuery) (datapoint.Page, error) {
	return datapoint.Page{}, nil
}
func (s datapointConsumerStore) Detail(context.Context, string) (datapoint.DataPoint, error) {
	return datapoint.DataPoint{}, nil
}
func (s datapointConsumerStore) Update(context.Context, string, datapoint.UpdateInput, audit.Event) (datapoint.DataPoint, error) {
	return datapoint.DataPoint{}, nil
}
func (s datapointConsumerStore) SetEnabled(context.Context, string, bool, audit.Event) (datapoint.DataPoint, error) {
	return datapoint.DataPoint{}, nil
}
func (s datapointConsumerStore) Project(context.Context, datapoint.RawObservation) error {
	return s.err
}

func TestRawSnapshotConsumerKeepsQoS0ProjectionFailuresBestEffort(t *testing.T) {
	service, err := datapoint.NewService(datapointConsumerStore{err: errors.New("database unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	consumer := newRawSnapshotConsumer(service)
	source := "source-1"
	message := mqtt.RawRegisterSnapshotMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-1", DeviceID: &source, ReceivedAt: time.Now().UTC()}, Data: []byte(`{"communicationStatus":"ONLINE","blocks":[]}`)}
	if got := consumer.Consume(context.Background(), message); got != mqtt.OutcomeAccepted {
		t.Fatalf("raw failure outcome = %q, want accepted", got)
	}
}
