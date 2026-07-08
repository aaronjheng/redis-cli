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

	"github.com/gomodule/redigo/redis"

	"github.com/aaronjheng/redis-cli/internal/ssh"
)

var errCertLoadFailed = errors.New("couldn't load cert data")

type DialConfig struct {
	URI        string
	Host       string
	Port       int
	User       string
	Password   string
	DB         int
	Cluster    bool
	TLS        bool
	ServerName string
	Insecure   bool
	SSH        *ssh.Config
	Cert       []byte
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
func Dial(cfg DialConfig) (redis.Conn, error) {
	if cfg.Cluster {
		return dialCluster(cfg)
	}

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

func buildDialOptions(cfg DialConfig) ([]redis.DialOption, error) {
	dialOptions := []redis.DialOption{}

	config, err := newTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	dialOptions = append(dialOptions, redis.DialTLSConfig(config))

	if cfg.SSH != nil {
		dialFunc, err := ssh.NewDialerFunc(cfg.SSH)
		if err != nil {
			return nil, fmt.Errorf("create SSH dialer: %w", err)
		}

		dialOptions = append(dialOptions,
			redis.DialContextFunc(dialFunc),
			redis.DialReadTimeout(0),
			redis.DialWriteTimeout(0),
		)
	}

	return dialOptions, nil
}

//nolint:ireturn
func dialRedis(connectionurl string, dialOptions []redis.DialOption) (redis.Conn, error) {
	conn, err := redis.DialURL(connectionurl, dialOptions...)
	if err != nil && err.Error() == "ERR wrong number of arguments for 'auth' command" {
		re := regexp.MustCompile(`^(rediss?://)(.*)(:.*@.*)`)
		connectionurl = re.ReplaceAllString(connectionurl, `$1$3`)
		conn, err = redis.DialURL(connectionurl, dialOptions...)
	}

	if err != nil {
		return nil, fmt.Errorf("dial URL: %w", err)
	}

	return conn, nil
}
