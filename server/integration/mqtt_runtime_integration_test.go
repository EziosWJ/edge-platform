//go:build integration

package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
	mqtt311 "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
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
			publishMQTTMessage(t, endpointForTest(broker, publishTLS), publishTLS, "edge/edge-01/device/device-01/event", readMQTTV1Fixture(t, "device-event.json"), 1)
			waitForKinds(t, received, 1)
		})
	}
}

func TestMQTTRuntimePublishesFrozenCommandQoS1(t *testing.T) {
	for _, protocol := range []mqtt.Protocol{mqtt.ProtocolMQTT5, mqtt.ProtocolMQTT311} {
		t.Run(string(protocol), func(t *testing.T) {
			broker := startMQTTBroker(t, mqttBrokerOptions{})
			cfg := runtimeConfig(t, broker, protocol, false, false)
			runtime, err := mqtt.NewRuntime(cfg, mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
				return mqtt.OutcomeAccepted
			}), nil, mqtt.NewMetrics(nil), nil)
			if err != nil {
				t.Fatalf("create MQTT runtime: %v", err)
			}
			startRuntime(t, runtime)

			payload := []byte(`{"schema":"device-command/v1","commandId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","deviceId":"device-01","name":"close","args":{"value":90071992547409931234567890},"issuedAt":"2026-09-22T05:00:00Z","expiresAt":"2026-09-22T05:00:30Z"}`)
			received := make(chan mqtt311.Message, 1)
			subscriberOptions := mqtt311.NewClientOptions().
				AddBroker(broker.plaintext.URL("mqtt")).
				SetClientID(fmt.Sprintf("command-subscriber-%s-%d", protocol, time.Now().UnixNano())).
				SetCleanSession(true).
				SetConnectTimeout(10 * time.Second).
				SetDefaultPublishHandler(func(_ mqtt311.Client, message mqtt311.Message) { received <- message })
			subscriber := mqtt311.NewClient(subscriberOptions)
			connect := subscriber.Connect()
			if !connect.WaitTimeout(10*time.Second) || connect.Error() != nil {
				t.Fatalf("connect command subscriber: %v", connect.Error())
			}
			t.Cleanup(func() { subscriber.Disconnect(100) })
			subscribe := subscriber.Subscribe("edge/edge-01/device/device-01/command", 1, nil)
			if !subscribe.WaitTimeout(10*time.Second) || subscribe.Error() != nil {
				t.Fatalf("subscribe command topic: %v", subscribe.Error())
			}

			if err := runtime.PublishCommand(context.Background(), mqtt.CommandPublication{
				CommandID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				Topic:     "edge/edge-01/device/device-01/command",
				Payload:   payload,
			}); err != nil {
				t.Fatalf("PublishCommand() error: %v", err)
			}
			select {
			case message := <-received:
				if message.Qos() != 1 || message.Retained() || message.Topic() != "edge/edge-01/device/device-01/command" || string(message.Payload()) != string(payload) {
					t.Fatalf("received command = qos %d retained %v topic %s payload %s", message.Qos(), message.Retained(), message.Topic(), message.Payload())
				}
			case <-time.After(10 * time.Second):
				t.Fatal("command subscriber did not receive QoS1 publication")
			}
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

			publishMQTTMessage(t, broker.plaintext, nil, "edge/edge-01/device/device-01/event", readMQTTV1Fixture(t, "device-event.json"), 1)

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

func TestMQTTRuntimeContractMetadata(t *testing.T) {
	broker := startMQTTBroker(t, mqttBrokerOptions{})
	publishMQTTMessageWithRetain(t, broker.plaintext, nil, "edge/edge-01/status", readMQTTV1Fixture(t, "edge-status.json"), 1, true)

	registry := prometheus.NewRegistry()
	metrics := mqtt.NewMetrics(registry)
	received := make(chan mqtt.IngressMessage, 2)
	consumer := mqtt.ConsumerFunc(func(_ context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
		received <- message
		return mqtt.OutcomeAccepted
	})
	runtime, err := mqtt.NewRuntime(runtimeConfig(t, broker, mqtt.ProtocolMQTT5, false, false), consumer, nil, metrics, nil)
	if err != nil {
		t.Fatalf("create MQTT runtime: %v", err)
	}
	startRuntime(t, runtime)

	select {
	case message := <-received:
		if message.Kind() != mqtt.KindEdgeStatus || !message.Metadata().Retained {
			t.Fatalf("retained status delivery = kind %q, retained %v", message.Kind(), message.Metadata().Retained)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("retained status replay was not delivered")
	}

	publishMQTTMessage(t, broker.plaintext, nil, "edge/edge-01/device/device-01/raw", readMQTTV1Fixture(t, "raw-register-snapshot.json"), 0)
	select {
	case message := <-received:
		if message.Kind() != mqtt.KindRawRegisterSnapshot {
			t.Fatalf("raw delivery kind = %q", message.Kind())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("raw QoS0 delivery was not received")
	}

	if got := contractViolationCount(t, registry, string(mqtt.ViolationQoS)); got != 0 {
		t.Fatalf("QoS contract violations = %v, want 0", got)
	}
	if got := contractViolationCount(t, registry, string(mqtt.ViolationRetained)); got != 0 {
		t.Fatalf("retain contract violations = %v, want 0", got)
	}
}

func TestMQTTRuntimeTLSFailurePaths(t *testing.T) {
	t.Run("wrong-ca-keeps-runtime-unready", func(t *testing.T) {
		broker := startMQTTBroker(t, mqttBrokerOptions{})
		cfg := runtimeConfig(t, broker, mqtt.ProtocolMQTT5, true, false)
		// Do not use the broker's server leaf as the wrong CA. Go's TLS
		// verifier accepts an explicitly configured certificate as a trust
		// anchor, even when it is not marked as a CA, which would make this
		// failure-path test pass for the wrong reason. Generate a separate CA
		// with an unrelated key and certificate instead.
		wrongCA := createCertificate(t, "mqtt-integration-wrong-ca", nil, true, false, false)
		wrongCAPath := filepath.Join(t.TempDir(), "wrong-ca.crt")
		writeCertificateFile(t, wrongCAPath, wrongCA.certificatePEM)
		cfg.TLS.CACertFile = wrongCAPath
		runtime, err := mqtt.NewRuntime(cfg, mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
			return mqtt.OutcomeAccepted
		}), nil, mqtt.NewMetrics(nil), nil)
		if err != nil {
			t.Fatalf("create runtime with wrong CA: %v", err)
		}
		startRuntimeWithoutWaitingForReady(t, runtime)
		waitForRuntimeErrorCode(t, runtime, "CONNECT_FAILED")
		if runtime.Ready() {
			t.Fatal("runtime became ready with the wrong CA")
		}
	})

	t.Run("hostname-mismatch-keeps-runtime-unready", func(t *testing.T) {
		broker := startMQTTBroker(t, mqttBrokerOptions{serverCertificateDNSOnly: true})
		cfg := runtimeConfig(t, broker, mqtt.ProtocolMQTT5, true, false)
		runtime, err := mqtt.NewRuntime(cfg, mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
			return mqtt.OutcomeAccepted
		}), nil, mqtt.NewMetrics(nil), nil)
		if err != nil {
			t.Fatalf("create runtime with hostname mismatch: %v", err)
		}
		startRuntimeWithoutWaitingForReady(t, runtime)
		waitForRuntimeErrorCode(t, runtime, "CONNECT_FAILED")
		if runtime.Ready() {
			t.Fatal("runtime became ready with a hostname mismatch")
		}
	})

	t.Run("client-certificate-key-mismatch-fails-validation", func(t *testing.T) {
		broker := startMQTTBroker(t, mqttBrokerOptions{requireClientCertificate: true})
		cfg := runtimeConfig(t, broker, mqtt.ProtocolMQTT5, true, true)
		cfg.TLS.ClientKeyFile = broker.tlsFiles.serverKeyPath
		if _, err := mqtt.NewRuntime(cfg, mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
			return mqtt.OutcomeAccepted
		}), nil, mqtt.NewMetrics(nil), nil); err == nil {
			t.Fatal("client certificate/private key mismatch was accepted")
		}
	})
}

