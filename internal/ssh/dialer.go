package ssh

import (
	"context"
	"net"
)

type DialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func NewDialerFunc(cfg *Config) (DialerFunc, error) {
	return newDialerFunc(cfg)
}

func (d DialerFunc) Dial(network, addr string) (net.Conn, error) {
	return d(context.Background(), network, addr)
}

func newDialerFunc(cfg *Config) (DialerFunc, error) {
	sshClient, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	return sshClient.Dial, nil
}
