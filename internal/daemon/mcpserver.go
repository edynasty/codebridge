package daemon

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CallerClass classifies the ingress caller.
type CallerClass string

const (
	// CallerRemoteAI is a request authenticated with the per-launch tunnel
	// bearer secret: the remote model path through tunnel-client.
	CallerRemoteAI CallerClass = "remote_ai"
	// CallerLocalMCP is a same-user local connection without the tunnel secret.
	CallerLocalMCP CallerClass = "local_mcp"
)

type callerClassKey struct{}

func withCallerClass(ctx context.Context, class CallerClass) context.Context {
	return context.WithValue(ctx, callerClassKey{}, class)
}

func callerClassFrom(r *http.Request) CallerClass {
	if v, ok := r.Context().Value(callerClassKey{}).(CallerClass); ok {
		return v
	}
	return CallerLocalMCP
}

type emptyInput struct{}

type pingOutput struct {
	Status        string     `json:"status"`
	Tool          string     `json:"tool"`
	DaemonVersion string     `json:"daemon_version"`
	Protocol      string     `json:"protocol"`
	UptimeMS      int64      `json:"uptime_ms"`
	CallerClass   string     `json:"caller_class"`
	CheckedAt     string     `json:"checked_at"`
	NativeHost    ExecResult `json:"native_host"`
}

type healthStore struct {
	Wired         bool        `json:"wired"`
	SchemaVersion int         `json:"schema_version,omitempty"`
	Error         string      `json:"error,omitempty"`
	Stats         *StoreStats `json:"stats,omitempty"`
}

type healthListeners struct {
	MCPSocket    string `json:"mcp_socket"`
	HostSocket   string `json:"host_socket"`
	MCPTransport string `json:"mcp_transport"`
}

type healthNativeHost struct {
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	Cwd         string `json:"cwd"`
	HomePresent bool   `json:"home_present"`
	PathEntries int    `json:"path_entries"`
}

type healthOutput struct {
	Status        string           `json:"status"`
	DaemonVersion string           `json:"daemon_version"`
	Protocol      string           `json:"protocol"`
	UptimeMS      int64            `json:"uptime_ms"`
	DaemonPID     int              `json:"daemon_pid"`
	CallerClass   string           `json:"caller_class"`
	Store         healthStore      `json:"store"`
	Tunnel        TunnelHealth     `json:"tunnel"`
	Listeners     healthListeners  `json:"listeners"`
	NativeHost    healthNativeHost `json:"native_host"`
}

// buildMCPServers creates the two long-lived MCP servers: one for the
// authenticated remote_ai caller class and one for same-user local_mcp.
//
// They are built once per daemon, not per request: tool state (approval tokens,
// widget sessions, leases) must survive across HTTP requests, and an ingress
// that rebuilt the server every request would silently lose it. Building them
// here also makes a broken tool registration fail loudly at startup instead of
// turning every request into a recovered panic.
func (d *Daemon) buildMCPServers() {
	if d.servers != nil {
		return
	}
	d.servers = map[CallerClass]*mcp.Server{
		CallerRemoteAI: d.newMCPServer(CallerRemoteAI, true),
		CallerLocalMCP: d.newMCPServer(CallerLocalMCP, false),
	}
}

func (d *Daemon) newMCPServer(class CallerClass, widgetSpike bool) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: d.cfg.Version}, nil)
	d.registerPing(s, class)
	d.registerHealth(s, class)
	if widgetSpike && d.cfg.WidgetSpike {
		// The opt-in widget/approval spike is remote_ai only: a local_mcp
		// connection can never see or drive approval surfaces.
		registerWidgetSpike(s)
	}
	return s
}

// serverForRequest selects the caller class's cached MCP server. It never
// constructs a server, so per-request identity is stable for the daemon's life.
func (d *Daemon) serverForRequest(r *http.Request) *mcp.Server {
	if s, ok := d.servers[callerClassFrom(r)]; ok {
		return s
	}
	return d.servers[CallerLocalMCP]
}

