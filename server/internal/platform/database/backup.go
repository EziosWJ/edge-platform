package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/config"
	"github.com/mattn/go-sqlite3"
)

const (
	sqliteBackupStepPages = 128
	sqliteBackupTimeout   = 30 * time.Second
)

// BackupSQLite creates a consistent online backup of a file-backed SQLite
// database. It uses SQLite's online backup API through the official CGO
// driver, so no sqlite3 executable is required and the source may remain in
// service while the snapshot is made.
func BackupSQLite(ctx context.Context, sourcePath, destinationPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	sourcePath, destinationPath, err := validateBackupPaths(sourcePath, destinationPath)
	if err != nil {
		return err
	}

	source, err := openSQLiteFile(ctx, sourcePath)
	if err != nil {
		return fmt.Errorf("open SQLite backup source: %w", err)
	}
	defer func() { _ = source.Close() }()
	destination, err := openSQLiteFile(ctx, destinationPath)
	if err != nil {
		return fmt.Errorf("open SQLite backup destination: %w", err)
	}
	defer func() { _ = destination.Close() }()

	sourceConn, err := source.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite backup source connection: %w", err)
	}
	defer func() { _ = sourceConn.Close() }()
	destinationConn, err := destination.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite backup destination connection: %w", err)
	}
	defer func() { _ = destinationConn.Close() }()

	if err := destinationConn.Raw(func(destinationDriver any) error {
		destinationSQLite, ok := destinationDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return errors.New("SQLite backup destination is not using the mattn/go-sqlite3 driver")
		}
		return sourceConn.Raw(func(sourceDriver any) error {
			sourceSQLite, ok := sourceDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return errors.New("SQLite backup source is not using the mattn/go-sqlite3 driver")
			}
			return runOnlineBackup(ctx, destinationSQLite, sourceSQLite)
		})
	}); err != nil {
		return fmt.Errorf("run SQLite online backup: %w", err)
	}

	if err := os.Chmod(destinationPath, 0o600); err != nil {
		return fmt.Errorf("set SQLite backup permissions: %w", err)
	}
	return nil
}

// VerifySQLiteFile performs the low-level consistency check used by recovery
// tests and deployment smoke checks. Migration-version and business-data
// checks remain with the caller because they depend on the release contract.
func VerifySQLiteFile(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := validateExistingSQLitePath(path)
	if err != nil {
		return err
	}
	db, err := openSQLiteFile(ctx, path)
	if err != nil {
		return fmt.Errorf("open SQLite file for verification: %w", err)
	}
	defer func() { _ = db.Close() }()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("run SQLite integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("SQLite integrity check failed: %s", result)
	}
	return nil
}

func runOnlineBackup(ctx context.Context, destination, source *sqlite3.SQLiteConn) error {
	backup, err := destination.Backup("main", source, "main")
	if err != nil {
		return err
	}
	defer func() { _ = backup.Close() }()

	deadline := time.NewTimer(sqliteBackupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		done, err := backup.Step(sqliteBackupStepPages)
		if err != nil {
			return err
		}
		if done {
			return backup.Finish()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("SQLite backup exceeded the 30 second busy boundary")
		case <-ticker.C:
		}
	}
}

func openSQLiteFile(ctx context.Context, path string) (*sql.DB, error) {
	dsn, err := buildSQLiteDSN(configForSQLite(path))
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(sqlite3DriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := configureSQLiteConnection(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

const sqlite3DriverName = "sqlite3"

func configForSQLite(path string) config.DatabaseConfig {
	return config.DatabaseConfig{Driver: DriverSQLite, URL: path}
}

func validateBackupPaths(source, destination string) (string, string, error) {
	source, err := validateExistingSQLitePath(source)
	if err != nil {
		return "", "", fmt.Errorf("invalid SQLite source: %w", err)
	}
	destination = filepath.Clean(sourcePath(destination))
	if _, err := buildSQLiteDSN(configForSQLite(destination)); err != nil {
		return "", "", fmt.Errorf("invalid SQLite destination: %w", err)
	}
	destinationAbs, err := filepath.Abs(destination)
	if err != nil {
		return "", "", fmt.Errorf("resolve SQLite destination: %w", err)
	}
	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve SQLite source: %w", err)
	}
	if sourceAbs == destinationAbs {
		return "", "", errors.New("SQLite backup source and destination must be different files")
	}
	parent := filepath.Dir(destinationAbs)
	info, err := os.Stat(parent)
	if err != nil {
		return "", "", fmt.Errorf("stat SQLite backup destination directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", errors.New("SQLite backup destination parent is not a directory")
	}
	if info, err := os.Stat(destinationAbs); err == nil {
		if info.IsDir() {
			return "", "", errors.New("SQLite backup destination is a directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect SQLite backup destination: %w", err)
	}
	return sourceAbs, destinationAbs, nil
}

func validateExistingSQLitePath(path string) (string, error) {
	path = filepath.Clean(path)
	if _, err := buildSQLiteDSN(configForSQLite(path)); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("SQLite database path is not a regular file")
	}
	return path, nil
}

func sourcePath(path string) string {
	return filepath.Clean(path)
}
