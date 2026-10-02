# Phase 0 — runtime store schema v1 evidence

## Assumption

Runtime owns durable CodeBridge state: projects, sessions, runs, provider sessions,
computer sessions, the event journal, artifacts, policy rules, grants and approvals, in one
SQLite (WAL) store under Application Support. A state change and its journal event commit in
one transaction; the journal has a contiguous per-stream `seq` and a global `pos`; migrations
are forward-only, preceded by a backup, with no downgrade. Frozen in
[docs/v2/architecture.md](../architecture.md) §5.4, §5.6, §7, §8, §10, §13.1, §13.2, §18,
[docs/v2/roadmap.md](../roadmap.md) Phase 0 "Store schema v1" and
[docs/v2/migration.md](../migration.md) §2.1/§7.

## Environment

2026-10-02, macOS 27.0 arm64, `go1.26.5 darwin/arm64`, baseline commit `8d1fcdf` with a clean
worktree. `internal/runtime` imports only the standard library, `modernc.org/sqlite` (already a
module dependency, same driver as V1) and `schema/store/v1`; it does not import
`internal/agentops` or any V1 package. No V1 file was modified (`git status` shows no change
under `internal/agentops`, `internal/manager`, `internal/protocol`, `cmd/manager`,
`cmd/client`). This task touches no TCC-protected resource and makes no TCC or signing claim;
code-signing evidence belongs to the native/daemon spikes.

## Deliverables

- `schema/store/v1/schema.sql` — language-neutral DDL, version 1 (authoritative for the version).
- `schema/store/v1/schema.go` — package `storev1`: `Version = 1`, embedded `SQL()`.
- `internal/runtime/` — `store.go`, `migrate.go`, `paths.go`, `tx.go`, `entities.go`,
  `entities_ops.go`, `runs_ops.go`, `policy_ops.go`, `events.go`, `transitions.go`,
  `store_test.go`, `events_test.go`, `migrate_test.go`.
- `cmd/store-smoke/main.go` — runnable Phase 0 smoke.

## Decisions

1. **The SQL file is the source of truth.** `schema/store/v1/schema.sql` is language-neutral, so
   Go and (later) Swift read the same artifact; `schema/store/v1/schema.go` only embeds it
   (`storev1.SQL()`, no codegen in Phase 0). The directory carries the version, the package is
   named `storev1` so call sites read unambiguously.
2. **Typed current fields only.** Enumerations that the architecture freezes are `CHECK`
   constrained (`run status`, `session status`, `run kind`, caller class, computer state,
   controller kind/channel, event source, policy effect, grant scope, approval status/channel,
   artifact kind/producer). Ordered lists are typed child tables (`project_roots`,
   `project_sensitive_paths`, `computer_session_verbs`, `computer_session_apps`), not serialized
   blobs. The only free text is the event envelope's `payload` / `raw_ref`, which the frozen
   envelope requires to be metadata-first and bounded.
3. **`schema_version` is a ledger table** (`version`, `applied_at`), one row per applied version,
   owned and written by the migration runner (not by `schema.sql`), so the DDL file describes
   exactly one version and a partially applied ledger is impossible.
4. **Forward-only migration.** A store whose ledger is above the binary is refused *before any
   write* (`ErrSchemaNewer`: no DDL, no backup, no ledger change). A store below the binary is
   backed up first with `VACUUM INTO` — SQLite's own consistent snapshot, which includes
   committed WAL content and cannot capture a torn page, unlike copying `runtime.db` / `-wal` /
   `-shm` — and then all pending steps run in **one** transaction, so a failing step leaves the
   store and its ledger untouched.
5. **Durability on commit.** The connection opens with `journal_mode=WAL`,
   `foreign_keys=1`, `busy_timeout=5000`, `synchronous=FULL`, `_txlock=immediate`, and the pool
   is capped at one connection so writers serialize instead of racing to upgrade a read
   transaction. `FULL` (not `NORMAL`) because architecture §8 requires a terminal run transition
   and an approval decision to be durable before they are acknowledged; `TestOpenAppliesDurabilityPragmas`
   reads `PRAGMA synchronous`/`foreign_keys`/`journal_mode` back from the live connection.
