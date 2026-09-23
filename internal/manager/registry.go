package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/gorilla/websocket"
)

type Device struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Online      bool                 `json:"online"`
	ConnectedAt time.Time            `json:"connected_at"`
	LastSeen    time.Time            `json:"last_seen"`
	Workspaces  []protocol.Workspace `json:"workspaces"`
}

type pendingCall struct {
	ch chan protocol.AgentResponse
}

type AgentConn struct {
	device Device
	ws     *websocket.Conn
	mu     sync.Mutex
	pendMu sync.Mutex
	pend   map[string]pendingCall
}

type Registry struct {
	mu      sync.RWMutex
	devices map[string]*AgentConn
}

func NewRegistry() *Registry {
	return &Registry{devices: make(map[string]*AgentConn)}
}

func (r *Registry) Put(dev Device, ws *websocket.Conn) *AgentConn {
	conn := &AgentConn{device: dev, ws: ws, pend: make(map[string]pendingCall)}
	r.mu.Lock()
	if old := r.devices[dev.ID]; old != nil {
		_ = old.ws.Close()
	}
	r.devices[dev.ID] = conn
	r.mu.Unlock()
	return conn
}

func (r *Registry) Remove(id string, target *AgentConn) {
	r.mu.Lock()
	if r.devices[id] == target {
		delete(r.devices, id)
	}
	r.mu.Unlock()
}

func (r *Registry) Touch(id string) {
	r.mu.Lock()
	if d := r.devices[id]; d != nil {
		d.device.LastSeen = time.Now().UTC()
	}
	r.mu.Unlock()
}

func (r *Registry) List() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Device, 0, len(r.devices))
	for _, c := range r.devices {
		d := c.device
		d.Online = true
		out = append(out, d)
	}
	return out
}

func (r *Registry) Get(id string) (*AgentConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.devices[id]
	return c, ok
}

func (r *Registry) Disconnect(id string) bool {
	r.mu.RLock()
	c := r.devices[id]
	r.mu.RUnlock()
	if c == nil {
		return false
	}
	_ = c.ws.Close()
	return true
}

func (r *Registry) Workspaces(id string) ([]protocol.Workspace, error) {
	c, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("device %q is offline or unknown", id)
	}
	return append([]protocol.Workspace(nil), c.device.Workspaces...), nil
}

func (r *Registry) Call(ctx context.Context, deviceID string, req protocol.AgentRequest) (json.RawMessage, error) {
	conn, ok := r.Get(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %q is offline or unknown", deviceID)
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	wait := make(chan protocol.AgentResponse, 1)
	conn.pendMu.Lock()
	conn.pend[id] = pendingCall{ch: wait}
	conn.pendMu.Unlock()
	defer func() {
		conn.pendMu.Lock()
		delete(conn.pend, id)
		conn.pendMu.Unlock()
	}()

	conn.mu.Lock()
	err = conn.ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRequest, RequestID: id, DeviceID: deviceID, Payload: payload})
	conn.mu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-wait:
		if !resp.OK {
			if resp.Error == "" {
				resp.Error = "agent request failed"
			}
			return nil, errors.New(resp.Error)
		}
		return resp.Data, nil
	}
}

func (c *AgentConn) Deliver(requestID string, resp protocol.AgentResponse) {
	c.pendMu.Lock()
	p, ok := c.pend[requestID]
	c.pendMu.Unlock()
	if ok {
		select {
		case p.ch <- resp:
		default:
		}
	}
}
