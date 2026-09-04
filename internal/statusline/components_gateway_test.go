package statusline

import (
	"strings"
	"testing"
	"time"
)

func gatewayCtx(g *GatewayUsage) *RenderCtx {
	return &RenderCtx{In: &Input{Gateway: g}, Theme: GetTheme("graphite"), Now: time.Now()}
}

func TestGatewayBudgetRender(t *testing.T) {
	theme := GetTheme("graphite")
	opts := ComponentOpts{WarnAt: 70, CriticalAt: 90}
	cases := []struct {
		name     string
		usage    *GatewayUsage
		opts     ComponentOpts
		wantText string
		wantFG   Color
		wantBold bool
		empty    bool
	}{
		{name: "hidden without gateway", usage: nil, empty: true},
		{name: "ok", usage: &GatewayUsage{SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000}, opts: opts,
			wantText: "🟢 R$ 73,53 / R$ 520,00 (14%)", wantFG: theme.SegOf("gateway_budget").FG},
		{name: "warn at 70", usage: &GatewayUsage{SpentBRLMicro: 364_000_000, LimitBRLMicro: 520_000_000}, opts: opts,
			wantText: "🟡 R$ 364,00 / R$ 520,00 (70%)", wantFG: theme.Status.Warn},
		{name: "crit at 90", usage: &GatewayUsage{SpentBRLMicro: 468_000_000, LimitBRLMicro: 520_000_000}, opts: opts,
			wantText: "🔴 R$ 468,00 / R$ 520,00 (90%)", wantFG: theme.Status.Crit},
		{name: "blocked", usage: &GatewayUsage{SpentBRLMicro: 520_000_000, LimitBRLMicro: 520_000_000, Exceeded: true}, opts: opts,
			wantText: "🚫 BLOQUEADO R$ 520,00 / R$ 520,00", wantFG: theme.Status.Crit, wantBold: true},
		{name: "no limit shows spent only", usage: &GatewayUsage{SpentBRLMicro: 73_530_000}, opts: opts,
			wantText: "🟢 R$ 73,53", wantFG: theme.SegOf("gateway_budget").FG},
		{name: "default thresholds when opts empty", usage: &GatewayUsage{SpentBRLMicro: 468_000_000, LimitBRLMicro: 520_000_000},
			wantText: "🔴 R$ 468,00 / R$ 520,00 (90%)", wantFG: theme.Status.Crit},
		{name: "label prefix", usage: &GatewayUsage{SpentBRLMicro: 1_000_000, LimitBRLMicro: 10_000_000}, opts: ComponentOpts{LabelPrefix: "gw "},
			wantText: "gw 🟢 R$ 1,00 / R$ 10,00 (10%)", wantFG: theme.SegOf("gateway_budget").FG},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seg := Get("gateway_budget").Render(gatewayCtx(tc.usage), tc.opts)
			if tc.empty {
				if !seg.Empty() {
					t.Fatalf("expected empty, got %q", seg.Text)
				}
				return
			}
			if seg.Text != tc.wantText {
				t.Fatalf("text = %q, want %q", seg.Text, tc.wantText)
			}
			if seg.FG != tc.wantFG {
				t.Fatalf("fg = %+v, want %+v", seg.FG, tc.wantFG)
			}
			if seg.Bold != tc.wantBold {
				t.Fatalf("bold = %v, want %v", seg.Bold, tc.wantBold)
			}
		})
	}
}

func TestGatewayTokensAndResetRender(t *testing.T) {
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix()
	cases := []struct {
		name  string
		comp  string
		usage *GatewayUsage
		want  string
	}{
		{"tokens", "gateway_tokens", &GatewayUsage{Tokens: 9_700_000}, "9.7M tokens"},
		{"tokens hidden at zero", "gateway_tokens", &GatewayUsage{}, ""},
		{"tokens hidden without gateway", "gateway_tokens", nil, ""},
		{"reset", "gateway_reset", &GatewayUsage{WindowEnd: end}, "reset 01/10"},
		{"reset hidden without window", "gateway_reset", &GatewayUsage{}, ""},
		{"reset hidden without gateway", "gateway_reset", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seg := Get(tc.comp).Render(gatewayCtx(tc.usage), ComponentOpts{})
			if strings.TrimSpace(seg.Text) != tc.want {
				t.Fatalf("%s text = %q, want %q", tc.comp, seg.Text, tc.want)
			}
		})
	}
}

func TestGatewayMetasNeedGateway(t *testing.T) {
	for _, name := range []string{"gateway_budget", "gateway_tokens", "gateway_reset"} {
		meta, ok := componentMetas[name]
		if !ok || !meta.NeedsGateway || meta.Category != "gateway" {
			t.Fatalf("meta %s = %+v (ok=%v)", name, meta, ok)
		}
	}
	if !componentMetas["gateway_budget"].HasWarnAt {
		t.Fatal("gateway_budget must accept thresholds")
	}
}
