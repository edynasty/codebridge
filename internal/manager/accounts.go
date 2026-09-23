package manager

import (
	"strings"

	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AccountResolver binds OAuth subjects to tenant accounts.
type AccountResolver struct {
	Map map[string]string
}

// ForSubject resolves the account for an authenticated subject. An empty
// subject (OAuth disabled, local development) maps to the default account.
// Unmapped subjects resolve to their own account so no subject can reach
// another tenant's devices without an explicit operator mapping.
func (a *AccountResolver) ForSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return authstore.DefaultAccount
	}
	if a != nil {
		if mapped := strings.TrimSpace(a.Map[subject]); mapped != "" {
			return mapped
		}
	}
	return subject
}

func (t *ToolService) accountFor(req *mcp.CallToolRequest) string {
	subject := ""
	if req != nil && req.Extra != nil && req.Extra.TokenInfo != nil {
		subject = req.Extra.TokenInfo.UserID
	}
	return t.Accounts.ForSubject(subject)
}
