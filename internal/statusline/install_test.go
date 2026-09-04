package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPreset(t *testing.T) {
	const existingContent = "theme = \"nord\"\nstyle = \"plain\"\n"

	tests := []struct {
		name         string
		seedExisting bool
		force        bool
		wantApplied  bool
		wantBackup   bool
	}{
		{
			name:         "config ausente grava o preset",
			seedExisting: false,
			force:        false,
			wantApplied:  true,
			wantBackup:   false,
		},
		{
			name:         "config existente sem force preserva o conteúdo",
			seedExisting: true,
			force:        false,
			wantApplied:  false,
			wantBackup:   false,
		},
		{
			name:         "config existente com force sobrescreve e faz backup",
			seedExisting: true,
			force:        true,
			wantApplied:  true,
			wantBackup:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			if tt.seedExisting {
				if err := os.WriteFile(path, []byte(existingContent), 0600); err != nil {
					t.Fatal(err)
				}
			}

			applied, backup, err := ApplyPreset(path, Presets["gateway"], tt.force)
			if err != nil {
				t.Fatalf("ApplyPreset: %v", err)
			}
			if applied != tt.wantApplied {
				t.Fatalf("applied = %v, want %v", applied, tt.wantApplied)
			}
			if tt.wantBackup && backup == "" {
				t.Fatal("expected backup path, got empty")
			}
			if !tt.wantBackup && backup != "" {
				t.Fatalf("expected no backup, got %q", backup)
			}

			written, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read written config: %v", err)
			}

			switch {
			case !tt.seedExisting:
				if !strings.Contains(string(written), "gateway_budget") {
					t.Fatalf("expected preset config with gateway_budget, got %s", written)
				}
			case !tt.force:
				if string(written) != existingContent {
					t.Fatalf("expected content preserved, got %s", written)
				}
			default:
				if !strings.Contains(string(written), "gateway_budget") {
					t.Fatalf("expected overwritten config with gateway_budget, got %s", written)
				}
				backupContent, err := os.ReadFile(backup)
				if err != nil {
					t.Fatalf("read backup: %v", err)
				}
				if string(backupContent) != existingContent {
					t.Fatalf("expected backup to hold original content, got %s", backupContent)
				}
			}
		})
	}
}
