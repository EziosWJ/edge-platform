//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mqtt311 "github.com/eclipse/paho.mqtt.golang"
)

const mosquittoImage = "eclipse-mosquitto:2.0.22"

const (
	mosquittoPlaintextPort = 1883
	mosquittoTLSPort       = 8883
	mosquittoStartupWait   = 45 * time.Second
)

// mqttBrokerOptions controls the broker features needed by later protocol
// tests. Persistence is always enabled so a fixture can be restarted without
// changing the broker configuration used by a test.
type mqttBrokerOptions struct {
	requireClientCertificate bool
	serverCertificateDNSOnly bool
}

type mqttEndpoint struct {
	Host string
	Port int
}

func (endpoint mqttEndpoint) Address() string {
	return net.JoinHostPort(endpoint.Host, fmt.Sprintf("%d", endpoint.Port))
}

func (endpoint mqttEndpoint) URL(scheme string) string {
	return scheme + "://" + endpoint.Address()
}

// mqttTLSMaterial contains a private test CA, server certificate, and client
// certificate. It is generated per test and mounted into Mosquitto read-only.
// The client certificate is available even for a non-mTLS fixture so future
// tests can opt into client authentication without changing the helper shape.
type mqttTLSMaterial struct {
	caPath         string
	serverCertPath string
	serverKeyPath  string
	clientCertPath string
	clientKeyPath  string
	caCertificate  *x509.CertPool
}

// mqttBrokerFixture is deliberately independent from an MQTT client library.
// Protocol-specific tests can use Plaintext/TLS and TLSConfig with either an
// MQTT 5 or MQTT 3.1.1 adapter once the server MQTT API is available.
type mqttBrokerFixture struct {
	container string
	plaintext mqttEndpoint
	tls       mqttEndpoint
	tlsFiles  mqttTLSMaterial
	options   mqttBrokerOptions
}

// TestMosquittoFixtureProvidesPlaintextAndTLS verifies the reusable broker
// base without pretending that the MQTT runtime business contract exists yet.
func TestMosquittoFixtureProvidesPlaintextAndTLS(t *testing.T) {
	broker := startMQTTBroker(t, mqttBrokerOptions{})

	if broker.plaintext.Port == broker.tls.Port {
		t.Fatalf("Mosquitto exposed the same port for plaintext and TLS: %d", broker.plaintext.Port)
	}
	if got := broker.plaintext.URL("mqtt"); !strings.HasPrefix(got, "mqtt://127.0.0.1:") {
		t.Fatalf("plaintext endpoint = %q, want a loopback mqtt URL", got)
	}
	if got := broker.tls.URL("mqtts"); !strings.HasPrefix(got, "mqtts://127.0.0.1:") {
		t.Fatalf("TLS endpoint = %q, want a loopback mqtts URL", got)
	}
}

// TestMosquittoFixtureSupportsMTLSAndRestart exercises the two lifecycle
// properties needed by future MQTT 5/3.1.1 reconnect and persistent-session
// tests: client certificate authentication and broker restart.
func TestMosquittoFixtureSupportsMTLSAndRestart(t *testing.T) {
	broker := startMQTTBroker(t, mqttBrokerOptions{requireClientCertificate: true})
	broker.restart(t)
}

func TestMosquittoFixtureQueuesQoS1ForPersistentClient(t *testing.T) {
	broker := startMQTTBroker(t, mqttBrokerOptions{})
	clientID := fmt.Sprintf("persistent-fixture-%d", time.Now().UnixNano())
	topic := "edge/edge-01/device/device-01/event"
	received := make(chan struct{}, 1)

	firstOptions := mqtt311.NewClientOptions().
		AddBroker(broker.plaintext.URL("mqtt")).
		SetClientID(clientID).
		SetCleanSession(false).
		SetAutoReconnect(false).
		SetConnectRetry(false)
	first := mqtt311.NewClient(firstOptions)
	connect := first.Connect()
	if !connect.WaitTimeout(10*time.Second) || connect.Error() != nil {
		t.Fatalf("connect persistent fixture client: %v", connect.Error())
	}
	subscribe := first.Subscribe("edge/+/device/+/event", 1, nil)
	if !subscribe.WaitTimeout(10*time.Second) || subscribe.Error() != nil {
		t.Fatalf("subscribe persistent fixture client: %v", subscribe.Error())
	}
	first.Disconnect(100)

	publishMQTTMessage(t, broker.plaintext, nil, topic, readMQTTV1Fixture(t, "device-event.json"), 1)

	secondOptions := mqtt311.NewClientOptions().
		AddBroker(broker.plaintext.URL("mqtt")).
		SetClientID(clientID).
		SetCleanSession(false).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetDefaultPublishHandler(func(_ mqtt311.Client, _ mqtt311.Message) {
			received <- struct{}{}
		})
	second := mqtt311.NewClient(secondOptions)
	secondConnect := second.Connect()
	if !secondConnect.WaitTimeout(10*time.Second) || secondConnect.Error() != nil {
		t.Fatalf("reconnect persistent fixture client: %v", secondConnect.Error())
	}
	t.Cleanup(func() { second.Disconnect(100) })

	select {
	case <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("persistent fixture client did not receive queued QoS1 message")
	}
}

