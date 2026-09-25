# Local Client configuration

The Client can be configured without putting workspace paths into a service definition or shell profile.

## Configuration file

Default path:

```text
~/.config/codebridge/client.json
```

Example:

```json
{
  "manager_host": "codebridge.example.com:8081",
  "device_id": "mbp-m1",
  "device_name": "MacBook Pro",
  "workspaces": {
    "pms": "/Users/me/code/pms",
    "portal": "/Users/me/code/portal"
  },
  "writable_workspaces": ["pms"]
}
```

A copy is available as `client.example.json`.

The JSON file is intentionally **non-secret**. Do not put enrollment codes, device credentials, OAuth tokens, or admin tokens in it.

The device credential remains in:

```text
~/.config/codebridge/credentials.json
```

with mode `0600`.

Unknown keys are ignored silently: a field that no longer exists (for example an old URL-form manager field) is not an error, it simply has no effect.

## Configuration reference

| Field | Type | Default | Also settable via |
| --- | --- | --- | --- |
| `manager_host` | string, `host[:port]` of the Manager gRPC endpoint; no scheme, no path | `127.0.0.1:8081` | `CODEBRIDGE_MANAGER_HOST`, `--manager-host` |
| `device_id` | string | hostname slug | `CODEBRIDGE_DEVICE_ID`, `--device-id` |
| `device_name` | string | hostname | `CODEBRIDGE_DEVICE_NAME`, `--device-name` |
| `access_mode` | `"full"` or `"workspaces"` | `workspaces` | local UI |
| `workspaces` | object, logical name → absolute local path | none; at least one workspace is required | `CODEBRIDGE_WORKSPACES`, `--workspaces` |
| `writable_workspaces` | array of workspace names | empty (write mode off) | `CODEBRIDGE_WRITABLE_WORKSPACES`, `--writable-workspaces` |
| `enabled_tools` | array of built-in MCP tool names, or `["*"]` | empty → Manager's default core set | local UI |
| `disabled_tools` | array of built-in MCP tool names | empty | local UI |
| `custom_tools` | array of `{name, description, tool, args}` | empty | local UI |
| `subagent_profiles` | array of profile objects | empty | local UI |
| `credential_file` | path | `~/.config/codebridge/credentials.json` | `CODEBRIDGE_CREDENTIAL_FILE`, `--credential-file` |
| `allow_sensitive_files` | bool | `false` | `CODEBRIDGE_ALLOW_SENSITIVE_FILES`, `--allow-sensitive-files` |
| `enable_lsp` | bool | `false` | `CODEBRIDGE_ENABLE_LSP`, `--enable-lsp` |
| `bash_allowlist` | array of strings | empty; legacy, no longer consulted | — |
| `permissions` | array of permission rules | empty (every command asks) | `CODEBRIDGE_BASH_PERMISSIONS=full\|deny` appends a catch-all rule |
| `checkpoint_dir` | path to the local state directory | `$XDG_CACHE_HOME/codebridge` (`~/Library/Caches/codebridge` on macOS, `~/.cache/codebridge` elsewhere) | `CODEBRIDGE_STATE_DIR` |

`manager_host` is validated: a URL (`scheme://…`) or a value containing `/` is rejected, and the Client keeps retrying with the last good configuration. A bare host without a port gets port `8081`. A dial target that is a name containing a dot (and not `.local` or `.internal`) uses TLS with public CA verification; IP, loopback, and LAN targets stay plaintext. `CODEBRIDGE_GRPC_TARGET` overrides the dial target.

`checkpoint_dir` is a literal path — `~` is not expanded there. `credential_file` does expand a leading `~/`.

`bash_allowlist` is kept in the file for compatibility with older configurations; bash is now authorized by `permissions` (see below).

## Configuration precedence

For normal fields:

```text
CLI flag
  > environment variable
  > client.json
  > built-in default
```

Workspace behavior is:

- `--workspaces` overrides everything;
- otherwise `CODEBRIDGE_WORKSPACES` overrides the JSON map;
- otherwise `client.json.workspaces` is used.

Custom config path:

```bash
codebridge-client --config /path/to/client.json
```

or:

```bash
export CODEBRIDGE_CONFIG=/path/to/client.json
codebridge-client
```

An explicitly configured file must exist. The default `~/.config/codebridge/client.json` is optional so environment/flag-only operation remains supported.

