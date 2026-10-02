// Package daemon implements the Phase 0 codebridged authority: a foreground or
// per-user-launchd process that owns the MCP ingress (Streamable HTTP over a
// Unix socket), the Host IPC listener for CodeBridge.app, the tunnel-client
// supervision and the runtime store port.
//
// Phase 0 deliberately exposes one MCP tool pair (ping, health) and no Computer
// capability; every computer.*/approval.* Host IPC method answers unsupported.
package daemon

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is the Phase 0 daemon version reported over both wires. It can be
// overridden at link time with -X main.version=...
const Version = "0.1.0-phase0"

// Env var names. Values are never logged.
const (
	EnvRunDir          = "CODEBRIDGE_RUN_DIR"
	EnvDataDir         = "CODEBRIDGE_DATA_DIR"
	EnvMCPSocket       = "CODEBRIDGE_MCP_SOCKET"
	EnvHostSocket      = "CODEBRIDGE_HOSTIPC_SOCKET"
	EnvLogFile         = "CODEBRIDGE_LOG"
	EnvAppTeamID       = "CODEBRIDGE_APP_TEAM_ID"
	EnvAppBundleID     = "CODEBRIDGE_APP_BUNDLE_ID"
	EnvTunnelBinary    = "CODEBRIDGE_TUNNEL_BINARY"
	EnvTunnelTokenEnv  = "CODEBRIDGE_TUNNEL_TOKEN_ENV"
	EnvTunnelTokenFile = "CODEBRIDGE_TUNNEL_TOKEN_FILE"
	EnvDisableTunnel   = "CODEBRIDGE_DISABLE_TUNNEL"
	EnvRuntimeDB       = "CODEBRIDGE_RUNTIME_DB"
	EnvNoStore         = "CODEBRIDGE_NO_STORE"
	EnvWidgetSpike     = "CODEBRIDGE_WIDGET_SPIKE"
	EnvPhase0Debug     = "CODEBRIDGE_PHASE0_DEBUG"
	EnvPhase0Probe     = "CODEBRIDGE_PHASE0_PROBE"
	EnvPhase0Harness   = "CODEBRIDGE_PHASE0_HARNESS"
	// DefaultTunnelTokenEnv is the environment variable the daemon fills with
	// the per-launch tunnel bearer secret for its tunnel-client child.
	DefaultTunnelTokenEnv = "CODEBRIDGE_TUNNEL_TOKEN"
)

// Config is the daemon configuration. Zero values are not valid; start from
// DefaultConfig and override.
type Config struct {
	Version       string
	DataDir       string
	RunDir        string
	MCPSocket     string
	HostSocket    string
	LogFile       string
	ProtocolMinor int

	// AppTeamID/AppBundleID configure the code requirement an "app" peer must
	// satisfy. When either is empty the app role is refused (fail closed).
	AppTeamID   string
	AppBundleID string

	// Tunnel configures supervision of the [OI] tunnel-client child.
	Tunnel TunnelConfig

	// PingCommand is the fixed, non-caller-controlled command the MCP `ping`
	// tool executes to prove real native host execution.
	PingCommand []string

	// WidgetSpike registers the opt-in Phase 0 widget/approval spike tools.
	WidgetSpike bool

	// Phase0Debug enables the fixed probe subprocess method.
	Phase0Debug bool
	// Phase0ProbePath is an absolute path to the standalone probe binary.
	Phase0ProbePath string
	// Phase0HarnessPath is an absolute path to a harness probe binary.
	Phase0HarnessPath string

	// SmokeTimeout bounds each fixed native-host smoke check.
	SmokeTimeout time.Duration
	// ProbeTimeout bounds a probe subprocess run.
	ProbeTimeout time.Duration
	// ShutdownGrace bounds graceful shutdown before the tunnel is killed.
	ShutdownGrace time.Duration

	// RuntimeDB is the durable runtime database path. Empty selects
	// DataDir/runtime.db.
	RuntimeDB string
	// NoStore disables opening the runtime store entirely (runtime.* then
	// answers unsupported).
	NoStore bool

	// Store is the runtime store port. cmd/codebridged opens it from RuntimeDB;
	// nil leaves runtime.* answering unsupported.
	Store Store
}

