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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/clientconfig"
	"github.com/edynasty/codebridge/internal/clientcred"
	"github.com/edynasty/codebridge/internal/config"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/edynasty/codebridge/internal/ui"
	"github.com/gorilla/websocket"
)

var version = "dev"

const maxAgentResponseBytes = 768 * 1024

// runtime is one fully-resolved connection attempt: everything runSession
// needs, rebuilt from scratch whenever the UI saves a new configuration.
type runtime struct {
	managerURL      string
	allowInsecureWS bool
	reg             protocol.RegisterRequest
	service         *agentops.Service
	credKey         string
	credentialFile  string
}

type clientState struct {
	credential    atomic.Value // string
	enrollCode    atomic.Value // string
	connected     atomic.Bool
	reconnects    atomic.Int64
	configFile    string
	credentialFil string
}

func (c *clientState) Credential() string {
	if v, ok := c.credential.Load().(string); ok {
		return v
	}
	return ""
}

func (c *clientState) EnrollmentCode() string {
	if v, ok := c.enrollCode.Load().(string); ok {
		return v
	}
	return ""
}

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
	allowInsecureWSDefault, allowInsecurePinned := envBoolPinned("CODEBRIDGE_ALLOW_INSECURE_WS", false)
	allowSensitiveDefault, allowSensitivePinned := envBoolPinned("CODEBRIDGE_ALLOW_SENSITIVE_FILES", fileConfig.AllowSensitiveFiles)
	enableLSPDefault, enableLSPPinned := envBoolPinned("CODEBRIDGE_ENABLE_LSP", fileConfig.EnableLSP)
	uiAddr := flag.String("ui-addr", firstNonEmpty(os.Getenv("CODEBRIDGE_UI_ADDR"), "127.0.0.1:8190"), "local configuration UI listen address; empty disables the UI")
	_ = flag.Bool("allow-insecure-ws", allowInsecureWSDefault, "allow plaintext ws:// to a non-loopback manager (local container networking only)")
	_ = flag.Bool("allow-sensitive-files", allowSensitiveDefault, "allow MCP tools to read normally blocked sensitive files inside workspaces (equivalent to CODEBRIDGE_ALLOW_SENSITIVE_FILES)")
	writableRaw := flag.String("writable-workspaces", firstNonEmpty(os.Getenv("CODEBRIDGE_WRITABLE_WORKSPACES"), strings.Join(fileConfig.WritableWorkspaces, ",")), "comma-separated workspaces with explicit local write opt-in (empty disables write mode)")
	_ = flag.Bool("enable-lsp", enableLSPDefault, "use locally installed language servers (gopls, typescript-language-server, jdtls) for symbol tools (equivalent to CODEBRIDGE_ENABLE_LSP)")
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

	logRing := ui.NewLogRing(500, os.Stderr)
	log.SetOutput(logRing)

	state := &clientState{configFile: configPath, credentialFil: *credentialFile}
	state.credential.Store(strings.TrimSpace(*credentialOverride))
	state.enrollCode.Store(strings.TrimSpace(*enrollmentCode))

	// boot is the configuration that produced this process start: env and
	// flags override the JSON file, and those origins cannot be changed by
	// the UI (only the JSON file is editable at runtime).
	boot := bootOptions{
		managerURLFromEnvOrFlag: *managerURL,
		workspacesFromEnv:       strings.TrimSpace(*workspacesRaw),
		deviceID:                *deviceID,
		deviceName:              *deviceName,
		allowInsecureWS:         allowInsecureWSDefault,
		allowInsecurePinned:     allowInsecurePinned,
		allowSensitive:          allowSensitiveDefault,
		allowSensitivePinned:    allowSensitivePinned,
		writableRaw:             *writableRaw,
		writablePinned:          strings.TrimSpace(os.Getenv("CODEBRIDGE_WRITABLE_WORKSPACES")) != "",
		enableLSP:               enableLSPDefault,
		enableLSPPinned:         enableLSPPinned,
		indexDir:                indexDir,
		checkpointDir:           checkpointDir,
		credentialFile:          *credentialFile,
	}
	if state.Credential() == "" {
		cred, err := clientcred.Load(boot.credentialFile, clientcred.Key(boot.managerURLFromEnvOrFlag, boot.deviceID))
		if err != nil {
			log.Fatalf("load device credential: %v", err)
		}
		state.credential.Store(cred)
	} else if err := clientcred.Save(boot.credentialFile, clientcred.Key(boot.managerURLFromEnvOrFlag, boot.deviceID), state.Credential()); err != nil {
		log.Fatalf("save device credential override: %v", err)
	}

	if state.Credential() == "" && state.EnrollmentCode() == "" {
		log.Printf("device is not enrolled: set an enrollment code in the local UI or CODEBRIDGE_ENROLL_CODE")
	}

	reload := make(chan struct{}, 1)
	disconnect := make(chan struct{}, 1)
	started := time.Now()
	rt, err := buildRuntime(boot, fileConfig, state)
	if err != nil {
		log.Printf("initial configuration problem: %v", err)
	}

	uiServer := startUI(*uiAddr, state, boot, rt, reload, disconnect, logRing, started)
	if uiServer != "" {
		log.Printf("configuration UI listening on http://%s", uiServer)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backoff := time.Second
	for ctx.Err() == nil {
		if rt == nil {
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				// keep retrying with the last good or edited config
				rt, err = buildRuntime(boot, loadConfigFile(configPath), state)
				if err != nil {
					log.Printf("configuration still invalid: %v", err)
					rt = nil
				}
			}
			continue
		}
		sessionCtx, cancelSession := context.WithCancel(ctx)
		sessionDone := make(chan error, 1)
		go func() {
			sessionDone <- runSession(sessionCtx, rt, state, func(issued string) error {
				return persistCredential(rt, issued, state)
			})
		}()
		select {
		case <-ctx.Done():
			cancelSession()
			return
		case <-reload:
			cancelSession()
			<-sessionDone
			next, err := buildRuntime(boot, loadConfigFile(configPath), state)
			if err != nil {
				log.Printf("new configuration rejected, keeping the previous one: %v", err)
			} else {
				rt = next
			}
			backoff = time.Second
			log.Printf("configuration applied; reconnecting as %s", rt.reg.DeviceID)
			continue
		case <-disconnect:
			cancelSession()
			<-sessionDone
			log.Printf("manual disconnect from UI; reconnecting")
			continue
		case err := <-sessionDone:
			cancelSession()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				state.connected.Store(false)
				state.reconnects.Add(1)
				log.Printf("connection ended: %v; reconnecting in %s", err, backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < 15*time.Second {
					backoff *= 2
				}
			} else {
				time.Sleep(time.Second)
			}
		}
	}
}

