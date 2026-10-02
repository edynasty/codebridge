-- CodeBridge runtime store schema v1.
--
-- Authority: codebridged (docs/v2/architecture.md §5.4, §18). Entities and
-- event fields follow docs/v2/architecture.md §7 and §8; the table list is the
-- Phase 0 definition in docs/v2/roadmap.md ("Store schema v1").
--
-- Rules encoded here:
--   * columns are the current typed fields only; there are no generic
--     context/JSON bags;
--   * the schema version lives in the `schema_version` ledger (version,
--     applied_at), which the migration runner (internal/runtime) owns and
--     writes, not this file;
--   * migrations are forward-only: this file describes exactly one version, a
--     store whose ledger is newer than the binary is refused before any write,
--     and each migration is preceded by a consistent SQLite backup;
--   * connection settings are applied by the migration runner, not here:
--     journal_mode=WAL, foreign_keys=ON, busy_timeout=5000,
--     synchronous=NORMAL, BEGIN IMMEDIATE transactions.
--
-- Identifiers keep the V2 prefixes: prj_ (Project), ses_ (Session), run_ (Run),
-- psn_ (ProviderSession), cmp_ (ComputerSession), art_ (Artifact). Policy rule,
-- grant and approval ids are opaque.
--
-- Timestamps are Unix milliseconds; 0 means unset. Empty optional references are
-- stored as NULL and read back as "".

CREATE TABLE IF NOT EXISTS projects (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL DEFAULT 0
);

-- Project roots are an ordered, typed list, not a serialized blob.
CREATE TABLE IF NOT EXISTS project_roots (
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	position   INTEGER NOT NULL,
	path       TEXT NOT NULL,
	writable   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (project_id, position)
);

CREATE TABLE IF NOT EXISTS project_sensitive_paths (
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	position   INTEGER NOT NULL,
	pattern    TEXT NOT NULL,
	PRIMARY KEY (project_id, position)
);

