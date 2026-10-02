package hostipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// HandlerFunc serves one authorized Host IPC request. It returns either a
// JSON-serializable result or an *Error, never both.
type HandlerFunc func(ctx context.Context, c *Conn, params json.RawMessage) (any, *Error)

// VerifyAppPeer validates the live process that claims role "app". A nil
// verifier refuses the app role outright: there is no unsigned app path.
type VerifyAppPeer func(pid int) error

// ServerOptions configures a Host IPC v1 listener.
type ServerOptions struct {
	// SocketPath is the Unix socket path (informational; Serve takes a listener).
	SocketPath string
	// Version is the daemon version reported in host.hello/host.health.
	Version string
	// Protocol is the daemon's protocol version. Clients may be one minor behind.
	Protocol Protocol
	// Capabilities are the service groups the daemon can offer.
	Capabilities []Capability
	// Limits bounds inbound frames.
	Limits Limits
	// Logger receives stable structured lines. Defaults to a discard logger.
	Logger *slog.Logger
	// UID is the expected peer UID. Zero selects os.Getuid().
	UID uint32
	// VerifyAppPeer is required for role "app".
	VerifyAppPeer VerifyAppPeer
	// AppRequirement is the code requirement text, logged for diagnostics only.
	AppRequirement string
	// HelloTimeout bounds how long an unauthenticated connection may stay open.
	HelloTimeout time.Duration
}

// Server is a Host IPC v1 listener. codebridged listens; CodeBridge.app
// connects.
type Server struct {
	opt      ServerOptions
	log      *slog.Logger
	started  time.Time
	closed   chan struct{}
	closeOne sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu       sync.Mutex
	ln       net.Listener
	conns    map[*Conn]struct{}
	handlers map[string]HandlerFunc
}

// NewServer builds a server. Handlers are registered with Handle.
func NewServer(opt ServerOptions) *Server {
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opt.UID == 0 {
		opt.UID = uint32(os.Getuid())
	}
	if opt.Protocol.Major == 0 {
		opt.Protocol = Protocol{Major: ProtocolMajor, Minor: DefaultProtocolMinor}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		opt:      opt,
		log:      opt.Logger,
		started:  time.Now(),
		closed:   make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
		conns:    map[*Conn]struct{}{},
		handlers: map[string]HandlerFunc{},
	}
}

