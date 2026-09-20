package mqtt

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct {
	done       chan struct{}
	once       sync.Once
	subscribed atomic.Bool
}

func (c *fakeClient) Subscribe(context.Context, []Subscription) error {
	c.subscribed.Store(true)
	return nil
}
func (c *fakeClient) Close(context.Context) error { c.once.Do(func() { close(c.done) }); return nil }
func (c *fakeClient) Done() <-chan struct{}       { return c.done }

type fakeFactory struct {
	mu            sync.Mutex
	clients       []*fakeClient
	handlers      []PublishHandler
	connectErrors []error
}

func (f *fakeFactory) Connect(_ context.Context, _ uint64, handler PublishHandler) (Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.connectErrors) > 0 {
		err := f.connectErrors[0]
		f.connectErrors = f.connectErrors[1:]
		return nil, err
	}
	c := &fakeClient{done: make(chan struct{})}
	f.clients = append(f.clients, c)
	f.handlers = append(f.handlers, handler)
	return c, nil
}
func (f *fakeFactory) emit(index int, message Received) {
	f.mu.Lock()
	handler := f.handlers[index]
	f.mu.Unlock()
	handler(message)
}
func (f *fakeFactory) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.clients) }

func eventMessage() Received {
	return Received{Topic: "edge/edge-1/device/device-1/event", QoS: 1, Payload: validPayload()}
}
func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met")
}

func TestRuntimeReadyManualAckAndGracefulStop(t *testing.T) {
	cfg := enabledConfig()
	cfg.ReconnectMin = time.Millisecond
	cfg.ReconnectMax = 2 * time.Millisecond
	factory := &fakeFactory{}
	var accepted atomic.Int32
	consumer := ConsumerFunc(func(context.Context, IngressMessage) DeliveryOutcome { accepted.Add(1); return OutcomeAccepted })
	r, err := NewRuntime(cfg, consumer, factory, NewMetrics(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return r.Ready() && factory.count() == 1 })
	var acked atomic.Bool
	m := eventMessage()
	m.Ack = func() error { acked.Store(true); return nil }
	factory.emit(0, m)
	waitUntil(t, func() bool { return accepted.Load() == 1 && acked.Load() })
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if got := r.Status().State; got != StateStopped {
		t.Fatalf("state = %s", got)
	}
}

func TestRuntimeDrainsAcceptedQoS1BeforeDisconnect(t *testing.T) {
	cfg := enabledConfig()
	cfg.ReconnectMin = time.Millisecond
	cfg.ReconnectMax = 2 * time.Millisecond
	factory := &fakeFactory{}
	r, err := NewRuntime(cfg, ConsumerFunc(func(context.Context, IngressMessage) DeliveryOutcome { return OutcomeAccepted }), factory, NewMetrics(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return r.Ready() && factory.count() == 1 })
	var acked atomic.Bool
	m := eventMessage()
	m.Ack = func() error { acked.Store(true); return nil }
	factory.emit(0, m)
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if !acked.Load() {
		t.Fatal("accepted QoS1 message was not acknowledged during shutdown")
	}
}

func TestRuntimeRetryReplacesConnectionGeneration(t *testing.T) {
	cfg := enabledConfig()
	cfg.ReconnectMin = time.Millisecond
	cfg.ReconnectMax = 2 * time.Millisecond
	factory := &fakeFactory{}
	var calls atomic.Int32
	consumer := ConsumerFunc(func(context.Context, IngressMessage) DeliveryOutcome {
		if calls.Add(1) == 1 {
			return OutcomeRetry
		}
		return OutcomeAccepted
	})
	r, err := NewRuntime(cfg, consumer, factory, NewMetrics(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return r.Ready() && factory.count() == 1 })
	var oldAck atomic.Bool
	m := eventMessage()
	m.Ack = func() error { oldAck.Store(true); return nil }
	factory.emit(0, m)
	waitUntil(t, func() bool { return factory.count() >= 2 })
	if oldAck.Load() {
		t.Fatal("retry message was acknowledged")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeReportsFactoryFailureAndCanStop(t *testing.T) {
	cfg := enabledConfig()
	cfg.ReconnectMin = time.Millisecond
	cfg.ReconnectMax = 2 * time.Millisecond
	cfg.ConnectTimeout = 20 * time.Millisecond
	factory := &fakeFactory{}
	for i := 0; i < 10; i++ {
		factory.connectErrors = append(factory.connectErrors, errors.New("not authorized"))
	}
	r, err := NewRuntime(cfg, ConsumerFunc(func(context.Context, IngressMessage) DeliveryOutcome { return OutcomeRejected }), factory, NewMetrics(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return r.Status().RecentErrorCode == "AUTH_REJECTED" })
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeRequiresConsumer(t *testing.T) {
	_, err := NewRuntime(enabledConfig(), nil, nil, nil, nil)
	if err == nil {
		t.Fatal("nil consumer accepted")
	}
}
