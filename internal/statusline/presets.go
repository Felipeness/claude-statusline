package statusline

// Presets são configs prontas pra users escolherem no install.
var Presets = map[string]*Config{
	"compact":   compactPreset(),
	"max":       maxPreset(),
	"powerline": powerlinePreset(),
	"plan":      planPreset(),
	"gateway":   gatewayPreset(),
}

// PresetNames lista os nomes (ordem fixa pra UI/help).
var PresetNames = []string{"compact", "max", "powerline", "plan", "gateway"}

// planPreset é pra quem usa assinatura (Pro/Max/Team): esforço na linha de
// identidade e, na de consumo, contexto, limites do plano com dia/hora do
// reset e tempo de sessão. Janela que o plano não manda (ex.: 5h) some sozinha.
func planPreset() *Config {
	c := DefaultConfig()
	c.Style = "plain"
	c.Lines = []Line{
		{
			Components: []string{"cwd", "git", "model", "effort"},
			Separator:  " │ ",
		},
		{
			Components: []string{"context_pct", "context_tokens", "rate_5h", "rate_7d", "session_time", "cost_session"},
			Separator:  " │ ",
		},
	}
	contextOpts := c.Components["context_pct"]
	contextOpts.LabelPrefix = "ctx "
	c.Components["context_pct"] = contextOpts
	return c
}

func compactPreset() *Config {
	c := DefaultConfig()
	c.Style = "plain"
	c.Lines = []Line{
		{
			Components: []string{"cwd", "git", "model", "context_pct", "cost_session"},
			Separator:  " │ ",
		},
	}
	return c
}

func maxPreset() *Config {
	c := DefaultConfig()
	c.Style = "plain"
	c.Lines = []Line{
		{
			Components: []string{"cwd", "git", "ticket", "model", "context_pct", "cost_session", "burn_rate", "rate_5h"},
			Separator:  " │ ",
		},
		{
			Components: []string{"cost_today", "cost_month", "lines_changed", "cluster", "time"},
			Separator:  " · ",
		},
	}
	return c
}

func powerlinePreset() *Config {
	c := DefaultConfig()
	c.Style = "powerline"
	c.Theme = "graphite"
	c.Lines = []Line{
		{
			Components: []string{"cwd", "git", "model", "context_pct", "cost_session"},
		},
	}
	return c
}

// gatewayPreset é o equivalente ao statusline bash do time no LLM Gateway:
// budget em reais, tokens do período, reset e contadores da sessão.
func gatewayPreset() *Config {
	c := DefaultConfig()
	c.Style = "plain"
	c.Lines = []Line{
		{
			Components: []string{"cwd", "git", "model", "gateway_budget", "gateway_tokens", "gateway_reset"},
			Separator:  " │ ",
		},
		{
			Components: []string{"context_pct", "tokens_in", "tokens_out", "tokens_total", "tokens_cache"},
			Separator:  " · ",
		},
	}
	return c
}