## Workspaces and access mode

A workspace is a logical name plus an absolute local path. Only the logical name and workspace-relative paths ever leave the machine; the physical root is never advertised or echoed in errors. Paths are resolved through symlinks and containment is re-checked, so `..` and symlink escapes are rejected.

`access_mode` is a local convenience switch:

- `workspaces` (default) exposes only the workspaces listed in the file;
- `full` keeps the workspace named `host` (the whole home tree) registered as well. Saving from the local UI in `workspaces` mode drops that entry.

The mode itself does not grant anything: what a device exposes is exactly the `workspaces` map, and what it may write is exactly `writable_workspaces`.

Workspace names must be unique, at most 128 bytes, and the paths must be existing directories. A `writable_workspaces` entry that is not a configured workspace makes the configuration invalid.

## Write mode and checkpoints

`writable_workspaces` is the local opt-in for `write`, `edit`, `apply_patch` and `rollback_patch`. A remote MCP caller can never enable it — the client advertises `writable` per workspace from its own configuration. Any ordinary directory works; write mode does not require a git repository.

Writes are two-phase: the first call must use `preview: true` (the response contains a unified diff and the per-file plan, nothing is written), then the same call is repeated with `confirm: true`. Bounds:

- at most 20 files per patch;
- at most 512 KiB of new content per file, 2 MiB total;
- UTF-8 text only — binary content is rejected;
- one edit per path, `create` requires the file to be absent, `replace` requires `old_text` to match exactly once, `delete` requires `old_text` to equal the whole current content;
- absolute paths, `..` escapes, and sensitive paths are always refused, even with `allow_sensitive_files: true`.

Every applied patch snapshots the affected files into a content-addressed local store first and returns a `checkpoint_id`. `rollback_patch` with that id restores the pre-patch content and removes files the patch created. The most recent 20 checkpoints are retained; older ones and their blobs are pruned.

Checkpoint metadata and blobs live under `checkpoint_dir` (`checkpoints/` and `checkpoints/blobs/`); the symbol index lives under `index/` (override with `CODEBRIDGE_INDEX_DIR`). Both stay on the agent machine and are never uploaded.

## Sensitive file protection

By default the Client blocks source-content access to common credential and secret locations even when they are inside an allowed workspace. This includes:

- `.git`, `.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`, `.docker`, and `.secrets`;
- `.env` and environment-specific `.env.*` files, while example/sample/template/dist variants remain readable;
- common private-key and keystore formats such as `.pem`, `.key`, `.p12`, `.pfx`, `.jks`, and `.keystore`;
- Terraform state and variable files (`.tfstate`, `.tfvars`);
- `.envrc`, `.npmrc`, `.pypirc`, `.netrc`, `.git-credentials`, and the `id_rsa`/`id_dsa`/`id_ecdsa`/`id_ed25519` key files.

A workspace whose own root directory name is sensitive cannot be used at all.

The protection is enforced on the file tools: `read` fails, `list` omits matching entries, and the write tools refuse matching paths unconditionally. It is **not** a sandbox for `bash` or `agent`: those run as your local user, so they can reach anything your account can reach. Use permission rules to keep them closed (see below), and keep the sensitive-file policy on.

For an exceptional local workflow, protection can be disabled explicitly:

```json
{
  "allow_sensitive_files": true
}
```

or:

```bash
CODEBRIDGE_ALLOW_SENSITIVE_FILES=true codebridge-client
```

or with `--allow-sensitive-files`. The Client prints a warning when this protection is disabled. Keep the default `false` for normal ChatGPT/MCP use. Sensitive paths stay unwritable even when it is `true`.

## Bash and permission rules

`bash` runs a command with the workspace root as working directory: 30 s timeout, 256 KiB combined output cap, no TTY and no interactive input, `TERM=dumb`, `PAGER=cat`, plus `GIT_DISCOVERY_ACROSS_FILESYSTEM=1` and a git `safe.directory` override so git commands work in read-only mounts.

Every `bash` command and every `agent` harness invocation is checked against `permissions`:

```json
{
  "permissions": [
    { "effect": "allow", "tool": "bash", "pattern": "git status*", "reason": "read-only git" },
    { "effect": "allow", "tool": "bash", "pattern": "git", "reason": "read-only git" },
    { "effect": "deny", "tool": "bash", "pattern": "rm", "reason": "never" },
    { "effect": "allow", "tool": "agent", "pattern": "omp", "reason": "local subagent harness" }
  ]
}
```

