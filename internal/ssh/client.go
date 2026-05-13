package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Config struct {
	Host         string
	Port         string
	Username     string
	Password     string
	IdentityFile string
}

type Client struct {
	*ssh.Client
}

func (c *Client) Dial(ctx context.Context, protocol, address string) (net.Conn, error) {
	conn, err := c.DialContext(ctx, protocol, address)
	if err != nil {
		return nil, fmt.Errorf("dial context: %w", err)
	}

	return conn, nil
}

//nolint:gosec
func insecureHostKeyCallback() ssh.HostKeyCallback {
	return ssh.InsecureIgnoreHostKey()
}

func newClient(cfg *Config) (*Client, error) {
	identityFiles := []string{
		"~/.ssh/id_ed25519",
		"~/.ssh/id_ecdsa",
		"~/.ssh/id_dsa",
		"~/.ssh/id_rsa",
	}

	if cfg.IdentityFile == "" {
		identityFiles = append([]string{cfg.IdentityFile}, identityFiles...)
	}

	signers, err := sshSignersFromIdentityFiles(identityFiles)
	if err != nil {
		return nil, fmt.Errorf("sshSignersFromIdentityFiles error: %w", err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		HostKeyCallback: insecureHostKeyCallback(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signers...),
		},
	}

	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", cfg.Host, cfg.Port), sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh.Dial error: %w", err)
	}

	client := &Client{
		Client: sshClient,
	}

	return client, nil
}

func sshSignersFromIdentityFiles(identityFiles []string) ([]ssh.Signer, error) {
	signers := make([]ssh.Signer, 0, len(identityFiles))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("os.UserHomeDir error: %w", err)
	}

	for _, identityFile := range identityFiles {
		resolved := identityFile

		if homeDir != "" && strings.HasPrefix(resolved, "~/") {
			resolved = filepath.Join(homeDir, resolved[2:])
		}

		_, statErr := os.Stat(resolved)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}

			return nil, fmt.Errorf("os.Stat error: %w", statErr)
		}

		raw, fileErr := os.ReadFile(resolved)
		if fileErr != nil {
			return nil, fmt.Errorf("os.ReadFile error: %w", fileErr)
		}

		signer, parseErr := ssh.ParsePrivateKey(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("ssh.ParsePrivateKey error: %w", parseErr)
		}

		signers = append(signers, signer)
	}

	return signers, nil
}
