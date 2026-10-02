package daemon

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/edynasty/codebridge/internal/hostipc"
)

// hostCaller describes the authenticated Host IPC caller.
type hostCaller struct {
	Role              string `json:"role"`
	Class             string `json:"class"`
	PeerUID           uint32 `json:"peer_uid"`
	PeerPID           int    `json:"peer_pid"`
	SignatureVerified bool   `json:"signature_verified"`
}

// hostSigning reports whether the daemon can verify an app peer at all. The
// requirement text is configuration, never a secret.
type hostSigning struct {
	AppRoleConfigured bool   `json:"app_role_configured"`
	TeamID            string `json:"team_id,omitempty"`
	BundleID          string `json:"bundle_id,omitempty"`
}

// hostListeners reports the only two endpoints the daemon opens.
type hostListeners struct {
	MCPSocket     string `json:"mcp_socket"`
	MCPSocketMode string `json:"mcp_socket_mode"`
	HostSocket    string `json:"host_socket"`
	TCPListeners  int    `json:"tcp_listeners"`
}

// hostHealthResult is the host.health result.
type hostHealthResult struct {
	Status        string           `json:"status"`
	DaemonVersion string           `json:"daemon_version"`
	Protocol      hostipc.Protocol `json:"protocol"`
	UptimeMS      int64            `json:"uptime_ms"`
	PID           int              `json:"pid"`
	Caller        hostCaller       `json:"caller"`
	Store         map[string]any   `json:"store"`
	Tunnel        TunnelHealth     `json:"tunnel"`
	Listeners     hostListeners    `json:"listeners"`
	Signing       hostSigning      `json:"signing"`
	ProbeEnabled  bool             `json:"probe_enabled"`
}

type nativeHostSmokeParams struct {
	Only []string `json:"only,omitempty"`
}

type nativeHostSmokeResult struct {
	Report NativeHostReport `json:"report"`
}

type phase0ProbeParams struct {
	Probe     string   `json:"probe"`
	Args      []string `json:"args,omitempty"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
}

type phase0ProbeResult struct {
	Argv     []string `json:"argv"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	TimedOut bool     `json:"timed_out"`
	Error    string   `json:"error,omitempty"`
}

type runtimeHealthResult struct {
	StoreWired    bool `json:"store_wired"`
	SchemaVersion int  `json:"schema_version,omitempty"`
	RunsActive    int  `json:"runs_active"`
	SessionsOpen  int  `json:"sessions_open"`
}

