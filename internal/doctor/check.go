package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	PublicURL        string
	AdminURL         string
	AdminToken       string
	AccessToken      string
	DeviceID         string
	Workspace        string
	Timeout          time.Duration
	ExpectOAuth      bool
	ExpectAdminBlock bool
}

type Result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

type adminDevice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

func Run(ctx context.Context, opts Options) ([]Result, bool) {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: opts.Timeout}
	base, err := normalizeBaseURL(opts.PublicURL)
	if err != nil {
		return []Result{{Name: "public_url", OK: false, Detail: err.Error()}}, false
	}

	results := []Result{}
	add := func(name string, ok bool, detail string) {
		results = append(results, Result{Name: name, OK: ok, Detail: detail})
	}

	status, body, headers, err := request(ctx, client, http.MethodGet, base+"/healthz", "")
	if err != nil {
		add("health", false, err.Error())
	} else {
		add("health", status == http.StatusOK && strings.TrimSpace(string(body)) == "ok", fmt.Sprintf("HTTP %d", status))
	}

	status, body, _, err = request(ctx, client, http.MethodGet, base+"/.well-known/oauth-protected-resource", "")
	if err != nil {
		add("oauth_metadata", !opts.ExpectOAuth, err.Error())
	} else if status == http.StatusOK {
		var meta protectedResourceMetadata
		decodeErr := json.Unmarshal(body, &meta)
		ok := decodeErr == nil && meta.Resource != "" && len(meta.AuthorizationServers) > 0
		detail := fmt.Sprintf("resource=%s authorization_servers=%d scopes=%d", meta.Resource, len(meta.AuthorizationServers), len(meta.ScopesSupported))
		if decodeErr != nil {
			detail = decodeErr.Error()
		}
		add("oauth_metadata", ok, detail)
	} else {
		add("oauth_metadata", !opts.ExpectOAuth, fmt.Sprintf("HTTP %d", status))
	}

	status, _, headers, err = request(ctx, client, http.MethodGet, base+"/mcp", "")
	if err != nil {
		add("mcp_auth_challenge", false, err.Error())
	} else if opts.ExpectOAuth {
		challenge := headers.Get("WWW-Authenticate")
		ok := status == http.StatusUnauthorized &&
			strings.Contains(strings.ToLower(challenge), "bearer") &&
			strings.Contains(challenge, "resource_metadata=")
		add("mcp_auth_challenge", ok, fmt.Sprintf("HTTP %d; %s", status, challenge))
	} else {
		add("mcp_reachable", status != http.StatusNotFound, fmt.Sprintf("HTTP %d", status))
	}

	if opts.ExpectAdminBlock {
		status, _, _, err = request(ctx, client, http.MethodGet, base+"/admin/devices", "")
		if err != nil {
			add("public_admin_blocked", false, err.Error())
		} else {
			add("public_admin_blocked", status == http.StatusNotFound, fmt.Sprintf("HTTP %d", status))
		}
	}

	if strings.TrimSpace(opts.AccessToken) != "" {
		for _, result := range authenticatedMCPSmoke(ctx, base, opts) {
			add(result.Name, result.OK, result.Detail)
		}
	}

	if strings.TrimSpace(opts.Workspace) != "" && strings.TrimSpace(opts.DeviceID) == "" {
		add("mcp_project_info", false, "--workspace requires --device-id")
	}

	if strings.TrimSpace(opts.AdminURL) != "" {
		adminBase, parseErr := normalizeBaseURL(opts.AdminURL)
		if parseErr != nil {
			add("admin_url", false, parseErr.Error())
		} else if strings.TrimSpace(opts.AdminToken) == "" {
			add("admin_auth", false, "admin token is required when --admin-url is set")
		} else {
			status, body, _, err = request(ctx, client, http.MethodGet, adminBase+"/admin/devices", opts.AdminToken)
			if err != nil {
				add("admin_devices", false, err.Error())
			} else if status != http.StatusOK {
				add("admin_devices", false, fmt.Sprintf("HTTP %d", status))
			} else {
				var devices []adminDevice
				if err := json.Unmarshal(body, &devices); err != nil {
					add("admin_devices", false, err.Error())
				} else {
					add("admin_devices", true, fmt.Sprintf("devices=%d", len(devices)))
					if strings.TrimSpace(opts.DeviceID) != "" {
						found := false
						online := false
						for _, d := range devices {
							if d.ID == opts.DeviceID {
								found = true
								online = d.Online
								break
							}
						}
						switch {
						case !found:
							add("device_online", false, fmt.Sprintf("device %q not found", opts.DeviceID))
						case !online:
							add("device_online", false, fmt.Sprintf("device %q is offline", opts.DeviceID))
						default:
							add("device_online", true, fmt.Sprintf("device %q is online", opts.DeviceID))
						}
					}
				}
			}
		}
	}

	allOK := true
	for _, result := range results {
		if !result.OK {
			allOK = false
		}
	}
	return results, allOK
}

