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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/felipeness/claude-statusline/internal/server"
	"github.com/felipeness/claude-statusline/internal/statusline"
)

const usage = `claude-statusline — statusline custom + Studio visual pro Claude Code

USAGE:
  claude-statusline render                     consome stdin do Claude Code, escreve linha ANSI
  claude-statusline install [--preset X]       escreve statusLine no ~/.claude/settings.json e o /budget em ~/.claude/commands
                  [--refresh N] [--force]      [--uninstall remove] (preset gateway usa --refresh 60 por padrão)
  claude-statusline preview [--theme] [--style] [--all]
  claude-statusline studio [--port 5556]       abre Web UI Studio em http://localhost:5556
  claude-statusline budget [--json]            consumo no LLM Gateway da empresa (texto ou JSON pro /budget)
  claude-statusline version                    versão do binário

PRESETS: compact (default), max, powerline, gateway
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
	case "budget":
		cmdBudget(os.Args[2:])
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
	// Snapshot do estado ORIGINAL do stdin antes do probe mergear
	// rate_limits — detectAuthMode usa isso pra distinguir "Claude Code
	// nos enviou rate_limits" de "probe encheu rate_limits".
	stdinHadRateLimits := in.RateLimits != nil &&
		(in.RateLimits.FiveHour != nil || in.RateLimits.SevenDay != nil)
	var probe *statusline.ProbeResult
	if cfg.OAuthProbe.Enabled {
		probe = statusline.ProbeOAuth(cfg.OAuthProbe)
		if probe != nil {
			statusline.MergeProbeIntoInput(&in, probe)
		}
	}
	gateway := statusline.ProbeGateway(cfg.Gateway)
	if gateway != nil {
		in.Gateway = gateway.Usage
	}
	onGateway := gateway != nil || statusline.GatewayConfigured(cfg.Gateway)
	in.AuthMode = detectAuthMode(stdinHadRateLimits, probe, onGateway)
	fmt.Println(statusline.Render(&in, cfg))
}

func cmdInstall(args []string) {
	preset := "compact"
	refresh := -1 // -1 = flag ausente, distingue de "--refresh 0" explícito
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
	commandsDir := filepath.Join(home, ".claude", "commands")
	if uninstall {
		removed, backup, err := statusline.Uninstall(settingsPath)
		if err != nil {
			fatal(err)
		}
		// Roda e reporta antes de qualquer return: senão um settings.json
		// sem statusLine (ou um segundo --uninstall) deixa o /budget órfão.
		budgetRemoved, budgetErr := statusline.RemoveBudgetCommand(commandsDir)
		switch {
		case budgetErr != nil:
			fmt.Println("⚠ não consegui remover o /budget:", budgetErr)
		case budgetRemoved:
			fmt.Println("✓ /budget removido de", statusline.BudgetCommandPath(commandsDir))
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
	// Em Windows, Claude Code roda o command via Git Bash (ou PowerShell
	// quando bash ausente). Em bash, backslashes em paths nao-quoted sao
	// interpretados como escape (\U, \b, \f viram literais), corrompendo
	// o caminho. Forward slash funciona nos dois shells e em qualquer
	// versao do Windows desde XP, entao normalizamos.
	if runtime.GOOS == "windows" {
		self = strings.ReplaceAll(self, `\`, `/`)
	}
	cmd := self + " render"
	if preset == "gateway" && refresh == -1 {
		refresh = 60 // budget muda sem turno novo; mesmo intervalo do script do time
	}
	if refresh == -1 {
		refresh = 0 // flag ausente e preset não é gateway: event-driven, sem interval
	}
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
	written, err := statusline.WriteBudgetCommand(commandsDir, self)
	switch {
	case err != nil:
		fmt.Println("⚠ não consegui gravar o /budget:", err)
	case written:
		fmt.Printf("✓ /budget instalado em %s\n", statusline.BudgetCommandPath(commandsDir))
	default:
		fmt.Printf("⚠ %s já existe e não é nosso — preservado\n", statusline.BudgetCommandPath(commandsDir))
	}
	fmt.Println("\nPróximo passo: reinicia o Claude Code (statusLine só carrega no boot).")
}

// cmdBudget imprime o consumo no gateway. Sempre exit 0: o /budget do
// Claude Code precisa do texto (inclusive do erro) pra explicar ao usuário.
func cmdBudget(args []string) {
	asJSON := len(args) > 0 && args[0] == "--json"
	cfg, err := statusline.LoadConfig(configPath())
	if err != nil {
		writeBudgetError(os.Stdout, asJSON, err)
		return
	}
	runBudget(os.Stdout, cfg, asJSON)
}

// runBudget faz o probe do gateway e escreve o relatório em w — texto pro
// humano, ou JSON pro /budget. Isolado de cmdBudget (que resolve config e
// stdout) pra dar pra testar sem tocar rede real: um GatewayConfig com
// Enabled=false já basta pra exercitar o caminho de erro determinístico.
func runBudget(w io.Writer, cfg *statusline.Config, asJSON bool) {
	res, err := statusline.ProbeGatewayDetailed(cfg.Gateway)
	if err != nil {
		writeBudgetError(w, asJSON, err)
		return
	}
	opts := cfg.Components["gateway_budget"]
	report := statusline.NewBudgetReport(res, opts.WarnAt, opts.CriticalAt)
	if !asJSON {
		fmt.Fprint(w, report.Text())
		return
	}
	if err := json.NewEncoder(w).Encode(report); err != nil {
		// w falhou (ex: pipe fechado do lado do Claude Code): não dá pra
		// escrever o relatório nele, mas ainda reportamos o problema em
		// stderr e seguimos exit 0 — nunca quebrar o /budget.
		writeBudgetErrorEnvelope(os.Stderr, err.Error())
	}
}

// writeBudgetError escreve a mensagem de erro tipado do probe em w: texto
// pro humano, ou envelope {"error": ...} em JSON. Se o encode do envelope em
// w falhar, cai pra stderr via Marshal direto (sem Encoder), que não falha
// pra um map[string]string.
func writeBudgetError(w io.Writer, asJSON bool, err error) {
	msg := statusline.BudgetErrorMessage(err)
	if !asJSON {
		fmt.Fprintln(w, "Budget do LLM Gateway indisponível:", msg)
		return
	}
	if encErr := json.NewEncoder(w).Encode(map[string]string{"error": msg}); encErr != nil {
		writeBudgetErrorEnvelope(os.Stderr, msg)
	}
}

func writeBudgetErrorEnvelope(w io.Writer, msg string) {
	data, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintln(w, string(data))
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

// detectAuthMode decide se a sessao do Claude Code esta autenticada via
// LLM Gateway da empresa, env ANTHROPIC_API_KEY ou via OAuth (Claude Max/Pro).
//
// Hierarquia (do sinal mais autoritativo pro mais frouxo):
//
//  0. Sessão via LLM Gateway da empresa (base URL + token do Auth0, ou
//     probe do gateway respondeu) → gateway. Ganha de tudo: nesse modo
//     o Claude Code não manda rate_limits e não há ANTHROPIC_API_KEY.
//
//  1. Claude Code enviou rate_limits no stdin → OAuth. Esse e o unico
//     sinal direto da sessao corrente; CC so popula rate_limits quando
//     a sessao roda em OAuth mode. Ganha sobre env vars porque cobre o
//     caso "Max + ANTHROPIC_API_KEY no env como fallback de outras libs"
//     (CC ignora a env e usa Max — o statusline tem que refletir isso).
//
//  2. env ANTHROPIC_API_KEY presente E stdin sem rate_limits → api_key.
//     Cobre terminais onde a env var foi setada e CC nao tem OAuth ativo
//     pra essa sessao (mesmo que credentials.json exista de um login
//     anterior em outro terminal).
//
//  3. Probe HTTP devolveu rate limits → OAuth. Fallback pra binarios
//     antigos de CC que nao mandam rate_limits no stdin mas tem token
//     OAuth valido no disco.
//
//  4. Default → api_key.
func detectAuthMode(stdinHadRateLimits bool, probe *statusline.ProbeResult, gateway bool) string {
	if gateway {
		return "gateway"
	}
	if stdinHadRateLimits {
		return "oauth"
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "api_key"
	}
	if probe != nil && (probe.FiveHour != nil || probe.SevenDay != nil) {
		return "oauth"
	}
	return "api_key"
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
		Context: func() statusline.ContextWindow {
			cw := statusline.ContextWindow{UsedPercentage: 42, TotalInputTokens: 18432, TotalOutputTokens: 4521}
			cw.Current.InputTokens = 3
			cw.Current.OutputTokens = 436
			cw.Current.CacheReadInputTokens = 38630
			return cw
		}(),
		Cost: statusline.CostInfo{
			TotalCostUSD:      0.32,
			TotalLinesAdded:   45,
			TotalLinesRemoved: 12,
		},
		RateLimits: &statusline.RateLimits{
			FiveHour: &statusline.RateLimitWindow{UsedPercentage: 73},
			SevenDay: &statusline.RateLimitWindow{UsedPercentage: 18},
		},
		Gateway: &statusline.GatewayUsage{
			SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 520_000_000,
			Tokens: 9_700_000, WindowEnd: 1790812800, Scope: "user", CalendarPeriod: "monthly",
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
