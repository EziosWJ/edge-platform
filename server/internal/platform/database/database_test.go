package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/edge-platform/server/internal/config"
	platformerrors "github.com/EziosWJ/edge-platform/server/internal/platform/errors"
	"github.com/mattn/go-sqlite3"
)

func TestNewDialectorUsesPostgresWithoutConnecting(t *testing.T) {
	dialector, err := newDialector(config.DatabaseConfig{
		Driver:   DriverPostgres,
		URL:      "postgres://127.0.0.1:1/unused?sslmode=disable",
		Username: "unused",
		Password: "unused",
	})
	if err != nil {
		t.Fatalf("newDialector() error = %v", err)
	}
	if got := dialector.Name(); got != DriverPostgres {
		t.Fatalf("dialector name = %q, want %q", got, DriverPostgres)
	}
}

func TestNewDialectorRejectsUnsupportedDriverAndIncompleteConfiguration(t *testing.T) {
	_, err := newDialector(config.DatabaseConfig{Driver: "mysql"})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsupported driver error = %v", err)
	}

	_, err = newDialector(config.DatabaseConfig{Driver: DriverPostgres})
	if err == nil || !strings.Contains(err.Error(), "database.url") {
		t.Fatalf("empty database URL error = %v", err)
	}
}

func TestBuildDSNCombinesURLAndCredentials(t *testing.T) {
	dsn, err := buildDSN(config.DatabaseConfig{
		URL:      "postgres://127.0.0.1:5432/edge_platform?sslmode=disable",
		Username: "api-user",
		Password: "p@ss word",
	})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}
	if want := "postgres://api-user:p%40ss%20word@127.0.0.1:5432/edge_platform?sslmode=disable"; dsn != want {
		t.Errorf("buildDSN() = %q, want %q", dsn, want)
	}
}

func TestBuildDSNRejectsCredentialsInURL(t *testing.T) {
	_, err := buildDSN(config.DatabaseConfig{
		URL:      "postgres://api-user:password@127.0.0.1:5432/edge_platform",
		Username: "api-user",
		Password: "password",
	})
	if err == nil || !strings.Contains(err.Error(), "must not include username or password") {
		t.Fatalf("buildDSN() error = %v", err)
	}
}

func TestBuildSQLiteDSNUsesPersistentFileAndSafetyOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "app.db")
	dsn, err := buildSQLiteDSN(config.DatabaseConfig{Driver: DriverSQLite, URL: path})
	if err != nil {
		t.Fatalf("buildSQLiteDSN() error = %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse SQLite DSN: %v", err)
	}
	if parsed.Scheme != "file" || parsed.Path != path {
		t.Fatalf("SQLite DSN = %q, want file path %q", dsn, path)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"_foreign_keys": "on",
		"_journal_mode": "WAL",
		"_busy_timeout": "5000",
		"_synchronous":  "NORMAL",
		"_loc":          "UTC",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("SQLite DSN option %s = %q, want %q", key, got, want)
		}
	}
}

func TestBuildSQLiteDSNRejectsMemoryURIAndCredentials(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.DatabaseConfig
		want string
	}{
		{name: "memory", cfg: config.DatabaseConfig{Driver: DriverSQLite, URL: ":memory:"}, want: "in-memory"},
		{name: "memory URI", cfg: config.DatabaseConfig{Driver: DriverSQLite, URL: "file::memory:?cache=shared"}, want: "in-memory"},
		{name: "URI", cfg: config.DatabaseConfig{Driver: DriverSQLite, URL: "file:/tmp/app.db"}, want: "not a URI"},
		{name: "credentials", cfg: config.DatabaseConfig{Driver: DriverSQLite, URL: "app.db", Username: "user"}, want: "must be empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildSQLiteDSN(test.cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("buildSQLiteDSN() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestOpenSQLiteConfiguresPragmasAndPersistsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	database, err := Open(context.Background(), config.DatabaseConfig{Driver: DriverSQLite, URL: path})
	if err != nil {
		t.Fatalf("Open(SQLite) error = %v", err)
	}
	if _, err := database.SQL.Exec("CREATE TABLE persistence_check (value TEXT NOT NULL)"); err != nil {
		_ = database.Close()
		t.Fatalf("create SQLite table: %v", err)
	}
	if _, err := database.SQL.Exec("INSERT INTO persistence_check (value) VALUES (?)", "persisted"); err != nil {
		_ = database.Close()
		t.Fatalf("insert SQLite row: %v", err)
	}
	for pragma, want := range map[string]string{
		"foreign_keys": "1",
		"journal_mode": "wal",
		"busy_timeout": "5000",
		"synchronous":  "1",
	} {
		var got string
		if err := database.SQL.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			_ = database.Close()
			t.Fatalf("query SQLite pragma %s: %v", pragma, err)
		}
		if got != want {
			t.Errorf("SQLite pragma %s = %q, want %q", pragma, got, want)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close SQLite database: %v", err)
	}

	reopened, err := Open(context.Background(), config.DatabaseConfig{Driver: DriverSQLite, URL: path})
	if err != nil {
		t.Fatalf("reopen SQLite database: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	var value string
	if err := reopened.SQL.QueryRow("SELECT value FROM persistence_check").Scan(&value); err != nil {
		t.Fatalf("read persisted SQLite row: %v", err)
	}
	if value != "persisted" {
		t.Fatalf("persisted SQLite value = %q, want persisted", value)
	}
}

func TestOpenSQLiteSupportsRelativeFilePath(t *testing.T) {
	t.Chdir(t.TempDir())

	database, err := Open(context.Background(), config.DatabaseConfig{
		Driver: DriverSQLite,
		URL:    ".data/app.db",
	})
	if err != nil {
		t.Fatalf("Open(SQLite relative path) error = %v", err)
	}
	defer func() { _ = database.Close() }()

	if _, err := database.SQL.Exec("CREATE TABLE relative_path_check (value TEXT NOT NULL)"); err != nil {
		t.Fatalf("create SQLite table: %v", err)
	}
}

func TestNormalizeErrorHidesSQLiteBusyAndLocked(t *testing.T) {
	for _, code := range []sqlite3.ErrNo{sqlite3.ErrBusy, sqlite3.ErrLocked} {
		t.Run(code.Error(), func(t *testing.T) {
			got := NormalizeError(fmt.Errorf("wrapped: %w", sqlite3.Error{Code: code}))
			if !errors.Is(got, platformerrors.ErrTemporarilyUnavailable) {
				t.Fatalf("NormalizeError() = %v, want temporary-unavailable sentinel", got)
			}
		})
	}
}

func TestDatabaseDialectHelpers(t *testing.T) {
	if got, err := GooseDialect(DriverPostgres); err != nil || got != "postgres" {
		t.Fatalf("GooseDialect(postgres) = (%q, %v)", got, err)
	}
	if got, err := GooseDialect(DriverSQLite); err != nil || got != "sqlite3" {
		t.Fatalf("GooseDialect(sqlite) = (%q, %v)", got, err)
	}
	if got, err := Rebind(DriverPostgres, "UPDATE table_name SET a = ?, b = ?"); err != nil || got != "UPDATE table_name SET a = $1, b = $2" {
		t.Fatalf("Rebind(postgres) = (%q, %v)", got, err)
	}
	if got, err := Rebind(DriverSQLite, "UPDATE table_name SET a = ?"); err != nil || got != "UPDATE table_name SET a = ?" {
		t.Fatalf("Rebind(sqlite) = (%q, %v)", got, err)
	}
}

func TestReadyRejectsUninitializedPoolWithoutConnecting(t *testing.T) {
	var database Database
	err := database.Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Ready() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