func authenticatedMCPSmoke(ctx context.Context, base string, opts Options) []Result {
	results := []Result{}
	add := func(name string, ok bool, detail string) {
		results = append(results, Result{Name: name, OK: ok, Detail: detail})
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	smokeCtx, cancel := context.WithTimeout(ctx, 2*timeout)
	defer cancel()

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: bearerTransport{
			token: strings.TrimSpace(opts.AccessToken),
			base:  http.DefaultTransport,
		},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "CodeBridge Doctor", Version: "doctor"}, nil)
	session, err := client.Connect(smokeCtx, &mcp.StreamableClientTransport{
		Endpoint:   base + "/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		add("mcp_authenticated", false, err.Error())
		return results
	}
	defer session.Close()

	tools, err := session.ListTools(smokeCtx, nil)
	if err != nil {
		add("mcp_authenticated", false, "list tools: "+err.Error())
		return results
	}
	hasListDevices := false
	for _, tool := range tools.Tools {
		if tool.Name == "list_devices" {
			hasListDevices = true
			break
		}
	}
	if !hasListDevices {
		add("mcp_authenticated", false, "list_devices tool is not advertised")
		return results
	}

	deviceResult, err := session.CallTool(smokeCtx, &mcp.CallToolParams{Name: "list_devices"})
	if err != nil {
		add("mcp_authenticated", false, "list_devices: "+err.Error())
		return results
	}
	if deviceResult.IsError {
		add("mcp_authenticated", false, "list_devices returned a tool error")
		return results
	}
	var devices []struct {
		ID string `json:"id"`
	}
	if err := decodeToolJSON(deviceResult, &devices); err != nil {
		add("mcp_authenticated", false, "decode list_devices: "+err.Error())
		return results
	}
	add("mcp_authenticated", true, fmt.Sprintf("tools=%d devices=%d", len(tools.Tools), len(devices)))

	deviceID := strings.TrimSpace(opts.DeviceID)
	if deviceID == "" {
		return results
	}
	foundDevice := false
	for _, device := range devices {
		if device.ID == deviceID {
			foundDevice = true
			break
		}
	}
	if !foundDevice {
		add("mcp_device", false, fmt.Sprintf("device %q is not listed by MCP", deviceID))
		return results
	}

	workspaceResult, err := session.CallTool(smokeCtx, &mcp.CallToolParams{
		Name:      "list_workspaces",
		Arguments: map[string]any{"device_id": deviceID},
	})
	if err != nil {
		add("mcp_device", false, "list_workspaces: "+err.Error())
		return results
	}
	if workspaceResult.IsError {
		add("mcp_device", false, "list_workspaces returned a tool error")
		return results
	}
	var workspaces []struct {
		Name string `json:"name"`
		Path string `json:"path,omitempty"`
	}
	if err := decodeToolJSON(workspaceResult, &workspaces); err != nil {
		add("mcp_device", false, "decode list_workspaces: "+err.Error())
		return results
	}
	for _, workspace := range workspaces {
		if workspace.Path != "" {
			add("mcp_device", false, "workspace response exposed a physical path")
			return results
		}
	}
	add("mcp_device", true, fmt.Sprintf("device=%s workspaces=%d", deviceID, len(workspaces)))

	workspaceName := strings.TrimSpace(opts.Workspace)
	if workspaceName == "" {
		return results
	}
	foundWorkspace := false
	for _, workspace := range workspaces {
		if workspace.Name == workspaceName {
			foundWorkspace = true
			break
		}
	}
	if !foundWorkspace {
		add("mcp_project_info", false, fmt.Sprintf("workspace %q is not advertised by device %q", workspaceName, deviceID))
		return results
	}

	projectResult, err := session.CallTool(smokeCtx, &mcp.CallToolParams{
		Name: "project_info",
		Arguments: map[string]any{
			"device_id": deviceID,
			"workspace": workspaceName,
		},
	})
	if err != nil {
		add("mcp_project_info", false, "project_info: "+err.Error())
		return results
	}
	if projectResult.IsError {
		add("mcp_project_info", false, "project_info returned a tool error")
		return results
	}
	var projectInfo struct {
		Markers []string `json:"markers"`
		HasGit  bool     `json:"has_git"`
	}
	if err := decodeToolJSON(projectResult, &projectInfo); err != nil {
		add("mcp_project_info", false, "decode project_info: "+err.Error())
		return results
	}
	add("mcp_project_info", true, fmt.Sprintf("workspace=%s markers=%d has_git=%t", workspaceName, len(projectInfo.Markers), projectInfo.HasGit))
	return results
}

func decodeToolJSON(result *mcp.CallToolResult, out any) error {
	if result == nil || len(result.Content) == 0 {
		return fmt.Errorf("tool returned no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return fmt.Errorf("unexpected content type %T", result.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), out); err != nil {
		return err
	}
	return nil
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+b.token)
	base := b.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid URL %q", raw)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("URL must use http or https")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("URL must not contain query or fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("URL must be an origin without a path")
	}
	u.Path = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func request(ctx context.Context, client *http.Client, method, target, bearer string) (int, []byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Clone(), err
	}
	return resp.StatusCode, body, resp.Header.Clone(), nil
}