func persistCredential(rt *runtime, issued string, state *clientState) error {
	if err := clientcred.Save(rt.credentialFile, rt.credKey, issued); err != nil {
		return err
	}
	state.credential.Store(issued)
	state.enrollCode.Store("")
	log.Printf("device enrollment completed; credential stored in %s", rt.credentialFile)
	return nil
}

// bootOptions captures the flag/env origin of each setting so reloads can
// tell "user edited the JSON file" apart from "operator pinned this via env".
type bootOptions struct {
	managerURLFromEnvOrFlag string
	workspacesFromEnv       string
	deviceID                string
	deviceName              string
	allowInsecureWS         bool
	allowInsecurePinned     bool
	allowSensitive          bool
	allowSensitivePinned    bool
	writableRaw             string
	writablePinned          bool
	enableLSP               bool
	enableLSPPinned         bool
	indexDir                string
	checkpointDir           string
	credentialFile          string
}

func loadConfigFile(path string) clientconfig.Config {
	cfg, err := clientconfig.Load(path, false)
	if err != nil {
		return clientconfig.Config{}
	}
	return cfg
}

// buildRuntime merges boot precedence with the current JSON file and produces
// the next connection attempt. It never falls back silently: invalid input
// returns an error and the caller keeps the previous runtime.
func buildRuntime(boot bootOptions, file clientconfig.Config, state *clientState) (*runtime, error) {
	cfg := file

	managerURL := firstNonEmpty(boot.managerURLFromEnvOrFlag, cfg.ManagerURL, "ws://127.0.0.1:8080/agent")
	deviceID := firstNonEmpty(boot.deviceID, cfg.DeviceID, hostnameSlug())
	deviceName := firstNonEmpty(boot.deviceName, cfg.DeviceName, hostname())
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("device id is required")
	}
	allowInsecureWS := boot.allowInsecureWS
	if !boot.allowInsecurePinned {
		allowInsecureWS = cfg.AllowInsecureWS
	}
	allowSensitive := boot.allowSensitive
	if !boot.allowSensitivePinned {
		allowSensitive = cfg.AllowSensitiveFiles
	}
	enableLSP := boot.enableLSP
	if !boot.enableLSPPinned {
		enableLSP = cfg.EnableLSP
	}

	writable := map[string]bool{}
	writableSrc := boot.writableRaw
	if !boot.writablePinned && strings.TrimSpace(writableSrc) == "" {
		writableSrc = strings.Join(cfg.WritableWorkspaces, ",")
	}
	for _, name := range strings.Split(writableSrc, ",") {
		if name = strings.TrimSpace(name); name != "" {
			writable[name] = true
		}
	}

	var roots map[string]string
	var advertised []protocol.Workspace
	if boot.workspacesFromEnv != "" {
		var err error
		roots, advertised, err = config.ParseWorkspaces(boot.workspacesFromEnv, writable)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		roots, advertised, err = config.ParseWorkspaceMap(cfg.Workspaces, writable)
		if err != nil {
			return nil, err
		}
	}
	return assemble(boot, managerURL, deviceID, deviceName, roots, advertised, writable, allowSensitive, enableLSP, allowInsecureWS, state, cfg)
}