// Handle registers the handler for a method. The method must exist in the
// frozen table; registering an unknown name panics, which is a programming
// error in the daemon rather than a runtime condition.
func (s *Server) Handle(method string, h HandlerFunc) {
	if _, ok := LookupMethod(method); !ok {
		panic("hostipc: Handle for unknown method " + method)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Started reports when the server began serving.
func (s *Server) Started() time.Time { return s.started }

// Protocol returns the daemon protocol version.
func (s *Server) Protocol() Protocol { return s.opt.Protocol }

// Serve accepts connections until the listener is closed or Close is called.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	s.log.Info("hostipc listening", "socket", s.opt.SocketPath, "protocol", s.opt.Protocol.String(), "uid", s.opt.UID)
	for {
		nc, err := ln.Accept()
		if err != nil {
			select {
			case <-s.closed:
				// Shutdown: wait for in-flight connections, then return.
				s.wg.Wait()
				return nil
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				s.wg.Wait()
				return nil
			}
			s.log.Error("hostipc accept failed", "error", err.Error())
			return err
		}
		unixConn, ok := nc.(*net.UnixConn)
		if !ok {
			s.log.Error("hostipc rejected non-unix connection", "remote", nc.RemoteAddr().String())
			_ = nc.Close()
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveConn(unixConn)
		}()
	}
}

// Close stops accepting, closes every connection and waits for handlers.
func (s *Server) Close() error {
	s.closeOne.Do(func() {
		close(s.closed)
		s.cancel()
		s.mu.Lock()
		ln := s.ln
		conns := make([]*Conn, 0, len(s.conns))
		for c := range s.conns {
			conns = append(conns, c)
		}
		s.mu.Unlock()
		if ln != nil {
			_ = ln.Close()
		}
		for _, c := range conns {
			c.Close()
		}
	})
	return nil
}

// ConnCount reports the live connection count (tests and health).
func (s *Server) ConnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// HasHandler reports whether a handler is registered for method, and whether
// the method exists in the frozen table at all.
func (s *Server) HasHandler(method string) (registered bool, known bool) {
	_, known = LookupMethod(method)
	_, registered = s.handler(method)
	return registered, known
}

// Broadcast sends a notification to every handshaken app connection.
func (s *Server) Broadcast(method string, params any) error {
	msg, err := NewNotification(method, params)
	if err != nil {
		return err
	}
	s.mu.Lock()
	targets := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c.handshaken() {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	for _, c := range targets {
		if err := c.send(msg); err != nil {
			s.log.Warn("hostipc notification failed", "method", method, "error", err.Error())
		}
	}
	return nil
}

func (s *Server) addConn(c *Conn) {
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) removeConn(c *Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
}

func (s *Server) handler(method string) (HandlerFunc, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.handlers[method]
	return h, ok
}

func (s *Server) serveConn(nc *net.UnixConn) {
	remote := nc.RemoteAddr().String()
	cred, err := PeerCredentials(nc)
	if err != nil {
		s.log.Error("hostipc peer credential read failed", "remote", remote, "error", err.Error())
		_ = nc.Close()
		return
	}
	if cred.UID != s.opt.UID {
		s.log.Error("hostipc rejected peer uid", "remote", remote, "peer_uid", cred.UID, "want_uid", s.opt.UID)
		_ = nc.Close()
		return
	}
	c := newConn(s, nc, cred)
	s.addConn(c)
	s.log.Info("hostipc connection accepted", "peer_uid", cred.UID, "peer_pid", cred.PID)
	defer func() {
		s.removeConn(c)
		c.Close()
		s.log.Info("hostipc connection closed", "peer_uid", cred.UID, "peer_pid", cred.PID, "role", string(c.role))
	}()

	if s.opt.HelloTimeout > 0 {
		_ = nc.SetReadDeadline(time.Now().Add(s.opt.HelloTimeout))
	}
	for {
		kind, payload, _, err := ReadFrame(nc, s.opt.Limits)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			var perr *ProtocolError
			if errors.As(err, &perr) {
				s.log.Warn("hostipc frame rejected", "peer_pid", cred.PID, "error", perr.Error())
				c.respondError(nil, NewError(perr.Code, perr.Reason, perr.Detail))
			} else {
				s.log.Warn("hostipc read failed", "peer_pid", cred.PID, "error", err.Error())
			}
			return
		}
		if kind == FrameAttachment {
			// No Phase 0 method carries attachments; an unsolicited attachment
			// frame is a protocol error rather than silent data loss.
			c.respondError(nil, NewError(CodeInvalidRequest, "invalid_request", "unsolicited attachment frame"))
			return
		}
		msg, derr := DecodeMessage(payload)
		if derr != nil {
			s.log.Warn("hostipc message rejected", "peer_pid", cred.PID, "reason", derr.Data.Reason)
			c.respondError(nil, derr)
			return
		}
		switch msg.Kind() {
		case KindRequest:
			if closeAfter := s.handleRequest(c, msg); closeAfter {
				return
			}
		case KindNotification:
			s.log.Warn("hostipc ignoring client notification", "method", msg.Method, "peer_pid", cred.PID)
		case KindResponse:
			c.deliverResponse(msg)
		default:
			c.respondError(msg.ID, NewError(CodeInvalidRequest, "invalid_request", "message has neither method nor id"))
		}
	}
}

// handleRequest dispatches one request. It reports whether the connection must
// be closed after the response (version/auth/protocol failures are fail closed).
func (s *Server) handleRequest(c *Conn, msg *Message) (closeAfter bool) {
	if msg.Method == MethodHello {
		return s.handleHello(c, msg)
	}
	if !c.handshaken() {
		c.respondError(msg.ID, errNotInitialized())
		return true
	}
	if _, aerr := Authorize(msg.Method, c.role); aerr != nil {
		s.log.Warn("hostipc request denied", "method", msg.Method, "role", string(c.role), "reason", aerr.Data.Reason)
		c.respondError(msg.ID, aerr)
		return false
	}
	// The handler registry is authoritative for availability: a method whose
	// Phase 0 row is not implemented stays unregistered (so it answers
	// unsupported), while an explicitly enabled method (the debug-gated probe,
	// or runtime.* once a store is wired) registers a handler.
	h, ok := s.handler(msg.Method)
	if !ok {
		c.respondError(msg.ID, errUnsupported(msg.Method))
		return false
	}
	result, herr := h(s.ctx, c, msg.Params)
	if herr != nil {
		s.log.Warn("hostipc handler error", "method", msg.Method, "role", string(c.role), "reason", herr.Data.Reason)
		c.respondError(msg.ID, herr)
		return false
	}
	if err := c.respond(msg.ID, result); err != nil {
		s.log.Warn("hostipc response write failed", "method", msg.Method, "error", err.Error())
		return true
	}
	return false
}

// handleHello performs the handshake. Every failure path closes the connection.
func (s *Server) handleHello(c *Conn, msg *Message) (closeAfter bool) {
	if c.handshaken() {
		c.respondError(msg.ID, NewError(CodeInvalidRequest, "invalid_request", "host.hello already completed"))
		return false
	}
	var params HelloParams
	if perr := DecodeParams(msg.Params, &params); perr != nil {
		c.respondError(msg.ID, perr)
		return true
	}
	if !params.Role.Known() {
		c.respondError(msg.ID, NewError(CodeInvalidParams, "unknown_role", fmt.Sprintf("role %q is not defined", params.Role)))
		return true
	}
	neg, verr := Negotiate(s.opt.Protocol, params.Protocol)
	if verr != nil {
		s.log.Warn("hostipc handshake rejected", "peer_pid", c.cred.PID, "role", string(params.Role),
			"client_protocol", params.Protocol.String(), "reason", verr.Data.Reason)
		c.respondError(msg.ID, verr)
		return true
	}

	verified := false
	if params.Role == RoleApp {
		// The live pid is re-read here: the value carried in hello is a hint
		// and is never trusted.
		livePID, err := PeerPID(c.nc)
		if err != nil {
			s.log.Error("hostipc app peer pid read failed", "peer_pid", c.cred.PID, "error", err.Error())
			c.respondError(msg.ID, errUnauthorized("peer_pid_unavailable"))
			return true
		}
		if livePID != c.cred.PID {
			s.log.Error("hostipc app peer pid changed", "pid_at_accept", c.cred.PID, "pid_at_hello", livePID)
			c.respondError(msg.ID, errUnauthorized("peer_pid_changed"))
			return true
		}
		if s.opt.VerifyAppPeer == nil {
			// Recorded blockage: no configured signing identity, so no app role.
			s.log.Error("hostipc app role refused: no signing identity configured",
				"peer_pid", c.cred.PID, "hint_bundle_id", params.BundleID, "hint_team_id", params.TeamID)
			c.respondError(msg.ID, errUnauthorized("signing_identity_not_configured"))
			return true
		}
		if err := s.opt.VerifyAppPeer(c.cred.PID); err != nil {
			reason := "app_signature_invalid"
			var serr *SignatureError
			if errors.As(err, &serr) && serr.Reason != "" {
				reason = serr.Reason
			}
			// Recorded blockage: the app role is refused, never downgraded.
			s.log.Error("hostipc app role refused",
				"peer_pid", c.cred.PID, "reason", reason, "error", err.Error())
			c.respondError(msg.ID, errUnauthorized(reason))
			return true
		}
		verified = true
	}

	c.setHandshaken(params.Role, neg, NegotiateCapabilities(s.opt.Capabilities, params.Capabilities), verified)
	if s.opt.HelloTimeout > 0 {
		_ = c.nc.SetReadDeadline(time.Time{})
	}
	result := HelloResult{
		Accepted:          true,
		Protocol:          neg,
		DaemonVersion:     s.opt.Version,
		Capabilities:      c.capabilities,
		Role:              params.Role,
		SignatureVerified: verified,
		PeerUID:           c.cred.UID,
	}
	s.log.Info("hostipc handshake accepted", "role", string(params.Role), "peer_pid", c.cred.PID,
		"class", params.Role.CallerClass(), "protocol", neg.String(), "signature_verified", verified,
		"client_version", params.AppVersion)
	if err := c.respond(msg.ID, result); err != nil {
		return true
	}
	return false
}

// Conn is one accepted Host IPC connection.
type Conn struct {
	srv  *Server
	nc   *net.UnixConn
	cred PeerCred

	mu                sync.Mutex
	wmu               sync.Mutex
	role              Role
	negotiated        Protocol
	capabilities      []Capability
	helloSeen         bool
	signatureVerified bool
	closed            bool
	pendSeq           uint64
	pending           map[string]chan *Message
}

func newConn(s *Server, nc *net.UnixConn, cred PeerCred) *Conn {
	return &Conn{srv: s, nc: nc, cred: cred, pending: map[string]chan *Message{}}
}

func (c *Conn) handshaken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.helloSeen
}

