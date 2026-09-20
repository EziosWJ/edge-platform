package mqtt

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Subscription struct {
	Filter string
	QoS    byte
}

type Received struct {
	Topic    string
	Payload  []byte
	QoS      byte
	Retained bool
	Ack      func() error
}

type PublishHandler func(Received)

type Client interface {
	Subscribe(context.Context, []Subscription) error
	Close(context.Context) error
	Done() <-chan struct{}
}

type ClientFactory interface {
	Connect(context.Context, uint64, PublishHandler) (Client, error)
}

type Runtime struct {
	cfg      Config
	consumer Consumer
	factory  ClientFactory
	metrics  *Metrics
	logger   *slog.Logger
	parser   *Parser
	reliable *reliableQueue
	raw      *rawQueue

	mu          sync.RWMutex
	status      Status
	client      Client
	generation  uint64
	reconnect   chan struct{}
	cancel      context.CancelFunc
	workersDone chan struct{}
	done        chan struct{}
	stopWorkers chan struct{}
	workerWG    sync.WaitGroup
	stopOnce    sync.Once
	started     atomic.Bool
	accepting   atomic.Bool
}

func NewRuntime(cfg Config, consumer Consumer, factory ClientFactory, metrics *Metrics, logger *slog.Logger) (*Runtime, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.ValidateDurationBounds(); err != nil {
		return nil, err
	}
	if consumer == nil {
		return nil, errors.New("mqtt consumer is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = NewMetrics(nil)
	}
	parser := NewParser(cfg.MaxPayloadBytes, metrics, logger)
	if err := SetParserPrefix(parser, cfg.TopicPrefix); err != nil {
		return nil, err
	}
	_, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		cfg: cfg, consumer: consumer, factory: factory, metrics: metrics, logger: logger,
		parser: parser, reliable: newReliableQueue(cfg.ReliableQueueSize), raw: newRawQueue(cfg.RawQueueSize),
		reconnect: make(chan struct{}, 1), cancel: cancel,
		workersDone: make(chan struct{}), done: make(chan struct{}), stopWorkers: make(chan struct{}),
		status: Status{Enabled: cfg.Enabled, State: StateDisabled, Protocol: cfg.Protocol, Subscriptions: subscriptionStatus(cfg.TopicPrefix)},
	}
	return r, nil
}

func subscriptionStatus(prefix string) map[string]bool {
	result := make(map[string]bool, 4)
	for _, filter := range Filters(prefix) {
		result[filter] = false
	}
	return result
}

func (r *Runtime) Start(ctx context.Context) error {
	if !r.cfg.Enabled {
		r.setState(StateDisabled, "")
		return nil
	}
	if r.factory == nil {
		r.factory = NewPahoFactory(r.cfg, r.metrics, r.logger)
	}
	if !r.started.CompareAndSwap(false, true) {
		return errors.New("mqtt runtime already started")
	}
	r.accepting.Store(true)
	runCtx, runCancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.cancel = runCancel
	r.mu.Unlock()
	r.workerWG.Add(2)
	go r.consumeLoop(false)
	go r.consumeLoop(true)
	go func() { r.workerWG.Wait(); close(r.workersDone) }()
	go r.run(runCtx)
	return nil
}

