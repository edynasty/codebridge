//go:build darwin && cgo

package hostipc

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdio.h>
#include <stdlib.h>

// cb_verify_pid validates the LIVE process identified by pid against a code
// requirement string using SecCodeCheckValidity. It never inspects a path.
static OSStatus cb_verify_pid(int pid, const char *requirement) {
	CFNumberRef pidnum = CFNumberCreate(NULL, kCFNumberIntType, &pid);
	if (pidnum == NULL) return errSecAllocate;
	const void *keys[1] = { kSecGuestAttributePid };
	const void *vals[1] = { pidnum };
	CFDictionaryRef attrs = CFDictionaryCreate(NULL, keys, vals, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFRelease(pidnum);
	if (attrs == NULL) return errSecAllocate;

	SecCodeRef guest = NULL;
	OSStatus st = SecCodeCopyGuestWithAttributes(NULL, attrs, kSecCSDefaultFlags, &guest);
	CFRelease(attrs);
	if (st != errSecSuccess || guest == NULL) return st;

	CFStringRef reqStr = CFStringCreateWithCString(NULL, requirement, kCFStringEncodingUTF8);
	if (reqStr == NULL) { CFRelease(guest); return errSecAllocate; }
	SecRequirementRef req = NULL;
	st = SecRequirementCreateWithString(reqStr, kSecCSDefaultFlags, &req);
	CFRelease(reqStr);
	if (st == errSecSuccess && req != NULL) {
		st = SecCodeCheckValidity(guest, kSecCSDefaultFlags, req);
		CFRelease(req);
	}
	CFRelease(guest);
	return st;
}

// cb_copy_identity writes "team=<OU>;identifier=<id>" for diagnostics.
static OSStatus cb_copy_identity(int pid, char *out, size_t outlen) {
	out[0] = 0;
	CFNumberRef pidnum = CFNumberCreate(NULL, kCFNumberIntType, &pid);
	if (pidnum == NULL) return errSecAllocate;
	const void *keys[1] = { kSecGuestAttributePid };
	const void *vals[1] = { pidnum };
	CFDictionaryRef attrs = CFDictionaryCreate(NULL, keys, vals, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFRelease(pidnum);
	if (attrs == NULL) return errSecAllocate;

	SecCodeRef guest = NULL;
	OSStatus st = SecCodeCopyGuestWithAttributes(NULL, attrs, kSecCSDefaultFlags, &guest);
	CFRelease(attrs);
	if (st != errSecSuccess || guest == NULL) return st;

	CFDictionaryRef info = NULL;
	st = SecCodeCopySigningInformation(guest, kSecCSSigningInformation, &info);
	CFRelease(guest);
	if (st != errSecSuccess || info == NULL) return st;

	CFStringRef team = (CFStringRef)CFDictionaryGetValue(info, kSecCodeInfoTeamIdentifier);
	CFStringRef ident = (CFStringRef)CFDictionaryGetValue(info, kSecCodeInfoIdentifier);
	char teambuf[128] = {0};
	char idbuf[512] = {0};
	if (team != NULL) CFStringGetCString(team, teambuf, sizeof(teambuf), kCFStringEncodingUTF8);
	if (ident != NULL) CFStringGetCString(ident, idbuf, sizeof(idbuf), kCFStringEncodingUTF8);
	snprintf(out, outlen, "team=%s;identifier=%s", teambuf, idbuf);
	CFRelease(info);
	return errSecSuccess;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// SignatureError reports a failed live peer code-signature validation.
type SignatureError struct {
	PID    int
	Status int32
	Reason string
	Detail string
}

func (e *SignatureError) Error() string {
	return fmt.Sprintf("hostipc: %s (pid %d, OSStatus %d): %s", e.Reason, e.PID, e.Status, e.Detail)
}

// AppRequirement builds the code requirement that role "app" must satisfy:
// an Apple-anchored signature whose team identifier and bundle identifier match
// the configured CodeBridge.app identity.
func AppRequirement(teamID, bundleID string) string {
	return fmt.Sprintf("anchor apple generic and certificate leaf[subject.OU] = %q and identifier %q", teamID, bundleID)
}

// VerifyPeerCodeSignature validates the running process with the given pid
// against requirement. Authorization uses the live pid taken from
// LOCAL_PEERPID at accept time; no executable path is consulted.
func VerifyPeerCodeSignature(pid int, requirement string) error {
	if pid <= 0 {
		return &SignatureError{PID: pid, Reason: "app_signature_invalid", Detail: "no live peer pid"}
	}
	if requirement == "" {
		return &SignatureError{PID: pid, Reason: "signing_identity_not_configured", Detail: "empty code requirement"}
	}
	creq := C.CString(requirement)
	defer C.free(unsafe.Pointer(creq))
	st := int32(C.cb_verify_pid(C.int(pid), creq))
	if st != 0 {
		return &SignatureError{
			PID:    pid,
			Status: st,
			Reason: "app_signature_invalid",
			Detail: fmt.Sprintf("SecCodeCheckValidity failed for requirement %q", requirement),
		}
	}
	return nil
}

// PeerSignatureInfo returns the live signer's team identifier and signing
// identifier for diagnostics and evidence. It never authorizes anything.
func PeerSignatureInfo(pid int) (string, error) {
	buf := make([]byte, 640)
	st := int32(C.cb_copy_identity(C.int(pid), (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	if st != 0 {
		return "", &SignatureError{PID: pid, Status: st, Reason: "app_signature_indeterminate", Detail: "signing information unavailable"}
	}
	return cString(buf), nil
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}
