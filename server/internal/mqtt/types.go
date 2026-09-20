package mqtt

import (
	"context"
	"encoding/json"
	"time"
)

// Protocol identifies the MQTT wire protocol used by Runtime.
type Protocol string

const (
	ProtocolMQTT5   Protocol = "mqtt5"
	ProtocolMQTT311 Protocol = "mqtt311"
)

// RuntimeState is deliberately finite. A transient connection failure is
// represented by RECONNECTING and RecentError, not by an additional ERROR
// state.
type RuntimeState string

const (
	StateDisabled     RuntimeState = "DISABLED"
	StateConnecting   RuntimeState = "CONNECTING"
	StateSubscribing  RuntimeState = "SUBSCRIBING"
	StateReady        RuntimeState = "READY"
	StateReconnecting RuntimeState = "RECONNECTING"
	StateStopping     RuntimeState = "STOPPING"
	StateStopped      RuntimeState = "STOPPED"
)

// DeliveryOutcome is returned by Consumer for a successfully parsed message.
type DeliveryOutcome string

const (
	OutcomeAccepted DeliveryOutcome = "accepted"
	OutcomeRejected DeliveryOutcome = "rejected"
	OutcomeRetry    DeliveryOutcome = "retry"
)

// Config is independent from the application config package so the MQTT
// module can be adopted before the app wiring is ready.
type Config struct {
	Enabled   bool
	BrokerURL string
	Protocol  Protocol
	// ClientID identifies the persistent subscription session. It must change
	// whenever BrokerURL, Protocol, or TopicPrefix changes so an old session
	// cannot continue receiving the previous subscription set.
	ClientID          string
	TopicPrefix       string
	Username          string
	Password          string
	TLS               TLSConfig
	KeepAlive         time.Duration
	ConnectTimeout    time.Duration
	ReconnectMin      time.Duration
	ReconnectMax      time.Duration
	SessionExpiry     time.Duration
	MaxPayloadBytes   int
	ReliableQueueSize int
	RawQueueSize      int
	ConsumerTimeout   time.Duration
	ShutdownTimeout   time.Duration
	Jitter            float64
}

type TLSConfig struct {
	CACertFile     string
	ClientCertFile string
	ClientKeyFile  string
}

func DefaultConfig() Config {
	return Config{
		Enabled: false, Protocol: ProtocolMQTT5, TopicPrefix: "edge",
		KeepAlive: 30 * time.Second, ConnectTimeout: 10 * time.Second,
		ReconnectMin: time.Second, ReconnectMax: 30 * time.Second,
		SessionExpiry: 24 * time.Hour, MaxPayloadBytes: 1 << 20,
		ReliableQueueSize: 1024, RawQueueSize: 256,
		ConsumerTimeout: 5 * time.Second, ShutdownTimeout: 10 * time.Second,
		Jitter: 0.2,
	}
}

// IngressMetadata is shared by all four typed ingress messages.
type IngressMetadata struct {
	Topic           string
	Schema          string
	MessageID       string
	EdgeID          string
	DeviceID        *string
	SourceTimestamp time.Time
	ReceivedAt      time.Time
	QoS             byte
	Retained        bool
}

type IngressMessage interface {
	Metadata() IngressMetadata
	Kind() MessageKind
}

type MessageKind string

const (
	KindEdgeStatus          MessageKind = "edge-status"
	KindDeviceStatus        MessageKind = "device-status"
	KindRawRegisterSnapshot MessageKind = "raw-register-snapshot"
	KindDeviceEvent         MessageKind = "device-event"
)

// Data is intentionally kept as JSON at the ingest seam. Future domain
// adapters own the schema-specific interpretation and therefore raw collector
// concepts do not leak into Cloud domain modules.
type EdgeStatusMessage struct {
	IngressMetadata
	Data json.RawMessage
}
type DeviceStatusMessage struct {
	IngressMetadata
	Data json.RawMessage
}
type RawRegisterSnapshotMessage struct {
	IngressMetadata
	Data json.RawMessage
}
type DeviceEventMessage struct {
	IngressMetadata
	Data json.RawMessage
}

func (m EdgeStatusMessage) Metadata() IngressMetadata          { return m.IngressMetadata }
func (m DeviceStatusMessage) Metadata() IngressMetadata        { return m.IngressMetadata }
func (m RawRegisterSnapshotMessage) Metadata() IngressMetadata { return m.IngressMetadata }
func (m DeviceEventMessage) Metadata() IngressMetadata         { return m.IngressMetadata }
func (EdgeStatusMessage) Kind() MessageKind                    { return KindEdgeStatus }
func (DeviceStatusMessage) Kind() MessageKind                  { return KindDeviceStatus }
func (RawRegisterSnapshotMessage) Kind() MessageKind           { return KindRawRegisterSnapshot }
func (DeviceEventMessage) Kind() MessageKind                   { return KindDeviceEvent }

type ContractViolation string

const (
	ViolationQoS      ContractViolation = "qos"
	ViolationRetained ContractViolation = "retained"
)

type Delivery struct {
	Message    IngressMessage
	Generation uint64
	QoS        byte
	Retained   bool
	Violations []ContractViolation
	ack        func() error
}

// Consumer is the only business-facing runtime port. The context is
// cancelled on timeout or shutdown. A retry result intentionally leaves QoS1
// unacknowledged; the runtime reconnects to force persistent-session redelivery.
type Consumer interface {
	Consume(context.Context, IngressMessage) DeliveryOutcome
}

type ConsumerFunc func(context.Context, IngressMessage) DeliveryOutcome

func (f ConsumerFunc) Consume(ctx context.Context, m IngressMessage) DeliveryOutcome {
	return f(ctx, m)
}

type Readiness interface{ Ready() bool }

type Status struct {
	Enabled         bool
	State           RuntimeState
	Protocol        Protocol
	ConnectedAt     time.Time
	DisconnectedAt  time.Time
	LastMessageAt   time.Time
	RecentErrorAt   time.Time
	RecentErrorCode string
	RetryCount      int
	NextRetryAt     time.Time
	Subscriptions   map[string]bool
}
