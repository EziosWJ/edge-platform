// Package database owns long-lived database infrastructure.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	platformerrors "github.com/EziosWJ/edge-platform/server/internal/platform/errors"
	"github.com/mattn/go-sqlite3"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/EziosWJ/edge-platform/server/internal/config"
)

const (
	DriverPostgres = "postgres"
	DriverSQLite   = "sqlite"

	sqliteBusyTimeout = 5 * time.Second
)

// Database contains the single process-wide GORM handle and the database/sql
// connection pool underneath it. Neither API startup nor this package executes
// schema migrations or GORM AutoMigrate.
type Database struct {
	GORM *gorm.DB
	SQL  *sql.DB
}

// Open creates the selected GORM handle, configures its underlying pool, and
// verifies the connection before returning it. The caller remains responsible
// for running explicit schema and seed migrations before starting the API.
func Open(ctx context.Context, cfg config.DatabaseConfig) (*Database, error) {
	dialector, err := newDialector(cfg)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Driver == DriverSQLite {
		if err := ensureSQLiteParent(cfg.URL); err != nil {
			return nil, err
		}
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open GORM database: %w", err)
	}
	if cfg.Driver == DriverSQLite {
		if err := registerSQLiteErrorNormalizer(gormDB); err != nil {
			if sqlDB, dbErr := gormDB.DB(); dbErr == nil {
				_ = sqlDB.Close()
			}
			return nil, fmt.Errorf("configure SQLite error handling: %w", err)
		}
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get database/sql pool: %w", err)
	}
	configurePool(sqlDB, cfg)
	if cfg.Driver == DriverSQLite {
		if err := configureSQLiteConnection(ctx, sqlDB); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{GORM: gormDB, SQL: sqlDB}, nil
}

// Ready satisfies the HTTP readiness-check contract without exposing a driver
// concern to Handlers or Services.
func (d *Database) Ready(ctx context.Context) error {
	if d == nil || d.SQL == nil {
		return errors.New("database pool is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := d.SQL.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func (d *Database) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

func newDialector(cfg config.DatabaseConfig) (gorm.Dialector, error) {
	switch cfg.Driver {
	case DriverPostgres:
		dsn, err := buildDSN(cfg)
		if err != nil {
			return nil, err
		}
		return postgres.Open(dsn), nil
	case DriverSQLite:
		dsn, err := buildSQLiteDSN(cfg)
		if err != nil {
			return nil, err
		}
		return sqlite.Open(dsn), nil
	default:
		return nil, fmt.Errorf("database driver %q is not supported; supported drivers are postgres and sqlite", cfg.Driver)
	}
}

func buildSQLiteDSN(cfg config.DatabaseConfig) (string, error) {
	path := strings.TrimSpace(cfg.URL)
	if path == "" {
		return "", errors.New("database.url is required")
	}
	if isSQLiteMemoryPath(path) {
		return "", errors.New("database.url must be a persistent SQLite file path; in-memory SQLite is not supported")
	}
	if strings.TrimSpace(cfg.Username) != "" || strings.TrimSpace(cfg.Password) != "" {
		return "", errors.New("database.username and database.password must be empty when database.driver is sqlite")
	}
	if strings.HasPrefix(strings.ToLower(path), "file:") {
		return "", errors.New("database.url must be a local SQLite file path, not a URI")
	}
	if filepath.Clean(path) == "." {
		return "", errors.New("database.url must identify a SQLite database file")
	}
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve SQLite database path: %w", err)
	}

	// mattn/go-sqlite3 applies these URI options whenever it opens a new
	// connection. That matters because foreign_keys, busy_timeout and
	// synchronous are connection-local pragmas, while WAL is persisted by the
	// database. _loc=UTC keeps scanned business timestamps as UTC values.
	endpoint := url.URL{Scheme: "file", Path: absolutePath}
	query := endpoint.Query()
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "WAL")
	query.Set("_busy_timeout", strconv.FormatInt(sqliteBusyTimeout.Milliseconds(), 10))
	query.Set("_synchronous", "NORMAL")
	query.Set("_loc", "UTC")
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func configureSQLiteConnection(ctx context.Context, sqlDB *sql.DB) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := sqlDB.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure SQLite %s: %w", statement, err)
		}
	}
	return nil
}

