package mqtt

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func validPayload() []byte {
	return []byte(`{"schema":"device-event/v1","messageId":"m-1","edgeId":"edge-1","deviceId":"device-1","timestamp":"2026-09-20T01:02:03.123456789+08:00","data":{"name":"changed"},"futureField":true}`)
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
	payload := []byte(`{"schema":"edge-status/v1","messageId":"m","edgeId":"other","timestamp":"2026-09-20T00:00:00Z","data":{}}`)
	if _, _, err := p.Parse("edge/edge-1/status", payload, 1, false); err != ErrIdentityMismatch {
		t.Fatalf("identity error = %v", err)
	}
	if _, _, err := p.Parse("edge/edge-1/unknown", payload, 1, false); err != ErrTopicNotSubscribed {
		t.Fatalf("topic error = %v", err)
	}
}

func TestParserToleratesContractViolationButReportsIt(t *testing.T) {
	p := NewParser(1024, nil, slog.Default())
	payload := []byte(`{"schema":"raw-register-snapshot/v1","messageId":"m","edgeId":"edge-1","deviceId":"device-1","timestamp":"2026-09-20T00:00:00Z","data":{},"unknown":42}`)
	message, violations, err := p.Parse("edge/edge-1/device/device-1/raw", payload, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if message.Kind() != KindRawRegisterSnapshot || len(violations) != 2 {
		t.Fatalf("message=%v violations=%v", message.Kind(), violations)
	}
}

func TestParserValidatesKindSpecificDeliveryContract(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		fixture  string
		qos      byte
		retained bool
		want     []ContractViolation
	}{
		{name: "edge status live", topic: "edge/edge-01/status", fixture: "edge-status.json", qos: 1},
		{name: "edge status retained replay", topic: "edge/edge-01/status", fixture: "edge-status.json", qos: 1, retained: true},
		{name: "edge status wrong qos", topic: "edge/edge-01/status", fixture: "edge-status.json", qos: 0, want: []ContractViolation{ViolationQoS}},
		{name: "device status live", topic: "edge/edge-01/device/device-01/status", fixture: "device-status.json", qos: 1},
		{name: "device status retained replay", topic: "edge/edge-01/device/device-01/status", fixture: "device-status.json", qos: 1, retained: true},
		{name: "device status wrong qos", topic: "edge/edge-01/device/device-01/status", fixture: "device-status.json", qos: 2, want: []ContractViolation{ViolationQoS}},
		{name: "raw live", topic: "edge/edge-01/device/device-01/raw", fixture: "raw-register-snapshot.json", want: nil},
		{name: "raw retained", topic: "edge/edge-01/device/device-01/raw", fixture: "raw-register-snapshot.json", retained: true, want: []ContractViolation{ViolationRetained}},
		{name: "raw wrong qos", topic: "edge/edge-01/device/device-01/raw", fixture: "raw-register-snapshot.json", qos: 1, want: []ContractViolation{ViolationQoS}},
		{name: "event live", topic: "edge/edge-01/device/device-01/event", fixture: "device-event.json", qos: 1},
		{name: "event retained", topic: "edge/edge-01/device/device-01/event", fixture: "device-event.json", qos: 1, retained: true, want: []ContractViolation{ViolationRetained}},
		{name: "event wrong qos and retained", topic: "edge/edge-01/device/device-01/event", fixture: "device-event.json", retained: true, want: []ContractViolation{ViolationQoS, ViolationRetained}},
	}

	parser := NewParser(1<<20, nil, slog.Default())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, violations, err := parser.Parse(test.topic, readMQTTV1Fixture(t, test.fixture), test.qos, test.retained)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(append([]ContractViolation(nil), violations...), test.want) {
				t.Fatalf("violations = %v, want %v", violations, test.want)
			}
			if message == nil {
				t.Fatal("Parse() returned a nil message")
			}
		})
	}
}

func TestParserAcceptsEdgeCollectorWireFixtures(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		kind     MessageKind
		deviceID string
		qos      byte
		retained bool
	}{
		{name: "edge-status.json", topic: "edge/edge-01/status", kind: KindEdgeStatus, qos: 1},
		{name: "device-status.json", topic: "edge/edge-01/device/device-01/status", kind: KindDeviceStatus, deviceID: "device-01", qos: 1},
		{name: "raw-register-snapshot.json", topic: "edge/edge-01/device/device-01/raw", kind: KindRawRegisterSnapshot, deviceID: "device-01", qos: 0},
		{name: "device-event.json", topic: "edge/edge-01/device/device-01/event", kind: KindDeviceEvent, deviceID: "device-01", qos: 1},
	}

	parser := NewParser(1<<20, nil, slog.Default())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, _, err := parser.Parse(test.topic, readMQTTV1Fixture(t, test.name), test.qos, test.retained)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if message.Kind() != test.kind {
				t.Fatalf("message kind = %q, want %q", message.Kind(), test.kind)
			}
			metadata := message.Metadata()
			if metadata.SourceTimestamp.Location() != time.UTC {
				t.Fatalf("source timestamp location = %v, want UTC", metadata.SourceTimestamp.Location())
			}
			if test.deviceID == "" {
				if metadata.DeviceID != nil {
					t.Fatalf("device ID = %q, want omitted", *metadata.DeviceID)
				}
			} else if metadata.DeviceID == nil || *metadata.DeviceID != test.deviceID {
				t.Fatalf("device ID = %v, want %q", metadata.DeviceID, test.deviceID)
			}
		})
	}
}

func TestParserRejectsSourceTimestampOnlyEnvelope(t *testing.T) {
	parser := NewParser(1024, nil, slog.Default())
	payload := []byte(`{"schema":"edge-status/v1","messageId":"legacy","edgeId":"edge-01","sourceTimestamp":"2026-09-20T00:00:00Z","data":{"online":true}}`)
	if _, _, err := parser.Parse("edge/edge-01/status", payload, 1, false); err != ErrMalformedEnvelope {
		t.Fatalf("sourceTimestamp-only payload error = %v, want %v", err, ErrMalformedEnvelope)
	}
}

func readMQTTV1Fixture(t *testing.T, name string) []byte {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "../../testdata/mqtt-v1", name))
	if err != nil {
		t.Fatalf("read MQTT v1 fixture %s: %v", name, err)
	}
	return payload
}
