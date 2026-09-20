package mqtt

import "testing"

func TestQueuesHaveIndependentOverloadSemantics(t *testing.T) {
	r := newReliableQueue(1)
	if !r.push(&inbound{}) || r.push(&inbound{}) {
		t.Fatal("reliable queue should reject when full")
	}
	q := newRawQueue(2)
	first, second, third := &inbound{}, &inbound{}, &inbound{}
	if q.push(first) != nil || q.push(second) != nil {
		t.Fatal("raw queue unexpectedly dropped")
	}
	if got := q.push(third); got != first {
		t.Fatalf("dropped = %p, want first %p", got, first)
	}
	if q.pop() != second || q.pop() != third || q.pop() != nil {
		t.Fatal("raw queue did not retain newest messages")
	}
	if q.dropped() != 1 {
		t.Fatalf("drops = %d", q.dropped())
	}
}
