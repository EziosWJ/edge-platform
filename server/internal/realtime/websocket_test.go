package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type fakeSessionValidator struct {
	mu     sync.Mutex
	active bool
	calls  int
}

func (f *fakeSessionValidator) ValidateSession(context.Context, auth.Principal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if !f.active {
		return ErrSessionInvalid
	}
	return nil
}

func (f *fakeSessionValidator) setActive(active bool) {
	f.mu.Lock()
	f.active = active
	f.mu.Unlock()
}

type fakePointReader struct {
	point Point
	err   error
}

func (f fakePointReader) ReadCurrent(context.Context, string, string) (Point, error) {
	return f.point, f.err
}

type mapPointReader struct {
	points map[pointSubscription]Point
}

func (r mapPointReader) ReadCurrent(_ context.Context, deviceID, pointKey string) (Point, error) {
	point, ok := r.points[pointSubscription{deviceID: deviceID, pointKey: pointKey}]
	if !ok {
		return Point{}, ErrPointNotFound
	}
	return point, nil
}

func newQueuedConnection(t *testing.T, reader PointReader, queueSize int) (*Service, *connection) {
	t.Helper()
	service, err := NewService(&fakeSessionValidator{active: true}, reader, Config{}, NewHub(queueSize))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	connection := &connection{service: service, subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	if !service.hub.register(connection) {
		t.Fatal("hub.register() rejected connection")
	}
	t.Cleanup(func() {
		service.hub.unregister(connection)
		_ = service.Close(context.Background())
	})
	return service, connection
}

func drainQueuedMessages(c *connection) []any {
	messages := make([]any, 0)
	for {
		message, ok := c.queue.dequeue()
		if !ok {
			return messages
		}
		messages = append(messages, message)
	}
}

func pointForTest(deviceID, pointKey string, revision int64) Point {
	return Point{DataPointID: deviceID + "-" + pointKey, DeviceID: deviceID, PointKey: pointKey, ValueType: datapoint.ValueTypeNumber, Value: float64(revision), Quality: datapoint.QualityGood, Revision: revision}
}

func TestBatchSubscribeIsPartialAndRegistersValidPoints(t *testing.T) {
	_, connection := newQueuedConnection(t, mapPointReader{points: map[pointSubscription]Point{
		{deviceID: "device-1", pointKey: "temperature"}: pointForTest("device-1", "temperature", 1),
		{deviceID: "device-1", pointKey: "enabled"}:     pointForTest("device-1", "enabled", 2),
	}}, 32)
	connection.handle([]byte(`{"type":"subscribe","requestId":"batch-1","points":[{"deviceId":"device-1","pointKey":"temperature"},{"deviceId":"device-1","pointKey":"missing"},{"deviceId":"device-1","pointKey":"enabled"}]}`))
	messages := drainQueuedMessages(connection)
	if len(messages) != 5 {
		t.Fatalf("batch messages = %d, want subscribed/snapshot/error/subscribed/snapshot: %#v", len(messages), messages)
	}
	if _, ok := messages[0].(subscribedMessage); !ok {
		t.Fatalf("first message = %#v, want subscribed", messages[0])
	}
	if _, ok := messages[1].(snapshotMessage); !ok {
		t.Fatalf("second message = %#v, want snapshot", messages[1])
	}
	partial, ok := messages[2].(protocolError)
	if !ok || partial.Code != "point_not_found" || partial.Point == nil || partial.Point.PointKey != "missing" {
		t.Fatalf("partial error = %#v", messages[2])
	}
	if _, ok := messages[3].(subscribedMessage); !ok {
		t.Fatalf("fourth message = %#v, want subscribed", messages[3])
	}
	if _, ok := messages[4].(snapshotMessage); !ok {
		t.Fatalf("fifth message = %#v, want snapshot", messages[4])
	}
	if !connection.isSubscribed("device-1", "temperature") || !connection.isSubscribed("device-1", "enabled") || connection.isSubscribed("device-1", "missing") {
		t.Fatal("batch subscription state is incorrect")
	}
}

func TestBatchSubscribeAndUnsubscribeAreIdempotent(t *testing.T) {
	_, connection := newQueuedConnection(t, mapPointReader{points: map[pointSubscription]Point{
		{deviceID: "device-1", pointKey: "temperature"}: pointForTest("device-1", "temperature", 1),
	}}, 32)
	request := []byte(`{"type":"subscribe","requestId":"same","points":[{"deviceId":"device-1","pointKey":"temperature"},{"deviceId":"device-1","pointKey":"temperature"}]}`)
	connection.handle(request)
	first := drainQueuedMessages(connection)
	connection.handle(request)
	second := drainQueuedMessages(connection)
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("duplicate subscribe messages = %d/%d, want 2/2", len(first), len(second))
	}
	connection.handle([]byte(`{"type":"unsubscribe","requestId":"remove","points":[{"deviceId":"device-1","pointKey":"temperature"},{"deviceId":"device-1","pointKey":"temperature"}]}`))
	connection.handle([]byte(`{"type":"unsubscribe","requestId":"remove-again","points":[{"deviceId":"device-1","pointKey":"temperature"}]}`))
	messages := drainQueuedMessages(connection)
	if len(messages) != 2 {
		t.Fatalf("duplicate unsubscribe messages = %d, want 2", len(messages))
	}
	if _, ok := messages[0].(unsubscribedMessage); !ok {
		t.Fatalf("first unsubscribe message = %#v", messages[0])
	}
	if connection.isSubscribed("device-1", "temperature") {
		t.Fatal("point remains subscribed after unsubscribe")
	}
}

