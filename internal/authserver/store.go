package authserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ── key and store persistence ────────────────────────────────────────────

func (s *Server) loadKey() error {
	b, err := os.ReadFile(s.cfg.KeyPath)
	if err == nil {
		block, _ := pem.Decode(b)
		if block == nil {
			return errors.New("authserver key file is not PEM")
		}
		key, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes)
		if parseErr != nil {
			return fmt.Errorf("parse authserver key: %w", parseErr)
		}
		s.keys = key
	} else if errors.Is(err, os.ErrNotExist) {
		key, genErr := rsa.GenerateKey(rand.Reader, 2048)
		if genErr != nil {
			return genErr
		}
		b = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
		if err := os.WriteFile(s.cfg.KeyPath, b, 0o600); err != nil {
			return fmt.Errorf("persist authserver key: %w", err)
		}
		s.keys = key
	} else {
		return fmt.Errorf("read authserver key: %w", err)
	}
	// Stable kid derived from the public modulus so JWKS and JWTs stay
	// aligned across restarts.
	kidSum := base64.RawURLEncoding.EncodeToString(s.keys.PublicKey.N.Bytes())
	if len(kidSum) > 10 {
		kidSum = kidSum[:10]
	}
	s.kid = "cb-" + kidSum
	return nil
}

func (s *Server) loadStore() error {
	b, err := os.ReadFile(s.cfg.StorePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read authserver store: %w", err)
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("decode authserver store: %w", err)
	}
	if p.DCRClients != nil {
		s.dcrClients = p.DCRClients
	}
	if p.Refresh != nil {
		s.refresh = p.Refresh
	}
	s.prune()
	return nil
}

func (s *Server) persist() {
	p := persisted{DCRClients: s.dcrClients, Refresh: s.refresh}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	tmp := s.cfg.StorePath + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, s.cfg.StorePath)
	}
}

func (s *Server) prune() {
	now := s.now()
	for id, rt := range s.refresh {
		if rt.ExpiresAt.Before(now) {
			delete(s.refresh, id)
		}
	}
	for id, sess := range s.sessions {
		if sess.ExpiresAt.Before(now) {
			delete(s.sessions, id)
		}
	}
}

// ── sessions and login UI ─────────────────────────────────────────────────

func (s *Server) sessionSubject(r *http.Request) string {
	c, err := r.Cookie("codebridge_as_session")
	if err != nil || c.Value == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok || sess.ExpiresAt.Before(s.now()) {
		return ""
	}
	return sess.Subject
}

// renderLogin emits the password form, preserving the original query so the
// POST can complete the authorization flow.
func (s *Server) renderLogin(w http.ResponseWriter, q url.Values, errMsg ...string) {
	msg := ""
	if len(errMsg) > 0 {
		msg = errMsg[0]
	}
	escaped := url.Values{}
	for k, vs := range q {
		for _, v := range vs {
			escaped.Add(k, v)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>CodeBridge 登录</title>
<style>
body{margin:0;background:#f7f8f8;font-family:-apple-system,"PingFang SC",sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#fff;border:1px solid rgba(16,18,27,.11);border-radius:12px;padding:32px;width:320px;box-shadow:0 1px 3px rgba(16,18,27,.08)}
h1{font-size:15px;margin:0 0 4px}
p{color:#6b7280;font-size:12.5px;margin:0 0 18px}
input{width:100%;box-sizing:border-box;border:1px solid rgba(16,18,27,.16);border-radius:6px;padding:9px 11px;font-size:13px;margin-bottom:12px;background:#fff}
input:focus{outline:none;border-color:#4d58c4;box-shadow:0 0 0 3px rgba(77,88,196,.14)}
button{width:100%;background:#4d58c4;color:#fff;border:none;border-radius:6px;padding:10px;font-size:13px;font-weight:600;cursor:pointer}
.err{color:#dc2626;font-size:12px;margin:-6px 0 12px}
</style></head><body><div class="card">
<h1>CodeBridge 授权</h1><p>登录以允许该应用访问你的 CodeBridge</p>
`+(func() string {
		if msg != "" {
			return `<div class="err">` + msg + `</div>`
		}
		return ""
	}())+`
<form method="POST">
`+hiddenFields(q)+`
<input name="username" placeholder="用户名" autocomplete="username" required autofocus>
<input name="password" type="password" placeholder="密码" autocomplete="current-password" required>
<button type="submit">登录并授权</button>
</form></div></body></html>`)
}

func hiddenFields(q url.Values) string {
	var b strings.Builder
	for _, k := range []string{"client_id", "redirect_uri", "code_challenge", "code_challenge_method", "resource", "scope", "state"} {
		if v := q.Get(k); v != "" {
			fmt.Fprintf(&b, `<input type="hidden" name="%s" value="%s">`, k, htmlEscape(v))
		}
	}
	return b.String()
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}

func bcryptCompare(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ── authorization code issuance ──────────────────────────────────────────

func (s *Server) issueCode(w http.ResponseWriter, r *http.Request, form url.Values, subject string) {
	req, err := s.validateAuthorizeRequest(form)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scopes := strings.Fields(form.Get("scope"))
	s.mu.Lock()
	code := randomToken(32)
	s.codes[code] = authCode{
		ClientID: req.ClientID, RedirectURI: req.RedirectURI,
		Resource: req.Resource, Scopes: scopes, Subject: subject,
		CodeChallenge: req.CodeChallenge, ExpiresAt: s.now().Add(authCodeTTL),
	}
	// One-time codes: drop expired entries in the same pass.
	for c, v := range s.codes {
		if v.ExpiresAt.Before(s.now()) {
			delete(s.codes, c)
		}
	}
	s.mu.Unlock()

	redirect, _ := url.Parse(req.RedirectURI)
	qs := redirect.Query()
	qs.Set("code", code)
	if req.State != "" {
		qs.Set("state", req.State)
	}
	// RFC 9207 issuer identification; ChatGPT requires this.
	qs.Set("iss", s.cfg.Issuer)
	redirect.RawQuery = qs.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}
