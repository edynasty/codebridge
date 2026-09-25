package main

import "testing"

func TestValidateManagerHost(t *testing.T) {
	good := []string{
		"127.0.0.1:8081",
		"manager:8081",
		"codebridge.example.com",
		"codebridge.example.com:443",
	}
	for _, raw := range good {
		if err := validateManagerHost(raw); err != nil {
			t.Errorf("validateManagerHost(%q) = %v, want nil", raw, err)
		}
	}
	for _, raw := range []string{"", "   ", "://", "https://", "ws://manager:8080/agent"} {
		if err := validateManagerHost(raw); err == nil {
			t.Errorf("validateManagerHost(%q) accepted", raw)
		}
	}
}
