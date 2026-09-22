package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type pointIdentity struct {
	DeviceID string `json:"deviceId"`
	PointKey string `json:"pointKey"`
}

type command struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	DeviceID  string          `json:"deviceId"`
	PointKey  string          `json:"pointKey"`
	Points    []pointIdentity `json:"points"`
}

type protocolError struct {
	Type      string         `json:"type"`
	RequestID string         `json:"requestId,omitempty"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Point     *pointIdentity `json:"point,omitempty"`
}

type subscribedMessage struct {
	Type      string        `json:"type"`
	RequestID string        `json:"requestId,omitempty"`
	Point     pointIdentity `json:"point"`
}

type unsubscribedMessage struct {
	Type      string        `json:"type"`
	RequestID string        `json:"requestId,omitempty"`
	Point     pointIdentity `json:"point"`
}

type snapshotMessage struct {
	Type            string     `json:"type"`
	DataPointID     string     `json:"dataPointId"`
	DeviceID        string     `json:"deviceId"`
	PointKey        string     `json:"pointKey"`
	ValueType       string     `json:"valueType"`
	Value           any        `json:"value"`
	Quality         string     `json:"quality"`
	SourceTimestamp *time.Time `json:"sourceTimestamp"`
	ObservedAt      *time.Time `json:"observedAt"`
	Revision        int64      `json:"revision"`
}

type updateMessage struct {
	Type            string     `json:"type"`
	DataPointID     string     `json:"dataPointId"`
	DeviceID        string     `json:"deviceId"`
	PointKey        string     `json:"pointKey"`
	ValueType       string     `json:"valueType"`
	Value           any        `json:"value"`
	Quality         string     `json:"quality"`
	SourceTimestamp *time.Time `json:"sourceTimestamp"`
	ObservedAt      *time.Time `json:"observedAt"`
	Revision        int64      `json:"revision"`
}

type connection struct {
	service   *Service
	ws        *websocket.Conn
	ctx       context.Context
	principal auth.Principal
	queue     *outboundQueue

	writeMu         sync.Mutex
	subscriptionsMu sync.RWMutex
	subscriptions   map[pointSubscription]struct{}
	closeOnce       sync.Once
	closeStateMu    sync.RWMutex
	closeCode       int
	closeReason     string
	closeHook       func(int, string)
	done            chan struct{}
}

func (s *Service) serveWebSocket(c *gin.Context) {
	ticket := c.Query("ticket")
	record, err := s.peekTicket(ticket)
	if err != nil {
		http.Error(c.Writer, "invalid realtime ticket", http.StatusUnauthorized)
		return
	}
	if err := s.sessions.ValidateSession(c.Request.Context(), record.principal); err != nil {
		http.Error(c.Writer, "invalid realtime session", http.StatusUnauthorized)
		return
	}
	if _, err := s.reserveTicket(ticket, record.principal); err != nil {
		http.Error(c.Writer, "invalid realtime ticket", http.StatusUnauthorized)
		return
	}

	upgradeSucceeded := false
	defer func() {
		if !upgradeSucceeded {
			s.releaseTicket(ticket, record.principal)
		}
	}()
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     s.checkOrigin,
	}
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	if err := s.commitTicket(ticket, record.principal); err != nil {
		_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid realtime ticket"), time.Now().Add(2*time.Second))
		_ = ws.Close()
		return
	}
	upgradeSucceeded = true
	conn := &connection{service: s, ws: ws, ctx: c.Request.Context(), principal: record.principal, subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	if !s.addConnection(conn) {
		conn.closeWithCode(websocket.CloseGoingAway, "server shutting down")
		return
	}
	defer s.removeConnection(conn)
	conn.run()
}

func (s *Service) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || len(s.config.AllowedOrigins) == 0 {
		return true
	}
	for _, allowed := range s.config.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func (c *connection) run() {
	go c.writeLoop()
	go c.monitor()
	defer c.closeWithCode(websocket.CloseNormalClosure, "")
	// Read the protocol payload through a bounded reader so an oversized
	// command can be rejected without destroying subscriptions that already
	// exist on this connection. The remainder of an oversized frame is drained
	// before reading the next command.
	for {
		messageType, payload, err := c.readProtocolMessage()
		if err != nil {
			if errors.Is(err, errProtocolMessageTooLarge) {
				c.writeError("", "message_too_large", "realtime message exceeds 256 KiB")
				continue
			}
			return
		}
		if messageType != websocket.TextMessage {
			c.writeError("", "invalid_message", "text messages are required")
			continue
		}
		c.handle(payload)
	}
}

func (c *connection) readProtocolMessage() (int, []byte, error) {
	messageType, reader, err := c.ws.NextReader()
	if err != nil {
		return 0, nil, err
	}
	payload, err := io.ReadAll(io.LimitReader(reader, maxProtocolMessageSize+1))
	if err != nil {
		return messageType, nil, err
	}
	if len(payload) > maxProtocolMessageSize {
		_, _ = io.Copy(io.Discard, reader)
		return messageType, nil, errProtocolMessageTooLarge
	}
	return messageType, payload, nil
}

func (c *connection) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case <-c.queue.signal:
			for {
				value, ok := c.queue.dequeue()
				if !ok {
					break
				}
				if update, ok := value.(updateMessage); ok && !c.isSubscribed(update.DeviceID, update.PointKey) {
					continue
				}
				if err := c.writeJSON(value); err != nil {
					c.closeWithCode(websocket.CloseAbnormalClosure, "connection lost")
					return
				}
			}
		}
	}
}

func (c *connection) monitor() {
	sessionTicker := time.NewTicker(c.service.config.SessionCheckPeriod)
	pingTicker := time.NewTicker(c.service.config.PingPeriod)
	defer sessionTicker.Stop()
	defer pingTicker.Stop()
	for {
		select {
		case <-sessionTicker.C:
			if err := c.service.sessions.ValidateSession(c.ctx, c.principal); err != nil {
				c.closeWithCode(websocket.ClosePolicyViolation, "session expired or revoked")
				return
			}
		case <-pingTicker.C:
			c.writeMu.Lock()
			err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			c.writeMu.Unlock()
			if err != nil {
				c.closeWithCode(websocket.CloseAbnormalClosure, "connection lost")
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *connection) handle(payload []byte) {
	var request command
	if err := json.Unmarshal(payload, &request); err != nil {
		c.writeError("", "invalid_message", "malformed JSON")
		return
	}
	if !validRequestID(request.RequestID) {
		c.writeError(request.RequestID, "invalid_request_id", "requestId must be a non-empty printable string of at most 128 bytes")
		return
	}
	switch request.Type {
	case "subscribe":
		c.subscribe(request)
	case "unsubscribe":
		c.unsubscribe(request)
	default:
		c.writeError(request.RequestID, "unsupported", "unsupported realtime operation")
	}
}

func (c *connection) subscribe(request command) {
	points, ok := requestedPoints(request)
	if !ok {
		c.writeError(request.RequestID, "invalid_identity", "points must contain deviceId and pointKey only")
		return
	}
	points = uniquePoints(points)
	registered := make([]pointIdentity, 0, len(points))
	for _, point := range points {
		if !validIdentity(point) {
			c.writePointError(request.RequestID, point, "invalid_identity", "deviceId and pointKey must be non-empty and trimmed")
			continue
		}
		if !c.addSubscription(point) {
			if c.isClosed() {
				c.writeError(request.RequestID, "connection_closed", "connection is closed")
				return
			}
			c.writePointError(request.RequestID, point, "subscription_limit", "a connection may subscribe to at most 1000 points")
			continue
		}
		registered = append(registered, point)
	}
	for _, point := range registered {
		current, err := c.service.points.ReadCurrent(c.ctx, point.DeviceID, point.PointKey)
		if err != nil {
			c.removeSubscription(point)
			code, message := pointError(err)
			c.writePointError(request.RequestID, point, code, message)
			continue
		}
		_ = c.enqueueControl(subscribedMessage{Type: "subscribed", RequestID: request.RequestID, Point: point})
		_ = c.enqueueControl(snapshotMessage{
			Type:            "snapshot",
			DataPointID:     current.DataPointID,
			DeviceID:        current.DeviceID,
			PointKey:        current.PointKey,
			ValueType:       string(current.ValueType),
			Value:           current.Value,
			Quality:         string(current.Quality),
			SourceTimestamp: current.SourceTimestamp,
			ObservedAt:      current.ObservedAt,
			Revision:        current.Revision,
		})
	}
}

func (c *connection) unsubscribe(request command) {
	points, ok := requestedPoints(request)
	if !ok {
		c.writeError(request.RequestID, "invalid_identity", "points must contain deviceId and pointKey only")
		return
	}
	for _, point := range uniquePoints(points) {
		if !validIdentity(point) {
			c.writePointError(request.RequestID, point, "invalid_identity", "deviceId and pointKey must be non-empty and trimmed")
			continue
		}
		c.removeSubscription(point)
		if c.queue != nil {
			c.queue.removeUpdates(point)
		}
		_ = c.enqueueControl(unsubscribedMessage{Type: "unsubscribed", RequestID: request.RequestID, Point: point})
	}
}

func requestedPoints(request command) ([]pointIdentity, bool) {
	if len(request.Points) > 0 {
		if request.DeviceID != "" || request.PointKey != "" {
			return nil, false
		}
		return request.Points, true
	}
	// Keep the #29 single-point shape readable for existing clients while all
	// new clients use points[].
	if request.DeviceID != "" || request.PointKey != "" {
		return []pointIdentity{{DeviceID: request.DeviceID, PointKey: request.PointKey}}, true
	}
	return nil, false
}

func uniquePoints(points []pointIdentity) []pointIdentity {
	seen := make(map[pointSubscription]struct{}, len(points))
	unique := make([]pointIdentity, 0, len(points))
	for _, point := range points {
		key := pointSubscription{deviceID: point.DeviceID, pointKey: point.PointKey}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, point)
	}
	return unique
}

func validIdentity(point pointIdentity) bool {
	return strings.TrimSpace(point.DeviceID) != "" && point.DeviceID == strings.TrimSpace(point.DeviceID) &&
		strings.TrimSpace(point.PointKey) != "" && point.PointKey == strings.TrimSpace(point.PointKey)
}

func pointError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrPointNotFound):
		return "point_not_found", "data point does not exist"
	case errors.Is(err, ErrInvalidBinding):
		return "invalid_binding", "data point binding is invalid"
	case errors.Is(err, ErrInvalidPoint):
		return "invalid_point", "data point identity is invalid"
	default:
		return "internal_error", "unable to read current value"
	}
}

func (c *connection) writeError(requestID, code, message string) {
	_ = c.enqueueControl(protocolError{Type: "error", RequestID: requestID, Code: code, Message: message})
}

func (c *connection) writePointError(requestID string, point pointIdentity, code, message string) {
	_ = c.enqueueControl(protocolError{Type: "error", RequestID: requestID, Code: code, Message: message, Point: &point})
}

func (c *connection) enqueueControl(value any) bool {
	if c.queue == nil {
		return false
	}
	accepted := c.queue.enqueue(value)
	if !accepted {
		c.closeSlowClient()
	}
	return accepted
}

func (c *connection) enqueueUpdate(change datapoint.CurrentValueChange) bool {
	if c.queue == nil {
		return false
	}
	accepted := c.queue.enqueueUpdate(change)
	if !accepted {
		c.closeSlowClient()
	}
	return accepted
}

func (c *connection) addSubscription(point pointIdentity) bool {
	c.subscriptionsMu.Lock()
	defer c.subscriptionsMu.Unlock()
	select {
	case <-c.done:
		return false
	default:
	}
	key := pointSubscription{deviceID: point.DeviceID, pointKey: point.PointKey}
	if _, exists := c.subscriptions[key]; !exists && len(c.subscriptions) >= maxActivePoints {
		return false
	}
	c.subscriptions[key] = struct{}{}
	return true
}

func (c *connection) removeSubscription(point pointIdentity) {
	c.subscriptionsMu.Lock()
	delete(c.subscriptions, pointSubscription{deviceID: point.DeviceID, pointKey: point.PointKey})
	c.subscriptionsMu.Unlock()
}

func (c *connection) isSubscribed(deviceID, pointKey string) bool {
	c.subscriptionsMu.RLock()
	_, ok := c.subscriptions[pointSubscription{deviceID: deviceID, pointKey: pointKey}]
	c.subscriptionsMu.RUnlock()
	return ok
}

func (c *connection) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *connection) writeJSON(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return c.ws.WriteJSON(value)
}

func validRequestID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxRequestIDBytes || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func (c *connection) closeWithCode(code int, reason string) {
	if !c.markClosed(code, reason) {
		return
	}
	c.finishClose(code, reason)
}

func (c *connection) markClosed(code int, reason string) bool {
	closed := false
	c.closeOnce.Do(func() {
		c.closeStateMu.Lock()
		c.closeCode = code
		c.closeReason = reason
		c.closeStateMu.Unlock()
		close(c.done)
		if c.queue != nil {
			c.queue.close()
		}
		closed = true
	})
	return closed
}

func (c *connection) finishClose(code int, reason string) {
	if c.closeHook != nil {
		c.closeHook(code, reason)
		return
	}
	if c.ws == nil {
		return
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(2*time.Second))
	_ = c.ws.Close()
}
