package redis

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"

	redigo "github.com/gomodule/redigo/redis"

	"github.com/aaronjheng/redis-cli/internal/ssh"
)

var errCertLoadFailed = errors.New("couldn't load cert data")

type DialConfig struct {
	URI             string
	Host            string
	Port            int
	User            string
	Password        string
	DB              int
	TLS             bool
	ServerName      string
	Insecure        bool
	SSHURI          string
	SSHIdentityFile string
	Cert            []byte
}

func LoadCert(caCertFile, certB64 string) ([]byte, error) {
	if caCertFile != "" {
		cert, err := os.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("read cert file: %w", err)
		}

		return cert, nil
	} else if certB64 != "" {
		cert, err := base64.StdEncoding.DecodeString(certB64)
		if err != nil {
			return nil, fmt.Errorf("decode base64 cert: %w", err)
		}

		return cert, nil
	}

	return nil, nil
}

//nolint:ireturn
func Dial(cfg DialConfig) (redigo.Conn, error) {
	connectionurl := buildConnectionURL(cfg)

	dialOptions, err := buildDialOptions(cfg)
	if err != nil {
		return nil, err
	}

	conn, err := dialRedis(connectionurl, dialOptions)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	return conn, nil
}

func buildConnectionURL(cfg DialConfig) string {
	if cfg.URI != "" {
		return cfg.URI
	}

	var connectionurl string

	if cfg.TLS {
		connectionurl = "rediss://"
	} else {
		connectionurl = "redis://"
	}

	if cfg.Password != "" {
		connectionurl += url.QueryEscape(cfg.User) + ":" + url.QueryEscape(cfg.Password) + "@"
	}

	connectionurl += cfg.Host + ":" + strconv.Itoa(cfg.Port) + "/" + strconv.Itoa(cfg.DB)

	return connectionurl
}

//nolint:gosec
func newTLSConfig(cfg DialConfig) (*tls.Config, error) {
	config := &tls.Config{InsecureSkipVerify: cfg.Insecure}

	if len(cfg.Cert) > 0 {
		config.RootCAs = x509.NewCertPool()
		config.ClientAuth = tls.RequireAndVerifyClientCert

		ok := config.RootCAs.AppendCertsFromPEM(cfg.Cert)
		if !ok {
			return nil, errCertLoadFailed
		}
	}

	if cfg.ServerName != "" {
		config.ServerName = cfg.ServerName
	}

	return config, nil
}

func buildDialOptions(cfg DialConfig) ([]redigo.DialOption, error) {
	dialOptions := []redigo.DialOption{}

	config, err := newTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	dialOptions = append(dialOptions, redigo.DialTLSConfig(config))

	if cfg.SSHURI != "" {
		sshDialOpts, err := buildSSHDialOptions(cfg.SSHURI, cfg.SSHIdentityFile)
		if err != nil {
			return nil, err
		}

		dialOptions = append(dialOptions, sshDialOpts...)
	}

	return dialOptions, nil
}

func buildSSHDialOptions(sshURI, sshIdentityFile string) ([]redigo.DialOption, error) {
	sshURL, err := url.Parse("ssh://" + sshURI)
	if err != nil {
		return nil, fmt.Errorf("parse SSH URI: %w", err)
	}

	port := sshURL.Port()
	if port == "" {
		port = "22"
	}

	portNum, err := strconv.ParseInt(port, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse SSH port: %w", err)
	}

	cfg := &ssh.Config{
		Host:         sshURL.Hostname(),
		Port:         int32(portNum),
		User:         sshURL.User.Username(),
		IdentityFile: sshIdentityFile,
	}

	dialFunc, err := ssh.NewDialerFunc(cfg)
	if err != nil {
		return nil, fmt.Errorf("create SSH dialer: %w", err)
	}

	return []redigo.DialOption{
		redigo.DialContextFunc(dialFunc),
		redigo.DialReadTimeout(0),
		redigo.DialWriteTimeout(0),
	}, nil
}

//nolint:ireturn
func dialRedis(connectionurl string, dialOptions []redigo.DialOption) (redigo.Conn, error) {
	conn, err := redigo.DialURL(connectionurl, dialOptions...)
	if err != nil && err.Error() == "ERR wrong number of arguments for 'auth' command" {
		re := regexp.MustCompile(`^(rediss?://)(.*)(:.*@.*)`)
		connectionurl = re.ReplaceAllString(connectionurl, `$1$3`)
		conn, err = redigo.DialURL(connectionurl, dialOptions...)
	}

	if err != nil {
		return nil, fmt.Errorf("dial URL: %w", err)
	}

	return conn, nil
}
