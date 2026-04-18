package pluginhost

import (
	"context"
	"errors"
	"net"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc/credentials"
)

// PeerCredsAuthInfo carries the kernel-validated UID/GID/PID of the peer
// process connecting on a Unix domain socket. The PID is the trust anchor
// used to authenticate which plugin opened the connection — it cannot be
// spoofed in user space because SO_PEERCRED is filled in by the kernel
// at connect() time.
type PeerCredsAuthInfo struct {
	PID int32
	UID uint32
	GID uint32
}

// AuthType implements credentials.AuthInfo.
func (PeerCredsAuthInfo) AuthType() string { return "unix-peer-creds" }

// peerCredsTransport is a server-side TransportCredentials that performs
// no cryptographic handshake — it simply reads SO_PEERCRED from the
// incoming Unix socket and exposes it as AuthInfo on the connection. The
// underlying byte stream stays plain HTTP/2, so plugin clients connect
// with regular insecure credentials.
type peerCredsTransport struct{}

// NewPeerCredsTransport returns a TransportCredentials that augments
// each Unix-socket connection with the peer's PID/UID/GID.
func NewPeerCredsTransport() credentials.TransportCredentials {
	return peerCredsTransport{}
}

func (peerCredsTransport) ClientHandshake(_ context.Context, _ string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, nil, nil
}

func (peerCredsTransport) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, nil, errors.New("peer creds: expected *net.UnixConn")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	var ucred *unix.Ucred
	var ucredErr error
	ctrlErr := raw.Control(func(fd uintptr) {
		ucred, ucredErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if ctrlErr != nil {
		return nil, nil, ctrlErr
	}
	if ucredErr != nil {
		return nil, nil, ucredErr
	}
	return conn, PeerCredsAuthInfo{
		PID: ucred.Pid,
		UID: ucred.Uid,
		GID: ucred.Gid,
	}, nil
}

func (peerCredsTransport) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "insecure"}
}

func (peerCredsTransport) Clone() credentials.TransportCredentials {
	return peerCredsTransport{}
}

func (peerCredsTransport) OverrideServerName(string) error {
	return nil
}
