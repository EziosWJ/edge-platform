package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromDirUsesFixedPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `
service:
  name: base-file
http:
  address: ":8081"
database:
  url: postgres://localhost:5432/base_file?sslmode=disable
  username: yaml-user
  password: yaml-password
jwt:
  secret: yaml-secret
log:
  level: info
  format: json
`)
	writeConfig(t, dir, "config.test.yaml", `
service:
  name: environment-file
http:
  address: ":8082"
log:
  level: warn
  format: text
`)

	t.Setenv("APP_ENV", "TEST")
	t.Setenv("APP_SERVICE__NAME", "environment-variable")
	t.Setenv("APP_HTTP__ADDRESS", ":9090")
	t.Setenv("APP_CORS__ALLOWED_ORIGINS", "https://admin.example, https://ops.example")
	t.Setenv("APP_HTTP__TRUSTED_PROXIES", "10.0.0.0/8, 127.0.0.1")
	t.Setenv("APP_DATABASE__USERNAME", "environment-user")
	t.Setenv("APP_JWT__SECRET", "test-only-secret")
	t.Setenv("APP_AUTH__LOGIN_GUARD__IP_MAX_ATTEMPTS", "7")

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	if cfg.Environment != EnvironmentTest {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, EnvironmentTest)
	}
	if cfg.Service.Name != "environment-variable" {
		t.Errorf("Service.Name = %q, want environment-variable", cfg.Service.Name)
	}
	if cfg.HTTP.Address != ":9090" {
		t.Errorf("HTTP.Address = %q, want :9090", cfg.HTTP.Address)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want environment YAML value warn", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want environment YAML value text", cfg.Log.Format)
	}
	wantOrigins := []string{"https://admin.example", "https://ops.example"}
	if len(cfg.CORS.AllowedOrigins) != len(wantOrigins) {
		t.Fatalf("CORS.AllowedOrigins = %#v, want %#v", cfg.CORS.AllowedOrigins, wantOrigins)
	}
	for i := range wantOrigins {
		if cfg.CORS.AllowedOrigins[i] != wantOrigins[i] {
			t.Errorf("CORS.AllowedOrigins[%d] = %q, want %q", i, cfg.CORS.AllowedOrigins[i], wantOrigins[i])
		}
	}
	if len(cfg.HTTP.TrustedProxies) != 2 || cfg.HTTP.TrustedProxies[0] != "10.0.0.0/8" || cfg.HTTP.TrustedProxies[1] != "127.0.0.1" {
		t.Errorf("HTTP.TrustedProxies = %#v, want parsed proxy list", cfg.HTTP.TrustedProxies)
	}
	if cfg.Auth.LoginGuard.IPMaxAttempts != 7 || cfg.Auth.LoginGuard.UsernameMaxAttempts != 5 {
		t.Errorf("login guard config = %+v, want IP override and username default", cfg.Auth.LoginGuard)
	}
	if cfg.Database.URL != "postgres://localhost:5432/base_file?sslmode=disable" {
		t.Errorf("Database.URL = %q, want YAML value", cfg.Database.URL)
	}
	if cfg.Database.Username != "environment-user" {
		t.Errorf("Database.Username = %q, want environment-user", cfg.Database.Username)
	}
	if cfg.Database.Password != "yaml-password" {
		t.Errorf("Database.Password = %q, want YAML value", cfg.Database.Password)
	}
	if cfg.JWT.Secret != "test-only-secret" {
		t.Error("JWT secret from environment was not loaded")
	}
	if cfg.JWT.TTL != 2*time.Hour {
		t.Errorf("JWT.TTL = %s, want 2h", cfg.JWT.TTL)
	}
}