6. **Application Support, never the cache.** `runtime.DefaultDir()` =
   `os.UserConfigDir()/CodeBridge`, observed on this workstation as
   `/Users/tangxingpeng/Library/Application Support/CodeBridge`; `DefaultPath()` =
   `.../runtime.db`. `os.UserCacheDir` is never used.
7. **One write path: no standalone mutators.** The store exposes no `Store.Put*`; every write
   goes through `Store.Update(func(*Tx) error)`, which is one `BEGIN IMMEDIATE` transaction that
   commits or fully rolls back. A state change that has a lifecycle event appends that event with
   `Tx.AppendEvent` in the same transaction, so the two can never diverge. The demonstrated
   lifecycle APIs are `Store.TransitionRun` (`run.<status>`) and `Store.SetComputerController`
   (increments `controller_epoch`, `computer.controller.changed`); callers own payload text, the
   store assigns stream/`seq`/`pos`/kind.
8. **Journal.** `pos` is the row identity of `events` (`INTEGER PRIMARY KEY AUTOINCREMENT`, a
   global monotonic cursor); `seq` is assigned per stream as `MAX(seq)+1` inside the write
   transaction with `UNIQUE(stream, seq)`. An append on a `run:<id>` stream also maintains
   `runs.last_seq` (and a plain update never rolls that head back). A rolled-back transaction
   consumes no `seq`, so streams stay gap-free.
   `runtime.ValidateContiguous(events, afterSeq)` is the replay-side gap check.
9. **Run state machine.** `TransitionRun` uses the V1 transition table unchanged
   (`queued → running | cancelled`; `running → completed | failed | cancelled | timeout |
   interrupted`; terminal statuses have no successors) and rejects anything else with
   `ErrIllegalTransition` (the error category consumers map to `invalid_state`) without changing
   the row or appending an event.
10. **Schema-enforced invariants beyond column types.**
    - at most one active Run per ProviderSession: partial unique index
      `runs(provider_session_id) WHERE status IN ('queued','running')` (provider-contracts §5);
    - a `project` grant scope is rejected for `computer.*` verbs (architecture §13.1);
    - a `session`-scoped grant must bind `session_id` (architecture §7.1);
    - `remote_human` grants may only carry `once`/`session` scope and never cover `secret.read`
      or `cloud.upload` (architecture §13.2);
    - foreign keys are enforced at runtime.

## Commands and observed results

Scoped to the new packages; no project-wide build/test was run mid-flight (see Pending).

1. `gofmt -l internal/runtime schema/store/v1 cmd/store-smoke` — clean.
2. `go build ./internal/runtime ./schema/store/v1 ./cmd/store-smoke` — exit 0.
3. `go vet ./internal/runtime ./schema/store/v1 ./cmd/store-smoke` — exit 0.
4. `go test ./internal/runtime -count=1` — `ok github.com/edynasty/codebridge/internal/runtime 0.452s`,
   23 tests pass: `TestOpenAppliesDurabilityPragmas`, `TestOpenCreatesSchemaAndReopens`,
   `TestOpenPathWithSpaces`, `TestUpdateRollsBackOnError`, `TestActiveProviderRunConstraint`,
   `TestEntityRoundTrip`, `TestGrantScopeBoundaries`, `TestIllegalRunTransitions`,
   `TestIdentifierPrefixValidation`, `TestComputerControllerValidation`, `TestAppendEventSeqAndPos`,
   `TestEventContinuityAcrossReopen`, `TestFailedTransactionLeavesNoSeqGap`,
   `TestRunLastSeqFollowsJournal`, `TestValidateContiguousReportsGap`, `TestEventPageLimits`,
   `TestStats`, `TestPingAndMissingIDs`, `TestComputerControllerEventsAreJournaled`,
   `TestMigrateBacksUpBeforeMigrating`, `TestOpenRejectsNewerSchema`, `TestMigrateRollsBackOnFailure`,
   `TestMigrateWithoutPathFails`.
5. `go run ./cmd/store-smoke` — PASS. Observed output (the temporary path is elided):

