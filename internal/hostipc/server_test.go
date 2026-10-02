package hostipc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testServer starts a Host IPC server on a short temp socket with a health
// handler and an injectable app-peer verifier.
func testServer(t *testing.T, minor int, verify VerifyAppPeer) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cbipc")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "ipc.sock")
	srv := NewServer(ServerOptions{
		SocketPath:    sock,
		Version:       "0.1.0-phase0",
		Protocol:      Protocol{Major: ProtocolMajor, Minor: minor},
		Capabilities:  []Capability{CapHost, CapNativeHostSmoke},
		VerifyAppPeer: verify,
		HelloTimeout:  5 * time.Second,
	})
	srv.Handle(MethodHealth, func(_ context.Context, c *Conn, _ json.RawMessage) (any, *Error) {
		return map[string]any{
			"role":               string(c.Role()),
			"class":              c.Class(),
			"signature_verified": c.SignatureVerified(),
			"protocol":           c.Protocol().String(),
			"capabilities":       c.Capabilities(),
		}, nil
	})
	ln, err := ListenUnix(sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}

func dialTest(t *testing.T, sock string, role Role) (*Client, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return Dial(ctx, ClientOptions{
		SocketPath: sock,
		Role:       role,
		AppVersion: "test",
		Capabilities: []Capability{
			CapHost, CapComputer, CapApproval, CapRuntime, CapNativeHostSmoke,
		},
		Timeout: 5 * time.Second,
	})
}

func waitClosed(t *testing.T, c *Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.IsClosed() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("connection stayed open; read error = %v", c.ErrRead())
}

func callHealth(t *testing.T, c *Client) map[string]any {
	t.Helper()
	var out map[string]any
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Call(ctx, MethodHealth, nil, &out); err != nil {
		t.Fatalf("host.health: %v", err)
	}
	return out
}

func TestAppRoleRequiresVerifiedPeer(t *testing.T) {
	sock := testServer(t, 0, func(pid int) error {
		if pid <= 0 {
			t.Fatalf("verifier called with pid %d", pid)
		}
		return nil
	})
	c, err := dialTest(t, sock, RoleApp)
	if err != nil {
		t.Fatalf("dial app: %v", err)
	}
	defer func() { _ = c.Close() }()
	hello := c.Hello()
	if !hello.SignatureVerified {
		t.Fatal("signature_verified should be true for a verified app peer")
	}
	if hello.Role != RoleApp {
		t.Fatalf("role = %q", hello.Role)
	}
	got := callHealth(t, c)
	if got["class"] != "local_ui" {
		t.Fatalf("class = %v, want local_ui", got["class"])
	}
	caps, ok := got["capabilities"].([]any)
	if !ok || len(caps) == 0 {
		t.Fatalf("capabilities = %v, want the negotiated subset", got["capabilities"])
	}
	for _, cap := range caps {
		if cap == string(CapComputer) || cap == string(CapApproval) {
			t.Fatalf("daemon granted %v although no Computer/approval service is implemented", cap)
		}
	}
}

func TestAppRoleRefusedWithoutVerifier(t *testing.T) {
	sock := testServer(t, 0, nil)
	_, err := dialTest(t, sock, RoleApp)
	if err == nil {
		t.Fatal("app role was accepted without a configured verifier")
	}
	if !IsProtocolError(err, CodeUnauthorizedPeer) {
		t.Fatalf("err = %v, want -32010", err)
	}
	var perr *Error
	if errors.As(err, &perr) && perr.Data.Reason != "signing_identity_not_configured" {
		t.Fatalf("reason = %q", perr.Data.Reason)
	}
}

func TestAppRoleRefusedWhenVerifierFails(t *testing.T) {
	sock := testServer(t, 0, func(pid int) error {
		return &SignatureError{PID: pid, Reason: "app_signature_invalid", Detail: "no identity"}
	})
	_, err := dialTest(t, sock, RoleApp)
	if !IsProtocolError(err, CodeUnauthorizedPeer) {
		t.Fatalf("err = %v, want -32010", err)
	}
}

