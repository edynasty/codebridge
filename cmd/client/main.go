package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
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
	writableRaw := flag.String("writable-workspaces", firstNonEmpty(os.Getenv("CODEBRIDGE_WRITABLE_WORKSPACES"), strings.Join(fileConfig.WritableWorkspaces, ",")), "comma-separated workspaces with explicit local write opt-in (empty disables write mode)")
	enableLSPDefault := fileConfig.EnableLSP
	if raw, exists := os.LookupEnv("CODEBRIDGE_ENABLE_LSP"); exists {
		value, parseErr := strconv.ParseBool(strings.TrimSpace(raw))
		if parseErr != nil {
			log.Fatal("CODEBRIDGE_ENABLE_LSP must be true or false")
		}
		enableLSPDefault = value
	}
	enableLSP := flag.Bool("enable-lsp", enableLSPDefault, "use locally installed language servers (gopls, typescript-language-server, jdtls) for symbol tools")
	stateDir := firstNonEmpty(os.Getenv("CODEBRIDGE_STATE_DIR"), fileConfig.CheckpointDir, agentops.DefaultClientStateDir())
	indexDir := firstNonEmpty(os.Getenv("CODEBRIDGE_INDEX_DIR"), filepath.Join(stateDir, "index"))
	checkpointDir := filepath.Join(stateDir, "checkpoints")
	_ = flag.String("config", configPath, "client JSON config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	writable := map[string]bool{}
	for _, name := range strings.Split(*writableRaw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			writable[name] = true
		}
	}

	var roots map[string]string
	var advertised []protocol.Workspace
	if strings.TrimSpace(*workspacesRaw) != "" {
		roots, advertised, err = config.ParseWorkspaces(*workspacesRaw, writable)
	} else {
		roots, advertised, err = config.ParseWorkspaceMap(fileConfig.Workspaces, writable)
	}
	if err != nil {
		log.Fatal(err)
	}
	service := &agentops.Service{
		Roots:               roots,
		AllowSensitiveFiles: *allowSensitiveFiles,
		Writable:            writable,
		IndexDir:            indexDir,
		CheckpointDir:       checkpointDir,
		EnableLSP:           *enableLSP,
	}
	if *allowSensitiveFiles {
		log.Printf("WARNING: sensitive workspace file protection is disabled for this client")
	}
	if len(writable) > 0 {
		names := make([]string, 0, len(writable))
		for name := range writable {
			names = append(names, name)
		}
		sort.Strings(names)
		log.Printf("write mode enabled for workspace(s): %s (git checkpoints active)", strings.Join(names, ", "))
	}
	if *enableLSP {
		log.Printf("LSP symbol tools enabled: language servers run locally and fall back to the portable parser when unavailable")
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
	if _, err := validateManagerURL(managerURL); err != nil {
		return err
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

func validateManagerURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid manager URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("manager URL must not contain user info, query, or fragment")
	}
	if u.Scheme == "wss" {
		return u, nil
	}
	if u.Scheme == "ws" {
		host := u.Hostname()
		if host == "localhost" {
			return u, nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return u, nil
		}
		return nil, fmt.Errorf("remote manager URL must use wss://; ws:// is allowed only for loopback development")
	}
	return nil, fmt.Errorf("manager URL must use wss:// (or ws:// for loopback development)")
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
