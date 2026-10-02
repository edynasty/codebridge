package hostipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ListenUnix creates a Unix-domain listener for a per-user socket:
// the parent directory is created 0700 and the socket is chmod 0600, so only
// the owning user can reach it. A stale socket file is removed first; any other
// existing file at path is a hard error.
func ListenUnix(path string) (net.Listener, error) {
	// Darwin (and Linux) cap a Unix-socket path well under PATH_MAX; failing
	// here with a clear message beats a bare "invalid argument" from bind.
	if len(path) > 100 {
		return nil, fmt.Errorf("hostipc: socket path is %d bytes, which exceeds the 100-byte safe limit: %s", len(path), path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("hostipc: create socket dir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("hostipc: chmod socket dir %s: %w", dir, err)
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("hostipc: %s exists and is not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("hostipc: remove stale socket %s: %w", path, err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("hostipc: listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("hostipc: chmod socket %s: %w", path, err)
	}
	return ln, nil
}

// ClientOptions configures a Host IPC client (daemon-side probes/tests and the
// CLI control surface).
type ClientOptions struct {
	SocketPath   string
	Role         Role
	AppVersion   string
	Capabilities []Capability
	Limits       Limits
	// Timeout bounds the handshake and each Call. Zero selects 5s.
	Timeout time.Duration
	// OnNotification receives daemon-initiated notifications.
	OnNotification func(method string, params json.RawMessage)
}

// Client is a Host IPC v1 client.
type Client struct {
	nc  *net.UnixConn
	opt ClientOptions

	mu      sync.Mutex
	wmu     sync.Mutex
	pending map[string]chan *Message
	seq     uint64
	hello   HelloResult
	readErr error

	closeOne sync.Once
	done     chan struct{}
}

// Dial connects and performs the handshake. A handshake rejection returns the
// protocol *Error so callers can report the exact code and reason.
func Dial(ctx context.Context, opt ClientOptions) (*Client, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = 5 * time.Second
	}
	nc, err := net.Dial("unix", opt.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("hostipc: dial %s: %w", opt.SocketPath, err)
	}
	uc, ok := nc.(*net.UnixConn)
	if !ok {
		_ = nc.Close()
		return nil, fmt.Errorf("hostipc: %s is not a unix socket", opt.SocketPath)
	}
	c := &Client{nc: uc, opt: opt, pending: map[string]chan *Message{}, done: make(chan struct{})}
	go c.readLoop()
	hello, herr := c.helloHandshake(ctx)
	if herr != nil {
		c.Close()
		return nil, herr
	}
	c.hello = hello
	return c, nil
}

// Hello returns the handshake result.
func (c *Client) Hello() HelloResult { return c.hello }

// Close closes the connection.
func (c *Client) Close() error {
	c.closeOne.Do(func() {
		close(c.done)
		_ = c.nc.Close()
	})
	return nil
}

// Call performs one request and decodes the result into out.
func (c *Client) Call(ctx context.Context, method string, params any, out any) *Error {
	c.mu.Lock()
	c.seq++
	id := json.RawMessage(fmt.Sprintf("%q", fmt.Sprintf("c-%d", c.seq)))
	ch := make(chan *Message, 1)
	c.pending[string(id)] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, string(id))
		c.mu.Unlock()
	}()

	msg, err := NewRequest(id, method, params)
	if err != nil {
		return errInternal(err.Error())
	}
	if err := c.write(msg); err != nil {
		return errInternal(err.Error())
	}
	timer := time.NewTimer(c.opt.Timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errInternal("request cancelled")
	case <-timer.C:
		return errInternal("request timed out")
	case <-c.done:
		return errInternal("connection closed")
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if out != nil {
			if perr := DecodeParams(resp.Result, out); perr != nil {
				return perr
			}
		}
		return nil
	}
}

// Notify sends a notification.
func (c *Client) Notify(method string, params any) error {
	msg, err := NewNotification(method, params)
	if err != nil {
		return err
	}
	return c.write(msg)
}

func (c *Client) helloHandshake(ctx context.Context) (HelloResult, *Error) {
	params := HelloParams{
		Protocol:     Protocol{Major: ProtocolMajor, Minor: DefaultProtocolMinor},
		AppVersion:   c.opt.AppVersion,
		Role:         c.opt.Role,
		Capabilities: c.opt.Capabilities,
	}
	var result HelloResult
	if herr := c.Call(ctx, MethodHello, params, &result); herr != nil {
		return HelloResult{}, herr
	}
	return result, nil
}

func (c *Client) write(msg *Message) error {
	payload, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return WriteFrame(c.nc, FrameJSON, payload, c.opt.Limits)
}

func (c *Client) readLoop() {
	defer c.Close()
	for {
		kind, payload, _, err := ReadFrame(c.nc, c.opt.Limits)
		if err != nil {
			c.failAll(err)
			return
		}
		if kind != FrameJSON {
			c.failAll(fmt.Errorf("hostipc: unexpected attachment frame"))
			return
		}
		msg, derr := DecodeMessage(payload)
		if derr != nil {
			c.failAll(derr)
			return
		}
		switch msg.Kind() {
		case KindResponse:
			c.mu.Lock()
			ch, ok := c.pending[string(msg.ID)]
			c.mu.Unlock()
			if !ok {
				continue
			}
			select {
			case ch <- msg:
			default:
			}
		case KindNotification:
			if c.opt.OnNotification != nil {
				c.opt.OnNotification(msg.Method, msg.Params)
			}
		case KindRequest:
			// The daemon does not issue requests the CLI must answer in Phase 0.
			_ = c.write(NewErrorResponse(msg.ID, errUnsupported(msg.Method)))
		}
	}
}

func (c *Client) failAll(err error) {
	c.mu.Lock()
	c.readErr = err
	pend := c.pending
	c.pending = map[string]chan *Message{}
	c.mu.Unlock()
	for _, ch := range pend {
		select {
		case ch <- &Message{Error: errInternal(err.Error())}:
		default:
		}
	}
}

// ErrRead reports the terminal read error, if any.
func (c *Client) ErrRead() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}

// IsClosed reports whether the peer closed the connection.
func (c *Client) IsClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// IsProtocolError reports whether err is a Host IPC protocol error with the
// given code.
func IsProtocolError(err error, code int) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

// ErrorCode extracts the Host IPC error code, or 0 when err is not a protocol
// error.
func ErrorCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}