func startMQTTBroker(t *testing.T, options mqttBrokerOptions) *mqttBrokerFixture {
	t.Helper()
	requireDocker(t)

	fixtureDir := t.TempDir()
	// t.TempDir creates a private 0700 directory. Mosquitto runs as a
	// non-root user in the official image, so the container must be able to
	// traverse the temporary directory to reach its mounted configuration.
	if err := os.Chmod(fixtureDir, 0o755); err != nil {
		t.Fatalf("make Mosquitto fixture directory traversable: %v", err)
	}
	configDir := filepath.Join(fixtureDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create Mosquitto config directory: %v", err)
	}
	ports := freeTCPPorts(t, 2)

	tlsFiles := createMQTTTLSMaterial(t, configDir, !options.serverCertificateDNSOnly)
	writeMosquittoConfig(t, configDir, options.requireClientCertificate)

	container := fmt.Sprintf("edge-platform-mqtt-%d", time.Now().UnixNano())
	cleanup := func() {
		_ = exec.Command("docker", "rm", "--force", container).Run()
	}
	t.Cleanup(cleanup)

	command := exec.Command(
		"docker", "run", "--detach", "--rm", "--name", container,
		"--publish", "127.0.0.1:"+strconv.Itoa(ports[0])+":1883/tcp",
		"--publish", "127.0.0.1:"+strconv.Itoa(ports[1])+":8883/tcp",
		"--volume", configDir+":/mosquitto/config:ro",
		mosquittoImage,
		"mosquitto", "-c", "/mosquitto/config/mosquitto.conf",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start Mosquitto %s: %v\n%s", mosquittoImage, err, output)
	}

	broker := &mqttBrokerFixture{
		container: container,
		plaintext: mqttEndpoint{Host: "127.0.0.1", Port: ports[0]},
		tls:       mqttEndpoint{Host: "127.0.0.1", Port: ports[1]},
		tlsFiles:  tlsFiles,
		options:   options,
	}
	broker.waitUntilReady(t)
	return broker
}

func (broker *mqttBrokerFixture) restart(t *testing.T) {
	t.Helper()
	command := exec.Command("docker", "restart", broker.container)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("restart Mosquitto container %s: %v\n%s", broker.container, err, output)
	}
	broker.waitUntilReady(t)
}

func (broker *mqttBrokerFixture) waitUntilReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(mosquittoStartupWait)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := dialEndpoint(broker.plaintext); err != nil {
			lastErr = fmt.Errorf("plaintext listener: %w", err)
			time.Sleep(250 * time.Millisecond)
			continue
		}

		tlsConfig := broker.tlsConfig(t, broker.options.requireClientCertificate)
		connection, err := tlsDial(broker.tls, tlsConfig)
		if err == nil {
			_ = connection.Close()
			return
		}
		lastErr = fmt.Errorf("TLS listener: %w", err)
		time.Sleep(250 * time.Millisecond)
	}

	logs, _ := exec.Command("docker", "logs", broker.container).CombinedOutput()
	t.Fatalf("Mosquitto did not become ready within %s: %v\nlogs:\n%s", mosquittoStartupWait, lastErr, logs)
}

func (broker *mqttBrokerFixture) tlsConfig(t *testing.T, withClientCertificate bool) *tls.Config {
	t.Helper()
	config := &tls.Config{
		RootCAs:    broker.tlsFiles.caCertificate,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	}
	if withClientCertificate {
		certificate, err := tls.LoadX509KeyPair(broker.tlsFiles.clientCertPath, broker.tlsFiles.clientKeyPath)
		if err != nil {
			t.Fatalf("load generated MQTT client certificate: %v", err)
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	return config
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker CLI is not available; skipping MQTT integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput(); err != nil {
		t.Skipf("Docker daemon is not available; skipping MQTT integration test: %v (%s)", err, strings.TrimSpace(string(output)))
	}
}

func freeTCPPorts(t *testing.T, count int) []int {
	t.Helper()
	listeners := make([]net.Listener, 0, count)
	ports := make([]int, 0, count)
	for range count {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			for _, openListener := range listeners {
				_ = openListener.Close()
			}
			t.Fatalf("allocate Mosquitto host port: %v", err)
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatalf("release Mosquitto host port: %v", err)
		}
	}
	return ports
}

func dialEndpoint(endpoint mqttEndpoint) error {
	connection, err := net.DialTimeout("tcp", endpoint.Address(), time.Second)
	if err != nil {
		return err
	}
	return connection.Close()
}

func tlsDial(endpoint mqttEndpoint, config *tls.Config) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: time.Second}
	return tls.DialWithDialer(dialer, "tcp", endpoint.Address(), config)
}

