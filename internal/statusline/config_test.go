package statusline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMergesGatewaySection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	toml := "[gateway]\nenabled = false\nbase_url = \"https://gw.example/llm-gtw\"\nttl = \"30s\"\n"
	if err := os.WriteFile(path, []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.IsEnabled() {
		t.Fatal("expected gateway disabled")
	}
	if cfg.Gateway.BaseURL != "https://gw.example/llm-gtw" || cfg.Gateway.TTL != "30s" {
		t.Fatalf("unexpected gateway cfg %+v", cfg.Gateway)
	}
	if cfg.Gateway.StaleTTL != "1h" || cfg.Gateway.Timeout != "4s" {
		t.Fatalf("defaults not preserved: %+v", cfg.Gateway)
	}
	if cfg.Components["gateway_budget"].WarnAt != 70 || cfg.Components["gateway_budget"].CriticalAt != 90 {
		t.Fatalf("gateway_budget thresholds = %+v", cfg.Components["gateway_budget"])
	}
}

func TestDefaultConfigGatewayEnabled(t *testing.T) {
	if !DefaultConfig().Gateway.IsEnabled() {
		t.Fatal("gateway must be enabled by default")
	}
}
