package statusline

import "testing"

func TestGatewayPreset(t *testing.T) {
	cfg := Presets["gateway"]
	if cfg == nil {
		t.Fatal("preset gateway missing")
	}
	if len(cfg.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(cfg.Lines))
	}
	for _, name := range append(cfg.Lines[0].Components, cfg.Lines[1].Components...) {
		if Get(name) == nil {
			t.Fatalf("preset references unknown component %q", name)
		}
	}
	if cfg.Style != "plain" {
		t.Fatalf("style = %q, want plain", cfg.Style)
	}
	if PresetNames[len(PresetNames)-1] != "gateway" {
		t.Fatalf("PresetNames = %v, want gateway last", PresetNames)
	}
}
