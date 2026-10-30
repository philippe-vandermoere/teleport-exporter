package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clearEnv resets every environment variable the config package reads, and
// restores the previous value once the test finishes (via t.Setenv). The
// package treats an empty string the same as an unset variable.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		envConfigPath, envListenAddress, envAddress, envIdentityFile, envInsecure,
		envRefreshInterval, envCollectTimeout,
	} {
		t.Setenv(key, "")
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestLoad_YAMLFileWithDefaults(t *testing.T) {
	clearEnv(t)

	yamlPath := writeTempFile(t, "config.yaml", `
address: teleport.example.com:443
identity_file: /var/run/teleport-identity/identity
`)

	cfg, err := Load(yamlPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ListenAddress != defaultListenAddress {
		t.Errorf("ListenAddress = %q, want default %q", cfg.ListenAddress, defaultListenAddress)
	}
	if cfg.RefreshIntervalDuration() != defaultRefreshInterval {
		t.Errorf("RefreshInterval = %v, want default %v", cfg.RefreshIntervalDuration(), defaultRefreshInterval)
	}
	if cfg.CollectTimeoutDuration() != defaultCollectTimeout {
		t.Errorf("CollectTimeout = %v, want default %v", cfg.CollectTimeoutDuration(), defaultCollectTimeout)
	}
	if cfg.Insecure {
		t.Errorf("Insecure = true, want false by default")
	}
	if cfg.AddressHost() != "teleport.example.com" {
		t.Errorf("AddressHost = %q, want teleport.example.com", cfg.AddressHost())
	}
}

func TestLoad_YAMLWithExplicitValues(t *testing.T) {
	clearEnv(t)

	yamlPath := writeTempFile(t, "config.yaml", `
listen_address: ":9999"
address: teleport:3080
identity_file: /identity
insecure: true
refresh_interval: 1m
collect_timeout: 15
`)

	cfg, err := Load(yamlPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ListenAddress != ":9999" {
		t.Errorf("ListenAddress = %q", cfg.ListenAddress)
	}
	if !cfg.Insecure {
		t.Errorf("Insecure = false, want true")
	}
	if cfg.RefreshIntervalDuration() != time.Minute {
		t.Errorf("RefreshInterval = %v, want 1m", cfg.RefreshIntervalDuration())
	}
	if cfg.CollectTimeoutDuration() != 15*time.Second {
		t.Errorf("CollectTimeout = %v, want 15s (plain integer = seconds)", cfg.CollectTimeoutDuration())
	}
}

func TestLoad_EnvOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv(envAddress, "teleport:3080")
	t.Setenv(envIdentityFile, "/identity")
	t.Setenv(envInsecure, "true")
	t.Setenv(envRefreshInterval, "10s")
	t.Setenv(envCollectTimeout, "5s")
	t.Setenv(envListenAddress, ":9000")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Address != "teleport:3080" || cfg.IdentityFile != "/identity" || !cfg.Insecure || cfg.ListenAddress != ":9000" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.RefreshIntervalDuration() != 10*time.Second || cfg.CollectTimeoutDuration() != 5*time.Second {
		t.Errorf("unexpected durations: refresh=%v collect=%v", cfg.RefreshIntervalDuration(), cfg.CollectTimeoutDuration())
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	clearEnv(t)

	yamlPath := writeTempFile(t, "config.yaml", `
address: from-yaml:443
identity_file: /from-yaml
`)
	t.Setenv(envConfigPath, yamlPath)
	t.Setenv(envAddress, "from-env:443")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Address != "from-env:443" {
		t.Errorf("Address = %q, want the env value", cfg.Address)
	}
	if cfg.IdentityFile != "/from-yaml" {
		t.Errorf("IdentityFile = %q, want the YAML value", cfg.IdentityFile)
	}
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing address", map[string]string{envIdentityFile: "/identity"}, "address is required"},
		{"address without port", map[string]string{envAddress: "teleport", envIdentityFile: "/identity"}, "invalid address"},
		{"missing identity file", map[string]string{envAddress: "teleport:443"}, "identity_file is required"},
		{"invalid insecure", map[string]string{envAddress: "teleport:443", envIdentityFile: "/identity", envInsecure: "maybe"}, envInsecure},
		{"invalid refresh interval", map[string]string{envAddress: "teleport:443", envIdentityFile: "/identity", envRefreshInterval: "soon"}, envRefreshInterval},
		{"negative collect timeout", map[string]string{envAddress: "teleport:443", envIdentityFile: "/identity", envCollectTimeout: "-1s"}, "collect_timeout must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := Load("")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
