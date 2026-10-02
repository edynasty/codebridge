package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPProbeResult is one tool call against the daemon's own ingress.
type MCPProbeResult struct {
	Tool       string          `json:"tool"`
	IsError    bool            `json:"is_error"`
	Structured json.RawMessage `json:"structured,omitempty"`
	Text       string          `json:"text,omitempty"`
}

// headerTransport injects a fixed header into every request.
type headerTransport struct {
	base http.RoundTripper
	auth string
}

func (h *headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	if h.auth != "" {
		clone.Header.Set("Authorization", h.auth)
	}
	return h.base.RoundTrip(clone)
}

// dialUnixClient returns an HTTP client whose transport always connects to the
// daemon's Unix socket, so a probe exercises the real tunnel-client code path
// (Streamable HTTP with a loopback URL) without any TCP listener. auth is sent
// verbatim as the Authorization header when non-empty.
func dialUnixClient(socketPath, auth string) *http.Client {
	return &http.Client{
		Transport: &headerTransport{
			base: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
				DisableCompression: true,
			},
			auth: auth,
		},
		Timeout: 30 * time.Second,
	}
}

// ProbeMCP connects to the ingress, authenticates with token (when non-empty)
// and calls one tool. token is never logged or echoed.
func ProbeMCP(ctx context.Context, socketPath, token, tool string, args map[string]any) (MCPProbeResult, error) {
	auth := ""
	if token != "" {
		auth = "Bearer " + token
	}
	httpClient := dialUnixClient(socketPath, auth)
	client := mcp.NewClient(&mcp.Implementation{Name: "codebridged-probe", Version: Version}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   "http://localhost/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		return MCPProbeResult{}, fmt.Errorf("mcp connect over %s: %w", socketPath, err)
	}
	defer func() { _ = session.Close() }()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return MCPProbeResult{}, fmt.Errorf("mcp call %s: %w", tool, err)
	}
	out := MCPProbeResult{Tool: tool, IsError: res.IsError}
	if res.StructuredContent != nil {
		if raw, merr := json.Marshal(res.StructuredContent); merr == nil {
			out.Structured = raw
		}
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if out.Text != "" {
				out.Text += "\n"
			}
			out.Text += tc.Text
		}
	}
	return out, nil
}

// ReadTokenFile reads the per-launch bearer secret for local probes. The value
// is returned to the caller and must not be printed.
func ReadTokenFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no token file configured")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}
