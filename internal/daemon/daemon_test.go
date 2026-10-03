package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/approvalspike"
	"github.com/edynasty/codebridge/internal/hostipc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testConfig returns an isolated configuration: temp data/run dirs, no tunnel,
// no code-signing identity.
func testConfig(t *testing.T) Config {
	t.Helper()
	// A short base directory: macOS caps a Unix-socket path at ~104 bytes and
	// t.TempDir() embeds the (long) test name.
	base, err := os.MkdirTemp("", "cb")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	cfg := DefaultConfig()
	cfg.DataDir = filepath.Join(base, "data")
	cfg.RunDir = filepath.Join(base, "run")
	cfg.MCPSocket = filepath.Join(cfg.RunDir, "mcp.sock")
	cfg.HostSocket = filepath.Join(cfg.RunDir, "hostipc.sock")
	cfg.Tunnel.Enabled = false
	cfg.Tunnel.TokenFile = filepath.Join(cfg.RunDir, "tunnel-token")
	cfg.Tunnel.HealthSocket = filepath.Join(cfg.RunDir, "tunnel-health.sock")
	cfg.LogFile = ""
	cfg.SmokeTimeout = 20 * time.Second
	return cfg
}

func testDaemon(t *testing.T, mutate func(*Config)) *Daemon {
	t.Helper()
	cfg := testConfig(t)
	if mutate != nil {
		mutate(&cfg)
	}
	d, err := New(cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	return d
}

// startDaemon runs a daemon until the test ends and waits for its sockets.
func startDaemon(t *testing.T, mutate func(*Config)) *Daemon {
	t.Helper()
	d := testDaemon(t, mutate)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForPath(t, d.Config().MCPSocket)
	waitForPath(t, d.Config().HostSocket)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon.Run: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("daemon did not shut down")
		}
	})
	return d
}

func waitForPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %s did not appear", path)
}

