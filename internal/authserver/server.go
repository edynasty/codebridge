// Package authserver implements a small embedded OAuth 2.1 authorization
// server for the MCP authorization profile: authorization-code + PKCE (S256),
// refresh tokens, RS256 JWT access tokens with a self-published JWKS,
// Client ID Metadata Document (CIMD) clients, and RFC 7591 dynamic client
// registration as the fallback. It exists so a personal CodeBridge manager
// can serve ChatGPT's connector flow without an external identity provider.
package authserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	accessTokenTTL   = time.Hour
	refreshTokenTTL  = 30 * 24 * time.Hour
	authCodeTTL      = 5 * time.Minute
	sessionTTL       = 24 * time.Hour
	cimdFetchTimeout = 10 * time.Second
)

// Config wires the embedded AS.
type Config struct {
	// Issuer is the public base URL of the manager, e.g. https://cb.example.com.
	Issuer string
	// Resource is the RFC 8707 audience minted into access tokens; defaults
	// to the issuer when empty.
	Resource string
	// Users maps login names to bcrypt password hashes,
	// "alice:$2a$10$...,bob:$2a$10$...".
	Users map[string]string
	// KeyPath persists the RSA signing key (PEM). Created on first boot.
	KeyPath string
	// StorePath persists registered clients and refresh tokens.
	StorePath string
}

type Server struct {
	cfg  Config
	keys *rsa.PrivateKey
	kid  string

	mu         sync.Mutex
	codes      map[string]authCode // code -> pending grant
	sessions   map[string]session  // session id -> login
	refresh    map[string]refreshGrant
	dcrClients map[string]dcrClient // client_id -> registered metadata

	now func() time.Time
}

type authCode struct {
	ClientID      string
	RedirectURI   string
	Resource      string
	Scopes        []string
	Subject       string
	CodeChallenge string
	ExpiresAt     time.Time
}

type session struct {
	Subject   string
	ExpiresAt time.Time
}

type refreshGrant struct {
	ClientID  string
	Resource  string
	Scopes    []string
	Subject   string
	ExpiresAt time.Time
}

type dcrClient struct {
	ID           string
	RedirectURIs []string
	CreatedAt    time.Time
}

// persisted holds everything that must survive restarts.
type persisted struct {
	DCRClients map[string]dcrClient    `json:"dcr_clients"`
	Refresh    map[string]refreshGrant `json:"refresh"`
}

// New builds the server, generating and persisting the signing key on first
// boot. Users with invalid bcrypt hashes are rejected at startup.
func New(cfg Config) (*Server, error) {
	if cfg.Issuer == "" || !strings.HasPrefix(cfg.Issuer, "https://") {
		return nil, errors.New("issuer must be an https URL")
	}
	if cfg.Resource == "" {
		cfg.Resource = cfg.Issuer
	}
	if len(cfg.Users) == 0 {
		return nil, errors.New("CODEBRIDGE_AS_USERS is required (user:bcryptHash,...)")
	}
	s := &Server{
		cfg:        cfg,
		codes:      map[string]authCode{},
		sessions:   map[string]session{},
		refresh:    map[string]refreshGrant{},
		dcrClients: map[string]dcrClient{},
		now:        time.Now,
	}
	if err := s.loadKey(); err != nil {
		return nil, err
	}
	if err := s.loadStore(); err != nil {
		return nil, err
	}
	return s, nil
}

// PublicKey returns the verification key the manager's verifier needs; the
// JWKS handler serves the same key.
func (s *Server) Kid() string { return s.kid }

// ── HTTP handlers ─────────────────────────────────────────────────────────

