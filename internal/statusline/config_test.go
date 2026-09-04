package statusline

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestSaveConfigCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.toml")
	if err := SaveConfig(path, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}
}

// TestSaveConfigCreatesParentDirWindowsPath cobre o bug original do
// parentDir hand-rolled (removido): ele só procurava por "/", então num
// path com separador "\" (o que filepath.Join produz no Windows) nunca
// achava o diretório pai de verdade. Esse teste não tem como falhar contra
// o código antigo rodando em Linux CI: lá filepath.Join já produz "/", e o
// parentDir manual também acertava — o bug só existe onde o separador nativo
// é "\". Por isso ele só roda com runtime.GOOS == "windows"; não dá pra
// simular esse separador de outro SO sem fingir o teste.
func TestSaveConfigCreatesParentDirWindowsPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("separador \\ só é relevante no Windows")
	}
	base := t.TempDir()
	path := base + `\nested\dir\config.toml`
	if got, want := filepath.Dir(path), base+`\nested\dir`; got != want {
		t.Fatalf("filepath.Dir(%q) = %q, want %q", path, got, want)
	}
	if err := SaveConfig(path, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}
}
