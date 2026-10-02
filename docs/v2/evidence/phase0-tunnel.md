# Phase 0 — MCP ingress + tunnel-client supervision evidence

Owner: DaemonBaseline. Covers how `codebridged` exposes MCP to the `[OI]` tunnel-client, how the
per-launch secret is handled, and what supervision was observed against the **real** tunnel-client
binary.

Observed on this machine unless marked **[UNTESTED]** / **[BLOCKED]**.

## 1. Shape

```
ChatGPT / [OI] control plane
        │  (outbound tunnel)
        ▼
tunnel-client run … ──UDS──▶ codebridged MCP ingress  ──▶ tools: ping, health
   (supervised child)          (Streamable HTTP over unix socket, stateless)
```

- The daemon serves **Streamable HTTP over a Unix socket**; it opens **no TCP listener**
  (`tcp_listeners: 0` in health; verified with `lsof -a -i` → empty).
- The tunnel-client child is pointed at it with `--mcp.server-url url=http://localhost/mcp,unix-socket=<mcp.sock>`.
- Health is exposed on `--health.unix-socket <run>/tunnel-health.sock`, never a TCP port.
- Bearer auth: the daemon mints a 32-byte random secret per launch and passes it to the child through
  the environment (`CODEBRIDGE_TUNNEL_TOKEN="Bearer <secret>"`), referenced from
  `--mcp.extra-headers` / `--mcp.discovery-extra-headers` as `Authorization: env:CODEBRIDGE_TUNNEL_TOKEN`.
  The value never appears in an argv and never in a log line (observed: `token leaked into log: False`).
  It is also written to `<run>/tunnel-token` (`0600`) so a local probe can exercise the authenticated
  path; the file is removed on shutdown.

## 2. Tool surface (parent-requested minimum)

| Tool | Behaviour |
| --- | --- |
| `ping` | runs the daemon's fixed command (`/usr/bin/printf codebridge-native-host` by default, `-ping-command` overrides) with no caller input; returns argv, exit code, stdout/stderr, and the caller class |
| `health` | pure state read: version, protocol, uptime, daemon pid, store state, tunnel state, socket paths, transport |

Both are annotated read-only, non-destructive, non-open-world. `-widget-spike` additionally registers
`internal/approvalspike` (parent-owned; off by default).

## 3. Observed: real tunnel-client under supervision

```
$ /var/folders/…/tunnel/tunnel-client run --help   # flag surface confirmed
  --mcp.server-url stringArray      (url=…,channel=…,unix-socket=…)
  --mcp.extra-headers stringArray   (Key: Value, values accept env:VAR)
  --mcp.discovery-extra-headers stringArray
  --health.unix-socket string       ("serves health over the socket instead of binding TCP")

$ bin/codebridged run … -tunnel-binary <that binary> -tunnel-max-restarts 2
```

Daemon log:

```
INFO msg="tunnel starting" binary=… subcommand=run
     server_url="url=http://localhost/mcp,unix-socket=/tmp/cb/run/mcp.sock"
     health_socket=/tmp/cb/run/tunnel-health.sock token_env=CODEBRIDGE_TUNNEL_TOKEN
INFO msg="tunnel running" pid=98793
WARN msg="tunnel exited" restarts=1 error="exit status 1"
INFO msg="tunnel running" pid=98817
WARN msg="tunnel exited" restarts=2 error="exit status 1"
```

- Process tree at runtime: `98793 98792 (tunnel-client)` — the child's **ppid is the daemon**.
- Child output shows it accepted our flags and reached its own configuration validation:
  `configure tunnel-client: tunnel ID is required; set --control-plane.tunnel-id …`. Flag parsing
  succeeded; only the (parent-supplied) tunnel credentials are missing.
- Restarts are isolated processes with a doubling backoff (2s → …), bounded by `-tunnel-max-restarts`;
  state reported as `restarting` with `restarts`, `last_error`.
- After SIGTERM: `daemon exit: 0`, `stragglers after shutdown: []`.
- Doctor (no secrets printed):

```json
{"binary":"…/tunnel-client","binary_exists":true,"binary_executable":true,
 "server_url":"url=http://localhost/mcp,unix-socket=/tmp/cb/run/mcp.sock",
 "health_socket":"/tmp/cb/run/tunnel-health.sock","token_env":"CODEBRIDGE_TUNNEL_TOKEN",
 "args":["run","--mcp.server-url","url=http://localhost/mcp,unix-socket=/tmp/cb/run/mcp.sock",
         "--health.unix-socket","/tmp/cb/run/tunnel-health.sock",
         "--mcp.extra-headers","Authorization: env:CODEBRIDGE_TUNNEL_TOKEN",
         "--mcp.discovery-extra-headers","Authorization: env:CODEBRIDGE_TUNNEL_TOKEN"],
 "exec_exit_code":0}
```

