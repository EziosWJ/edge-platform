// Package realtime owns the process-local WebSocket transport for semantic
// DataPoint CurrentValue snapshots. It deliberately knows no source mapping
// or collector transport details.
package realtime

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
)

var (
	ErrInvalidTicket           = errors.New("invalid realtime ticket")
	ErrInvalidPoint            = errors.New("invalid realtime point")
	ErrPointNotFound           = errors.New("realtime point not found")
	ErrInvalidBinding          = errors.New("realtime point binding is invalid")
	ErrUnsupported             = errors.New("unsupported realtime operation")
	ErrSessionInvalid          = errors.New("realtime session is invalid")
	errProtocolMessageTooLarge = errors.New("realtime protocol message is too large")
)

const (
	defaultTicketTTL          = 30 * time.Second
	defaultSessionCheckPeriod = 30 * time.Second
	defaultPingPeriod         = 25 * time.Second
	maxProtocolMessageSize    = 256 * 1024
	maxActivePoints           = 1000
	maxRequestIDBytes         = 128
)

// Point is the complete semantic payload exposed by the realtime protocol.
// No mapping, MQTT, Modbus, or source-device fields may be added here.
type Point struct {
	DataPointID     string
	DeviceID        string
	PointKey        string
	ValueType       datapoint.ValueType
	Value           any
	Quality         datapoint.Quality
	SourceTimestamp *time.Time
	ObservedAt      *time.Time
	Revision        int64
}

type PointReader interface {
	ReadCurrent(context.Context, string, string) (Point, error)
}

type Config struct {
	TicketTTL          time.Duration
	SessionCheckPeriod time.Duration
	PingPeriod         time.Duration
	OutboundQueueSize  int
	AllowedOrigins     []string
	Now                func() time.Time
}

type TicketResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
	ExpiresIn int64     `json:"expiresIn"`
}
