package main

import (
	"bytes"
	"testing"

	"github.com/felipeness/claude-statusline/internal/statusline"
)

func TestDetectAuthMode(t *testing.T) {
	withLimits := &statusline.ProbeResult{FiveHour: &statusline.RateLimitWindow{UsedPercentage: 10}}
	cases := []struct {
		name       string
		stdinLimit bool
		probe      *statusline.ProbeResult
		gateway    bool
		apiKeyEnv  string
		want       string
	}{
		{"gateway wins over everything", true, withLimits, true, "sk-ant", "gateway"},
		{"stdin rate limits mean oauth", true, nil, false, "sk-ant", "oauth"},
		{"api key env without limits", false, nil, false, "sk-ant", "api_key"},
		{"probe limits mean oauth", false, withLimits, false, "", "oauth"},
		{"default api key", false, nil, false, "", "api_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", tc.apiKeyEnv)
			if got := detectAuthMode(tc.stdinLimit, tc.probe, tc.gateway); got != tc.want {
				t.Fatalf("detectAuthMode = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRunBudgetErrorShapes cobre as duas formas de erro do runBudget sem
// tocar rede: gateway desligado no config (Enabled=false) é determinístico e
// dá ErrGatewayDisabled direto, tanto em texto quanto em JSON.
func TestRunBudgetErrorShapes(t *testing.T) {
	disabled := false
	cfg := statusline.DefaultConfig()
	cfg.Gateway.Enabled = &disabled

	cases := []struct {
		name   string
		asJSON bool
		want   string
	}{
		{"text error message", false, "Budget do LLM Gateway indisponível: probe do gateway desligado no config\n"},
		{"json error envelope", true, `{"error":"probe do gateway desligado no config"}` + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			runBudget(&buf, cfg, tc.asJSON)
			if got := buf.String(); got != tc.want {
				t.Fatalf("runBudget output = %q, want %q", got, tc.want)
			}
		})
	}
}
