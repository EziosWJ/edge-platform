// Package command owns the Cloud command journal, durable delivery fact,
// result-projection seam, and the first-send dispatcher boundary.
package command

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	DefaultTTLSeconds    = 30
	MinTTLSeconds        = 1
	MaxTTLSeconds        = 300
	MaxPayloadBytes      = 256 * 1024
	ListPermission       = "command:list"
	DetailPermission     = "command:detail"
	ExecutePermission    = "command:execute"
	CommandStatusPending = "PENDING"
)

var (
	ErrInvalid             = errors.New("参数错误")
	ErrNotFound            = errors.New("数据不存在")
	ErrConflict            = errors.New("请求与已有 Command 冲突")
	ErrForbidden           = errors.New("无权执行 Command")
	ErrPayloadSize         = errors.New("Command payload 超过 256 KiB 限制")
	ErrInvalidResult       = errors.New("invalid command result")
	ErrUnknownCommand      = errors.New("command result references unknown command")
	ErrResultRouteMismatch = errors.New("command result route does not match frozen command")
	ErrResultNameMismatch  = errors.New("command result name does not match command")
)

const (
	CommandStatusAccepted  = "ACCEPTED"
	CommandStatusRejected  = "REJECTED"
	CommandStatusExpired   = "EXPIRED"
	CommandStatusSucceeded = "SUCCEEDED"
	CommandStatusFailed    = "FAILED"
)

var validResultStatuses = map[string]struct{}{
	CommandStatusAccepted: {}, CommandStatusRejected: {}, CommandStatusExpired: {},
	CommandStatusSucceeded: {}, CommandStatusFailed: {},
}

type ResultDisposition string

const (
	ResultApplied      ResultDisposition = "applied"
	ResultDuplicate    ResultDisposition = "duplicate"
	ResultConflict     ResultDisposition = "conflict"
	ResultLateAccepted ResultDisposition = "late_accepted"
)

type ResultProjection struct {
	Disposition ResultDisposition
}

// JSONDocument keeps JSON text lossless at the persistence boundary. In
// particular, it never passes through float64 or a generic map on the way to
// PostgreSQL JSONB/SQLite JSON storage.
type JSONDocument string

func (j JSONDocument) Value() (driver.Value, error) {
	if j == "" {
		return nil, nil
	}
	return string(j), nil
}

func (j *JSONDocument) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*j = ""
	case string:
		*j = JSONDocument(v)
	case []byte:
		*j = JSONDocument(string(v))
	default:
		return fmt.Errorf("scan JSON document from %T", value)
	}
	return nil
}

type Command struct {
	CommandID         string       `gorm:"column:command_id;primaryKey" json:"commandId"`
	DeviceID          string       `gorm:"column:device_id" json:"deviceId"`
	EdgeID            string       `gorm:"column:edge_id" json:"edgeId"`
	SourceDeviceID    string       `gorm:"column:source_device_id" json:"sourceDeviceId"`
	Topic             string       `gorm:"column:topic" json:"-"`
	Name              string       `gorm:"column:name" json:"name"`
	Args              JSONDocument `gorm:"column:args;type:jsonb" json:"-"`
	RequestedBy       int64        `gorm:"column:requested_by" json:"requestedBy"`
	RequestHash       string       `gorm:"column:request_hash" json:"-"`
	IssuedAt          time.Time    `gorm:"column:issued_at" json:"issuedAt"`
	ExpiresAt         time.Time    `gorm:"column:expires_at" json:"expiresAt"`
	Status            string       `gorm:"column:status" json:"status"`
	DeliveryExpiredAt *time.Time   `gorm:"column:delivery_expired_at" json:"deliveryExpiredAt,omitempty"`
	EdgeReceivedAt    *time.Time   `gorm:"column:edge_received_at" json:"edgeReceivedAt,omitempty"`
	StartedAt         *time.Time   `gorm:"column:started_at" json:"startedAt,omitempty"`
	CompletedAt       *time.Time   `gorm:"column:completed_at" json:"completedAt,omitempty"`
	ResultReceivedAt  *time.Time   `gorm:"column:result_received_at" json:"resultReceivedAt,omitempty"`
	Result            JSONDocument `gorm:"column:result;type:jsonb" json:"-"`
	ErrorType         *string      `gorm:"column:error_type" json:"errorType,omitempty"`
	ErrorMessage      *string      `gorm:"column:error_message" json:"errorMessage,omitempty"`
}

