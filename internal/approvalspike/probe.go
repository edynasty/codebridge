package approvalspike

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This opt-in feasibility probe never creates Runtime grants or computer sessions.
//
//go:embed widget.html
var widgetHTML string

const resourceURI = "ui://codebridge/phase0-approval.html"

type pending struct {
	id      string
	widget  string
	expires time.Time
}

type Probe struct {
	mu     sync.Mutex
	tokens map[[32]byte]pending
	now    func() time.Time
}

type RequestInput struct{}
type RequestOutput struct {
	ApprovalID string `json:"approval_id"`
	Verb       string `json:"verb"`
	Status     string `json:"status"`
	ExpiresAt  string `json:"expires_at"`
}
type DecisionInput struct {
	ApprovalID string `json:"approval_id"`
	Token      string `json:"token"`
	Decision   string `json:"decision"`
	Scope      string `json:"scope"`
}
type DecisionOutput struct {
	ApprovalID   string `json:"approval_id"`
	Status       string `json:"status"`
	Scope        string `json:"scope"`
	GrantCreated bool   `json:"grant_created"`
}
type SignalInput struct {
	ApprovalID string `json:"approval_id"`
	Token      string `json:"token"`
	SDP        string `json:"sdp"`
}
type SignalOutput struct {
	SDP string `json:"sdp"`
}

func randomID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (p *Probe) request(widget string) (RequestOutput, string, error) {
	token, err := randomID()
	if err != nil {
		return RequestOutput{}, "", err
	}
	id, err := randomID()
	if err != nil {
		return RequestOutput{}, "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	for key, v := range p.tokens {
		if !now.Before(v.expires) {
			delete(p.tokens, key)
		}
	}
	if len(p.tokens) >= 64 {
		return RequestOutput{}, "", errors.New("unavailable: approval probe capacity")
	}
	item := pending{id: "apr_" + id, widget: widget, expires: now.Add(5 * time.Minute)}
	p.tokens[sha256.Sum256([]byte(token))] = item
	return RequestOutput{ApprovalID: item.id, Verb: "computer.observe", Status: "pending", ExpiresAt: item.expires.UTC().Format(time.RFC3339)}, token, nil
}

func (p *Probe) authorize(id, token, widget string) (pending, [32]byte, error) {
	key := sha256.Sum256([]byte(token))
	item, ok := p.tokens[key]
	if !ok || item.id != id || !p.now().Before(item.expires) || (item.widget != "" && item.widget != widget) {
		return pending{}, key, errors.New("permission_denied: invalid or expired approval token")
	}
	return item, key, nil
}

func (p *Probe) decide(in DecisionInput, widget string) (DecisionOutput, error) {
	if in.Scope != "once" && in.Scope != "session" {
		return DecisionOutput{}, errors.New("permission_denied: remote approval scope")
	}
	if in.Decision != "allow" && in.Decision != "deny" {
		return DecisionOutput{}, errors.New("invalid_state: approval decision")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item, key, err := p.authorize(in.ApprovalID, in.Token, widget)
	if err != nil {
		return DecisionOutput{}, err
	}
	delete(p.tokens, key)
	return DecisionOutput{ApprovalID: item.id, Status: in.Decision, Scope: in.Scope, GrantCreated: false}, nil
}

func widgetID(req *mcp.CallToolRequest) string {
	value, _ := req.Params.GetMeta()["openai/widgetSessionId"].(string)
	return value
}

// Register exposes only a bounded approval transport experiment. Enable it
// explicitly; UI visibility is not authentication, possession of _meta token is.
func Register(server *mcp.Server) {
	p := &Probe{tokens: make(map[[32]byte]pending), now: time.Now}
	server.AddResource(&mcp.Resource{URI: resourceURI, Name: "Phase 0 approval probe", MIMEType: "text/html;profile=mcp-app"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: resourceURI, MIMEType: "text/html;profile=mcp-app", Text: widgetHTML, Meta: mcp.Meta{"ui": map[string]any{"csp": map[string]any{"connectDomains": []string{}, "resourceDomains": []string{}}, "prefersBorder": true}}}}}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "phase0_approval", Description: "Open an approval-only feasibility widget. No permission or grant is created.", Meta: mcp.Meta{"ui": map[string]any{"resourceUri": resourceURI}, "openai/outputTemplate": resourceURI}}, func(_ context.Context, req *mcp.CallToolRequest, _ RequestInput) (*mcp.CallToolResult, RequestOutput, error) {
		out, token, err := p.request(widgetID(req))
		if err != nil {
			return nil, RequestOutput{}, err
		}
		return &mcp.CallToolResult{Meta: mcp.Meta{"approval_token": token, "widget_bound": widgetID(req) != ""}}, out, nil
	})
	decisionSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"approval_id": map[string]any{"type": "string"},
			"token":       map[string]any{"type": "string"},
			"decision":    map[string]any{"type": "string", "enum": []string{"allow", "deny"}},
			"scope":       map[string]any{"type": "string", "enum": []string{"once", "session"}},
		},
		"required": []string{"approval_id", "token", "decision", "scope"},
	}
	mcp.AddTool(server, &mcp.Tool{Name: "phase0_approval_decide", Description: "UI-only approval probe; once/session only. Does not create a Grant.", InputSchema: decisionSchema, Meta: mcp.Meta{"ui": map[string]any{"visibility": []string{"app"}}, "openai/widgetAccessible": true}}, func(_ context.Context, req *mcp.CallToolRequest, in DecisionInput) (*mcp.CallToolResult, DecisionOutput, error) {
		out, err := p.decide(in, widgetID(req))
		return nil, out, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "phase0_signal", Description: "UI-only bounded SDP signaling round trip for Phase 0; no computer/media session is created.", Meta: mcp.Meta{"ui": map[string]any{"visibility": []string{"app"}}, "openai/widgetAccessible": true}}, func(_ context.Context, req *mcp.CallToolRequest, in SignalInput) (*mcp.CallToolResult, SignalOutput, error) {
		if len(in.SDP) > 65536 {
			return nil, SignalOutput{}, errors.New("invalid_state: SDP exceeds probe limit")
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if _, _, err := p.authorize(in.ApprovalID, in.Token, widgetID(req)); err != nil {
			return nil, SignalOutput{}, err
		}
		return nil, SignalOutput{SDP: in.SDP}, nil
	})
}
