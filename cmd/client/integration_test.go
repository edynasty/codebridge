package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/manager"
	"github.com/edynasty/codebridge/internal/protocol"
)

func TestManagerClientEnrollmentToolCallAndCredentialReconnect(t *testing.T) {
	workspace := t.TempDir()
	const wantContent = "hello through codebridge\n"
	if err := os.WriteFile(workspace+"/hello.txt", []byte(wantContent), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := authstore.Open(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}
	enrollmentCode, _, err := store.CreateEnrollment(5*time.Minute, authstore.DefaultAccount)
	if err != nil {
		t.Fatal(err)
	}

	registry := manager.NewRegistry(4)
	grpcSrv, grpcLis, err := manager.ServeGRPC("127.0.0.1:0", &manager.GRPCAgentServer{
		Registry: registry,
		Auth:     store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer grpcSrv.Stop()
	grpcTarget := grpcLis.Addr().String()

	service := &agentops.Service{Roots: map[string]string{"demo": workspace}}
	firstReg := protocol.RegisterRequest{
		EnrollmentCode: enrollmentCode,
		DeviceID:       "test-device",
		DeviceName:     "Integration Test Device",
		Version:        "test",
		Workspaces:     []protocol.Workspace{{Name: "demo"}},
	}

	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	issued := make(chan string, 1)
	go func() {
		firstDone <- runGRPCSession(firstCtx, &runtime{grpcTarget: grpcTarget, reg: firstReg, service: service}, &clientState{}, func(credential string) error {
			issued <- credential
			return nil
		})
	}()

	waitForDeviceState(t, registry, "test-device", true)

	var credential string
	select {
	case credential = <-issued:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not issue device credential")
	}
	if credential == "" || !store.VerifyDevice("test-device", credential) {
		t.Fatal("issued credential is empty or cannot be verified")
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 3*time.Second)
	raw, err := registry.Call(callCtx, authstore.DefaultAccount, "test-device", protocol.AgentRequest{
		Tool:      "read",
		Workspace: "demo",
		Args:      map[string]any{"path": "hello.txt"},
	})
	callCancel()
	if err != nil {
		t.Fatalf("read_file over real websocket: %v", err)
	}
	var read agentops.ReadFileResult
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatal(err)
	}
	if read.Content != wantContent || read.Path != "hello.txt" || read.Truncated {
		t.Fatalf("unexpected read_file result: %#v", read)
	}

	if _, err := store.EnrollDevice(enrollmentCode, "second-device", "Second"); err == nil {
		t.Fatal("one-time enrollment code was accepted twice")
	}

	firstCancel()
	if err := waitSession(firstDone); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("first session ended unexpectedly: %v", err)
	}
	waitForDeviceState(t, registry, "test-device", false)

	secondCtx, secondCancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	reissued := make(chan string, 1)
	secondReg := protocol.RegisterRequest{
		DeviceCredential: credential,
		DeviceID:         "test-device",
		DeviceName:       "Integration Test Device Renamed",
		Version:          "test-reconnect",
		Workspaces:       []protocol.Workspace{{Name: "demo"}},
	}
	go func() {
		secondDone <- runGRPCSession(secondCtx, &runtime{grpcTarget: grpcTarget, reg: secondReg, service: service}, &clientState{}, func(newCredential string) error {
			reissued <- newCredential
			return nil
		})
	}()

	waitForDeviceState(t, registry, "test-device", true)
	select {
	case got := <-reissued:
		t.Fatalf("credential reconnect unexpectedly issued a new credential: %q", got)
	case <-time.After(150 * time.Millisecond):
	}

	callCtx, callCancel = context.WithTimeout(context.Background(), 3*time.Second)
	raw, err = registry.Call(callCtx, authstore.DefaultAccount, "test-device", protocol.AgentRequest{
		Tool:      "project_info",
		Workspace: "demo",
	})
	callCancel()
	if err != nil {
		t.Fatalf("project_info after credential reconnect: %v", err)
	}
	var info map[string]any
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if _, ok := info["markers"]; !ok {
		t.Fatalf("unexpected project_info result: %#v", info)
	}

	secondCancel()
	if err := waitSession(secondDone); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("second session ended unexpectedly: %v", err)
	}
	waitForDeviceState(t, registry, "test-device", false)
}

func waitForDeviceState(t *testing.T, registry *manager.Registry, deviceID string, online bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, exists := registry.Get(deviceID)
		if exists == online {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, exists := registry.Get(deviceID)
	t.Fatalf("device %q online=%v, want %v", deviceID, exists, online)
}

func waitSession(done <-chan error) error {
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("session did not stop")
	}
}
