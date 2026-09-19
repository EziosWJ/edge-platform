// Package audit owns operation audit events and persistence.
package audit

import "context"

type Metadata struct {
	ActorID                                                   int64
	RequestID, ClientIP, UserAgent, RequestMethod, RequestURL string
}
type Event struct {
	Action, Resource string
	ResourceID       int64
	Summary          string
	Metadata         Metadata
}
type Recorder interface {
	Record(context.Context, Event) error
}
type noop struct{}

func (noop) Record(context.Context, Event) error { return nil }

func Noop() Recorder { return noop{} }