func contractViolationCount(t *testing.T, registry *prometheus.Registry, kind string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather MQTT metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "mqtt_ingress_contract_violations_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "kind" && label.GetValue() == kind {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
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

func startRuntimeWithoutWaitingForReady(t *testing.T, runtime *mqtt.Runtime) {
	t.Helper()
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start MQTT runtime: %v", err)
	}
	t.Cleanup(func() { stopRuntime(t, runtime) })
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

func waitForRuntimeErrorCode(t *testing.T, runtime *mqtt.Runtime, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if status := runtime.Status(); status.RecentErrorCode == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("runtime did not report error code %q: %+v", want, runtime.Status())
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
	publishMQTTMessageWithRetain(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-01/status", readMQTTV1Fixture(t, "edge-status.json"), 1, true)
	publishMQTTMessageWithRetain(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-01/device/device-01/status", readMQTTV1Fixture(t, "device-status.json"), 1, true)
	publishMQTTMessage(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-01/device/device-01/raw", readMQTTV1Fixture(t, "raw-register-snapshot.json"), 0)
	publishMQTTMessage(t, endpointForTest(broker, tlsConfig), tlsConfig, "edge/edge-01/device/device-01/event", readMQTTV1Fixture(t, "device-event.json"), 1)
}

func endpointForTest(broker *mqttBrokerFixture, tlsConfig *tls.Config) mqttEndpoint {
	if tlsConfig != nil {
		return broker.tls
	}
	return broker.plaintext
}

func publishMQTTMessage(t *testing.T, endpoint mqttEndpoint, tlsConfig *tls.Config, topic string, payload []byte, qos byte) {
	publishMQTTMessageWithRetain(t, endpoint, tlsConfig, topic, payload, qos, false)
}

func publishMQTTMessageWithRetain(t *testing.T, endpoint mqttEndpoint, tlsConfig *tls.Config, topic string, payload []byte, qos byte, retained bool) {
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
	publish := client.Publish(topic, qos, retained, payload)
	if !publish.WaitTimeout(10*time.Second) || publish.Error() != nil {
		t.Fatalf("publish %s: %v", topic, publish.Error())
	}
	client.Disconnect(100)
}

func readMQTTV1Fixture(t *testing.T, name string) []byte {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "..", "testdata", "mqtt-v1", name))
	if err != nil {
		t.Fatalf("read MQTT v1 fixture %s: %v", name, err)
	}
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
