---
name: codebridge-ops
description: Operate the user's machines through CodeBridge — discover devices/workspaces, inspect files, understand local tool policy, and run permission-gated bash commands. Use when the user asks to inspect, search, diagnose, or run commands on their own computer. For file modifications use the codebridge-write skill; for delegated coding tasks use codebridge-subagent.
---

# Operating the user's machine

CodeBridge connects ChatGPT to the user's own hardware. Remote callers work with
**logical workspace names and workspace-relative paths**. Never invent or request
a physical absolute path when a workspace-relative path can be used.

## Start with capability discovery

1. Call `list_devices`.
2. Pick the correct online device.
3. Inspect its advertised:
   - `workspaces`;
   - each workspace's `writable` flag;
   - `tool_policy.enabled_tools`, `disabled_tools`, and any
     `custom_tools`.
4. Call `list_workspaces` when you need a fresh workspace view or the device
   result is ambiguous.

Do not assume every device exposes every built-in tool. The local Client is the
authority for tool policy.

## Reading and browsing

Prefer the smallest primitive that answers the question:

- `list` — list one workspace-relative directory.
- `read` — read one bounded text file.
- `bash` — use only when a command genuinely improves the task, such as
  `git status`, `git diff`, `rg`, `find`, build/test commands, or
  project-specific CLIs.

For repository exploration:

1. list the relevant directory;
2. read the smallest useful files;
3. use permission-gated `bash` for searches or commands that would otherwise
   require many file reads.

Do not use `bash` merely to bypass a dedicated CodeBridge primitive.

## Permission-gated bash

`bash` executes in the selected workspace root, with bounded output and a
timeout.

A command can return a permission request instead of executing. When that
happens:

1. read the returned `request_id`;
2. call `permission_grant` with:
   - `once` for a single execution;
   - `always` only when the user clearly wants the command pattern persisted;
   - `deny` when the command should not run;
3. retry the exact same `bash` call after an approval.

Never silently broaden a command just because a narrower one was denied.

## Safety and privacy

- Treat sensitive-path protections enforced by the Client as authoritative.
- Do not ask the user to disable sensitive-file protection merely to make a
  task easier.
- Do not expose or echo physical workspace roots when logical workspace names
  suffice.
- Prefer read-only inspection unless the user's goal requires a modification.
- If the device is offline or unknown, refresh with `list_devices` before
  retrying.

## Routing to other CodeBridge skills

Use **codebridge-write** when the task requires creating, replacing, deleting,
or rolling back files.

Use **codebridge-subagent** whenever a local coding agent should investigate
or implement autonomously. Prefer its persistent `agent_start` workflow when
the task duration is unknown; do not invent a wall-clock timeout just because
the task looks small.
