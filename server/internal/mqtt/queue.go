package mqtt

import "sync"

type inbound struct {
	Delivery   *Delivery
	Ack        func() error
	Generation uint64
}

// reliableQueue rejects new work when full. The caller must leave QoS1
// unacknowledged and reconnect so the persistent session can redeliver it.
type reliableQueue struct {
	mu    sync.Mutex
	items []*inbound
	limit int
}

func newReliableQueue(limit int) *reliableQueue { return &reliableQueue{limit: limit} }
func (q *reliableQueue) push(v *inbound) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.limit {
		return false
	}
	q.items = append(q.items, v)
	return true
}
func (q *reliableQueue) pop() *inbound {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	v := q.items[0]
	q.items = q.items[1:]
	return v
}
func (q *reliableQueue) len() int { q.mu.Lock(); defer q.mu.Unlock(); return len(q.items) }

// rawQueue preserves the newest observations when overloaded.
type rawQueue struct {
	mu    sync.Mutex
	items []*inbound
	limit int
	drops uint64
}

func newRawQueue(limit int) *rawQueue { return &rawQueue{limit: limit} }
func (q *rawQueue) push(v *inbound) (dropped *inbound) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.limit {
		dropped, q.items = q.items[0], q.items[1:]
		q.drops++
	}
	q.items = append(q.items, v)
	return dropped
}
func (q *rawQueue) pop() *inbound {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	v := q.items[0]
	q.items = q.items[1:]
	return v
}
func (q *rawQueue) len() int        { q.mu.Lock(); defer q.mu.Unlock(); return len(q.items) }
func (q *rawQueue) dropped() uint64 { q.mu.Lock(); defer q.mu.Unlock(); return q.drops }
