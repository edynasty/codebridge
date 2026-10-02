package runtime

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	storev1 "github.com/edynasty/codebridge/schema/store/v1"
)

// writeLegacyStore creates a database in the shape an older build would have
// left behind: tables but no migration ledger.
func writeLegacyStore(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("open legacy store: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE legacy_marker (msg TEXT NOT NULL)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO legacy_marker (msg) VALUES ('pre-migration')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
}

// rawCount opens path outside the store and counts rows of one query.
func rawCount(t *testing.T, path, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("open raw store %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %s on %s: %v", query, path, err)
	}
	return n
}

func backupsOf(path string) []string {
	matches, err := filepath.Glob(path + ".backup-*")
	if err != nil {
		panic(err)
	}
	return matches
}

func TestMigrateBacksUpBeforeMigrating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	writeLegacyStore(t, path)

	// A synthetic v1 step that leaves a canary row after the real DDL, so the
	// backup's contents prove whether it was taken before or after the
	// migration wrote.
	migs := []migration{{version: 1, apply: func(tx *sql.Tx) error {
		if _, err := tx.Exec(storev1.SQL()); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO legacy_marker (msg) VALUES ('post-migration')`)
		return err
	}}}

	s, err := openStore(path, migs)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	defer func() { _ = s.Close() }()

	version, err := s.readVersion()
	if err != nil {
		t.Fatalf("readVersion: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("version after migration: want %d, got %d", SchemaVersion, version)
	}
	backups := backupsOf(path)
	if len(backups) != 1 {
		t.Fatalf("backups: want 1, got %v", backups)
	}
	// The snapshot is a consistent SQLite database holding the pre-migration
	// state: the legacy row is present, the post-migration canary is not.
	if got := rawCount(t, backups[0], `SELECT COUNT(*) FROM legacy_marker WHERE msg = 'pre-migration'`); got != 1 {
		t.Fatalf("backup legacy row: want 1, got %d", got)
	}
	if got := rawCount(t, backups[0], `SELECT COUNT(*) FROM legacy_marker WHERE msg = 'post-migration'`); got != 0 {
		t.Fatalf("backup contains post-migration rows: %d", got)
	}
	// The migrated store is usable.
	mustWrite(t, s, func(tx *Tx) error {
		return tx.PutProject(Project{ID: "prj_migrated", Name: "migrated", CreatedAt: testNow, UpdatedAt: testNow})
	})
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("open raw store: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE ` + ledgerTable + ` (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO `+ledgerTable+` (version, applied_at) VALUES (?, 0)`, SchemaVersion+1); err != nil {
		t.Fatalf("write newer version: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE marker (msg TEXT NOT NULL)`); err != nil {
		t.Fatalf("create marker: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO marker (msg) VALUES ('untouched')`); err != nil {
		t.Fatalf("insert marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw store: %v", err)
	}

	if _, err := Open(path); !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("Open of a newer store: want ErrSchemaNewer, got %v", err)
	}
	// The refusal must not have written anything: no v1 tables, no backup.
	if got := rawCount(t, path, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'projects'`); got != 0 {
		t.Fatalf("store was modified: %d projects tables", got)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM marker`); got != 1 {
		t.Fatalf("marker rows: want 1, got %d", got)
	}
	if backups := backupsOf(path); len(backups) != 0 {
		t.Fatalf("backups for a refused open: %v", backups)
	}
}

func TestMigrateRollsBackOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	writeLegacyStore(t, path)

	migs := []migration{{version: 1, apply: func(tx *sql.Tx) error {
		if _, err := tx.Exec(storev1.SQL()); err != nil {
			return err
		}
		return errors.New("migration step failed")
	}}}

	if _, err := openStore(path, migs); err == nil || !strings.Contains(err.Error(), "migration step failed") {
		t.Fatalf("openStore error: want the step failure, got %v", err)
	}
	// The failed migration left no schema and no ledger entry.
	if got := rawCount(t, path, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'projects'`); got != 0 {
		t.Fatalf("projects table after a failed migration: %d", got)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'schema_version'`); got != 0 {
		t.Fatalf("ledger after a failed migration: %d", got)
	}
	// The backup was still taken before the attempt.
	if backups := backupsOf(path); len(backups) != 1 {
		t.Fatalf("backups: want 1, got %v", backups)
	}
}

func TestMigrateWithoutPathFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	writeLegacyStore(t, path)

	if _, err := openStore(path, nil); err == nil || !strings.Contains(err.Error(), "no migration path") {
		t.Fatalf("openStore without migrations: want a no-path error, got %v", err)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM legacy_marker`); got != 1 {
		t.Fatalf("legacy store was modified: %d rows", got)
	}
}
