package authserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestAdminPasswordLogin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Si6UOArEzIWotQPWyZSB"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{
		Issuer: "https://cb.example.com", Resource: "https://cb.example.com",
		Users:   map[string]string{"admin": string(hash)},
		KeyPath: t.TempDir() + "/k.pem", StorePath: t.TempDir() + "/s.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now() }

	// register a DCR client
	reg := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"redirect_uris":["https://chatgpt.com/aip/connector/callback"]}`))
	regRec := httptest.NewRecorder()
	s.Register()(regRec, reg)
	var dcr struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(regRec.Body.Bytes(), &dcr); err != nil || dcr.ClientID == "" {
		t.Fatalf("dcr failed: %v %s", err, regRec.Body.String())
	}

	form := url.Values{
		"client_id":             {dcr.ClientID},
		"redirect_uri":          {"https://chatgpt.com/aip/connector/callback"},
		"code_challenge":        {pkceS256("verifier123")},
		"code_challenge_method": {"S256"},
		"resource":              {"https://cb.example.com"},
		"scope":                 {"codebridge.read"},
		"username":              {"admin"},
		"password":              {"Si6UOArEzIWotQPWyZSB"},
	}
	req := httptest.NewRequest(http.MethodPost, "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Authorize()(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d; body head: %s", rec.Code, rec.Body.String()[:200])
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "code=") {
		t.Fatalf("redirect missing code: %s", loc)
	}
	t.Log("login+code OK:", loc[:60])
}
