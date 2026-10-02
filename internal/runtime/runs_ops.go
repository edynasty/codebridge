package runtime

import (
	"database/sql"
	"errors"
	"fmt"
)

// -------------------------------------------------------------------- runs --

const runColumns = `id, session_id, project_id, kind, provider, model, autonomy, role,
	parent_run_id, continues_run_id, provider_session_id, status, process_pid,
	process_started_at, process_group_id, output_ref, error_category, last_seq,
	created_at, started_at, finished_at`

func putRun(q querier, r Run) error {
	if err := checkID(r.ID, idRun); err != nil {
		return err
	}
	// last_seq and created_at are not overwritten: the journal maintains the
	// stream head and the row keeps its creation time.
	if _, err := q.Exec(`INSERT INTO runs (`+runColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			project_id = excluded.project_id,
			kind = excluded.kind,
			provider = excluded.provider,
			model = excluded.model,
			autonomy = excluded.autonomy,
			role = excluded.role,
			parent_run_id = excluded.parent_run_id,
			continues_run_id = excluded.continues_run_id,
			provider_session_id = excluded.provider_session_id,
			status = excluded.status,
			process_pid = excluded.process_pid,
			process_started_at = excluded.process_started_at,
			process_group_id = excluded.process_group_id,
			output_ref = excluded.output_ref,
			error_category = excluded.error_category,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at`,
		r.ID, r.SessionID, nullText(r.ProjectID), string(r.Kind), r.Provider, r.Model, r.Autonomy,
		r.Role, nullText(r.ParentRunID), nullText(r.ContinuesRunID), nullText(r.ProviderSessionID),
		string(r.Status), r.ProcessPID, ms(r.ProcessStartedAt), r.ProcessGroupID, r.OutputRef,
		r.ErrorCategory, r.LastSeq, ms(r.CreatedAt), ms(r.StartedAt), ms(r.FinishedAt)); err != nil {
		return fmt.Errorf("runtime: put run: %w", err)
	}
	return nil
}

func scanRun(row rowScanner) (*Run, error) {
	var (
		r                                                    Run
		projectID, parentRunID, continuesRunID, providerSnID sql.NullString
		processStartedAt, createdAt, startedAt, finishedAt   int64
	)
	if err := row.Scan(&r.ID, &r.SessionID, &projectID, &r.Kind, &r.Provider, &r.Model, &r.Autonomy,
		&r.Role, &parentRunID, &continuesRunID, &providerSnID, &r.Status, &r.ProcessPID,
		&processStartedAt, &r.ProcessGroupID, &r.OutputRef, &r.ErrorCategory, &r.LastSeq,
		&createdAt, &startedAt, &finishedAt); err != nil {
		return nil, err
	}
	r.ProjectID, r.ParentRunID = text(projectID), text(parentRunID)
	r.ContinuesRunID, r.ProviderSessionID = text(continuesRunID), text(providerSnID)
	r.ProcessStartedAt, r.CreatedAt = timeFromMS(processStartedAt), timeFromMS(createdAt)
	r.StartedAt, r.FinishedAt = timeFromMS(startedAt), timeFromMS(finishedAt)
	return &r, nil
}

func getRun(q querier, id string) (*Run, error) {
	run, err := scanRun(q.QueryRow(`SELECT `+runColumns+` FROM runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: run %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get run: %w", err)
	}
	return run, nil
}

// PutRun stores a Run inside the caller's transaction. At most one Run with
// status queued or running may reference the same ProviderSession; a second one
// is rejected by the store.
func (t *Tx) PutRun(r Run) error { return putRun(t.tx, r) }

// GetRun returns one Run.
func (t *Tx) GetRun(id string) (*Run, error) { return getRun(t.tx, id) }

// GetRun returns one Run.
func (s *Store) GetRun(id string) (*Run, error) { return getRun(s.db, id) }

// ------------------------------------------------------- computer sessions --

