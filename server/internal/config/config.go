package config

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultDir is the configuration directory used by Load.
	DefaultDir = "configs"

	EnvironmentDev  = "dev"
	EnvironmentTest = "test"
	EnvironmentProd = "prod"
)

// Config contains all process-level configuration. Environment-specific YAML
// may contain deployment credentials; APP_ environment variables override YAML.
type Config struct {
	Environment string         `koanf:"env"`
	Service     ServiceConfig  `koanf:"service"`
	HTTP        HTTPConfig     `koanf:"http"`
	Swagger     SwaggerConfig  `koanf:"swagger"`
	CORS        CORSConfig     `koanf:"cors"`
	Database    DatabaseConfig `koanf:"database"`
	File        FileConfig     `koanf:"file"`
	JWT         JWTConfig      `koanf:"jwt"`
	Auth        AuthConfig     `koanf:"auth"`
	Log         LogConfig      `koanf:"log"`
}

type ServiceConfig struct {
	Name string `koanf:"name"`
}

type HTTPConfig struct {
	Address           string        `koanf:"address"`
	TrustedProxies    []string      `koanf:"trusted_proxies"`
	ReadHeaderTimeout time.Duration `koanf:"read_header_timeout"`
	ReadTimeout       time.Duration `koanf:"read_timeout"`
	WriteTimeout      time.Duration `koanf:"write_timeout"`
	IdleTimeout       time.Duration `koanf:"idle_timeout"`
	ShutdownTimeout   time.Duration `koanf:"shutdown_timeout"`
}

type SwaggerConfig struct {
	Enabled bool `koanf:"enabled"`
}

type CORSConfig struct {
	AllowedOrigins   []string      `koanf:"allowed_origins"`
	AllowedMethods   []string      `koanf:"allowed_methods"`
	AllowedHeaders   []string      `koanf:"allowed_headers"`
	ExposedHeaders   []string      `koanf:"exposed_headers"`
	MaxAge           time.Duration `koanf:"max_age"`
	AllowCredentials bool          `koanf:"allow_credentials"`
}

