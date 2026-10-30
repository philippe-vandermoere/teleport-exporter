// Package config loads the exporter configuration from a YAML file,
// environment variables, or a combination of both.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultListenAddress   = ":9180"
	defaultRefreshInterval = 30 * time.Second
	defaultCollectTimeout  = 20 * time.Second

	envConfigPath      = "TELEPORT_EXPORTER_CONFIG"
	envListenAddress   = "TELEPORT_EXPORTER_LISTEN_ADDRESS"
	envAddress         = "TELEPORT_EXPORTER_ADDRESS"
	envIdentityFile    = "TELEPORT_EXPORTER_IDENTITY_FILE"
	envInsecure        = "TELEPORT_EXPORTER_INSECURE"
	envRefreshInterval = "TELEPORT_EXPORTER_REFRESH_INTERVAL"
	envCollectTimeout  = "TELEPORT_EXPORTER_COLLECT_TIMEOUT"
)

// Duration wraps time.Duration to support YAML values like "5s" as well as
// plain integers (interpreted as seconds).
type Duration time.Duration

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case string:
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", v, err)
		}
		*d = Duration(parsed)
	case int:
		*d = Duration(time.Duration(v) * time.Second)
	default:
		return fmt.Errorf("unsupported duration value: %v", raw)
	}
	return nil
}

// Config is the fully resolved exporter configuration.
type Config struct {
	ListenAddress string `yaml:"listen_address"`

	// Address is the Teleport proxy (or auth) server, as host:port.
	Address string `yaml:"address"`

	// IdentityFile is the path to a Teleport identity file, typically
	// written and periodically renewed by a tbot "identity" output. It is
	// re-read before every collection, so a renewed certificate is picked up
	// without restarting the exporter.
	IdentityFile string `yaml:"identity_file"`

	// Insecure skips TLS verification of the proxy's web certificate
	// during address discovery (self-signed test clusters only).
	Insecure bool `yaml:"insecure"`

	// RefreshInterval is how often the Teleport inventory is collected in
	// the background. Scrapes are served from the last collection and never
	// call the Teleport API themselves.
	RefreshInterval Duration `yaml:"refresh_interval"`

	// CollectTimeout bounds one full collection (every API call it makes).
	CollectTimeout Duration `yaml:"collect_timeout"`
}

// Load builds the configuration from an optional YAML file path and
// environment variables. flagConfigPath takes precedence over the
// TELEPORT_EXPORTER_CONFIG environment variable; either may be empty, in
// which case the configuration is built entirely from environment
// variables.
func Load(flagConfigPath string) (*Config, error) {
	path := flagConfigPath
	if path == "" {
		path = os.Getenv(envConfigPath)
	}

	cfg := &Config{}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", path, err)
		}
	}

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}

	setDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) error {
	if v := os.Getenv(envListenAddress); v != "" {
		cfg.ListenAddress = v
	}
	if v := os.Getenv(envAddress); v != "" {
		cfg.Address = v
	}
	if v := os.Getenv(envIdentityFile); v != "" {
		cfg.IdentityFile = v
	}
	if v := os.Getenv(envInsecure); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid %s value %q: %w", envInsecure, v, err)
		}
		cfg.Insecure = b
	}
	if err := durationFromEnv(envRefreshInterval, &cfg.RefreshInterval); err != nil {
		return err
	}
	return durationFromEnv(envCollectTimeout, &cfg.CollectTimeout)
}

func durationFromEnv(name string, dst *Duration) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", name, v, err)
	}
	*dst = Duration(d)
	return nil
}

func setDefaults(cfg *Config) {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = defaultListenAddress
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = Duration(defaultRefreshInterval)
	}
	if cfg.CollectTimeout == 0 {
		cfg.CollectTimeout = Duration(defaultCollectTimeout)
	}
}

func validate(cfg *Config) error {
	if cfg.Address == "" {
		return fmt.Errorf("address is required: set address in the config file or %s", envAddress)
	}
	if _, _, err := net.SplitHostPort(cfg.Address); err != nil {
		return fmt.Errorf("invalid address %q: %w", cfg.Address, err)
	}
	// The identity file itself is not checked here: with a tbot sidecar it
	// may legitimately not exist yet at startup, and is retried on every
	// collection instead.
	if cfg.IdentityFile == "" {
		return fmt.Errorf("identity_file is required: set identity_file in the config file or %s", envIdentityFile)
	}
	if cfg.RefreshInterval < 0 {
		return fmt.Errorf("refresh_interval must be positive, got %s", cfg.RefreshInterval)
	}
	if cfg.CollectTimeout < 0 {
		return fmt.Errorf("collect_timeout must be positive, got %s", cfg.CollectTimeout)
	}
	return nil
}

// AddressHost returns the host part of Address, without its port.
func (c *Config) AddressHost() string {
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil {
		return c.Address
	}
	return host
}

// RefreshIntervalDuration returns the configured refresh interval as a
// time.Duration.
func (c *Config) RefreshIntervalDuration() time.Duration {
	return time.Duration(c.RefreshInterval)
}

// CollectTimeoutDuration returns the configured collect timeout as a
// time.Duration.
func (c *Config) CollectTimeoutDuration() time.Duration {
	return time.Duration(c.CollectTimeout)
}
