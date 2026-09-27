---
name: codebridge-subagent
description: Delegate self-contained coding and investigation tasks to local subagents (omp by default; opencode or codex when a profile pins them) on the user's machine. Prefer persistent agent_start + agent_status/result when task duration is unknown; use synchronous agent only for trivially bounded work.
---

# Delegating to local subagents

CodeBridge runs local coding agents inside one logical workspace. A subagent
does **not** inherit this conversation, so every delegated task must be
self-contained.

## Capability discovery

Before delegating:

1. call `list_devices` and select an online device;
2. verify the target workspace is exposed;
3. inspect the device `tool_policy` so you know whether Agent tools are
   enabled;
4. call `agents_list` to discover the actual profiles configured on that
   device.

Do not assume profile names. Prefer the description returned by
`agents_list` over memorized names.

## Pick the execution mode

Default to `agent_start` for delegated coding/research work. Task duration is
usually unknown, and a persistent run survives an MCP/session interruption.

Use synchronous `agent` only for a trivially bounded task where waiting for
one MCP call is clearly appropriate. Do not choose `agent` merely because a
task description looks short.

Typical long-task flow:

1. `agent_start` → capture `run_id`;
2. poll `agent_status` at a reasonable cadence;
3. once terminal, call `agent_result`;
4. use `agent_cancel` only when the run is no longer useful;
5. use `agent_runs` to recover runs after a reconnect or when a `run_id`
   was lost.

Do not start a second duplicate run merely because a status call was
interrupted. Check `agent_runs` first.

## Timeout policy

Do **not** guess how long an agent task should take.

- Omit `timeout_seconds` by default.
- `timeout_seconds: 0` or an omitted field means no wall-clock limit.
- A no-limit `agent_start` run continues until the harness finishes or the
  caller explicitly uses `agent_cancel`.
- Set a positive timeout only when the user explicitly requested a deadline,
  execution budget, or other hard wall-clock limit.
- Do not convert phrases like "long task", "deep analysis", or "write the
  documentation" into an invented timeout.

Use `agent_status` and progress events to decide whether a run is healthy;
lack of a guessed deadline is not a reason to start a duplicate run.

## Choosing a profile

Always choose from `agents_list`.

Match the returned profile description to the task, for example:

- fast repository exploration → an exploration/read-oriented profile;
- hard reasoning or architecture → a reasoning-heavy profile;
- implementation → a coding/deep implementation profile;
- UI/frontend → a visual/frontend profile;
- documentation → a writing/documentation profile.

Only override `model` or `thinking` when the user asked for it or the task
has a clear requirement. Otherwise preserve the selected profile's defaults.

A model string returned by `agents_list` may already encode an effort suffix
(for example `provider/model:max`). Do not rewrite it unless the user asks
for a different model or effort.

## Permission flow

Agent execution can be permission-gated by the local Client.

If `agent` or `agent_start` reports a permission request:

1. capture its `request_id`;
2. use `permission_grant` with `once`, `always`, or `deny`;
3. retry the same Agent call after approval.

Do not assume an Agent permission also grants `bash`; permissions are
tool-specific.

## Writing a good delegated task

Include:

- the exact goal;
- the workspace context;
- the relevant files/modules or symptoms if known;
- whether the subagent may edit files or must stay read-only;
- required tests/verification;
- constraints such as model/client/thinking when the user specified them;
- the expected final output.

Bad:

```text
fix the bug
```

Better:

```text
In this workspace, inspect the CodeBridge Agent permission flow. Find why a
long-running run can survive an MCP disconnect, verify the relevant tests, and
report the exact files and behavior. Do not modify files.
```

## Result handling

Treat a completed subagent result as evidence, not as automatic truth.

For consequential code changes:

- inspect the changed files or diff;
- run the relevant tests;
- use CodeBridge write/rollback primitives rather than inventing an untracked
  recovery mechanism;
- if a run failed, inspect its error and progress before retrying.

A terminal status can be `completed`, `failed`, `cancelled`, `timeout`,
or `interrupted`.
