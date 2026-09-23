package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/clientconfig"
	"github.com/edynasty/codebridge/internal/clientcred"
	"github.com/edynasty/codebridge/internal/config"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/gorilla/websocket"
)

var version = "dev"

const maxAgentResponseBytes = 768 * 1024

func main() {
	configPath, configRequired, err := clientconfig.ResolvePath(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	fileConfig, err := clientconfig.Load(configPath, configRequired)
	if err != nil {
		log.Fatal(err)
	}

	managerURL := flag.String("manager", firstNonEmpty(os.Getenv("CODEBRIDGE_MANAGER_URL"), fileConfig.ManagerURL, "ws://127.0.0.1:8080/agent"), "manager websocket URL")
	deviceID := flag.String("device-id", firstNonEmpty(os.Getenv("CODEBRIDGE_DEVICE_ID"), fileConfig.DeviceID, hostnameSlug()), "stable device ID")
	deviceName := flag.String("device-name", firstNonEmpty(os.Getenv("CODEBRIDGE_DEVICE_NAME"), fileConfig.DeviceName, hostname()), "device display name")
	workspacesRaw := flag.String("workspaces", os.Getenv("CODEBRIDGE_WORKSPACES"), "name=/path,name2=/path; overrides config workspaces")
	enrollmentCode := flag.String("enrollment-code", os.Getenv("CODEBRIDGE_ENROLL_CODE"), "one-time manager enrollment code")
	credentialFile := flag.String("credential-file", firstNonEmpty(os.Getenv("CODEBRIDGE_CREDENTIAL_FILE"), fileConfig.CredentialFile, clientcred.DefaultPath()), "device credential file")
	credentialOverride := flag.String("device-credential", os.Getenv("CODEBRIDGE_DEVICE_CREDENTIAL"), "device credential override (normally loaded from credential file)")
	allowSensitiveDefault := fileConfig.AllowSensitiveFiles
	if raw, exists := os.LookupEnv("CODEBRIDGE_ALLOW_SENSITIVE_FILES"); exists {
		value, parseErr := strconv.ParseBool(strings.TrimSpace(raw))
		if parseErr != nil {
			log.Fatal("CODEBRIDGE_ALLOW_SENSITIVE_FILES must be true or false")
		}
		allowSensitiveDefault = value
	}
	allowSensitiveFiles := flag.Bool("allow-sensitive-files", allowSensitiveDefault, "allow MCP tools to read normally blocked sensitive files inside workspaces")
	_ = flag.String("config", configPath, "client JSON config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	var roots map[string]string
	var advertised []protocol.Workspace
	if strings.TrimSpace(*workspacesRaw) != "" {
		roots, advertised, err = config.ParseWorkspaces(*workspacesRaw)
	} else {
		roots, advertised, err = config.ParseWorkspaceMap(fileConfig.Workspaces)
	}
	if err != nil {
		log.Fatal(err)
	}
	service := &agentops.Service{Roots: roots, AllowSensitiveFiles: *allowSensitiveFiles}
	if *allowSensitiveFiles {
		log.Printf("WARNING: sensitive workspace file protection is disabled for this client")
	}
	credKey := clientcred.Key(*managerURL, *deviceID)
	credential := strings.TrimSpace(*credentialOverride)
	if credential == "" {
		credential, err = clientcred.Load(*credentialFile, credKey)
		if err != nil {
			log.Fatalf("load device credential: %v", err)
		}
	} else if err := clientcred.Save(*credentialFile, credKey, credential); err != nil {
		log.Fatalf("save device credential override: %v", err)
	}
	if credential == "" && strings.TrimSpace(*enrollmentCode) == "" {
		log.Fatal("device is not enrolled: set CODEBRIDGE_ENROLL_CODE once, or provide an existing device credential")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backoff := time.Second
	for ctx.Err() == nil {
		reg := protocol.RegisterRequest{
			EnrollmentCode:   strings.TrimSpace(*enrollmentCode),
			DeviceCredential: credential,
			DeviceID:         *deviceID,
			DeviceName:       *deviceName,
			Version:          version,
			Workspaces:       advertised,
		}
		err := runSession(ctx, *managerURL, reg, service, func(issued string) error {
			if err := clientcred.Save(*credentialFile, credKey, issued); err != nil {
				return err
			}
			credential = issued
			*enrollmentCode = ""
			log.Printf("device enrollment completed; credential stored in %s", *credentialFile)
			return nil
		})
		if ctx.Err() != nil {
			break
		}
		log.Printf("connection ended: %v; reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func runSession(ctx context.Context, managerURL string, reg protocol.RegisterRequest, service *agentops.Service, onCredential func(string) error) error {
	u, err := url.Parse(managerURL)
	if err != nil {
		return err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return fmt.Errorf("manager URL must use ws:// or wss://")
	}
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, managerURL, nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	ws.SetReadLimit(1024 * 1024)

	payload, _ := json.Marshal(reg)
	if err := ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegister, DeviceID: reg.DeviceID, Payload: payload}); err != nil {
		return err
	}
	var ack protocol.Envelope
	if err := ws.ReadJSON(&ack); err != nil {
		return err
	}
	if ack.Type != protocol.TypeRegistered {
		return fmt.Errorf("unexpected register response %q", ack.Type)
	}
	var rr protocol.RegisterResponse
	if err := json.Unmarshal(ack.Payload, &rr); err != nil {
		return err
	}
	if !rr.Accepted {
		return fmt.Errorf("registration rejected: %s", rr.Message)
	}
	if rr.DeviceCredential != "" && onCredential != nil {
		if err := onCredential(rr.DeviceCredential); err != nil {
			return fmt.Errorf("persist issued device credential: %w", err)
		}
	}
	log.Printf("registered with manager as %s; workspaces=%d", reg.DeviceID, len(reg.Workspaces))

	writeMu := make(chan struct{}, 1)
	writeMu <- struct{}{}
	write := func(v any) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-writeMu:
		}
		defer func() { writeMu <- struct{}{} }()
		return ws.WriteJSON(v)
	}

	done := make(chan error, 1)
	go func() {
		for {
			var env protocol.Envelope
			if err := ws.ReadJSON(&env); err != nil {
				done <- err
				return
			}
			if env.Type != protocol.TypeRequest {
				continue
			}
			go func(env protocol.Envelope) {
				var req protocol.AgentRequest
				resp := protocol.AgentResponse{OK: false}
				if err := json.Unmarshal(env.Payload, &req); err != nil {
					resp.Error = err.Error()
				} else {
					callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
					result, err := service.Execute(callCtx, req)
					cancel()
					if err != nil {
						resp.Error = err.Error()
					} else {
						data, marshalErr := json.Marshal(result)
						if marshalErr != nil {
							resp.Error = marshalErr.Error()
						} else if len(data) > maxAgentResponseBytes {
							resp.Error = fmt.Sprintf("tool response exceeds %d bytes; narrow the request", maxAgentResponseBytes)
						} else {
							resp.OK = true
							resp.Data = data
						}
					}
				}
				b, _ := json.Marshal(resp)
				_ = write(protocol.Envelope{Type: protocol.TypeResponse, RequestID: env.RequestID, DeviceID: reg.DeviceID, Payload: b})
			}(env)
		}
	}()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			return err
		case <-ticker.C:
			if err := write(protocol.Envelope{Type: protocol.TypeHeartbeat, DeviceID: reg.DeviceID}); err != nil {
				return err
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "codebridge-device"
	}
	return h
}

func hostnameSlug() string {
	h := strings.ToLower(hostname())
	h = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, h)
	return strings.Trim(h, "-")
}
