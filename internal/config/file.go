package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

const (
	configDirPermission = 0o755
	defaultRedisPort    = 6379
)

func defaultConfig() *Config {
	return &Config{
		filepath:       "",
		DefaultProfile: "default",
		Profiles: map[string]*ProfileConfig{
			"default": {
				Host: "127.0.0.1",
				Port: defaultRedisPort,
			},
		},
	}
}

func LoadConfig(cfgFilepath string) (*Config, error) {
	cfgRoot := filepath.Join(xdg.ConfigHome, "redis")

	viperConfig := viper.New()
	if cfgFilepath != "" {
		viperConfig.SetConfigFile(cfgFilepath)
	} else {
		err := os.MkdirAll(cfgRoot, configDirPermission)
		if err != nil {
			return nil, fmt.Errorf("create config dir: %w", err)
		}

		viperConfig.SetConfigName("redis")
		viperConfig.SetConfigType("yaml")
		viperConfig.AddConfigPath(".")
		viperConfig.AddConfigPath(cfgRoot)
	}

	err := viperConfig.ReadInConfig()
	if err != nil {
		var errNotFound viper.ConfigFileNotFoundError
		if cfgFilepath == "" && errors.As(err, &errNotFound) {
			return defaultConfig(), nil
		}

		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		filepath:       viperConfig.ConfigFileUsed(),
		DefaultProfile: "",
		Profiles:       nil,
	}

	err = viperConfig.Unmarshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	err = cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}