func assemble(boot bootOptions, managerURL, deviceID, deviceName string, roots map[string]string, advertised []protocol.Workspace, writable map[string]bool, allowSensitive, enableLSP, allowInsecureWS bool, state *clientState, cfg clientconfig.Config) (*runtime, error) {
	if _, err := validateManagerURL(managerURL, allowInsecureWS); err != nil {
		return nil, err
	}
	rules := cfg.Permissions
	switch strings.TrimSpace(os.Getenv("CODEBRIDGE_BASH_PERMISSIONS")) {
	case "full":
		rules = append(rules, agentops.PermissionRule{Effect: "allow", Tool: "bash", Pattern: "*", Reason: "CODEBRIDGE_BASH_PERMISSIONS=full"})
	case "deny":
		rules = append(rules, agentops.PermissionRule{Effect: "deny", Tool: "bash", Pattern: "*", Reason: "CODEBRIDGE_BASH_PERMISSIONS=deny"})
	}
	agentops.SetPermissions(rules)
	if len(rules) > 0 {
		log.Printf("bash permission rules active: %d", len(rules))
	}
	if profiles := subagentProfilesFrom(cfg); len(profiles) > 0 {
		agentops.SubagentProfiles = profiles
		log.Printf("subagent profiles active: %d", len(profiles))
	}
	service := &agentops.Service{
		Roots:               roots,
		AllowSensitiveFiles: allowSensitive,
		Writable:            writable,
		IndexDir:            boot.indexDir,
		CheckpointDir:       boot.checkpointDir,
		EnableLSP:           enableLSP,
		BashAllowlist:       cfg.BashAllowlist,
	}
	service.SetRulePersister(func(rules []agentops.PermissionRule) {
		next := loadConfigFile(state.configFile)
		next.Permissions = rules
		if err := clientconfig.Save(state.configFile, next); err != nil {
			log.Printf("persist permission rules: %v", err)
		}
	})
	if allowSensitive {
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
	policy := &protocol.ToolPolicy{
		EnabledTools:  cfg.EnabledTools,
		DisabledTools: cfg.DisabledTools,
		CustomTools:   sanitizeCustomTools(cfg.CustomTools),
	}
	if len(policy.EnabledTools) == 0 && len(policy.DisabledTools) == 0 && len(policy.CustomTools) == 0 {
		policy = nil
	} else {
		log.Printf("tool policy: %d enabled, %d disabled, %d custom", len(policy.EnabledTools), len(policy.DisabledTools), len(policy.CustomTools))
	}
	return &runtime{
		credentialFile:  boot.credentialFile,
		managerURL:      managerURL,
		allowInsecureWS: allowInsecureWS,
		reg: protocol.RegisterRequest{
			EnrollmentCode:   state.EnrollmentCode(),
			DeviceCredential: state.Credential(),
			DeviceID:         deviceID,
			DeviceName:       deviceName,
			Version:          version,
			Workspaces:       advertised,
			ToolPolicy:       policy,
		},
		service: service,
		credKey: clientcred.Key(managerURL, deviceID),
	}, nil
}

func runSession(ctx context.Context, rt *runtime, state *clientState, onCredential func(string) error) error {
	if _, err := validateManagerURL(rt.managerURL, rt.allowInsecureWS); err != nil {
		return err
	}
	reg := rt.reg
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, rt.managerURL, nil)
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
	state.connected.Store(true)
	log.Printf("registered with manager as %s; workspaces=%d", reg.DeviceID, len(reg.Workspaces))
	defer state.connected.Store(false)

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
					// Subagent runs are long; every other tool stays bounded.
					budget := 30 * time.Second
					if req.Tool == "agent" {
						if secs := intArgAny(req.Args, "timeout_seconds", 0); secs > 0 {
							budget = time.Duration(secs) * time.Second
						} else {
							budget = 10 * time.Minute
						}
					}
					callCtx, cancel := context.WithTimeout(ctx, budget)
					result, err := rt.service.Execute(callCtx, req)
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

func intArgAny(m map[string]any, key string, def int) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return def
}

