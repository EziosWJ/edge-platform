package mqtt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrTopicNotSubscribed = errors.New("topic is not one of the MQTT v1 ingress filters")
	ErrPayloadTooLarge    = errors.New("mqtt payload exceeds configured limit")
	ErrMalformedEnvelope  = errors.New("malformed MQTT v1 envelope")
	ErrIdentityMismatch   = errors.New("topic and envelope identity do not match")
	ErrUnknownSchema      = errors.New("unknown MQTT v1 schema")
)

type Topic struct {
	Kind     MessageKind
	Prefix   string
	EdgeID   string
	DeviceID string
}

func Filters(prefix string) []string {
	return []string{
		prefix + "/+/status",
		prefix + "/+/device/+/status",
		prefix + "/+/device/+/raw",
		prefix + "/+/device/+/event",
	}
}

func ParseTopic(prefix, topic string) (Topic, error) {
	if err := validatePrefix(prefix); err != nil {
		return Topic{}, err
	}
	parts := strings.Split(topic, "/")
	base := strings.Split(prefix, "/")
	if len(parts) == len(base)+2 && equalParts(parts[:len(base)], base) && parts[len(base)+1] == "status" {
		if !validTopicID(parts[len(base)]) {
			return Topic{}, ErrTopicNotSubscribed
		}
		return Topic{Kind: KindEdgeStatus, Prefix: prefix, EdgeID: parts[len(base)]}, nil
	}
	if len(parts) != len(base)+4 || !equalParts(parts[:len(base)], base) || parts[len(base)] == "" || parts[len(base)+1] != "device" || parts[len(base)+2] == "" {
		return Topic{}, ErrTopicNotSubscribed
	}
	if !validTopicID(parts[len(base)]) || !validTopicID(parts[len(base)+2]) {
		return Topic{}, ErrTopicNotSubscribed
	}
	kind := map[string]MessageKind{"status": KindDeviceStatus, "raw": KindRawRegisterSnapshot, "event": KindDeviceEvent}[parts[len(base)+3]]
	if kind == "" {
		return Topic{}, ErrTopicNotSubscribed
	}
	return Topic{Kind: kind, Prefix: prefix, EdgeID: parts[len(base)], DeviceID: parts[len(base)+2]}, nil
}