```text
ok   schema v1; default store dir /Users/tangxingpeng/Library/Application Support/CodeBridge; default db /Users/tangxingpeng/Library/Application Support/CodeBridge/runtime.db
ok   open + ping /var/folders/.../codebridge-store-smoke-.../runtime.db
ok   seed project/session/provider session/run + run.running event in one transaction
ok   rejected: second active run on one provider session, and completed -> running; run_a is terminal
ok   journal: run_smoke_b has 5 contiguous events, run.last_seq follows the stream head
ok   computer session: controller change journaled with controller_epoch 1
ok   stats runs_active=1 runs_total=2 sessions_open=1 computer_sessions_open=1 pending_approvals=0 journal_pos=8
ok   closed store
ok   after reopen: appended seq 6 pos 9, 6 events on the stream, journal head 9, replay page 2
ok   reopened store: per-stream seq and global pos continue where they stopped
store-smoke: PASS
```

6. `go run ./cmd/store-smoke -path /tmp/cb-phase0-store/runtime.db` then
   `sqlite3 .../runtime.db "SELECT type||' '||name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name;"` —
   20 objects: tables `approvals`, `artifacts`, `computer_session_apps`, `computer_session_verbs`,
   `computer_sessions`, `events`, `grants`, `policy_rules`, `project_roots`,
   `project_sensitive_paths`, `projects`, `provider_sessions`, `runs`, `schema_version`,
   `sessions`; indexes `idx_artifacts_producer`, `idx_computer_sessions_session`,
   `idx_provider_sessions_native`, `idx_runs_active_provider_session`, `idx_runs_session`,
   `idx_runs_status`. `PRAGMA journal_mode` → `wal`; `SELECT version FROM schema_version` → `1`;
   the stored `grants` DDL contains the three `CHECK` clauses above. After a clean close only
   `runtime.db` remains (WAL is checkpointed); the file is `0600` and directories the store
   creates are `0700` (observed with a nested `-path`).
7. `go list -deps ./internal/runtime | grep codebridge` → only
   `github.com/edynasty/codebridge/internal/runtime` and
   `github.com/edynasty/codebridge/schema/store/v1`.

### What the tests prove

- **Rollback**: a failed `Update` leaves no run row and no event, and the state survives a
  reopen; a rolled-back append consumes no `seq` (`TestUpdateRollsBackOnError`,
  `TestFailedTransactionLeavesNoSeqGap`).
- **Sequence/reopen**: per-stream `seq` starts at 1 and stays contiguous while global `pos`
  advances across streams; both continue after `Close`/`Open`; `EventsAfter` /
  `EventsForStream` replay from a cursor (`TestAppendEventSeqAndPos`,
  `TestEventContinuityAcrossReopen`, `TestRunLastSeqFollowsJournal`).
- **Migration/backup/forward version**: a legacy store (tables, no ledger) gets a snapshot in
  which the pre-migration row is present and a row written by the migration step is absent —
  i.e. the backup really precedes the migration; a newer ledger is refused with no writes and no
  backup; a failing migration rolls back completely (no schema, no ledger) while the backup
  still exists (`TestMigrateBacksUpBeforeMigrating`, `TestOpenRejectsNewerSchema`,
  `TestMigrateRollsBackOnFailure`, `TestMigrateWithoutPathFails`).
- **Durability settings**: the live connection reports `synchronous=FULL`, `foreign_keys=1` and
  `journal_mode=wal` (`TestOpenAppliesDurabilityPragmas`).
- **Constraints**: a second running/queued Run on one ProviderSession is rejected and accepted
  again once the first run terminates; `project`-scope computer grants, session grants without a
  session, and forbidden `remote_human` grants are rejected while the legal combinations are
  accepted; wrong id prefixes and events without a kind are rejected
  (`TestActiveProviderRunConstraint`, `TestGrantScopeBoundaries`,
  `TestIdentifierPrefixValidation`, `TestComputerControllerValidation`).
- **Run state machine**: legal `queued → running → completed` succeeds; `completed → running |
  queued | completed | cancelled`, `queued → completed` and `running → queued` fail with
  `ErrIllegalTransition`, leaving the row and the journal unchanged
  (`TestIllegalRunTransitions`).
- **Typed round trip**: every entity, including ordered roots/sensitive paths, granted verbs and
  app selectors, survives a put/get unchanged (`TestEntityRoundTrip`).
- **Daemon surface**: `Stats` counts active/total runs, open sessions, non-closed computer
  sessions, pending approvals and `JournalPos`; page limits are default 200 / cap 500; unknown
  ids return `ErrNotFound`; `Ping` works (`TestStats`, `TestEventPageLimits`,
  `TestPingAndMissingIDs`).

