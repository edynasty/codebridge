package authserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestCIMDLoopbackFlow(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.DefaultCost)
	s, err := New(Config{
		Issuer: "https://cb.example.com", Resource: "https://cb.example.com",
		Users:   map[string]string{"admin": string(hash)},
		KeyPath: t.TempDir() + "/k.pem", StorePath: t.TempDir() + "/s.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now() }

	form := url.Values{
		"client_id":             {"https://chatgpt.com/oauth/codex/client.json"},
		"redirect_uri":          {"http://127.0.0.1:57917/callback"},
		"code_challenge":        {pkceS256("v123")},
		"code_challenge_method": {"S256"},
		"resource":              {"https://cb.example.com"},
		"scope":                 {"codebridge.read"},
		"username":              {"admin"},
		"password":              {"pw"},
	}
	req := httptest.NewRequest(http.MethodPost, "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Authorize()(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d, body: %s", rec.Code, rec.Body.String()[:300])
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "code=") {
		t.Fatalf("no code in redirect: %s", loc)
	}
	t.Log("CIMD + loopback redirect OK:", loc[:70])
}