type runtimeEventsParams struct {
	Stream   string `json:"stream,omitempty"`
	AfterSeq int64  `json:"after_seq,omitempty"`
	AfterPos int64  `json:"after_pos,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type runtimeEventsResult struct {
	Events  []JournalEvent `json:"events"`
	NextPos int64          `json:"next_pos"`
}

// registerHostServices installs the Phase 0 Host IPC handlers. Methods that are
// part of the frozen contract but not implemented in Phase 0 stay unregistered
// and answer unsupported.
func (d *Daemon) registerHostServices(s *hostipc.Server) {
	s.Handle(hostipc.MethodHealth, d.handleHostHealth)
	s.Handle(hostipc.MethodNativeHostSmoke, d.handleNativeHostSmoke)
	if d.ProbeEnabled() {
		s.Handle(hostipc.MethodPhase0Probe, d.handlePhase0Probe)
	}
	if d.cfg.Store != nil {
		s.Handle(hostipc.MethodRuntimeHealth, d.handleRuntimeHealth)
		s.Handle(hostipc.MethodRuntimeEvents, d.handleRuntimeEvents)
	}
}

func (d *Daemon) callerOf(c *hostipc.Conn) hostCaller {
	cred := c.Peer()
	return hostCaller{
		Role:              string(c.Role()),
		Class:             c.Class(),
		PeerUID:           cred.UID,
		PeerPID:           cred.PID,
		SignatureVerified: c.SignatureVerified(),
	}
}

func (d *Daemon) handleHostHealth(_ context.Context, c *hostipc.Conn, _ json.RawMessage) (any, *hostipc.Error) {
	mode := ""
	if fi, err := os.Stat(d.cfg.MCPSocket); err == nil {
		mode = fi.Mode().Perm().String()
	}
	status := "ok"
	if th := d.tunnel.Health(); th.Configured && th.State == TunnelFailed {
		status = "degraded"
	}
	store := storeHealth(d.cfg.Store)
	if err, ok := store["error"]; ok && err != nil {
		status = "degraded"
	}
	return hostHealthResult{
		Status:        status,
		DaemonVersion: d.cfg.Version,
		Protocol:      hostipc.Protocol{Major: hostipc.ProtocolMajor, Minor: d.cfg.ProtocolMinor},
		UptimeMS:      time.Since(d.started).Milliseconds(),
		PID:           os.Getpid(),
		Caller:        d.callerOf(c),
		Store:         store,
		Tunnel:        d.tunnel.Health(),
		Listeners: hostListeners{
			MCPSocket:     d.cfg.MCPSocket,
			MCPSocketMode: mode,
			HostSocket:    d.cfg.HostSocket,
			TCPListeners:  0,
		},
		Signing: hostSigning{
			AppRoleConfigured: d.cfg.AppSigningConfigured(),
			TeamID:            d.cfg.AppTeamID,
			BundleID:          d.cfg.AppBundleID,
		},
		ProbeEnabled: d.ProbeEnabled(),
	}, nil
}

func (d *Daemon) handleNativeHostSmoke(ctx context.Context, _ *hostipc.Conn, params json.RawMessage) (any, *hostipc.Error) {
	var in nativeHostSmokeParams
	if perr := hostipc.DecodeParams(params, &in); perr != nil {
		return nil, perr
	}
	report, err := d.RunNativeHostSmoke(ctx, in.Only)
	if err != nil {
		return nil, hostipc.NewError(hostipc.CodeInvalidParams, "invalid_params", err.Error())
	}
	return nativeHostSmokeResult{Report: report}, nil
}

func (d *Daemon) handlePhase0Probe(ctx context.Context, _ *hostipc.Conn, params json.RawMessage) (any, *hostipc.Error) {
	var in phase0ProbeParams
	if perr := hostipc.DecodeParams(params, &in); perr != nil {
		return nil, perr
	}
	res, err := d.RunPhase0Probe(ctx, in.Probe, in.TimeoutMS)
	if err != nil {
		return nil, hostipc.NewError(hostipc.CodeInvalidParams, "invalid_params", err.Error())
	}
	return phase0ProbeResult{
		Argv:     res.Argv,
		ExitCode: res.ExitCode,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		TimedOut: res.TimedOut,
		Error:    res.Err,
	}, nil
}

func (d *Daemon) handleRuntimeHealth(_ context.Context, _ *hostipc.Conn, _ json.RawMessage) (any, *hostipc.Error) {
	if d.cfg.Store == nil {
		return nil, hostipc.NewError(hostipc.CodeUnsupported, "unsupported", "runtime store is not wired")
	}
	stats, err := d.cfg.Store.Stats()
	if err != nil {
		return nil, hostipc.NewError(hostipc.CodeInternal, "internal", err.Error())
	}
	return runtimeHealthResult{
		StoreWired:    true,
		SchemaVersion: stats.SchemaVersion,
		RunsActive:    stats.RunsActive,
		SessionsOpen:  stats.SessionsOpen,
	}, nil
}

func (d *Daemon) handleRuntimeEvents(_ context.Context, _ *hostipc.Conn, params json.RawMessage) (any, *hostipc.Error) {
	if d.cfg.Store == nil {
		return nil, hostipc.NewError(hostipc.CodeUnsupported, "unsupported", "runtime store is not wired")
	}
	var in runtimeEventsParams
	if perr := hostipc.DecodeParams(params, &in); perr != nil {
		return nil, perr
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	var (
		events []JournalEvent
		err    error
	)
	if in.Stream == "" {
		events, err = d.cfg.Store.EventsAfter(in.AfterPos, limit)
	} else {
		events, err = d.cfg.Store.EventsForStream(in.Stream, in.AfterSeq, limit)
	}
	if err != nil {
		return nil, hostipc.NewError(hostipc.CodeInternal, "internal", err.Error())
	}
	next := in.AfterPos
	for _, ev := range events {
		if ev.Pos > next {
			next = ev.Pos
		}
	}
	return runtimeEventsResult{Events: events, NextPos: next}, nil
}
