package app

import (
	"context"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

func TestIngressConsumerKeepsEdgeDeviceRawAndEventFactsIsolated(t *testing.T) {
	edgeStore := &consumerStore{}
	edgeService, err := edge.NewService(edgeStore)
	if err != nil {
		t.Fatal(err)
	}
	deviceStore := &deviceConsumerStore{}
	deviceService, err := device.NewService(deviceStore)
	if err != nil {
		t.Fatal(err)
	}
	consumer := ingressConsumer{
		edge:   newEdgeStatusConsumer(edgeService),
		device: newDeviceStatusConsumer(deviceService),
	}
	receivedAt := time.Date(2026, 9, 21, 6, 0, 0, 0, time.UTC)
	sourceDeviceID := "source-01"

	edgeOffline := mqtt.EdgeStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt},
		Data:            []byte(`{"online":false}`),
	}
	if got := consumer.Consume(context.Background(), edgeOffline); got != mqtt.OutcomeAccepted {
		t.Fatalf("EdgeStatus outcome = %q", got)
	}
	deviceOnline := mqtt.DeviceStatusMessage{
		IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", DeviceID: &sourceDeviceID, ReceivedAt: receivedAt.Add(time.Second)},
		Data:            []byte(`{"status":"ONLINE","lastAttemptAt":null,"lastSuccessAt":null,"error":null}`),
	}
	if got := consumer.Consume(context.Background(), deviceOnline); got != mqtt.OutcomeAccepted {
		t.Fatalf("DeviceStatus outcome = %q", got)
	}
	if len(edgeStore.observations) != 1 || edgeStore.observations[0].Status != edge.StatusOffline {
		t.Fatalf("Edge observations = %+v", edgeStore.observations)
	}
	if len(deviceStore.observations) != 1 || deviceStore.observations[0].Snapshot.CommunicationStatus != device.StatusOnline {
		t.Fatalf("Device observations = %+v", deviceStore.observations)
	}

	for _, message := range []mqtt.IngressMessage{
		mqtt.RawRegisterSnapshotMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", ReceivedAt: receivedAt.Add(2 * time.Second)}, Data: []byte(`{"register":1}`)},
		mqtt.DeviceEventMessage{IngressMetadata: mqtt.IngressMetadata{EdgeID: "edge-01", DeviceID: &sourceDeviceID, ReceivedAt: receivedAt.Add(3 * time.Second)}, Data: []byte(`{"event":"ignored"}`)},
	} {
		if got := consumer.Consume(context.Background(), message); got != mqtt.OutcomeAccepted {
			t.Errorf("isolated %T outcome = %q", message, got)
		}
	}
	if len(edgeStore.observations) != 1 || len(deviceStore.observations) != 1 {
		t.Fatalf("raw/event mutated domain projections: edges=%+v devices=%+v", edgeStore.observations, deviceStore.observations)
	}
}
