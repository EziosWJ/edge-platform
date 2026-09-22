package realtime

import (
	"sync"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/gorilla/websocket"
)

const defaultOutboundQueueSize = 64

const slowClientCloseReason = "slow client outbound queue full"

// Hub is the process-local, best-effort fanout seam between committed
// CurrentValue projections and WebSocket connections. It never performs
// network I/O in TryPublish.
type Hub struct {
	mu          sync.Mutex
	connections map[*connection]struct{}
	queueSize   int
	closed      bool
}

func NewHub(queueSize int) *Hub {
	if queueSize <= 0 {
		queueSize = defaultOutboundQueueSize
	}
	return &Hub{connections: make(map[*connection]struct{}), queueSize: queueSize}
}

// TryPublish implements datapoint.CurrentValueNotifier. A closed hub rejects
// new work; a live hub accepts the notification without waiting on clients.
func (h *Hub) TryPublish(change datapoint.CurrentValueChange) bool {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return false
	}
	connections := make([]*connection, 0, len(h.connections))
	for connection := range h.connections {
		connections = append(connections, connection)
	}
	h.mu.Unlock()

	accepted := true
	for _, connection := range connections {
		if !connection.isSubscribed(change.DeviceID, change.PointKey) {
			continue
		}
		if !connection.enqueueUpdate(change) {
			accepted = false
			// Mark the connection closed synchronously so no subsequent
			// notification can enter its queue. The close frame and socket
			// close happen asynchronously and therefore never backpressure
			// the post-commit notifier or MQTT ingest.
			if !connection.isClosed() {
				connection.closeSlowClient()
			}
		}
	}
	return accepted
}

func (c *connection) closeSlowClient() {
	if !c.markClosed(websocket.CloseTryAgainLater, slowClientCloseReason) {
		return
	}
	go c.finishClose(websocket.CloseTryAgainLater, slowClientCloseReason)
}

func (h *Hub) register(connection *connection) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	connection.queue = newOutboundQueue(h.queueSize)
	h.connections[connection] = struct{}{}
	return true
}

func (h *Hub) unregister(connection *connection) {
	h.mu.Lock()
	delete(h.connections, connection)
	h.mu.Unlock()
}

func (h *Hub) closeAll(code int, reason string) {
	h.mu.Lock()
	h.closed = true
	connections := make([]*connection, 0, len(h.connections))
	for connection := range h.connections {
		connections = append(connections, connection)
	}
	h.mu.Unlock()
	for _, connection := range connections {
		connection.closeWithCode(code, reason)
	}
}

type pointSubscription struct {
	deviceID string
	pointKey string
}

type queuedMessage struct {
	value          any
	updateKey      string
	updateRevision int64
	coalescible    bool
}

type outboundQueue struct {
	mu       sync.Mutex
	capacity int
	items    []queuedMessage
	pending  map[string]int
	signal   chan struct{}
	closed   bool
}

func newOutboundQueue(capacity int) *outboundQueue {
	if capacity <= 0 {
		capacity = defaultOutboundQueueSize
	}
	return &outboundQueue{
		capacity: capacity,
		pending:  make(map[string]int),
		signal:   make(chan struct{}, 1),
	}
}

func (q *outboundQueue) enqueue(value any) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || len(q.items) >= q.capacity {
		return false
	}
	q.items = append(q.items, queuedMessage{value: value})
	q.notifyLocked()
	return true
}

func (q *outboundQueue) enqueueUpdate(change datapoint.CurrentValueChange) bool {
	key := change.DeviceID + "\x00" + change.PointKey
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	if index, ok := q.pending[key]; ok {
		if q.items[index].updateRevision >= change.Revision {
			return true
		}
		q.items[index] = queuedMessage{
			value:          updateMessageFromChange(change),
			updateKey:      key,
			updateRevision: change.Revision,
			coalescible:    true,
		}
		q.notifyLocked()
		return true
	}
	if len(q.items) >= q.capacity {
		return false
	}
	q.pending[key] = len(q.items)
	q.items = append(q.items, queuedMessage{
		value:          updateMessageFromChange(change),
		updateKey:      key,
		updateRevision: change.Revision,
		coalescible:    true,
	})
	q.notifyLocked()
	return true
}

func (q *outboundQueue) notifyLocked() {
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

func (q *outboundQueue) dequeue() (any, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	if item.coalescible {
		delete(q.pending, item.updateKey)
	}
	for key, index := range q.pending {
		if index == 0 {
			delete(q.pending, key)
			continue
		}
		q.pending[key] = index - 1
	}
	return item.value, true
}

func (q *outboundQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.items = nil
	q.pending = nil
	q.mu.Unlock()
}

func (q *outboundQueue) removeUpdates(point pointIdentity) {
	key := pointSubscription{deviceID: point.DeviceID, pointKey: point.PointKey}
	updateKey := key.deviceID + "\x00" + key.pointKey
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	filtered := q.items[:0]
	for _, item := range q.items {
		if item.coalescible && item.updateKey == updateKey {
			continue
		}
		filtered = append(filtered, item)
	}
	q.items = filtered
	q.pending = make(map[string]int)
	for index, item := range q.items {
		if item.coalescible {
			q.pending[item.updateKey] = index
		}
	}
}

func updateMessageFromChange(change datapoint.CurrentValueChange) updateMessage {
	return updateMessage{
		Type:            "update",
		DataPointID:     change.DataPointID,
		DeviceID:        change.DeviceID,
		PointKey:        change.PointKey,
		ValueType:       string(change.ValueType),
		Value:           change.Value,
		Quality:         string(change.Quality),
		SourceTimestamp: cloneTime(change.SourceTimestamp),
		ObservedAt:      cloneTime(change.ObservedAt),
		Revision:        change.Revision,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
