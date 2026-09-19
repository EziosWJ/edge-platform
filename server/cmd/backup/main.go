// Command backup creates and optionally verifies an online SQLite backup.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	platformdatabase "github.com/EziosWJ/edge-platform/server/internal/platform/database"
)

const backupUsage = "usage: backup --source <sqlite-file> --destination <backup-file> [--verify]"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("SQLite backup failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	source := flags.String("source", "", "source SQLite database file")
	destination := flags.String("destination", "", "destination backup file")
	driver := flags.String("driver", platformdatabase.DriverSQLite, "database driver; only sqlite is supported")
	verify := flags.Bool("verify", false, "run an integrity check after creating the backup")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse backup flags: %w", err)
	}
	if flags.NArg() != 0 || *driver != platformdatabase.DriverSQLite || *source == "" || *destination == "" {
		return errors.New(backupUsage)
	}
	if err := platformdatabase.BackupSQLite(ctx, *source, *destination); err != nil {
		return err
	}
	if *verify {
		if err := platformdatabase.VerifySQLiteFile(ctx, *destination); err != nil {
			return fmt.Errorf("verify SQLite backup: %w", err)
		}
	}
	slog.Info("SQLite backup completed", "source", *source, "destination", *destination, "verified", *verify)
	return nil
}
