package runtime

import (
	"database/sql"
	"errors"
	"fmt"
)

// --------------------------------------------------------------- artifacts --

func putArtifact(q querier, a Artifact) error {
	if err := checkID(a.ID, idArtifact); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO artifacts
		(id, producer_kind, producer_id, kind, reference, digest, size_bytes, sensitivity,
		 retention, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			producer_kind = excluded.producer_kind,
			producer_id = excluded.producer_id,
			kind = excluded.kind,
			reference = excluded.reference,
			digest = excluded.digest,
			size_bytes = excluded.size_bytes,
			sensitivity = excluded.sensitivity,
			retention = excluded.retention`,
		a.ID, string(a.ProducerKind), a.ProducerID, string(a.Kind), a.Reference, a.Digest,
		a.SizeBytes, a.Sensitivity, a.Retention, ms(a.CreatedAt)); err != nil {
		return fmt.Errorf("runtime: put artifact: %w", err)
	}
	return nil
}

func getArtifact(q querier, id string) (*Artifact, error) {
	var (
		a         Artifact
		createdAt int64
	)
	err := q.QueryRow(`SELECT id, producer_kind, producer_id, kind, reference, digest, size_bytes,
		sensitivity, retention, created_at FROM artifacts WHERE id = ?`, id).
		Scan(&a.ID, &a.ProducerKind, &a.ProducerID, &a.Kind, &a.Reference, &a.Digest, &a.SizeBytes,
			&a.Sensitivity, &a.Retention, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: artifact %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get artifact: %w", err)
	}
	a.CreatedAt = timeFromMS(createdAt)
	return &a, nil
}

// PutArtifact stores Artifact metadata inside the caller's transaction. Payload
// bytes never enter the store; an artifact creation that has a lifecycle event
// appends artifact.created in the same transaction.
func (t *Tx) PutArtifact(a Artifact) error { return putArtifact(t.tx, a) }

// GetArtifact returns one Artifact.
func (t *Tx) GetArtifact(id string) (*Artifact, error) { return getArtifact(t.tx, id) }

// GetArtifact returns one Artifact.
func (s *Store) GetArtifact(id string) (*Artifact, error) { return getArtifact(s.db, id) }

// ------------------------------------------------------------ policy rules --

func putPolicyRule(q querier, r PolicyRule) error {
	if err := requireText("policy rule id", r.ID); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO policy_rules (id, effect, verb, selector, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			effect = excluded.effect,
			verb = excluded.verb,
			selector = excluded.selector,
			updated_at = excluded.updated_at`,
		r.ID, string(r.Effect), string(r.Verb), r.Selector, ms(r.CreatedAt), ms(r.UpdatedAt)); err != nil {
		return fmt.Errorf("runtime: put policy rule: %w", err)
	}
	return nil
}

func getPolicyRule(q querier, id string) (*PolicyRule, error) {
	var (
		r                    PolicyRule
		createdAt, updatedAt int64
	)
	err := q.QueryRow(`SELECT id, effect, verb, selector, created_at, updated_at
		FROM policy_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.Effect, &r.Verb, &r.Selector, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: policy rule %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get policy rule: %w", err)
	}
	r.CreatedAt, r.UpdatedAt = timeFromMS(createdAt), timeFromMS(updatedAt)
	return &r, nil
}

// PutPolicyRule stores one Policy rule (verb + selector + effect) inside the
// caller's transaction.
func (t *Tx) PutPolicyRule(r PolicyRule) error { return putPolicyRule(t.tx, r) }

// GetPolicyRule returns one Policy rule.
func (t *Tx) GetPolicyRule(id string) (*PolicyRule, error) { return getPolicyRule(t.tx, id) }

// GetPolicyRule returns one Policy rule.
func (s *Store) GetPolicyRule(id string) (*PolicyRule, error) { return getPolicyRule(s.db, id) }

// ------------------------------------------------------------------ grants --

func putGrant(q querier, g Grant) error {
	if err := requireText("grant id", g.ID); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO grants
		(id, verb, selector, scope, session_id, project_id, run_id, approval_id, channel,
		 expires_at, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			verb = excluded.verb,
			selector = excluded.selector,
			scope = excluded.scope,
			session_id = excluded.session_id,
			project_id = excluded.project_id,
			run_id = excluded.run_id,
			approval_id = excluded.approval_id,
			channel = excluded.channel,
			expires_at = excluded.expires_at,
			revoked_at = excluded.revoked_at`,
		g.ID, string(g.Verb), g.Selector, string(g.Scope), nullText(g.SessionID),
		nullText(g.ProjectID), nullText(g.RunID), nullText(g.ApprovalID), string(g.Channel),
		ms(g.ExpiresAt), ms(g.CreatedAt), ms(g.RevokedAt)); err != nil {
		return fmt.Errorf("runtime: put grant: %w", err)
	}
	return nil
}

