package main

import "testing"

func TestValidateManagerURL(t *testing.T) {
	good := []string{
		"wss://codebridge.example.com/agent",
		"ws://localhost:8080/agent",
		"ws://127.0.0.1:8080/agent",
		"ws://[::1]:8080/agent",
	}
	for _, raw := range good {
		if _, err := validateManagerURL(raw); err != nil {
			t.Errorf("%s rejected: %v", raw, err)
		}
	}

	bad := []string{
		"ws://codebridge.example.com/agent",
		"http://127.0.0.1:8080/agent",
		"wss:///agent",
		"wss://user:pass@codebridge.example.com/agent",
		"wss://codebridge.example.com/agent?token=secret",
		"wss://codebridge.example.com/agent#fragment",
	}
	for _, raw := range bad {
		if _, err := validateManagerURL(raw); err == nil {
			t.Errorf("%s was accepted", raw)
		}
	}
}