// TunnelConfig configures the tunnel-client supervision.
type TunnelConfig struct {
	Enabled bool
	// Subcommand is the tunnel-client command that owns the mcp/health flags
	// (upstream CLI shape: "tunnel-client run --mcp.server-url ...").
	Subcommand   string
	Binary       string
	ExtraArgs    []string
	TokenEnv     string
	TokenFile    string
	HealthSocket string
	// RestartBackoff is the initial delay between restarts; it doubles up to
	// RestartBackoffMax.
	RestartBackoff    time.Duration
	RestartBackoffMax time.Duration
	// MaxRestarts stops supervision after this many consecutive restarts.
	// Zero means unlimited (launchd owns the outer lifetime).
	MaxRestarts int
}

// DefaultConfig returns the Phase 0 defaults under the user's Application
// Support / Logs directories.
func DefaultConfig() Config {
	base := ""
	if dir, err := os.UserConfigDir(); err == nil {
		base = filepath.Join(dir, "CodeBridge")
	} else {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Library", "Application Support", "CodeBridge")
	}
	logDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		logDir = filepath.Join(home, "Library", "Logs", "CodeBridge")
	}
	cfg := Config{
		Version:       Version,
		DataDir:       base,
		RunDir:        filepath.Join(base, "run"),
		LogFile:       filepath.Join(logDir, "codebridged.log"),
		ProtocolMinor: 0,
		PingCommand:   []string{"/usr/bin/printf", "codebridge-native-host"},
		SmokeTimeout:  10 * time.Second,
		ProbeTimeout:  60 * time.Second,
		ShutdownGrace: 5 * time.Second,
		Tunnel: TunnelConfig{
			Enabled:           true,
			Subcommand:        "run",
			TokenEnv:          DefaultTunnelTokenEnv,
			RestartBackoff:    2 * time.Second,
			RestartBackoffMax: 60 * time.Second,
		},
	}
	cfg.RuntimeDB = filepath.Join(cfg.DataDir, "runtime.db")
	cfg.MCPSocket = filepath.Join(cfg.RunDir, "mcp.sock")
	cfg.HostSocket = filepath.Join(cfg.RunDir, "hostipc.sock")
	cfg.Tunnel.HealthSocket = filepath.Join(cfg.RunDir, "tunnel-health.sock")
	cfg.Tunnel.TokenFile = filepath.Join(cfg.RunDir, "tunnel-token")
	return cfg
}

