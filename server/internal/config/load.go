package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	koanf "github.com/knadh/koanf/v2"
)

const delimiter = "."

const sqliteConfigProfile = "sqlite"

// Load reads configuration from DefaultDir.
func Load() (*Config, error) {
	return LoadFromDir(DefaultDir)
}

// LoadFromDir loads configuration in the fixed precedence order:
// defaults, config.yaml, config.{APP_ENV}.yaml, an optional named profile,
// then APP_ environment variables. Environment-specific YAML is optional;
// config.yaml is required.
func LoadFromDir(dir string) (*Config, error) {
	environment, err := selectedEnvironment()
	if err != nil {
		return nil, err
	}

	k := koanf.New(delimiter)
	if err := k.Load(confmap.Provider(defaultValues(), delimiter), nil); err != nil {
		return nil, fmt.Errorf("load default config: %w", err)
	}

	basePath := filepath.Join(dir, "config.yaml")
	if err := loadYAML(k, basePath, true); err != nil {
		return nil, err
	}

	environmentPath := filepath.Join(dir, "config."+environment+".yaml")
	if err := loadYAML(k, environmentPath, false); err != nil {
		return nil, err
	}

	if err := loadConfigProfile(k, dir); err != nil {
		return nil, err
	}

	// APP_ENV is the authoritative environment selector. Do not allow a YAML
	// key to select one file while reporting a different runtime environment.
	if err := k.Set("env", environment); err != nil {
		return nil, fmt.Errorf("set selected environment: %w", err)
	}

	if err := k.Load(env.ProviderWithValue("APP_", delimiter, envKeyValue), nil); err != nil {
		return nil, fmt.Errorf("load APP_ environment: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func loadConfigProfile(k *koanf.Koanf, dir string) error {
	profile := strings.TrimSpace(strings.ToLower(os.Getenv("APP_CONFIG_PROFILE")))
	if profile == "" {
		return nil
	}
	if profile != sqliteConfigProfile {
		return fmt.Errorf("APP_CONFIG_PROFILE must be empty or %q, got %q", sqliteConfigProfile, profile)
	}
	return loadYAML(k, filepath.Join(dir, "config."+profile+".yaml"), true)
}

func selectedEnvironment() (string, error) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("APP_ENV")))
	if value == "" {
		return EnvironmentDev, nil
	}
	if !oneOf(value, EnvironmentDev, EnvironmentTest, EnvironmentProd) {
		return "", fmt.Errorf("APP_ENV must be one of dev, test, prod, got %q", value)
	}

	return value, nil
}

func loadYAML(k *koanf.Koanf, path string, required bool) error {
	err := k.Load(file.Provider(path), yaml.Parser())
	if err == nil {
		return nil
	}
	if !required && errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return fmt.Errorf("load %s: %w", path, err)
}

func envKeyValue(key, value string) (string, interface{}) {
	key = strings.TrimPrefix(key, "APP_")
	key = strings.ToLower(strings.ReplaceAll(key, "__", delimiter))

	switch key {
	case "env":
		return key, strings.ToLower(strings.TrimSpace(value))
	case "cors.allowed_origins", "cors.allowed_methods", "cors.allowed_headers", "cors.exposed_headers", "http.trusted_proxies":
		return key, splitList(value)
	default:
		return key, value
	}
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}

	return values
}

func defaultValues() map[string]interface{} {
	return map[string]interface{}{
		"env":                                    EnvironmentDev,
		"service.name":                           "edge-platform-server",
		"http.address":                           ":8080",
		"http.trusted_proxies":                   []string{},
		"http.read_header_timeout":               "10s",
		"http.read_timeout":                      "5m",
		"http.write_timeout":                     "15s",
		"http.idle_timeout":                      "60s",
		"http.shutdown_timeout":                  "10s",
		"swagger.enabled":                        false,
		"cors.allowed_origins":                   []string{"*"},
		"cors.allowed_methods":                   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		"cors.allowed_headers":                   []string{"Authorization", "Content-Type", "X-Request-ID"},
		"cors.exposed_headers":                   []string{"X-Request-ID"},
		"cors.max_age":                           "12h",
		"cors.allow_credentials":                 false,
		"database.driver":                        "postgres",
		"database.max_open_conns":                25,
		"database.max_idle_conns":                5,
		"database.conn_max_lifetime":             "30m",
		"database.conn_max_idle_time":            "5m",
		"file.storage_root":                      "/var/lib/edge-platform/uploads",
		"jwt.issuer":                             "edge-platform-server",
		"jwt.audience":                           "edge-platform-web",
		"jwt.ttl":                                "2h",
		"auth.login_guard.ip_window":             "10m",
		"auth.login_guard.ip_max_attempts":       20,
		"auth.login_guard.username_window":       "10m",
		"auth.login_guard.username_max_attempts": 5,
		"auth.login_guard.backoff_initial":       "1s",
		"auth.login_guard.backoff_max":           "30s",
		"auth.login_guard.lock_duration":         "10m",
		"auth.login_guard.max_entries":           10000,
		"log.level":                              "info",
		"log.format":                             "json",
		"log.add_source":                         false,
	}
}
