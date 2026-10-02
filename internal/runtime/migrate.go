package runtime

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	storev1 "github.com/edynasty/codebridge/schema/store/v1"
)

// migration is one forward-only step. apply receives a transaction already
// containing the ledger; the runner records the version after apply returns.
type migration struct {
	version int
	apply   func(*sql.Tx) error
}

// ledgerTable holds one row per applied schema version; its highest version is
// the store's current schema version.
const ledgerTable = "schema_version"

// migrations is the ordered migration list. It ends at SchemaVersion and gains
// one entry per schema version; there is no down migration.
var migrations = []migration{
	{version: 1, apply: schemaV1},
}

func schemaV1(tx *sql.Tx) error {
	_, err := tx.Exec(storev1.SQL())
	return err
}

// readVersion returns the ledger version, or 0 when the store has no ledger yet
// (a new file, or a database created by an older build).
func (s *Store) readVersion() (int, error) {
	var ledger int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, ledgerTable,
	).Scan(&ledger); err != nil {
		return 0, fmt.Errorf("runtime: inspect store: %w", err)
	}
	if ledger == 0 {
		return 0, nil
	}
	var version int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM ` + ledgerTable).Scan(&version); err != nil {
		return 0, fmt.Errorf("runtime: read schema version: %w", err)
	}
	return version, nil
}

// hasTables reports whether the database carries any user table, which
// distinguishes a fresh empty file from a store that needs a backup.
func (s *Store) hasTables() (bool, error) {
	var tables int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> ?`,
		ledgerTable,
	).Scan(&tables); err != nil {
		return false, fmt.Errorf("runtime: inspect store tables: %w", err)
	}
	return tables > 0, nil
}

// backup writes a consistent snapshot next to the store, before a migration
// touches it, and returns the snapshot path. VACUUM INTO is SQLite's own copy:
// it includes committed WAL content and cannot capture a torn page, unlike
// copying runtime.db / -wal / -shm while a writer is active.
func (s *Store) backup(fromVersion int) (string, error) {
	dst := fmt.Sprintf("%s.backup-v%d-%s", s.path, fromVersion, s.now().UTC().Format("20060102T150405.000000000Z"))
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("runtime: backup target already exists: %s", dst)
	}
	if _, err := s.db.Exec(`VACUUM INTO '` + strings.ReplaceAll(dst, "'", "''") + `'`); err != nil {
		return "", fmt.Errorf("runtime: back up store: %w", err)
	}
	if err := os.Chmod(dst, 0o600); err != nil {
		return "", fmt.Errorf("runtime: restrict backup file: %w", err)
	}
	return dst, nil
}

// migrate applies every migration above from in one transaction, so a failed
// step leaves the store exactly as it was.
func (s *Store) migrate(from int, migs []migration) error {
	steps := pendingMigrations(from, migs)
	if len(steps) == 0 {
		return fmt.Errorf("runtime: no migration path from v%d to v%d", from, SchemaVersion)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("runtime: begin migration: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS ` + ledgerTable + ` (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("runtime: create migration ledger: %w", err)
	}
	for _, m := range steps {
		if err := m.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("runtime: migrate to v%d: %w", m.version, err)
		}
		if _, err := tx.Exec(`INSERT INTO `+ledgerTable+` (version, applied_at) VALUES (?, ?)`,
			m.version, s.now().UnixMilli()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("runtime: record migration v%d: %w", m.version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("runtime: commit migration: %w", err)
	}
	return nil
}

// pendingMigrations returns the contiguous steps above from up to
// SchemaVersion, or nil when the list does not cover the path.
func pendingMigrations(from int, migs []migration) []migration {
	var steps []migration
	want := from + 1
	for _, m := range migs {
		if m.version < want {
			continue
		}
		if m.version != want {
			return nil
		}
		steps = append(steps, m)
		want++
	}
	if len(steps) == 0 || steps[len(steps)-1].version != SchemaVersion {
		return nil
	}
	return steps
}
