package command

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

var ErrNoPendingDelivery = errors.New("no pending command delivery")

// DeliveryStore is the narrow persistence port for M6-04. It intentionally
// does not expose command creation or result projection to the worker.
type DeliveryStore interface {
	NextPendingDelivery(context.Context, time.Time) (Delivery, error)
	RecordDeliveryAttempt(context.Context, string, time.Time, error) error
	ExpirePendingDeliveries(context.Context, time.Time) (int, error)
}

// CommandPublisher only accepts a frozen Delivery. It is deliberately not a
// generic topic publisher; the MQTT adapter validates the command wire shape
// and forces QoS1/retain=false.
type CommandPublisher interface {
	PublishCommand(context.Context, Delivery) error
}

type Dispatcher struct {
	store     DeliveryStore
	publisher CommandPublisher
	now       func() time.Time
	interval  time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewDispatcher(service *Service, publisher CommandPublisher) (*Dispatcher, error) {
	if service == nil || publisher == nil {
		return nil, errors.New("command dispatcher dependencies are required")
	}
	store, ok := service.store.(DeliveryStore)
	if !ok {
		return nil, errors.New("command delivery store is not configured")
	}
	return &Dispatcher{store: store, publisher: publisher, now: time.Now, interval: 250 * time.Millisecond}, nil
}

// DispatchOnce performs one scheduled send attempt. It fetches without holding a
// database lock across PublishCommand; the result projector may delete the
// row while this packet is in flight, which is the permitted in-flight race.
func (d *Dispatcher) DispatchOnce(ctx context.Context) (bool, error) {
	now := d.clockNow()
	delivery, err := d.store.NextPendingDelivery(ctx, now)
	if errors.Is(err, ErrNoPendingDelivery) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	publishErr := d.publisher.PublishCommand(ctx, delivery)
	recordErr := d.store.RecordDeliveryAttempt(ctx, delivery.CommandID, now, publishErr)
	if recordErr != nil {
		return true, recordErr
	}
	return true, publishErr
}

func (d *Dispatcher) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	if d.cancel != nil {
		d.mu.Unlock()
		return errors.New("command dispatcher already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.done = make(chan struct{})
	d.mu.Unlock()
	go d.run(runCtx)
	return nil
}

func (d *Dispatcher) run(ctx context.Context) {
	d.mu.Lock()
	done := d.done
	interval := d.interval
	d.mu.Unlock()
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := d.store.ExpirePendingDeliveries(ctx, d.clockNow()); err != nil && !errors.Is(err, context.Canceled) {
			slog.Default().Warn("command delivery expiry failed", "reason", "expiry_persistence")
		}
		if _, err := d.DispatchOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Default().Warn("command delivery attempt failed", "reason", "transport_or_persistence")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	cancel, done := d.cancel, d.done
	d.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		d.mu.Lock()
		if d.done == done {
			d.cancel = nil
			d.done = nil
		}
		d.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dispatcher) clockNow() time.Time {
	if d.now == nil {
		return time.Now().UTC()
	}
	return d.now().UTC()
}
