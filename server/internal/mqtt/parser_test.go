package mqtt

import (
	"log/slog"
	"testing"
	"time"
)

func validPayload() []byte {
	return []byte(`{"schema":"device-event/v1","messageId":"m-1","edgeId":"edge-1","deviceId":"device-1","sourceTimestamp":"2026-09-20T01:02:03.123456789+08:00","data":{"name":"changed"},"futureField":true}`)
}

func TestParseTopicAndEnvelope(t *testing.T) {
	p := NewParser(1024, nil, slog.Default())
	if err := SetParserPrefix(p, "edge/cloud"); err != nil {
		t.Fatal(err)
	}
	message, violations, err := p.Parse("edge/cloud/edge-1/device/device-1/event", validPayload(), 1, false)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %v", violations)
	}
	event, ok := message.(DeviceEventMessage)
	if !ok || event.EdgeID != "edge-1" || event.DeviceID == nil || *event.DeviceID != "device-1" {
		t.Fatalf("message = %#v", message)
	}
	if event.SourceTimestamp.Location() != time.UTC || event.SourceTimestamp.Year() != 2026 {
		t.Fatalf("source timestamp = %v", event.SourceTimestamp)
	}
	if string(event.Data) != `{"name":"changed"}` {
		t.Fatalf("data = %s", event.Data)
	}
	if got := Filters("edge"); len(got) != 4 || got[2] != "edge/+/device/+/raw" {
		t.Fatalf("filters = %v", got)
	}
}

func TestParserRejectsIdentityAndPayloadLimit(t *testing.T) {
	p := NewParser(10, nil, slog.Default())
	if _, _, err := p.Parse("edge/edge-1/status", validPayload(), 1, false); err != ErrPayloadTooLarge {
		t.Fatalf("large payload error = %v", err)
	}
	p.MaxPayloadBytes = 1024
	payload := []byte(`{"schema":"edge-status/v1","messageId":"m","edgeId":"other","sourceTimestamp":"2026-09-20T00:00:00Z","data":{}}`)
	if _, _, err := p.Parse("edge/edge-1/status", payload, 1, false); err != ErrIdentityMismatch {
		t.Fatalf("identity error = %v", err)
	}
	if _, _, err := p.Parse("edge/edge-1/unknown", payload, 1, false); err != ErrTopicNotSubscribed {
		t.Fatalf("topic error = %v", err)
	}
}

func TestParserToleratesContractViolationButReportsIt(t *testing.T) {
	p := NewParser(1024, nil, slog.Default())
	payload := []byte(`{"schema":"edge-status/v1","messageId":"m","edgeId":"edge-1","sourceTimestamp":"2026-09-20T00:00:00Z","data":{},"unknown":42}`)
	message, violations, err := p.Parse("edge/edge-1/status", payload, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if message.Kind() != KindEdgeStatus || len(violations) != 2 {
		t.Fatalf("message=%v violations=%v", message.Kind(), violations)
	}
}
