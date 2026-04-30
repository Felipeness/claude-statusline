// claude-statusline — visual editor + render engine pra Claude Code statusLine.
//
// CLI:
//   claude-statusline render             # consumido pelo Claude Code via stdin
//   claude-statusline install            # configura ~/.claude/settings.json
//   claude-statusline preview [--all]    # vê todos themes × styles
//   claude-statusline studio [--port N]  # abre Web UI Studio
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/felipeness/claude-statusline/internal/server"
	"github.com/felipeness/claude-statusline/internal/statusline"
)

const usage = `claude-statusline — statusline custom + Studio visual pro Claude Code

USAGE:
  claude-statusline render                     consome stdin do Claude Code, escreve linha ANSI
  claude-statusline install [--preset X]       escreve statusLine no ~/.claude/settings.json
                  [--refresh N] [--force]      [--uninstall remove]
  claude-statusline preview [--theme] [--style] [--all]
  claude-statusline studio [--port 5556]       abre Web UI Studio em http://localhost:5556

PRESETS: compact (default), max, powerline
THEMES:  graphite (default), nord, dracula, sakura, mono
STYLES:  plain, powerline, capsule

EXAMPLES:
  claude-statusline preview --all              # 15 combinações theme×style no terminal
  claude-statusline install --preset compact   # plug no Claude Code
  claude-statusline studio                     # editor visual web
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "render":
		cmdRender()
	case "install":
		cmdInstall(os.Args[2:])
	case "preview":
		cmdPreview(os.Args[2:])
	case "studio":
		cmdStudio(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", os.Args[1], usage)
		os.Exit(1)
	}
}

func cmdRender() {
	cfg, err := statusline.LoadConfig(configPath())
	if err != nil {
		return
	}
	var in statusline.Input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return
	}
	fmt.Println(statusline.Render(&in, cfg))
}

func cmdInstall(args []string) {
	preset := "compact"
	refresh := 0
	force := false
	uninstall := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--preset":
			if i+1 < len(args) {
				preset = args[i+1]
				i++
			}
		case "--refresh":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					refresh = n
				}
				i++
			}
		case "--force", "-f":
			force = true
		case "--uninstall":
			uninstall = true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if uninstall {
		removed, backup, err := statusline.Uninstall(settingsPath)
		if err != nil {
			fatal(err)
		}
		if !removed {
			fmt.Println("settings.json não tinha statusLine — nada a remover")
			return
		}
		fmt.Printf("✓ statusLine removido de %s\n  backup: %s\n", settingsPath, backup)
		return
	}
	self, err := os.Executable()
	if err != nil {
		fatal(err)
	}
	cmd := self + " render"
	if _, err := os.Stat(configPath()); errors.Is(err, os.ErrNotExist) {
		cfg := statusline.Presets[preset]
		if cfg == nil {
			cfg = statusline.DefaultConfig()
		}
		if err := statusline.SaveConfig(configPath(), cfg); err != nil {
			fatal(err)
		}
		fmt.Printf("✓ config criado em %s (preset: %s)\n", configPath(), preset)
	} else {
		fmt.Printf("✓ config já existe em %s — preservado\n", configPath())
	}
	res, err := statusline.Install(statusline.InstallOptions{
		SettingsPath:    settingsPath,
		Command:         cmd,
		RefreshInterval: refresh,
		Force:           force,
	})
	if err != nil {
		fatal(err)
	}
	if res.Backup != "" {
		fmt.Printf("✓ backup: %s\n", res.Backup)
	}
	if res.Replaced {
		fmt.Println("⚠ statusLine anterior foi sobrescrito")
	}
	fmt.Printf("✓ statusLine instalado em %s\n  command: %s\n", settingsPath, cmd)
	fmt.Println("\nPróximo passo: reinicia o Claude Code (statusLine só carrega no boot).")
}

func cmdPreview(args []string) {
	theme, style := "", ""
	all := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--theme":
			if i+1 < len(args) {
				theme = args[i+1]
				i++
			}
		case "--style":
			if i+1 < len(args) {
				style = args[i+1]
				i++
			}
		case "--all":
			all = true
		}
	}
	mock := mockInput()
	cfg, _ := statusline.LoadConfig(configPath())
	if all {
		styles := []string{"plain", "powerline", "capsule"}
		themes := []string{"graphite", "nord", "dracula", "sakura", "mono"}
		for _, t := range themes {
			for _, st := range styles {
				cfg.Theme = t
				cfg.Style = st
				fmt.Printf("─ %s · %s\n%s\n\n", t, st, statusline.Render(mock, cfg))
			}
		}
		return
	}
	if theme != "" {
		cfg.Theme = theme
	}
	if style != "" {
		cfg.Style = style
	}
	fmt.Println(statusline.Render(mock, cfg))
}

func cmdStudio(args []string) {
	port := 5556
	openBrowser := true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "-p":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					port = n
				}
				i++
			}
		case "--no-open":
			openBrowser = false
		}
	}
	listen := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &server.Server{
		ConfigPath: configPath(),
		Static:     webStatic,
	}
	if openBrowser {
		go func() { _ = openURL("http://" + listen) }()
	}
	fmt.Printf("Studio em http://%s — Ctrl+C pra parar\n", listen)
	if err := server.Run(srv, listen); err != nil {
		fatal(err)
	}
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude-statusline", "config.toml")
}

func mockInput() *statusline.Input {
	return &statusline.Input{
		CWD:       "/Users/dev/projects/my-app",
		SessionID: "preview-mock",
		Model: statusline.ModelInfo{
			DisplayName: "Opus 4.7",
			ID:          "claude-opus-4-7",
		},
		Workspace: statusline.Workspace{
			CurrentDir: "/Users/dev/projects/my-app",
			ProjectDir: "/Users/dev/projects/my-app",
		},
		Context: statusline.ContextWindow{
			UsedPercentage:    42,
			TotalInputTokens:  18432,
			TotalOutputTokens: 4521,
		},
		Cost: statusline.CostInfo{
			TotalCostUSD:      0.32,
			TotalLinesAdded:   45,
			TotalLinesRemoved: 12,
		},
		RateLimits: &statusline.RateLimits{
			FiveHour: &statusline.RateLimitWindow{UsedPercentage: 73},
			SevenDay: &statusline.RateLimitWindow{UsedPercentage: 18},
		},
		Worktree: &statusline.WorktreeInfo{Branch: "feat/CC-1234-statusline"},
	}
}

func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