- `effect` is `allow` or `deny`; any other value behaves as `allow`.
- `tool` is the tool the rule applies to (`bash`, `agent`); an empty value matches every tool.
- `reason` is free text carried with the rule for operators reading the file later.
- `pattern` is matched against the command line: `""` or `"*"` matches everything, a bare word (`git`) matches commands whose first word is that word, and a trailing `*` (`git status*`) matches any command starting with that prefix.
- For `agent`, the "command" is the harness binary name (`omp`, `opencode`, `codex`).
- `deny` beats `allow` regardless of rule order.
- a call no rule matches is `ask`: it is parked and the tool returns a permission request id instead of running.

The caller resolves a parked request with the `permission_grant` tool:

| `decision` | Effect |
| --- | --- |
| `once` | approves this single execution; retry the blocked call |
| `always` | persists an `allow` rule for the command's first word into this config file, then retry |
| `deny` | drops the request |

Requests expire after 5 minutes. `CODEBRIDGE_BASH_PERMISSIONS=full` appends an `allow`/`*` rule and `CODEBRIDGE_BASH_PERMISSIONS=deny` appends a `deny`/`*` rule for `bash`, without editing the config file.

## Subagent profiles

`subagent_profiles` configures the local coding-agent harnesses the `agent` tool can run. Profiles are listed to callers by `agents_list` and selected by name.

