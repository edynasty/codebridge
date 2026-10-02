# Phase 0 — native host smoke + daemon-attributed probe evidence

Owner: DaemonBaseline (daemon-side execution authority). The probe *binary* is NativeSpike's
`codebridge-probe`; this document covers what the daemon runs, with which argv, and what was
observed.

All results are **observed** on this machine unless marked **[UNTESTED]** / **[BLOCKED]**.

## 1. Two execution surfaces

| Surface | Where it runs | Who may trigger it |
| --- | --- | --- |
| `host.native_host_smoke` (Host IPC) / `codebridged ctl smoke` | **inside the long-lived `codebridged` process** | `app`, `diagnostics`, `harness` |
| `codebridged smoke native-host` (one-shot CLI) | a short-lived CLI process | local operator |
| `host.phase0_probe` (Host IPC) / `codebridged ctl probe` | fixed probe binary as a **direct child of the daemon** | `app`, `diagnostics`, debug-gated |

The smoke the parent wants ("running authority") is the first row: the command executes in the
process that launchd owns, so its cwd, environment and process tree are the daemon's.

The check argv is a compile-time constant of the daemon (`internal/daemon/smoke.go`); the only
caller input is an optional `only` selector validated against that fixed set. There is no general
argv, no shell interpolation, no stdin (`internal/daemon/exec.go`).

## 2. The fixed check set

| Name | argv |
| --- | --- |
| `shell` | `/bin/sh -c "printf codebridge-native-host"` |
| `git` | `git --version` |
| `docker` | `docker version` (Client **and Server**; earlier one-shot below used `--version`) |
| `ssh` | `ssh -V` |
| `kubectl` | `kubectl version --client` |
| `path` | `/usr/bin/which -a git` |

Each check is bounded by `-smoke-timeout` (default 10s), captured to ≤64 KiB per stream, and reported
with `argv`, `started`, `exit_code`, `timed_out`, `duration_ms`, `stdout`, `stderr`. A missing
executable is reported as `started:false` with an error rather than silently skipped.

## 3. Observed one-shot run

```
$ bin/codebridged smoke native-host
```

```json
{"daemon_pid": 88505, "cwd": "/Users/tangxingpeng/IdeaProjects/me/codebridge",
 "env": {"PATH": "…", "HOME": "/Users/tangxingpeng", "USER": "tangxingpeng"},
 "launch": {"pid": 88505, "ppid": 45286, "executable": ".../bin/codebridged",
            "run_dir": ".../CodeBridge/run", "host_socket": ".../CodeBridge/run/hostipc.sock"},
 "checks": [
   {"name":"shell","started":true,"exit_code":0,"stdout":"codebridge-native-host"},
   {"name":"git","started":true,"exit_code":0,"stdout":"git version 2.50.1 (Apple Git-155)"},
   {"name":"docker","started":true,"exit_code":0,"stdout":"Docker version 29.7.2, build a7dcaa6"},
   {"name":"ssh","started":true,"exit_code":0,"stdout":"","stderr":"OpenSSH_10.3p1, LibreSSL 3.3.6"},
   {"name":"kubectl","started":true,"exit_code":0,"stdout":"Client Version: v1.36.1\nKustomize Version: v5.8.1"},
   {"name":"path","started":true,"exit_code":0,"stdout":"/usr/bin/git"}]}
```

All six checks started and exited 0. `ssh -V` writes to stderr, captured and reported.

## 4. Observed integrated run (daemon authority)

```
$ bin/codebridged run -run-dir /tmp/cb-run -data-dir /tmp/cb-data -tunnel=false -log /tmp/cb.log &
$ bin/codebridged ctl smoke -host-socket /tmp/cb-run/hostipc.sock -only shell,git
handshake: role=diagnostics class=local_mcp signature_verified=false protocol=1.0 daemon=0.1.0-phase0
{"report": {"daemon_pid": 91465,
  "launch": {"pid": 91465, "ppid": 89654, "executable": ".../bin/codebridged",
             "run_dir": "/tmp/cb-phase0/run", "host_socket": "/tmp/cb-phase0/run/hostipc.sock"},
  "checks": [{"name":"shell","exit_code":0,"stdout":"codebridge-native-host"},
             {"name":"git","exit_code":0,"stdout":"git version 2.50.1 (Apple Git-155)"}]}}
```