func (c *Conn) setHandshaken(role Role, p Protocol, caps []Capability, verified bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.role = role
	c.negotiated = p
	c.capabilities = caps
	c.helloSeen = true
	c.signatureVerified = verified
}

// Role reports the negotiated role (empty before the handshake).
func (c *Conn) Role() Role {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.role
}

// Class reports the Bridge caller class for this connection.
func (c *Conn) Class() string { return c.Role().CallerClass() }

// Protocol reports the negotiated protocol.
func (c *Conn) Protocol() Protocol {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.negotiated
}

// Capabilities reports the granted capabilities.
func (c *Conn) Capabilities() []Capability {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Capability, len(c.capabilities))
	copy(out, c.capabilities)
	return out
}

// SignatureVerified reports whether the live app signature was validated.
func (c *Conn) SignatureVerified() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.signatureVerified
}

// Peer returns the peer credentials read from the live socket.
func (c *Conn) Peer() PeerCred { return c.cred }

// Close closes the connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	return c.nc.Close()
}

// Notify sends a notification to this connection.
func (c *Conn) Notify(method string, params any) error {
	msg, err := NewNotification(method, params)
	if err != nil {
		return err
	}
	return c.send(msg)
}

// Request sends a daemon-initiated request and waits for the response.
func (c *Conn) Request(ctx context.Context, method string, params any, out any) *Error {
	c.mu.Lock()
	c.pendSeq++
	id := json.RawMessage(fmt.Sprintf("%q", fmt.Sprintf("d-%d", c.pendSeq)))
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
	if err := c.send(msg); err != nil {
		return errInternal(err.Error())
	}
	select {
	case <-ctx.Done():
		return errInternal("request cancelled")
	case <-c.srv.closed:
		return errInternal("server shutting down")
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

func (c *Conn) respond(id json.RawMessage, result any) error {
	msg, err := NewResponse(id, result)
	if err != nil {
		return c.send(NewErrorResponse(id, errInternal(err.Error())))
	}
	return c.send(msg)
}

func (c *Conn) respondError(id json.RawMessage, e *Error) {
	if e == nil {
		e = errInternal("unknown failure")
	}
	_ = c.send(NewErrorResponse(id, e))
}

func (c *Conn) send(msg *Message) error {
	payload, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return WriteFrame(c.nc, FrameJSON, payload, c.srv.opt.Limits)
}

func (c *Conn) deliverResponse(msg *Message) {
	c.mu.Lock()
	ch, ok := c.pending[string(msg.ID)]
	c.mu.Unlock()
	if !ok {
		c.srv.log.Warn("hostipc response for unknown request", "id", string(msg.ID))
		return
	}
	select {
	case ch <- msg:
	default:
	}
}