func envBool(name string, def bool) bool {
	v, _ := envBoolPinned(name, def)
	return v
}

// envBoolPinned reports whether the variable was explicitly set, so unset
// values do not shadow settings coming from the JSON config file.
func envBoolPinned(name string, def bool) (value, pinned bool) {
	if raw, exists := os.LookupEnv(name); exists {
		value, parseErr := strconv.ParseBool(strings.TrimSpace(raw))
		if parseErr != nil {
			log.Fatalf("%s must be true or false", name)
		}
		return value, true
	}
	return def, false
}

func startUI(addr string, state *clientState, boot bootOptions, rt *runtime, reload, disconnect chan struct{}, logRing *ui.AttachLogRing, started time.Time) string {
	server := ui.NewServer()
	server.LoadConfig = func() ui.Config {
		cfg := loadConfigFile(state.configFile)
		return ui.Config{
			ManagerURL:          effectiveManagerURL(boot, cfg),
			AllowInsecureWS:     cfg.AllowInsecureWS || boot.allowInsecurePinned,
			DeviceID:            effectiveDeviceID(boot, cfg),
			DeviceName:          effectiveDeviceName(boot, cfg),
			AccessMode:          effectiveAccessMode(cfg),
			Workspaces:          cfg.Workspaces,
			WritableWorkspaces:  cfg.WritableWorkspaces,
			EnabledTools:        cfg.EnabledTools,
			DisabledTools:       cfg.DisabledTools,
			BashAllowlist:       cfg.BashAllowlist,
			Permissions:         cfg.Permissions,
			CustomTools:         cfg.CustomTools,
			SubagentProfiles:    subagentProfilesToDTO(cfg.SubagentProfiles),
			AllowSensitiveFiles: cfg.AllowSensitiveFiles || boot.allowSensitivePinned,
			EnableLSP:           cfg.EnableLSP || boot.enableLSPPinned,
			EnrollmentCode:      "",
		}
	}
	server.SaveConfig = func(in ui.Config) (bool, error) {
		if in.ManagerURL == "\x00enroll-only" {
			state.enrollCode.Store(in.EnrollmentCode)
			state.connected.Store(false)
			select {
			case reload <- struct{}{}:
			default:
			}
			return true, nil
		}
		if _, err := validateManagerURL(in.ManagerURL, in.AllowInsecureWS || boot.allowInsecurePinned); err != nil {
			return false, err
		}
		cfg := loadConfigFile(state.configFile)
		cfg.ManagerURL = in.ManagerURL
		cfg.DeviceID = in.DeviceID
		cfg.DeviceName = in.DeviceName
		cfg.Workspaces = in.Workspaces
		cfg.WritableWorkspaces = in.WritableWorkspaces
		cfg.AllowSensitiveFiles = in.AllowSensitiveFiles
		cfg.AllowInsecureWS = in.AllowInsecureWS
		cfg.EnableLSP = in.EnableLSP
		cfg.AccessMode = "workspaces"
		if in.AccessMode == "full" {
			cfg.AccessMode = "full"
		}
		cfg.EnabledTools = in.EnabledTools
		cfg.DisabledTools = in.DisabledTools
		cfg.BashAllowlist = in.BashAllowlist
		cfg.Permissions = in.Permissions
		cfg.CustomTools = sanitizeCustomTools(in.CustomTools)
		cfg.SubagentProfiles = subagentProfilesFromDTO(in.SubagentProfiles)
		if strings.TrimSpace(in.EnrollmentCode) != "" {
			state.enrollCode.Store(strings.TrimSpace(in.EnrollmentCode))
		}
		if _, _, err := config.ParseWorkspaceMap(cfg.Workspaces, writableSet(cfg.WritableWorkspaces)); err != nil {
			return false, err
		}
		if err := clientconfig.Save(state.configFile, cfg); err != nil {
			return false, err
		}
		select {
		case reload <- struct{}{}:
		default:
		}
		return true, nil
	}
	server.StateSnapshot = func() ui.State {
		cfg := loadConfigFile(state.configFile)
		workspaces := make([]ui.Workspace, 0, len(cfg.Workspaces))
		names := make([]string, 0, len(cfg.Workspaces))
		for name := range cfg.Workspaces {
			names = append(names, name)
		}
		sort.Strings(names)
		writable := writableSet(cfg.WritableWorkspaces)
		for _, name := range names {
			workspaces = append(workspaces, ui.Workspace{
				Name:      name,
				Path:      cfg.Workspaces[name],
				Writable:  writable[name],
				Sensitive: cfg.AllowSensitiveFiles,
			})
		}
		var currentURL, deviceID, deviceName string
		if rt != nil {
			currentURL = rt.managerURL
			deviceID = rt.reg.DeviceID
			deviceName = rt.reg.DeviceName
		}
		return ui.State{
			Connected:      state.connected.Load(),
			ManagerURL:     currentURL,
			DeviceID:       deviceID,
			DeviceName:     deviceName,
			Enrolled:       state.Credential() != "",
			CredentialFile: state.credentialFil,
			ConfigFile:     state.configFile,
			Workspaces:     workspaces,
			StartedAt:      started,
			Reconnects:     int(state.reconnects.Load()),
			Version:        version,
		}
	}
	server.Disconnect = func() {
		select {
		case disconnect <- struct{}{}:
		default:
		}
	}
	server.LogTail = logRing.Tail
	bound, err := server.Listen(addr)
	if err != nil {
		log.Printf("UI disabled: %v", err)
		return ""
	}
	return bound
}

