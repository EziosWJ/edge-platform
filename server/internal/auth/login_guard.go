package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	loginAttemptOutcomeSuccess          = "success"
	loginAttemptOutcomeCredentialFailed = "credential_failure"
	loginAttemptOutcomeGuardRejected    = "guard_rejected"
)

var (
	ErrLoginRateLimited = errors.New("login rate limited")

	loginAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "base_go_api_login_attempts_total",
			Help: "Total number of structurally valid login attempts by outcome.",
		},
		[]string{"outcome"},
	)
	loginGuardRejections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "base_go_api_login_guard_rejections_total",
			Help: "Total number of login attempts rejected by the in-process login guard.",
		},
		[]string{"dimension"},
	)
	loginGuardEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "base_go_api_login_guard_enabled",
			Help: "Whether each login guard dimension is enabled (1) or disabled (0).",
		},
		[]string{"dimension"},
	)
)

func init() {
	prometheus.MustRegister(loginAttempts, loginGuardRejections)
	prometheus.MustRegister(loginGuardEnabled)
}

// LoginGuardConfig controls the in-process login attempt guard. A max-attempts
// value of zero disables that dimension.
type LoginGuardConfig struct {
	IPWindow            time.Duration
	IPMaxAttempts       int
	UsernameWindow      time.Duration
	UsernameMaxAttempts int
	BackoffInitial      time.Duration
	BackoffMax          time.Duration
	LockDuration        time.Duration
	MaxEntries          int
}

// LoginRateLimitError is returned before credential lookup when an attempt is
// rejected by the login guard. RetryAfter is intentionally the only detail
// exposed to the HTTP layer; the hit dimension remains private.
type LoginRateLimitError struct {
	RetryAfter time.Duration
}

func (e *LoginRateLimitError) Error() string {
	return ErrLoginRateLimited.Error()
}

func (e *LoginRateLimitError) Unwrap() error {
	return ErrLoginRateLimited
}

type loginGuardState struct {
	windowStarted       time.Time
	attempts            int
	consecutiveFailures int
	blockedUntil        time.Time
	lastSeen            time.Time
}

// LoginGuard is a bounded, process-local admission guard for password login.
// It deliberately has no database or network dependency.
type LoginGuard struct {
	config LoginGuardConfig
	now    func() time.Time

	mu             sync.Mutex
	ipStates       map[string]*loginGuardState
	usernameStates map[string]*loginGuardState
}

func NewLoginGuard(config LoginGuardConfig) (*LoginGuard, error) {
	if config.IPMaxAttempts < 0 {
		return nil, errors.New("login guard IP max attempts must not be negative")
	}
	if config.UsernameMaxAttempts < 0 {
		return nil, errors.New("login guard username max attempts must not be negative")
	}
	if config.IPMaxAttempts > 0 && config.IPWindow <= 0 {
		return nil, errors.New("login guard IP window must be greater than zero when enabled")
	}
	if config.UsernameMaxAttempts > 0 && config.UsernameWindow <= 0 {
		return nil, errors.New("login guard username window must be greater than zero when enabled")
	}
	if config.BackoffInitial <= 0 {
		return nil, errors.New("login guard initial backoff must be greater than zero")
	}
	if config.BackoffMax < config.BackoffInitial {
		return nil, errors.New("login guard maximum backoff must not be less than initial backoff")
	}
	if config.LockDuration <= 0 {
		return nil, errors.New("login guard lock duration must be greater than zero")
	}
	if config.MaxEntries <= 0 {
		return nil, errors.New("login guard maximum entries must be greater than zero")
	}
	loginGuardEnabled.WithLabelValues("ip").Set(enabledMetricValue(config.IPMaxAttempts > 0))
	loginGuardEnabled.WithLabelValues("username").Set(enabledMetricValue(config.UsernameMaxAttempts > 0))

	return &LoginGuard{
		config:         config,
		now:            time.Now,
		ipStates:       make(map[string]*loginGuardState),
		usernameStates: make(map[string]*loginGuardState),
	}, nil
}

// Admit atomically checks both dimensions and consumes one attempt from every
// enabled dimension. It must be called before user lookup and password hash
// comparison.
func (g *LoginGuard) Admit(clientIP, username string) error {
	if g == nil || !g.enabled() {
		return nil
	}

	now := g.nowUTC()
	username = strings.TrimSpace(username)
	ipKey := loginGuardKey("ip", clientIP)
	usernameKey := loginGuardKey("username", username)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupLocked(now)

	if g.config.IPMaxAttempts > 0 {
		if retryAfter, blocked := g.canAdmitLocked(g.ipStates[ipKey], now, g.config.IPWindow, g.config.IPMaxAttempts); blocked {
			return g.reject("ip", retryAfter)
		}
	}
	if g.config.UsernameMaxAttempts > 0 {
		if retryAfter, blocked := g.canAdmitLocked(g.usernameStates[usernameKey], now, g.config.UsernameWindow, g.config.UsernameMaxAttempts); blocked {
			return g.reject("username", retryAfter)
		}
	}

	if g.config.IPMaxAttempts > 0 {
		state := g.ensureStateLocked(g.ipStates, ipKey, now)
		state.attempts++
		state.lastSeen = now
	}
	if g.config.UsernameMaxAttempts > 0 {
		state := g.ensureStateLocked(g.usernameStates, usernameKey, now)
		state.attempts++
		state.lastSeen = now
	}
	return nil
}

