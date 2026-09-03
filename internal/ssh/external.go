package ssh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const (
	externalReadyTimeout = 15 * time.Second
	externalRetryDelay   = 100 * time.Millisecond
	externalProbeTimeout = 300 * time.Millisecond
	externalKillGrace    = 2 * time.Second
)

var (
	errSSHExitedBeforeReady = errors.New("ssh exited before tunnel was ready")
	errSSHTunnelNotReady    = errors.New("ssh tunnel not ready")
	errUnexpectedListenAddr = errors.New("unexpected listen address type")
)

// externalTunnel dials addresses through a SOCKS5 proxy
// exposed by an external ssh(1) process.
type externalTunnel struct {
	cmd       *exec.Cmd
	proxyAddr string
	stderr    *stderrBuffer
	done      chan struct{}
	// separateGroup is true when the ssh process runs in its own process
	// group so that it can be terminated together with its children.
	separateGroup bool
}

func newExternalTunnel(ctx context.Context, cfg *Config) (*Tunnel, error) {
	port, err := freeLocalPort(ctx)
	if err != nil {
		return nil, fmt.Errorf("freeLocalPort error: %w", err)
	}

	proxyAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	tunnel, err := startExternalSSH(ctx, cfg, proxyAddr)
	if err != nil {
		return nil, err
	}

	err = tunnel.waitReady(ctx)
	if err != nil {
		_ = tunnel.Close()

		return nil, err
	}

	socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		_ = tunnel.Close()

		return nil, fmt.Errorf("proxy.SOCKS5 error: %w", err)
	}

	return &Tunnel{
		DialerFunc: func(_ context.Context, network, addr string) (net.Conn, error) {
			return socksDialer.Dial(network, addr)
		},
		closeFn: tunnel.Close,
	}, nil
}

func startExternalSSH(ctx context.Context, cfg *Config, proxyAddr string) (*externalTunnel, error) {
	args, err := buildSSHArgs(cfg, proxyAddr)
	if err != nil {
		return nil, err
	}

	slog.Info("starting external ssh tunnel", "command", "ssh "+strings.Join(args, " "))

	// #nosec G204 -- launching the user's ssh(1) binary is the purpose of external mode.
	cmd := exec.CommandContext(ctx, "ssh", args...)

	// Interactive ssh prompts read from /dev/tty, which only works when the
	// child shares our foreground process group. Without a controlling
	// terminal, use a separate process group so that all tunnel children are
	// cleaned up together.
	separateGroup := !hasControllingTTY()
	configureProcessGroup(cmd, separateGroup)

	stderr := &stderrBuffer{}
	cmd.Stderr = stderr

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("cmd.Start error: %w", err)
	}

	tunnel := &externalTunnel{
		cmd:           cmd,
		proxyAddr:     proxyAddr,
		stderr:        stderr,
		done:          make(chan struct{}),
		separateGroup: separateGroup,
	}

	go func() {
		_ = cmd.Wait()

		close(tunnel.done)
	}()

	return tunnel, nil
}

func buildSSHArgs(cfg *Config, proxyAddr string) ([]string, error) {
	args := []string{
		"-N",
		"-D", proxyAddr,
		"-o", "ExitOnForwardFailure=yes",
	}

	if cfg.IdentityFile != "" {
		identityFile, err := expandPath(cfg.IdentityFile)
		if err != nil {
			return nil, fmt.Errorf("expandPath error: %w", err)
		}

		args = append(args, "-i", identityFile)
	}

	if cfg.Port != 0 {
		args = append(args, "-p", strconv.Itoa(int(cfg.Port)))
	}

	args = append(args, cfg.Options...)

	target := cfg.Host
	if cfg.User != "" {
		target = cfg.User + "@" + target
	}

	return append(args, target), nil
}

// Close terminates the ssh(1) process that owns the tunnel.
func (t *externalTunnel) Close() error {
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}

	select {
	case <-t.done:
		return nil
	default:
	}

	err := t.terminate()
	if err != nil {
		return fmt.Errorf("terminate error: %w", err)
	}

	select {
	case <-t.done:
		return nil
	case <-time.After(externalKillGrace):
	}

	_ = t.kill()
	<-t.done

	return nil
}

func (t *externalTunnel) terminate() error {
	if t.separateGroup {
		return terminateProcessGroup(t.cmd)
	}

	return terminateProcess(t.cmd)
}

func (t *externalTunnel) kill() error {
	if t.separateGroup {
		return killProcessGroup(t.cmd)
	}

	return killProcess(t.cmd)
}

func (t *externalTunnel) waitReady(ctx context.Context) error {
	timer := time.NewTimer(externalReadyTimeout)
	defer timer.Stop()

	ticker := time.NewTicker(externalRetryDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waitReady error: %w", ctx.Err())
		case <-t.done:
			return fmt.Errorf("%w: %s", errSSHExitedBeforeReady, t.stderr.String())
		case <-ticker.C:
			dialer := &net.Dialer{Timeout: externalProbeTimeout}

			conn, err := dialer.DialContext(ctx, "tcp", t.proxyAddr)
			if err == nil {
				_ = conn.Close()

				return nil
			}
		case <-timer.C:
			return fmt.Errorf("%w within %s: %s", errSSHTunnelNotReady, externalReadyTimeout, t.stderr.String())
		}
	}
}

func freeLocalPort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("net.Listen error: %w", err)
	}

	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("%w: %T", errUnexpectedListenAddr, listener.Addr())
	}

	return addr.Port, nil
}

// stderrBuffer is a concurrency-safe buffer for capturing ssh(1) stderr output.
type stderrBuffer struct {
	mutex sync.Mutex
	buf   strings.Builder
}

func (b *stderrBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	written, err := b.buf.Write(p)
	if err != nil {
		return written, fmt.Errorf("strings.Builder.Write error: %w", err)
	}

	return written, nil
}

func (b *stderrBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return strings.TrimSpace(b.buf.String())
}