func TestLoadFromDirLoadsMQTTBaselineAndAPPOverrides(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `
database:
  url: postgres://localhost:5432/base?sslmode=disable
  username: user
  password: password
jwt:
  secret: secret
mqtt:
  enabled: false
  url: mqtts://broker.example:8883
  username: yaml-user
  password: yaml-password
`)
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_MQTT__ENABLED", "true")
	t.Setenv("APP_MQTT__PREFIX", "factory")
	t.Setenv("APP_MQTT__KEEPALIVE", "45s")
	t.Setenv("APP_MQTT__PASSWORD", "environment-password")

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	if !cfg.MQTT.Enabled || cfg.MQTT.URL != "mqtts://broker.example:8883" {
		t.Fatalf("MQTT enabled/URL = (%v, %q)", cfg.MQTT.Enabled, cfg.MQTT.URL)
	}
	if cfg.MQTT.Protocol != MQTTProtocol5 || cfg.MQTT.ClientID != "edge-platform-server" {
		t.Errorf("MQTT protocol/client ID = (%q, %q), want default baseline", cfg.MQTT.Protocol, cfg.MQTT.ClientID)
	}
	if cfg.MQTT.Prefix != "factory" || cfg.MQTT.KeepAlive != 45*time.Second {
		t.Errorf("MQTT APP_ overrides = (%q, %s)", cfg.MQTT.Prefix, cfg.MQTT.KeepAlive)
	}
	if cfg.MQTT.Username != "yaml-user" || cfg.MQTT.Password != "environment-password" {
		t.Errorf("MQTT credentials precedence = (%q, %q)", cfg.MQTT.Username, cfg.MQTT.Password)
	}
	if cfg.MQTT.ConnectTimeout != 10*time.Second || cfg.MQTT.ReconnectMin != time.Second || cfg.MQTT.ReconnectMax != 30*time.Second {
		t.Errorf("MQTT reconnect baseline = (%s, %s, %s)", cfg.MQTT.ConnectTimeout, cfg.MQTT.ReconnectMin, cfg.MQTT.ReconnectMax)
	}
	if cfg.MQTT.SessionExpiry != 24*time.Hour || cfg.MQTT.MaxPayloadBytes != 1<<20 || cfg.MQTT.ReliableQueueSize != 1024 || cfg.MQTT.RawQueueSize != 256 {
		t.Errorf("MQTT delivery baseline = (%s, %d, %d, %d)", cfg.MQTT.SessionExpiry, cfg.MQTT.MaxPayloadBytes, cfg.MQTT.ReliableQueueSize, cfg.MQTT.RawQueueSize)
	}
	if cfg.MQTT.ConsumerTimeout != 5*time.Second || cfg.MQTT.ShutdownTimeout != 10*time.Second {
		t.Errorf("MQTT timeout baseline = (%s, %s)", cfg.MQTT.ConsumerTimeout, cfg.MQTT.ShutdownTimeout)
	}
}

func TestValidateMQTTSecurityConstraints(t *testing.T) {
	base := func() Config {
		return Config{
			MQTT: MQTTConfig{
				Enabled:           true,
				URL:               "mqtts://broker.example:8883",
				Protocol:          MQTTProtocol5,
				ClientID:          "test-client",
				Prefix:            "edge",
				KeepAlive:         30 * time.Second,
				ConnectTimeout:    10 * time.Second,
				ReconnectMin:      time.Second,
				ReconnectMax:      30 * time.Second,
				SessionExpiry:     24 * time.Hour,
				MaxPayloadBytes:   1 << 20,
				ReliableQueueSize: 1024,
				RawQueueSize:      256,
				ConsumerTimeout:   5 * time.Second,
				ShutdownTimeout:   10 * time.Second,
				ClientCertFile:    "/etc/edge/client.crt",
				ClientKeyFile:     "/etc/edge/client.key",
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*MQTTConfig)
		wantErr string
	}{
		{name: "userinfo", mutate: func(c *MQTTConfig) { c.URL = "mqtts://user:secret@broker.example:8883" }, wantErr: "must not contain userinfo"},
		{name: "insecure verify", mutate: func(c *MQTTConfig) { c.InsecureSkipVerify = true }, wantErr: "insecure_skip_verify is not supported"},
		{name: "certificate pair", mutate: func(c *MQTTConfig) { c.ClientKeyFile = "" }, wantErr: "client_key_file is required"},
		{name: "relative CA path", mutate: func(c *MQTTConfig) { c.CAFile = "ca.pem" }, wantErr: "ca_file must be an absolute file path"},
		{name: "mqtt TLS path", mutate: func(c *MQTTConfig) { c.URL = "mqtt://broker.example:1883" }, wantErr: "require an mqtts URL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base()
			test.mutate(&cfg.MQTT)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadFromDirRejectsInvalidTrustedProxyAndLoginGuard(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{name: "trusted proxy", config: "http:\n  trusted_proxies: [not-a-proxy]\n", wantErr: "http.trusted_proxies[0] must be an IP or CIDR"},
		{name: "negative IP limit", config: "auth:\n  login_guard:\n    ip_max_attempts: -1\n", wantErr: "auth.login_guard.ip_max_attempts must not be negative"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost:5432/base?sslmode=disable\n  username: user\n  password: password\njwt:\n  secret: secret\n"+test.config)
			t.Setenv("APP_ENV", "test")

			_, err := LoadFromDir(dir)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadFromDir() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadFromDirRequiresDatabaseAndJWTConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "{}\n")
	t.Setenv("APP_ENV", "test")

	_, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("LoadFromDir() error = nil, want required database and JWT configuration error")
	}
	for _, want := range []string{"database.url is required", "database.username is required", "database.password is required", "jwt.secret is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadFromDir() error = %v, want containing %q", err, want)
		}
	}
}

// TestLoadFromDirDevSucceedsWithoutEnvironmentYAML documents that the actual
// environment YAML is optional: with only config.yaml present, APP_ENV=dev
// and APP_ overrides must still produce the intended development settings.
// This is exactly what Docker Compose relies on, since it never sees a
// config.dev.yaml inside the image.
func TestLoadFromDirDevSucceedsWithoutEnvironmentYAML(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `
swagger:
  enabled: false
database:
  url: postgres://postgres:5432/edge_platform?sslmode=disable
log:
  level: info
  format: json
`)
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_DATABASE__USERNAME", "compose-user")
	t.Setenv("APP_DATABASE__PASSWORD", "compose-password")
	t.Setenv("APP_JWT__SECRET", "compose-secret")
	t.Setenv("APP_SWAGGER__ENABLED", "true")
	t.Setenv("APP_LOG__LEVEL", "debug")
	t.Setenv("APP_LOG__FORMAT", "text")

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() without environment YAML error = %v", err)
	}

	if cfg.Environment != EnvironmentDev {
		t.Errorf("Environment = %q, want dev", cfg.Environment)
	}
	if cfg.HTTP.ReadHeaderTimeout != 10*time.Second || cfg.HTTP.ReadTimeout != 5*time.Minute {
		t.Errorf("HTTP timeouts = (%s, %s), want (10s, 5m)", cfg.HTTP.ReadHeaderTimeout, cfg.HTTP.ReadTimeout)
	}
	if cfg.Swagger.Enabled != true {
		t.Errorf("Swagger.Enabled = %v, want true from APP_ override", cfg.Swagger.Enabled)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Errorf("Log = (%s, %s), want (debug, text) from APP_ overrides", cfg.Log.Level, cfg.Log.Format)
	}
	if cfg.Database.Username != "compose-user" || cfg.Database.Password != "compose-password" {
		t.Errorf("Database.Username/Password = (%s, %s), want compose env values", cfg.Database.Username, cfg.Database.Password)
	}
	if cfg.JWT.Secret != "compose-secret" {
		t.Errorf("JWT.Secret = %q, want compose env value", cfg.JWT.Secret)
	}
}

