//go:build integration

package integration

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
	mqtt311 "github.com/eclipse/paho.mqtt.golang"
)

func TestMQTTRuntimeProtocolsAndTransports(t *testing.T) {
	tests := []struct {
		name       string
		protocol   mqtt.Protocol
		tls        bool
		clientCert bool
		brokerTLS  bool
	}{
		{name: "mqtt5-plaintext", protocol: mqtt.ProtocolMQTT5},
		{name: "mqtt5-tls", protocol: mqtt.ProtocolMQTT5, tls: true},
		{name: "mqtt5-mtls", protocol: mqtt.ProtocolMQTT5, tls: true, clientCert: true, brokerTLS: true},
		{name: "mqtt311-plaintext", protocol: mqtt.ProtocolMQTT311},
		{name: "mqtt311-tls", protocol: mqtt.ProtocolMQTT311, tls: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			broker := startMQTTBroker(t, mqttBrokerOptions{requireClientCertificate: test.brokerTLS})
			cfg := runtimeConfig(t, broker, test.protocol, test.tls, test.clientCert)
			received := make(chan mqtt.MessageKind, 8)
			consumer := mqtt.ConsumerFunc(func(_ context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
				received <- message.Kind()
				return mqtt.OutcomeAccepted
			})
			runtime, err := mqtt.NewRuntime(cfg, consumer, nil, mqtt.NewMetrics(nil), nil)
			if err != nil {
				t.Fatalf("create MQTT runtime: %v", err)
			}
			startRuntime(t, runtime)

			publishAllIngressMessages(t, broker, test.tls, test.clientCert)
			waitForKinds(t, received, 4)

			connectedAt := runtime.Status().ConnectedAt
			broker.restart(t)
			waitForReconnect(t, runtime, connectedAt)
			publishTLS := (*tls.Config)(nil)
			if test.tls {
				publishTLS = broker.tlsConfig(t, test.clientCert)
			}
			publishMQTTMessage(t, endpointForTest(broker, publishTLS), publishTLS, "edge/edge-1/device/device-1/event", ingressPayload("device-event/v1", "after-restart", "edge-1", "device-1"), 1)
			waitForKinds(t, received, 1)
		})
	}
}

func TestMQTTRuntimePersistentSessions(t *testing.T) {
	for _, protocol := range []mqtt.Protocol{mqtt.ProtocolMQTT5, mqtt.ProtocolMQTT311} {
		t.Run(string(protocol), func(t *testing.T) {
			broker := startMQTTBroker(t, mqttBrokerOptions{})
			cfg := runtimeConfig(t, broker, protocol, false, false)
			received := make(chan mqtt.MessageKind, 1)
			consumer := mqtt.ConsumerFunc(func(_ context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
				received <- message.Kind()
				return mqtt.OutcomeAccepted
			})

			first, err := mqtt.NewRuntime(cfg, consumer, nil, mqtt.NewMetrics(nil), nil)
			if err != nil {
				t.Fatalf("create first runtime: %v", err)
			}
			startRuntime(t, first)
			stopRuntime(t, first)

			publishMQTTMessage(t, broker.plaintext, nil, "edge/edge-1/device/device-1/event", ingressPayload("device-event/v1", "queued", "edge-1", "device-1"), 1)

			second, err := mqtt.NewRuntime(cfg, consumer, nil, mqtt.NewMetrics(nil), nil)
			if err != nil {
				t.Fatalf("create second runtime: %v", err)
			}
			startRuntime(t, second)
			waitForKinds(t, received, 1)
			stopRuntime(t, second)
		})
	}
}

func runtimeConfig(t *testing.T, broker *mqttBrokerFixture, protocol mqtt.Protocol, useTLS, useClientCertificate bool) mqtt.Config {
	t.Helper()
	config := mqtt.Config{
		Enabled:           true,
		Protocol:          protocol,
		ClientID:          fmt.Sprintf("edge-platform-%s-%d", protocol, time.Now().UnixNano()),
		TopicPrefix:       "edge",
		KeepAlive:         5 * time.Second,
		ConnectTimeout:    10 * time.Second,
		ReconnectMin:      100 * time.Millisecond,
		ReconnectMax:      2 * time.Second,
		SessionExpiry:     time.Hour,
		MaxPayloadBytes:   1 << 20,
		ReliableQueueSize: 32,
		RawQueueSize:      8,
		ConsumerTimeout:   2 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	if useTLS {
		config.BrokerURL = broker.tls.URL("mqtts")
		config.TLS.CACertFile = broker.tlsFiles.caPath
		if useClientCertificate {
			config.TLS.ClientCertFile = broker.tlsFiles.clientCertPath
			config.TLS.ClientKeyFile = broker.tlsFiles.clientKeyPath
		}
	} else {
		config.BrokerURL = broker.plaintext.URL("mqtt")
	}
	return config
}

func startRuntime(t *testing.T, runtime *mqtt.Runtime) {
	t.Helper()
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start MQTT runtime: %v", err)
	}
	t.Cleanup(func() { stopRuntime(t, runtime) })
	waitForRuntimeReady(t, runtime, time.Now().Add(45*time.Second))
}

