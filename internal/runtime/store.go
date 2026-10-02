// Package runtime owns CodeBridge's durable runtime state: projects, sessions,
// runs, provider sessions, computer sessions, the event journal, artifacts,
// policy rules, grants and approvals. V2 makes codebridged the single durable
// authority for this state (docs/v2/architecture.md §5.4, §18).
//
// Phase 0 provides the store and journal. The RunManager lift, AgentProviders
// and the Computer broker arrive in later phases and build on this package.
package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the schema version this binary reads and writes. A store
// whose ledger is above it is refused before any write: migrations are
// forward-only and a downgrade is unsupported (architecture §5.6).
const SchemaVersion = 1

var (
	// ErrNotFound is returned by a getter for an unknown id.
	ErrNotFound = errors.New("runtime: not found")
	// ErrSchemaNewer reports a store written by a newer binary.
	ErrSchemaNewer = errors.New("runtime: store schema is newer than this binary")
	// ErrIllegalTransition reports a run status change the V1 state machine does
	// not allow; consumers map it to the invalid_state error category.
	ErrIllegalTransition = errors.New("runtime: illegal transition")
)

// Store is one open runtime database. It is safe for concurrent use: the
// underlying pool is capped at one connection and every transaction begins as
// BEGIN IMMEDIATE, so writers serialize instead of racing to upgrade a read
// transaction.
type Store struct {
	db   *sql.DB
	path string
	now  func() time.Time
}

// Open opens, creating or migrating as needed, the runtime store at path. An
// empty path is an error: only tests use a discarded store, and they use a
// temporary directory.
func Open(path string) (*Store, error) {
	return openStore(path, migrations)
}

func openStore(path string, migs []migration) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("runtime: store path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve store path: %w", err)
	}
	path = abs
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("runtime: create store directory: %w", err)
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("runtime: open store: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, now: time.Now}
	if err := store.prepare(migs); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("runtime: restrict store file: %w", err)
	}
	return store, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path is the absolute path of the open database file.
func (s *Store) Path() string { return s.path }

// prepare brings the database to SchemaVersion: it validates the ledger, backs
// up an existing store before a migration and applies pending migrations in one
// transaction.
func (s *Store) prepare(migs []migration) error {
	version, err := s.readVersion()
	if err != nil {
		return err
	}
	switch {
	case version > SchemaVersion:
		return fmt.Errorf("%w: store is v%d, this binary supports v%d", ErrSchemaNewer, version, SchemaVersion)
	case version == SchemaVersion:
		return nil
	}
	// A store with tables but no ledger is an older build's database; back it up
	// before touching it. A brand-new empty file has nothing to lose.
	if existing, err := s.hasTables(); err != nil {
		return err
	} else if existing {
		if _, err := s.backup(version); err != nil {
			return err
		}
	}
	return s.migrate(version, migs)
}

// Update runs fn in one transaction and commits it. Returning an error from fn
// rolls the whole transaction back, so a caller can write a state change and its
// journal event together and never leave one without the other.
//
// Update is the only way to write: the store exposes no standalone mutator, so
// every state change that has a lifecycle event must append that event
// (Tx.AppendEvent) in the same transaction (architecture §8).
func (s *Store) Update(fn func(*Tx) error) error {
	return s.UpdateContext(context.Background(), fn)
}

// UpdateContext is Update with a caller-provided context.
func (s *Store) UpdateContext(ctx context.Context, fn func(*Tx) error) error {
	if fn == nil {
		return errors.New("runtime: Update requires a function")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("runtime: begin transaction: %w", err)
	}
	t := &Tx{tx: tx}
	if err := fn(t); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("runtime: commit transaction: %w", err)
	}
	return nil
}

// querier is the read/write surface shared by the store and a transaction, so
// entity helpers work identically inside and outside an Update.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// dsn renders the connection string. A file: URI keeps paths such as
// "~/Library/Application Support/..." intact; the pragmas make the store
// durable (WAL), enforcing (foreign keys) and single-writer serialized
// (BEGIN IMMEDIATE) without every caller having to remember them.
//
// synchronous is FULL, not NORMAL: architecture §8 requires a terminal run
// transition and an approval decision to be durable before they are
// acknowledged, and NORMAL can lose the most recent commits when the OS or the
// machine fails.
func dsn(path string) string {
	u := &url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Set("_txlock", "immediate")
	for _, pragma := range []string{
		"busy_timeout(5000)",
		"foreign_keys(1)",
		"journal_mode(WAL)",
		"synchronous(FULL)",
	} {
		q.Add("_pragma", pragma)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