func TestConnectionSubscriptionLimitDoesNotDisturbExistingPoints(t *testing.T) {
	_, connection := newQueuedConnection(t, mapPointReader{points: map[pointSubscription]Point{
		{deviceID: "device-1", pointKey: "last"}: pointForTest("device-1", "last", 1),
	}}, 8)
	for index := 0; index < maxActivePoints; index++ {
		if !connection.addSubscription(pointIdentity{DeviceID: "device", PointKey: "point-" + fmt.Sprint(index)}) {
			t.Fatalf("addSubscription(%d) failed", index)
		}
	}
	connection.handle([]byte(`{"type":"subscribe","requestId":"limit","points":[{"deviceId":"device-1","pointKey":"last"}]}`))
	messages := drainQueuedMessages(connection)
	if len(messages) != 1 {
		t.Fatalf("limit messages = %#v", messages)
	}
	limit, ok := messages[0].(protocolError)
	if !ok || limit.Code != "subscription_limit" {
		t.Fatalf("limit response = %#v", messages[0])
	}
	if !connection.isSubscribed("device", "point-0") || len(connection.subscriptions) != maxActivePoints {
		t.Fatal("existing subscriptions were disturbed by limit rejection")
	}
}

func TestConnectionsOnlyReceiveTheirSubscribedPoints(t *testing.T) {
	hub := NewHub(8)
	service, err := NewService(&fakeSessionValidator{active: true}, fakePointReader{}, Config{}, hub)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	first := &connection{service: service, subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	second := &connection{service: service, subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	if !hub.register(first) || !hub.register(second) {
		t.Fatal("hub.register() rejected connection")
	}
	defer hub.unregister(first)
	defer hub.unregister(second)
	first.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "temperature"})
	second.addSubscription(pointIdentity{DeviceID: "device-2", PointKey: "temperature"})
	hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "point-1", DeviceID: "device-1", PointKey: "temperature", ValueType: datapoint.ValueTypeNumber, Value: 1, Quality: datapoint.QualityGood, Revision: 1})
	firstMessages := drainQueuedMessages(first)
	secondMessages := drainQueuedMessages(second)
	if len(firstMessages) != 1 || len(secondMessages) != 0 {
		t.Fatalf("connection isolation messages = %d/%d, want 1/0", len(firstMessages), len(secondMessages))
	}
}

