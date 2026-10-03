// Command codebridged is the CodeBridge V2 daemon: the per-user authority that
// owns the MCP ingress, the Host IPC listener, tunnel-client supervision and
// the runtime store. Phase 0 runs it in the foreground or under a per-user
// LaunchAgent.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/edynasty/codebridge/internal/approvalspike"
	"github.com/edynasty/codebridge/internal/daemon"
	"github.com/edynasty/codebridge/internal/hostipc"
)

// version is overridden at link time: -X main.version=...
var version = daemon.Version

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "run":
		os.Exit(runDaemon(args))
	case "smoke":
		os.Exit(runSmoke(args))
	case "probe":
		os.Exit(runProbe(args))
	case "ctl":
		os.Exit(runCtl(args))
	case "tunnel":
		os.Exit(runTunnel(args))
	case "version", "-version", "--version", "-v":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "codebridged: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `codebridged — CodeBridge V2 daemon (Phase 0)

Usage:
  codebridged run [flags]                 run the daemon in the foreground (launchd authority)
  codebridged smoke native-host [flags]   run the daemon's fixed harmless host checks, print the JSON report
  codebridged probe ingress [flags]       call the MCP ingress over its Unix socket (bearer-auth smoke)
  codebridged ctl health [flags]          ask a RUNNING daemon over Host IPC (role diagnostics)
  codebridged ctl smoke [flags]           ask a RUNNING daemon to run its native-host smoke
  codebridged ctl probe [flags]           ask a RUNNING daemon to run a fixed harness or native probe
  codebridged ctl call -method M [flags]  ask a RUNNING daemon any Host IPC method (debugging)
  codebridged tunnel doctor [flags]       print the resolved tunnel-client configuration (no secrets)
  codebridged version

