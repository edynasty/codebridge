package main

import "testing"

func TestLoadOAuthConfigDisabled(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "")
	cfg, err := loadOAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Fatalf("expected OAuth disabled, got %#v", cfg)
	}
}

func TestLoadOAuthConfig(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "https://codebridge.example.com/")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "https://auth.example.com/tenant/")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "https://auth.example.com/tenant/jwks")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "")
	t.Setenv("CODEBRIDGE_OAUTH_SCOPE", "codebridge.read")

	cfg, err := loadOAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://codebridge.example.com" {
		t.Fatalf("public URL = %q", cfg.PublicURL)
	}
	if cfg.Resource != "https://codebridge.example.com" {
		t.Fatalf("resource = %q", cfg.Resource)
	}
	if cfg.Issuer != "https://auth.example.com/tenant/" {
		t.Fatalf("issuer was normalized unexpectedly: %q", cfg.Issuer)
	}
}

func TestLoadOAuthConfigRejectsPartialConfiguration(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "https://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "")
	if _, err := loadOAuthConfig(); err == nil {
		t.Fatal("partial OAuth configuration was accepted")
	}
}

func TestLoadOAuthConfigRejectsInsecureResource(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "https://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "https://auth.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "https://auth.example.com/jwks")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "http://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_SCOPE", "codebridge.read")
	if _, err := loadOAuthConfig(); err == nil {
		t.Fatal("insecure non-loopback OAuth resource was accepted")
	}
}

func TestLoadOAuthConfigRejectsMultipleScopesInSingularSetting(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "https://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "https://auth.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "https://auth.example.com/jwks")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "https://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_SCOPE", "codebridge.read profile")
	if _, err := loadOAuthConfig(); err == nil {
		t.Fatal("multiple scope values were accepted in CODEBRIDGE_OAUTH_SCOPE")
	}
}

func TestLoadOAuthConfigAllowedSubjects(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "https://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "https://auth.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "https://auth.example.com/jwks")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "https://codebridge.example.com")
	t.Setenv("CODEBRIDGE_OAUTH_SCOPE", "codebridge.read")
	t.Setenv("CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS", " user-a, user-b,user-a ")

	cfg, err := loadOAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedSubjects) != 2 || cfg.AllowedSubjects[0] != "user-a" || cfg.AllowedSubjects[1] != "user-b" {
		t.Fatalf("allowed subjects = %#v", cfg.AllowedSubjects)
	}
}

func TestLoadOAuthConfigRejectsAllowlistWithoutOAuth(t *testing.T) {
	t.Setenv("CODEBRIDGE_PUBLIC_URL", "")
	t.Setenv("CODEBRIDGE_OAUTH_ISSUER", "")
	t.Setenv("CODEBRIDGE_OAUTH_JWKS_URL", "")
	t.Setenv("CODEBRIDGE_OAUTH_RESOURCE", "")
	t.Setenv("CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS", "user-a")
	if _, err := loadOAuthConfig(); err == nil {
		t.Fatal("subject allowlist without OAuth configuration was accepted")
	}
}
