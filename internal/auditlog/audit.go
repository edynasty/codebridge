package auditlog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time       time.Time `json:"time"`
	Event      string    `json:"event"`
	RequestID  string    `json:"request_id,omitempty"`
	ActorID    string    `json:"actor_id,omitempty"`
	Tool       string    `json:"tool,omitempty"`
	DeviceID   string    `json:"device_id,omitempty"`
	Workspace  string    `json:"workspace,omitempty"`
	Success    *bool     `json:"success,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	ErrorKind  string    `json:"error_kind,omitempty"`
}

type Logger struct {
	mu     sync.Mutex
	writer io.Writer
	closer io.Closer
}

func New(path string) (*Logger, error) {
	if path == "" {
		return &Logger{writer: os.Stdout}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Logger{writer: f, closer: f}, nil
}

func (l *Logger) Log(event Event) error {
	if l == nil {
		return nil
	}
	if event.Event == "" {
		return errors.New("audit event name is required")
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.writer.Write(line)
	return err
}

func (l *Logger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	return l.closer.Close()
}

func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// The audit identifier is not a credential. Keep request handling alive
		// even if the OS entropy source unexpectedly fails.
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

func Bool(v bool) *bool { return &v }
