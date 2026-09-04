package statusline

import (
	"fmt"
	"math"
)

// Thresholds usados quando o config do usuário não tem gateway_budget
// (config antigo, anterior a esse component).
const (
	gatewayWarnDefault = 70
	gatewayCritDefault = 90
)

func severityIcon(s Severity) string {
	switch s {
	case SevWarn:
		return "🟡"
	case SevCrit:
		return "🔴"
	default:
		return "🟢"
	}
}

// =============================================================================
// gateway_budget — 🟢 R$ gasto / R$ limite (pct%), 🚫 BLOQUEADO quando exceeded.
// =============================================================================

type gatewayBudgetComp struct{}

func (gatewayBudgetComp) Name() string { return "gateway_budget" }
func (gatewayBudgetComp) Render(c *RenderCtx, opts ComponentOpts) Segment {
	g := c.In.Gateway
	if g == nil {
		return Segment{}
	}
	seg := c.Theme.SegOf("gateway_budget")
	spent := FormatBRLMicro(g.SpentBRLMicro)
	if g.Exceeded {
		text := opts.LabelPrefix + "🚫 BLOQUEADO " + spent
		if g.LimitBRLMicro > 0 {
			text += " / " + FormatBRLMicro(g.LimitBRLMicro)
		}
		return Segment{Name: "gateway_budget", Text: text, FG: c.Theme.Status.Crit, BG: seg.BG, Bold: true}
	}
	if g.LimitBRLMicro <= 0 {
		return Segment{Name: "gateway_budget", Text: opts.LabelPrefix + "🟢 " + spent, FG: seg.FG, BG: seg.BG}
	}
	warn, crit := opts.WarnAt, opts.CriticalAt
	if warn == 0 && crit == 0 {
		warn, crit = gatewayWarnDefault, gatewayCritDefault
	}
	pct := g.Pct()
	sev := Classify(pct, warn, crit)
	text := fmt.Sprintf("%s%s %s / %s (%d%%)", opts.LabelPrefix, severityIcon(sev), spent, FormatBRLMicro(g.LimitBRLMicro), int(math.Floor(pct)))
	fg := seg.FG
	if sev != SevOK {
		fg = c.Theme.SeverityFG(sev)
	}
	return Segment{Name: "gateway_budget", Text: text, FG: fg, BG: seg.BG}
}

// =============================================================================
// gateway_tokens — tokens processados no período (todas as sessões).
// =============================================================================

type gatewayTokensComp struct{}

func (gatewayTokensComp) Name() string { return "gateway_tokens" }
func (gatewayTokensComp) Render(c *RenderCtx, opts ComponentOpts) Segment {
	g := c.In.Gateway
	if g == nil || g.Tokens == 0 {
		return Segment{}
	}
	seg := c.Theme.SegOf("gateway_tokens")
	return Segment{Name: "gateway_tokens", Text: opts.LabelPrefix + FormatTokens(g.Tokens) + " tokens", FG: seg.FG, BG: seg.BG}
}

// =============================================================================
// gateway_reset — "reset dd/mm" do fim da janela de budget.
// =============================================================================

type gatewayResetComp struct{}

func (gatewayResetComp) Name() string { return "gateway_reset" }
func (gatewayResetComp) Render(c *RenderCtx, opts ComponentOpts) Segment {
	g := c.In.Gateway
	if g == nil || g.ResetDate() == "" {
		return Segment{}
	}
	seg := c.Theme.SegOf("gateway_reset")
	return Segment{Name: "gateway_reset", Text: opts.LabelPrefix + "reset " + g.ResetDate(), FG: seg.FG, BG: seg.BG}
}

func init() {
	Register(gatewayBudgetComp{})
	Register(gatewayTokensComp{})
	Register(gatewayResetComp{})
}
