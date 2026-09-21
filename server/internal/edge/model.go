// Package edge owns the Cloud Edge projection.
package edge

import (
	"errors"
	"time"
)

type Status string

const (
	StatusOnline  Status = "ONLINE"
	StatusOffline Status = "OFFLINE"
)

var (
	ErrNotFound = errors.New("数据不存在")
	ErrInvalid  = errors.New("参数错误")
)

// Edge contains only the M2 Cloud projection. It intentionally has no MQTT
// envelope, topic, source timestamp, or collector payload fields.
type Edge struct {
	EdgeID       string    `gorm:"column:edge_id" json:"edgeId"`
	Status       Status    `gorm:"column:status" json:"status"`
	RegisteredAt time.Time `gorm:"column:registered_at" json:"registeredAt"`
	LastSeenAt   time.Time `gorm:"column:last_seen_at" json:"lastSeenAt"`
}

func (Edge) TableName() string { return "edge" }

// Observation is the only domain input produced by an ingress adapter.
type Observation struct {
	EdgeID     string
	Status     Status
	ReceivedAt time.Time
}

type PageQuery struct {
	Page     int
	PageSize int
	Status   *Status
	EdgeID   string
}

type Page struct {
	Records  []Edge `json:"records"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
