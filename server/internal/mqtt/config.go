package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrDisabled      = errors.New("mqtt runtime is disabled")
	ErrInvalidConfig = errors.New("invalid mqtt configuration")
)

func (c Config) WithDefaults() Config {
	d := DefaultConfig()
	if c.Protocol == "" {
		c.Protocol = d.Protocol
	}
	if c.TopicPrefix == "" {
		c.TopicPrefix = d.TopicPrefix
	}
	if c.KeepAlive == 0 {
		c.KeepAlive = d.KeepAlive
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = d.ConnectTimeout
	}
	if c.ReconnectMin == 0 {
		c.ReconnectMin = d.ReconnectMin
	}
	if c.ReconnectMax == 0 {
		c.ReconnectMax = d.ReconnectMax
	}
	if c.SessionExpiry == 0 {
		c.SessionExpiry = d.SessionExpiry
	}
	if c.MaxPayloadBytes == 0 {
		c.MaxPayloadBytes = d.MaxPayloadBytes
	}
	if c.ReliableQueueSize == 0 {
		c.ReliableQueueSize = d.ReliableQueueSize
	}
	if c.RawQueueSize == 0 {
		c.RawQueueSize = d.RawQueueSize
	}
	if c.ConsumerTimeout == 0 {
		c.ConsumerTimeout = d.ConsumerTimeout
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = d.ShutdownTimeout
	}
	if c.Jitter == 0 {
		c.Jitter = d.Jitter
	}
	return c
}

func (c Config) Validate() error {
	c = c.WithDefaults()
	if !c.Enabled {
		return nil
	}
	if c.BrokerURL == "" {
		return fmt.Errorf("%w: broker URL is required", ErrInvalidConfig)
	}
	u, err := url.Parse(c.BrokerURL)
	if err != nil || u.Host == "" || (u.Scheme != "mqtt" && u.Scheme != "mqtts") {
		return fmt.Errorf("%w: broker URL must use mqtt:// or mqtts://", ErrInvalidConfig)
	}
	if u.User != nil {
		return fmt.Errorf("%w: broker URL must not contain userinfo", ErrInvalidConfig)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: broker URL must contain only scheme and broker host", ErrInvalidConfig)
	}
	if c.Protocol != ProtocolMQTT5 && c.Protocol != ProtocolMQTT311 {
		return fmt.Errorf("%w: protocol must be mqtt5 or mqtt311", ErrInvalidConfig)
	}
	if c.ClientID == "" || len(c.ClientID) > 256 || !utf8.ValidString(c.ClientID) {
		return fmt.Errorf("%w: client id must be valid UTF-8 and 1..256 bytes", ErrInvalidConfig)
	}
	if err := validatePrefix(c.TopicPrefix); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if c.KeepAlive <= 0 || c.ConnectTimeout <= 0 || c.ReconnectMin <= 0 || c.ReconnectMax < c.ReconnectMin || c.SessionExpiry <= 0 {
		return fmt.Errorf("%w: invalid connection timing", ErrInvalidConfig)
	}
	if c.MaxPayloadBytes <= 0 || c.ReliableQueueSize <= 0 || c.RawQueueSize <= 0 || c.ConsumerTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return fmt.Errorf("%w: limits and timeouts must be positive", ErrInvalidConfig)
	}
	if c.Jitter < 0 || c.Jitter > 1 {
		return fmt.Errorf("%w: jitter must be between 0 and 1", ErrInvalidConfig)
	}
	if u.Scheme == "mqtt" && (c.TLS.CACertFile != "" || c.TLS.ClientCertFile != "" || c.TLS.ClientKeyFile != "") {
		return fmt.Errorf("%w: TLS files require mqtts://", ErrInvalidConfig)
	}
	if (c.TLS.ClientCertFile == "") != (c.TLS.ClientKeyFile == "") {
		return fmt.Errorf("%w: client certificate and key must be configured together", ErrInvalidConfig)
	}
	return nil
}

func validatePrefix(prefix string) error {
	if prefix == "" || strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") || strings.ContainsAny(prefix, "+#") {
		return errors.New("topic prefix must be non-empty and contain no leading/trailing slash or wildcard")
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "" {
			return errors.New("topic prefix contains an empty segment")
		}
	}
	return nil
}

func (c Config) TLSConfig() (*tls.Config, error) {
	u, err := url.Parse(c.BrokerURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "mqtts" {
		return nil, nil
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), InsecureSkipVerify: false} //nolint:gosec // explicitly prohibited from being configurable
	if c.TLS.CACertFile != "" {
		pem, err := os.ReadFile(c.TLS.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("read custom CA: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("custom CA contains no certificates")
		}
		tlsCfg.RootCAs = pool
	}
	if c.TLS.ClientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.TLS.ClientCertFile, c.TLS.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	return tlsCfg, nil
}

func (c Config) ValidateDurationBounds() error {
	if c.ReconnectMax > 24*time.Hour {
		return fmt.Errorf("%w: reconnect max is excessive", ErrInvalidConfig)
	}
	return nil
}