func (d *Daemon) registerPing(s *mcp.Server, class CallerClass) {
	tool := &mcp.Tool{
		Name: "ping",
		Description: "Prove the CodeBridge daemon is alive on the native host. Runs the daemon's own fixed, " +
			"harmless command (no caller input, no shell) and returns its argv, exit code and captured output. " +
			"Use it to confirm the daemon process, not the client, executed the command. Phase 0 exposes no " +
			"Computer Use or arbitrary shell tool.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: new(false),
			OpenWorldHint:   new(false),
		},
	}
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, pingOutput, error) {
		res := d.PingNativeHost(ctx)
		out := pingOutput{
			Status:        "ok",
			Tool:          "ping",
			DaemonVersion: d.cfg.Version,
			Protocol:      fmt.Sprintf("hostipc/%d.%d", 1, d.cfg.ProtocolMinor),
			UptimeMS:      time.Since(d.started).Milliseconds(),
			CallerClass:   string(class),
			CheckedAt:     time.Now().UTC().Format(time.RFC3339Nano),
			NativeHost:    res,
		}
		if !res.Started || res.ExitCode != 0 || res.TimedOut {
			out.Status = "degraded"
		}
		return nil, out, nil
	})
}

func (d *Daemon) registerHealth(s *mcp.Server, class CallerClass) {
	tool := &mcp.Tool{
		Name: "health",
		Description: "Report CodeBridge daemon health without executing host commands: version, negotiated " +
			"protocol, uptime, runtime store state, tunnel-client supervision state and the Unix-socket " +
			"listeners (the daemon opens no TCP listener).",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: new(false),
			OpenWorldHint:   new(false),
		},
	}
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, healthOutput, error) {
		cwd, _ := os.Getwd()
		out := healthOutput{
			Status:        "ok",
			DaemonVersion: d.cfg.Version,
			Protocol:      fmt.Sprintf("hostipc/%d.%d", 1, d.cfg.ProtocolMinor),
			UptimeMS:      time.Since(d.started).Milliseconds(),
			DaemonPID:     os.Getpid(),
			CallerClass:   string(class),
			Store:         healthStore{Wired: d.cfg.Store != nil},
			Tunnel:        d.tunnel.Health(),
			Listeners: healthListeners{
				MCPSocket:    d.cfg.MCPSocket,
				HostSocket:   d.cfg.HostSocket,
				MCPTransport: "streamable-http+unix",
			},
			NativeHost: healthNativeHost{
				GOOS:        runtime.GOOS,
				GOARCH:      runtime.GOARCH,
				Cwd:         cwd,
				HomePresent: os.Getenv("HOME") != "",
				PathEntries: len(strings.Split(os.Getenv("PATH"), ":")),
			},
		}
		if d.cfg.Store != nil {
			out.Store.SchemaVersion = d.cfg.Store.SchemaVersion()
			if stats, err := d.cfg.Store.Stats(); err == nil {
				out.Store.Stats = &stats
			} else {
				out.Store.Error = err.Error()
				out.Status = "degraded"
			}
		}
		if d.tunnel.Health().Configured && d.tunnel.Health().State == TunnelFailed {
			out.Status = "degraded"
		}
		return nil, out, nil
	})
}

// registerWidgetSpike is replaced by the parent-owned approvalspike registration
// when the widget spike is enabled. It is a package-level hook so the daemon
// core never imports the spike package unless the feature is built in.
var registerWidgetSpike = func(s *mcp.Server) {}

// SetWidgetSpikeRegistrar installs the opt-in widget spike registration. The
// parent integration calls it from main before Run.
func SetWidgetSpikeRegistrar(fn func(s *mcp.Server)) {
	if fn == nil {
		registerWidgetSpike = func(s *mcp.Server) {}
		return
	}
	registerWidgetSpike = fn
}
