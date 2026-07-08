package config

import (
	"github.com/aaronjheng/redis-cli/internal/ssh"
)

type ProfileConfig struct {
	URI        string      `mapstructure:"uri"`
	Host       string      `mapstructure:"host"`
	Port       int         `mapstructure:"port"`
	DB         int         `mapstructure:"db"`
	Cluster    bool        `mapstructure:"cluster"`
	User       string      `mapstructure:"user"`
	Password   string      `mapstructure:"password"`
	TLS        bool        `mapstructure:"tls"`
	ServerName string      `mapstructure:"sni"`
	Insecure   bool        `mapstructure:"insecure"`
	CACert     string      `mapstructure:"cacert"`
	CertB64    string      `mapstructure:"certb64"`
	SSH        *ssh.Config `mapstructure:"ssh"`
}
