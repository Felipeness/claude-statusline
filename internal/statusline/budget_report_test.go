package statusline

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewBudgetReport(t *testing.T) {
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	res := &GatewayProbeResult{
		Usage: &GatewayUsage{
			SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 600_000_000,
			Tokens: 9_700_000, WindowEnd: end.Unix(), Scope: "user", CalendarPeriod: "monthly",
		},
		Raw:       json.RawMessage(`{"entries":[]}`),
		FetchedAt: end.Add(-24 * time.Hour),
	}
	r := NewBudgetReport(res, 70, 90)
	if r.SpentBRL != 73.53 || r.LimitBRL != 520 || r.BaseLimitBRL != 600 {
		t.Fatalf("amounts %+v", r)
	}
	if r.Pct != 14 || r.Status != "ok" || r.Period != "monthly" || r.WindowEnd != "2026-10-01T00:00:00Z" {
		t.Fatalf("fields %+v", r)
	}
	if string(r.Raw) != `{"entries":[]}` {
		t.Fatalf("raw not preserved: %s", r.Raw)
	}
	out, err := json.Marshal(r)
	if err != nil || !strings.Contains(string(out), `"spent_brl":73.53`) || !strings.Contains(string(out), `"raw":{"entries":[]}`) {
		t.Fatalf("json = %s err=%v", out, err)
	}
}

func TestBudgetReportStatus(t *testing.T) {
	cases := []struct {
		name  string
		usage GatewayUsage
		want  string
	}{
		{"ok", GatewayUsage{SpentBRLMicro: 10, LimitBRLMicro: 100}, "ok"},
		{"warn", GatewayUsage{SpentBRLMicro: 70, LimitBRLMicro: 100}, "warn"},
		{"crit", GatewayUsage{SpentBRLMicro: 95, LimitBRLMicro: 100}, "crit"},
		{"blocked", GatewayUsage{SpentBRLMicro: 100, LimitBRLMicro: 100, Exceeded: true}, "blocked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := tc.usage
			r := NewBudgetReport(&GatewayProbeResult{Usage: &u, FetchedAt: time.Now()}, 70, 90)
			if r.Status != tc.want {
				t.Fatalf("status = %q, want %q", r.Status, tc.want)
			}
		})
	}
}

func TestBudgetReportText(t *testing.T) {
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		usage    GatewayUsage
		stale    bool
		mustHave []string
		mustNot  []string
	}{
		{
			name:     "individual ok",
			usage:    GatewayUsage{SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 520_000_000, Tokens: 9_700_000, WindowEnd: end.Unix(), Scope: "user", CalendarPeriod: "monthly"},
			mustHave: []string{"Budget do LLM Gateway", "R$ 73,53 de R$ 520,00 (14%)", "mensal", "reseta em 01/10/2026", "individual", "9.7M no período", "status    OK"},
			mustNot:  []string{"licença", "dados de"},
		},
		{
			name:     "license pool",
			usage:    GatewayUsage{SpentBRLMicro: 10, LimitBRLMicro: 100_000_000, BaseLimitBRLMicro: 200_000_000, Scope: "license", CalendarPeriod: "weekly"},
			mustHave: []string{"licença (pool compartilhado", "semanal"},
			mustNot:  []string{"teto individual limitado pelo teto da licença"},
		},
		{
			name:     "individual capped limit",
			usage:    GatewayUsage{SpentBRLMicro: 10, LimitBRLMicro: 100_000_000, BaseLimitBRLMicro: 200_000_000, Scope: "user", CalendarPeriod: "weekly"},
			mustHave: []string{"individual", "teto individual limitado pelo teto da licença", "semanal"},
			mustNot:  []string{"licença (pool compartilhado"},
		},
		{
			name:     "blocked",
			usage:    GatewayUsage{SpentBRLMicro: 100, LimitBRLMicro: 100, Exceeded: true, WindowEnd: end.Unix()},
			mustHave: []string{"BLOQUEADO até 01/10/2026"},
		},
		{
			name:     "stale note",
			usage:    GatewayUsage{SpentBRLMicro: 1, LimitBRLMicro: 100},
			stale:    true,
			mustHave: []string{"gateway não respondeu agora"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := tc.usage
			text := NewBudgetReport(&GatewayProbeResult{Usage: &u, FetchedAt: time.Now(), Stale: tc.stale}, 70, 90).Text()
			for _, s := range tc.mustHave {
				if !strings.Contains(text, s) {
					t.Fatalf("text missing %q:\n%s", s, text)
				}
			}
			for _, s := range tc.mustNot {
				if strings.Contains(text, s) {
					t.Fatalf("text must not contain %q:\n%s", s, text)
				}
			}
		})
	}
}

func TestBudgetErrorMessage(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrGatewayNotConfigured, "ANTHROPIC_BASE_URL"},
		{ErrGatewayTokenMissing, "login"},
		{ErrGatewayTokenExpired, "renovar"},
		{errors.Join(ErrGatewayUnreachable, errors.New("HTTP 502")), "inacessível"},
		{ErrGatewayNoBudget, "sem budget"},
		{ErrGatewayDisabled, "desligado"},
		{ErrGatewayUnauthorized, "refazer o login"},
		{errors.New("weird"), "weird"},
	}
	for _, tc := range cases {
		if got := BudgetErrorMessage(tc.err); !strings.Contains(got, tc.want) {
			t.Fatalf("BudgetErrorMessage(%v) = %q, want containing %q", tc.err, got, tc.want)
		}
	}
}

func TestNewBudgetReportCacheWriteError(t *testing.T) {
	u := GatewayUsage{SpentBRLMicro: 10, LimitBRLMicro: 100}

	t.Run("set when probe failed to write cache", func(t *testing.T) {
		res := &GatewayProbeResult{Usage: &u, FetchedAt: time.Now(), CacheWriteError: errors.New("disk full")}
		r := NewBudgetReport(res, 70, 90)
		if r.CacheWriteError != "disk full" {
			t.Fatalf("CacheWriteError = %q, want %q", r.CacheWriteError, "disk full")
		}
		out, err := json.Marshal(r)
		if err != nil || !strings.Contains(string(out), `"cache_write_error":"disk full"`) {
			t.Fatalf("json = %s err=%v", out, err)
		}
	})

	t.Run("absent from json when nil", func(t *testing.T) {
		res := &GatewayProbeResult{Usage: &u, FetchedAt: time.Now()}
		r := NewBudgetReport(res, 70, 90)
		out, err := json.Marshal(r)
		if err != nil || strings.Contains(string(out), "cache_write_error") {
			t.Fatalf("json = %s err=%v, want no cache_write_error field", out, err)
		}
	})
}
