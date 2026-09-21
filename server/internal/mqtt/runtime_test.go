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

func TestRuntimeReconnectReadinessAndDiagnostics(t *testing.T) {
	cfg := enabledConfig()
	cfg.ReconnectMin = 50 * time.Millisecond
	cfg.ReconnectMax = 50 * time.Millisecond
	cfg.Jitter = 0
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
	factory.emit(0, eventMessage())
	waitUntil(t, func() bool { return !r.Status().NextRetryAt.IsZero() })
	status := r.Status()
	if status.State == StateReady || r.Ready() {
		t.Fatalf("runtime remained ready during reconnect backoff: %+v", status)
	}
	if status.State != StateReconnecting {
		t.Fatalf("reconnect state = %s, want %s", status.State, StateReconnecting)
	}
	if status.DisconnectedAt.IsZero() {
		t.Fatal("active reconnect did not record DisconnectedAt")
	}
	if status.Subscriptions[Filters(cfg.TopicPrefix)[0]] {
		t.Fatal("active reconnect left edge status subscription ready")
	}

	waitUntil(t, func() bool { return factory.count() >= 2 && r.Ready() })
	status = r.Status()
	if status.RetryCount != 0 {
		t.Fatalf("RetryCount after reconnect = %d, want 0", status.RetryCount)
	}
	if !status.NextRetryAt.IsZero() {
		t.Fatalf("NextRetryAt after reconnect = %v, want zero", status.NextRetryAt)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeTimeoutWaitsForConsumerExitBeforeReconnect(t *testing.T) {
	cfg := enabledConfig()
	cfg.ConsumerTimeout = 10 * time.Millisecond
	cfg.ReconnectMin = time.Millisecond
	cfg.ReconnectMax = 2 * time.Millisecond
	cfg.Jitter = 0
	factory := &fakeFactory{}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	var first atomic.Bool
	var active atomic.Int32
	var maximum atomic.Int32
	consumer := ConsumerFunc(func(ctx context.Context, _ IngressMessage) DeliveryOutcome {
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		defer active.Add(-1)
		if first.CompareAndSwap(false, true) {
			close(firstStarted)
			<-ctx.Done()
			<-releaseFirst
			return OutcomeRetry
		}
		close(secondStarted)
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
	message := eventMessage()
	message.Ack = func() error { oldAck.Store(true); return nil }
	factory.emit(0, message)
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first consumer call did not start")
	}
	select {
	case <-secondStarted:
		t.Fatal("redelivery started before the timed-out consumer exited")
	case <-time.After(30 * time.Millisecond):
	}
	if got := factory.count(); got != 1 {
		t.Fatalf("connection count before timed-out consumer exit = %d, want 1", got)
	}

	close(releaseFirst)
	waitUntil(t, func() bool { return factory.count() >= 2 })
	waitUntil(t, func() bool { return r.Ready() })
	var redeliveryAck atomic.Bool
	redelivery := eventMessage()
	redelivery.Ack = func() error { redeliveryAck.Store(true); return nil }
	factory.emit(1, redelivery)
	waitUntil(t, func() bool { return redeliveryAck.Load() })
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent Consumer calls = %d, want 1", maximum.Load())
	}
	if oldAck.Load() {
		t.Fatal("timed-out delivery was acknowledged")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeUnresponsiveConsumerDoesNotReconnectOrAcknowledge(t *testing.T) {
	cfg := enabledConfig()
	cfg.ConsumerTimeout = 10 * time.Millisecond
	cfg.ReconnectMin = time.Millisecond
	cfg.ReconnectMax = 2 * time.Millisecond
	cfg.Jitter = 0
	factory := &fakeFactory{}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var acked atomic.Bool
	consumer := ConsumerFunc(func(context.Context, IngressMessage) DeliveryOutcome {
		close(started)
		<-release
		close(finished)
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
	message := eventMessage()
	message.Ack = func() error { acked.Store(true); return nil }
	factory.emit(0, message)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("unresponsive consumer call did not start")
	}
	time.Sleep(30 * time.Millisecond)
	if got := factory.count(); got != 1 {
		t.Fatalf("connection count after unresponsive timeout = %d, want 1", got)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = r.Stop(stopCtx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want context deadline", err)
	}
	if acked.Load() {
		t.Fatal("unresponsive QoS1 delivery was acknowledged")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("unresponsive consumer did not finish after release")
	}
	if acked.Load() {
		t.Fatal("late Consumer completion acknowledged a shutdown-expired delivery")
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