type DatabaseConfig struct {
	Driver          string        `koanf:"driver"`
	URL             string        `koanf:"url"`
	Username        string        `koanf:"username"`
	Password        string        `koanf:"password"`
	MaxOpenConns    int           `koanf:"max_open_conns"`
	MaxIdleConns    int           `koanf:"max_idle_conns"`
	ConnMaxLifetime time.Duration `koanf:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `koanf:"conn_max_idle_time"`
}
type FileConfig struct {
	StorageRoot string `koanf:"storage_root"`
}

type JWTConfig struct {
	Secret   string        `koanf:"secret"`
	Issuer   string        `koanf:"issuer"`
	Audience string        `koanf:"audience"`
	TTL      time.Duration `koanf:"ttl"`
}

type AuthConfig struct {
	LoginGuard LoginGuardConfig `koanf:"login_guard"`
}

type LoginGuardConfig struct {
	IPWindow            time.Duration `koanf:"ip_window"`
	IPMaxAttempts       int           `koanf:"ip_max_attempts"`
	UsernameWindow      time.Duration `koanf:"username_window"`
	UsernameMaxAttempts int           `koanf:"username_max_attempts"`
	BackoffInitial      time.Duration `koanf:"backoff_initial"`
	BackoffMax          time.Duration `koanf:"backoff_max"`
	LockDuration        time.Duration `koanf:"lock_duration"`
	MaxEntries          int           `koanf:"max_entries"`
}

type LogConfig struct {
	Level     string `koanf:"level"`
	Format    string `koanf:"format"`
	AddSource bool   `koanf:"add_source"`
}

// Validate rejects invalid startup configuration before infrastructure is
// created. This keeps configuration errors deterministic and close to startup.
func (c Config) Validate() error {
	var errs []error

	if !oneOf(c.Environment, EnvironmentDev, EnvironmentTest, EnvironmentProd) {
		errs = append(errs, fmt.Errorf("env must be one of dev, test, prod"))
	}
	if strings.TrimSpace(c.Service.Name) == "" {
		errs = append(errs, errors.New("service.name is required"))
	}

	if err := validateAddress(c.HTTP.Address); err != nil {
		errs = append(errs, err)
	}
	if c.HTTP.ReadHeaderTimeout <= 0 {
		errs = append(errs, errors.New("http.read_header_timeout must be greater than zero"))
	}
	if c.HTTP.ReadTimeout <= 0 {
		errs = append(errs, errors.New("http.read_timeout must be greater than zero"))
	}
	if c.HTTP.WriteTimeout <= 0 {
		errs = append(errs, errors.New("http.write_timeout must be greater than zero"))
	}
	if c.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("http.idle_timeout must be greater than zero"))
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("http.shutdown_timeout must be greater than zero"))
	}
	for index, proxy := range c.HTTP.TrustedProxies {
		proxy = strings.TrimSpace(proxy)
		if proxy == "" {
			errs = append(errs, fmt.Errorf("http.trusted_proxies[%d] must not be empty", index))
			continue
		}
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				errs = append(errs, fmt.Errorf("http.trusted_proxies[%d] must be an IP or CIDR", index))
			}
		}
	}
	if c.Swagger.Enabled && c.Environment != EnvironmentDev {
		errs = append(errs, errors.New("swagger.enabled may only be true in dev"))
	}

	if len(c.CORS.AllowedOrigins) == 0 {
		errs = append(errs, errors.New("cors.allowed_origins must not be empty"))
	}
	if contains(c.CORS.AllowedOrigins, "*") && len(c.CORS.AllowedOrigins) != 1 {
		errs = append(errs, errors.New("cors.allowed_origins wildcard must be the only origin"))
	}
	if len(c.CORS.AllowedMethods) == 0 {
		errs = append(errs, errors.New("cors.allowed_methods must not be empty"))
	}
	if len(c.CORS.AllowedHeaders) == 0 {
		errs = append(errs, errors.New("cors.allowed_headers must not be empty"))
	}
	if c.CORS.MaxAge < 0 {
		errs = append(errs, errors.New("cors.max_age must not be negative"))
	}
	if c.CORS.AllowCredentials {
		errs = append(errs, errors.New("cors.allow_credentials must be false for Bearer Token authentication"))
	}

	if !oneOf(c.Database.Driver, "postgres", "sqlite") {
		errs = append(errs, errors.New("database.driver must be one of postgres or sqlite; mysql is not supported"))
	}
	if strings.TrimSpace(c.Database.URL) == "" {
		errs = append(errs, errors.New("database.url is required"))
	}
	if c.Database.Driver == "sqlite" {
		if isSQLiteMemoryPath(c.Database.URL) {
			errs = append(errs, errors.New("database.url must be a persistent SQLite file path; in-memory SQLite is not supported"))
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.Database.URL)), "file:") {
			errs = append(errs, errors.New("database.url must be a local SQLite file path, not a URI"))
		}
		if strings.TrimSpace(c.Database.URL) != "" && filepath.Clean(strings.TrimSpace(c.Database.URL)) == "." {
			errs = append(errs, errors.New("database.url must identify a SQLite database file"))
		}
		if strings.TrimSpace(c.Database.Username) != "" {
			errs = append(errs, errors.New("database.username must be empty when database.driver is sqlite"))
		}
		if strings.TrimSpace(c.Database.Password) != "" {
			errs = append(errs, errors.New("database.password must be empty when database.driver is sqlite"))
		}
	} else {
		if strings.TrimSpace(c.Database.Username) == "" {
			errs = append(errs, errors.New("database.username is required"))
		}
		if strings.TrimSpace(c.Database.Password) == "" {
			errs = append(errs, errors.New("database.password is required"))
		}
	}
	if c.Database.MaxOpenConns <= 0 {
		errs = append(errs, errors.New("database.max_open_conns must be greater than zero"))
	}
	if c.Database.MaxIdleConns < 0 {
		errs = append(errs, errors.New("database.max_idle_conns must not be negative"))
	}
	if c.Database.MaxOpenConns > 0 && c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		errs = append(errs, errors.New("database.max_idle_conns must not exceed max_open_conns"))
	}
	if c.Database.ConnMaxLifetime < 0 {
		errs = append(errs, errors.New("database.conn_max_lifetime must not be negative"))
	}
	if c.Database.ConnMaxIdleTime < 0 {
		errs = append(errs, errors.New("database.conn_max_idle_time must not be negative"))
	}
	if !filepath.IsAbs(c.File.StorageRoot) {
		errs = append(errs, errors.New("file.storage_root must be an absolute path"))
	}

	if strings.TrimSpace(c.JWT.Secret) == "" {
		errs = append(errs, errors.New("jwt.secret is required"))
	}
	if strings.TrimSpace(c.JWT.Issuer) == "" {
		errs = append(errs, errors.New("jwt.issuer is required"))
	}
	if strings.TrimSpace(c.JWT.Audience) == "" {
		errs = append(errs, errors.New("jwt.audience is required"))
	}
	if c.JWT.TTL <= 0 {
		errs = append(errs, errors.New("jwt.ttl must be greater than zero"))
	}

	guard := c.Auth.LoginGuard
	if guard.IPMaxAttempts < 0 {
		errs = append(errs, errors.New("auth.login_guard.ip_max_attempts must not be negative"))
	}
	if guard.UsernameMaxAttempts < 0 {
		errs = append(errs, errors.New("auth.login_guard.username_max_attempts must not be negative"))
	}
	if guard.IPMaxAttempts > 0 && guard.IPWindow <= 0 {
		errs = append(errs, errors.New("auth.login_guard.ip_window must be greater than zero when IP protection is enabled"))
	}
	if guard.UsernameMaxAttempts > 0 && guard.UsernameWindow <= 0 {
		errs = append(errs, errors.New("auth.login_guard.username_window must be greater than zero when username protection is enabled"))
	}
	if guard.BackoffInitial <= 0 {
		errs = append(errs, errors.New("auth.login_guard.backoff_initial must be greater than zero"))
	}
	if guard.BackoffMax < guard.BackoffInitial {
		errs = append(errs, errors.New("auth.login_guard.backoff_max must not be less than backoff_initial"))
	}
	if guard.LockDuration <= 0 {
		errs = append(errs, errors.New("auth.login_guard.lock_duration must be greater than zero"))
	}
	if guard.MaxEntries <= 0 {
		errs = append(errs, errors.New("auth.login_guard.max_entries must be greater than zero"))
	}

	if !oneOf(c.Log.Level, "debug", "info", "warn", "error") {
		errs = append(errs, errors.New("log.level must be one of debug, info, warn, error"))
	}
	if !oneOf(c.Log.Format, "json", "text") {
		errs = append(errs, errors.New("log.format must be one of json, text"))
	}

	return errors.Join(errs...)
}

func validateAddress(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("http.address must be in host:port form: %w", err)
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("http.address port must be between 1 and 65535")
	}

	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}

	return false
}

func isSQLiteMemoryPath(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == ":memory:" || strings.HasPrefix(value, "file::memory:")
}
