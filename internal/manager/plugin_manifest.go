package manager

import (
	"encoding/json"
	"net/http"
	"strings"
)

// GPT plugin manifest (OpenAI plugins specification, manifest_version 1).
//
// Security model: CodeBridge is an OAuth 2.1 resource server (RFC 9728),
// so the manifest declares no client credentials of its own — auth happens
// at the MCP endpoint via the protected-resource metadata it already
// serves. For local static-token deployments the manifest is not exposed
// at all (plugins only make sense for public HTTPS managers).
type pluginManifest struct {
	SchemaVersion       string      `json:"schema_version"`
	NameForHuman        string      `json:"name_for_human"`
	NameForModel        string      `json:"name_for_model"`
	DescriptionForHuman string      `json:"description_for_human"`
	DescriptionForModel string      `json:"description_for_model"`
	Auth                *pluginAuth `json:"auth"`
	API                 pluginAPI   `json:"api"`
	LogoURL             string      `json:"logo_url"`
	ContactEmail        string      `json:"contact_email"`
	LegalInfoURL        string      `json:"legal_info_url"`
}

// pluginAuth type "none": the plugin surface is the MCP endpoint itself,
// which enforces OAuth per RFC 9728. GPT clients that support the MCP
// connector perform the OAuth dance against the protected-resource
// metadata; no secrets are embedded in the manifest.
type pluginAuth struct {
	Type string `json:"type"`
}

type pluginAPI struct {
	Type            string `json:"type"`
	URL             string `json:"url"`
	IsConsequential bool   `json:"is_consequential"`
}

func (h *AdminHandler) ServePluginManifest(publicURL, resource, contactEmail string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(publicURL, "https://") {
			// Plugins are only meaningful for public HTTPS deployments; a
			// misconfigured manager must not advertise an insecure origin.
			http.Error(w, "plugin manifest requires CODEBRIDGE_PUBLIC_URL over HTTPS", http.StatusNotFound)
			return
		}
		manifest := pluginManifest{
			SchemaVersion:       "1",
			NameForHuman:        "CodeBridge",
			NameForModel:        "codebridge",
			DescriptionForHuman: "Read, browse and (when locally enabled) edit files and run allowlisted commands on your own machines, and delegate tasks to local coding subagents.",
			DescriptionForModel: "Access the user's local workspaces: list and read files, run permission-gated bash commands, and delegate self-contained tasks to local coding subagents. Tools operate on logical workspace names, never raw paths.",
			Auth:                &pluginAuth{Type: "none"},
			API: pluginAPI{
				Type:            "mcp",
				URL:             publicURL + "/mcp",
				IsConsequential: true,
			},
			LogoURL:      publicURL + "/.well-known/codebridge-logo.png",
			ContactEmail: contactEmail,
			LegalInfoURL: publicURL + "/.well-known/codebridge-legal.txt",
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		_ = json.NewEncoder(w).Encode(manifest)
	}
}
