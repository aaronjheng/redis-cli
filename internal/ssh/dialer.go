package ssh

import (
	"context"
	"fmt"
	"net"
	"time"
)

type DialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func NewDialerFunc(cfg *Config) (DialerFunc, error) {
	sshClient, err := NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("NewClient error: %w", err)
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := sshClient.Dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		return &deadlineConn{Conn: conn}, nil
	}, nil
}

func (d DialerFunc) Dial(network, addr string) (net.Conn, error) {
	return d(context.Background(), network, addr)
}

func (d DialerFunc) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d(ctx, network, addr)
}

type deadlineConn struct {
	net.Conn
}

func (c *deadlineConn) SetDeadline(_ time.Time) error      { return nil }
func (c *deadlineConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *deadlineConn) SetWriteDeadline(_ time.Time) error { return nil }
