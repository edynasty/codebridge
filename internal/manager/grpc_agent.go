package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/pb"
	"github.com/edynasty/codebridge/internal/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// GRPCAgentServer adapts the gRPC bidi stream onto the existing Registry,
// reusing the WebSocket handler's registration, account-binding and
// dispatch semantics. One stream equals one device session.
type GRPCAgentServer struct {
	pb.UnimplementedAgentServiceServer
	Auth     *authstore.Store
	Registry *Registry
	Audit    *auditlog.Logger
}

// ServeGRPC starts the agent gRPC listener with transport-level keepalive
// so half-open connections are detected in seconds, not by the 20s
// application heartbeat.
func ServeGRPC(addr string, srv *GRPCAgentServer) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	ka := grpc.KeepaliveParams(keepalive.ServerParameters{
		Time:    20 * time.Second,
		Timeout: 10 * time.Second,
	})
	ke := grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	})
	s := grpc.NewServer(ka, ke)
	pb.RegisterAgentServiceServer(s, srv)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Printf("grpc agent server: %v", err)
		}
	}()
	return s, lis, nil
}

// grpcConn adapts a gRPC stream to the AgentConn interface the Registry
// expects: buffered sends with flow control via a writer goroutine.
type grpcConn struct {
	send      chan *pb.Envelope
	done      chan struct{}
	closeOnce sync.Once
}

func (c *grpcConn) Send(env *pb.Envelope) {
	select {
	case c.send <- env:
	case <-c.done:
	}
}

func (c *grpcConn) Close() { c.closeOnce.Do(func() { close(c.done) }) }

// Connect implements the device session: register, then relay envelopes.
func (s *GRPCAgentServer) Connect(stream pb.AgentService_ConnectServer) error {
	ctx := stream.Context()

	// Registration must be the first inbound message.
	first, err := stream.Recv()
	if err != nil {
		return status.Error(codes.InvalidArgument, "expected register message")
	}
	if first.Type != "register" {
		return status.Error(codes.InvalidArgument, "first message must be register")
	}
	var reg protocol.RegisterRequest
	if err := json.Unmarshal(first.Payload, &reg); err != nil {
		return status.Error(codes.InvalidArgument, "invalid register payload")
	}
	if err := sanitizeRegistration(&reg); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	requestID := auditlog.NewRequestID()
	issuedCredential := ""
	if s.Auth.VerifyDevice(reg.DeviceID, reg.DeviceCredential) {
		if err := s.Auth.UpdateDeviceName(reg.DeviceID, reg.DeviceName); err != nil {
			log.Printf("update device identity: %v", err)
		}
	} else {
		credential, err := s.Auth.EnrollDevice(reg.EnrollmentCode, reg.DeviceID, reg.DeviceName)
		if err != nil {
			_ = s.Audit.Log(auditlog.Event{Event: "device.enroll", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(false), ErrorKind: "invalid_enrollment"})
			return sendRegisterResult(stream, false, err.Error(), "")
		}
		issuedCredential = credential
		_ = s.Audit.Log(auditlog.Event{Event: "device.enroll", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(true)})
	}
	// The tenant account always comes from persisted device state, never
	// from agent-supplied input.
	account, ok := s.Auth.DeviceAccount(reg.DeviceID)
	if !ok {
		return sendRegisterResult(stream, false, "device account binding not found", "")
	}

	now := time.Now().UTC()
	conn := &grpcConn{send: make(chan *pb.Envelope, 64), done: make(chan struct{})}
	registryConn := s.Registry.Put(Device{
		ID: reg.DeviceID, Name: reg.DeviceName, AccountID: account, Version: reg.Version,
		Online: true, ConnectedAt: now, LastSeen: now, Workspaces: reg.Workspaces, ToolPolicy: reg.ToolPolicy,
	}, grpcAgentAdapter{conn: conn, send: func(env *protocol.Envelope) {
		payload, _ := json.Marshal(env)
		// The adapter re-marshals; send the raw wire envelope directly.
		_ = payload
	}})
	defer s.Registry.Remove(reg.DeviceID, registryConn)
	defer conn.Close()

	// Writer goroutine: serializes sends onto the HTTP/2 stream.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case env := <-conn.send:
				if err := stream.Send(env); err != nil {
					return
				}
			case <-conn.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Registration ack.
	ackPayload, _ := json.Marshal(protocol.RegisterResponse{Accepted: true, DeviceCredential: issuedCredential})
	conn.Send(&pb.Envelope{Type: protocol.TypeRegistered, DeviceId: reg.DeviceID, Payload: ackPayload})
	_ = s.Audit.Log(auditlog.Event{Event: "device.connect", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(true)})
	log.Printf("agent online (grpc): %s (%s), workspaces=%d", reg.DeviceName, reg.DeviceID, len(reg.Workspaces))
	defer func() {
		_ = s.Audit.Log(auditlog.Event{Event: "device.disconnect", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(true)})
		log.Printf("agent offline (grpc): %s (%s)", reg.DeviceName, reg.DeviceID)
	}()

	// Heartbeat backstop: transport keepalive normally detects dead peers;
	// this catches peers that keep HTTP/2 open but stop application
	// heartbeats. The recv goroutine records lastSeen; the main loop polls.
	heartbeatTimeout := 45 * time.Second
	var lastSeen atomic.Int64
	lastSeen.Store(time.Now().Unix())
	recvErr := make(chan error, 1)
	go func() {
		for {
			env, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			lastSeen.Store(time.Now().Unix())
			s.Registry.Touch(reg.DeviceID)
			switch env.Type {
			case protocol.TypeHeartbeat:
				continue
			case protocol.TypeResponse:
				var resp protocol.AgentResponse
				if json.Unmarshal(env.Payload, &resp) == nil {
					if len(resp.Data) > maxAgentPayloadBytes {
						resp = protocol.AgentResponse{OK: false, Error: "agent response exceeded manager payload limit"}
					}
					registryConn.Deliver(env.RequestId, resp)
				}
			case protocol.TypeProgress:
				registryConn.DeliverEvent(env.RequestId, json.RawMessage(env.Payload))
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			conn.Close()
			<-writerDone
			return nil
		case err := <-recvErr:
			conn.Close()
			<-writerDone
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case <-time.After(10 * time.Second):
			if time.Since(time.Unix(lastSeen.Load(), 0)) > heartbeatTimeout {
				conn.Close()
				<-writerDone
				return status.Error(codes.DeadlineExceeded, "agent heartbeat timeout")
			}
		}
	}
}

func sendRegisterResult(stream pb.AgentService_ConnectServer, accepted bool, message, credential string) error {
	payload, _ := json.Marshal(protocol.RegisterResponse{Accepted: accepted, Message: message, DeviceCredential: credential})
	return stream.Send(&pb.Envelope{Type: protocol.TypeRegistered, Payload: payload})
}

// grpcAgentAdapter bridges grpcConn to the AgentConn interface expected by
// the Registry (WriteJSON + Deliver/DeliverEvent/Close).
type grpcAgentAdapter struct {
	conn *grpcConn
	send func(env *protocol.Envelope)
}

func (a grpcAgentAdapter) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var env protocol.Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("grpc adapter: not an envelope")
	}
	payload := env.Payload
	a.conn.Send(&pb.Envelope{Type: env.Type, RequestId: env.RequestID, DeviceId: env.DeviceID, Payload: payload})
	return nil
}

func (a grpcAgentAdapter) Close() error { a.conn.Close(); return nil }

var _ = strings.TrimSpace
