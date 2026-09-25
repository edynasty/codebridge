package manager

import (
	"encoding/json"
	"testing"
)

func TestReadOnlyToolDeclaresOAuthSecurityScheme(t *testing.T) {
	svc := &ToolService{OAuthScopes: []string{"codebridge.read"}}
	tool := svc.readOnlyTool("read", "read")
	raw, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor map[string]any
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		t.Fatal(err)
	}
	meta, ok := descriptor["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("tool metadata missing: %s", raw)
	}
	schemes, ok := meta["securitySchemes"].([]any)
	if !ok || len(schemes) != 1 {
		t.Fatalf("securitySchemes missing: %s", raw)
	}
	scheme, ok := schemes[0].(map[string]any)
	if !ok || scheme["type"] != "oauth2" {
		t.Fatalf("unexpected security scheme: %#v", schemes[0])
	}
	scopes, ok := scheme["scopes"].([]any)
	if !ok || len(scopes) != 1 || scopes[0] != "codebridge.read" {
		t.Fatalf("unexpected scopes: %#v", scheme["scopes"])
	}
}

func TestReadOnlyToolOmitsSecuritySchemeWithoutOAuth(t *testing.T) {
	svc := &ToolService{}
	tool := svc.readOnlyTool("read", "read")
	if tool.Meta != nil {
		if _, exists := tool.Meta["securitySchemes"]; exists {
			t.Fatal("OAuth metadata present when OAuth is disabled")
		}
	}
}