```json
{
  "subagent_profiles": [
    {
      "name": "explore",
      "description": "Read-only codebase exploration",
      "client": "omp",
      "tab": "omp",
      "model": "provider/model",
      "thinking": "low",
      "timeout_seconds": 300,
      "extra_args": ["--print-logs=false"]
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `name` | profile selector; also used as the harness agent name when `agent` is empty |
| `description` | surfaced to AI clients through `agents_list` |
| `client` | `omp` (default), `opencode`, or `codex` |
| `agent` | opencode agent name or codex profile (`--profile`); **ignored by omp**, which picks its agent from its own configuration |
| `model` | model override in `provider/model` form; `provider/model:effort` also selects the reasoning effort |
| `thinking` | `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max` |
| `timeout_seconds` | wall-clock budget; `0` means no limit |
| `extra_args` | extra CLI flags appended verbatim before the task |
| `tab` | configuration-UI grouping only; it never changes how the profile runs |

The harnesses are run as installed binaries (`omp -p --mode json …`, `opencode run …`, `codex exec …`) inside the workspace root, at most two at a time, and are gated by the `agent` permission rules. A per-call `model`, `thinking`, or `timeout_seconds` narrows the profile; the profile fills in whatever the call leaves empty.

### Subagent tabs

The configuration UI groups profiles into tabs. A tab is a display name bound to one harness, stored in `subagent_tabs`:

```json
{
  "subagent_tabs": [
    { "name": "omp", "client": "omp" },
    { "name": "omp-deep", "client": "omp", "locked": true },
    { "name": "codex", "client": "codex" }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `name` | unique tab name, shown in the UI |
| `client` | the harness every profile in this tab runs on |
| `locked` | freezes the tab's harness binding in the UI |

Several tabs may share one harness, so separate model/effort sets can coexist for the same tool. A tab is pure UI metadata: the harness a profile runs on is always its own `client`, so `agents_list` and the `agent` tool behave the same whether or not tabs are configured.

Editing rules in the UI:

- profiles are created inside the active tab and are stamped with that tab's `client` and `tab` on save;
- a locked tab's harness select is disabled, so the binding cannot be changed by accident;
- deleting a tab deletes its profiles with it, and the last tab cannot be deleted;
- loading is harness-preserving: a profile whose saved `tab` is missing falls back to the first tab with the same `client`, and only then creates a new tab, so a profile never silently switches harness.

If `subagent_tabs` is absent, the UI starts with one tab per harness (`omp`, `opencode`, `codex`) and routes each existing profile to the first tab matching its `client`.

## Tool policy

`enabled_tools`, `disabled_tools`, and `custom_tools` travel with device registration and are applied by the Manager when it answers `tools/list`:

- with no policy configured, the Manager exposes its default core set (`list_devices`, `list_workspaces`, `list`, `read`, `write`, `edit`, `bash`, `agents_list`, `agent`, `permission_grant`); `apply_patch` and `rollback_patch` are not in that default set and must be enabled explicitly;
- `enabled_tools` is an explicit allowlist — a listed tool is exposed, `"*"` exposes every built-in tool;
- `disabled_tools` hides built-in tools; a built-in tool is hidden as soon as one registered device of the account hides it, and a device with no tool policy hides everything outside the default core set;
- a built-in tool that is not enabled is hidden from callers even if the client can run it.

The discovery, symbol, and git helper tools of earlier releases are no longer advertised by the Manager. `read` returns at most 256 KiB per call (`max_bytes`), and `list` refuses a directory with more than 1000 entries.

`custom_tools` adds operator-defined MCP methods that wrap one built-in tool with preset arguments:

```json
{
  "custom_tools": [
    {
      "name": "pms_file",
      "description": "Read one file from the pms workspace",
      "tool": "read",
      "args": { "device_id": "mbp-m1", "workspace": "pms" }
    }
  ]
}
```

`name` must be at most 64 characters and must not shadow a built-in tool name; `tool` must be a built-in tool name. Caller-supplied arguments override the preset ones, except `device_id` and `workspace`, which stay pinned. A wrapper advertises the output schema of the built-in tool it forwards to, so its structured results are described exactly like that tool's.

`enable_lsp` opts into locally installed language servers (`gopls`, `typescript-language-server --stdio`, `jdtls`) on the agent machine. Only the legacy symbol tools consult them, and the Manager no longer advertises those tools, so the setting has no effect on the current tool surface.

## Local configuration UI

The Client serves a local configuration UI on `http://127.0.0.1:8190` (`CODEBRIDGE_UI_ADDR`, empty disables it). It edits the JSON file — workspaces, write opt-in, tool policy, permission rules, subagent profiles, enrollment code — and triggers a reconnect. A setting pinned by environment or flag still wins at runtime even after the UI rewrites the file, and the UI only ever shows the credential file path, never the credential itself.

## First enrollment

Keep the long-lived configuration in `client.json`, but provide the one-time enrollment code separately:

```bash
CODEBRIDGE_ENROLL_CODE='enr_...' codebridge-client
```

After successful enrollment the returned device credential is persisted automatically. Future starts need no enrollment environment variable.

## macOS launchd

For a user-level background service:

1. install `codebridge-client` at a stable absolute path;
2. create `~/.config/codebridge/client.json`;
3. copy `deploy/launchd/io.github.edynasty.codebridge-client.plist.example` to:
   `~/Library/LaunchAgents/io.github.edynasty.codebridge-client.plist`;
4. replace `__CODEBRIDGE_CLIENT_PATH__` with the absolute client binary path;
5. load it:

```bash
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/io.github.edynasty.codebridge-client.plist
launchctl kickstart -k gui/$(id -u)/io.github.edynasty.codebridge-client
```

Status:

```bash
launchctl print gui/$(id -u)/io.github.edynasty.codebridge-client
```

To unload:

```bash
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/io.github.edynasty.codebridge-client.plist
```

The Client reconnect loop handles temporary Manager/network outages.

## Linux systemd user service

Install the binary:

```bash
mkdir -p ~/.local/bin ~/.config/codebridge
install -m 0755 codebridge-client ~/.local/bin/codebridge-client
```

Copy the unit:

```bash
mkdir -p ~/.config/systemd/user
cp deploy/systemd/codebridge-client.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now codebridge-client
```

Inspect:

```bash
systemctl --user status codebridge-client
journalctl --user -u codebridge-client -f
```

For a user service that should survive logout, configure user lingering according to your Linux distribution's policy.

## Security notes

Use an ordinary user account rather than root. Whatever the Client can reach, a permitted `bash` command or subagent can reach too, so:

- keep `writable_workspaces` to the repositories you actually want written and keep `allow_sensitive_files` at `false`;
- keep `permissions` closed by default and add narrow `allow` patterns only for commands you would run yourself;
- a background service does not widen filesystem access beyond the workspace roots in `client.json`, but it does keep the write opt-in and the permission rules in force for as long as it runs.

The Client requires `bash` on the host for the `bash` tool, and `git` only if you want git commands through it. Optional local tooling: `rg` for the legacy text-search path, `gopls` / `typescript-language-server` / `jdtls` for `enable_lsp`, and the `omp` / `opencode` / `codex` binaries for `subagent_profiles`.
