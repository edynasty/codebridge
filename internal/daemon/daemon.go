package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/edynasty/codebridge/internal/hostipc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Daemon is the Phase 0 codebridged authority.
type Daemon struct {
	cfg     Config
	log     *slog.Logger
	started time.Time
	token   string
	tunnel  *TunnelSupervisor

	servers map[CallerClass]*mcp.Server
	hostSrv *hostipc.Server
	hostLn  net.Listener
	mcpSrv  *http.Server
	mcpLn   net.Listener

	errCh    chan error
	errOnce  sync.Once
	stopOnce sync.Once
}

// New builds a daemon. The caller owns the logger and Store lifetime.
func New(cfg Config, log *slog.Logger) (*Daemon, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	token, err := newIngressToken()
	if err != nil {
		return nil, err
	}
	d := &Daemon{
		cfg:     cfg,
		log:     log,
		started: time.Now(),
		token:   token,
		errCh:   make(chan error, 4),
	}
	d.tunnel = NewTunnelSupervisor(cfg, token, log)
	// Fail fast at startup: a registrar that panics or a tool whose schema is
	// invalid must stop the daemon, not turn every ingress request into a
	// recovered panic (which surfaces to the client as a bare EOF).
	d.buildMCPServers()
	return d, nil
}

// Config returns the effective configuration.
func (d *Daemon) Config() Config { return d.cfg }

// Tunnel exposes the supervisor for the CLI doctor and health surfaces.
func (d *Daemon) Tunnel() *TunnelSupervisor { return d.tunnel }

// Token returns the per-launch bearer secret. Callers must never log it.
func (d *Daemon) Token() string { return d.token }

// Run serves until ctx is cancelled or a fatal listener error occurs. It
// performs a controlled shutdown: stop accepting, close listeners, terminate
// tunnel-client with a grace period, then close the store.
func (d *Daemon) Run(ctx context.Context) error {
	if err := os.MkdirAll(d.cfg.RunDir, 0o700); err != nil {
		return fmt.Errorf("daemon: create run dir: %w", err)
	}
	if err := os.Chmod(d.cfg.RunDir, 0o700); err != nil {
		return fmt.Errorf("daemon: chmod run dir: %w", err)
	}
	if err := os.MkdirAll(d.cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("daemon: create data dir: %w", err)
	}
	if err := writeTokenFile(d.cfg.Tunnel.TokenFile, d.token); err != nil {
		return err
	}
	if d.cfg.Tunnel.TokenFile != "" {
		defer func() { _ = os.Remove(d.cfg.Tunnel.TokenFile) }()
	}

	// Host IPC listener (CodeBridge.app side).
	hostSrv := hostipc.NewServer(hostipc.ServerOptions{
		SocketPath:     d.cfg.HostSocket,
		Version:        d.cfg.Version,
		Protocol:       hostipc.Protocol{Major: hostipc.ProtocolMajor, Minor: d.cfg.ProtocolMinor},
		Capabilities:   d.hostCapabilities(),
		Logger:         d.log,
		VerifyAppPeer:  d.verifyAppPeer,
		AppRequirement: hostipc.AppRequirement(d.cfg.AppTeamID, d.cfg.AppBundleID),
		HelloTimeout:   10 * time.Second,
	})
	d.registerHostServices(hostSrv)
	hostLn, err := hostipc.ListenUnix(d.cfg.HostSocket)
	if err != nil {
		return err
	}
	d.hostSrv = hostSrv
	d.hostLn = hostLn
	go func() {
		if err := hostSrv.Serve(hostLn); err != nil {
			d.reportErr(fmt.Errorf("host ipc: %w", err))
		}
	}()

	// MCP ingress (tunnel-client side).
	if err := d.startMCPIngress(); err != nil {
		_ = hostSrv.Close()
		return err
	}

	d.tunnel.Start(ctx)
	d.log.Info("codebridged ready",
		"version", d.cfg.Version,
		"pid", os.Getpid(),
		"data_dir", d.cfg.DataDir,
		"run_dir", d.cfg.RunDir,
		"mcp_socket", d.cfg.MCPSocket,
		"host_socket", d.cfg.HostSocket,
		"protocol", fmt.Sprintf("%d.%d", hostipc.ProtocolMajor, d.cfg.ProtocolMinor),
		"tcp_listeners", 0,
		"store_wired", d.cfg.Store != nil,
		"app_signing_configured", d.cfg.AppSigningConfigured(),
		"probe_enabled", d.ProbeEnabled(),
	)
	if !d.cfg.AppSigningConfigured() {
		d.log.Warn("app role refused: no signing identity configured; pass -app-team-id and -app-bundle-id",
			"reason", "signing_identity_not_configured", "bypass", "none")
	}

	select {
	case <-ctx.Done():
		d.log.Info("codebridged shutdown requested")
	case err := <-d.errCh:
		d.log.Error("codebridged fatal error", "error", err.Error())
		d.shutdown(context.Background())
		return err
	}
	d.shutdown(context.Background())
	return nil
}

func (d *Daemon) shutdown(parent context.Context) {
	d.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), d.cfg.ShutdownGrace)
		defer cancel()
		d.tunnel.Stop()
		d.stopMCPIngress(ctx)
		if d.hostSrv != nil {
			_ = d.hostSrv.Close()
		}
		if d.hostLn != nil {
			_ = d.hostLn.Close()
		}
		if d.cfg.Store != nil {
			if err := d.cfg.Store.Close(); err != nil {
				d.log.Warn("store close failed", "error", err.Error())
			}
		}
		for _, path := range []string{d.cfg.MCPSocket, d.cfg.HostSocket} {
			if err := os.Remove(path); err == nil {
				d.log.Info("socket removed", "path", path)
			}
		}
		d.log.Info("codebridged stopped")
	})
}

// reportErr delivers the first fatal listener error to Run.
func (d *Daemon) reportErr(err error) {
	d.errOnce.Do(func() {
		select {
		case d.errCh <- err:
		default:
		}
	})
}

// hostCapabilities lists what the daemon can actually offer in Phase 0. computer
// and approval are deliberately absent: the frozen contract declares them, but
// nothing is implemented, so no client is told they are available.
func (d *Daemon) hostCapabilities() []hostipc.Capability {
	caps := []hostipc.Capability{hostipc.CapHost, hostipc.CapNativeHostSmoke}
	if d.cfg.Store != nil {
		caps = append(caps, hostipc.CapRuntime)
	}
	return caps
}

// verifyAppPeer validates the live app process against the configured signing
// identity. A missing identity is a refusal, never a bypass.
func (d *Daemon) verifyAppPeer(pid int) error {
	if !d.cfg.AppSigningConfigured() {
		return &hostipc.SignatureError{PID: pid, Reason: "signing_identity_not_configured",
			Detail: "no -app-team-id/-app-bundle-id configured"}
	}
	requirement := hostipc.AppRequirement(d.cfg.AppTeamID, d.cfg.AppBundleID)
	return hostipc.VerifyPeerCodeSignature(pid, requirement)
}

// SocketPaths reports the two sockets the daemon owns (tests and evidence).
func (d *Daemon) SocketPaths() (mcp, host string) {
	return d.cfg.MCPSocket, d.cfg.HostSocket
}

// ErrNoRunDir reports a missing run directory.
var ErrNoRunDir = errors.New("daemon: run dir does not exist")

// EnsureRunDir creates the per-user run directory 0700.
func EnsureRunDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// SocketPath returns the default socket path inside a run directory.
func SocketPath(runDir, name string) string { return filepath.Join(runDir, name) }
