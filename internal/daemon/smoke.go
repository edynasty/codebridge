package daemon

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"
)

// SmokeCheck is one fixed, daemon-owned harmless check. The argv is a compile
// time constant of the daemon; no caller input ever reaches it, which is what
// keeps host.native_host_smoke from being a general exec surface.
type SmokeCheck struct {
	Name string
	Argv []string
}

// nativeHostChecks is the frozen Phase 0 check set. It exists to prove that the
// daemon, running under its real (launchd or foreground) authority, can spawn
// the same local tooling an agent harness will need.
var nativeHostChecks = []SmokeCheck{
	{Name: "shell", Argv: []string{"/bin/sh", "-c", "printf codebridge-native-host"}},
	{Name: "git", Argv: []string{"git", "--version"}},
	// `docker version` contacts the Docker Desktop daemon, so the check proves
	// SERVER access, not just that a client binary exists.
	{Name: "docker", Argv: []string{"docker", "version"}},
	{Name: "ssh", Argv: []string{"ssh", "-V"}},
	{Name: "kubectl", Argv: []string{"kubectl", "version", "--client"}},
	{Name: "path", Argv: []string{"/usr/bin/which", "-a", "git"}},
}

// LaunchIdentity reports which process tree the checks ran in. It is the
// daemon's own identity, not the caller's.
type LaunchIdentity struct {
	PID        int    `json:"pid"`
	PPID       int    `json:"ppid"`
	Executable string `json:"executable"`
	Cwd        string `json:"cwd"`
	RunDir     string `json:"run_dir"`
	HostSocket string `json:"host_socket"`
}

// NativeHostReport is the serialized result of the fixed native-host smoke.
type NativeHostReport struct {
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at"`
	DaemonPID  int            `json:"daemon_pid"`
	Cwd        string         `json:"cwd"`
	Env        envReport      `json:"env"`
	Launch     LaunchIdentity `json:"launch"`
	Checks     []ExecResult   `json:"checks"`
}

// NativeHostChecks lists the frozen check names in order.
func NativeHostChecks() []string {
	names := make([]string, 0, len(nativeHostChecks))
	for _, c := range nativeHostChecks {
		names = append(names, c.Name)
	}
	return names
}

// RunNativeHostSmoke executes the fixed check set (optionally filtered by
// only) and returns the serialized report. An unknown name in only is an error
// rather than a silent skip.
func (d *Daemon) RunNativeHostSmoke(ctx context.Context, only []string) (NativeHostReport, error) {
	selected := nativeHostChecks
	if len(only) > 0 {
		want := map[string]bool{}
		for _, name := range only {
			want[name] = true
		}
		known := map[string]bool{}
		for _, c := range nativeHostChecks {
			known[c.Name] = true
		}
		var unknown []string
		for name := range want {
			if !known[name] {
				unknown = append(unknown, name)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return NativeHostReport{}, fmt.Errorf("unknown smoke check(s) %v; known checks: %v", unknown, NativeHostChecks())
		}
		selected = nil
		for _, c := range nativeHostChecks {
			if want[c.Name] {
				selected = append(selected, c)
			}
		}
	}

	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	report := NativeHostReport{
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		DaemonPID: os.Getpid(),
		Cwd:       cwd,
		Env: envReport{
			PATH:  os.Getenv("PATH"),
			HOME:  os.Getenv("HOME"),
			SHELL: os.Getenv("SHELL"),
			USER:  os.Getenv("USER"),
			Cwd:   cwd,
		},
		Launch: LaunchIdentity{
			PID:        os.Getpid(),
			PPID:       os.Getppid(),
			Executable: exe,
			Cwd:        cwd,
			RunDir:     d.cfg.RunDir,
			HostSocket: d.cfg.HostSocket,
		},
		Checks: make([]ExecResult, 0, len(selected)),
	}
	for _, c := range selected {
		report.Checks = append(report.Checks, runFixed(ctx, c.Name, c.Argv, d.cfg.SmokeTimeout))
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	d.log.Info("native host smoke complete", "checks", len(report.Checks), "run_dir", d.cfg.RunDir)
	return report, nil
}

// PingNativeHost runs the configured fixed ping command. It is the only
// command the MCP ping tool executes and it takes no caller input.
func (d *Daemon) PingNativeHost(ctx context.Context) ExecResult {
	argv := d.cfg.PingCommand
	name := filepathBase(argv)
	return runFixed(ctx, name, argv, d.cfg.SmokeTimeout)
}

func filepathBase(argv []string) string {
	if len(argv) == 0 {
		return "ping"
	}
	base := argv[0]
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '/' {
			return base[i+1:]
		}
	}
	return base
}
