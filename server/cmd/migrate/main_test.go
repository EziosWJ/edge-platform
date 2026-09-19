package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default schema", args: []string{"up"}, want: migrationKindSchema},
		{name: "seed", args: []string{"up", "--kind", migrationKindSeed}, want: migrationKindSeed},
		{name: "all", args: []string{"up", "--kind", migrationKindAll}, want: migrationKindAll},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseArguments(test.args)
			if err != nil {
				t.Fatalf("parseArguments() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseArguments() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHasSQLMigrations(t *testing.T) {
	directory := t.TempDir()
	if got, err := hasSQLMigrations(directory); err != nil || got {
		t.Fatalf("empty directory = (%t, %v), want (false, nil)", got, err)
	}
	if err := os.WriteFile(filepath.Join(directory, "00001_test.sql"), []byte("-- +goose Up\nSELECT 1;"), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}
	if got, err := hasSQLMigrations(directory); err != nil || !got {
		t.Fatalf("SQL migration directory = (%t, %v), want (true, nil)", got, err)
	}
}

func TestParseArgumentsRejectsImplicitOrUnknownCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"status"}, {"up", "--kind", "unknown"}, {"up", "extra"}} {
		_, err := parseArguments(args)
		if err == nil {
			t.Fatalf("parseArguments(%v) error = nil", args)
		}
	}
}

func TestMigrationDirectory(t *testing.T) {
	if got := migrationDirectory(migrationKindSchema); got != "migrations/schema" {
		t.Fatalf("schema dir = %q", got)
	}
	if got := migrationDirectory(migrationKindSeed); !strings.HasSuffix(got, "migrations/seed") {
		t.Fatalf("seed dir = %q", got)
	}
}

func TestMigrationKindsAndVersionTables(t *testing.T) {
	if got := migrationKinds(migrationKindAll); len(got) != 2 || got[0] != migrationKindSchema || got[1] != migrationKindSeed {
		t.Fatalf("all migration order = %v, want schema then seed", got)
	}
	if got := migrationTableName(migrationKindSchema); got != "goose_schema_db_version" {
		t.Fatalf("schema version table = %q", got)
	}
	if got := migrationTableName(migrationKindSeed); got != "goose_seed_db_version" {
		t.Fatalf("seed version table = %q", got)
	}
}
func TestLogClearEnabledDefaultByEnvironment(t *testing.T) {
	for _, test := range []struct {
		environment string
		want        string
	}{
		{environment: "dev", want: "true"},
		{environment: "test", want: "false"},
		{environment: "prod", want: "false"},
	} {
		t.Run(test.environment, func(t *testing.T) {
			if got := logClearEnabledDefault(test.environment); got != test.want {
				t.Fatalf("logClearEnabledDefault(%q) = %q, want %q", test.environment, got, test.want)
			}
		})
	}
}

func TestShouldApplyLogClearDefaultOnlyForCorrectiveMigration(t *testing.T) {
	for _, test := range []struct {
		name     string
		previous int64
		current  int64
		want     bool
	}{
		{name: "before corrective migration", previous: 2, current: 2, want: false},
		{name: "corrective migration just applied", previous: 2, current: 3, want: true},
		{name: "new database", previous: 0, current: 3, want: true},
		{name: "already normalized", previous: 3, current: 3, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldApplyLogClearDefault(test.previous, test.current); got != test.want {
				t.Fatalf("shouldApplyLogClearDefault(%d, %d) = %t, want %t", test.previous, test.current, got, test.want)
			}
		})
	}
}