// RegisterFlags adds the daemon flags to fs.
func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.DataDir, "data-dir", c.DataDir, "durable data directory (default ~/Library/Application Support/CodeBridge)")
	fs.StringVar(&c.RunDir, "run-dir", c.RunDir, "per-user runtime directory holding sockets (0700)")
	fs.StringVar(&c.MCPSocket, "mcp-socket", c.MCPSocket, "MCP Streamable HTTP Unix socket (no TCP listener is ever opened)")
	fs.StringVar(&c.HostSocket, "host-socket", c.HostSocket, "Host IPC Unix socket for CodeBridge.app")
	fs.StringVar(&c.LogFile, "log", c.LogFile, "log file path (empty logs to stderr only)")
	fs.IntVar(&c.ProtocolMinor, "protocol-minor", c.ProtocolMinor, "Host IPC protocol minor version")
	fs.StringVar(&c.AppTeamID, "app-team-id", c.AppTeamID, "Apple Team ID required of an app-role peer (empty refuses the app role)")
	fs.StringVar(&c.AppBundleID, "app-bundle-id", c.AppBundleID, "bundle identifier required of an app-role peer (empty refuses the app role)")
	fs.BoolVar(&c.Tunnel.Enabled, "tunnel", c.Tunnel.Enabled, "supervise tunnel-client")
	fs.StringVar(&c.Tunnel.Subcommand, "tunnel-subcommand", c.Tunnel.Subcommand, "tunnel-client subcommand that owns the mcp/health flags")
	fs.StringVar(&c.Tunnel.Binary, "tunnel-binary", c.Tunnel.Binary, "absolute path to the tunnel-client binary")
	fs.Var(&stringList{&c.Tunnel.ExtraArgs}, "tunnel-extra-arg", "extra tunnel-client argument (repeatable)")
	fs.StringVar(&c.Tunnel.TokenEnv, "tunnel-token-env", c.Tunnel.TokenEnv, "environment variable name carrying the per-launch bearer secret")
	fs.StringVar(&c.Tunnel.TokenFile, "tunnel-token-file", c.Tunnel.TokenFile, "0600 file receiving the per-launch secret for local probes (empty disables)")
	fs.StringVar(&c.Tunnel.HealthSocket, "tunnel-health-socket", c.Tunnel.HealthSocket, "tunnel-client --health.unix-socket path (never a TCP port)")
	fs.IntVar(&c.Tunnel.MaxRestarts, "tunnel-max-restarts", c.Tunnel.MaxRestarts, "stop supervising after N consecutive restarts (0 = unlimited)")
	fs.Var(&stringList{&c.PingCommand}, "ping-command", "fixed command the ping tool runs (space separated, no shell)")
	fs.StringVar(&c.RuntimeDB, "runtime-db", c.RuntimeDB, "durable runtime database path (default <data-dir>/runtime.db)")
	fs.BoolVar(&c.NoStore, "no-store", c.NoStore, "do not open the runtime store")
	fs.BoolVar(&c.WidgetSpike, "widget-spike", c.WidgetSpike, "register the opt-in Phase 0 widget/approval spike tools")
	fs.BoolVar(&c.Phase0Debug, "phase0-debug", c.Phase0Debug, "enable the fixed Phase 0 probe subprocess method")
	fs.StringVar(&c.Phase0ProbePath, "phase0-probe", c.Phase0ProbePath, "absolute path to the standalone probe binary")
	fs.StringVar(&c.Phase0HarnessPath, "phase0-harness", c.Phase0HarnessPath, "absolute path to a harness probe binary")
}

// ApplyEnv overlays environment overrides onto the config.
func (c *Config) ApplyEnv() {
	setIf := func(target *string, key string) {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			*target = v
		}
	}
	setIf(&c.DataDir, EnvDataDir)
	setIf(&c.RunDir, EnvRunDir)
	setIf(&c.MCPSocket, EnvMCPSocket)
	setIf(&c.HostSocket, EnvHostSocket)
	setIf(&c.LogFile, EnvLogFile)
	setIf(&c.AppTeamID, EnvAppTeamID)
	setIf(&c.AppBundleID, EnvAppBundleID)
	setIf(&c.Tunnel.Binary, EnvTunnelBinary)
	setIf(&c.Tunnel.TokenEnv, EnvTunnelTokenEnv)
	setIf(&c.Tunnel.TokenFile, EnvTunnelTokenFile)
	setIf(&c.RuntimeDB, EnvRuntimeDB)
	setIf(&c.Phase0ProbePath, EnvPhase0Probe)
	setIf(&c.Phase0HarnessPath, EnvPhase0Harness)
	if v := os.Getenv(EnvDisableTunnel); v == "1" || strings.EqualFold(v, "true") {
		c.Tunnel.Enabled = false
	}
	if v := os.Getenv(EnvWidgetSpike); v == "1" || strings.EqualFold(v, "true") {
		c.WidgetSpike = true
	}
	if v := os.Getenv(EnvPhase0Debug); v == "1" || strings.EqualFold(v, "true") {
		c.Phase0Debug = true
	}
	if v := os.Getenv(EnvNoStore); v == "1" || strings.EqualFold(v, "true") {
		c.NoStore = true
	}
	if c.NoStore {
		c.Store = nil
	}
}

