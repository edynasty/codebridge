package main

import (
	"encoding/json"
	"fmt"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/pb"
	"github.com/edynasty/codebridge/internal/protocol"
)

const (
	maxRunSyncHeads    = 256
	runReplayBatchSize = 200
)

type envelopeSender func(*pb.Envelope) error

func runtimeEventEnvelope(deviceID string, ev agentops.RunEvent) (*pb.Envelope, error) {
	wire := protocol.RunEvent{
		RunID: ev.RunID, Seq: ev.Seq, At: ev.At,
		Source: ev.Source, Kind: ev.Kind, Payload: ev.Payload,
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if len(payload) > maxAgentResponseBytes {
		wire.Payload = fmt.Sprintf("[runtime event payload omitted: %d bytes]", len(ev.Payload))
		payload, err = json.Marshal(wire)
		if err != nil {
			return nil, err
		}
	}
	return &pb.Envelope{Type: protocol.TypeRunEvent, DeviceId: deviceID, Payload: payload}, nil
}

// sendRunHeads advertises recent local durable journal heads after every
// successful gRPC registration. The Manager replies only for missing suffixes.
func sendRunHeads(runs *agentops.RunManager, deviceID string, send envelopeSender) error {
	if runs == nil {
		return nil
	}
	snaps := runs.List(maxRunSyncHeads)
	heads := protocol.RunHeads{Runs: make([]protocol.RunHead, 0, len(snaps))}
	for _, snap := range snaps {
		if snap == nil || snap.ID == "" || snap.LastSeq <= 0 {
			continue
		}
		heads.Runs = append(heads.Runs, protocol.RunHead{
			RunID: snap.ID, LastSeq: snap.LastSeq, Status: string(snap.Status),
		})
	}
	if len(heads.Runs) == 0 {
		return nil
	}
	payload, err := json.Marshal(heads)
	if err != nil {
		return err
	}
	if len(payload) > maxAgentResponseBytes {
		return fmt.Errorf("runtime run-head payload exceeds %d bytes", maxAgentResponseBytes)
	}
	return send(&pb.Envelope{Type: protocol.TypeRunHeads, DeviceId: deviceID, Payload: payload})
}

// replayRunEvents sends exactly the suffix the Manager requested. Replay is
// bounded by ThroughSeq captured during the head handshake; concurrently
// appended events continue on the live subscription and do not extend this
// loop indefinitely.
func replayRunEvents(runs *agentops.RunManager, request protocol.RunReplayRequest, deviceID string, send envelopeSender) error {
	if runs == nil {
		return nil
	}
	if len(request.Runs) > maxRunSyncHeads {
		return fmt.Errorf("too many runtime replay cursors: %d", len(request.Runs))
	}
	for _, cursor := range request.Runs {
		if cursor.RunID == "" || cursor.AfterSeq < 0 || cursor.ThroughSeq < cursor.AfterSeq {
			return fmt.Errorf("invalid runtime replay cursor for %q", cursor.RunID)
		}
		next := cursor.AfterSeq
		for next < cursor.ThroughSeq {
			events := runs.EventsAfter(cursor.RunID, next, runReplayBatchSize)
			if len(events) == 0 {
				return fmt.Errorf("runtime replay gap for %s after seq %d through %d", cursor.RunID, next, cursor.ThroughSeq)
			}
			progressed := false
			for _, ev := range events {
				if ev.Seq <= next {
					continue
				}
				if ev.Seq > cursor.ThroughSeq {
					break
				}
				if ev.Seq != next+1 {
					return fmt.Errorf("runtime replay gap for %s: got seq %d after %d", cursor.RunID, ev.Seq, next)
				}
				env, err := runtimeEventEnvelope(deviceID, ev)
				if err != nil {
					return err
				}
				if err := send(env); err != nil {
					return err
				}
				next = ev.Seq
				progressed = true
			}
			if !progressed {
				return fmt.Errorf("runtime replay made no progress for %s after seq %d", cursor.RunID, next)
			}
		}
	}
	return nil
}