## Parent verification complete

- Project-wide `make fmt-check`, `go test ./...`, `go vet ./...`, `go build ./...` PASS after integration (17 test packages, 8 packages without tests); V1 included unchanged.
- Parent independently executed `go run ./cmd/store-smoke`: illegal transition rejection, transaction/journal, controller epoch, reopen and seq/pos continuation PASS.
- Real launchd daemon wired this Store, opened schema 1, was killed/restarted and reopened it successfully. An invalid `/dev/null/runtime.db` startup exited 1 before creating the authority sockets; no degraded placeholder Store is served.
- Bilingual Roadmap and closeout record this definition/migration milestone as PASS; no V1 database import or complete Runtime scheduler is claimed.

## Untested / not claimed

- The real `~/Library/Application Support/CodeBridge` store was **never opened, created,
  migrated or written**; only the path was resolved. No claim is made that a daemon-owned store
  exists at that path.
- Multi-process access to one store file is untested; within a process the pool is one
  connection by design.
- Only schema v1 exists, so the migration path was exercised with a synthetic v0→v1 step, not a
  real upgrade; restoring from a backup file was not exercised (only backup creation and its
  pre-migration consistency).
- No backup rotation/retention policy beyond one file per migration attempt.
- No journal retention or pruning (`harness.raw` retention from architecture §8 is not
  implemented).
- No run-manager lift, no V1 history import, no provider binding, no computer-broker wiring:
  `internal/runtime` currently exposes the store and journal only.
- No Swift/Host IPC binding of the schema and no code generation.
- `synchronous=FULL` is asserted as the applied connection setting; its crash/power-loss behavior
  was not measured on this machine (no fault-injection or power-cut test).
- No latency/throughput measurement; the 600-event test only checks paging limits.
- `StoreStats.ComputerSessionsOpen` is defined as starting/active/suspended; the choice is open
  to Phase 1 review.

## Interface for the daemon (agreed with `DaemonBaseline`)

`github.com/edynasty/codebridge/internal/runtime` (package `runtime`); the daemon core does not
import it directly — the parent wires one adapter.

- `runtime.SchemaVersion` (const 1), `runtime.Open(path) (*Store, error)`, `(*Store).Close()`,
  `(*Store).Path()`, `runtime.DefaultDir()`, `runtime.DefaultPath()`.
- `(*Store).Update(func(*Tx) error) error` / `UpdateContext(ctx, fn)` — the only write path, one
  `BEGIN IMMEDIATE` transaction, full commit or full rollback.
- `(*Tx).AppendEvent(Event) (Event, error)` — assigns the contiguous per-stream `seq` and the
  global `pos`; `(*Tx).Put*` for Project, Session, Run, ProviderSession, ComputerSession,
  Artifact, PolicyRule, Grant, Approval.
- Lifecycle: `(*Store).TransitionRun(RunTransition) (Run, Event, error)`,
  `(*Store).SetComputerController(ControllerChange) (ComputerSession, Event, error)`.
- Reads (`*Store`): `Get*` for every entity, `EventsAfter(pos, limit)`,
  `EventsForStream(stream, afterSeq, limit)` (default 200, capped at 500), `JournalHead()`,
  `StreamHead(stream)`, `Stats()`/`StatsContext(ctx)` (RunsActive, RunsTotal, SessionsOpen,
  ComputerSessionsOpen, PendingApprovals, JournalPos), `Ping(ctx)`.
- Streams: `StreamRun(id)` = `run:<id>`, `StreamComputer(id)`, `StreamSession(id)`,
  `StreamSystem`; `ValidateContiguous(events, afterSeq)` for replay-side gap detection.
- Errors: `ErrNotFound`, `ErrSchemaNewer`, `ErrIllegalTransition`.

## Smoke for the parent

```bash
go run ./cmd/store-smoke                 # fresh temp dir, removed on exit
go run ./cmd/store-smoke -keep           # keep the temp store for inspection
go run ./cmd/store-smoke -path /tmp/cb/runtime.db
```

The command exits non-zero on the first failure, prints one `ok` line per step and only touches
the path it is given (default: a temporary directory). It proves schema creation, a
one-transaction state change plus event, the active-provider-run constraint, rejection of an
illegal run transition, `seq`/`pos` continuity across a reopen, replay and stats.