func TestDiagnosticsNeedsNoSignatureAndIsLocalMCP(t *testing.T) {
	sock := testServer(t, 0, func(int) error {
		t.Fatal("verifier must not be called for a diagnostics peer")
		return nil
	})
	c, err := dialTest(t, sock, RoleDiagnostics)
	if err != nil {
		t.Fatalf("dial diagnostics: %v", err)
	}
	defer func() { _ = c.Close() }()
	if c.Hello().SignatureVerified {
		t.Fatal("diagnostics peer must not be reported as signature verified")
	}
	if got := callHealth(t, c); got["class"] != "local_mcp" {
		t.Fatalf("class = %v, want local_mcp", got["class"])
	}
}

func TestUnknownRoleRejected(t *testing.T) {
	sock := testServer(t, 0, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Dial(ctx, ClientOptions{SocketPath: sock, Role: Role("root"), Timeout: 5 * time.Second})
	if err == nil {
		t.Fatal("unknown role was accepted")
	}
	if !IsProtocolError(err, CodeInvalidParams) {
		t.Fatalf("err = %v, want -32602", err)
	}
}

func TestMajorMismatchClosesConnection(t *testing.T) {
	sock := testServer(t, 0, nil)
	c, perr := rawHello(t, sock, HelloParams{
		Protocol: Protocol{Major: 2, Minor: 0},
		Role:     RoleDiagnostics,
	})
	if perr == nil || perr.Code != CodeProtocolMajorMismatch {
		t.Fatalf("err = %v, want -32000", perr)
	}
	waitClosed(t, c)
}

func TestMinorNegotiationNAndNMinus1(t *testing.T) {
	cases := []struct {
		serverMinor int
		clientMinor int
		ok          bool
	}{
		{0, 0, true},
		{0, 1, false},
		{2, 2, true},
		{2, 1, true},
		{2, 0, false},
		{2, 3, false},
	}
	for _, tc := range cases {
		sock := testServer(t, tc.serverMinor, nil)
		c, perr := rawHello(t, sock, HelloParams{
			Protocol: Protocol{Major: 1, Minor: tc.clientMinor},
			Role:     RoleDiagnostics,
		})
		if tc.ok {
			if perr != nil {
				t.Fatalf("server minor %d, client minor %d: rejected with %v", tc.serverMinor, tc.clientMinor, perr)
			}
			if got := c.Hello().Protocol; got.Minor != tc.clientMinor {
				t.Fatalf("negotiated minor = %d, want %d", got.Minor, tc.clientMinor)
			}
			_ = c.Close()
			continue
		}
		if perr == nil {
			t.Fatalf("server minor %d, client minor %d: accepted, want rejection", tc.serverMinor, tc.clientMinor)
		}
		if perr.Code != CodeProtocolMinorUnsupported {
			t.Fatalf("server minor %d, client minor %d: code = %d, want -32001", tc.serverMinor, tc.clientMinor, perr.Code)
		}
		waitClosed(t, c)
	}
}

// rawHello dials, sends a handshake and returns the raw response error (if any).
func rawHello(t *testing.T, sock string, params HelloParams) (*Client, *Error) {
	t.Helper()
	nc, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	uc := nc.(*net.UnixConn)
	c := &Client{nc: uc, opt: ClientOptions{SocketPath: sock, Timeout: 3 * time.Second},
		pending: map[string]chan *Message{}, done: make(chan struct{})}
	go c.readLoop()
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var res HelloResult
	perr := c.Call(ctx, MethodHello, params, &res)
	if perr == nil {
		c.hello = res
	}
	return c, perr
}

func TestHelloRequiredBeforeOtherMethods(t *testing.T) {
	sock := testServer(t, 0, nil)
	nc, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	uc := nc.(*net.UnixConn)
	c := &Client{nc: uc, opt: ClientOptions{SocketPath: sock, Timeout: 3 * time.Second},
		pending: map[string]chan *Message{}, done: make(chan struct{})}
	go c.readLoop()
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = c.Call(ctx, MethodHealth, nil, nil)
	if !IsProtocolError(err, CodeNotInitialized) {
		t.Fatalf("err = %v, want -32002", err)
	}
	waitClosed(t, c)
}

func TestUnknownMethodIsUnsupported(t *testing.T) {
	sock := testServer(t, 0, nil)
	c, err := dialTest(t, sock, RoleDiagnostics)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cerr := c.Call(ctx, "computer.teleport", nil, nil)
	if !IsProtocolError(cerr, CodeUnsupported) {
		t.Fatalf("err = %v, want -32601", cerr)
	}
	if c.IsClosed() {
		t.Fatal("unsupported method must not close the connection")
	}
}

func TestPhase0UnimplementedMethodsAnswerUnsupported(t *testing.T) {
	sock := testServer(t, 0, func(int) error { return nil })
	c, err := dialTest(t, sock, RoleApp)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, method := range []string{
		// daemon -> app requests: the daemon does not serve what the app implements
		MethodComputerDescribe, MethodComputerTargets, MethodComputerOpenSession,
		MethodComputerObserve, MethodComputerAct, MethodComputerControl,
		MethodComputerCloseSession, MethodApprovalPresent, MethodApprovalCancel,
		// frozen app -> daemon methods Phase 0 deliberately does not implement
		MethodPrepareRestart, MethodApprovalDecision,
		MethodRuntimeHealth, MethodRuntimeEvents, MethodRuntimeCommand,
	} {
		cerr := c.Call(ctx, method, nil, nil)
		if !IsProtocolError(cerr, CodeUnsupported) {
			t.Fatalf("%s: err = %v, want -32601 unsupported", method, cerr)
		}
	}
}

func TestDirectionGuardAndRoleBoundaries(t *testing.T) {
	sock := testServer(t, 0, nil)
	for _, role := range []Role{RoleDiagnostics, RoleHarness} {
		c, err := dialTest(t, sock, role)
		if err != nil {
			t.Fatalf("dial %s: %v", role, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// Methods the daemon issues to the app are never served to any peer,
		// whatever its role: the direction guard answers unsupported.
		for _, method := range []string{
			MethodComputerDescribe, MethodComputerTargets, MethodComputerOpenSession,
			MethodComputerObserve, MethodComputerAct, MethodComputerControl,
			MethodComputerCloseSession, MethodApprovalPresent, MethodApprovalCancel,
			MethodNotifyPost, "computer.teleport",
		} {
			if cerr := c.Call(ctx, method, nil, nil); !IsProtocolError(cerr, CodeUnsupported) {
				t.Fatalf("%s as %s: err = %v, want -32601 unsupported", method, role, cerr)
			}
		}
		// Methods the daemon does serve (direction app -> daemon) but that this
		// role may not call: a CLI role never gets local_ui powers.
		for _, method := range []string{
			MethodPrepareRestart, MethodApprovalDecision,
			MethodRuntimeHealth, MethodRuntimeEvents, MethodRuntimeCommand,
		} {
			if cerr := c.Call(ctx, method, nil, nil); !IsProtocolError(cerr, CodeRoleForbidden) {
				t.Fatalf("%s as %s: err = %v, want -32011 role_forbidden", method, role, cerr)
			}
		}
		cancel()
		_ = c.Close()
	}
}

func TestSecondHelloRejected(t *testing.T) {
	sock := testServer(t, 0, func(int) error { return nil })
	c, err := dialTest(t, sock, RoleApp)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cerr := c.Call(ctx, MethodHello, HelloParams{Protocol: Protocol{Major: 1}, Role: RoleApp}, nil)
	if !IsProtocolError(cerr, CodeInvalidRequest) {
		t.Fatalf("err = %v, want -32600", cerr)
	}
}

func TestOversizeFrameClosesConnection(t *testing.T) {
	sock := testServer(t, 0, nil)
	nc, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	uc := nc.(*net.UnixConn)
	c := &Client{nc: uc, opt: ClientOptions{SocketPath: sock, Timeout: 3 * time.Second},
		pending: map[string]chan *Message{}, done: make(chan struct{})}
	go c.readLoop()
	t.Cleanup(func() { _ = c.Close() })
	// A JSON frame claiming MaxJSONFrame+1 bytes.
	header := binary.BigEndian.AppendUint32(nil, MaxJSONFrame+1)
	if _, err := uc.Write(header); err != nil {
		t.Fatalf("write header: %v", err)
	}
	waitClosed(t, c)
}

func TestSocketPermissions(t *testing.T) {
	sock := testServer(t, 0, nil)
	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %v, want 0600", perm)
	}
	dir, err := os.Stat(filepath.Dir(sock))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dir.Mode().Perm(); perm != 0o700 {
		t.Fatalf("socket dir mode = %v, want 0700", perm)
	}
}

func TestListenUnixRefusesNonSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-socket")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ListenUnix(path); err == nil {
		t.Fatal("ListenUnix replaced a regular file")
	}
}
