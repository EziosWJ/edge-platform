// Command migrate applies one explicit Goose migration kind.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/pressly/goose/v3"

	"github.com/EziosWJ/edge-platform/server/internal/config"
	"github.com/EziosWJ/edge-platform/server/internal/platform/database"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
)

const (
	migrationKindSchema = "schema"
	migrationKindSeed   = "seed"
	migrationKindAll    = "all"

	logClearDefaultMigrationVersion int64 = 3
)

// Goose's legacy package API keeps dialect and version-table configuration as
// process-global state. CLI invocations are separate processes; this mutex also
// makes the command deterministic when run() is exercised in-process by tests.
var gooseMu sync.Mutex

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	kind, err := parseArguments(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("close database pool", "error", closeErr)
		}
	}()

	for _, migrationKind := range migrationKinds(kind) {
		previousVersion, currentVersion, err := applyMigrationsForDriver(ctx, db.SQL, cfg.Database.Driver, migrationKind)
		if err != nil {
			return err
		}
		if migrationKind == migrationKindSeed && shouldApplyLogClearDefault(previousVersion, currentVersion) {
			if err := applyLogClearDefault(ctx, db.SQL, cfg.Database.Driver, cfg.Environment); err != nil {
				return err
			}
		}
	}

	slog.Info("migrations applied", "kind", kind)
	return nil
}

func applyMigrations(ctx context.Context, sqlDB *sql.DB, kind string) (int64, int64, error) {
	return applyMigrationsForDriver(ctx, sqlDB, database.DriverPostgres, kind)
}

func applyMigrationsForDriver(ctx context.Context, sqlDB *sql.DB, driver, kind string) (int64, int64, error) {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	directory := migrationDirectoryForDriver(driver, kind)
	hasMigrations, err := hasSQLMigrations(directory)
	if err != nil {
		return 0, 0, err
	}
	if !hasMigrations {
		slog.Info("no migrations to apply", "kind", kind, "directory", directory)
		return 0, 0, nil
	}

	gooseDialect, err := database.GooseDialect(driver)
	if err != nil {
		return 0, 0, err
	}
	if err := goose.SetDialect(gooseDialect); err != nil {
		return 0, 0, fmt.Errorf("set Goose dialect: %w", err)
	}
	goose.SetTableName(migrationTableName(kind))
	previousVersion, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s migration version: %w", kind, err)
	}
	if err := goose.UpContext(ctx, sqlDB, directory); err != nil {
		return 0, 0, fmt.Errorf("apply %s migrations: %w", kind, err)
	}
	currentVersion, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s migration version after apply: %w", kind, err)
	}
	return previousVersion, currentVersion, nil
}

func shouldApplyLogClearDefault(previousVersion, currentVersion int64) bool {
	return previousVersion < logClearDefaultMigrationVersion && currentVersion >= logClearDefaultMigrationVersion
}

func logClearEnabledDefault(environment string) string {
	if environment == config.EnvironmentDev {
		return "true"
	}
	return "false"
}

func applyLogClearDefault(ctx context.Context, sqlDB *sql.DB, driver, environment string) error {
	query, err := database.Rebind(driver, `
		UPDATE sys_config
		SET config_value = ?,
		    update_time = CURRENT_TIMESTAMP
		WHERE config_key = ?
		  AND is_builtin = 1
		  AND deleted = 0`)
	if err != nil {
		return fmt.Errorf("prepare log-clear default for %s: %w", driver, err)
	}
	_, err = sqlDB.ExecContext(ctx, query, logClearEnabledDefault(environment), sysconfig.LogClearEnabledKey)
	if err != nil {
		return fmt.Errorf("apply log-clear default for %s: %w", environment, err)
	}
	return nil
}

func hasSQLMigrations(directory string) (bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, fmt.Errorf("read migration directory %s: %w", directory, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			return true, nil
		}
	}
	return false, nil
}

func parseArguments(args []string) (string, error) {
	if len(args) == 0 || args[0] != "up" {
		return "", errors.New("usage: migrate up [--kind schema|seed|all]")
	}

	flags := flag.NewFlagSet("migrate up", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	kind := flags.String("kind", migrationKindSchema, "migration kind: schema, seed, or all")
	if err := flags.Parse(args[1:]); err != nil {
		return "", fmt.Errorf("parse migration flags: %w", err)
	}
	if flags.NArg() != 0 {
		return "", errors.New("usage: migrate up [--kind schema|seed|all]")
	}
	if *kind != migrationKindSchema && *kind != migrationKindSeed && *kind != migrationKindAll {
		return "", fmt.Errorf("migration kind must be %q, %q, or %q", migrationKindSchema, migrationKindSeed, migrationKindAll)
	}
	return *kind, nil
}

func migrationDirectory(kind string) string {
	return filepath.Join("migrations", kind)
}

func migrationDirectoryForDriver(driver, kind string) string {
	if driver == database.DriverSQLite {
		return filepath.Join("migrations", "sqlite", kind)
	}
	return migrationDirectory(kind)
}

func migrationTableName(kind string) string {
	return "goose_" + kind + "_db_version"
}

func migrationKinds(kind string) []string {
	if kind == migrationKindAll {
		return []string{migrationKindSchema, migrationKindSeed}
	}
	return []string{kind}
}
