package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProbeResult is the result of one fixed probe subprocess run.
type ProbeResult struct {
	Probe    string   `json:"probe"`
	Argv     []string `json:"argv"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	TimedOut bool     `json:"timed_out"`
	Err      string   `json:"error,omitempty"`
}

// probeBinaryProbes are the probes executed by the standalone probe binary.
var probeBinaryProbes = map[string]bool{
	"permissions":   true,
	"signing":       true,
	"native-host":   true,
	"harness":       true,
	"lock-state":    true,
	"input-monitor": true,
	"list":          true,
	"all":           true,
}

// KnownProbes lists the accepted probe names.
func KnownProbes() []string {
	out := make([]string, 0, len(probeBinaryProbes))
	for name := range probeBinaryProbes {
		out = append(out, name)
	}
	return out
}

// ProbeEnabled reports whether the debug-gated probe method is available.
func (d *Daemon) ProbeEnabled() bool {
	return d.cfg.Phase0Debug && strings.TrimSpace(d.cfg.Phase0ProbePath) != ""
}

// ProbeBinary resolves the probe binary for a probe name: the harness binary for
// "harness" when configured, otherwise the standalone probe binary.
func (d *Daemon) ProbeBinary(probe string) string {
	if probe == "harness" && strings.TrimSpace(d.cfg.Phase0HarnessPath) != "" {
		return d.cfg.Phase0HarnessPath
	}
	return d.cfg.Phase0ProbePath
}

// RunPhase0Probe spawns the exact configured probe binary with a fixed argv
// (<binary> <probe> --json). The caller cannot supply an executable or extra
// argv, so this is a fixed probe, not a general exec surface.
func (d *Daemon) RunPhase0Probe(ctx context.Context, probe string, timeoutMS int) (ProbeResult, error) {
	if !probeBinaryProbes[probe] {
		return ProbeResult{}, fmt.Errorf("unknown probe %q", probe)
	}
	if !d.ProbeEnabled() {
		return ProbeResult{}, fmt.Errorf("probe method is disabled (set CODEBRIDGE_PHASE0_DEBUG=1 and CODEBRIDGE_PHASE0_PROBE)")
	}
	binary := d.ProbeBinary(probe)
	if !filepath.IsAbs(binary) {
		return ProbeResult{}, fmt.Errorf("probe binary %q must be an absolute path", binary)
	}
	fi, err := os.Stat(binary)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe binary %q: %w", binary, err)
	}
	if fi.IsDir() || fi.Mode()&0o111 == 0 {
		return ProbeResult{}, fmt.Errorf("probe binary %q is not an executable file", binary)
	}
	timeout := d.cfg.ProbeTimeout
	if timeoutMS > 0 {
		timeout = time.Duration(timeoutMS) * time.Millisecond
	}
	argv := []string{binary, probe, "--json"}
	res := runFixed(ctx, probe, argv, timeout)
	return ProbeResult{
		Probe:    probe,
		Argv:     res.Argv,
		ExitCode: res.ExitCode,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		TimedOut: res.TimedOut,
		Err:      res.Err,
	}, nil
}