func equalParts(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validTopicID(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsAny(value, "/+#\x00")
}

type envelope struct {
	Schema          string          `json:"schema"`
	MessageID       string          `json:"messageId"`
	EdgeID          string          `json:"edgeId"`
	DeviceID        *string         `json:"deviceId"`
	SourceTimestamp string          `json:"sourceTimestamp"`
	Data            json.RawMessage `json:"data"`
}

type Parser struct {
	MaxPayloadBytes int
	TopicPrefix     string
	Metrics         *Metrics
	Logger          *slog.Logger
	Now             func() time.Time
}

func NewParser(maxPayloadBytes int, metrics *Metrics, logger *slog.Logger) *Parser {
	if maxPayloadBytes <= 0 {
		maxPayloadBytes = DefaultConfig().MaxPayloadBytes
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Parser{MaxPayloadBytes: maxPayloadBytes, TopicPrefix: DefaultConfig().TopicPrefix, Metrics: metrics, Logger: logger, Now: time.Now}
}

func (p *Parser) Parse(topic string, payload []byte, qos byte, retained bool) (IngressMessage, []ContractViolation, error) {
	t, err := ParseTopicFromConfiguredParser(p, topic)
	if err != nil {
		p.reject("topic")
		return nil, nil, err
	}
	if len(payload) > p.MaxPayloadBytes {
		p.reject("payload_too_large")
		return nil, nil, ErrPayloadTooLarge
	}
	var raw envelope
	dec := json.NewDecoder(bytes.NewReader(payload))
	if err := dec.Decode(&raw); err != nil || raw.Schema == "" || raw.MessageID == "" || raw.EdgeID == "" || raw.SourceTimestamp == "" || len(raw.Data) == 0 || bytes.Equal(raw.Data, []byte("null")) {
		p.reject("malformed_envelope")
		return nil, nil, ErrMalformedEnvelope
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		p.reject("malformed_envelope")
		return nil, nil, ErrMalformedEnvelope
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw.Data, &object); err != nil || object == nil {
		p.reject("invalid_data")
		return nil, nil, ErrMalformedEnvelope
	}
	if !utf8.ValidString(raw.MessageID) || len([]byte(raw.MessageID)) > 256 || !validTopicID(raw.EdgeID) || (raw.DeviceID != nil && !validTopicID(*raw.DeviceID)) {
		p.reject("invalid_identity")
		return nil, nil, ErrMalformedEnvelope
	}
	source, err := time.Parse(time.RFC3339Nano, raw.SourceTimestamp)
	if err != nil {
		p.reject("invalid_source_timestamp")
		return nil, nil, ErrMalformedEnvelope
	}
	if raw.EdgeID != t.EdgeID || (t.DeviceID != "" && (raw.DeviceID == nil || *raw.DeviceID != t.DeviceID)) || (t.DeviceID == "" && raw.DeviceID != nil && *raw.DeviceID != "") {
		p.reject("identity_mismatch")
		return nil, nil, ErrIdentityMismatch
	}
	if !schemaMatches(t.Kind, raw.Schema) {
		p.reject("unknown_schema")
		return nil, nil, ErrUnknownSchema
	}
	metadata := IngressMetadata{Topic: topic, Schema: raw.Schema, MessageID: raw.MessageID, EdgeID: raw.EdgeID, SourceTimestamp: source.UTC(), ReceivedAt: p.now().UTC(), QoS: qos, Retained: retained}
	if raw.DeviceID != nil {
		deviceID := *raw.DeviceID
		metadata.DeviceID = &deviceID
	}
	violations := make([]ContractViolation, 0, 2)
	if qos != 1 {
		violations = append(violations, ViolationQoS)
		p.violation(string(ViolationQoS))
	}
	if retained {
		violations = append(violations, ViolationRetained)
		p.violation(string(ViolationRetained))
	}
	switch t.Kind {
	case KindEdgeStatus:
		return EdgeStatusMessage{IngressMetadata: metadata, Data: cloneJSON(raw.Data)}, violations, nil
	case KindDeviceStatus:
		return DeviceStatusMessage{IngressMetadata: metadata, Data: cloneJSON(raw.Data)}, violations, nil
	case KindRawRegisterSnapshot:
		return RawRegisterSnapshotMessage{IngressMetadata: metadata, Data: cloneJSON(raw.Data)}, violations, nil
	case KindDeviceEvent:
		return DeviceEventMessage{IngressMetadata: metadata, Data: cloneJSON(raw.Data)}, violations, nil
	default:
		return nil, nil, ErrUnknownSchema
	}
}

func ParseTopicFromConfiguredParser(p *Parser, topic string) (Topic, error) {
	if p == nil {
		return Topic{}, ErrTopicNotSubscribed
	}
	return ParseTopic(p.TopicPrefix, topic)
}

// ParserPrefix is set by Runtime and can also be used by app adapters.
func ParserPrefix(p *Parser) string {
	if p == nil || p.TopicPrefix == "" {
		return DefaultConfig().TopicPrefix
	}
	return p.TopicPrefix
}

func SetParserPrefix(p *Parser, prefix string) error {
	if p == nil {
		return errors.New("nil parser")
	}
	if err := validatePrefix(prefix); err != nil {
		return err
	}
	p.TopicPrefix = prefix
	return nil
}

func schemaMatches(kind MessageKind, schema string) bool {
	return schema == string(kind)+"/v1"
}

func (p *Parser) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func cloneJSON(raw json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), raw...) }

func (p *Parser) reject(reason string) {
	if p.Metrics != nil {
		p.Metrics.Rejections.WithLabelValues(reason).Inc()
	}
	if p.Logger != nil {
		p.Logger.Warn("mqtt ingress rejected", "reason", reason)
	}
}

func (p *Parser) violation(kind string) {
	if p.Metrics != nil {
		p.Metrics.ContractViolations.WithLabelValues(kind).Inc()
	}
	if p.Logger != nil {
		p.Logger.Warn("mqtt ingress contract violation", "kind", kind)
	}
}

func (t Topic) String() string { return fmt.Sprintf("%s/%s", t.Kind, t.EdgeID) }
