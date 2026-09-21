// Package device owns the Cloud Device current projection.
package device

import (
	"errors"
	"time"
)

type CommunicationStatus string

const (
	StatusInitial  CommunicationStatus = "INITIAL"
	StatusOnline   CommunicationStatus = "ONLINE"
	StatusDegraded CommunicationStatus = "DEGRADED"
	StatusOffline  CommunicationStatus = "OFFLINE"
)

var (
	ErrNotFound       = errors.New("数据不存在")
	ErrInvalid        = errors.New("参数错误")
	ErrParentNotFound = errors.New("父 Edge 不存在")
)

// Device is the complete M3 current projection. It deliberately contains no
// MQTT envelope, topic, message identity, raw payload, or collector-internal
// configuration fields.
type Device struct {
	DeviceID            string              `gorm:"column:device_id" json:"deviceId"`
	EdgeID              string              `gorm:"column:edge_id" json:"edgeId"`
	SourceDeviceID      string              `gorm:"column:source_device_id" json:"sourceDeviceId"`
	CommunicationStatus CommunicationStatus `gorm:"column:communication_status" json:"communicationStatus"`
	RegisteredAt        time.Time           `gorm:"column:registered_at" json:"registeredAt"`
	LastSeenAt          time.Time           `gorm:"column:last_seen_at" json:"lastSeenAt"`
	LastAttemptAt       *time.Time          `gorm:"column:last_attempt_at" json:"lastAttemptAt"`
	LastSuccessAt       *time.Time          `gorm:"column:last_success_at" json:"lastSuccessAt"`
	CommunicationError  *string             `gorm:"column:communication_error" json:"communicationError"`
}

func (Device) TableName() string { return "device" }

// Snapshot is the domain-level DeviceStatus current-state assertion. It has
// no dependency on MQTT or its delivery metadata.
type Snapshot struct {
	CommunicationStatus CommunicationStatus
	LastAttemptAt       *time.Time
	LastSuccessAt       *time.Time
	CommunicationError  *string
}

// Observation is the only input accepted by the Device domain from ingress.
// ReceivedAt is the Cloud observation time; the two source times retain their
// Collector-side meaning.
type Observation struct {
	EdgeID         string
	SourceDeviceID string
	Snapshot       Snapshot
	ReceivedAt     time.Time
}

type PageQuery struct {
	Page           int
	PageSize       int
	EdgeID         string
	DeviceID       string
	SourceDeviceID string
	Status         *CommunicationStatus
}

type Page struct {
	Records  []Device `json:"records"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

func validStatus(status CommunicationStatus) bool {
	switch status {
	case StatusInitial, StatusOnline, StatusDegraded, StatusOffline:
		return true
	default:
		return false
	}
}

func normalizeSnapshot(snapshot Snapshot) Snapshot {
	if snapshot.LastAttemptAt != nil {
		value := snapshot.LastAttemptAt.UTC()
		snapshot.LastAttemptAt = &value
	}
	if snapshot.LastSuccessAt != nil {
		value := snapshot.LastSuccessAt.UTC()
		snapshot.LastSuccessAt = &value
	}
	if snapshot.CommunicationError != nil && *snapshot.CommunicationError == "" {
		snapshot.CommunicationError = nil
	}
	return snapshot
}

func normalizeDevice(value Device) Device {
	value.RegisteredAt = value.RegisteredAt.UTC()
	value.LastSeenAt = value.LastSeenAt.UTC()
	if value.LastAttemptAt != nil {
		at := value.LastAttemptAt.UTC()
		value.LastAttemptAt = &at
	}
	if value.LastSuccessAt != nil {
		at := value.LastSuccessAt.UTC()
		value.LastSuccessAt = &at
	}
	return value
}
