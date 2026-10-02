package runtime

import (
	"database/sql"
	"errors"
	"fmt"
)

// This file holds the entity putters and getters plus their Tx and Store
// wrappers. The querier-level functions are the implementation; a Tx wraps a
// transaction so a caller can compose several writes (and their journal events)
// into one commit, and a Store wrapper is the same write in its own
// transaction.

// orderedText reads one text column of an ordered child table.
func orderedText(q querier, query string, args ...any) ([]string, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- projects --

func putProject(q querier, p Project) error {
	if err := checkID(p.ID, idProject); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at`,
		p.ID, p.Name, ms(p.CreatedAt), ms(p.UpdatedAt)); err != nil {
		return fmt.Errorf("runtime: put project: %w", err)
	}
	if _, err := q.Exec(`DELETE FROM project_roots WHERE project_id = ?`, p.ID); err != nil {
		return fmt.Errorf("runtime: replace project roots: %w", err)
	}
	for i, root := range p.Roots {
		if _, err := q.Exec(`INSERT INTO project_roots (project_id, position, path, writable)
			VALUES (?, ?, ?, ?)`, p.ID, i, root.Path, boolInt(root.Writable)); err != nil {
			return fmt.Errorf("runtime: put project root: %w", err)
		}
	}
	if _, err := q.Exec(`DELETE FROM project_sensitive_paths WHERE project_id = ?`, p.ID); err != nil {
		return fmt.Errorf("runtime: replace project sensitive paths: %w", err)
	}
	for i, pattern := range p.SensitivePaths {
		if _, err := q.Exec(`INSERT INTO project_sensitive_paths (project_id, position, pattern)
			VALUES (?, ?, ?)`, p.ID, i, pattern); err != nil {
			return fmt.Errorf("runtime: put project sensitive path: %w", err)
		}
	}
	return nil
}

func getProject(q querier, id string) (*Project, error) {
	var (
		p                    Project
		createdAt, updatedAt int64
	)
	err := q.QueryRow(`SELECT id, name, created_at, updated_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: project %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get project: %w", err)
	}
	p.CreatedAt, p.UpdatedAt = timeFromMS(createdAt), timeFromMS(updatedAt)

	roots, err := q.Query(`SELECT path, writable FROM project_roots WHERE project_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("runtime: read project roots: %w", err)
	}
	defer roots.Close()
	for roots.Next() {
		var (
			root     ProjectRoot
			writable int
		)
		if err := roots.Scan(&root.Path, &writable); err != nil {
			return nil, fmt.Errorf("runtime: read project root: %w", err)
		}
		root.Writable = writable != 0
		p.Roots = append(p.Roots, root)
	}
	if err := roots.Err(); err != nil {
		return nil, fmt.Errorf("runtime: read project roots: %w", err)
	}

	p.SensitivePaths, err = orderedText(q,
		`SELECT pattern FROM project_sensitive_paths WHERE project_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("runtime: read project sensitive paths: %w", err)
	}
	return &p, nil
}

// PutProject stores a Project and its ordered roots and sensitive paths inside
// the caller's transaction, replacing the previous lists.
func (t *Tx) PutProject(p Project) error { return putProject(t.tx, p) }

// GetProject returns one Project with its roots and sensitive paths.
func (t *Tx) GetProject(id string) (*Project, error) { return getProject(t.tx, id) }

// GetProject returns one Project.
func (s *Store) GetProject(id string) (*Project, error) { return getProject(s.db, id) }

// ---------------------------------------------------------------- sessions --

func putSession(q querier, s Session) error {
	if err := checkID(s.ID, idSession); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO sessions
		(id, project_id, title, caller_class, external_conversation, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			title = excluded.title,
			caller_class = excluded.caller_class,
			external_conversation = excluded.external_conversation,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		s.ID, nullText(s.ProjectID), s.Title, string(s.CallerClass), s.ExternalConversation,
		string(s.Status), ms(s.CreatedAt), ms(s.UpdatedAt)); err != nil {
		return fmt.Errorf("runtime: put session: %w", err)
	}
	return nil
}

func scanSession(row rowScanner) (*Session, error) {
	var (
		s                    Session
		projectID            sql.NullString
		createdAt, updatedAt int64
	)
	if err := row.Scan(&s.ID, &projectID, &s.Title, &s.CallerClass, &s.ExternalConversation,
		&s.Status, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	s.ProjectID = text(projectID)
	s.CreatedAt, s.UpdatedAt = timeFromMS(createdAt), timeFromMS(updatedAt)
	return &s, nil
}

func getSession(q querier, id string) (*Session, error) {
	session, err := scanSession(q.QueryRow(`SELECT id, project_id, title, caller_class,
		external_conversation, status, created_at, updated_at FROM sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: session %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get session: %w", err)
	}
	return session, nil
}

// PutSession stores a Session inside the caller's transaction.
func (t *Tx) PutSession(s Session) error { return putSession(t.tx, s) }

// GetSession returns one Session.
func (t *Tx) GetSession(id string) (*Session, error) { return getSession(t.tx, id) }

// GetSession returns one Session.
func (s *Store) GetSession(id string) (*Session, error) { return getSession(s.db, id) }

// -------------------------------------------------------- provider sessions --

func putProviderSession(q querier, p ProviderSession) error {
	if err := checkID(p.ID, idProviderSession); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO provider_sessions
		(id, provider, native_id, locator, project_id, can_resume, can_history, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider = excluded.provider,
			native_id = excluded.native_id,
			locator = excluded.locator,
			project_id = excluded.project_id,
			can_resume = excluded.can_resume,
			can_history = excluded.can_history,
			updated_at = excluded.updated_at`,
		p.ID, p.Provider, p.NativeID, p.Locator, nullText(p.ProjectID),
		boolInt(p.CanResume), boolInt(p.CanHistory), ms(p.CreatedAt), ms(p.UpdatedAt)); err != nil {
		return fmt.Errorf("runtime: put provider session: %w", err)
	}
	return nil
}

func getProviderSession(q querier, id string) (*ProviderSession, error) {
	var (
		p                     ProviderSession
		projectID             sql.NullString
		canResume, canHistory int
		createdAt, updatedAt  int64
	)
	err := q.QueryRow(`SELECT id, provider, native_id, locator, project_id, can_resume, can_history,
		created_at, updated_at FROM provider_sessions WHERE id = ?`, id).
		Scan(&p.ID, &p.Provider, &p.NativeID, &p.Locator, &projectID, &canResume, &canHistory,
			&createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: provider session %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get provider session: %w", err)
	}
	p.ProjectID = text(projectID)
	p.CanResume, p.CanHistory = canResume != 0, canHistory != 0
	p.CreatedAt, p.UpdatedAt = timeFromMS(createdAt), timeFromMS(updatedAt)
	return &p, nil
}

// PutProviderSession stores a ProviderSession inside the caller's transaction.
func (t *Tx) PutProviderSession(p ProviderSession) error { return putProviderSession(t.tx, p) }

// GetProviderSession returns one ProviderSession.
func (t *Tx) GetProviderSession(id string) (*ProviderSession, error) {
	return getProviderSession(t.tx, id)
}

// GetProviderSession returns one ProviderSession.
func (s *Store) GetProviderSession(id string) (*ProviderSession, error) {
	return getProviderSession(s.db, id)
}