func splitTrim(raw, sep string) []string {
	var out []string
	for _, part := range strings.Split(raw, sep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func subagentProfilesToDTO(in []clientconfig.SubagentProfile) []agentops.SubagentProfileConfig {
	out := make([]agentops.SubagentProfileConfig, 0, len(in))
	for _, p := range in {
		out = append(out, agentops.SubagentProfileConfig{
			Name: p.Name, Client: p.Client, Agent: p.Agent, Model: p.Model,
			Thinking: p.Thinking, TimeoutSec: p.TimeoutSec, ExtraArgs: p.ExtraArgs,
		})
	}
	return out
}

func subagentProfilesFromDTO(in []agentops.SubagentProfileConfig) []clientconfig.SubagentProfile {
	out := make([]clientconfig.SubagentProfile, 0, len(in))
	for _, p := range in {
		out = append(out, clientconfig.SubagentProfile{
			Name: p.Name, Client: p.Client, Agent: p.Agent, Model: p.Model,
			Thinking: p.Thinking, TimeoutSec: p.TimeoutSec, ExtraArgs: p.ExtraArgs,
		})
	}
	return out
}

func subagentProfilesFrom(cfg clientconfig.Config) []agentops.SubagentProfile {
	out := make([]agentops.SubagentProfile, 0, len(cfg.SubagentProfiles))
	seen := map[string]bool{}
	for _, p := range cfg.SubagentProfiles {
		name := strings.TrimSpace(p.Name)
		if name == "" || seen[name] || len(name) > 64 {
			continue
		}
		seen[name] = true
		out = append(out, agentops.SubagentProfile{
			Name: name, Client: p.Client, Agent: p.Agent, Model: p.Model,
			Thinking: p.Thinking, TimeoutSec: p.TimeoutSec, ExtraArgs: p.ExtraArgs,
		})
	}
	return out
}

func disabledSet(names []string) map[string]bool {
	out := map[string]bool{}
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	return out
}

// sanitizeCustomTools drops wrappers whose name is empty or shadows a
// built-in tool, and whose target tool is unknown; bad rows must never reach
// the manager's tools/list.
func sanitizeCustomTools(tools []protocol.CustomTool) []protocol.CustomTool {
	builtins := map[string]bool{
		"list_devices": true, "list_workspaces": true, "list_directory": true, "read_file": true,
		"find_files": true, "search_code": true, "git_status": true, "git_diff": true, "project_info": true,
		"find_symbol": true, "find_references": true, "read_symbol": true, "dependency_graph": true,
		"apply_patch": true, "rollback_patch": true,
		"bash": true, "agent": true, "permission_grant": true,
		"list": true, "read": true, "write": true, "edit": true,
	}
	out := make([]protocol.CustomTool, 0, len(tools))
	for _, tool := range tools {
		if !builtins[tool.Tool] || builtins[tool.Name] || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		if len(tool.Name) > 64 {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func writableSet(names []string) map[string]bool {
	out := map[string]bool{}
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	return out
}

func effectiveAccessMode(cfg clientconfig.Config) string {
	if cfg.AccessMode == "full" {
		return "full"
	}
	return "workspaces"
}

func effectiveManagerURL(boot bootOptions, cfg clientconfig.Config) string {
	return firstNonEmpty(boot.managerURLFromEnvOrFlag, cfg.ManagerURL, "ws://127.0.0.1:8080/agent")
}

func effectiveDeviceID(boot bootOptions, cfg clientconfig.Config) string {
	return firstNonEmpty(boot.deviceID, cfg.DeviceID, hostnameSlug())
}

func effectiveDeviceName(boot bootOptions, cfg clientconfig.Config) string {
	return firstNonEmpty(boot.deviceName, cfg.DeviceName, hostname())
}

func validateManagerURL(raw string, allowInsecureWS bool) (*url.URL, error) {
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
		ip := net.ParseIP(host)
		isLoopback := host == "localhost" || host == "host.docker.internal" ||
			(ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast()))
		if isLoopback || allowInsecureWS {
			if !isLoopback {
				log.Printf("WARNING: insecure ws:// manager connection allowed by CODEBRIDGE_ALLOW_INSECURE_WS; use only on a trusted local network")
			}
			return u, nil
		}
		return nil, fmt.Errorf("remote manager URL must use wss://; ws:// is allowed only for loopback development (or with CODEBRIDGE_ALLOW_INSECURE_WS for local container networking)")
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
