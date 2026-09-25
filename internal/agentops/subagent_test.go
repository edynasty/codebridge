package agentops

import (
	"strings"
	"testing"
)

// The harness default is omp; opencode and codex stay selectable per call.
func TestResolveSubagentCallDefaultsToOMP(t *testing.T) {
	saved := SubagentProfiles
	t.Cleanup(func() { SubagentProfiles = saved })
	SubagentProfiles = nil

	client, agent, model, thinking, timeout, extra := resolveSubagentCall("", "", "", "", 0)
	if client != SubagentClientOMP {
		t.Fatalf("default client = %q, want %q", client, SubagentClientOMP)
	}
	if agent != "" || model != "" || thinking != "" || timeout != 0 || extra != nil {
		t.Fatalf("unexpected defaults: agent=%q model=%q thinking=%q timeout=%d extra=%v", agent, model, thinking, timeout, extra)
	}
}

// A profile carries the harness and its parameters; explicit call fields still
// win, so a caller can narrow model or effort per task.
func TestResolveSubagentCallMergesProfileAndCall(t *testing.T) {
	saved := SubagentProfiles
	t.Cleanup(func() { SubagentProfiles = saved })
	SubagentProfiles = []SubagentProfile{{
		Name: "explore", Client: SubagentClientOpencode, Agent: "librarian",
		Model: "profile/model", Thinking: "low", TimeoutSec: 300, ExtraArgs: []string{"--print-logs=false"},
	}}

	client, agent, model, thinking, timeout, extra := resolveSubagentCall("", "explore", "call/model", "", 0)
	if client != SubagentClientOpencode || agent != "librarian" {
		t.Fatalf("profile not applied: client=%q agent=%q", client, agent)
	}
	if model != "call/model" {
		t.Fatalf("explicit model should win: %q", model)
	}
	if thinking != "low" || timeout != 300 {
		t.Fatalf("profile thinking/timeout not applied: %q %d", thinking, timeout)
	}
	if len(extra) != 1 || extra[0] != "--print-logs=false" {
		t.Fatalf("profile extra args not applied: %v", extra)
	}

	// A profile without a harness falls back to the default one.
	SubagentProfiles = []SubagentProfile{{Name: "quick", Thinking: "low"}}
	if client, _, _, _, _, _ = resolveSubagentCall("", "quick", "", "", 0); client != SubagentClientOMP {
		t.Fatalf("profile without client = %q, want %q", client, SubagentClientOMP)
	}
	catalog := AgentCatalog()
	if len(catalog) != 1 || catalog[0].Client != SubagentClientOMP {
		t.Fatalf("agents_list should report the effective harness: %#v", catalog)
	}
}

func TestSubagentArgsPerHarness(t *testing.T) {
	const root = "/workspace"

	omp := subagentArgs(SubagentClientOMP, root, "explore", "m/1", "high", []string{"--no-skills"}, "do it")
	if omp[0] != "-p" || omp[1] != "--mode" || omp[2] != "json" {
		t.Fatalf("omp must run headless with JSONL events: %v", omp)
	}
	joined := strings.Join(omp, " ")
	for _, want := range []string{"--cwd " + root, "--auto-approve", "--allow-home", "--model m/1", "--thinking high", "--no-skills"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("omp args missing %q: %v", want, omp)
		}
	}
	// omp has no CLI agent selector; the profile name must not leak into argv.
	if strings.Contains(joined, "explore") {
		t.Fatalf("omp args leaked the agent name: %v", omp)
	}
	if omp[len(omp)-2] != "--" || omp[len(omp)-1] != "do it" {
		t.Fatalf("omp must pass the task after --: %v", omp)
	}

	opencode := subagentArgs(SubagentClientOpencode, root, "explore", "m/1", "high", nil, "do it")
	if strings.Join(opencode, " ") != "run --format json --auto --dir "+root+" --agent explore --model m/1 --variant high -- do it" {
		t.Fatalf("opencode args changed: %v", opencode)
	}
	if off := subagentArgs(SubagentClientOpencode, root, "", "", "off", nil, "t"); strings.Contains(strings.Join(off, " "), "--variant") {
		t.Fatalf("opencode must not pass --variant off: %v", off)
	}

	codex := subagentArgs(SubagentClientCodex, root, "work", "m/1", "high", nil, "do it")
	if strings.Join(codex, " ") != "exec --profile work --model m/1 -c model_reasoning_effort=high do it" {
		t.Fatalf("codex args changed: %v", codex)
	}
}

// lastOMPMessage must return the final assistant text out of the JSONL event
// stream, ignoring user turns, deltas and tool-only messages.
func TestLastOMPMessageExtractsFinalAssistantText(t *testing.T) {
	transcript := strings.Join([]string{
		`{"type":"session","version":3,"id":"01a0d895","cwd":"/tmp/probe"}`,
		`{"type":"agent_start"}`,
		`{"type":"message_start","message":{"role":"user","content":[{"type":"text","text":"Reply with exactly: PROBE_OK"}]}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"PRO"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"working"}],"provider":"local"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"tool_use","name":"read"}]}}`,
		`{"type":"agent_end","messages":[{"role":"user","content":[{"type":"text","text":"Reply with exactly: PROBE_OK"}]},{"role":"assistant","content":[{"type":"text","text":"PROBE_OK"}]}],"isTerminal":true}`,
		`not json at all`,
	}, "\n")

	if got := lastOMPMessage(transcript); got != "PROBE_OK" {
		t.Fatalf("lastOMPMessage = %q, want PROBE_OK", got)
	}
	if got := lastOMPMessage(""); got != "" {
		t.Fatalf("empty transcript should yield no message, got %q", got)
	}
	// A run that only emitted tool activity has no final text to lead with.
	toolOnly := `{"type":"agent_end","messages":[{"role":"assistant","content":[{"type":"tool_use","name":"bash"}]}]}`
	if got := lastOMPMessage(toolOnly); got != "" {
		t.Fatalf("tool-only run should yield no message, got %q", got)
	}
}