func TestNativeHostSmokeReport(t *testing.T) {
	d := testDaemon(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := d.RunNativeHostSmoke(ctx, nil)
	if err != nil {
		t.Fatalf("RunNativeHostSmoke: %v", err)
	}
	if report.DaemonPID != os.Getpid() {
		t.Fatalf("daemon_pid = %d, want %d", report.DaemonPID, os.Getpid())
	}
	if report.Env.PATH == "" {
		t.Fatal("report PATH is empty: the smoke must show the daemon environment")
	}
	if report.Env.Cwd != report.Cwd || report.Cwd == "" {
		t.Fatalf("cwd = %q env.cwd = %q", report.Cwd, report.Env.Cwd)
	}
	byName := map[string]ExecResult{}
	for _, c := range report.Checks {
		byName[c.Name] = c
	}
	for _, want := range NativeHostChecks() {
		if _, ok := byName[want]; !ok {
			t.Fatalf("missing smoke check %q in %v", want, report.Checks)
		}
	}
	shell := byName["shell"]
	if !shell.Started || shell.ExitCode != 0 {
		t.Fatalf("shell check = %+v, want a successful run", shell)
	}
	if got := strings.TrimSpace(shell.Stdout); got != "codebridge-native-host" {
		t.Fatalf("shell stdout = %q", got)
	}
	if shell.Argv[0] != "/bin/sh" {
		t.Fatalf("shell argv = %v, want the fixed /bin/sh form", shell.Argv)
	}
	// A check whose executable is absent must be reported as not started rather
	// than silently dropped.
	for _, c := range report.Checks {
		if !c.Started && c.Err == "" {
			t.Fatalf("check %q did not start but carries no error", c.Name)
		}
	}
	if report.Launch.PID != os.Getpid() || report.Launch.HostSocket != d.Config().HostSocket {
		t.Fatalf("launch identity = %+v", report.Launch)
	}
}

func TestNativeHostSmokeUnknownCheckRejected(t *testing.T) {
	d := testDaemon(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := d.RunNativeHostSmoke(ctx, []string{"bogus"}); err == nil {
		t.Fatal("unknown smoke check was accepted")
	}
}

func TestNativeHostSmokeFilter(t *testing.T) {
	d := testDaemon(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := d.RunNativeHostSmoke(ctx, []string{"shell"})
	if err != nil {
		t.Fatalf("RunNativeHostSmoke: %v", err)
	}
	if len(report.Checks) != 1 || report.Checks[0].Name != "shell" {
		t.Fatalf("checks = %+v, want only shell", report.Checks)
	}
}

func TestPingCommandExecutesFixedCommand(t *testing.T) {
	d := testDaemon(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res := d.PingNativeHost(ctx)
	if !res.Started || res.ExitCode != 0 {
		t.Fatalf("ping result = %+v, want a successful run", res)
	}
	if got := strings.TrimSpace(res.Stdout); got != "codebridge-native-host" {
		t.Fatalf("ping stdout = %q, want the fixed marker", got)
	}
	if len(res.Argv) != 2 || res.Argv[0] != "/usr/bin/printf" {
		t.Fatalf("ping argv = %v, want the fixed daemon-owned command", res.Argv)
	}
}

func TestMCPIngressToolsAndBearerAuth(t *testing.T) {
	d := startDaemon(t, nil)
	cfg := d.Config()
	token, err := ReadTokenFile(cfg.Tunnel.TokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if token == "" {
		t.Fatal("token file is empty")
	}
	if fi, err := os.Stat(cfg.Tunnel.TokenFile); err != nil {
		t.Fatalf("stat token file: %v", err)
	} else if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file mode = %v, want 0600", perm)
	}
	if fi, err := os.Stat(cfg.MCPSocket); err != nil {
		t.Fatalf("stat mcp socket: %v", err)
	} else {
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("mcp socket mode = %v, want 0600", perm)
		}
		if fi.Mode()&os.ModeSocket == 0 {
			t.Fatalf("mcp endpoint is not a unix socket: %v", fi.Mode())
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("bearer token is remote_ai", func(t *testing.T) {
		ping, err := ProbeMCP(ctx, cfg.MCPSocket, token, "ping", nil)
		if err != nil {
			t.Fatalf("ping: %v", err)
		}
		if ping.IsError {
			t.Fatalf("ping returned an error result: %s", ping.Text)
		}
		var out pingOutput
		if err := json.Unmarshal(ping.Structured, &out); err != nil {
			t.Fatalf("decode ping output: %v (%s)", err, ping.Structured)
		}
		if out.CallerClass != string(CallerRemoteAI) {
			t.Fatalf("caller_class = %q, want remote_ai", out.CallerClass)
		}
		if out.NativeHost.ExitCode != 0 || !out.NativeHost.Started {
			t.Fatalf("native host exec = %+v, want a successful fixed command", out.NativeHost)
		}
		if strings.TrimSpace(out.NativeHost.Stdout) != "codebridge-native-host" {
			t.Fatalf("native host stdout = %q", out.NativeHost.Stdout)
		}
	})

	t.Run("no token is local_mcp", func(t *testing.T) {
		health, err := ProbeMCP(ctx, cfg.MCPSocket, "", "health", nil)
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		var out healthOutput
		if err := json.Unmarshal(health.Structured, &out); err != nil {
			t.Fatalf("decode health output: %v (%s)", err, health.Structured)
		}
		if out.CallerClass != string(CallerLocalMCP) {
			t.Fatalf("caller_class = %q, want local_mcp", out.CallerClass)
		}
		if out.Listeners.MCPTransport != "streamable-http+unix" {
			t.Fatalf("mcp transport = %q", out.Listeners.MCPTransport)
		}
		if out.DaemonPID != os.Getpid() {
			t.Fatalf("daemon_pid = %d", out.DaemonPID)
		}
	})

	t.Run("wrong token rejected", func(t *testing.T) {
		if _, err := ProbeMCP(ctx, cfg.MCPSocket, "00000000000000000000000000000000", "ping", nil); err == nil {
			t.Fatal("a wrong bearer token was accepted")
		}
	})
}

func TestHostIPCHealthAndRoleSeparation(t *testing.T) {
	d := startDaemon(t, nil)
	cfg := d.Config()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := hostipc.Dial(ctx, hostipc.ClientOptions{
		SocketPath: cfg.HostSocket,
		Role:       hostipc.RoleDiagnostics,
		AppVersion: "test",
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial host ipc: %v", err)
	}
	defer func() { _ = client.Close() }()
	if client.Hello().SignatureVerified {
		t.Fatal("diagnostics handshake must not claim signature verification")
	}

	var health map[string]any
	if herr := client.Call(ctx, hostipc.MethodHealth, nil, &health); herr != nil {
		t.Fatalf("host.health: %v", herr)
	}
	caller, _ := health["caller"].(map[string]any)
	if caller["class"] != "local_mcp" || caller["role"] != "diagnostics" {
		t.Fatalf("caller = %v, want the local_mcp diagnostics class", caller)
	}
	listeners, _ := health["listeners"].(map[string]any)
	if got := fmt.Sprint(listeners["tcp_listeners"]); got != "0" {
		t.Fatalf("tcp_listeners = %v, want 0", got)
	}
	if health["probe_enabled"] != false {
		t.Fatalf("probe_enabled = %v, want false without the debug gate", health["probe_enabled"])
	}

	// Methods the daemon issues to the app are never served to any peer: a CLI
	// role cannot reach Computer or the approval surfaces at all.
	for _, method := range []string{
		hostipc.MethodComputerDescribe, hostipc.MethodComputerObserve,
		hostipc.MethodApprovalPresent, hostipc.MethodApprovalCancel,
		hostipc.MethodNotifyPost,
	} {
		herr := client.Call(ctx, method, nil, nil)
		if !hostipc.IsProtocolError(herr, hostipc.CodeUnsupported) {
			t.Fatalf("%s: err = %v, want -32601 unsupported", method, herr)
		}
	}
	// Methods the daemon does serve but that a CLI role may not call.
	for _, method := range []string{
		hostipc.MethodRuntimeHealth, hostipc.MethodRuntimeEvents,
		hostipc.MethodRuntimeCommand, hostipc.MethodApprovalDecision,
		hostipc.MethodPrepareRestart,
	} {
		herr := client.Call(ctx, method, nil, nil)
		if !hostipc.IsProtocolError(herr, hostipc.CodeRoleForbidden) {
			t.Fatalf("%s: err = %v, want -32011 role_forbidden", method, herr)
		}
	}

	// host.prepare_restart is frozen but deliberately unimplemented in Phase 0:
	// the daemon must answer unsupported rather than fake a prepared=false.
	if registered, known := d.hostSrv.HasHandler(hostipc.MethodPrepareRestart); registered || !known {
		t.Fatalf("host.prepare_restart: registered=%t known=%t, want known-but-unregistered", registered, known)
	}

	// The debug-gated probe method is absent unless it is explicitly enabled.
	if herr := client.Call(ctx, hostipc.MethodPhase0Probe, map[string]any{"probe": "permissions"}, nil); !hostipc.IsProtocolError(herr, hostipc.CodeUnsupported) {
		t.Fatalf("host.phase0_probe: err = %v, want -32601 without the gate", herr)
	}

	// The fixed native-host smoke is available to the diagnostics role.
	var smoke map[string]any
	if herr := client.Call(ctx, hostipc.MethodNativeHostSmoke, map[string]any{"only": []string{"shell"}}, &smoke); herr != nil {
		t.Fatalf("host.native_host_smoke: %v", herr)
	}
	report, _ := smoke["report"].(map[string]any)
	checks, _ := report["checks"].([]any)
	if len(checks) != 1 {
		t.Fatalf("checks = %v, want exactly the shell check", checks)
	}
}

func TestPhase0ProbeDisabledWithoutGate(t *testing.T) {
	d := testDaemon(t, nil)
	if d.ProbeEnabled() {
		t.Fatal("probe enabled without the debug gate")
	}
	for _, probe := range []string{"permissions", "daemon-permissions", "daemon-files-folders"} {
		if _, err := d.RunPhase0Probe(context.Background(), probe, 0); err == nil {
			t.Fatalf("%s ran without the debug gate", probe)
		}
	}
}

func TestPhase0ProbeRunsFixedArgvOnly(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-probe.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"probe\":\"%s\",\"arg\":\"%s\"}\\n' \"$1\" \"$2\"\n"), 0o755); err != nil {
		t.Fatalf("write probe script: %v", err)
	}
	d := startDaemon(t, func(cfg *Config) {
		cfg.Phase0Debug = true
		cfg.Phase0ProbePath = script
	})
	cfg := d.Config()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := d.RunPhase0Probe(ctx, "permissions", 5000)
	if err != nil {
		t.Fatalf("RunPhase0Probe: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, res.Stderr)
	}
	wantArgv := []string{script, "permissions", "--json"}
	if strings.Join(res.Argv, " ") != strings.Join(wantArgv, " ") {
		t.Fatalf("argv = %v, want the fixed %v", res.Argv, wantArgv)
	}

	// Through Host IPC the caller cannot inject an executable or extra argv.
	client, err := hostipc.Dial(ctx, hostipc.ClientOptions{
		SocketPath: cfg.HostSocket,
		Role:       hostipc.RoleDiagnostics,
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial host ipc: %v", err)
	}
	defer func() { _ = client.Close() }()
	var out map[string]any
	herr := client.Call(ctx, hostipc.MethodPhase0Probe, map[string]any{
		"probe": "permissions",
		"args":  []string{"/bin/rm", "-rf", "/"},
	}, &out)
	if herr != nil {
		t.Fatalf("host.phase0_probe: %v", herr)
	}
	argv, _ := out["argv"].([]any)
	if len(argv) != 3 || argv[0] != script || argv[1] != "permissions" || argv[2] != "--json" {
		t.Fatalf("argv = %v, want the fixed probe argv regardless of caller args", argv)
	}
}

func TestTunnelArgsUseUnixSocketsAndEnvToken(t *testing.T) {
	cfg := testConfig(t)
	cfg.Tunnel.Enabled = true
	cfg.Tunnel.Binary = "/usr/local/bin/tunnel-client"
	cfg.Tunnel.TokenEnv = "CODEBRIDGE_TUNNEL_TOKEN"
	sup := NewTunnelSupervisor(cfg, "s3cret-value", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	args := sup.BuildArgs()
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"url=http://localhost/mcp,unix-socket=" + cfg.MCPSocket,
		"--health.unix-socket " + cfg.Tunnel.HealthSocket,
		"--mcp.extra-headers Authorization: env:CODEBRIDGE_TUNNEL_TOKEN",
		"--mcp.discovery-extra-headers Authorization: env:CODEBRIDGE_TUNNEL_TOKEN",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "s3cret-value") {
		t.Fatal("the bearer secret leaked into the tunnel argument vector")
	}
	env := strings.Join(sup.BuildEnv(), "\n")
	if !strings.Contains(env, "CODEBRIDGE_TUNNEL_TOKEN=Bearer s3cret-value") {
		t.Fatal("the bearer secret was not exported through the configured env var")
	}
	if h := sup.Health(); h.HealthSocket != cfg.Tunnel.HealthSocket || h.Configured != true {
		t.Fatalf("health = %+v", h)
	}
	if h := sup.Health(); strings.Contains(h.LastError, "s3cret-value") {
		t.Fatal("the bearer secret leaked into tunnel health")
	}
}

func TestHostCapabilitiesExcludeUnimplementedServices(t *testing.T) {
	d := testDaemon(t, nil)
	for _, cap := range d.hostCapabilities() {
		if cap == hostipc.CapComputer || cap == hostipc.CapApproval {
			t.Fatalf("daemon advertised %q although no service implements it", cap)
		}
	}
}

func TestRuntimeStoreWiring(t *testing.T) {
	base, err := os.MkdirTemp("", "cb")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	dbPath := filepath.Join(base, "runtime.db")
	store, err := OpenRuntimeStore(dbPath)
	if err != nil {
		t.Fatalf("OpenRuntimeStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if got := store.SchemaVersion(); got == 0 {
		t.Fatal("store schema version is zero")
	}
	if _, err := store.JournalHead(); err != nil {
		t.Fatalf("JournalHead: %v", err)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.SchemaVersion == 0 {
		t.Fatalf("stats = %+v, want the adapter to fill schema_version", stats)
	}
	// A running daemon owns the store (its shutdown closes it).
	d := startDaemon(t, func(cfg *Config) { cfg.Store = store })
	res, herr := d.handleRuntimeHealth(context.Background(), nil, nil)
	if herr != nil {
		t.Fatalf("runtime.health: %v", herr)
	}
	out, ok := res.(runtimeHealthResult)
	if !ok || !out.StoreWired || out.SchemaVersion == 0 {
		t.Fatalf("runtime.health = %#v", res)
	}
	// runtime.command is deliberately NOT implemented in Phase 0: no handler is
	// registered, so the method answers unsupported instead of faking a no-op.
	if registered, _ := d.hostSrv.HasHandler(hostipc.MethodRuntimeCommand); registered {
		t.Fatal("runtime.command must not be registered in Phase 0")
	}
	if registered, _ := d.hostSrv.HasHandler(hostipc.MethodRuntimeHealth); !registered {
		t.Fatal("runtime.health must be served once a store is wired")
	}
}

// connectTestMCP opens a Streamable HTTP MCP session against the daemon's Unix
// socket, exactly like the local probe / tunnel-client path does.
func connectTestMCP(t *testing.T, ctx context.Context, socket, token string) *mcp.ClientSession {
	t.Helper()
	auth := ""
	if token != "" {
		auth = "Bearer " + token
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "codebridged-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   "http://localhost/mcp",
		HTTPClient: dialUnixClient(socket, auth),
	}, nil)
	if err != nil {
		t.Fatalf("mcp connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func listToolNames(t *testing.T, ctx context.Context, session *mcp.ClientSession) []string {
	t.Helper()
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// TestApprovalStateSurvivesAcrossHTTPRequests is the regression for the ingress
// losing tool state: each tools/call is its own HTTP request, so an ingress that
// built a fresh MCP server per request would register a new approval Probe and
// forget the bootstrap token.
func TestApprovalStateSurvivesAcrossHTTPRequests(t *testing.T) {
	SetWidgetSpikeRegistrar(approvalspike.Register)
	t.Cleanup(func() { SetWidgetSpikeRegistrar(nil) })

	d := startDaemon(t, func(cfg *Config) {
		cfg.WidgetSpike = true
		cfg.Tunnel.Enabled = true
		cfg.Tunnel.TokenFile = filepath.Join(cfg.RunDir, "tunnel-token")
	})
	socket := d.Config().MCPSocket
	token, err := ReadTokenFile(d.Config().Tunnel.TokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// remote_ai sees the opt-in widget tools.
	remote := connectTestMCP(t, ctx, socket, token)
	remoteTools := listToolNames(t, ctx, remote)
	for _, want := range []string{"ping", "health", "phase0_approval", "phase0_approval_decide"} {
		if !slices.Contains(remoteTools, want) {
			t.Fatalf("remote_ai tools = %v, missing %q", remoteTools, want)
		}
	}

	// HTTP request 1: bootstrap the approval, token arrives in _meta only.
	bootstrap, err := remote.CallTool(ctx, &mcp.CallToolParams{Name: "phase0_approval", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("phase0_approval: %v", err)
	}
	if bootstrap.IsError {
		t.Fatalf("phase0_approval returned a tool error: %v", bootstrap.Content)
	}
	rawToken, _ := bootstrap.Meta["approval_token"].(string)
	if rawToken == "" {
		t.Fatalf("phase0_approval did not return an approval token in _meta: %#v", bootstrap.Meta)
	}
	structured, err := json.Marshal(bootstrap.StructuredContent)
	if err != nil {
		t.Fatalf("marshal bootstrap: %v", err)
	}
	var boot struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.Unmarshal(structured, &boot); err != nil || boot.ApprovalID == "" {
		t.Fatalf("bootstrap approval_id missing: %s (%v)", structured, err)
	}

	// HTTP request 2, on a SEPARATE session (the UI connection): decide with the
	// token minted by request 1. This only works if probe state is long-lived.
	decider := connectTestMCP(t, ctx, socket, token)
	decideParams := &mcp.CallToolParams{Name: "phase0_approval_decide", Arguments: map[string]any{
		"approval_id": boot.ApprovalID,
		"token":       rawToken,
		"decision":    "allow",
		"scope":       "once",
	}}
	decision, err := decider.CallTool(ctx, decideParams)
	if err != nil {
		t.Fatalf("phase0_approval_decide across requests: %v", err)
	}
	if decision.IsError {
		t.Fatalf("cross-request decision was rejected, so server state did not survive: %v", decision.Content)
	}

	// HTTP request 3: the same token must not be reusable (once/session only).
	reuse, err := decider.CallTool(ctx, decideParams)
	if err != nil {
		t.Fatalf("phase0_approval_decide reuse: %v", err)
	}
	if !reuse.IsError {
		t.Fatal("a reused approval token was accepted")
	}
	if text := contentText(reuse.Content); !strings.Contains(text, "permission_denied") {
		t.Fatalf("reuse error = %q, want permission_denied", text)
	}

	// local_mcp never sees or drives the approval surfaces.
	local := connectTestMCP(t, ctx, socket, "")
	localTools := listToolNames(t, ctx, local)
	if !slices.Contains(localTools, "ping") || !slices.Contains(localTools, "health") {
		t.Fatalf("local_mcp tools = %v, want ping and health", localTools)
	}
	for _, name := range localTools {
		if strings.HasPrefix(name, "phase0_") {
			t.Fatalf("local_mcp can see widget tool %q: %v", name, localTools)
		}
	}
}

func contentText(content []mcp.Content) string {
	var b strings.Builder
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestBrokenToolRegistrationFailsAtStartup pins the fail-fast contract: a bad
// registrar must stop the daemon at construction, never turn every ingress
// request into a recovered panic (which the client sees as a bare EOF).
func TestBrokenToolRegistrationFailsAtStartup(t *testing.T) {
	SetWidgetSpikeRegistrar(func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "broken_schema",
			InputSchema: map[string]any{"type": "string"},
		}, func(context.Context, *mcp.CallToolRequest, emptyInput) (*mcp.CallToolResult, emptyInput, error) {
			return nil, emptyInput{}, nil
		})
	})
	t.Cleanup(func() { SetWidgetSpikeRegistrar(nil) })

	cfg := testConfig(t)
	cfg.WidgetSpike = true
	defer func() {
		if recover() == nil {
			t.Fatal("daemon construction accepted a broken tool registration")
		}
	}()
	_, _ = New(cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}
