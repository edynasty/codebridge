package manager

import (
	"testing"

	"github.com/edynasty/codebridge/internal/authstore"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAccountResolverForSubject(t *testing.T) {
	resolver := &AccountResolver{Map: map[string]string{"team-member": "team-a", "padded": " team-b "}}
	cases := []struct {
		subject string
		want    string
	}{
		{"", authstore.DefaultAccount},
		{"   ", authstore.DefaultAccount},
		{"team-member", "team-a"},
		{"  team-member  ", "team-a"},
		{"padded", "team-b"},
		{"unmapped-user", "unmapped-user"},
	}
	for _, c := range cases {
		if got := resolver.ForSubject(c.subject); got != c.want {
			t.Fatalf("ForSubject(%q) = %q, want %q", c.subject, got, c.want)
		}
	}
}

func TestAccountResolverNilIsSafe(t *testing.T) {
	var resolver *AccountResolver
	if got := resolver.ForSubject(""); got != authstore.DefaultAccount {
		t.Fatalf("nil resolver with no subject = %q, want default account", got)
	}
	if got := resolver.ForSubject("alice"); got != "alice" {
		t.Fatalf("nil resolver with subject = %q, want the subject itself", got)
	}
}

func TestToolServiceAccountFor(t *testing.T) {
	svc := &ToolService{Accounts: &AccountResolver{Map: map[string]string{"bob": "team-a"}}}

	if got := svc.accountFor(nil); got != authstore.DefaultAccount {
		t.Fatalf("request without token info = %q, want default account", got)
	}
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{
		TokenInfo: &mcpauth.TokenInfo{UserID: "bob"},
	}}
	if got := svc.accountFor(req); got != "team-a" {
		t.Fatalf("mapped subject = %q, want team-a", got)
	}
	req = &mcp.CallToolRequest{Extra: &mcp.RequestExtra{
		TokenInfo: &mcpauth.TokenInfo{UserID: "carol"},
	}}
	if got := svc.accountFor(req); got != "carol" {
		t.Fatalf("unmapped subject = %q, want the subject itself", got)
	}
}