func writeMosquittoConfig(t *testing.T, configDir string, requireClientCertificate bool) {
	t.Helper()
	requireCertificate := "false"
	if requireClientCertificate {
		requireCertificate = "true"
	}
	config := fmt.Sprintf(`persistence true
persistence_location /mosquitto/data/
autosave_interval 1
# Mosquitto 2.0.12+ does not persist sessions with per_listener_settings.
max_queued_messages 1000
log_dest stdout

listener %d 0.0.0.0
allow_anonymous true

listener %d 0.0.0.0
allow_anonymous true
cafile /mosquitto/config/ca.crt
certfile /mosquitto/config/server.crt
keyfile /mosquitto/config/server.key
require_certificate %s
use_identity_as_username false
tls_version tlsv1.2
`, mosquittoPlaintextPort, mosquittoTLSPort, requireCertificate)
	if err := os.WriteFile(filepath.Join(configDir, "mosquitto.conf"), []byte(config), 0o644); err != nil {
		t.Fatalf("write Mosquitto config: %v", err)
	}
}

func createMQTTTLSMaterial(t *testing.T, configDir string, serverCertificateIncludesIP bool) mqttTLSMaterial {
	t.Helper()
	caCertificate := createCertificate(t, "mqtt-integration-ca", nil, true, false, false)
	serverCertificate := createCertificate(t, "localhost", &caCertificate, false, false, serverCertificateIncludesIP)
	clientCertificate := createCertificate(t, "mqtt-integration-client", &caCertificate, false, true, false)

	material := mqttTLSMaterial{
		caPath:         filepath.Join(configDir, "ca.crt"),
		serverCertPath: filepath.Join(configDir, "server.crt"),
		serverKeyPath:  filepath.Join(configDir, "server.key"),
		clientCertPath: filepath.Join(configDir, "client.crt"),
		clientKeyPath:  filepath.Join(configDir, "client.key"),
		caCertificate:  x509.NewCertPool(),
	}
	if !material.caCertificate.AppendCertsFromPEM(caCertificate.certificatePEM) {
		t.Fatal("append generated MQTT CA certificate to trust pool")
	}
	writeCertificateFile(t, material.caPath, caCertificate.certificatePEM)
	writeCertificateFile(t, material.serverCertPath, serverCertificate.certificatePEM)
	writeCertificateFile(t, material.serverKeyPath, serverCertificate.privateKeyPEM)
	writeCertificateFile(t, material.clientCertPath, clientCertificate.certificatePEM)
	writeCertificateFile(t, material.clientKeyPath, clientCertificate.privateKeyPEM)
	return material
}

type generatedCertificate struct {
	certificatePEM []byte
	privateKeyPEM  []byte
}

func createCertificate(t *testing.T, commonName string, issuer *generatedCertificate, isCA, clientAuth, includeServerIP bool) generatedCertificate {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate MQTT test certificate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatalf("generate MQTT test certificate serial: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	if clientAuth {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else if !isCA {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.DNSNames = []string{"localhost"}
		if includeServerIP {
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
	}

	issuerCertificate := template
	issuerKey := privateKey
	if issuer != nil {
		issuerCertificate, err = x509.ParseCertificate(pemDecode(t, issuer.certificatePEM))
		if err != nil {
			t.Fatalf("parse MQTT test CA certificate: %v", err)
		}
		issuerKey, err = x509.ParseECPrivateKey(pemDecode(t, issuer.privateKeyPEM))
		if err != nil {
			t.Fatalf("parse MQTT test CA key: %v", err)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuerCertificate, &privateKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("create MQTT test certificate %s: %v", commonName, err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	privateKeyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal MQTT test key %s: %v", commonName, err)
	}
	return generatedCertificate{
		certificatePEM: certificatePEM,
		privateKeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyDER}),
	}
}

func pemDecode(t *testing.T, value []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(value)
	if block == nil {
		t.Fatal("decode generated MQTT PEM")
	}
	return block.Bytes
}

func writeCertificateFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	// The temporary directory is private to this test. Mosquitto runs as a
	// non-root user in the official image, so mounted key files must be readable.
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write MQTT TLS fixture %s: %v", path, err)
	}
}
