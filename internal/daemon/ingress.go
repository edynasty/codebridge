package daemon

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/hostipc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connCredKey carries the accepted connection's peer credentials into the
// request context so authentication never trusts a header for identity.
type connCredKey struct{}

// newIngressToken mints the per-launch bearer secret used by tunnel-client.
// The value lives in memory (and optionally in a 0600 file for local probes)
// and is never logged.
func newIngressToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("daemon: generate tunnel token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// writeTokenFile writes the per-launch secret to a 0600 file so a local probe
// can authenticate directly against the ingress. The tunnel path never reads
// it back: tunnel-client receives the secret through its environment.
func writeTokenFile(path, token string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("daemon: write token file %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("daemon: chmod token file %s: %w", path, err)
	}
	return nil
}

// ingressHandler builds the MCP Streamable HTTP handler wrapped in the ingress
// authentication middleware.
func (d *Daemon) ingressHandler() http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(d.serverForRequest, &mcp.StreamableHTTPOptions{
		Stateless:           true,
		Logger:              d.log,
		MaxRequestBodyBytes: 4 << 20,
	})
	return d.ingressAuth(mcpHandler)
}

// ingressAuth authenticates every MCP request without trusting any
// caller-supplied field:
//
//   - the connection's peer UID must be the daemon's own UID (the socket is
//     0600 in a 0700 directory, so this is a second, independent check);
//   - a request carrying the per-launch bearer secret is classified remote_ai;
//   - a request without it is same-user local_mcp and gets no elevated surface;
//   - a request carrying a wrong secret is rejected with 401.
//
// The Host header is validated as defense in depth (DNS rebinding); the
// listener is a Unix socket, so no TCP port is reachable.
func (d *Daemon) ingressAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host) {
			d.log.Warn("ingress rejected host header", "host", r.Host, "remote", r.RemoteAddr)
			http.Error(w, "forbidden: invalid Host header", http.StatusForbidden)
			return
		}
		cred, ok := r.Context().Value(connCredKey{}).(hostipc.PeerCred)
		if !ok || cred.UID != uint32(os.Getuid()) {
			d.log.Warn("ingress rejected peer uid", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized: peer uid", http.StatusUnauthorized)
			return
		}
		class := CallerLocalMCP
		if header := r.Header.Get("Authorization"); header != "" {
			secret, found := strings.CutPrefix(header, "Bearer ")
			if !found || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(secret)), []byte(d.token)) != 1 {
				d.log.Warn("ingress rejected bearer token", "remote", r.RemoteAddr, "peer_uid", cred.UID)
				http.Error(w, "unauthorized: bad bearer token", http.StatusUnauthorized)
				return
			}
			class = CallerRemoteAI
		}
		next.ServeHTTP(w, r.WithContext(withCallerClass(r.Context(), class)))
	})
}

// hostAllowed accepts the loopback host forms tunnel-client is configured with.
func hostAllowed(host string) bool {
	if host == "" {
		return false
	}
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	switch strings.ToLower(name) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// startMCPIngress binds the Streamable HTTP MCP endpoint to a Unix socket and
// serves it in the background. No TCP listener is opened in any configuration.
func (d *Daemon) startMCPIngress() error {
	ln, err := hostipc.ListenUnix(d.cfg.MCPSocket)
	if err != nil {
		return err
	}
	d.mcpLn = ln
	handler := d.ingressHandler()
	d.mcpSrv = &http.Server{
		Handler: handler,
		// The socket is local and authenticated per request; these bounds only
		// stop a stuck peer from holding resources.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       5 * time.Minute,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			uc, ok := c.(*net.UnixConn)
			if !ok {
				return ctx
			}
			if cred, err := hostipc.PeerCredentials(uc); err == nil {
				return context.WithValue(ctx, connCredKey{}, cred)
			}
			return ctx
		},
	}
	d.log.Info("mcp ingress listening", "transport", "streamable-http+unix", "socket", d.cfg.MCPSocket,
		"tunnel_token_env", d.cfg.Tunnel.TokenEnv, "token_file", d.cfg.Tunnel.TokenFile)
	go func() {
		if err := d.mcpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			d.reportErr(fmt.Errorf("mcp ingress: %w", err))
		}
	}()
	return nil
}

func (d *Daemon) stopMCPIngress(ctx context.Context) {
	if d.mcpSrv == nil {
		return
	}
	if err := d.mcpSrv.Shutdown(ctx); err != nil {
		d.log.Warn("mcp ingress shutdown", "error", err.Error())
		_ = d.mcpSrv.Close()
	}
}