Run "codebridged <command> -h" for the flags of each command.
`)
}

// newLogger builds the daemon logger. Lines are stable key=value text.
func newLogger(logFile string) (*slog.Logger, func(), error) {
	w := os.Stderr
	cleanup := func() {}
	if strings.TrimSpace(logFile) != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			return nil, cleanup, fmt.Errorf("create log dir: %w", err)
		}
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, cleanup, fmt.Errorf("open log file: %w", err)
		}
		w = f
		cleanup = func() { _ = f.Close() }
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})), cleanup, nil
}

// configWithEnv builds the default config, applies environment overrides, then
// parses flags so explicit flags win over the environment.
func configWithEnv(name string, args []string, extra func(*flag.FlagSet)) (daemon.Config, *flag.FlagSet, error) {
	cfg := daemon.DefaultConfig()
	cfg.ApplyEnv()
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg.RegisterFlags(fs)
	if extra != nil {
		extra(fs)
	}
	if err := fs.Parse(args); err != nil {
		return cfg, fs, err
	}
	// Re-derive the socket paths from -run-dir unless they were set explicitly
	// by a flag or the environment.
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	for key, envName := range daemon.DerivedFlagEnv() {
		if envName != "" && strings.TrimSpace(os.Getenv(envName)) != "" {
			explicit[key] = true
		}
	}
	cfg.DerivePaths(explicit)
	cfg.Version = version
	return cfg, fs, cfg.Validate()
}

func runDaemon(args []string) int {
	cfg, _, err := configWithEnv("run", args, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged run: %v\n", err)
		return 2
	}
	logger, cleanup, err := newLogger(cfg.LogFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged run: %v\n", err)
		return 2
	}
	defer cleanup()
	if cfg.WidgetSpike {
		daemon.SetWidgetSpikeRegistrar(approvalspike.Register)
		logger.Info("widget spike enabled", "tools", "phase0_approval,phase0_approval_decide,phase0_signal")
	}
	if !cfg.NoStore {
		store, err := daemon.OpenRuntimeStore(cfg.RuntimeDB)
		if err != nil {
			logger.Error("runtime store unavailable", "path", cfg.RuntimeDB, "error", err.Error())
			return 1
		}
		cfg.Store = store
		logger.Info("runtime store opened", "path", cfg.RuntimeDB, "schema_version", store.SchemaVersion())
	}
	d, err := daemon.New(cfg, logger)
	if err != nil {
		logger.Error("startup failed", "error", err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := d.Run(ctx); err != nil {
		logger.Error("daemon exited with error", "error", err.Error())
		return 1
	}
	return 0
}

func runSmoke(args []string) int {
	if len(args) == 0 || args[0] != "native-host" {
		fmt.Fprintln(os.Stderr, "codebridged smoke: expected subcommand \"native-host\"")
		return 2
	}
	var only string
	cfg, _, err := configWithEnv("smoke native-host", args[1:], func(fs *flag.FlagSet) {
		fs.StringVar(&only, "only", "", "comma-separated check names to run (default: all)")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged smoke: %v\n", err)
		return 2
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	d, err := daemon.New(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged smoke: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := d.RunNativeHostSmoke(ctx, splitList(only))
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged smoke: %v\n", err)
		return 1
	}
	return printJSON(report)
}

func runProbe(args []string) int {
	if len(args) == 0 || args[0] != "ingress" {
		fmt.Fprintln(os.Stderr, "codebridged probe: expected subcommand \"ingress\"")
		return 2
	}
	cfg := daemon.DefaultConfig()
	cfg.ApplyEnv()
	fs := flag.NewFlagSet("probe ingress", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socket := fs.String("socket", cfg.MCPSocket, "MCP ingress Unix socket")
	tokenFile := fs.String("token-file", cfg.Tunnel.TokenFile, "file holding the per-launch bearer secret (0600)")
	tool := fs.String("tool", "ping", "tool to call: ping|health")
	expect := fs.String("expect", "authorized", "expected outcome: authorized|unauthorized")
	noToken := fs.Bool("no-token", false, "send no Authorization header (local_mcp path)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	token := ""
	if !*noToken {
		var err error
		token, err = daemon.ReadTokenFile(*tokenFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codebridged probe: read token file: %v\n", err)
			return 2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := daemon.ProbeMCP(ctx, *socket, token, *tool, nil)
	observed := "authorized"
	if err != nil {
		observed = "unauthorized"
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "401") && !strings.Contains(lower, "unauthorized") {
			fmt.Fprintf(os.Stderr, "codebridged probe: %v\n", err)
			return 1
		}
	}
	if observed != *expect {
		fmt.Fprintf(os.Stderr, "codebridged probe: expected %s but observed %s\n", *expect, observed)
		return 1
	}
	if err != nil {
		return printJSON(map[string]any{"tool": *tool, "observed": observed, "expected": *expect})
	}
	return printJSON(map[string]any{
		"tool":     *tool,
		"observed": observed,
		"expected": *expect,
		"result":   json.RawMessage(res.Structured),
	})
}

func runCtl(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "codebridged ctl: expected health|smoke|probe|call")
		return 2
	}
	sub := args[0]
	cfg := daemon.DefaultConfig()
	cfg.ApplyEnv()
	fs := flag.NewFlagSet("ctl "+sub, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socket := fs.String("host-socket", cfg.HostSocket, "running daemon's Host IPC socket")
	timeout := fs.Duration("timeout", 120*time.Second, "request timeout")
	role := fs.String("role", "diagnostics", "Host IPC role to connect with: diagnostics|app|harness")
	only := fs.String("only", "", "comma-separated smoke checks (ctl smoke)")
	probe := fs.String("probe", "permissions", "probe name (ctl probe)")
	timeoutMS := fs.Int("timeout-ms", 0, "probe timeout in milliseconds (ctl probe)")
	method := fs.String("method", "", "Host IPC method to invoke (ctl call; debugging)")
	params := fs.String("params", "", "JSON object of params for -method (ctl call)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client, err := hostipc.Dial(ctx, hostipc.ClientOptions{
		SocketPath:   *socket,
		Role:         hostipc.Role(*role),
		AppVersion:   version,
		Capabilities: []hostipc.Capability{hostipc.CapHost, hostipc.CapNativeHostSmoke},
		Timeout:      *timeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged ctl: connect %s: %v\n", *socket, err)
		return 1
	}
	defer func() { _ = client.Close() }()
	hello := client.Hello()
	fmt.Fprintf(os.Stderr, "handshake: role=%s class=%s signature_verified=%t protocol=%d.%d daemon=%s\n",
		hello.Role, hostipc.Role(hello.Role).CallerClass(), hello.SignatureVerified,
		hello.Protocol.Major, hello.Protocol.Minor, hello.DaemonVersion)

	switch sub {
	case "health", "ping":
		var out json.RawMessage
		if herr := client.Call(ctx, hostipc.MethodHealth, nil, &out); herr != nil {
			fmt.Fprintf(os.Stderr, "ctl %s: %v\n", sub, herr)
			return 1
		}
		return printJSON(out)
	case "smoke":
		var params map[string]any
		if list := splitList(*only); len(list) > 0 {
			params = map[string]any{"only": list}
		}
		var out json.RawMessage
		if herr := client.Call(ctx, hostipc.MethodNativeHostSmoke, params, &out); herr != nil {
			fmt.Fprintf(os.Stderr, "ctl smoke: %v\n", herr)
			return 1
		}
		return printJSON(out)
	case "probe":
		params := map[string]any{"probe": *probe}
		if *timeoutMS > 0 {
			params["timeout_ms"] = *timeoutMS
		}
		var out json.RawMessage
		if herr := client.Call(ctx, hostipc.MethodPhase0Probe, params, &out); herr != nil {
			fmt.Fprintf(os.Stderr, "ctl probe: %v\n", herr)
			return 1
		}
		return printJSON(out)
	case "call":
		if *method == "" {
			fmt.Fprintln(os.Stderr, "codebridged ctl call: -method is required")
			return 2
		}
		var in json.RawMessage
		if *params != "" {
			if !json.Valid([]byte(*params)) {
				fmt.Fprintf(os.Stderr, "codebridged ctl call: -params is not valid JSON\n")
				return 2
			}
			in = json.RawMessage(*params)
		}
		var out json.RawMessage
		if herr := client.Call(ctx, *method, in, &out); herr != nil {
			fmt.Fprintf(os.Stderr, "ctl call %s: %v\n", *method, herr)
			return 1
		}
		return printJSON(out)
	default:
		fmt.Fprintf(os.Stderr, "codebridged ctl: unknown subcommand %q\n", sub)
		return 2
	}
}

func runTunnel(args []string) int {
	if len(args) == 0 || args[0] != "doctor" {
		fmt.Fprintln(os.Stderr, "codebridged tunnel: expected subcommand \"doctor\"")
		return 2
	}
	var execHelp bool
	cfg, _, err := configWithEnv("tunnel doctor", args[1:], func(fs *flag.FlagSet) {
		fs.BoolVar(&execHelp, "exec", false, "run <binary> --help to prove the binary and args are usable")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "codebridged tunnel doctor: %v\n", err)
		return 2
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	sup := daemon.NewTunnelSupervisor(cfg, "", logger)

	type doctorReport struct {
		Binary       string   `json:"binary"`
		BinaryExists bool     `json:"binary_exists"`
		Executable   bool     `json:"binary_executable"`
		ServerURL    string   `json:"server_url"`
		HealthSocket string   `json:"health_socket"`
		TokenEnv     string   `json:"token_env"`
		TokenFile    string   `json:"token_file"`
		Args         []string `json:"args"`
		ExecError    string   `json:"exec_error,omitempty"`
		ExecExit     *int     `json:"exec_exit_code,omitempty"`
		ExecStdout   string   `json:"exec_stdout,omitempty"`
		ExecStderr   string   `json:"exec_stderr,omitempty"`
	}
	report := doctorReport{
		Binary:       cfg.Tunnel.Binary,
		ServerURL:    sup.ServerURL(),
		HealthSocket: cfg.Tunnel.HealthSocket,
		TokenEnv:     cfg.Tunnel.TokenEnv,
		TokenFile:    cfg.Tunnel.TokenFile,
		Args:         sup.BuildArgs(),
	}
	if cfg.Tunnel.Binary != "" {
		if fi, err := os.Stat(cfg.Tunnel.Binary); err == nil {
			report.BinaryExists = true
			report.Executable = fi.Mode()&0o111 != 0 && !fi.IsDir()
		} else {
			report.ExecError = err.Error()
		}
	}
	if execHelp && report.BinaryExists && report.Executable {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cfg.Tunnel.Binary, "--help")
		cmd.Env = append(os.Environ(), cfg.Tunnel.TokenEnv+"=Bearer <redacted>")
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		code := 0
		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				code = exitErr.ExitCode()
			} else {
				report.ExecError = runErr.Error()
				code = -1
			}
		}
		report.ExecExit = &code
		report.ExecStdout = stdout.String()
		report.ExecStderr = stderr.String()
	}
	return printJSON(report)
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}
