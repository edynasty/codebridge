package oauthresource

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyRSAJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-key"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jwks" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []any{rsaJWK(kid, &key.PublicKey)},
		})
	}))
	defer server.Close()

	audience := "https://codebridge.example.test"
	v, err := New(Config{Issuer: server.URL, Audience: audience, JWKSURL: server.URL + "/jwks"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   server.URL,
		"aud":   audience,
		"sub":   "user-123",
		"exp":   now.Add(5 * time.Minute).Unix(),
		"nbf":   now.Add(-time.Minute).Unix(),
		"iat":   now.Unix(),
		"scope": "codebridge.read profile",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	info, err := v.Verify(context.Background(), signed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.UserID != "user-123" {
		t.Fatalf("user id = %q", info.UserID)
	}
	if !contains(info.Scopes, "codebridge.read") {
		t.Fatalf("scopes = %#v", info.Scopes)
	}
}

func TestVerifyRejectsWrongAudience(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	kid := "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{rsaJWK(kid, &key.PublicKey)}})
	}))
	defer server.Close()
	v, err := New(Config{Issuer: server.URL, Audience: "https://expected.example", JWKSURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": server.URL, "aud": "https://wrong.example", "sub": "u",
		"exp": now.Add(time.Minute).Unix(), "scope": "codebridge.read",
	})
	tok.Header["kid"] = kid
	signed, _ := tok.SignedString(key)
	if _, err := v.Verify(context.Background(), signed, nil); err == nil {
		t.Fatal("wrong audience was accepted")
	}
}

func TestRequireHTTPSOrLoopback(t *testing.T) {
	for _, good := range []string{"https://auth.example.com", "http://127.0.0.1:8080", "http://localhost:8080"} {
		if err := requireHTTPSOrLoopback(good); err != nil {
			t.Fatalf("%s: %v", good, err)
		}
	}
	if err := requireHTTPSOrLoopback("http://auth.example.com"); err == nil {
		t.Fatal("non-loopback HTTP issuer was accepted")
	}
}

func rsaJWK(kid string, pub *rsa.PublicKey) map[string]any {
	e := big.NewInt(int64(pub.E)).Bytes()
	return map[string]any{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(e),
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestVerifyRejectsMissingSubject(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	kid := "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{rsaJWK(kid, &key.PublicKey)}})
	}))
	defer server.Close()

	audience := "https://codebridge.example.test"
	v, err := New(Config{Issuer: server.URL, Audience: audience, JWKSURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": server.URL, "aud": audience,
		"exp": now.Add(time.Minute).Unix(), "scope": "codebridge.read",
	})
	tok.Header["kid"] = kid
	signed, _ := tok.SignedString(key)
	if _, err := v.Verify(context.Background(), signed, nil); err == nil {
		t.Fatal("token without subject was accepted")
	}
}

func TestVerifySubjectAllowlist(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	kid := "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{rsaJWK(kid, &key.PublicKey)}})
	}))
	defer server.Close()

	audience := "https://codebridge.example.test"
	v, err := New(Config{
		Issuer: server.URL, Audience: audience, JWKSURL: server.URL,
		AllowedSubjects: []string{"allowed-user"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	makeToken := func(sub string) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": server.URL, "aud": audience, "sub": sub,
			"exp": now.Add(time.Minute).Unix(), "scope": "codebridge.read",
		})
		tok.Header["kid"] = kid
		signed, _ := tok.SignedString(key)
		return signed
	}
	if _, err := v.Verify(context.Background(), makeToken("blocked-user"), nil); err == nil {
		t.Fatal("non-allowlisted subject was accepted")
	}
	if _, err := v.Verify(context.Background(), makeToken("allowed-user"), nil); err != nil {
		t.Fatalf("allowlisted subject was rejected: %v", err)
	}
}
