package runtime

import (
	"errors"
	"fmt"
	"time"
)

// RunTransition is one Run state change together with the journal event that
// records it. The store applies both in one transaction (architecture §8: "a
// state change and its event commit in one transaction").
type RunTransition struct {
	RunID         string
	Status        RunStatus
	At            time.Time
	Source        EventSource
	ErrorCategory string
	OutputRef     string
	CorrelationID string
	Payload       string
}

// runTransitions is the complete legal run state machine. It is the V1 table
// (internal/agentops/run_manager.go) unchanged: V2 keeps the RunStatus machine
// (architecture §7.7). A terminal status has no successors, so a finished run
// never falls back into an earlier state.
var runTransitions = map[RunStatus][]RunStatus{
	RunQueued:  {RunRunning, RunCancelled},
	RunRunning: {RunCompleted, RunFailed, RunCancelled, RunTimeout, RunInterrupted},
}

// canTransition reports whether the Run state machine allows from -> to.
func canTransition(from, to RunStatus) bool {
	for _, next := range runTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// TransitionRun moves a Run to Status, records the terminal timestamps and
// appends the lifecycle event, returning the stored Run and the event. The event
// kind is run.<status>. A change the V1 state machine does not allow is rejected
// with ErrIllegalTransition and writes nothing.
func (s *Store) TransitionRun(tr RunTransition) (Run, Event, error) {
	if err := checkID(tr.RunID, idRun); err != nil {
		return Run{}, Event{}, err
	}
	if !tr.Status.valid() {
		return Run{}, Event{}, fmt.Errorf("runtime: unknown run status %q", tr.Status)
	}
	if tr.At.IsZero() {
		return Run{}, Event{}, errors.New("runtime: run transition requires a timestamp")
	}

	var (
		stored Run
		event  Event
	)
	err := s.Update(func(t *Tx) error {
		run, err := t.GetRun(tr.RunID)
		if err != nil {
			return err
		}
		if !canTransition(run.Status, tr.Status) {
			return fmt.Errorf("%w: run %s %s -> %s", ErrIllegalTransition, run.ID, run.Status, tr.Status)
		}
		run.Status = tr.Status
		if tr.ErrorCategory != "" {
			run.ErrorCategory = tr.ErrorCategory
		}
		if tr.OutputRef != "" {
			run.OutputRef = tr.OutputRef
		}
		switch {
		case tr.Status == RunRunning:
			if run.StartedAt.IsZero() {
				run.StartedAt = tr.At
			}
		case tr.Status.Terminal():
			run.FinishedAt = tr.At
		}
		if err := t.PutRun(*run); err != nil {
			return err
		}
		event, err = t.AppendEvent(Event{
			Stream:        StreamRun(run.ID),
			At:            tr.At,
			SessionID:     run.SessionID,
			ProjectID:     run.ProjectID,
			Source:        tr.Source,
			Kind:          "run." + string(tr.Status),
			CorrelationID: tr.CorrelationID,
			Payload:       tr.Payload,
		})
		if err != nil {
			return err
		}
		run.LastSeq = event.Seq
		stored = *run
		return nil
	})
	if err != nil {
		return Run{}, Event{}, err
	}
	return stored, event, nil
}

// ControllerChange is one ComputerSession controller change. The store
// increments ControllerEpoch and journals computer.controller.changed in the
// same transaction; every controller change increments the epoch
// (architecture §11.4).
type ControllerChange struct {
	ComputerSessionID string
	Controller        Controller
	At                time.Time
	Source            EventSource
	CorrelationID     string
	Payload           string
}

// SetComputerController applies a controller change to a ComputerSession and
// journals it, returning the stored session and the event.
func (s *Store) SetComputerController(cc ControllerChange) (ComputerSession, Event, error) {
	if err := checkID(cc.ComputerSessionID, idComputerSession); err != nil {
		return ComputerSession{}, Event{}, err
	}
	if err := validateController(cc.Controller); err != nil {
		return ComputerSession{}, Event{}, err
	}
	if cc.At.IsZero() {
		return ComputerSession{}, Event{}, errors.New("runtime: controller change requires a timestamp")
	}

	var (
		stored ComputerSession
		event  Event
	)
	err := s.Update(func(t *Tx) error {
		session, err := t.GetComputerSession(cc.ComputerSessionID)
		if err != nil {
			return err
		}
		session.Controller = cc.Controller
		session.ControllerEpoch++
		session.UpdatedAt = cc.At
		if err := t.PutComputerSession(*session); err != nil {
			return err
		}
		event, err = t.AppendEvent(Event{
			Stream:        StreamComputer(session.ID),
			At:            cc.At,
			SessionID:     session.SessionID,
			Source:        cc.Source,
			Kind:          "computer.controller.changed",
			CorrelationID: cc.CorrelationID,
			Payload:       cc.Payload,
		})
		if err != nil {
			return err
		}
		stored = *session
		return nil
	})
	if err != nil {
		return ComputerSession{}, Event{}, err
	}
	return stored, event, nil
}

// validateController enforces the frozen controller shape: human control
// carries a local or remote channel, agent and none carry none.
func validateController(c Controller) error {
	switch c.Kind {
	case ControllerHuman:
		if c.Channel != ChannelLocal && c.Channel != ChannelRemote {
			return fmt.Errorf("runtime: human controller requires a local or remote channel, got %q", c.Channel)
		}
	case ControllerAgent, ControllerNone:
		if c.Channel != ChannelNone {
			return fmt.Errorf("runtime: %s controller cannot carry channel %q", c.Kind, c.Channel)
		}
	default:
		return fmt.Errorf("runtime: unknown controller kind %q", c.Kind)
	}
	return nil
}