func putComputerSession(q querier, c ComputerSession) error {
	if err := checkID(c.ID, idComputerSession); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO computer_sessions
		(id, session_id, run_id, target, state, controller_kind, controller_holder,
		 controller_channel, controller_epoch, arbiter_instance, lease_holder,
		 lease_expires_at, can_observe, can_input, can_preview, last_frame_id,
		 geometry_generation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			run_id = excluded.run_id,
			target = excluded.target,
			state = excluded.state,
			controller_kind = excluded.controller_kind,
			controller_holder = excluded.controller_holder,
			controller_channel = excluded.controller_channel,
			controller_epoch = excluded.controller_epoch,
			arbiter_instance = excluded.arbiter_instance,
			lease_holder = excluded.lease_holder,
			lease_expires_at = excluded.lease_expires_at,
			can_observe = excluded.can_observe,
			can_input = excluded.can_input,
			can_preview = excluded.can_preview,
			last_frame_id = excluded.last_frame_id,
			geometry_generation = excluded.geometry_generation,
			updated_at = excluded.updated_at`,
		c.ID, c.SessionID, nullText(c.RunID), c.Target, string(c.State), string(c.Controller.Kind),
		c.Controller.Holder, string(c.Controller.Channel), c.ControllerEpoch, c.ArbiterInstance,
		c.LeaseHolder, ms(c.LeaseExpiresAt), boolInt(c.CanObserve), boolInt(c.CanInput),
		boolInt(c.CanPreview), c.LastFrameID, c.GeometryGeneration, ms(c.CreatedAt),
		ms(c.UpdatedAt)); err != nil {
		return fmt.Errorf("runtime: put computer session: %w", err)
	}
	if _, err := q.Exec(`DELETE FROM computer_session_verbs WHERE computer_session_id = ?`, c.ID); err != nil {
		return fmt.Errorf("runtime: replace computer session verbs: %w", err)
	}
	for i, verb := range c.GrantedVerbs {
		if _, err := q.Exec(`INSERT INTO computer_session_verbs (computer_session_id, position, verb)
			VALUES (?, ?, ?)`, c.ID, i, string(verb)); err != nil {
			return fmt.Errorf("runtime: put computer session verb: %w", err)
		}
	}
	if _, err := q.Exec(`DELETE FROM computer_session_apps WHERE computer_session_id = ?`, c.ID); err != nil {
		return fmt.Errorf("runtime: replace computer session app selectors: %w", err)
	}
	for i, bundleID := range c.AppSelectors {
		if _, err := q.Exec(`INSERT INTO computer_session_apps (computer_session_id, position, bundle_id)
			VALUES (?, ?, ?)`, c.ID, i, bundleID); err != nil {
			return fmt.Errorf("runtime: put computer session app selector: %w", err)
		}
	}
	return nil
}

func getComputerSession(q querier, id string) (*ComputerSession, error) {
	var (
		c                                    ComputerSession
		runID                                sql.NullString
		canObserve, canInput, canPreview     int
		leaseExpiresAt, createdAt, updatedAt int64
	)
	err := q.QueryRow(`SELECT id, session_id, run_id, target, state, controller_kind,
		controller_holder, controller_channel, controller_epoch, arbiter_instance, lease_holder,
		lease_expires_at, can_observe, can_input, can_preview, last_frame_id,
		geometry_generation, created_at, updated_at
		FROM computer_sessions WHERE id = ?`, id).
		Scan(&c.ID, &c.SessionID, &runID, &c.Target, &c.State, &c.Controller.Kind,
			&c.Controller.Holder, &c.Controller.Channel, &c.ControllerEpoch, &c.ArbiterInstance,
			&c.LeaseHolder, &leaseExpiresAt, &canObserve, &canInput, &canPreview, &c.LastFrameID,
			&c.GeometryGeneration, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: computer session %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get computer session: %w", err)
	}
	c.RunID = text(runID)
	c.CanObserve, c.CanInput, c.CanPreview = canObserve != 0, canInput != 0, canPreview != 0
	c.LeaseExpiresAt, c.CreatedAt, c.UpdatedAt = timeFromMS(leaseExpiresAt), timeFromMS(createdAt), timeFromMS(updatedAt)

	verbs, err := orderedText(q,
		`SELECT verb FROM computer_session_verbs WHERE computer_session_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("runtime: read computer session verbs: %w", err)
	}
	for _, verb := range verbs {
		c.GrantedVerbs = append(c.GrantedVerbs, Verb(verb))
	}
	c.AppSelectors, err = orderedText(q,
		`SELECT bundle_id FROM computer_session_apps WHERE computer_session_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("runtime: read computer session app selectors: %w", err)
	}
	return &c, nil
}

// PutComputerSession stores a ComputerSession with its granted verbs and app
// selectors inside the caller's transaction, replacing the previous lists.
func (t *Tx) PutComputerSession(c ComputerSession) error { return putComputerSession(t.tx, c) }

// GetComputerSession returns one ComputerSession.
func (t *Tx) GetComputerSession(id string) (*ComputerSession, error) {
	return getComputerSession(t.tx, id)
}

// GetComputerSession returns one ComputerSession.
func (s *Store) GetComputerSession(id string) (*ComputerSession, error) {
	return getComputerSession(s.db, id)
}