func stopRuntime(t *testing.T, runtime *mqtt.Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Stop(ctx); err != nil {
		t.Errorf("stop MQTT runtime: %v", err)
	}
}

func waitForRuntimeReady(t *testing.T, runtime *mqtt.Runtime, deadline time.Time) {
	t.Helper()
	for time.Now().Before(deadline) {
		if runtime.Ready() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("MQTT runtime did not become READY: %+v", runtime.Status())
}

func waitForReconnect(t *testing.T, runtime *mqtt.Runtime, previous time.Time) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		status := runtime.Status()
		if status.State == mqtt.StateReady && status.ConnectedAt.After(previous) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("MQTT runtime did not reconnect: %+v", runtime.Status())
}

func publishAllIngressMessages(t *testing.T, broker *mqttBrokerFixture, useTLS, clientCertificate bool) {
	t.Helper()
	tlsConfig := (*tls.Config)(nil)
	if useTLS {
		tlsConfig = broker.tlsConfig(t, broker.options.requireClientCertificate)
		if clientCertificate && !broker.options.requireClientCertificate {
			tlsConfig = broker.tlsConfig(t, true)
		}
	}
	publishMQTTMessage(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-1/status", ingressPayload("edge-status/v1", "edge-status", "edge-1", ""), 1)
	publishMQTTMessage(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-1/device/device-1/status", ingressPayload("device-status/v1", "device-status", "edge-1", "device-1"), 1)
	publishMQTTMessage(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-1/device/device-1/raw", ingressPayload("raw-register-snapshot/v1", "raw", "edge-1", "device-1"), 0)
	publishMQTTMessage(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-1/device/device-1/event", ingressPayload("device-event/v1", "event", "edge-1", "device-1"), 1)
}

func endpointForTest(broker *mqttBrokerFixture, tlsConfig *tls.Config) mqttEndpoint {
	if tlsConfig != nil {
		return broker.tls
	}
	return broker.plaintext
}

func publishMQTTMessage(t *testing.T, endpoint mqttEndpoint, tlsConfig *tls.Config, topic string, payload []byte, qos byte) {
	t.Helper()
	scheme := "tcp"
	if tlsConfig != nil {
		scheme = "ssl"
	}
	brokerURL := (&url.URL{Scheme: scheme, Host: endpoint.Address()}).String()
	options := mqtt311.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(fmt.Sprintf("publisher-%d", time.Now().UnixNano())).
		SetCleanSession(true).
		SetConnectTimeout(10 * time.Second).
		SetKeepAlive(5)
	if tlsConfig != nil {
		options.SetTLSConfig(tlsConfig)
	}
	client := mqtt311.NewClient(options)
	connect := client.Connect()
	if !connect.WaitTimeout(10*time.Second) || connect.Error() != nil {
		t.Fatalf("connect publisher to %s: %v", brokerURL, connect.Error())
	}
	t.Cleanup(func() { client.Disconnect(100) })
	publish := client.Publish(topic, qos, false, payload)
	if !publish.WaitTimeout(10*time.Second) || publish.Error() != nil {
		t.Fatalf("publish %s: %v", topic, publish.Error())
	}
}

func ingressPayload(schema, messageID, edgeID, deviceID string) []byte {
	envelope := map[string]any{
		"schema":          schema,
		"messageId":       messageID,
		"edgeId":          edgeID,
		"sourceTimestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"data":            map[string]any{"value": 1},
	}
	if deviceID != "" {
		envelope["deviceId"] = deviceID
	}
	payload, _ := json.Marshal(envelope)
	return payload
}

func waitForKinds(t *testing.T, received <-chan mqtt.MessageKind, count int) {
	t.Helper()
	seen := make(map[mqtt.MessageKind]struct{})
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	for len(seen) < count {
		select {
		case kind := <-received:
			seen[kind] = struct{}{}
		case <-deadline.C:
			t.Fatalf("received %d/%d MQTT ingress kinds: %v", len(seen), count, seen)
		}
	}
}