func (r *Runtime) run(parent context.Context) {
	defer close(r.done)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var retry int
	var first = true
	for {
		select {
		case <-ctx.Done():
			r.shutdownClient()
			r.setState(StateStopped, "")
			r.stopConsumers()
			return
		default:
		}
		if first {
			r.setState(StateConnecting, "")
			first = false
		} else {
			r.setState(StateReconnecting, "")
		}
		r.mu.Lock()
		r.generation++
		gen := r.generation
		r.mu.Unlock()
		connectCtx, cancelConnect := context.WithTimeout(ctx, r.cfg.ConnectTimeout)
		client, err := r.factory.Connect(connectCtx, gen, func(message Received) { r.handleReceived(gen, message) })
		cancelConnect()
		if err != nil {
			r.recordError(connectionErrorCode(err), err, retry)
			if !r.waitBackoff(ctx, retry) {
				r.setState(StateStopped, "")
				r.stopConsumers()
				return
			}
			retry++
			continue
		}
		r.mu.Lock()
		r.client = client
		r.mu.Unlock()
		r.setState(StateSubscribing, "")
		subCtx, cancelSub := context.WithTimeout(ctx, r.cfg.ConnectTimeout)
		err = client.Subscribe(subCtx, subscriptions(r.cfg.TopicPrefix))
		cancelSub()
		if err != nil {
			r.recordError("SUBSCRIBE_FAILED", err, retry)
			_ = client.Close(context.Background())
			if !r.waitBackoff(ctx, retry) {
				r.setState(StateStopped, "")
				r.stopConsumers()
				return
			}
			retry++
			continue
		}
		retry = 0
		r.setReady()
		select {
		case <-ctx.Done():
			r.shutdownClient()
			r.setState(StateStopped, "")
			r.stopConsumers()
			return
		case <-client.Done():
			r.markDisconnected()
			r.recordError("CONNECTION_LOST", errors.New("broker connection lost"), 0)
			if !r.waitBackoff(ctx, retry) {
				r.setState(StateStopped, "")
				r.stopConsumers()
				return
			}
			retry++
			continue
		case <-r.reconnect:
			r.shutdownClient()
			if !r.waitBackoff(ctx, retry) {
				r.setState(StateStopped, "")
				r.stopConsumers()
				return
			}
			retry++
			continue
		}
	}
}

func subscriptions(prefix string) []Subscription {
	filters := Filters(prefix)
	result := make([]Subscription, len(filters))
	for i, filter := range filters {
		result[i] = Subscription{Filter: filter, QoS: 1}
	}
	return result
}