func TestRequestIDValidationIsPredictable(t *testing.T) {
	for _, value := range []string{"", " leading", "trailing ", string([]byte{0x01})} {
		if validRequestID(value) {
			t.Errorf("validRequestID(%q) = true, want false", value)
		}
	}
	if !validRequestID("request-1") {
		t.Fatal("valid request ID rejected")
	}
}

type gatedPointReader struct {
	point   Point
	started chan struct{}
	release chan struct{}
}

func (r gatedPointReader) ReadCurrent(context.Context, string, string) (Point, error) {
	close(r.started)
	<-r.release
	return r.point, nil
}

func newRealtimeTestServer(t *testing.T, session *fakeSessionValidator, points PointReader, config Config) (*Service, *httptest.Server) {
	t.Helper()
	service, err := NewService(session, points, config)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, handler, testAuthenticator{principal: testPrincipal()})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test server: %v", err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: router}}
	server.Start()
	return service, server
}

type testAuthenticator struct {
	principal auth.Principal
	err       error
}

func (a testAuthenticator) Authenticate(context.Context, string) (auth.Principal, error) {
	if a.err != nil {
		return auth.Principal{}, a.err
	}
	return a.principal, nil
}

func testPrincipal() auth.Principal {
	return auth.Principal{UserID: 11, JTI: "jti-11", ExpiresAt: time.Now().Add(time.Hour)}
}

func issueTestTicket(t *testing.T, service *Service) string {
	t.Helper()
	result, err := service.IssueTicket(context.Background(), testPrincipal())
	if err != nil {
		t.Fatalf("IssueTicket() error = %v", err)
	}
	return result.Ticket
}

func TestTicketEndpointRequiresBearerSessionAndReturnsOpaqueTicket(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	service, err := NewService(session, fakePointReader{}, Config{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, handler, testAuthenticator{principal: testPrincipal()})

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/realtime/ticket", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ticket status = %d, want 401", unauthenticated.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/realtime/ticket", nil)
	request.Header.Set("Authorization", "Bearer session-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ticket status = %d body=%s, want 200", response.Code, response.Body.String())
	}
	var envelope struct {
		Data TicketResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode ticket response: %v", err)
	}
	if envelope.Data.Ticket == "" || strings.Contains(envelope.Data.Ticket, "Bearer") || envelope.Data.ExpiresIn != 30 {
		t.Fatalf("ticket response = %+v", envelope.Data)
	}
}

func dialTestWebSocket(t *testing.T, serverURL, ticket string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/realtime/ws?ticket=" + ticket
	conn, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("Dial() error = %v, HTTP status = %d", err, response.StatusCode)
		}
		t.Fatalf("Dial() error = %v", err)
	}
	return conn
}

func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode WebSocket JSON: %v", err)
	}
	return result
}

