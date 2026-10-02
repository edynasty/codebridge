package daemon

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// maxCapturedOutput bounds each captured stream so a misbehaving fixed command
// cannot exhaust daemon memory.
const maxCapturedOutput = 64 << 10

// ExecResult is one executed fixed command.
type ExecResult struct {
	Name       string   `json:"name"`
	Argv       []string `json:"argv"`
	Started    bool     `json:"started"`
	ExitCode   int      `json:"exit_code"`
	TimedOut   bool     `json:"timed_out"`
	DurationMS int64    `json:"duration_ms"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	Err        string   `json:"error,omitempty"`
}

// limitedBuffer captures at most maxCapturedOutput bytes and records overflow.
type limitedBuffer struct {
	buf      bytes.Buffer
	overflow bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxCapturedOutput - l.buf.Len()
	if remaining <= 0 {
		l.overflow = true
		return len(p), nil
	}
	if len(p) > remaining {
		l.overflow = true
		p = p[:remaining]
	}
	l.buf.Write(p)
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	s := l.buf.String()
	if l.overflow {
		s += "\n[output truncated]"
	}
	return s
}

// runFixed executes one fixed argv with no shell, no stdin and a hard timeout.
// argv[0] is resolved against the daemon's PATH (an absolute path is used
// verbatim). The child inherits the daemon environment so the report reflects
// the daemon's real native host context.
func runFixed(ctx context.Context, name string, argv []string, timeout time.Duration) ExecResult {
	res := ExecResult{Name: name, Argv: append([]string(nil), argv...), ExitCode: -1}
	if len(argv) == 0 {
		res.Err = "empty command"
		return res
	}
	binary := argv[0]
	if !filepath.IsAbs(binary) {
		resolved, err := exec.LookPath(binary)
		if err != nil {
			res.Err = "executable not found on PATH: " + binary
			return res
		}
		binary = resolved
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, binary, argv[1:]...)
	stdout := &limitedBuffer{}
	stderr := &limitedBuffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = nil
	cmd.WaitDelay = 2 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	started := time.Now()
	res.Started = true
	err := cmd.Run()
	res.DurationMS = time.Since(started).Milliseconds()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if err == nil {
		res.ExitCode = 0
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		if runCtx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
			res.Err = "timed out after " + timeout.String()
		}
		return res
	}
	if runCtx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.Err = "timed out after " + timeout.String()
		return res
	}
	res.Err = err.Error()
	return res
}

// envReport is the daemon environment subset the smoke reports.
type envReport struct {
	PATH  string `json:"PATH"`
	HOME  string `json:"HOME"`
	SHELL string `json:"SHELL,omitempty"`
	USER  string `json:"USER,omitempty"`
	Cwd   string `json:"cwd,omitempty"`
}

// sanitizeCell keeps a captured line printable and bounded for reports.
func sanitizeCell(s string, max int) string {
	s = strings.TrimRight(s, "\r\n")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