func getGrant(q querier, id string) (*Grant, error) {
	var (
		g                                       Grant
		sessionID, projectID, runID, approvalID sql.NullString
		expiresAt, createdAt, revokedAt         int64
	)
	err := q.QueryRow(`SELECT id, verb, selector, scope, session_id, project_id, run_id, approval_id,
		channel, expires_at, created_at, revoked_at FROM grants WHERE id = ?`, id).
		Scan(&g.ID, &g.Verb, &g.Selector, &g.Scope, &sessionID, &projectID, &runID, &approvalID,
			&g.Channel, &expiresAt, &createdAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: grant %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get grant: %w", err)
	}
	g.SessionID, g.ProjectID = text(sessionID), text(projectID)
	g.RunID, g.ApprovalID = text(runID), text(approvalID)
	g.ExpiresAt, g.CreatedAt, g.RevokedAt = timeFromMS(expiresAt), timeFromMS(createdAt), timeFromMS(revokedAt)
	return &g, nil
}

// PutGrant stores a permission grant inside the caller's transaction. Only the
// local UI or the opt-in remote_human channel produces one, and the schema
// rejects the channel/scope/verb combinations architecture §13.2 forbids.
func (t *Tx) PutGrant(g Grant) error { return putGrant(t.tx, g) }

// GetGrant returns one grant.
func (t *Tx) GetGrant(id string) (*Grant, error) { return getGrant(t.tx, id) }

// GetGrant returns one grant.
func (s *Store) GetGrant(id string) (*Grant, error) { return getGrant(s.db, id) }

// --------------------------------------------------------------- approvals --

func putApproval(q querier, a Approval) error {
	if err := requireText("approval id", a.ID); err != nil {
		return err
	}
	if _, err := q.Exec(`INSERT INTO approvals
		(id, run_id, session_id, project_id, verb, selector, status, channel, reason,
		 requested_at, decided_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			run_id = excluded.run_id,
			session_id = excluded.session_id,
			project_id = excluded.project_id,
			verb = excluded.verb,
			selector = excluded.selector,
			status = excluded.status,
			channel = excluded.channel,
			reason = excluded.reason,
			decided_at = excluded.decided_at`,
		a.ID, nullText(a.RunID), nullText(a.SessionID), nullText(a.ProjectID), string(a.Verb),
		a.Selector, string(a.Status), string(a.Channel), a.Reason, ms(a.RequestedAt),
		ms(a.DecidedAt)); err != nil {
		return fmt.Errorf("runtime: put approval: %w", err)
	}
	return nil
}

func getApproval(q querier, id string) (*Approval, error) {
	var (
		a                           Approval
		runID, sessionID, projectID sql.NullString
		requestedAt, decidedAt      int64
	)
	err := q.QueryRow(`SELECT id, run_id, session_id, project_id, verb, selector, status, channel,
		reason, requested_at, decided_at FROM approvals WHERE id = ?`, id).
		Scan(&a.ID, &runID, &sessionID, &projectID, &a.Verb, &a.Selector, &a.Status, &a.Channel,
			&a.Reason, &requestedAt, &decidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: approval %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime: get approval: %w", err)
	}
	a.RunID, a.SessionID, a.ProjectID = text(runID), text(sessionID), text(projectID)
	a.RequestedAt, a.DecidedAt = timeFromMS(requestedAt), timeFromMS(decidedAt)
	return &a, nil
}

// PutApproval stores an approval request and its decision inside the caller's
// transaction; an approval decision that must be durable appends its
// policy.approval.decided event there too.
func (t *Tx) PutApproval(a Approval) error { return putApproval(t.tx, a) }

// GetApproval returns one approval.
func (t *Tx) GetApproval(id string) (*Approval, error) { return getApproval(t.tx, id) }

// GetApproval returns one approval.
func (s *Store) GetApproval(id string) (*Approval, error) { return getApproval(s.db, id) }
