package manager

import (
	"errors"
	"fmt"
	"sync"

	"github.com/edynasty/codebridge/internal/protocol"
)

const (
	maxPendingRunEvents = 512
	maxRunSyncHeads     = 256
)

type runEventKey struct {
	account  string
	deviceID string
	runID    string
}

type runEventState struct {
	head    int64
	pending map[int64]protocol.RunEvent
	last    protocol.RunEvent
}

// RunEventBroker tracks the highest contiguous Runtime-event sequence received
// for each account/device/run. It is intentionally metadata-only: the Client
// remains the durable event store and RT-004 replays any gap after reconnect.
type RunEventBroker struct {
	mu   sync.RWMutex
	runs map[runEventKey]*runEventState
}

func NewRunEventBroker() *RunEventBroker {
	return &RunEventBroker{runs: map[runEventKey]*runEventState{}}
}

// Accept ingests one event idempotently. Out-of-order events are held in a
// bounded pending set until the missing sequence arrives; duplicates are
// ignored. The returned head is the highest contiguous sequence now known.
func (b *RunEventBroker) Accept(account, deviceID string, ev protocol.RunEvent) (head int64, accepted bool, err error) {
	if b == nil {
		return 0, false, errors.New("run event broker is nil")
	}
	if account == "" || deviceID == "" {
		return 0, false, errors.New("run event account and device are required")
	}
	if ev.RunID == "" {
		return 0, false, errors.New("run event run_id is required")
	}
	if ev.Seq <= 0 {
		return 0, false, errors.New("run event seq must be positive")
	}
	if ev.Kind == "" {
		return 0, false, errors.New("run event kind is required")
	}

	key := runEventKey{account: account, deviceID: deviceID, runID: ev.RunID}
	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.runs[key]
	if st == nil {
		st = &runEventState{pending: map[int64]protocol.RunEvent{}}
		b.runs[key] = st
	}
	if ev.Seq <= st.head {
		return st.head, false, nil
	}
	if _, exists := st.pending[ev.Seq]; exists {
		return st.head, false, nil
	}

	if ev.Seq == st.head+1 {
		st.head = ev.Seq
		st.last = ev
		for {
			next, ok := st.pending[st.head+1]
			if !ok {
				break
			}
			delete(st.pending, st.head+1)
			st.head = next.Seq
			st.last = next
		}
		return st.head, true, nil
	}

	if len(st.pending) >= maxPendingRunEvents {
		return st.head, false, fmt.Errorf("run event gap too large for %s: head=%d seq=%d", ev.RunID, st.head, ev.Seq)
	}
	st.pending[ev.Seq] = ev
	return st.head, true, nil
}

// Missing returns the exact journal suffixes the Manager needs from one
// reconnecting Client. Entries already fully synchronized are omitted.
func (b *RunEventBroker) Missing(account, deviceID string, heads []protocol.RunHead) (protocol.RunReplayRequest, error) {
	if b == nil {
		return protocol.RunReplayRequest{}, errors.New("run event broker is nil")
	}
	if account == "" || deviceID == "" {
		return protocol.RunReplayRequest{}, errors.New("run event account and device are required")
	}
	if len(heads) > maxRunSyncHeads {
		return protocol.RunReplayRequest{}, fmt.Errorf("too many run heads: %d > %d", len(heads), maxRunSyncHeads)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	out := protocol.RunReplayRequest{Runs: make([]protocol.RunReplayCursor, 0)}
	seen := make(map[string]struct{}, len(heads))
	for _, advertised := range heads {
		if advertised.RunID == "" || advertised.LastSeq < 0 {
			return protocol.RunReplayRequest{}, errors.New("invalid run head")
		}
		if _, duplicate := seen[advertised.RunID]; duplicate {
			return protocol.RunReplayRequest{}, fmt.Errorf("duplicate run head %q", advertised.RunID)
		}
		seen[advertised.RunID] = struct{}{}

		var managerHead int64
		if st := b.runs[runEventKey{account: account, deviceID: deviceID, runID: advertised.RunID}]; st != nil {
			managerHead = st.head
		}
		if advertised.LastSeq > managerHead {
			out.Runs = append(out.Runs, protocol.RunReplayCursor{
				RunID: advertised.RunID, AfterSeq: managerHead, ThroughSeq: advertised.LastSeq,
			})
		}
	}
	return out, nil
}

// Head returns the highest contiguous sequence the Manager has accepted for
// one account/device/run. RT-004 uses this as the replay cursor on reconnect.
func (b *RunEventBroker) Head(account, deviceID, runID string) int64 {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if st := b.runs[runEventKey{account: account, deviceID: deviceID, runID: runID}]; st != nil {
		return st.head
	}
	return 0
}

// Last returns the last event that advanced the contiguous head.
func (b *RunEventBroker) Last(account, deviceID, runID string) (protocol.RunEvent, bool) {
	if b == nil {
		return protocol.RunEvent{}, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	st := b.runs[runEventKey{account: account, deviceID: deviceID, runID: runID}]
	if st == nil || st.head == 0 {
		return protocol.RunEvent{}, false
	}
	return st.last, true
}
