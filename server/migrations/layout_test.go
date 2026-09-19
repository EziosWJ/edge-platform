package migrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var (
	gooseUpMarker   = regexp.MustCompile(`(?m)^-- \+goose Up\s*$`)
	gooseDownMarker = regexp.MustCompile(`(?m)^-- \+goose Down\s*$`)
	schemaDML       = regexp.MustCompile(`(?im)^\s*(INSERT|UPDATE|DELETE|MERGE)\b`)
	seedDDL         = regexp.MustCompile(`(?im)^\s*(CREATE|ALTER|DROP|TRUNCATE)\b`)
	bcryptHash      = regexp.MustCompile(`\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}`)
)

func TestMigrationStreamsHaveValidGooseFiles(t *testing.T) {
	for _, dialect := range []string{"postgres", "sqlite"} {
		schemaFiles := migrationFiles(t, dialectPath(dialect, "schema"))
		if len(schemaFiles) == 0 {
			t.Fatalf("%s schema migration stream must contain a baseline", dialect)
		}
		assertGooseFiles(t, schemaFiles)
		assertGooseFiles(t, migrationFiles(t, dialectPath(dialect, "seed")))
	}
}

func TestAdminSeedPasswordMatchesJavaContract(t *testing.T) {
	for _, dialect := range []string{"postgres", "sqlite"} {
		contents := readFile(t, filepath.Join(dialectPath(dialect, "seed"), "00001_auth_seed.sql"))
		hash := bcryptHash.Find(contents)
		if hash == nil {
			t.Fatalf("%s authentication seed must contain a BCrypt password hash", dialect)
		}
		if err := bcrypt.CompareHashAndPassword(hash, []byte("admin123")); err != nil {
			t.Fatalf("%s administrator seed password does not match admin123: %v", dialect, err)
		}
	}
}

func TestSchemaAndSeedResponsibilitiesStaySeparate(t *testing.T) {
	for _, dialect := range []string{"postgres", "sqlite"} {
		for _, name := range migrationFiles(t, dialectPath(dialect, "schema")) {
			contents := readFile(t, name)
			if schemaDML.Match(contents) {
				t.Errorf("schema migration %s contains seed-data DML", name)
			}
		}

		for _, name := range migrationFiles(t, dialectPath(dialect, "seed")) {
			contents := readFile(t, name)
			if seedDDL.Match(contents) {
				t.Errorf("seed migration %s contains schema DDL", name)
			}
		}
	}
}

func TestDialectMigrationVersionsStayInLockstep(t *testing.T) {
	for _, kind := range []string{"schema", "seed"} {
		postgres := migrationVersions(t, dialectPath("postgres", kind))
		sqlite := migrationVersions(t, dialectPath("sqlite", kind))
		if len(postgres) != len(sqlite) {
			t.Fatalf("%s migration versions differ: postgres=%v sqlite=%v", kind, postgres, sqlite)
		}
		for version := range postgres {
			if !sqlite[version] {
				t.Errorf("SQLite %s stream is missing logical migration version %05d", kind, version)
			}
		}
		for version := range sqlite {
			if !postgres[version] {
				t.Errorf("PostgreSQL %s stream is missing logical migration version %05d", kind, version)
			}
		}
	}
}

func assertGooseFiles(t *testing.T, files []string) {
	t.Helper()
	for _, name := range files {
		contents := readFile(t, name)
		up := gooseUpMarker.FindIndex(contents)
		down := gooseDownMarker.FindIndex(contents)
		if up == nil || down == nil {
			t.Errorf("migration %s must contain exact Goose Up and Down markers", name)
			continue
		}
		if up[0] >= down[0] {
			t.Errorf("migration %s places Goose Down before Goose Up", name)
		}
	}
}

func migrationFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migration directory %s: %v", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files
}

func migrationVersions(t *testing.T, dir string) map[int]bool {
	t.Helper()
	versions := make(map[int]bool)
	for _, name := range migrationFiles(t, dir) {
		base := filepath.Base(name)
		var version int
		if _, err := fmt.Sscanf(base, "%05d_", &version); err != nil {
			t.Fatalf("parse migration version %s: %v", name, err)
		}
		versions[version] = true
	}
	return versions
}

func dialectPath(dialect, kind string) string {
	if dialect == "postgres" {
		return kind
	}
	return filepath.Join(dialect, kind)
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return contents
}
