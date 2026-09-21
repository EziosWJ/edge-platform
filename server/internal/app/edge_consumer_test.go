package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/edge"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

type consumerStore struct {
	observations []edge.Observation
	err          error
}

func (s *consumerStore) Observe(_ context.Context, observation edge.Observation) (edge.Edge, error) {
	s.observations = append(s.observations, observation)
	if s.err != nil {
		return edge.Edge{}, s.err
	}
	return edge.Edge{EdgeID: observation.EdgeID, Status: observation.Status, RegisteredAt: observation.ReceivedAt, LastSeenAt: observation.ReceivedAt}, nil
}

func (s *consumerStore) Page(context.Context, edge.PageQuery) (edge.Page, error) {
	return edge.Page{}, nil
}

func (s *consumerStore) Detail(_ context.Context, edgeID string) (edge.Edge, error) {
	return edge.Edge{EdgeID: edgeID}, nil
}

func TestEdgeStatusConsumerProjectsOnlyValidEdgeStatus(t *testing.T) {
	store := &consumerStore{}
	service, err := edge.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 9, 21, 2, 3, 4, 0, time.UTC)
	consumer := newEdgeStatusConsumer(service)

	valid := mqtt.EdgeStatusMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt}, Data: []byte(`{"online":true,"reason":"ignored"}`)}
	if got := consumer.Consume(context.Background(), valid); got != mqtt.OutcomeAccepted {
		t.Fatalf("valid outcome = %q, want accepted", got)
	}
	if len(store.observations) != 1 || store.observations[0].Status != edge.StatusOnline {
		t.Fatalf("observations = %+v", store.observations)
	}

	device := mqtt.DeviceStatusMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt}, Data: []byte(`{"online":false}`)}
	raw := mqtt.RawRegisterSnapshotMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt}, Data: []byte(`{"value":1}`)}
	event := mqtt.DeviceEventMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt}, Data: []byte(`{"name":"ignored"}`)}
	for _, ignored := range []mqtt.IngressMessage{device, raw, event} {
		if got := consumer.Consume(context.Background(), ignored); got != mqtt.OutcomeAccepted {
			t.Fatalf("non-edge message %T outcome = %q, want accepted", ignored, got)
		}
	}
	if len(store.observations) != 1 {
		t.Fatalf("non-edge message changed Edge observations: %+v", store.observations)
	}

	firstOffline := mqtt.EdgeStatusMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-offline", ReceivedAt: receivedAt}, Data: []byte(`{"online":false}`)}
	if got := consumer.Consume(context.Background(), firstOffline); got != mqtt.OutcomeAccepted {
		t.Fatalf("first offline outcome = %q, want accepted", got)
	}
	if len(store.observations) != 2 || store.observations[1].Status != edge.StatusOffline {
		t.Fatalf("first offline observation = %+v", store.observations)
	}

	for _, data := range [][]byte{[]byte(`{}`), []byte(`{"online":null}`), []byte(`{"online":"true"}`), []byte(`{"online":1}`), []byte(`not-json`)} {
		invalid := mqtt.EdgeStatusMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt}, Data: data}
		if got := consumer.Consume(context.Background(), invalid); got != mqtt.OutcomeRejected {
			t.Errorf("invalid data %q outcome = %q, want rejected", data, got)
		}
	}
}

func TestEdgeStatusConsumerUsesCloudReceivedAtForRetainedAndLWT(t *testing.T) {
	store := &consumerStore{}
	service, err := edge.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	consumer := newEdgeStatusConsumer(service)
	receivedAt := time.Date(2026, 9, 21, 5, 6, 7, 8, time.UTC)
	sourceTimestamp := receivedAt.Add(-24 * time.Hour)

	retainedOnline := mqtt.EdgeStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{
			EdgeID: "edge-retained", SourceTimestamp: sourceTimestamp,
			ReceivedAt: receivedAt, Retained: true,
		},
		Data: []byte(`{"online":true}`),
	}
	if got := consumer.Consume(context.Background(), retainedOnline); got != mqtt.OutcomeAccepted {
		t.Fatalf("retained online outcome = %q, want accepted", got)
	}

	lwtReceivedAt := receivedAt.Add(time.Minute)
	lwtSourceTimestamp := lwtReceivedAt.Add(7 * 24 * time.Hour)
	lwtOffline := mqtt.EdgeStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{
			EdgeID: "edge-retained", SourceTimestamp: lwtSourceTimestamp,
			ReceivedAt: lwtReceivedAt, Retained: true,
		},
		Data: []byte(`{"online":false}`),
	}
	if got := consumer.Consume(context.Background(), lwtOffline); got != mqtt.OutcomeAccepted {
		t.Fatalf("retained LWT outcome = %q, want accepted", got)
	}
	if len(store.observations) != 2 {
		t.Fatalf("observations = %+v, want retained online and LWT offline", store.observations)
	}
	if got := store.observations[0]; got.ReceivedAt != receivedAt || got.Status != edge.StatusOnline {
		t.Fatalf("retained projection = %+v, want Cloud receive time and ONLINE", got)
	}
	if got := store.observations[1]; got.ReceivedAt != lwtReceivedAt || got.Status != edge.StatusOffline {
		t.Fatalf("LWT projection = %+v, want Cloud receive time and OFFLINE", got)
	}
}

func TestEdgeStatusConsumerRetriesPersistenceFailures(t *testing.T) {
	store := &consumerStore{err: errors.New("database unavailable")}
	service, err := edge.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	consumer := newEdgeStatusConsumer(service)
	message := mqtt.EdgeStatusMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: time.Now()}, Data: []byte(`{"online":false}`)}
	if got := consumer.Consume(context.Background(), message); got != mqtt.OutcomeRetry {
		t.Fatalf("persistence failure outcome = %q, want retry", got)
	}
}
