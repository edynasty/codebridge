//go:build !darwin

package hostipc

import (
	"fmt"
	"net"
)

// PeerCred identifies the process at the other end of a connected Unix socket.
type PeerCred struct {
	UID uint32
	PID int
}

// ErrPeerCredUnsupported reports that the platform cannot report peer
// credentials; the caller must fail closed.
var ErrPeerCredUnsupported = fmt.Errorf("hostipc: peer credentials are unavailable on this platform")

// PeerCredentials fails closed on non-Darwin platforms. Host IPC v1 is
// macOS-only; a Windows named-pipe transport would implement this function.
func PeerCredentials(c *net.UnixConn) (PeerCred, error) {
	return PeerCred{}, ErrPeerCredUnsupported
}

// PeerPID fails closed on non-Darwin platforms.
func PeerPID(c *net.UnixConn) (int, error) {
	return 0, ErrPeerCredUnsupported
}

// SocketPeerCredentials fails closed on non-Darwin platforms.
func SocketPeerCredentials(fd int) (PeerCred, error) {
	return PeerCred{}, ErrPeerCredUnsupported
}
