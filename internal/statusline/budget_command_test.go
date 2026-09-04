package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBudgetCommand(t *testing.T) {
	t.Run("creates file with marker and self path", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "commands")
		written, err := WriteBudgetCommand(dir, "C:/Users/dev/bin/claude-statusline.exe")
		if err != nil || !written {
			t.Fatalf("written=%v err=%v", written, err)
		}
		data, _ := os.ReadFile(BudgetCommandPath(dir))
		content := string(data)
		for _, s := range []string{"---\ndescription:", "allowed-tools: Bash(C:/Users/dev/bin/claude-statusline.exe budget:*)", budgetCommandMarker, "!`C:/Users/dev/bin/claude-statusline.exe budget --json`", "micro", "license", "exceeded"} {
			if !strings.Contains(content, s) {
				t.Fatalf("content missing %q:\n%s", s, content)
			}
		}
	})

	t.Run("quotes paths with spaces", func(t *testing.T) {
		content := BudgetCommandContent("C:/Program Files/cs/claude-statusline.exe")
		if !strings.Contains(content, `!`+"`"+`"C:/Program Files/cs/claude-statusline.exe" budget --json`+"`") {
			t.Fatalf("path not quoted:\n%s", content)
		}
	})

	t.Run("overwrites own file", func(t *testing.T) {
		dir := t.TempDir()
		_, _ = WriteBudgetCommand(dir, "/old/claude-statusline")
		written, err := WriteBudgetCommand(dir, "/new/claude-statusline")
		data, _ := os.ReadFile(BudgetCommandPath(dir))
		if err != nil || !written || !strings.Contains(string(data), "/new/claude-statusline") {
			t.Fatalf("written=%v err=%v content=%s", written, err, data)
		}
	})

	t.Run("preserves foreign file", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(BudgetCommandPath(dir), []byte("meu comando"), 0644)
		written, err := WriteBudgetCommand(dir, "/x/claude-statusline")
		data, _ := os.ReadFile(BudgetCommandPath(dir))
		if err != nil || written || string(data) != "meu comando" {
			t.Fatalf("written=%v err=%v content=%s", written, err, data)
		}
	})

	t.Run("propagates unexpected read error instead of overwriting", func(t *testing.T) {
		dir := t.TempDir()
		// budget.md como diretório: ReadFile falha com um erro que não é
		// "não existe" — não podemos sobrescrever algo que não conseguimos
		// nem ler.
		if err := os.Mkdir(BudgetCommandPath(dir), 0755); err != nil {
			t.Fatal(err)
		}
		written, err := WriteBudgetCommand(dir, "/x/claude-statusline")
		if err == nil || written {
			t.Fatalf("written=%v err=%v, want propagated read error", written, err)
		}
	})
}

func TestRemoveBudgetCommand(t *testing.T) {
	dir := t.TempDir()
	if removed, err := RemoveBudgetCommand(dir); err != nil || removed {
		t.Fatalf("missing file: removed=%v err=%v", removed, err)
	}
	_ = os.WriteFile(BudgetCommandPath(dir), []byte("meu comando"), 0644)
	if removed, err := RemoveBudgetCommand(dir); err != nil || removed {
		t.Fatalf("foreign file: removed=%v err=%v", removed, err)
	}
	// WriteBudgetCommand preserva arquivo estrangeiro (não sobrescreve); remove
	// antes de estabelecer o estado "arquivo nosso" pro próximo passo.
	_ = os.Remove(BudgetCommandPath(dir))
	_, _ = WriteBudgetCommand(dir, "/x/claude-statusline")
	if removed, err := RemoveBudgetCommand(dir); err != nil || !removed {
		t.Fatalf("own file: removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(BudgetCommandPath(dir)); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
}
