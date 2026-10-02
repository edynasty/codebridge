package hostipc

// Direction describes who issues a method and who implements it.
//
// The frozen ComputerProvider lives INSIDE CodeBridge.app: the app is the
// provider, the daemon is a broker. So `computer.*` and the approval
// presentation calls are requests the DAEMON sends to the app, and the app
// answers. The daemon never serves them, and a peer sending one as a request
// gets "unsupported".
type Direction string

const (
	// AppToDaemon is a request the app/CLI sends and the daemon answers.
	AppToDaemon Direction = "app_to_daemon"
	// DaemonToApp is a request the daemon sends and the app answers.
	DaemonToApp Direction = "daemon_to_app"
	// DaemonToAppNotification is a notification the daemon sends to the app.
	DaemonToAppNotification Direction = "daemon_to_app_notification"
)

// MethodSpec is one row of the frozen Host IPC v1 method table
// (schema/hostipc/v1/methods.json, README.md §6).
type MethodSpec struct {
	Name string
	// Direction is who issues the method.
	Direction Direction
	// Roles lists the connection roles allowed to call (AppToDaemon) or
	// answer (DaemonToApp) it. For DaemonToApp methods this is the receiver
	// set: only an app connection implements the ComputerProvider.
	Roles []Role
	// Implemented is false for methods that are part of the frozen contract
	// but intentionally not available in Phase 0.
	Implemented bool
}

// Method name constants.
const (
	MethodHello           = "host.hello"
	MethodHealth          = "host.health"
	MethodPrepareRestart  = "host.prepare_restart"
	MethodPhase0Probe     = "host.phase0_probe"
	MethodNativeHostSmoke = "host.native_host_smoke"

	MethodComputerDescribe     = "computer.describe"
	MethodComputerTargets      = "computer.targets"
	MethodComputerOpenSession  = "computer.open_session"
	MethodComputerObserve      = "computer.observe"
	MethodComputerAct          = "computer.act"
	MethodComputerControl      = "computer.control"
	MethodComputerCloseSession = "computer.close_session"

	MethodApprovalPresent  = "approval.present"
	MethodApprovalDecision = "approval.decision"
	MethodApprovalCancel   = "approval.cancel"

	MethodNotifyPost = "notify.post"

	MethodRuntimeHealth  = "runtime.health"
	MethodRuntimeEvents  = "runtime.events"
	MethodRuntimeCommand = "runtime.command"
)

// methodTable is the single source of truth for method existence, direction and
// role authorization.
//
// Phase 0 implements host.hello/host.health/host.native_host_smoke (plus the
// debug-gated host.phase0_probe and the store-backed runtime.health/
// runtime.events when they are registered). Every other row is part of the
// frozen contract and answers unsupported:
//
//   - host.prepare_restart, runtime.command and approval.decision are served by
//     the daemon in a later phase, so Phase 0 must NOT fake them;
//   - computer.* and approval.present/approval.cancel are daemon -> app
//     requests, so the daemon answers unsupported to a peer that sends them.
var methodTable = []MethodSpec{
	{Name: MethodHello, Direction: AppToDaemon, Roles: []Role{RoleApp, RoleDiagnostics, RoleHarness}, Implemented: true},
	{Name: MethodHealth, Direction: AppToDaemon, Roles: []Role{RoleApp, RoleDiagnostics, RoleHarness}, Implemented: true},
	{Name: MethodNativeHostSmoke, Direction: AppToDaemon, Roles: []Role{RoleApp, RoleDiagnostics, RoleHarness}, Implemented: true},
	{Name: MethodPhase0Probe, Direction: AppToDaemon, Roles: []Role{RoleApp, RoleDiagnostics}, Implemented: false},
	{Name: MethodPrepareRestart, Direction: AppToDaemon, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodRuntimeHealth, Direction: AppToDaemon, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodRuntimeEvents, Direction: AppToDaemon, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodRuntimeCommand, Direction: AppToDaemon, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodApprovalDecision, Direction: AppToDaemon, Roles: []Role{RoleApp}, Implemented: false},

	{Name: MethodComputerDescribe, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodComputerTargets, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodComputerOpenSession, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodComputerObserve, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodComputerAct, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodComputerControl, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodComputerCloseSession, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodApprovalPresent, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},
	{Name: MethodApprovalCancel, Direction: DaemonToApp, Roles: []Role{RoleApp}, Implemented: false},

	{Name: MethodNotifyPost, Direction: DaemonToAppNotification, Roles: []Role{RoleApp}, Implemented: true},
}

var methodIndex = func() map[string]MethodSpec {
	m := make(map[string]MethodSpec, len(methodTable))
	for _, spec := range methodTable {
		m[spec.Name] = spec
	}
	return m
}()

// LookupMethod returns the frozen spec for a method name.
func LookupMethod(name string) (MethodSpec, bool) {
	spec, ok := methodIndex[name]
	return spec, ok
}

// Phase0Implemented reports whether the frozen table marks a method as
// available in Phase 0. Handler registration, not this flag, decides whether a
// request is actually served.
func Phase0Implemented(method string) bool {
	spec, ok := methodIndex[method]
	return ok && spec.Implemented
}

// ServableByDaemon reports whether the daemon can answer a method at all: only
// app_to_daemon methods are answered by the daemon. computer.* and the approval
// presentation calls are issued BY the daemon, so it answers unsupported to a
// peer that sends them.
func ServableByDaemon(method string) bool {
	spec, ok := methodIndex[method]
	return ok && spec.Direction == AppToDaemon
}

// MethodTable returns a copy of the frozen table.
func MethodTable() []MethodSpec {
	out := make([]MethodSpec, len(methodTable))
	copy(out, methodTable)
	return out
}

// RoleAllowed reports whether role may call (AppToDaemon) or answer
// (DaemonToApp) spec.
func RoleAllowed(spec MethodSpec, role Role) bool {
	for _, r := range spec.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Authorize checks method existence, direction and role for an inbound request.
// Unknown methods and methods the daemon does not serve answer -32601; a method
// the daemon does serve but the role may not call answers -32011.
func Authorize(method string, role Role) (MethodSpec, *Error) {
	spec, ok := LookupMethod(method)
	if !ok {
		return MethodSpec{}, errUnsupported(method)
	}
	if spec.Direction != AppToDaemon {
		// The daemon issues this method to the app; it never serves it.
		return spec, errUnsupported(method)
	}
	if !RoleAllowed(spec, role) {
		return spec, errRoleForbidden(method, role)
	}
	return spec, nil
}