func ensureSQLiteParent(path string) error {
	parent := filepath.Dir(filepath.Clean(strings.TrimSpace(path)))
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create SQLite database directory: %w", err)
	}
	return nil
}

func buildDSN(cfg config.DatabaseConfig) (string, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return "", errors.New("database.url is required")
	}

	endpoint, err := url.Parse(cfg.URL)
	if err != nil {
		return "", fmt.Errorf("parse database.url: %w", err)
	}
	if endpoint.Scheme != "postgres" && endpoint.Scheme != "postgresql" {
		return "", errors.New("database.url must use postgres or postgresql scheme")
	}
	if endpoint.Host == "" {
		return "", errors.New("database.url must include host and port")
	}
	if strings.Trim(endpoint.Path, "/") == "" {
		return "", errors.New("database.url must include database name")
	}
	if endpoint.User != nil {
		return "", errors.New("database.url must not include username or password")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return "", errors.New("database.username is required")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return "", errors.New("database.password is required")
	}

	endpoint.User = url.UserPassword(cfg.Username, cfg.Password)
	return endpoint.String(), nil
}

func configurePool(sqlDB *sql.DB, cfg config.DatabaseConfig) {
	if cfg.Driver == DriverSQLite {
		// One API process has one SQLite writer. A single pooled connection keeps
		// normal request traffic from manufacturing avoidable writer contention;
		// separate handles remain available for backup and integration tests.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
}

// GooseDialect returns the migration dialect name for a configured driver.
// The mapping stays in the database infrastructure so migration commands do
// not need to duplicate driver-specific knowledge.
func GooseDialect(driver string) (string, error) {
	switch driver {
	case DriverPostgres:
		return "postgres", nil
	case DriverSQLite:
		return "sqlite3", nil
	default:
		return "", fmt.Errorf("database driver %q is not supported; supported drivers are postgres and sqlite", driver)
	}
}

// Rebind changes the portable '?' placeholders used by migration-side SQL to
// the PostgreSQL placeholders expected by pgx. SQLite keeps '?' unchanged.
func Rebind(driver, query string) (string, error) {
	if driver == DriverSQLite {
		return query, nil
	}
	if driver != DriverPostgres {
		return "", fmt.Errorf("database driver %q is not supported", driver)
	}
	var builder strings.Builder
	parameter := 0
	for _, character := range query {
		if character == '?' {
			parameter++
			builder.WriteByte('$')
			builder.WriteString(strconv.Itoa(parameter))
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String(), nil
}

// NormalizeError hides SQLite's busy/locked implementation errors behind the
// transport-independent temporary-unavailability contract. Other errors are
// returned unchanged so callers can preserve their existing domain mapping.
func NormalizeError(err error) error {
	if err == nil {
		return nil
	}
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked) {
		return platformerrors.ErrTemporarilyUnavailable
	}
	return err
}

func registerSQLiteErrorNormalizer(db *gorm.DB) error {
	normalize := func(db *gorm.DB) {
		db.Error = NormalizeError(db.Error)
	}
	registrations := []struct {
		register func(string, func(*gorm.DB)) error
		anchor   string
	}{
		{register: db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register, anchor: "create"},
		{register: db.Callback().Query().After("gorm:query").Register, anchor: "query"},
		{register: db.Callback().Update().After("gorm:commit_or_rollback_transaction").Register, anchor: "update"},
		{register: db.Callback().Delete().After("gorm:commit_or_rollback_transaction").Register, anchor: "delete"},
		{register: db.Callback().Row().After("gorm:row").Register, anchor: "row"},
		{register: db.Callback().Raw().After("gorm:raw").Register, anchor: "raw"},
	}
	for _, registration := range registrations {
		if err := registration.register("platform:normalize_sqlite_error_"+registration.anchor, normalize); err != nil {
			return err
		}
	}
	return nil
}

func isSQLiteMemoryPath(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == ":memory:" || strings.HasPrefix(value, "file::memory:")
}