func TestSubscribeReturnsSemanticCurrentValueSnapshotOnly(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	source := time.Date(2026, 9, 22, 11, 59, 0, 0, time.UTC)
	observed := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	service, server := newRealtimeTestServer(t, session, fakePointReader{point: Point{
		DataPointID:     "point-1",
		DeviceID:        "device-1",
		PointKey:        "temperature",
		ValueType:       datapoint.ValueTypeNumber,
		Value:           12.3,
		Quality:         datapoint.QualityGood,
		SourceTimestamp: &source,
		ObservedAt:      &observed,
		Revision:        4,
	}}, Config{SessionCheckPeriod: time.Hour, PingPeriod: time.Hour})
	defer server.Close()
	defer service.Close(context.Background())

	conn := dialTestWebSocket(t, server.URL, issueTestTicket(t, service))
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{"type": "subscribe", "requestId": "r1", "deviceId": "device-1", "pointKey": "temperature"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	subscribed := readJSON(t, conn)
	if subscribed["type"] != "subscribed" || subscribed["requestId"] != "r1" {
		t.Fatalf("subscribed message = %#v", subscribed)
	}
	snapshot := readJSON(t, conn)
	wantKeys := map[string]bool{"type": true, "dataPointId": true, "deviceId": true, "pointKey": true, "valueType": true, "value": true, "quality": true, "sourceTimestamp": true, "observedAt": true, "revision": true}
	if len(snapshot) != len(wantKeys) {
		t.Fatalf("snapshot keys = %#v", snapshot)
	}
	for key := range snapshot {
		if !wantKeys[key] {
			t.Errorf("snapshot leaked or added field %q", key)
		}
	}
	if snapshot["dataPointId"] != "point-1" || snapshot["deviceId"] != "device-1" || snapshot["pointKey"] != "temperature" || snapshot["valueType"] != "NUMBER" || snapshot["quality"] != "GOOD" {
		t.Fatalf("snapshot identity/semantic fields = %#v", snapshot)
	}
	if snapshot["value"] != 12.3 || snapshot["revision"] != float64(4) {
		t.Fatalf("snapshot value/revision = %#v", snapshot)
	}
	if _, leaked := snapshot["sourceMapping"]; leaked {
		t.Fatal("snapshot must not contain source mapping")
	}
}

func TestSubscribeRegistersBeforeSnapshotAndReceivesLiveUpdate(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	started := make(chan struct{})
	release := make(chan struct{})
	point := Point{DataPointID: "point-1", DeviceID: "device-1", PointKey: "temperature", ValueType: datapoint.ValueTypeNumber, Revision: 1}
	hub := NewHub(8)
	service, err := NewService(session, gatedPointReader{point: point, started: started, release: release}, Config{}, hub)
	if err != nil {
		t.Fatal(err)
	}
	connection := &connection{service: service, subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	if !hub.register(connection) {
		t.Fatal("hub.register() rejected connection")
	}
	defer hub.unregister(connection)
	go connection.subscribe(command{Type: "subscribe", RequestID: "r1", DeviceID: "device-1", PointKey: "temperature"})
	<-started
	if ok := hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "point-1", DeviceID: "device-1", PointKey: "temperature", ValueType: datapoint.ValueTypeNumber, Value: 42.0, Quality: datapoint.QualityGood, Revision: 2}); !ok {
		t.Fatal("TryPublish() rejected live update")
	}
	close(release)
	deadline := time.After(time.Second)
	for connection.queueLength() < 3 {
		select {
		case <-deadline:
			t.Fatalf("snapshot subscription messages = %d, want 3", connection.queueLength())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	first, _ := connection.queue.dequeue()
	if update, ok := first.(updateMessage); !ok || update.Revision != 2 {
		t.Fatalf("first queued message = %#v, want live revision 2 before snapshot", first)
	}
	second, _ := connection.queue.dequeue()
	if subscribed, ok := second.(subscribedMessage); !ok || subscribed.RequestID != "r1" {
		t.Fatalf("second queued message = %#v, want subscribed", second)
	}
	third, _ := connection.queue.dequeue()
	if snapshot, ok := third.(snapshotMessage); !ok || snapshot.Revision != 1 {
		t.Fatalf("third queued message = %#v, want snapshot revision 1", third)
	}
}

func (c *connection) queueLength() int {
	c.queue.mu.Lock()
	defer c.queue.mu.Unlock()
	return len(c.queue.items)
}

func TestHubCoalescesHighestRevisionAndDoesNotWaitForClient(t *testing.T) {
	hub := NewHub(1)
	connection := &connection{subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	if !hub.register(connection) {
		t.Fatal("hub.register() rejected connection")
	}
	defer hub.unregister(connection)
	if !connection.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "temperature"}) {
		t.Fatal("addSubscription() failed")
	}
	change := datapoint.CurrentValueChange{DataPointID: "point-1", DeviceID: "device-1", PointKey: "temperature", ValueType: datapoint.ValueTypeNumber, Value: 1.0, Quality: datapoint.QualityGood, Revision: 1}
	start := time.Now()
	if !hub.TryPublish(change) {
		t.Fatal("first TryPublish() rejected")
	}
	change.Value = 2.0
	change.Revision = 2
	if !hub.TryPublish(change) {
		t.Fatal("coalescing TryPublish() rejected")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("TryPublish() blocked for %s", elapsed)
	}
	value, ok := connection.queue.dequeue()
	if !ok {
		t.Fatal("queue is empty")
	}
	update, ok := value.(updateMessage)
	if !ok || update.Revision != 2 || update.Value != 2.0 {
		t.Fatalf("coalesced update = %#v, want revision 2 value 2", value)
	}
}

func TestHubClosesOnlySlowConnectionWhenQueueSaturates(t *testing.T) {
	hub := NewHub(1)
	slow := &connection{subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	healthy := &connection{subscriptions: make(map[pointSubscription]struct{}), done: make(chan struct{})}
	if !hub.register(slow) || !hub.register(healthy) {
		t.Fatal("hub.register() rejected connection")
	}
	defer hub.unregister(slow)
	defer hub.unregister(healthy)
	slow.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "slow"})
	slow.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "other"})
	healthy.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "healthy"})

	if !hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "slow-point", DeviceID: "device-1", PointKey: "slow", ValueType: datapoint.ValueTypeNumber, Value: 1.0, Quality: datapoint.QualityGood, Revision: 1}) {
		t.Fatal("initial slow-client update was rejected")
	}
	start := time.Now()
	if hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "other-point", DeviceID: "device-1", PointKey: "other", ValueType: datapoint.ValueTypeNumber, Value: 2.0, Quality: datapoint.QualityGood, Revision: 1}) {
		t.Fatal("saturated slow-client queue was accepted")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("slow-client isolation blocked TryPublish() for %s", elapsed)
	}
	if !slow.isClosed() {
		t.Fatal("slow client was not closed after queue saturation")
	}
	slow.closeStateMu.RLock()
	closeCode, closeReason := slow.closeCode, slow.closeReason
	slow.closeStateMu.RUnlock()
	if closeCode != websocket.CloseTryAgainLater || closeReason != slowClientCloseReason {
		t.Fatalf("slow-client close = %d/%q, want %d/%q", closeCode, closeReason, websocket.CloseTryAgainLater, slowClientCloseReason)
	}

	if !hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "healthy-point", DeviceID: "device-1", PointKey: "healthy", ValueType: datapoint.ValueTypeNumber, Value: 3.0, Quality: datapoint.QualityGood, Revision: 1}) {
		t.Fatal("healthy connection was affected by slow client")
	}
	messages := drainQueuedMessages(healthy)
	if len(messages) != 1 {
		t.Fatalf("healthy connection messages = %d, want 1", len(messages))
	}
	update, ok := messages[0].(updateMessage)
	if !ok || update.PointKey != "healthy" || update.Revision != 1 {
		t.Fatalf("healthy connection update = %#v", messages[0])
	}
}