// RecordFailure records a credential outcome after the password path has run.
func (g *LoginGuard) RecordFailure(clientIP, username string) {
	if g == nil || !g.enabled() {
		return
	}

	now := g.nowUTC()
	username = strings.TrimSpace(username)
	ipKey := loginGuardKey("ip", clientIP)
	usernameKey := loginGuardKey("username", username)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupLocked(now)
	g.recordFailureLocked(g.ipStates[ipKey], now, g.config.IPWindow, g.config.IPMaxAttempts)
	g.recordFailureLocked(g.usernameStates[usernameKey], now, g.config.UsernameWindow, g.config.UsernameMaxAttempts)
}

// RecordSuccess restores the username dimension after a successful login. IP
// state is intentionally retained so one success cannot reset source pressure.
func (g *LoginGuard) RecordSuccess(username string) {
	if g == nil || g.config.UsernameMaxAttempts == 0 {
		return
	}

	usernameKey := loginGuardKey("username", strings.TrimSpace(username))
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.usernameStates, usernameKey)
}

func (g *LoginGuard) enabled() bool {
	return g.config.IPMaxAttempts > 0 || g.config.UsernameMaxAttempts > 0
}

func (g *LoginGuard) nowUTC() time.Time {
	return g.now().UTC()
}

func (g *LoginGuard) canAdmitLocked(state *loginGuardState, now time.Time, window time.Duration, maxAttempts int) (time.Duration, bool) {
	if state == nil {
		return 0, false
	}
	g.rotateWindow(state, now, window)
	if state.blockedUntil.After(now) {
		return state.blockedUntil.Sub(now), true
	}
	if state.attempts >= maxAttempts {
		return state.windowStarted.Add(window).Sub(now), true
	}
	return 0, false
}

func (g *LoginGuard) recordFailureLocked(state *loginGuardState, now time.Time, window time.Duration, maxAttempts int) {
	if state == nil || maxAttempts == 0 {
		return
	}
	g.rotateWindow(state, now, window)
	state.consecutiveFailures++
	state.lastSeen = now

	blockedUntil := now.Add(exponentialBackoff(g.config.BackoffInitial, g.config.BackoffMax, state.consecutiveFailures))
	if state.attempts >= maxAttempts {
		lockUntil := now.Add(g.config.LockDuration)
		windowUntil := state.windowStarted.Add(window)
		if windowUntil.After(lockUntil) {
			lockUntil = windowUntil
		}
		if lockUntil.After(blockedUntil) {
			blockedUntil = lockUntil
		}
	}
	if blockedUntil.After(state.blockedUntil) {
		state.blockedUntil = blockedUntil
	}
}

func (g *LoginGuard) rotateWindow(state *loginGuardState, now time.Time, window time.Duration) {
	if state.windowStarted.IsZero() {
		state.windowStarted = now
		return
	}
	if !state.blockedUntil.After(now) && !now.Before(state.windowStarted.Add(window)) {
		*state = loginGuardState{windowStarted: now, lastSeen: now}
	}
}

func (g *LoginGuard) ensureStateLocked(states map[string]*loginGuardState, key string, now time.Time) *loginGuardState {
	if state := states[key]; state != nil {
		return state
	}
	if len(states) >= g.config.MaxEntries {
		g.evictOldestLocked(states)
	}
	state := &loginGuardState{windowStarted: now, lastSeen: now}
	states[key] = state
	return state
}

func (g *LoginGuard) cleanupLocked(now time.Time) {
	cleanupStates(g.ipStates, now, maxDuration(g.config.IPWindow, g.config.LockDuration, g.config.BackoffMax))
	cleanupStates(g.usernameStates, now, maxDuration(g.config.UsernameWindow, g.config.LockDuration, g.config.BackoffMax))
}

func cleanupStates(states map[string]*loginGuardState, now time.Time, retention time.Duration) {
	for key, state := range states {
		if state.blockedUntil.After(now) {
			continue
		}
		if !now.Before(state.lastSeen.Add(retention)) {
			delete(states, key)
		}
	}
}

func (g *LoginGuard) evictOldestLocked(states map[string]*loginGuardState) {
	var oldestKey string
	var oldest time.Time
	for key, state := range states {
		if oldestKey == "" || state.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = state.lastSeen
		}
	}
	if oldestKey != "" {
		delete(states, oldestKey)
	}
}

func (g *LoginGuard) reject(dimension string, retryAfter time.Duration) error {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	loginGuardRejections.WithLabelValues(dimension).Inc()
	return &LoginRateLimitError{RetryAfter: retryAfter}
}

func exponentialBackoff(initial, maximum time.Duration, failures int) time.Duration {
	delay := initial
	for index := 1; index < failures; index++ {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func loginGuardKey(kind, value string) string {
	digest := sha256.Sum256([]byte(kind + "\x00" + value))
	return hex.EncodeToString(digest[:])
}

func maxDuration(values ...time.Duration) time.Duration {
	var maximum time.Duration
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func enabledMetricValue(enabled bool) float64 {
	if enabled {
		return 1
	}
	return 0
}
