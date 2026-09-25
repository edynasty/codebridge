---
name: codebridge-ops
description: Operate the user's machines through CodeBridge — discover devices and workspaces, read and browse files, and run permission-gated bash commands. Use when the user asks to inspect, search, or run commands on their own computer.
---

# Operating the user's machine

CodeBridge connects ChatGPT to the user's own hardware. All paths are
**workspace-relative**; never ask for absolute paths.

## Discovery first

Always start by identifying what you're talking to:

1. `list_devices` — machines currently online.
2. `list_workspaces` on the chosen device — logical workspaces (`host` is
   usually the whole home directory).

## Reading files

- `list` — directory contents, like `ls`. Start here before reading.
- `read` — one text file, bounded. Pass `max_bytes` when you only need a
  peek.

## Running commands with bash

`bash` runs a shell command in a workspace root with a 30s timeout and
bounded output.

- The first call to a new command may return a **permission request id**
  instead of output. That is expected: resolve it with `permission_grant`
  (`decision: once` for a single run, `always` to persist a rule, `deny`
  to reject), then **retry the same bash call**.
- Prefer the read tools over bash for plain file access; use bash for
  things like `git -C <dir> log`, `rg` searches, or `find`.
- The `host` workspace is the real home directory. Commands run as the
  user's environment; assume macOS/Linux semantics.

## Working style

- Read before you write; list before you read.
- One command at a time for bash — inspect output before the next step.
- If a tool errors with "offline or unknown", re-run `list_devices`; the
  machine may have reconnected under a different device id.