func (r *Runtime) waitBackoff(ctx context.Context, retry int) bool {
	delay := r.cfg.ReconnectMin
	for i := 0; i < retry && delay < r.cfg.ReconnectMax; i++ {
		delay *= 2
		if delay > r.cfg.ReconnectMax {
			delay = r.cfg.ReconnectMax
		}
	}
	if r.cfg.Jitter > 0 {
		delay = time.Duration(float64(delay) * (1 - r.cfg.Jitter + 2*r.cfg.Jitter*rand.Float64()))
	}
	r.mu.Lock()
	r.status.RetryCount = retry
	r.status.NextRetryAt = time.Now().Add(delay)
	r.mu.Unlock()
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (r *Runtime) handleReceived(generation uint64, message Received) {
	if !r.accepting.Load() {
		return
	}
	r.mu.Lock()
	r.status.LastMessageAt = time.Now()
	r.mu.Unlock()
	parsed, violations, err := r.parser.Parse(message.Topic, message.Payload, message.QoS, message.Retained)
	if err != nil {
		if message.QoS == 1 {
			r.ack(generation, message.Ack)
		}
		return
	}
	d := &Delivery{Message: parsed, Generation: generation, QoS: message.QoS, Retained: message.Retained, Violations: violations, ack: message.Ack}
	item := &inbound{Delivery: d, Ack: message.Ack, Generation: generation}
	if parsed.Kind() == KindRawRegisterSnapshot && message.QoS == 0 {
		if dropped := r.raw.push(item); dropped != nil && r.metrics != nil {
			r.metrics.QueueDrops.WithLabelValues("raw").Inc()
		}
		return
	}
	if !r.reliable.push(item) {
		if r.metrics != nil {
			r.metrics.QueueDrops.WithLabelValues("reliable").Inc()
		}
		r.requestReconnect()
	}
}

func (r *Runtime) consumeLoop(raw bool) {
	defer r.workerWG.Done()
	for {
		var item *inbound
		if raw {
			item = r.raw.pop()
		} else {
			item = r.reliable.pop()
		}
		if item != nil {
			r.consume(item)
			continue
		}
		select {
		case <-r.stopWorkers:
			if (raw && r.raw.len() == 0) || (!raw && r.reliable.len() == 0) {
				return
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (r *Runtime) consume(item *inbound) {
	kind := string(item.Delivery.Message.Kind())
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.ConsumerTimeout)
	done := make(chan DeliveryOutcome, 1)
	go func() { done <- r.consumer.Consume(ctx, item.Delivery.Message) }()
	var outcome DeliveryOutcome
	select {
	case outcome = <-done:
	case <-ctx.Done():
		cancel()
		outcome = OutcomeRetry
		if r.metrics != nil {
			r.metrics.Timeouts.WithLabelValues(kind).Inc()
		}
	}
	cancel()
	switch outcome {
	case OutcomeAccepted:
		if r.metrics != nil {
			r.metrics.Accepted.WithLabelValues(kind).Inc()
		}
		r.ack(item.Generation, item.Ack)
	case OutcomeRejected:
		if r.metrics != nil {
			r.metrics.Rejected.WithLabelValues(kind).Inc()
		}
		r.ack(item.Generation, item.Ack)
	case OutcomeRetry, "":
		if r.metrics != nil {
			r.metrics.Retries.WithLabelValues(kind).Inc()
		}
		r.requestReconnect()
	default:
		if r.metrics != nil {
			r.metrics.Rejected.WithLabelValues(kind).Inc()
		}
		r.ack(item.Generation, item.Ack)
	}
}

func (r *Runtime) ack(generation uint64, ack func() error) {
	if ack == nil {
		return
	}
	r.mu.RLock()
	current := r.generation == generation && r.status.State != StateStopped
	r.mu.RUnlock()
	if current {
		if err := ack(); err != nil {
			r.logger.Warn("mqtt ack failed", "error", sanitizeError(err))
		}
	}
}

func (r *Runtime) requestReconnect() {
	select {
	case r.reconnect <- struct{}{}:
	default:
	}
}

func (r *Runtime) stopConsumers() {
	r.stopOnce.Do(func() { close(r.stopWorkers) })
}

func (r *Runtime) shutdownClient() {
	r.mu.Lock()
	client := r.client
	r.client = nil
	r.mu.Unlock()
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), r.cfg.ShutdownTimeout)
		defer cancel()
		_ = client.Close(ctx)
	}
}

func (r *Runtime) Stop(ctx context.Context) error {
	if !r.cfg.Enabled {
		r.setState(StateDisabled, "")
		return nil
	}
	if !r.started.Load() {
		r.setState(StateStopped, "")
		return nil
	}
	r.accepting.Store(false)
	r.setState(StateStopping, "")
	r.stopConsumers()
	select {
	case <-r.workersDone:
	case <-ctx.Done():
		r.cancel()
		return ctx.Err()
	}
	r.cancel()
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.cfg.Enabled || r.status.State == StateReady
}

func (r *Runtime) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.status
	s.Subscriptions = make(map[string]bool, len(r.status.Subscriptions))
	for key, value := range r.status.Subscriptions {
		s.Subscriptions[key] = value
	}
	return s
}

func (r *Runtime) setState(state RuntimeState, errorCode string) {
	r.mu.Lock()
	r.status.State = state
	if errorCode != "" {
		r.status.RecentErrorCode = errorCode
	}
	r.mu.Unlock()
}
func (r *Runtime) setReady() {
	r.mu.Lock()
	r.status.State = StateReady
	r.status.ConnectedAt = time.Now()
	for key := range r.status.Subscriptions {
		r.status.Subscriptions[key] = true
	}
	r.mu.Unlock()
}

func (r *Runtime) markDisconnected() {
	r.mu.Lock()
	r.status.DisconnectedAt = time.Now()
	for key := range r.status.Subscriptions {
		r.status.Subscriptions[key] = false
	}
	r.mu.Unlock()
}
func (r *Runtime) recordError(code string, err error, retry int) {
	r.setState(StateReconnecting, code)
	r.mu.Lock()
	r.status.RetryCount = retry
	r.status.RecentErrorAt = time.Now()
	r.mu.Unlock()
	r.logger.Warn("mqtt runtime error", "code", code, "error", sanitizeError(err))
}

func connectionErrorCode(err error) string {
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not authorized") || strings.Contains(message, "bad user") || strings.Contains(message, "authentication") {
			return "AUTH_REJECTED"
		}
	}
	return "CONNECT_FAILED"
}
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 256 {
		text = text[:256]
	}
	return text
}