// DerivePaths re-derives the paths that hang off RunDir and DataDir unless the
// caller set them explicitly (through a flag or the environment). This is what
// makes "-run-dir /tmp/x -data-dir /tmp/y" meaningful instead of leaving the
// sockets and the database in the default Application Support directory.
func (c *Config) DerivePaths(explicit map[string]bool) {
	if !explicit["runtime-db"] && c.DataDir != "" {
		c.RuntimeDB = filepath.Join(c.DataDir, "runtime.db")
	}
	if c.RunDir == "" {
		return
	}
	if !explicit["mcp-socket"] {
		c.MCPSocket = filepath.Join(c.RunDir, "mcp.sock")
	}
	if !explicit["host-socket"] {
		c.HostSocket = filepath.Join(c.RunDir, "hostipc.sock")
	}
	if !explicit["tunnel-health-socket"] {
		c.Tunnel.HealthSocket = filepath.Join(c.RunDir, "tunnel-health.sock")
	}
	if !explicit["tunnel-token-file"] {
		c.Tunnel.TokenFile = filepath.Join(c.RunDir, "tunnel-token")
	}
}

// DerivedFlagEnv maps each derived setting to the environment variable that
// marks it explicit.
func DerivedFlagEnv() map[string]string {
	return map[string]string{
		"mcp-socket":           EnvMCPSocket,
		"host-socket":          EnvHostSocket,
		"tunnel-health-socket": "",
		"tunnel-token-file":    EnvTunnelTokenFile,
		"runtime-db":           EnvRuntimeDB,
	}
}

// Validate reports configuration errors that must stop startup.
func (c *Config) Validate() error {
	if c.Version == "" {
		return errors.New("daemon: version must not be empty")
	}
	if c.RunDir == "" {
		return errors.New("daemon: run dir must not be empty")
	}
	if c.MCPSocket == "" || c.HostSocket == "" {
		return errors.New("daemon: both sockets must be configured")
	}
	if c.MCPSocket == c.HostSocket {
		return errors.New("daemon: MCP and Host IPC sockets must differ")
	}
	if len(c.PingCommand) == 0 {
		return errors.New("daemon: ping command must not be empty")
	}
	if c.ProtocolMinor < 0 {
		return errors.New("daemon: protocol minor must not be negative")
	}
	if c.Tunnel.Enabled && strings.TrimSpace(c.Tunnel.Binary) == "" {
		// Enabled without a binary is a documented prerequisite gap, not a
		// startup failure: the supervisor reports "not_configured" and the rest
		// of the daemon runs. See docs/v2/evidence/phase0-tunnel.md.
		return nil
	}
	if c.Tunnel.Enabled && !filepath.IsAbs(c.Tunnel.Binary) {
		return fmt.Errorf("daemon: tunnel binary %q must be an absolute path", c.Tunnel.Binary)
	}
	return nil
}

// AppSigningConfigured reports whether an app-role peer can ever be verified.
func (c *Config) AppSigningConfigured() bool {
	return strings.TrimSpace(c.AppTeamID) != "" && strings.TrimSpace(c.AppBundleID) != ""
}

// stringList is a repeatable string flag that also accepts a single
// space-separated value on first use (e.g. -ping-command "/usr/bin/true").
type stringList struct{ target *[]string }

func (s *stringList) String() string {
	if s.target == nil {
		return ""
	}
	return strings.Join(*s.target, " ")
}

func (s *stringList) Set(value string) error {
	if s.target == nil {
		return errors.New("daemon: string list target is nil")
	}
	if len(*s.target) == 0 {
		*s.target = splitCommand(value)
		return nil
	}
	*s.target = append(*s.target, value)
	return nil
}

// splitCommand splits a space-separated command line. Quoting is intentionally
// not supported: the daemon never runs a shell and the fixed commands need no
// arguments containing spaces.
func splitCommand(value string) []string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	return fields
}
