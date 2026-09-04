package statusline

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// BudgetReport é a visão normalizada do consumo no gateway, usada pelo
// subcomando budget (texto pro humano, JSON pro /budget do Claude Code).
type BudgetReport struct {
	SpentBRL        float64         `json:"spent_brl"`
	LimitBRL        float64         `json:"limit_brl"`
	BaseLimitBRL    float64         `json:"base_limit_brl"`
	Pct             float64         `json:"pct"`
	Tokens          int64           `json:"tokens"`
	WindowEnd       string          `json:"window_end,omitempty"` // RFC3339 UTC
	Period          string          `json:"period,omitempty"`
	Scope           string          `json:"scope,omitempty"`
	Exceeded        bool            `json:"exceeded"`
	Status          string          `json:"status"` // ok | warn | crit | blocked
	FetchedAt       string          `json:"fetched_at"`
	Stale           bool            `json:"stale,omitempty"`
	CacheWriteError string          `json:"cache_write_error,omitempty"`
	Raw             json.RawMessage `json:"raw,omitempty"`

	windowEnd time.Time
}

func NewBudgetReport(res *GatewayProbeResult, warn, crit float64) BudgetReport {
	u := res.Usage
	r := BudgetReport{
		SpentBRL:     brl(u.SpentBRLMicro),
		LimitBRL:     brl(u.LimitBRLMicro),
		BaseLimitBRL: brl(u.BaseLimitBRLMicro),
		Pct:          math.Floor(u.Pct()),
		Tokens:       u.Tokens,
		Period:       u.CalendarPeriod,
		Scope:        u.Scope,
		Exceeded:     u.Exceeded,
		Status:       budgetStatus(u, warn, crit),
		FetchedAt:    res.FetchedAt.UTC().Format(time.RFC3339),
		Stale:        res.Stale,
		Raw:          res.Raw,
	}
	if res.CacheWriteError != nil {
		r.CacheWriteError = res.CacheWriteError.Error()
	}
	if u.WindowEnd != 0 {
		r.windowEnd = time.Unix(u.WindowEnd, 0).UTC()
		r.WindowEnd = r.windowEnd.Format(time.RFC3339)
	}
	return r
}

func brl(micro int64) float64 { return math.Round(float64(micro)/10_000) / 100 }

func budgetStatus(u *GatewayUsage, warn, crit float64) string {
	if u.Exceeded {
		return "blocked"
	}
	switch Classify(u.Pct(), warn, crit) {
	case SevCrit:
		return "crit"
	case SevWarn:
		return "warn"
	default:
		return "ok"
	}
}

var periodLabels = map[string]string{"monthly": "mensal", "weekly": "semanal", "daily": "diário"}

// Text é o relatório em PT-BR pro terminal.
func (r BudgetReport) Text() string {
	var b strings.Builder
	b.WriteString("Budget do LLM Gateway\n")
	spent := FormatBRLMicro(int64(math.Round(r.SpentBRL * 1_000_000)))
	if r.LimitBRL > 0 {
		fmt.Fprintf(&b, "  gasto     %s de %s (%.0f%%)\n", spent, FormatBRLMicro(int64(math.Round(r.LimitBRL*1_000_000))), r.Pct)
	} else {
		fmt.Fprintf(&b, "  gasto     %s (sem teto informado)\n", spent)
	}
	b.WriteString("  período   " + r.periodLine() + "\n")
	b.WriteString("  escopo    " + r.scopeLine() + "\n")
	if r.Tokens > 0 {
		fmt.Fprintf(&b, "  tokens    %s no período\n", FormatTokens(r.Tokens))
	}
	b.WriteString("  status    " + r.statusLine() + "\n")
	if r.Stale {
		fmt.Fprintf(&b, "  (dados de %s, gateway não respondeu agora)\n", r.fetchedLocal())
	}
	if r.CacheWriteError != "" {
		fmt.Fprintf(&b, "  aviso     cache não gravado: %s\n", r.CacheWriteError)
	}
	return b.String()
}

func (r BudgetReport) periodLine() string {
	label := periodLabels[r.Period]
	if label == "" {
		label = r.Period
	}
	if label == "" {
		label = "período não informado"
	}
	if r.windowEnd.IsZero() {
		return label
	}
	return fmt.Sprintf("%s, reseta em %s", label, r.windowEnd.Format("02/01/2006"))
}

func (r BudgetReport) scopeLine() string {
	if r.Scope == "license" {
		return "licença (pool compartilhado, valores são o agregado da licença)"
	}
	line := "individual"
	if r.BaseLimitBRL > r.LimitBRL && r.LimitBRL > 0 {
		line += " (teto individual limitado pelo teto da licença)"
	}
	return line
}

func (r BudgetReport) statusLine() string {
	switch r.Status {
	case "blocked":
		if r.windowEnd.IsZero() {
			return "BLOQUEADO"
		}
		return "BLOQUEADO até " + r.windowEnd.Format("02/01/2006")
	case "crit":
		return "CRÍTICO"
	case "warn":
		return "ATENÇÃO"
	default:
		return "OK"
	}
}

func (r BudgetReport) fetchedLocal() string {
	t, err := time.Parse(time.RFC3339, r.FetchedAt)
	if err != nil {
		return r.FetchedAt
	}
	return t.Local().Format("15:04")
}

// BudgetErrorMessage traduz os erros tipados do probe pra uma frase que o
// /budget consegue repassar ao usuário.
func BudgetErrorMessage(err error) string {
	return err.Error()
}
