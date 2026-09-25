---
name: codebridge-subagent
description: Delegate self-contained coding and investigation tasks to local subagents (omp by default; opencode or codex when a profile pins them) on the user's machine. Use when a task is too big for inline tool calls — multi-file exploration, analysis, or focused implementation work.
---

# Delegating to local subagents

The `agent` tool spawns a local coding agent inside one workspace — `omp`
(oh-my-pi) by default, `opencode` or `codex` when the selected profile pins
one. The subagent shares no conversation context with you — write
**complete, self-contained task descriptions**.

## Workflow

1. Call `agents_list` to see available profiles with their descriptions,
   models, and reasoning efforts.
2. Pick a profile for the job:
   - `explore` — fast read-only code exploration
   - `ultrabrain` — hard logic-heavy problems
   - `visual-engineering` — frontend/UI work
   - `writing` — documentation and prose
   - `deep-high` — long autonomous implementation
3. Call `agent` with `workspace` + `task` + `agent: <profile>`.
4. The first call may return a **permission request id** — resolve with
   `permission_grant` (decision `once` or `always`), then retry.

## Task descriptions that work

Bad: "fix the bug"
Good: "In the workspace, the function runBash in internal/agentops rejects
valid commands when the allowlist is empty. Read the file, find the bug,
and reply with the corrected function body only."

Include: the goal, which files/areas to look at, what to return, and any
constraints (read-only, no modification, etc.).

## Reading results

The result carries `status` (completed/failed/timeout), the subagent's
final output, and elapsed time. If it failed, rephrase the task more
concretely rather than immediately retrying.
