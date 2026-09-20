package mqtt

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/packets"
	mqtt5 "github.com/eclipse/paho.golang/paho"
	mqtt311 "github.com/eclipse/paho.mqtt.golang"
)

// PahoFactory is the production adapter. It deliberately implements the
// module's small ClientFactory seam instead of exposing either Paho API to
// app/config callers.
type PahoFactory struct {
	cfg     Config
	metrics *Metrics
	logger  *slog.Logger
}

func NewPahoFactory(cfg Config, metrics *Metrics, logger *slog.Logger) *PahoFactory {
	return &PahoFactory{cfg: cfg.WithDefaults(), metrics: metrics, logger: logger}
}

func (f *PahoFactory) Connect(ctx context.Context, generation uint64, handler PublishHandler) (Client, error) {
	if f.cfg.Protocol == ProtocolMQTT311 {
		return f.connect311(ctx, handler)
	}
	return f.connect5(ctx, handler)
}

func (f *PahoFactory) connect311(ctx context.Context, handler PublishHandler) (Client, error) {
	broker, err := url.Parse(f.cfg.BrokerURL)
	if err != nil {
		return nil, fmt.Errorf("parse broker URL: %w", err)
	}
	server := *broker
	if server.Scheme == "mqtt" {
		server.Scheme = "tcp"
	} else {
		server.Scheme = "ssl"
	}
	tlsCfg, err := f.cfg.TLSConfig()
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }
	opts := mqtt311.NewClientOptions().
		AddBroker(server.String()).
		SetClientID(f.cfg.ClientID).
		SetProtocolVersion(4).
		SetCleanSession(false).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetResumeSubs(true).
		SetOrderMatters(true).
		SetKeepAlive(f.cfg.KeepAlive).
		SetConnectTimeout(f.cfg.ConnectTimeout).
		SetAutoAckDisabled(true).
		SetConnectionLostHandler(func(_ mqtt311.Client, err error) {
			if f.logger != nil && err != nil {
				f.logger.Warn("mqtt 3.1.1 connection lost", "error", sanitizeError(err))
			}
			closeDone()
		}).
		SetDefaultPublishHandler(func(_ mqtt311.Client, msg mqtt311.Message) {
			if handler == nil {
				return
			}
			payload := append([]byte(nil), msg.Payload()...)
			handler(Received{Topic: msg.Topic(), Payload: payload, QoS: msg.Qos(), Retained: msg.Retained(), Ack: func() error { msg.Ack(); return nil }})
		})
	if f.cfg.Username != "" {
		opts.SetUsername(f.cfg.Username)
	}
	if f.cfg.Password != "" {
		opts.SetPassword(f.cfg.Password)
	}
	if tlsCfg != nil {
		opts.SetTLSConfig(tlsCfg)
	}
	client := mqtt311.NewClient(opts)
	token := client.Connect()
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			client.Disconnect(0)
			return nil, err
		}
	case <-ctx.Done():
		client.Disconnect(0)
		return nil, ctx.Err()
	}
	return &paho311Client{client: client, done: done, closeDone: closeDone}, nil
}

type paho311Client struct {
	client    mqtt311.Client
	done      <-chan struct{}
	closeDone func()
}

func (c *paho311Client) Subscribe(ctx context.Context, subscriptions []Subscription) error {
	filters := make(map[string]byte, len(subscriptions))
	for _, subscription := range subscriptions {
		filters[subscription.Filter] = subscription.QoS
	}
	token := c.client.SubscribeMultiple(filters, nil)
	select {
	case <-token.Done():
		return token.Error()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *paho311Client) Close(ctx context.Context) error {
	if c.client.IsConnected() {
		c.client.Disconnect(1000)
	}
	c.closeDone()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *paho311Client) Done() <-chan struct{} { return c.done }

func (f *PahoFactory) connect5(ctx context.Context, handler PublishHandler) (Client, error) {
	u, err := url.Parse(f.cfg.BrokerURL)
	if err != nil {
		return nil, fmt.Errorf("parse broker URL: %w", err)
	}
	conn, err := dialBroker(ctx, u, f.cfg)
	if err != nil {
		return nil, err
	}
	properties := &mqtt5.ConnectProperties{}
	expiry := uint32(f.cfg.SessionExpiry / time.Second)
	properties.SessionExpiryInterval = &expiry
	client := mqtt5.NewClient(mqtt5.ClientConfig{
		ClientID:                   f.cfg.ClientID,
		Conn:                       packets.NewThreadSafeConn(conn),
		PacketTimeout:              f.cfg.ConnectTimeout,
		EnableManualAcknowledgment: true,
		OnPublishReceived: []func(mqtt5.PublishReceived) (bool, error){func(received mqtt5.PublishReceived) (bool, error) {
			if handler == nil || received.Packet == nil {
				return true, nil
			}
			packet := received.Packet
			payload := append([]byte(nil), packet.Payload...)
			handler(Received{Topic: packet.Topic, Payload: payload, QoS: packet.QoS, Retained: packet.Retain, Ack: func() error { return received.Client.Ack(packet) }})
			return true, nil
		}},
	})
	connect := &mqtt5.Connect{ClientID: f.cfg.ClientID, KeepAlive: uint16(f.cfg.KeepAlive / time.Second), CleanStart: false, Properties: properties}
	if f.cfg.Username != "" {
		connect.UsernameFlag = true
		connect.Username = f.cfg.Username
	}
	if f.cfg.Password != "" {
		connect.PasswordFlag = true
		connect.Password = []byte(f.cfg.Password)
	}
	if _, err := client.Connect(ctx, connect); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &paho5Client{client: client, conn: conn}, nil
}

type paho5Client struct {
	client *mqtt5.Client
	conn   net.Conn
}

func (c *paho5Client) Subscribe(ctx context.Context, subscriptions []Subscription) error {
	request := &mqtt5.Subscribe{Subscriptions: make([]mqtt5.SubscribeOptions, len(subscriptions))}
	for i, subscription := range subscriptions {
		request.Subscriptions[i] = mqtt5.SubscribeOptions{Topic: subscription.Filter, QoS: subscription.QoS}
	}
	_, err := c.client.Subscribe(ctx, request)
	return err
}

func (c *paho5Client) Close(ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	}
	return c.client.Disconnect(&mqtt5.Disconnect{ReasonCode: 0})
}
func (c *paho5Client) Done() <-chan struct{} { return c.client.Done() }

func dialBroker(ctx context.Context, u *url.URL, cfg Config) (net.Conn, error) {
	if u.Hostname() == "" {
		return nil, errors.New("broker URL has no hostname")
	}
	address := u.Host
	if !strings.Contains(address, ":") {
		if u.Scheme == "mqtts" {
			address += ":8883"
		} else {
			address += ":1883"
		}
	}
	dialer := net.Dialer{Timeout: cfg.ConnectTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial broker: %w", err)
	}
	if u.Scheme != "mqtts" {
		return conn, nil
	}
	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	secure := tls.Client(conn, tlsCfg)
	if err := secure.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	return secure, nil
}
