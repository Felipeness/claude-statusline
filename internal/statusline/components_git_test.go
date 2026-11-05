package statusline

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitCommandDisablesOptionalLocks(t *testing.T) {
	cmd := gitCommand(t.TempDir(), "status", "--porcelain")
	if !slices.Contains(cmd.Env, "GIT_OPTIONAL_LOCKS=0") {
		t.Fatalf("env sem GIT_OPTIONAL_LOCKS=0: %v", cmd.Env)
	}
}

func TestGitDirtyLeavesNoIndexLock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git indisponivel")
	}
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !gitDirty(dir) {
		t.Fatal("repo com arquivo novo deveria estar sujo")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "index.lock")); !os.IsNotExist(err) {
		t.Fatalf("index.lock deveria nao existir, err=%v", err)
	}
}
