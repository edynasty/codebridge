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
  "allow_sensitive_files": false,
  "workspaces": {
    "pms": "/Users/me/code/pms",
    "portal": "/Users/me/code/portal"
  }
}
```

A copy is available as `client.example.json`.

The JSON file is intentionally **non-secret**. Do not put enrollment codes, device credentials, OAuth tokens, or admin tokens in it.

The device credential remains in:

```text
~/.config/codebridge/credentials.json
```

with mode `0600`.

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

## Sensitive file protection

By default the Client blocks source-content access to common credential and secret locations even when they are inside an allowed workspace. This includes:

- `.git`, `.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`, `.docker`, and `.secrets`;
- `.env` and environment-specific `.env.*` files, while example/sample/template/dist variants remain readable;
- common private-key and keystore formats such as `.pem`, `.key`, `.p12`, `.pfx`, `.jks`, and `.keystore`;
- Terraform state and variable files;
- common credential files such as `.npmrc`, `.pypirc`, `.netrc`, and `.git-credentials`.

The protection applies to direct reads, directory/file discovery, code search, and `git_diff`. A full `git_diff` is generated only after sensitive changed files have been removed from the path list.

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

or with `--allow-sensitive-files`. The Client prints a warning when this protection is disabled. Keep the default `false` for normal ChatGPT/MCP use.

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

The Client remains read-only. A background service does not widen filesystem access beyond the workspace roots in `client.json`.

Use an ordinary user account rather than root. The process only needs read access to the configured repositories and execution access to `git` / optional `rg`.