func TestLoadFromDirAllowsPersistentSQLiteWithoutCredentials(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `
database:
  driver: sqlite
  url: /var/lib/server/data/app.db
jwt:
  secret: sqlite-test-secret
`)
	t.Setenv("APP_ENV", "test")

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() SQLite error = %v", err)
	}
	if cfg.Database.Driver != "sqlite" || cfg.Database.URL != "/var/lib/server/data/app.db" {
		t.Fatalf("SQLite database config = %+v", cfg.Database)
	}
	if cfg.Database.Username != "" || cfg.Database.Password != "" {
		t.Fatalf("SQLite credentials = (%q, %q), want empty", cfg.Database.Username, cfg.Database.Password)
	}
}

func TestLoadFromDirSQLiteProfileOverridesEnvironmentDatabase(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `
database:
  url: postgres://localhost:5432/base?sslmode=disable
  username: postgres-user
  password: postgres-password
jwt:
  secret: sqlite-test-secret
`)
	writeConfig(t, dir, "config.dev.yaml", `
database:
  driver: postgres
  username: dev-user
  password: dev-password
`)
	writeConfig(t, dir, "config.sqlite.yaml", `
database:
  driver: sqlite
  url: .data/server.db
  username: ""
  password: ""
`)
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_CONFIG_PROFILE", "sqlite")

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() SQLite profile error = %v", err)
	}
	if cfg.Database.Driver != "sqlite" || cfg.Database.URL != ".data/server.db" {
		t.Fatalf("SQLite profile database = %+v", cfg.Database)
	}
	if cfg.Database.Username != "" || cfg.Database.Password != "" {
		t.Fatalf("SQLite profile credentials = (%q, %q), want empty", cfg.Database.Username, cfg.Database.Password)
	}
}

func TestLoadFromDirRejectsUnknownConfigProfile(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "database:\n  url: postgres://localhost:5432/base?sslmode=disable\n  username: user\n  password: password\njwt:\n  secret: secret\n")
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_CONFIG_PROFILE", "mysql")

	_, err := LoadFromDir(dir)
	if err == nil || !strings.Contains(err.Error(), "APP_CONFIG_PROFILE") {
		t.Fatalf("LoadFromDir() error = %v, want unknown profile error", err)
	}
}

func TestLoadFromDirRejectsUnsupportedSQLiteConfigurations(t *testing.T) {
	for _, test := range []struct {
		name     string
		database string
		wantErr  string
	}{
		{name: "in memory", database: "driver: sqlite\n  url: ':memory:'\n", wantErr: "in-memory SQLite"},
		{name: "username", database: "driver: sqlite\n  url: /tmp/app.db\n  username: user\n", wantErr: "database.username must be empty"},
		{name: "password", database: "driver: sqlite\n  url: /tmp/app.db\n  password: secret\n", wantErr: "database.password must be empty"},
		{name: "mysql", database: "driver: mysql\n  url: mysql://localhost/app\n  username: user\n  password: secret\n", wantErr: "mysql is not supported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "config.yaml", "database:\n  "+test.database+"jwt:\n  secret: sqlite-test-secret\n")
			t.Setenv("APP_ENV", "test")

			_, err := LoadFromDir(dir)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadFromDir() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func writeConfig(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", name, err)
	}
}
