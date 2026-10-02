package approvalspike

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func testProbe() *Probe { return &Probe{tokens: make(map[[32]byte]pending), now: time.Now} }
func TestApprovalTokenBoundary(t *testing.T) {
	p := testProbe()
	out, token, err := p.request("widget-a")
	if err != nil {
		t.Fatal(err)
	}
	model, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(model), token) {
		t.Fatal("approval credential leaked to model output")
	}
	in := DecisionInput{ApprovalID: out.ApprovalID, Decision: "allow", Scope: "once"}
	if _, err = p.decide(in, "widget-a"); err == nil {
		t.Fatal("model approved without token")
	}
	in.Token = token
	if _, err = p.decide(in, "widget-b"); err == nil {
		t.Fatal("another widget approved")
	}
	in.Scope = "always"
	if _, err = p.decide(in, "widget-a"); err == nil {
		t.Fatal("remote approval widened scope")
	}
	in.Scope = "once"
	got, err := p.decide(in, "widget-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "allow" || got.Scope != "once" || got.GrantCreated {
		t.Fatalf("decision %+v", got)
	}
	if _, err = p.decide(in, "widget-a"); err == nil {
		t.Fatal("approval token replay succeeded")
	}
}
func TestApprovalWithoutWidgetBindingAndExpiry(t *testing.T) {
	p := testProbe()
	now := time.Now()
	p.now = func() time.Time { return now }
	out, token, err := p.request("")
	if err != nil {
		t.Fatal(err)
	}
	in := DecisionInput{ApprovalID: out.ApprovalID, Token: token, Decision: "deny", Scope: "session"}
	got, err := p.decide(in, "")
	if err != nil || got.Status != "deny" {
		t.Fatalf("unbound denial %+v %v", got, err)
	}
	out, token, err = p.request("")
	if err != nil {
		t.Fatal(err)
	}
	in.ApprovalID = out.ApprovalID
	in.Token = token
	now = now.Add(5 * time.Minute)
	if _, err = p.decide(in, ""); err == nil {
		t.Fatal("expired token approved")
	}
}
func TestApprovalConcurrentConsumption(t *testing.T) {
	p := testProbe()
	out, token, err := p.request("")
	if err != nil {
		t.Fatal(err)
	}
	in := DecisionInput{ApprovalID: out.ApprovalID, Token: token, Decision: "allow", Scope: "session"}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := p.decide(in, ""); results <- err })
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("token consumed %d times", succeeded)
	}
}
