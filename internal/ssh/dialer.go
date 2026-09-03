package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

var errUnsupportedSSHMode = errors.New("unsupported ssh mode")

// DialerFunc dials a network address through an SSH tunnel.
type DialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (d DialerFunc) Dial(network, addr string) (net.Conn, error) {
	return d(context.Background(), network, addr)
}

func (d DialerFunc) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d(ctx, network, addr)
}

// Tunnel dials addresses through an SSH server.
// Close must be called to release the underlying resources.
type Tunnel struct {
	DialerFunc

	closeFn func() error
}

// NewTunnel creates a tunnel using the built-in SSH client
// or an external ssh(1) process, depending on the mode in cfg.
func NewTunnel(ctx context.Context, cfg *Config) (*Tunnel, error) {
	switch cfg.Mode {
	case "", ModeBuiltin:
		return newBuiltinTunnel(cfg)
	case ModeExternal:
		return newExternalTunnel(ctx, cfg)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedSSHMode, cfg.Mode)
	}
}

func newBuiltinTunnel(cfg *Config) (*Tunnel, error) {
	sshClient, err := NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("NewClient error: %w", err)
	}

	return &Tunnel{
		DialerFunc: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := sshClient.Dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			return &deadlineConn{Conn: conn}, nil
		},
		closeFn: sshClient.Close,
	}, nil
}

// Close releases the resources used by the tunnel.
func (t *Tunnel) Close() error {
	if t == nil || t.closeFn == nil {
		return nil
	}

	return t.closeFn()
}

// deadlineConn wraps a net.Conn and silently ignores deadline errors.
// x/crypto/ssh channels do not support deadlines and return
// "ssh: tcpChan: deadline not supported", which causes failures.
type deadlineConn struct {
	net.Conn
}

func (c *deadlineConn) SetDeadline(_ time.Time) error      { return nil }
func (c *deadlineConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *deadlineConn) SetWriteDeadline(_ time.Time) error { return nil }