func TestHubPublishDoesNotWaitForSlowCloseTransport(t *testing.T) {
	hub := NewHub(1)
	closeStarted := make(chan struct{})
	releaseClose := make(chan struct{})
	slow := &connection{
		subscriptions: make(map[pointSubscription]struct{}),
		done:          make(chan struct{}),
		closeHook: func(int, string) {
			close(closeStarted)
			<-releaseClose
		},
	}
	if !hub.register(slow) {
		t.Fatal("hub.register() rejected connection")
	}
	defer hub.unregister(slow)
	slow.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "first"})
	slow.addSubscription(pointIdentity{DeviceID: "device-1", PointKey: "second"})
	if !hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "first-point", DeviceID: "device-1", PointKey: "first", ValueType: datapoint.ValueTypeNumber, Value: 1.0, Quality: datapoint.QualityGood, Revision: 1}) {
		t.Fatal("initial update was rejected")
	}

	start := time.Now()
	if hub.TryPublish(datapoint.CurrentValueChange{DataPointID: "second-point", DeviceID: "device-1", PointKey: "second", ValueType: datapoint.ValueTypeNumber, Value: 2.0, Quality: datapoint.QualityGood, Revision: 1}) {
		t.Fatal("saturated queue was accepted")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("TryPublish() waited for slow close transport for %s", elapsed)
	}
	select {
	case <-closeStarted:
	case <-time.After(time.Second):
		t.Fatal("slow close transport was not started asynchronously")
	}
	close(releaseClose)
}