The report's `daemon_pid`/`launch.pid` is the daemon's own PID, not the CLI's — the checks ran under
the daemon's authority. Under launchd the same run reports `ppid = 1`
(`docs/v2/evidence/phase0-ipc.md` §4.1).

MCP-side proof that the daemon (not the client) executed a command: the `ping` tool runs
`/usr/bin/printf codebridge-native-host` with no caller input and returns the argv, exit code and
captured stdout:

```json
{"status":"ok","caller_class":"remote_ai","native_host":{"argv":["/usr/bin/printf","codebridge-native-host"],
 "exit_code":0,"stdout":"codebridge-native-host","timed_out":false}}
```

## 5. Daemon-attributed probe (TCC attribution)

`host.phase0_probe` spawns **exactly** the binary named by `CODEBRIDGE_PHASE0_PROBE` (absolute path,
executable file) with the fixed argv `<binary> <probe> --json`, no shell, no caller argv, and returns
`{argv, exit_code, stdout, stderr, timed_out}`. Gated by `CODEBRIDGE_PHASE0_DEBUG=1`; without the
gate the method is unregistered and answers `-32601 unsupported` (observed).

Observed with a stand-in probe (the real `codebridge-probe` is NativeSpike's build):

```
$ CODEBRIDGE_PHASE0_DEBUG=1 CODEBRIDGE_PHASE0_PROBE=/tmp/cb-probe.sh bin/codebridged run … &
$ bin/codebridged ctl probe -host-socket /tmp/cb/run/hostipc.sock -probe permissions
{"argv": ["/tmp/cb-probe.sh", "permissions", "--json"], "exit_code": 0,
 "stdout": "{\"schema\":\"codebridge.phase0.probe.v1\",\"probe\":\"permissions\",\"arg\":\"--json\",…}"}
```

A caller passing `args: ["/bin/rm","-rf","/"]` still produced
`["/tmp/cb-probe.sh","permissions","--json"]`: caller args never reach the child.

The daemon is the parent, but **parent PID alone does not establish TCC attribution**. The initial stand-in above proved only the fixed debug-gated spawn path and role restriction (`app` and `diagnostics`, never `harness` or an MCP caller). The later real launchd-owned native probe and existing TCC attribution log are recorded in §7 and [TCC §7](phase0-tcc.md#7-parent-integration-f2-stop-condition).

## 6. [UNTESTED] / gaps

- Genuine signed App/daemon/harness isolation and signed protected-root grant retention remain blocked: no code-signing identity or Team ID is available.
- Actual lock/saver/sleep/wake/FUS transitions and CGEvent behavior remain unverified. Computer input is unavailable; permission/input experiments stopped at F2.

## 7. Parent real launchd native-host smoke

Assumption: the per-user daemon can reach the user's real development environment, including Docker Desktop Server, without Manager/container execution. Environment: macOS 27.0 arm64, UID 501, manually bootstrapped LaunchAgent, PID 62447, parent 1, cwd `/`, captured PATH/HOME/USER.

Steps: `launchctl bootstrap gui/501 <owned-fixture.plist>`; `bin/codebridged ctl smoke -host-socket <fixture>/run/hostipc.sock`. All fixed commands exited **0**: shell marker, git 2.50.1, **Docker Client 29.7.2 + Docker Desktop Server 4.86.0 (236216), Engine 29.7.2**, OpenSSH 10.3p1 (version on stderr), kubectl client v1.36.1, `/usr/bin/git` discovery. Exact argv/stdout/stderr/environment/cwd: [sanitized machine evidence](phase0-parent-smoke.json).

Conclusion: Task 4 **PASS**. Real Go MCP ingress also returned the native marker; that is local UDS evidence, not a real ChatGPT/Tunnel PASS. Architecture impact: no change to Native Host Default. Follow-up: signed SMAppService/TCC and real ChatGPT acceptance remain blocked; F2 attribution details are in [TCC evidence](phase0-tcc.md), not inferred from the process tree.

