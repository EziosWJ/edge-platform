package app

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// deviceStatusReplayRequester is the narrow composition seam for M3. It is
// intentionally not part of the HTTP runtime/readiness interface.
type deviceStatusReplayRequester interface {
	ReplayDeviceStatus(context.Context) error
}

const deviceStatusReplayTimeout = 15 * time.Second

// deviceStatusReplayCoordinator coalesces Edge registrations into at most one
// in-flight replay and one pending/dirty signal. It never stores a message or
// payload and never retries a failed replay itself.
type deviceStatusReplayCoordinator struct {
	requester deviceStatusReplayRequester
	logger    *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	signal chan struct{}
	done   chan struct{}

	mu       sync.Mutex
	pending  bool
	inFlight bool
}

func newDeviceStatusReplayCoordinator(requester deviceStatusReplayRequester, logger *slog.Logger) *deviceStatusReplayCoordinator {
	if requester == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	coordinator := &deviceStatusReplayCoordinator{
		requester: requester,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		signal:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	go coordinator.run()
	return coordinator
}

func (c *deviceStatusReplayCoordinator) Notify() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.pending = true
	c.mu.Unlock()
	select {
	case c.signal <- struct{}{}:
	default:
	}
}

func (c *deviceStatusReplayCoordinator) run() {
	defer close(c.done)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.signal:
			for {
				c.mu.Lock()
				if !c.pending {
					c.mu.Unlock()
					break
				}
				c.pending = false
				c.inFlight = true
				c.mu.Unlock()

				requestContext, cancel := context.WithTimeout(c.ctx, deviceStatusReplayTimeout)
				err := c.requester.ReplayDeviceStatus(requestContext)
				cancel()
				if err != nil && c.ctx.Err() == nil {
					c.logger.Warn("device status replay request failed", "error", err)
				}

				c.mu.Lock()
				c.inFlight = false
				followUp := c.pending
				c.mu.Unlock()
				if !followUp {
					break
				}
			}
		}
	}
}

func (c *deviceStatusReplayCoordinator) Stop(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.cancel()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
