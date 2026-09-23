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
)

type Options struct {
	PublicURL        string
	AdminURL         string
	AdminToken       string
	DeviceID         string
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
