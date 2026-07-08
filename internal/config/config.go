package config

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errProfileNotFound = errors.New("profile not found")
	errDefaultNotSet   = errors.New("default profile is not set")
	errNoProfiles      = errors.New("no profiles configured")
)

type Config struct {
	filepath       string
	DefaultProfile string                    `mapstructure:"default_profile"`
	Profiles       map[string]*ProfileConfig `mapstructure:"profiles"`
}

func (c *Config) Filepath() string {
	return c.filepath
}

func (c *Config) Profile(name string) (*ProfileConfig, error) {
	if name == "" {
		name = c.DefaultProfile
	}

	cfg, ok := c.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errProfileNotFound, name)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.DefaultProfile) == "" {
		return errDefaultNotSet
	}

	if len(c.Profiles) == 0 {
		return errNoProfiles
	}

	for name, profile := range c.Profiles {
		if profile == nil {
			return fmt.Errorf("profile %q: %w", name, errProfileNotFound)
		}
	}

	if _, ok := c.Profiles[c.DefaultProfile]; !ok {
		return fmt.Errorf("%w: %s", errProfileNotFound, c.DefaultProfile)
	}

	return nil
}