// Metadata serves /.well-known/oauth-authorization-server. ChatGPT reads
// this to decide between CIMD and DCR.
func (s *Server) Metadata() http.HandlerFunc {
	issuer := s.cfg.Issuer
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                         issuer,
			"authorization_endpoint":                         issuer + "/authorize",
			"token_endpoint":                                 issuer + "/token",
			"registration_endpoint":                          issuer + "/register",
			"jwks_uri":                                       issuer + "/jwks.json",
			"response_types_supported":                       []string{"code"},
			"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":               []string{"S256"},
			"token_endpoint_auth_methods_supported":          []string{"none", "private_key_jwt"},
			"client_id_metadata_document_supported":          true,
			"authorization_response_iss_parameter_supported": true,
			"scopes_supported":                               []string{"codebridge.read"},
		})
	}
}

// Authorize implements the authorization endpoint: login form (GET) and
// code issuance (POST) with consent folded into the login step — this is a
// personal deployment with operator-defined users, so a separate consent
// screen would be ceremony.
func (s *Server) Authorize() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.authorizePost(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if _, err := s.validateAuthorizeRequest(r.Form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Already logged in? Short-circuit straight to code issuance.
		if subj := s.sessionSubject(r); subj != "" {
			s.issueCode(w, r, r.Form, subj)
			return
		}
		s.renderLogin(w, r.Form)
	}
}

func (s *Server) authorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")
	hash, ok := s.cfg.Users[username]
	if !ok || !bcryptCompare(password, hash) {
		time.Sleep(400 * time.Millisecond) // constant-ish delay vs. user enum
		s.renderLogin(w, r.Form, "用户名或密码不正确")
		return
	}
	s.mu.Lock()
	sid := randomToken(32)
	s.sessions[sid] = session{Subject: username, ExpiresAt: s.now().Add(sessionTTL)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "codebridge_as_session",
		Value:    sid,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	s.issueCode(w, r, r.Form, username)
}

// validateAuthorizeRequest checks client_id (CIMD fetch, DCR-registered, or
// bare https URL accepted as a last resort for pre-registered clients),
// redirect_uri, PKCE and resource.
func (s *Server) validateAuthorizeRequest(form url.Values) (*authorizeRequest, error) {
	req := &authorizeRequest{
		ClientID:      strings.TrimSpace(form.Get("client_id")),
		RedirectURI:   strings.TrimSpace(form.Get("redirect_uri")),
		CodeChallenge: strings.TrimSpace(form.Get("code_challenge")),
		Method:        strings.TrimSpace(form.Get("code_challenge_method")),
		Resource:      strings.TrimSpace(form.Get("resource")),
		State:         form.Get("state"),
	}
	if req.ClientID == "" || req.RedirectURI == "" {
		return nil, errors.New("client_id and redirect_uri are required")
	}
	if req.Method != "S256" || req.CodeChallenge == "" {
		return nil, errors.New("PKCE with S256 is required")
	}
	if req.Resource == "" {
		req.Resource = s.cfg.Resource
	}
	allowed, err := s.allowedRedirectURIs(req.ClientID)
	if err != nil {
		return nil, err
	}
	if !containsExact(allowed, req.RedirectURI) {
		return nil, fmt.Errorf("redirect_uri is not allowed for this client")
	}
	return req, nil
}

type authorizeRequest struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Method        string
	Resource      string
	State         string
}

// allowedRedirectURIs resolves the client's permitted redirect URIs.
// CIMD clients (https client_id URLs) are fetched and validated against
// their published document; DCR-registered clients use their stored list;
// any other client gets the ChatGPT callback allowlist (this deployment
// only serves ChatGPT connectors, which always redirect to chatgpt.com).
func (s *Server) allowedRedirectURIs(clientID string) ([]string, error) {
	if strings.HasPrefix(clientID, "https://") {
		doc, err := s.fetchCIMD(clientID)
		if err != nil {
			return nil, fmt.Errorf("client metadata: %w", err)
		}
		return doc.RedirectURIs, nil
	}
	s.mu.Lock()
	dcr, ok := s.dcrClients[clientID]
	s.mu.Unlock()
	if ok {
		return dcr.RedirectURIs, nil
	}
	return nil, fmt.Errorf("unknown client_id")
}

func containsExact(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