func TestControlQueueSaturationClosesConnection(t *testing.T) {
	_, connection := newQueuedConnection(t, fakePointReader{}, 1)
	if !connection.enqueueControl(protocolError{Type: "error", Code: "first"}) {
		t.Fatal("initial control message was rejected")
	}
	if connection.enqueueControl(protocolError{Type: "error", Code: "second"}) {
		t.Fatal("saturated control queue was accepted")
	}
	if !connection.isClosed() {
		t.Fatal("connection remained open after control queue saturation")
	}
}

func TestSessionRevocationClosesConnectionWithPolicyViolation(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	service, server := newRealtimeTestServer(t, session, fakePointReader{}, Config{SessionCheckPeriod: 10 * time.Millisecond, PingPeriod: time.Hour})
	defer server.Close()

	conn := dialTestWebSocket(t, server.URL, issueTestTicket(t, service))
	defer conn.Close()
	session.setActive(false)
	_, _, err := conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != websocket.ClosePolicyViolation {
		t.Fatalf("revoked connection error = %v, want close code 1008", err)
	}
}

func TestServiceCloseUsesGoingAwayCode(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	service, server := newRealtimeTestServer(t, session, fakePointReader{}, Config{SessionCheckPeriod: time.Hour, PingPeriod: time.Hour})
	defer server.Close()

	conn := dialTestWebSocket(t, server.URL, issueTestTicket(t, service))
	defer conn.Close()
	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, _, err := conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != websocket.CloseGoingAway {
		t.Fatalf("shutdown connection error = %v, want close code 1001", err)
	}
}

func TestTicketCannotUpgradeAfterSessionInvalidation(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	service, server := newRealtimeTestServer(t, session, fakePointReader{}, Config{SessionCheckPeriod: time.Hour, PingPeriod: time.Hour})
	defer server.Close()
	ticket := issueTestTicket(t, service)
	session.setActive(false)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/realtime/ws?ticket=" + ticket
	_, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid session upgrade = err %v response %#v, want HTTP 401", err, response)
	}
}

func TestFailedUpgradeDoesNotConsumeTicket(t *testing.T) {
	session := &fakeSessionValidator{active: true}
	service, server := newRealtimeTestServer(t, session, fakePointReader{}, Config{
		AllowedOrigins:     []string{"https://allowed.example"},
		SessionCheckPeriod: time.Hour,
		PingPeriod:         time.Hour,
	})
	defer server.Close()
	defer service.Close(context.Background())
	ticket := issueTestTicket(t, service)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/realtime/ws?ticket=" + ticket

	dialer := websocket.DefaultDialer
	headers := http.Header{"Origin": []string{"https://rejected.example"}}
	_, response, err := dialer.Dial(url, headers)
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("rejected-origin upgrade = err %v response %#v, want HTTP 403", err, response)
	}

	service.config.AllowedOrigins = nil
	conn, response, err := dialer.Dial(url, nil)
	if err != nil || response == nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("retry upgrade = err %v response %#v, want success", err, response)
	}
	conn.Close()
}