CREATE TABLE IF NOT EXISTS sessions (
	id                    TEXT PRIMARY KEY,
	project_id            TEXT REFERENCES projects(id) ON DELETE SET NULL,
	title                 TEXT NOT NULL DEFAULT '',
	caller_class          TEXT NOT NULL CHECK (caller_class IN ('remote_ai', 'local_ui', 'local_mcp', 'agent_internal')),
	external_conversation TEXT NOT NULL DEFAULT '',
	status                TEXT NOT NULL CHECK (status IN ('open', 'archived')),
	created_at            INTEGER NOT NULL DEFAULT 0,
	updated_at            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS provider_sessions (
	id          TEXT PRIMARY KEY,
	provider    TEXT NOT NULL,
	native_id   TEXT NOT NULL DEFAULT '',
	locator     TEXT NOT NULL DEFAULT '',
	project_id  TEXT REFERENCES projects(id) ON DELETE SET NULL,
	can_resume  INTEGER NOT NULL DEFAULT 0,
	can_history INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL DEFAULT 0,
	updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_sessions_native
	ON provider_sessions(provider, native_id) WHERE native_id <> '';

CREATE TABLE IF NOT EXISTS runs (
	id                  TEXT PRIMARY KEY,
	session_id          TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	project_id          TEXT REFERENCES projects(id) ON DELETE SET NULL,
	kind                TEXT NOT NULL CHECK (kind IN ('agent', 'cloud')),
	provider            TEXT NOT NULL DEFAULT '',
	model               TEXT NOT NULL DEFAULT '',
	autonomy            TEXT NOT NULL DEFAULT '',
	role                TEXT NOT NULL DEFAULT '',
	parent_run_id       TEXT REFERENCES runs(id) ON DELETE SET NULL,
	continues_run_id    TEXT REFERENCES runs(id) ON DELETE SET NULL,
	provider_session_id TEXT REFERENCES provider_sessions(id) ON DELETE SET NULL,
	status              TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'timeout', 'interrupted')),
	process_pid         INTEGER NOT NULL DEFAULT 0,
	process_started_at  INTEGER NOT NULL DEFAULT 0,
	process_group_id    INTEGER NOT NULL DEFAULT 0,
	output_ref          TEXT NOT NULL DEFAULT '',
	error_category      TEXT NOT NULL DEFAULT '',
	last_seq            INTEGER NOT NULL DEFAULT 0,
	created_at          INTEGER NOT NULL DEFAULT 0,
	started_at          INTEGER NOT NULL DEFAULT 0,
	finished_at         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_runs_session ON runs(session_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

-- At most one active Run per ProviderSession (architecture §7.1 by way of
-- provider-contracts.md §5: "Runtime guarantees at most one active Run per
-- ProviderSession").
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_active_provider_session
	ON runs(provider_session_id)
	WHERE provider_session_id IS NOT NULL AND status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS computer_sessions (
	id                  TEXT PRIMARY KEY,
	session_id          TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	run_id              TEXT REFERENCES runs(id) ON DELETE SET NULL,
	target              TEXT NOT NULL,
	state               TEXT NOT NULL CHECK (state IN ('starting', 'active', 'suspended', 'closed', 'failed')),
	controller_kind     TEXT NOT NULL CHECK (controller_kind IN ('agent', 'human', 'none')),
	controller_holder   TEXT NOT NULL DEFAULT '',
	controller_channel  TEXT NOT NULL CHECK (controller_channel IN ('', 'local', 'remote')),
	controller_epoch    INTEGER NOT NULL DEFAULT 0,
	arbiter_instance    TEXT NOT NULL DEFAULT '',
	lease_holder        TEXT NOT NULL DEFAULT '',
	lease_expires_at    INTEGER NOT NULL DEFAULT 0,
	can_observe         INTEGER NOT NULL DEFAULT 0,
	can_input           INTEGER NOT NULL DEFAULT 0,
	can_preview         INTEGER NOT NULL DEFAULT 0,
	last_frame_id       TEXT NOT NULL DEFAULT '',
	geometry_generation INTEGER NOT NULL DEFAULT 0,
	created_at          INTEGER NOT NULL DEFAULT 0,
	updated_at          INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_computer_sessions_session ON computer_sessions(session_id);

CREATE TABLE IF NOT EXISTS computer_session_verbs (
	computer_session_id TEXT NOT NULL REFERENCES computer_sessions(id) ON DELETE CASCADE,
	position            INTEGER NOT NULL,
	verb                TEXT NOT NULL,
	PRIMARY KEY (computer_session_id, position)
);

CREATE TABLE IF NOT EXISTS computer_session_apps (
	computer_session_id TEXT NOT NULL REFERENCES computer_sessions(id) ON DELETE CASCADE,
	position            INTEGER NOT NULL,
	bundle_id           TEXT NOT NULL,
	PRIMARY KEY (computer_session_id, position)
);

CREATE TABLE IF NOT EXISTS artifacts (
	id            TEXT PRIMARY KEY,
	producer_kind TEXT NOT NULL CHECK (producer_kind IN ('run', 'computer_session')),
	producer_id   TEXT NOT NULL,
	kind          TEXT NOT NULL CHECK (kind IN ('file', 'patch', 'commit', 'branch_ref', 'report', 'test_result', 'log_bundle', 'cloud_result', 'saved_screenshot')),
	reference     TEXT NOT NULL,
	digest        TEXT NOT NULL DEFAULT '',
	size_bytes    INTEGER NOT NULL DEFAULT 0,
	sensitivity   TEXT NOT NULL DEFAULT '',
	retention     TEXT NOT NULL DEFAULT '',
	created_at    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_artifacts_producer ON artifacts(producer_kind, producer_id);

CREATE TABLE IF NOT EXISTS policy_rules (
	id         TEXT PRIMARY KEY,
	effect     TEXT NOT NULL CHECK (effect IN ('allow', 'deny', 'ask')),
	verb       TEXT NOT NULL,
	selector   TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS approvals (
	id           TEXT PRIMARY KEY,
	run_id       TEXT REFERENCES runs(id) ON DELETE SET NULL,
	session_id   TEXT REFERENCES sessions(id) ON DELETE SET NULL,
	project_id   TEXT REFERENCES projects(id) ON DELETE SET NULL,
	verb         TEXT NOT NULL,
	selector     TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
	channel      TEXT NOT NULL CHECK (channel IN ('local_ui', 'remote_human')),
	reason       TEXT NOT NULL DEFAULT '',
	requested_at INTEGER NOT NULL DEFAULT 0,
	decided_at   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS grants (
	id          TEXT PRIMARY KEY,
	verb        TEXT NOT NULL,
	selector    TEXT NOT NULL DEFAULT '',
	scope       TEXT NOT NULL CHECK (scope IN ('once', 'session', 'project', 'always')),
	session_id  TEXT REFERENCES sessions(id) ON DELETE CASCADE,
	project_id  TEXT REFERENCES projects(id) ON DELETE CASCADE,
	run_id      TEXT REFERENCES runs(id) ON DELETE SET NULL,
	approval_id TEXT REFERENCES approvals(id) ON DELETE SET NULL,
	channel     TEXT NOT NULL CHECK (channel IN ('local_ui', 'remote_human')),
	expires_at  INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL DEFAULT 0,
	revoked_at  INTEGER NOT NULL DEFAULT 0,
	-- architecture §13.1: the project scope does not apply to computer verbs.
	CHECK (scope <> 'project' OR verb NOT LIKE 'computer.%'),
	-- architecture §7.1/§13.1: a session-scoped grant names the session it is
	-- scoped to.
	CHECK (scope <> 'session' OR session_id IS NOT NULL),
	-- architecture §13.2: the opt-in remote_human channel carries only once or
	-- session scopes and never covers secret.read or cloud.upload.
	CHECK (channel <> 'remote_human' OR (
		scope IN ('once', 'session')
		AND verb NOT IN ('secret.read', 'cloud.upload')
	))
);

-- The journal: global pos (subscription cursor) and contiguous per-stream seq
-- (gap detection, idempotent replay). A state change and its event commit in
-- one transaction; providers never write here directly (architecture §8).
CREATE TABLE IF NOT EXISTS events (
	pos            INTEGER PRIMARY KEY AUTOINCREMENT,
	stream         TEXT NOT NULL,
	seq            INTEGER NOT NULL,
	at_ms          INTEGER NOT NULL,
	session_id     TEXT NOT NULL DEFAULT '',
	project_id     TEXT NOT NULL DEFAULT '',
	source         TEXT NOT NULL CHECK (source IN ('bridge', 'runtime', 'agent', 'computer', 'policy', 'system')),
	kind           TEXT NOT NULL,
	turn_ref       TEXT NOT NULL DEFAULT '',
	correlation_id TEXT NOT NULL DEFAULT '',
	payload        TEXT NOT NULL DEFAULT '',
	raw_ref        TEXT NOT NULL DEFAULT '',
	UNIQUE (stream, seq)
);
