package authserver

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token implements the token endpoint: authorization_code (+PKCE) and
// refresh_token grants. Access tokens are RS256 JWTs with aud = resource.
func (s *Server) Token() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			tokenError(w, "invalid_request", "bad form")
			return
		}
		switch r.PostFormValue("grant_type") {
		case "authorization_code":
			s.tokenCode(w, r)
		case "refresh_token":
			s.tokenRefresh(w, r)
		default:
			tokenError(w, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
		}
	}
}

func (s *Server) tokenCode(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.PostFormValue("code"))
	verifier := strings.TrimSpace(r.PostFormValue("code_verifier"))
	clientID := strings.TrimSpace(r.PostFormValue("client_id"))
	redirectURI := strings.TrimSpace(r.PostFormValue("redirect_uri"))

	s.mu.Lock()
	gr, ok := s.codes[code]
	if ok {
		delete(s.codes, code)
	}
	s.mu.Unlock()
	if !ok {
		tokenError(w, "invalid_grant", "authorization code is invalid or expired")
		return
	}
	if gr.ExpiresAt.Before(s.now()) {
		tokenError(w, "invalid_grant", "authorization code is expired")
		return
	}
	if clientID != gr.ClientID || redirectURI != gr.RedirectURI {
		tokenError(w, "invalid_grant", "client_id or redirect_uri does not match the authorization request")
		return
	}
	if pkceS256(verifier) != gr.CodeChallenge {
		tokenError(w, "invalid_grant", "PKCE verification failed")
		return
	}
	s.issueTokens(w, gr.ClientID, gr.Subject, gr.Resource, gr.Scopes)
}

func (s *Server) tokenRefresh(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.PostFormValue("refresh_token"))
	clientID := strings.TrimSpace(r.PostFormValue("client_id"))

	s.mu.Lock()
	gr, ok := s.refresh[raw]
	if ok {
		delete(s.refresh, raw) // rotation: single use
	}
	s.mu.Unlock()
	if !ok || gr.ExpiresAt.Before(s.now()) {
		tokenError(w, "invalid_grant", "refresh token is invalid or expired")
		return
	}
	if gr.ClientID != clientID {
		tokenError(w, "invalid_grant", "refresh token belongs to a different client")
		return
	}
	s.issueTokens(w, gr.ClientID, gr.Subject, gr.Resource, gr.Scopes)
}

func (s *Server) issueTokens(w http.ResponseWriter, clientID, subject, resource string, scopes []string) {
	now := s.now()
	access, err := s.signAccessToken(now, clientID, subject, resource, scopes)
	if err != nil {
		tokenError(w, "server_error", "signing failed")
		return
	}
	refresh := randomToken(32)
	s.mu.Lock()
	s.refresh[refresh] = refreshGrant{
		ClientID: clientID, Resource: resource, Scopes: scopes,
		Subject: subject, ExpiresAt: now.Add(refreshTokenTTL),
	}
	s.prune()
	s.mu.Unlock()
	s.persist()

	writeJSON(w, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"refresh_token": refresh,
		"scope":         strings.Join(scopes, " "),
	})
}

func (s *Server) signAccessToken(now time.Time, clientID, subject, resource string, scopes []string) (string, error) {
	claims := jwt.MapClaims{
		"iss": s.cfg.Issuer,
		"sub": subject,
		"aud": resource,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(accessTokenTTL).Unix(),
		"jti": randomToken(16),
	}
	if len(scopes) > 0 {
		claims["scope"] = strings.Join(scopes, " ")
	}
	if clientID != "" {
		claims["client_id"] = clientID
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.kid
	return tok.SignedString(s.keys)
}

func tokenError(w http.ResponseWriter, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
}

// ── JWKS ──────────────────────────────────────────────────────────────────

// JWKS serves the public signing key.
func (s *Server) JWKS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pub, ok := s.keys.Public().(*rsa.PublicKey)
		if !ok {
			http.Error(w, "key unavailable", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"keys": []any{map[string]any{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": s.kid,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	}
}

// ── dynamic client registration (RFC 7591) ────────────────────────────────

// Register accepts any DCR request and returns a fresh client_id with the
// caller's redirect URIs stored. This is a personal deployment: the AS is
// already public and rate-limited by the manager; we keep the register
// endpoint open so ChatGPT's DCR fallback works, but only redirect URIs on
// the ChatGPT callback host pass authorize-time validation anyway.
func (s *Server) Register() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var in struct {
			RedirectURIs []string `json:"redirect_uris"`
			ClientName   string   `json:"client_name"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		clientID := "cb-" + randomToken(12)
		s.mu.Lock()
		s.dcrClients[clientID] = dcrClient{ID: clientID, RedirectURIs: in.RedirectURIs, CreatedAt: s.now()}
		s.mu.Unlock()
		s.persist()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":                  clientID,
			"client_id_issued_at":        s.now().Unix(),
			"redirect_uris":              in.RedirectURIs,
			"token_endpoint_auth_method": "none",
			"grant_types":                []string{"authorization_code", "refresh_token"},
		})
	}
}

// ── CIMD client validation ────────────────────────────────────────────────

// cimdDocument is the metadata ChatGPT publishes at its client_id URL.
type cimdDocument struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// builtinCIMD carries the published Client ID Metadata Documents of the
// [OI] hosts this deployment serves. They are pinned so authorization
// works even where the network cannot reach chatgpt.com (mainland China);
// fetchCIMD still tries the live document first and falls back to these.
var builtinCIMD = map[string]cimdDocument{
	"https://chatgpt.com/oauth/client.json": {
		ClientID:   "https://chatgpt.com/oauth/client.json",
		ClientName: "ChatGPT",
		RedirectURIs: []string{
			"https://chatgpt.com/connector_platform_oauth_redirect",
			"https://chatgpt.com/aip/connector/callback",
			"https://chatgpt.com/backend-api/codex/connect",
		},
	},
	"https://chatgpt.com/oauth/codex/client.json": {
		ClientID:   "https://chatgpt.com/oauth/codex/client.json",
		ClientName: "Codex",
		RedirectURIs: []string{
			"http://127.0.0.1/callback",
			"http://localhost/callback",
			"http://127.0.0.1:57917/callback",
		},
	},
}

// fetchCIMD retrieves and sanity-checks a Client ID Metadata Document. The
// live document is preferred; when it is unreachable (blocked networks are
// common for personal deployments) the pinned builtin copy applies.
func (s *Server) fetchCIMD(clientID string) (*cimdDocument, error) {
	if !strings.HasPrefix(clientID, "https://") || !strings.Contains(strings.TrimPrefix(clientID, "https://"), "/") {
		return nil, fmt.Errorf("client_id is not an HTTPS metadata URL")
	}
	client := &http.Client{Timeout: cimdFetchTimeout}
	resp, err := client.Get(clientID)
	if err != nil {
		if doc, ok := builtinCIMD[clientID]; ok {
			return &doc, nil
		}
		return nil, fmt.Errorf("fetch client metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if doc, ok := builtinCIMD[clientID]; ok {
			return &doc, nil
		}
		return nil, fmt.Errorf("client metadata returned %d", resp.StatusCode)
	}
	var doc cimdDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode client metadata: %w", err)
	}
	if doc.ClientID != clientID {
		return nil, fmt.Errorf("client metadata client_id mismatch")
	}
	return &doc, nil
}

// isChatGPTRedirect reports whether the URI is on the ChatGPT connector
// callback host (the only legit destination for this personal deployment).
func isChatGPTRedirect(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && (u.Host == "chatgpt.com" || u.Host == "www.chatgpt.com")
}