`codebridged tunnel doctor -exec` runs `<binary> --help` with a redacted token in the environment.

## 4. Observed: ingress auth matrix (the path tunnel-client takes)

```
$ bin/codebridged probe ingress -socket "$RUN/mcp.sock" -token-file "$RUN/tunnel-token" -tool ping
  → caller_class=remote_ai, native_host.exit_code=0, stdout="codebridge-native-host"
$ … -no-token -tool ping                 → caller_class=local_mcp, tool served
$ … -token-file /tmp/wrong -expect unauthorized
  → unauthorized (401 on initialize), daemon log: ingress rejected bearer token
```

The probe speaks Streamable HTTP to `http://localhost/mcp` with the transport dialing the Unix socket,
i.e. the same URL/header shape tunnel-client uses, so this exercises the real ingress path without a
TCP port.

## 5. Commands

```bash
RUN=/tmp/cb-run; DATA=/tmp/cb-data; mkdir -p "$RUN"
bin/codebridged run -run-dir "$RUN" -data-dir "$DATA" -log /tmp/cb.log \
  -tunnel-binary /abs/path/tunnel-client \
  -tunnel-extra-arg --control-plane.tunnel-id -tunnel-extra-arg tunnel_xxx \
  -tunnel-extra-arg --control-plane.api-key -tunnel-extra-arg env:CONTROL_PLANE_API_KEY &

bin/codebridged tunnel doctor -run-dir "$RUN" -tunnel-binary /abs/path/tunnel-client -exec
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -token-file "$RUN/tunnel-token" -tool ping
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -token-file "$RUN/tunnel-token" -tool health
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -no-token -tool ping
# health of the child's own surfaces (no TCP):
curl --unix-socket "$RUN/tunnel-health.sock" http://localhost/healthz
```

Flags: `-tunnel` (default true), `-tunnel-binary`, `-tunnel-subcommand` (default `run`),
`-tunnel-extra-arg` (repeatable), `-tunnel-token-env`, `-tunnel-token-file`, `-tunnel-health-socket`,
`-tunnel-max-restarts`. Env: `CODEBRIDGE_TUNNEL_BINARY`, `CODEBRIDGE_TUNNEL_TOKEN_ENV`,
`CODEBRIDGE_TUNNEL_TOKEN_FILE`, `CODEBRIDGE_DISABLE_TUNNEL=1`.

## 6. [UNTESTED] / gaps

- **Real tunnel session** to the `[OI]` control plane: needs a tunnel ID **and** an API key
  (`--control-plane.tunnel-id`, `--control-plane.api-key` via `env:…`), which are parent-supplied.
  The child has never reached `running` here — it exits at its own config validation.
- **Real ChatGPT connector end-to-end**: parent reported the ChatGPT session is logged out and
  Platform redirects to login; no E2E is claimed. `health`/`ping` are ready for that run.
- The `[OI]` MCP discovery path (`--mcp.discovery-extra-headers`) is configured but has not been
  observed against a live control plane.
- The health socket was not probed with `curl` in this session (the child never stayed up); the
  argument is passed and documented, the endpoint is the child's to expose.

## 7. Parent UDS integration and registration regression

Observed: the real launchd daemon returned MCP initialize, tools/list and fixed-command ping over its permission-0600 UDS with bearer authorization; wrong bearer returned 401, local callers saw only health/ping. `lsof` found only the two Unix listeners and no TCP/UDP sockets. A per-launch restart rotated bearer authorization (old 401/new 200) and reopened the Store. [Sanitized results](phase0-parent-smoke.json).

The first opt-in widget ingress run failed with HTTP-recovered schema-registration panic / client EOF. Fixed the malformed `jsonschema` enum tags with explicit object input schemas, and built long-lived MCP servers once per caller class. Real separate HTTP requests now retain approval tokens and enforce one-use replay rejection; local callers cannot reach the widget tools. The Go SDK ingress command then passed. This is real local UDS smoke, **not** control-plane recovery or ChatGPT disconnect proof. Task 2 remains **FAIL/BLOCKED** until authenticated ChatGPT and real tunnel credentials are available.

