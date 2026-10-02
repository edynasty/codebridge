package daemon

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Tunnel states reported by host.health and the MCP health tool.
const (
	TunnelDisabled      = "disabled"
	TunnelNotConfigured = "not_configured"
	TunnelRunning       = "running"
	TunnelRestarting    = "restarting"
	TunnelFailed        = "failed"
	TunnelStopped       = "stopped"
)

// TunnelHealth is the read model for tunnel supervision.
type TunnelHealth struct {
	Configured   bool   `json:"configured"`
	Supervised   bool   `json:"supervised"`
	State        string `json:"state"`
	PID          int    `json:"pid,omitempty"`
	Restarts     int    `json:"restarts"`
	MaxRestarts  int    `json:"max_restarts,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	Binary       string `json:"binary,omitempty"`
	ServerURL    string `json:"server_url,omitempty"`
	HealthSocket string `json:"health_socket,omitempty"`
	TokenEnv     string `json:"token_env,omitempty"`
	TokenFile    string `json:"token_file,omitempty"`
}

// TunnelSupervisor owns the tunnel-client child process. The per-launch bearer
// secret is passed to the child through its environment only: it is never an
// argument and never appears in a log line.
type TunnelSupervisor struct {
	cfg   Config
	token string
	log   *slog.Logger

	mu       sync.Mutex
	cmd      *exec.Cmd
	state    string
	restarts int
	lastErr  string
	cancel   context.CancelFunc
	stopped  chan struct{}
	stopOnce sync.Once
}

// NewTunnelSupervisor builds a supervisor for cfg.
func NewTunnelSupervisor(cfg Config, token string, log *slog.Logger) *TunnelSupervisor {
	return &TunnelSupervisor{cfg: cfg, token: token, log: log, state: TunnelDisabled, stopped: make(chan struct{})}
}

// ServerURL is the value passed to tunnel-client's --mcp.server-url: the
// daemon's Streamable HTTP MCP endpoint reached over a Unix socket. No TCP
// endpoint exists.
func (t *TunnelSupervisor) ServerURL() string {
	return "url=http://localhost/mcp,unix-socket=" + t.cfg.MCPSocket
}

// BuildArgs renders the tunnel-client argument vector. The flags live on the
// "run" subcommand upstream, so that is always argv[1].
func (t *TunnelSupervisor) BuildArgs() []string {
	args := make([]string, 0, 11)
	if sub := strings.TrimSpace(t.cfg.Tunnel.Subcommand); sub != "" {
		args = append(args, sub)
	}
	args = append(args,
		"--mcp.server-url", t.ServerURL(),
		"--health.unix-socket", t.cfg.Tunnel.HealthSocket,
	)
	if t.cfg.Tunnel.TokenEnv != "" {
		header := "Authorization: env:" + t.cfg.Tunnel.TokenEnv
		args = append(args,
			"--mcp.extra-headers", header,
			"--mcp.discovery-extra-headers", header,
		)
	}
	args = append(args, t.cfg.Tunnel.ExtraArgs...)
	return args
}

// BuildEnv returns the child environment: the daemon's own plus the per-launch
// bearer secret. The value is "Bearer <secret>" because tunnel-client
// substitutes env references verbatim into the header value.
func (t *TunnelSupervisor) BuildEnv() []string {
	env := os.Environ()
	if t.cfg.Tunnel.TokenEnv != "" && t.token != "" {
		env = append(env, t.cfg.Tunnel.TokenEnv+"=Bearer "+t.token)
	}
	return env
}

// Start launches the supervision loop. It never blocks.
func (t *TunnelSupervisor) Start(ctx context.Context) {
	t.mu.Lock()
	switch {
	case !t.cfg.Tunnel.Enabled:
		t.state = TunnelDisabled
		t.mu.Unlock()
		t.log.Info("tunnel supervisor disabled")
		return
	case t.cfg.Tunnel.Binary == "":
		t.state = TunnelNotConfigured
		t.lastErr = "tunnel binary not configured"
		t.mu.Unlock()
		t.log.Warn("tunnel supervisor not configured: no -tunnel-binary; MCP ingress still serves the socket",
			"mcp_socket", t.cfg.MCPSocket)
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.state = TunnelRestarting
	t.mu.Unlock()
	go t.loop(runCtx)
}

func (t *TunnelSupervisor) loop(ctx context.Context) {
	defer close(t.stopped)
	backoff := t.cfg.Tunnel.RestartBackoff
	if backoff <= 0 {
		backoff = 2 * time.Second
	}
	maxBackoff := t.cfg.Tunnel.RestartBackoffMax
	if maxBackoff <= 0 {
		maxBackoff = 60 * time.Second
	}
	for {
		if ctx.Err() != nil {
			t.setState(TunnelStopped, 0, "supervisor stopped")
			return
		}
		args := t.BuildArgs()
		cmd := exec.CommandContext(ctx, t.cfg.Tunnel.Binary, args...)
		cmd.Env = t.BuildEnv()
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		// The bearer secret is passed through the environment; arguments and
		// log lines never contain it.
		t.log.Info("tunnel starting", "binary", t.cfg.Tunnel.Binary,
			"subcommand", t.cfg.Tunnel.Subcommand, "server_url", t.ServerURL(),
			"health_socket", t.cfg.Tunnel.HealthSocket, "token_env", t.cfg.Tunnel.TokenEnv)
		if err := cmd.Start(); err != nil {
			t.mu.Lock()
			t.restarts++
			t.lastErr = err.Error()
			restarts := t.restarts
			t.mu.Unlock()
			t.log.Error("tunnel start failed", "error", err.Error(), "restarts", restarts)
			if t.cfg.Tunnel.MaxRestarts > 0 && restarts >= t.cfg.Tunnel.MaxRestarts {
				t.setState(TunnelFailed, 0, err.Error())
				return
			}
			t.setState(TunnelRestarting, 0, err.Error())
			if !sleepCtx(ctx, backoff) {
				t.setState(TunnelStopped, 0, "supervisor stopped")
				return
			}
			backoff = growBackoff(backoff, maxBackoff)
			continue
		}
		t.mu.Lock()
		t.cmd = cmd
		t.state = TunnelRunning
		pid := cmd.Process.Pid
		t.lastErr = ""
		t.mu.Unlock()
		t.log.Info("tunnel running", "pid", pid)

		err := cmd.Wait()
		t.mu.Lock()
		t.cmd = nil
		t.mu.Unlock()
		if ctx.Err() != nil {
			t.setState(TunnelStopped, 0, "supervisor stopped")
			return
		}
		t.mu.Lock()
		t.restarts++
		restarts := t.restarts
		if err != nil {
			t.lastErr = err.Error()
		} else {
			t.lastErr = "tunnel exited with status 0"
		}
		lastErr := t.lastErr
		t.mu.Unlock()
		t.log.Warn("tunnel exited", "restarts", restarts, "error", lastErr)
		if t.cfg.Tunnel.MaxRestarts > 0 && restarts >= t.cfg.Tunnel.MaxRestarts {
			t.setState(TunnelFailed, 0, lastErr)
			return
		}
		t.setState(TunnelRestarting, 0, lastErr)
		if !sleepCtx(ctx, backoff) {
			t.setState(TunnelStopped, 0, "supervisor stopped")
			return
		}
		backoff = growBackoff(backoff, maxBackoff)
	}
}

// Stop terminates the child (SIGTERM, then SIGKILL after the shutdown grace).
func (t *TunnelSupervisor) Stop() {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		cancel := t.cancel
		cmd := t.cmd
		enabled := t.cfg.Tunnel.Enabled && t.cfg.Tunnel.Binary != ""
		t.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			deadline := time.Now().Add(t.cfg.ShutdownGrace)
			for time.Now().Before(deadline) {
				t.mu.Lock()
				done := t.cmd == nil
				t.mu.Unlock()
				if done {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.mu.Lock()
			stillRunning := t.cmd != nil
			t.mu.Unlock()
			if stillRunning {
				_ = cmd.Process.Kill()
			}
		}
		if !enabled {
			return
		}
		select {
		case <-t.stopped:
		case <-time.After(t.cfg.ShutdownGrace):
		}
	})
}

func (t *TunnelSupervisor) setState(state string, pid int, lastErr string) {
	t.mu.Lock()
	t.state = state
	if lastErr != "" {
		t.lastErr = lastErr
	}
	if pid > 0 && t.cmd != nil {
		t.cmd.Process.Pid = pid
	}
	t.mu.Unlock()
}

// Health reports the current supervision state.
func (t *TunnelSupervisor) Health() TunnelHealth {
	t.mu.Lock()
	defer t.mu.Unlock()
	h := TunnelHealth{
		Configured:   t.cfg.Tunnel.Enabled && t.cfg.Tunnel.Binary != "",
		Supervised:   t.cancel != nil,
		State:        t.state,
		Restarts:     t.restarts,
		MaxRestarts:  t.cfg.Tunnel.MaxRestarts,
		LastError:    t.lastErr,
		Binary:       t.cfg.Tunnel.Binary,
		ServerURL:    t.ServerURL(),
		HealthSocket: t.cfg.Tunnel.HealthSocket,
		TokenEnv:     t.cfg.Tunnel.TokenEnv,
		TokenFile:    t.cfg.Tunnel.TokenFile,
	}
	if t.cmd != nil && t.cmd.Process != nil {
		h.PID = t.cmd.Process.Pid
	}
	return h
}

// Token returns the per-launch bearer secret. Callers must never log it.
func (t *TunnelSupervisor) Token() string { return t.token }

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func growBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