func (Command) TableName() string { return "command" }

type Delivery struct {
	CommandID     string     `gorm:"column:command_id;primaryKey" json:"commandId"`
	Topic         string     `gorm:"column:topic" json:"topic"`
	Payload       []byte     `gorm:"column:payload" json:"-"`
	ExpiresAt     time.Time  `gorm:"column:expires_at" json:"expiresAt"`
	AttemptCount  int        `gorm:"column:attempt_count" json:"attemptCount"`
	LastAttemptAt *time.Time `gorm:"column:last_attempt_at" json:"lastAttemptAt,omitempty"`
	LastError     *string    `gorm:"column:last_error" json:"lastError,omitempty"`
	NextAttemptAt *time.Time `gorm:"column:next_attempt_at" json:"nextAttemptAt,omitempty"`
}

func (Delivery) TableName() string { return "command_delivery" }

type CreateInput struct {
	CommandID  string
	DeviceID   string
	Name       string
	Args       json.RawMessage
	TTLSeconds *int
}

// ResultObservation is the Cloud ingress-to-domain projection seam. Result
// contains the original JSON bytes, including a literal null, and the two
// clocks remain separate: Edge timestamps are carried alongside Cloud's
// ingress ReceivedAt.
type ResultObservation struct {
	CommandID       string
	EdgeID          string
	SourceDeviceID  string
	Name            string
	Status          string
	EdgeReceivedAt  time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CloudReceivedAt time.Time
	Result          JSONDocument
	ErrorType       *string
	ErrorMessage    *string
}

func validResultStatus(status string) bool {
	_, ok := validResultStatuses[status]
	return ok
}

// validCommandStatus is the complete business-state set exposed by Command
// queries. Result ingress deliberately continues to use validResultStatus,
// because PENDING is a Cloud-created state and is never an Edge result.
func validCommandStatus(status string) bool {
	return status == CommandStatusPending || validResultStatus(status)
}

func validStatusForQuery(status string) bool {
	return validCommandStatus(status)
}

type CommandView struct {
	CommandID         string          `json:"commandId"`
	DeviceID          string          `json:"deviceId"`
	EdgeID            string          `json:"edgeId"`
	SourceDeviceID    string          `json:"sourceDeviceId"`
	Name              string          `json:"name"`
	Args              json.RawMessage `json:"args"`
	RequestedBy       int64           `json:"requestedBy"`
	IssuedAt          time.Time       `json:"issuedAt"`
	ExpiresAt         time.Time       `json:"expiresAt"`
	Status            string          `json:"status"`
	DeliveryExpiredAt *time.Time      `json:"deliveryExpiredAt,omitempty"`
	EdgeReceivedAt    *time.Time      `json:"edgeReceivedAt,omitempty"`
	StartedAt         *time.Time      `json:"startedAt,omitempty"`
	CompletedAt       *time.Time      `json:"completedAt,omitempty"`
	ResultReceivedAt  *time.Time      `json:"resultReceivedAt,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	ErrorType         *string         `json:"errorType,omitempty"`
	ErrorMessage      *string         `json:"errorMessage,omitempty"`
}

type CommandPageQuery struct {
	Page        int
	PageSize    int
	CommandID   string
	DeviceID    string
	Name        string
	Status      *string
	RequestedBy *int64
}

type CommandPage struct {
	Records  []CommandView `json:"records"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

func (c Command) View() CommandView {
	args := json.RawMessage(c.Args)
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	view := CommandView{
		CommandID: c.CommandID, DeviceID: c.DeviceID, EdgeID: c.EdgeID,
		SourceDeviceID: c.SourceDeviceID, Name: c.Name, Args: append(json.RawMessage(nil), args...),
		RequestedBy: c.RequestedBy, IssuedAt: c.IssuedAt.UTC(), ExpiresAt: c.ExpiresAt.UTC(), Status: c.Status,
		DeliveryExpiredAt: utcTime(c.DeliveryExpiredAt), EdgeReceivedAt: utcTime(c.EdgeReceivedAt),
		StartedAt: utcTime(c.StartedAt), CompletedAt: utcTime(c.CompletedAt), ResultReceivedAt: utcTime(c.ResultReceivedAt),
		ErrorType: c.ErrorType, ErrorMessage: c.ErrorMessage,
	}
	if c.Result != "" {
		view.Result = append(json.RawMessage(nil), []byte(c.Result)...)
	}
	return view
}

func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}
