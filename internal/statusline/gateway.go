package statusline

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// GatewayUsage é o consumo do usuário no LLM Gateway da empresa (janela
// atual). Preenchido pelo probe em render, ou pelo mock do Studio.
// Valores monetários em micro-reais (1 real = 1_000_000).
type GatewayUsage struct {
	SpentBRLMicro     int64  `json:"spent_brl_micro"`
	LimitBRLMicro     int64  `json:"limit_brl_micro"`      // effectiveLimit, fallback limit
	BaseLimitBRLMicro int64  `json:"base_limit_brl_micro"` // limit (teto antes do teto da licença)
	Tokens            int64  `json:"tokens"`
	WindowEnd         int64  `json:"window_end"` // unix epoch; 0 = desconhecido
	Exceeded          bool   `json:"exceeded"`
	Scope             string `json:"scope,omitempty"`           // user | license | ""
	CalendarPeriod    string `json:"calendar_period,omitempty"` // monthly | weekly | ""
}

// Pct é o percentual gasto do limite efetivo. 0 quando não há limite.
func (g *GatewayUsage) Pct() float64 {
	if g == nil || g.LimitBRLMicro <= 0 {
		return 0
	}
	return float64(g.SpentBRLMicro) / float64(g.LimitBRLMicro) * 100
}

// ResetDate devolve "dd/mm" do fim da janela em UTC (mesmo dia que o
// gateway declara, sem deslocar pelo fuso local). Vazio quando desconhecido.
func (g *GatewayUsage) ResetDate() string {
	if g == nil || g.WindowEnd == 0 {
		return ""
	}
	return time.Unix(g.WindowEnd, 0).UTC().Format("02/01")
}

// Schema do GET /v1/usage do gateway, inferido do script do time. Tudo
// opcional e números como float64 pra tolerar decimais no JSON.
type gatewayUsageResponse struct {
	Entries []gatewayEntry `json:"entries"`
}

type gatewayEntry struct {
	Scope          string         `json:"scope"`
	CalendarPeriod string         `json:"calendarPeriod"`
	Budget         *gatewayBudget `json:"budget"`
}

type gatewayBudget struct {
	Limit          *gatewayLimit  `json:"limit"`
	EffectiveLimit *gatewayLimit  `json:"effectiveLimit"`
	Spend          *gatewaySpend  `json:"spend"`
	Window         *gatewayWindow `json:"window"`
	Exceeded       bool           `json:"exceeded"`
	Scope          string         `json:"scope"`
	CalendarPeriod string         `json:"calendarPeriod"`
}

type gatewayLimit struct {
	BRLLimitMicro float64 `json:"brlLimitMicro"`
}

type gatewaySpend struct {
	OfferBRLMicro float64 `json:"offerBrlMicro"`
	Tokens        float64 `json:"tokens"`
}

type gatewayWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ParseGatewayUsage converte a resposta crua do gateway. Devolve nil sem
// erro quando a resposta é válida mas não traz entries[0].budget.
func ParseGatewayUsage(raw []byte) (*GatewayUsage, error) {
	var resp gatewayUsageResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse gateway usage: %w", err)
	}
	if len(resp.Entries) == 0 || resp.Entries[0].Budget == nil {
		return nil, nil
	}
	entry := resp.Entries[0]
	budget := entry.Budget
	baseLimit := micro(budget.Limit)
	limit := micro(budget.EffectiveLimit)
	if limit == 0 {
		limit = baseLimit
	}
	usage := &GatewayUsage{
		LimitBRLMicro:     limit,
		BaseLimitBRLMicro: baseLimit,
		Exceeded:          budget.Exceeded,
		Scope:             firstNonEmpty(entry.Scope, budget.Scope),
		CalendarPeriod:    firstNonEmpty(entry.CalendarPeriod, budget.CalendarPeriod),
	}
	if budget.Spend != nil {
		usage.SpentBRLMicro = roundInt(budget.Spend.OfferBRLMicro)
		usage.Tokens = roundInt(budget.Spend.Tokens)
	}
	if budget.Window != nil {
		usage.WindowEnd = parseIsoEpoch(budget.Window.End)
	}
	return usage, nil
}

func micro(l *gatewayLimit) int64 {
	if l == nil {
		return 0
	}
	return roundInt(l.BRLLimitMicro)
}

func roundInt(f float64) int64 { return int64(math.Round(f)) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
