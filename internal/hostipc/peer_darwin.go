//go:build darwin

package hostipc

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// PeerCred identifies the process at the other end of a connected Unix socket.
type PeerCred struct {
	// UID is the effective UID of the peer (getpeereid / LOCAL_PEERCRED).
	UID uint32
	// PID is the peer's process id (LOCAL_PEERPID). It is read from the live
	// socket credential, never from a message field.
	PID int
}

// ErrPeerCredUnsupported reports that the platform cannot report peer
// credentials; the caller must fail closed.
var ErrPeerCredUnsupported = fmt.Errorf("hostipc: peer credentials are unavailable on this platform")

// PeerCredentials reads the peer UID and PID from a connected Unix socket.
func PeerCredentials(c *net.UnixConn) (PeerCred, error) {
	return readPeerCred(c, true)
}

// PeerPID re-reads only the peer PID. The handshake calls it again to detect a
// socket whose pid changed between accept and hello (fd passing).
func PeerPID(c *net.UnixConn) (int, error) {
	cred, err := readPeerCred(c, false)
	if err != nil {
		return 0, err
	}
	return cred.PID, nil
}

func readPeerCred(c *net.UnixConn, wantUID bool) (PeerCred, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return PeerCred{}, fmt.Errorf("hostipc: syscall conn: %w", err)
	}
	var cred PeerCred
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		if wantUID {
			xu, cerr := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
			if cerr != nil {
				sockErr = fmt.Errorf("getpeereid: %w", cerr)
				return
			}
			cred.UID = xu.Uid
		}
		pid, perr := unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if perr != nil {
			sockErr = fmt.Errorf("LOCAL_PEERPID: %w", perr)
			return
		}
		cred.PID = pid
	}); err != nil {
		return PeerCred{}, fmt.Errorf("hostipc: peer credential control: %w", err)
	}
	if sockErr != nil {
		return PeerCred{}, fmt.Errorf("hostipc: %w", sockErr)
	}
	return cred, nil
}

// SocketPeerCredentials reports the leading peer credentials of a listener's
// accepted connection using only the local syscall surface. It exists so the
// daemon can log the peer identity; authorization uses PeerCredentials.
func SocketPeerCredentials(fd int) (PeerCred, error) {
	xu, err := unix.GetsockoptXucred(fd, unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return PeerCred{}, fmt.Errorf("hostipc: getpeereid: %w", err)
	}
	pid, err := unix.GetsockoptInt(fd, unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	if err != nil {
		return PeerCred{}, fmt.Errorf("hostipc: LOCAL_PEERPID: %w", err)
	}
	return PeerCred{UID: xu.Uid, PID: pid}, nil
}
