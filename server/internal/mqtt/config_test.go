package mqtt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func enabledConfig() Config {
	c := DefaultConfig()
	c.Enabled = true
	c.BrokerURL = "mqtt://broker.example:1883"
	c.ClientID = "cloud-mqtt"
	return c
}

func TestConfigValidationSecurity(t *testing.T) {
	if err := enabledConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, broker := range []string{"http://broker", "mqtt://user:pass@broker", "mqtt+tls://broker"} {
		c := enabledConfig()
		c.BrokerURL = broker
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%q) accepted invalid broker", broker)
		}
	}
	c := enabledConfig()
	c.BrokerURL = "mqtts://broker.example:8883"
	c.TLS.ClientCertFile = "client.crt"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "together") {
		t.Fatalf("mTLS mismatch error = %v", err)
	}
	c = enabledConfig()
	c.TLS.CACertFile = "ca.pem"
	if err := c.Validate(); err == nil {
		t.Fatal("plain mqtt accepted TLS file")
	}
}

func TestTLSConfigUsesSystemRootsAndCustomCA(t *testing.T) {
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	c := enabledConfig()
	c.BrokerURL = "mqtts://broker.example:8883"
	c.TLS.CACertFile = ca
	if _, err := c.TLSConfig(); err == nil {
		t.Fatal("invalid custom CA accepted")
	}
}
