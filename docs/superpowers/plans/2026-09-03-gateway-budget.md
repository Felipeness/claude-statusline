# Gateway Budget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Absorver em Go as features do statusline bash do Marcelo (budget do LLM Gateway da Superlógica, tokens da sessão, `/budget`) e deixar o `claude-statusline` distribuível pro time via GitHub Releases.

**Architecture:** Um probe novo (`gateway_probe.go`) busca `/v1/usage` com o JWT do Auth0 e cacheia em disco; um tipo puro `GatewayUsage` entra no `Input` e alimenta 3 components novos; 4 components de tokens leem `context_window.current_usage`. O subcomando `budget` reusa o mesmo probe e um report puro. O Studio só ganha campos de mock e badge. Release por workflow que cross-compila 5 alvos.

**Tech Stack:** Go 1.26 (stdlib + BurntSushi/toml + x/term), Vite + React 19 + Tailwind v4 (Studio, build com Bun), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-03-gateway-budget-design.md`

## Global Constraints

- Zero dependências em runtime: nada de `exec.Command` novo (constitution I). Só `git` já existente.
- Render nunca escreve em stderr, nunca faz `os.Exit`, toda chamada HTTP com `context.WithTimeout` (constitution III).
- Token do Auth0 nunca vai pro cache em disco, log ou stdout (constitution IV).
- Todo component novo: `Register` no `init()`, entry em `componentMetas`, renderiza só a partir do `RenderCtx` (constitution V).
- Testes: `go test`, table-driven, nomes em 3ª pessoa sem "should" (ex: `TestFormatBRLMicro/rounds_half_up`), arquivo `_test.go` ao lado do source.
- Commits: conventional commits sem ticket (`feat: ...`, `test: ...`, `ci: ...`, `docs: ...`), descrição em minúscula, primeira linha ≤ 72 chars. Commit ao fim de cada task.
- Comentários e strings de UI em PT-BR, como o resto do repo. Nomes de código em inglês.
- Timeouts: gateway 4s, TTL 60s, stale 1h. Thresholds default do budget: warn 70, crit 90.
- Rodar `gofmt -l .` antes de cada commit (deve imprimir nada).
- Branch de trabalho: `feat/gateway-budget` (já criada).

---

## File Structure

| Arquivo | Responsabilidade |
|---|---|
| `internal/statusline/format.go` (novo) | `FormatBRLMicro`, `FormatTokens` (puros) |
| `internal/statusline/gateway.go` (novo) | `GatewayUsage` + `ParseGatewayUsage` (puros, sem IO) |
| `internal/statusline/gateway_probe.go` (novo) | `GatewayConfig`, erros tipados, token file, cache em disco, HTTP |
| `internal/statusline/components_gateway.go` (novo) | `gateway_budget`, `gateway_tokens`, `gateway_reset` |
| `internal/statusline/components_tokens.go` (novo) | `tokens_in`, `tokens_out`, `tokens_total`, `tokens_cache` |
| `internal/statusline/budget_report.go` (novo) | `BudgetReport` (JSON + texto PT-BR) e mensagens de erro |
| `internal/statusline/budget_command.go` (novo) | escreve/remove `~/.claude/commands/budget.md` |
| `internal/statusline/input.go` | campo `Gateway *GatewayUsage` |
| `internal/statusline/components.go` | metas novas, `NeedsGateway`, `auth_mode` com `gateway`, `init()` |
| `internal/statusline/config.go` | `Config.Gateway`, defaults, merge |
| `internal/statusline/presets.go` | preset `gateway` |
| `internal/statusline/theme.go` | cores graphite pros components novos |
| `internal/statusline/oauth_probe.go` | `parseIsoEpoch` aceita só data |
| `internal/server/server.go` | mock default com gateway e current_usage |
| `main.go` | `version`, `budget`, install do `/budget`, `detectAuthMode` com gateway, mock do preview |
| `web/src/types.ts`, `web/src/App.tsx` | mock, badge, campos |
| `.github/workflows/ci.yml`, `.github/workflows/release.yml` | CI e release |
| `install.sh`, `install.ps1` | bootstrap pro time |
| `README.md`, `README.en.md`, `docs/confluence-instalacao-time.md` | docs |

---

### Task 1: Formatadores BRL e tokens

**Files:**
- Create: `internal/statusline/format.go`
- Test: `internal/statusline/format_test.go`

**Interfaces:**
- Produces: `func FormatBRLMicro(micro int64) string` → `"R$ 1.520,00"`; `func FormatTokens(n int64) string` → `"999"`, `"38.6k"`, `"9.7M"`.

- [ ] **Step 1: Write the failing tests**

```go
package statusline

import "testing"

func TestFormatBRLMicro(t *testing.T) {
	cases := []struct {
		name  string
		micro int64
		want  string
	}{
		{"zero", 0, "R$ 0,00"},
		{"cents", 73_530_000, "R$ 73,53"},
		{"thousands separator", 1_520_000_000, "R$ 1.520,00"},
		{"millions", 1_234_567_890_000, "R$ 1.234.567,89"},
		{"rounds half up", 999_999, "R$ 1,00"},
		{"rounds down", 4_999, "R$ 0,00"},
		{"negative", -73_530_000, "-R$ 73,53"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatBRLMicro(tc.micro); got != tc.want {
				t.Fatalf("FormatBRLMicro(%d) = %q, want %q", tc.micro, got, tc.want)
			}
		})
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		name string
		n    int64
		want string
	}{
		{"units", 999, "999"},
		{"exact thousand", 1_000, "1.0k"},
		{"thousands", 38_630, "38.6k"},
		{"millions", 9_700_000, "9.7M"},
		{"zero", 0, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatTokens(tc.n); got != tc.want {
				t.Fatalf("FormatTokens(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestFormat' -v`
Expected: FAIL com `undefined: FormatBRLMicro`

- [ ] **Step 3: Write the implementation**

```go
package statusline

import (
	"fmt"
	"strings"
)

// FormatBRLMicro converte micro-reais (1 real = 1_000_000) em "R$ 1.520,00",
// formato pt-BR, arredondando half-up nos centavos.
func FormatBRLMicro(micro int64) string {
	negative := micro < 0
	if negative {
		micro = -micro
	}
	cents := (micro + 5_000) / 10_000
	reais, rest := cents/100, cents%100
	formatted := fmt.Sprintf("R$ %s,%02d", groupThousands(reais), rest)
	if negative {
		return "-" + formatted
	}
	return formatted
}

func groupThousands(n int64) string {
	digits := fmt.Sprintf("%d", n)
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// FormatTokens abrevia contagem de tokens: 999, 38.6k, 9.7M (1 casa decimal,
// mesmo formato que o time já vê no script do gateway).
func FormatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/statusline/ -run 'TestFormat' -v`
Expected: PASS (12 subtests)

- [ ] **Step 5: Commit**

```bash
gofmt -l . && git add internal/statusline/format.go internal/statusline/format_test.go && git commit -m "feat: add brl and token formatters"
```

---

### Task 2: Tipo `GatewayUsage` e parse do `/v1/usage`

**Files:**
- Create: `internal/statusline/gateway.go`
- Create: `internal/statusline/testdata/gateway_usage.json`
- Modify: `internal/statusline/input.go` (campo `Gateway`)
- Modify: `internal/statusline/oauth_probe.go:263-281` (`parseIsoEpoch` aceita `2006-01-02`)
- Test: `internal/statusline/gateway_test.go`

**Interfaces:**
- Consumes: `parseIsoEpoch(string) int64` (existente em `oauth_probe.go`).
- Produces:
  ```go
  type GatewayUsage struct {
      SpentBRLMicro, LimitBRLMicro, BaseLimitBRLMicro, Tokens, WindowEnd int64
      Exceeded bool
      Scope, CalendarPeriod string
  }
  func (g *GatewayUsage) Pct() float64        // 0 quando limite <= 0 ou g nil
  func (g *GatewayUsage) ResetDate() string   // "dd/mm" em UTC, "" quando WindowEnd == 0
  func ParseGatewayUsage(raw []byte) (*GatewayUsage, error) // nil, nil quando não há entries[0].budget
  ```
  `Input.Gateway *GatewayUsage` com tag `json:"gateway,omitempty"`.

- [ ] **Step 1: Write the fixture**

`internal/statusline/testdata/gateway_usage.json`:

```json
{
  "entries": [
    {
      "scope": "user",
      "calendarPeriod": "monthly",
      "budget": {
        "limit": { "brlLimitMicro": 600000000 },
        "effectiveLimit": { "brlLimitMicro": 520000000 },
        "spend": { "offerBrlMicro": 73530000, "tokens": 9700000 },
        "window": { "start": "2026-09-01T00:00:00Z", "end": "2026-10-01T00:00:00Z" },
        "exceeded": false
      }
    }
  ]
}
```

- [ ] **Step 2: Write the failing tests**

```go
package statusline

import (
	"os"
	"testing"
	"time"
)

func TestParseGatewayUsage(t *testing.T) {
	fixture, err := os.ReadFile("testdata/gateway_usage.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		raw     string
		want    *GatewayUsage
		wantErr bool
	}{
		{
			name: "full fixture",
			raw:  string(fixture),
			want: &GatewayUsage{
				SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 600_000_000,
				Tokens: 9_700_000, WindowEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix(),
				Scope: "user", CalendarPeriod: "monthly",
			},
		},
		{name: "empty entries", raw: `{"entries":[]}`, want: nil},
		{name: "entry without budget", raw: `{"entries":[{"scope":"user"}]}`, want: nil},
		{
			name: "falls back to limit when effectiveLimit missing",
			raw:  `{"entries":[{"budget":{"limit":{"brlLimitMicro":100000000},"spend":{"offerBrlMicro":5000000}}}]}`,
			want: &GatewayUsage{SpentBRLMicro: 5_000_000, LimitBRLMicro: 100_000_000, BaseLimitBRLMicro: 100_000_000},
		},
		{
			name: "date-only window end and scope inside budget",
			raw:  `{"entries":[{"budget":{"scope":"license","calendarPeriod":"weekly","window":{"end":"2026-09-08"},"exceeded":true}}]}`,
			want: &GatewayUsage{WindowEnd: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC).Unix(), Exceeded: true, Scope: "license", CalendarPeriod: "weekly"},
		},
		{
			name: "float micro values are rounded",
			raw:  `{"entries":[{"budget":{"limit":{"brlLimitMicro":100.6},"spend":{"offerBrlMicro":2.4,"tokens":10.5}}}]}`,
			want: &GatewayUsage{SpentBRLMicro: 2, LimitBRLMicro: 101, BaseLimitBRLMicro: 101, Tokens: 11},
		},
		{name: "invalid json", raw: `{`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseGatewayUsage([]byte(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGatewayUsagePct(t *testing.T) {
	cases := []struct {
		name string
		g    *GatewayUsage
		want float64
	}{
		{"nil receiver", nil, 0},
		{"zero limit", &GatewayUsage{SpentBRLMicro: 10}, 0},
		{"fourteen percent", &GatewayUsage{SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000}, 14.140384615384615},
		{"over limit", &GatewayUsage{SpentBRLMicro: 600, LimitBRLMicro: 500}, 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.g.Pct(); got != tc.want {
				t.Fatalf("Pct() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGatewayUsageResetDate(t *testing.T) {
	g := &GatewayUsage{WindowEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix()}
	if got := g.ResetDate(); got != "01/10" {
		t.Fatalf("ResetDate() = %q, want 01/10", got)
	}
	if got := (&GatewayUsage{}).ResetDate(); got != "" {
		t.Fatalf("ResetDate() on zero = %q, want empty", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestParseGatewayUsage|TestGatewayUsage' -v`
Expected: FAIL com `undefined: ParseGatewayUsage`

- [ ] **Step 4: Write `gateway.go`**

```go
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
```

- [ ] **Step 5: Add `Gateway` to `Input` and date-only format to `parseIsoEpoch`**

Em `internal/statusline/input.go`, logo depois de `AuthMode string \`json:"-"\``:

```go
	// Gateway é o consumo no LLM Gateway da empresa. Preenchido pelo probe
	// no render (nunca vem do stdin do Claude Code) ou pelo mock do Studio.
	Gateway *GatewayUsage `json:"gateway,omitempty"`
```

Em `internal/statusline/oauth_probe.go`, na lista `formats` de `parseIsoEpoch`, adicionar `"2006-01-02"` como último item.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/statusline/ -run 'TestParseGatewayUsage|TestGatewayUsage' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
gofmt -l . && git add internal/statusline/gateway.go internal/statusline/gateway_test.go internal/statusline/testdata/gateway_usage.json internal/statusline/input.go internal/statusline/oauth_probe.go && git commit -m "feat: parse gateway usage payload"
```

---

### Task 3: Probe do gateway com cache em disco

**Files:**
- Create: `internal/statusline/gateway_probe.go`
- Modify: `internal/statusline/config.go` (campo `Gateway`, defaults, merge)
- Test: `internal/statusline/gateway_probe_test.go`, `internal/statusline/config_test.go`

**Interfaces:**
- Consumes: `ParseGatewayUsage`, `GatewayUsage` (Task 2), `parseProbeDur` (existente).
- Produces:
  ```go
  type GatewayConfig struct {
      Enabled *bool; BaseURL, TokenFile, CacheFile, TTL, StaleTTL, Timeout string
  }
  func (c GatewayConfig) IsEnabled() bool
  func (c GatewayConfig) ResolvedBaseURL() string   // config, senão env ANTHROPIC_BASE_URL, sem "/" final
  func (c GatewayConfig) ResolvedTokenFile() string // config, senão ~/.claude/auth0-token-cache.json
  func GatewayConfigured(cfg GatewayConfig) bool    // habilitado + base URL + token file existe
  type GatewayProbeResult struct { Usage *GatewayUsage; Raw json.RawMessage; FetchedAt time.Time; Stale bool }
  var ErrGatewayDisabled, ErrGatewayNotConfigured, ErrGatewayTokenMissing, ErrGatewayTokenExpired, ErrGatewayUnreachable, ErrGatewayNoBudget error
  func ProbeGatewayDetailed(cfg GatewayConfig) (*GatewayProbeResult, error)
  func ProbeGateway(cfg GatewayConfig) *GatewayProbeResult // fail-open: nil em qualquer erro
  var Version = "dev" // setado pelo main; vai no User-Agent
  ```
  `Config.Gateway GatewayConfig` com defaults `TTL "60s"`, `StaleTTL "1h"`, `Timeout "4s"`.

- [ ] **Step 1: Write the failing probe tests**

```go
package statusline

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func writeTokenFile(t *testing.T, dir string, expiresAt int64) string {
	t.Helper()
	path := filepath.Join(dir, "auth0-token-cache.json")
	body := map[string]any{"access_token": "jwt-test", "expires_at": expiresAt}
	data, _ := json.Marshal(body)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func gatewayServer(t *testing.T, status int, body string, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.URL.Path != "/v1/usage" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-test" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestProbeGatewayDetailed(t *testing.T) {
	fixture, err := os.ReadFile("testdata/gateway_usage.json")
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()

	t.Run("fetches once and serves fresh cache without http", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{
			BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future),
			CacheFile: filepath.Join(dir, "cache.json"), TTL: "60s", StaleTTL: "1h", Timeout: "2s",
		}
		first, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if first.Usage == nil || first.Usage.SpentBRLMicro != 73_530_000 || first.Stale {
			t.Fatalf("unexpected first result %+v", first)
		}
		second, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if second.Usage.SpentBRLMicro != 73_530_000 || second.Stale {
			t.Fatalf("unexpected cached result %+v", second)
		}
		if hits != 1 {
			t.Fatalf("server hits = %d, want 1", hits)
		}
		cached, _ := os.ReadFile(cfg.CacheFile)
		if string(cached) == "" || contains(cached, "jwt-test") {
			t.Fatalf("cache must exist and never contain the token: %s", cached)
		}
	})

	t.Run("expired token never calls the gateway", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, past), CacheFile: filepath.Join(dir, "cache.json")}
		_, err := ProbeGatewayDetailed(cfg)
		if !errors.Is(err, ErrGatewayTokenExpired) {
			t.Fatalf("err = %v, want ErrGatewayTokenExpired", err)
		}
		if hits != 0 {
			t.Fatalf("server hits = %d, want 0", hits)
		}
	})

	t.Run("missing token file", func(t *testing.T) {
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: "http://127.0.0.1:1", TokenFile: filepath.Join(dir, "nope.json"), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayTokenMissing) {
			t.Fatalf("err = %v, want ErrGatewayTokenMissing", err)
		}
	})

	t.Run("not configured without base url", func(t *testing.T) {
		t.Setenv("ANTHROPIC_BASE_URL", "")
		cfg := GatewayConfig{TokenFile: writeTokenFile(t, t.TempDir(), future)}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayNotConfigured) {
			t.Fatalf("err = %v, want ErrGatewayNotConfigured", err)
		}
	})

	t.Run("base url falls back to env", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL+"/")
		dir := t.TempDir()
		cfg := GatewayConfig{TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil || res.Usage == nil {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("server error serves stale cache", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 500, `boom`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "cache.json")
		stale := gatewayDiskCache{FetchedAt: time.Now().Add(-5 * time.Minute).Unix(), Usage: &GatewayUsage{SpentBRLMicro: 42}}
		data, _ := json.Marshal(stale)
		_ = os.WriteFile(cacheFile, data, 0600)
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: cacheFile, TTL: "60s", StaleTTL: "1h"}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.Usage.SpentBRLMicro != 42 || !res.Stale {
			t.Fatalf("expected stale cache, got %+v", res)
		}
	})

	t.Run("server error without cache is unreachable", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 500, `boom`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayUnreachable) {
			t.Fatalf("err = %v, want ErrGatewayUnreachable", err)
		}
	})

	t.Run("response without budget", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, `{"entries":[]}`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayNoBudget) {
			t.Fatalf("err = %v, want ErrGatewayNoBudget", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		off := false
		if _, err := ProbeGatewayDetailed(GatewayConfig{Enabled: &off}); !errors.Is(err, ErrGatewayDisabled) {
			t.Fatalf("err = %v, want ErrGatewayDisabled", err)
		}
	})

	t.Run("fail-open wrapper returns nil on error", func(t *testing.T) {
		off := false
		if got := ProbeGateway(GatewayConfig{Enabled: &off}); got != nil {
			t.Fatalf("ProbeGateway = %+v, want nil", got)
		}
	})
}

func TestGatewayConfigured(t *testing.T) {
	dir := t.TempDir()
	token := writeTokenFile(t, dir, time.Now().Add(time.Hour).Unix())
	off := false
	cases := []struct {
		name string
		cfg  GatewayConfig
		env  string
		want bool
	}{
		{"url and token", GatewayConfig{BaseURL: "http://gw", TokenFile: token}, "", true},
		{"url from env", GatewayConfig{TokenFile: token}, "http://gw", true},
		{"no url", GatewayConfig{TokenFile: token}, "", false},
		{"no token file", GatewayConfig{BaseURL: "http://gw", TokenFile: filepath.Join(dir, "missing.json")}, "", false},
		{"disabled", GatewayConfig{Enabled: &off, BaseURL: "http://gw", TokenFile: token}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_BASE_URL", tc.env)
			if got := GatewayConfigured(tc.cfg); got != tc.want {
				t.Fatalf("GatewayConfigured = %v, want %v", got, tc.want)
			}
		})
	}
}

func contains(data []byte, needle string) bool {
	return len(needle) > 0 && string(data) != "" && indexOf(string(data), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Write the failing config test**

`internal/statusline/config_test.go`:

```go
package statusline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMergesGatewaySection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	toml := "[gateway]\nenabled = false\nbase_url = \"https://gw.example/llm-gtw\"\nttl = \"30s\"\n"
	if err := os.WriteFile(path, []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.IsEnabled() {
		t.Fatal("expected gateway disabled")
	}
	if cfg.Gateway.BaseURL != "https://gw.example/llm-gtw" || cfg.Gateway.TTL != "30s" {
		t.Fatalf("unexpected gateway cfg %+v", cfg.Gateway)
	}
	if cfg.Gateway.StaleTTL != "1h" || cfg.Gateway.Timeout != "4s" {
		t.Fatalf("defaults not preserved: %+v", cfg.Gateway)
	}
	if cfg.Components["gateway_budget"].WarnAt != 70 || cfg.Components["gateway_budget"].CriticalAt != 90 {
		t.Fatalf("gateway_budget thresholds = %+v", cfg.Components["gateway_budget"])
	}
}

func TestDefaultConfigGatewayEnabled(t *testing.T) {
	if !DefaultConfig().Gateway.IsEnabled() {
		t.Fatal("gateway must be enabled by default")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestProbeGateway|TestGatewayConfigured|TestLoadConfigMergesGateway|TestDefaultConfigGateway' -v`
Expected: FAIL com `undefined: GatewayConfig`

- [ ] **Step 4: Write `gateway_probe.go`**

```go
package statusline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version é injetada pelo main (ldflags). Vai no User-Agent dos probes.
var Version = "dev"

// GatewayConfig controla o probe do LLM Gateway da empresa (Kong com budget
// por usuário). Habilitado por padrão; só age quando há base URL (config ou
// env ANTHROPIC_BASE_URL) e o cache de token do Auth0 existe.
type GatewayConfig struct {
	Enabled   *bool  `toml:"enabled" json:"enabled,omitempty"`
	BaseURL   string `toml:"base_url" json:"base_url"`     // vazio = env ANTHROPIC_BASE_URL
	TokenFile string `toml:"token_file" json:"token_file"` // vazio = ~/.claude/auth0-token-cache.json
	CacheFile string `toml:"cache_file" json:"cache_file"` // vazio = ~/.cache/claude-statusline-gateway.json
	TTL       string `toml:"ttl" json:"ttl"`               // default 60s
	StaleTTL  string `toml:"stale_ttl" json:"stale_ttl"`   // default 1h
	Timeout   string `toml:"timeout" json:"timeout"`       // default 4s
}

func (c GatewayConfig) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

func (c GatewayConfig) ResolvedBaseURL() string {
	base := c.BaseURL
	if base == "" {
		base = os.Getenv("ANTHROPIC_BASE_URL")
	}
	return strings.TrimRight(base, "/")
}

func (c GatewayConfig) ResolvedTokenFile() string {
	if c.TokenFile != "" {
		return c.TokenFile
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "auth0-token-cache.json")
}

func (c GatewayConfig) resolvedCacheFile() string {
	if c.CacheFile != "" {
		return c.CacheFile
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cache")
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "claude-statusline-gateway.json")
}

func (c GatewayConfig) ttlDuration() time.Duration      { return parseProbeDur(c.TTL, 60*time.Second) }
func (c GatewayConfig) staleDuration() time.Duration    { return parseProbeDur(c.StaleTTL, time.Hour) }
func (c GatewayConfig) timeoutDuration() time.Duration  { return parseProbeDur(c.Timeout, 4*time.Second) }

// GatewayConfigured diz se a sessão parece rodar via gateway: probe ligado,
// base URL conhecida e cache de token do Auth0 presente em disco.
func GatewayConfigured(cfg GatewayConfig) bool {
	if !cfg.IsEnabled() || cfg.ResolvedBaseURL() == "" {
		return false
	}
	_, err := os.Stat(cfg.ResolvedTokenFile())
	return err == nil
}

// Erros tipados pro subcomando budget explicar o que falta. O render usa
// ProbeGateway, que engole tudo (fail-open).
var (
	ErrGatewayDisabled      = errors.New("probe do gateway desligado no config")
	ErrGatewayNotConfigured = errors.New("gateway não configurado: ANTHROPIC_BASE_URL ausente")
	ErrGatewayTokenMissing  = errors.New("token do Auth0 ausente: abra o Claude Code pra fazer login no gateway")
	ErrGatewayTokenExpired  = errors.New("token do Auth0 expirado: abra o Claude Code pra renovar")
	ErrGatewayUnreachable   = errors.New("gateway inacessível")
	ErrGatewayNoBudget      = errors.New("gateway respondeu sem budget pro usuário")
)

// GatewayProbeResult é o que o probe devolve: dados parseados, resposta crua
// (pro budget --json) e se veio de cache stale.
type GatewayProbeResult struct {
	Usage     *GatewayUsage
	Raw       json.RawMessage
	FetchedAt time.Time
	Stale     bool
}

// gatewayDiskCache é o único cache (cada render é um processo novo). Nunca
// guarda o token, só dados de consumo.
type gatewayDiskCache struct {
	FetchedAt int64           `json:"fetched_at"`
	Usage     *GatewayUsage   `json:"usage"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

// ProbeGateway é a face fail-open: nil em qualquer erro, pro render.
func ProbeGateway(cfg GatewayConfig) *GatewayProbeResult {
	res, err := ProbeGatewayDetailed(cfg)
	if err != nil {
		return nil
	}
	return res
}

// ProbeGatewayDetailed busca /v1/usage com cache em disco:
//  1. cache mais novo que TTL → devolve sem HTTP
//  2. token ausente/expirado → cache stale (< StaleTTL) ou erro tipado
//  3. HTTP falha ou resposta inválida → cache stale ou ErrGatewayUnreachable
//  4. OK → grava cache e devolve
func ProbeGatewayDetailed(cfg GatewayConfig) (*GatewayProbeResult, error) {
	if !cfg.IsEnabled() {
		return nil, ErrGatewayDisabled
	}
	base := cfg.ResolvedBaseURL()
	if base == "" {
		return nil, ErrGatewayNotConfigured
	}
	cachePath := cfg.resolvedCacheFile()
	if fresh := readGatewayCache(cachePath, cfg.ttlDuration()); fresh != nil {
		return fresh, nil
	}
	token, err := readAuth0Token(cfg.ResolvedTokenFile())
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), err)
	}
	raw, err := fetchGatewayUsage(base, token, cfg.timeoutDuration())
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), fmt.Errorf("%w: %v", ErrGatewayUnreachable, err))
	}
	usage, err := ParseGatewayUsage(raw)
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), fmt.Errorf("%w: %v", ErrGatewayUnreachable, err))
	}
	if usage == nil {
		return nil, ErrGatewayNoBudget
	}
	res := &GatewayProbeResult{Usage: usage, Raw: raw, FetchedAt: time.Now()}
	writeGatewayCache(cachePath, res)
	return res, nil
}

func staleOr(cachePath string, staleTTL time.Duration, err error) (*GatewayProbeResult, error) {
	if stale := readGatewayCache(cachePath, staleTTL); stale != nil {
		stale.Stale = true
		return stale, nil
	}
	return nil, err
}

type auth0TokenCache struct {
	AccessToken string  `json:"access_token"`
	ExpiresAt   float64 `json:"expires_at"` // epoch segundos (ms tolerado)
}

func readAuth0Token(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ErrGatewayTokenMissing
	}
	var cache auth0TokenCache
	if err := json.Unmarshal(data, &cache); err != nil || cache.AccessToken == "" {
		return "", ErrGatewayTokenMissing
	}
	expires := cache.ExpiresAt
	if expires > 1e12 { // veio em milissegundos
		expires /= 1000
	}
	if expires > 0 && float64(time.Now().Unix()) >= expires {
		return "", ErrGatewayTokenExpired
	}
	return cache.AccessToken, nil
}

func fetchGatewayUsage(base, token string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "claude-statusline/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func readGatewayCache(path string, maxAge time.Duration) *GatewayProbeResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entry gatewayDiskCache
	if err := json.Unmarshal(data, &entry); err != nil || entry.Usage == nil {
		return nil
	}
	fetched := time.Unix(entry.FetchedAt, 0)
	if time.Since(fetched) > maxAge {
		return nil
	}
	return &GatewayProbeResult{Usage: entry.Usage, Raw: entry.Raw, FetchedAt: fetched}
}

func writeGatewayCache(path string, res *GatewayProbeResult) {
	data, err := json.Marshal(gatewayDiskCache{FetchedAt: res.FetchedAt.Unix(), Usage: res.Usage, Raw: res.Raw})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
```

- [ ] **Step 5: Wire `Config.Gateway`**

Em `internal/statusline/config.go`:

1. Na struct `Config`, depois de `OAuthProbe`:
   ```go
   	Gateway    GatewayConfig            `toml:"gateway" json:"gateway"`
   ```
2. Em `DefaultConfig()`, no mapa `Components`, adicionar `"gateway_budget": {WarnAt: 70, CriticalAt: 90},` e, depois de `OAuthProbe: ...{...},`:
   ```go
   		Gateway: GatewayConfig{
   			TTL:      "60s",
   			StaleTTL: "1h",
   			Timeout:  "4s",
   		},
   ```
3. Em `mergeConfig`, depois de `mergeOAuthProbe(...)`: `mergeGateway(&cfg.Gateway, &user.Gateway)`.
4. Função nova:
   ```go
   func mergeGateway(cfg, user *GatewayConfig) {
   	if user.Enabled != nil {
   		cfg.Enabled = user.Enabled
   	}
   	for _, pair := range []struct{ dst *string; src string }{
   		{&cfg.BaseURL, user.BaseURL}, {&cfg.TokenFile, user.TokenFile}, {&cfg.CacheFile, user.CacheFile},
   		{&cfg.TTL, user.TTL}, {&cfg.StaleTTL, user.StaleTTL}, {&cfg.Timeout, user.Timeout},
   	} {
   		if pair.src != "" {
   			*pair.dst = pair.src
   		}
   	}
   }
   ```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/statusline/ -v`
Expected: PASS (todos, incluindo os das Tasks 1 e 2)

- [ ] **Step 7: Commit**

```bash
gofmt -l . && git add internal/statusline/gateway_probe.go internal/statusline/gateway_probe_test.go internal/statusline/config.go internal/statusline/config_test.go && git commit -m "feat: probe llm gateway usage with disk cache"
```

---

### Task 4: Components `gateway_budget`, `gateway_tokens`, `gateway_reset`

**Files:**
- Create: `internal/statusline/components_gateway.go`
- Modify: `internal/statusline/components.go:36-70` (`ComponentMeta.NeedsGateway`, metas) e `init()` no fim do arquivo
- Modify: `internal/statusline/theme.go:67-92` (graphite `Segs`)
- Test: `internal/statusline/components_gateway_test.go`

**Interfaces:**
- Consumes: `GatewayUsage` (Task 2), `FormatBRLMicro`, `FormatTokens` (Task 1), `Classify`, `Theme.SegOf`, `Theme.SeverityFG`, `Segment`, `Register`.
- Produces: components registrados `gateway_budget`, `gateway_tokens`, `gateway_reset`; `ComponentMeta.NeedsGateway bool \`json:"needs_gateway"\``; helper `severityIcon(Severity) string`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestGateway(Budget|Tokens|Metas)' -v`
Expected: FAIL (panic `nil pointer` em `Get("gateway_budget")` ou `unknown field NeedsGateway`)

- [ ] **Step 3: Write `components_gateway.go`**

```go
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
```

- [ ] **Step 4: Update metas, `ComponentMeta` and graphite theme**

Em `internal/statusline/components.go`:

1. Em `ComponentMeta`, depois de `NeedsHist`:
   ```go
   	NeedsGateway bool `json:"needs_gateway"` // true → só funciona com LLM Gateway configurado
   ```
2. Em `componentMetas`, adicionar:
   ```go
   	"gateway_budget": {Name: "gateway_budget", Label: "Budget gateway", Category: "gateway", Description: "R$ gasto / R$ limite (%) no LLM Gateway, 🚫 quando bloqueado — requer gateway", NeedsGateway: true, HasWarnAt: true},
   	"gateway_tokens": {Name: "gateway_tokens", Label: "Tokens período", Category: "gateway", Description: "Tokens processados no período atual (todas as sessões) — requer gateway", NeedsGateway: true},
   	"gateway_reset":  {Name: "gateway_reset", Label: "Reset budget", Category: "gateway", Description: "Data em que o budget zera (reset dd/mm) — requer gateway", NeedsGateway: true},
   ```
3. Atualizar o comentário de `Category` pra incluir `gateway`.

Em `internal/statusline/theme.go`, no `Segs` do `graphiteTheme()`, adicionar:
```go
			"gateway_budget": {BG: Hex("#4c1d95"), FG: Hex("#ede9fe")},
			"gateway_tokens": {BG: Hex("#134e4a"), FG: Hex("#ccfbf1")},
			"gateway_reset":  {BG: Hex("#1e3a8a"), FG: Hex("#dbeafe")},
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/statusline/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add internal/statusline/components_gateway.go internal/statusline/components_gateway_test.go internal/statusline/components.go internal/statusline/theme.go && git commit -m "feat: add gateway budget, tokens and reset components"
```

---

### Task 5: Components de tokens da sessão

**Files:**
- Create: `internal/statusline/components_tokens.go`
- Modify: `internal/statusline/components.go` (metas)
- Modify: `internal/statusline/theme.go` (graphite `Segs`)
- Test: `internal/statusline/components_tokens_test.go`

**Interfaces:**
- Consumes: `Input.Context.Current.{InputTokens,OutputTokens,CacheReadInputTokens}`, `Input.Context.{TotalInputTokens,TotalOutputTokens}`, `FormatTokens`.
- Produces: components `tokens_in`, `tokens_out`, `tokens_total`, `tokens_cache`; helper `sessionTokens(in *Input) (input, output, cache int64)`.

- [ ] **Step 1: Write the failing tests**

```go
package statusline

import (
	"strings"
	"testing"
	"time"
)

func tokensCtx(in *Input) *RenderCtx {
	return &RenderCtx{In: in, Theme: GetTheme("graphite"), Now: time.Now()}
}

func TestSessionTokenComponents(t *testing.T) {
	withCurrent := &Input{}
	withCurrent.Context.Current.InputTokens = 3
	withCurrent.Context.Current.OutputTokens = 436
	withCurrent.Context.Current.CacheReadInputTokens = 38_630
	withCurrent.Context.TotalInputTokens = 99_999

	totalsOnly := &Input{}
	totalsOnly.Context.TotalInputTokens = 18_432
	totalsOnly.Context.TotalOutputTokens = 4_521

	cases := []struct {
		name string
		comp string
		in   *Input
		want string
	}{
		{"in from current usage", "tokens_in", withCurrent, "In: 3"},
		{"out from current usage", "tokens_out", withCurrent, "Out: 436"},
		{"total sums in and out", "tokens_total", withCurrent, "Total: 439"},
		{"cache formatted", "tokens_cache", withCurrent, "Cache: 38.6k"},
		{"in falls back to totals", "tokens_in", totalsOnly, "In: 18.4k"},
		{"out falls back to totals", "tokens_out", totalsOnly, "Out: 4.5k"},
		{"total from totals", "tokens_total", totalsOnly, "Total: 23.0k"},
		{"cache hidden at zero", "tokens_cache", totalsOnly, ""},
		{"in visible at zero", "tokens_in", &Input{}, "In: 0"},
		{"total visible at zero", "tokens_total", &Input{}, "Total: 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seg := Get(tc.comp).Render(tokensCtx(tc.in), ComponentOpts{})
			if strings.TrimSpace(seg.Text) != tc.want {
				t.Fatalf("%s text = %q, want %q", tc.comp, seg.Text, tc.want)
			}
		})
	}
}

func TestSessionTokenMetas(t *testing.T) {
	for _, name := range []string{"tokens_in", "tokens_out", "tokens_total", "tokens_cache"} {
		meta, ok := componentMetas[name]
		if !ok || meta.Category != "context" {
			t.Fatalf("meta %s = %+v (ok=%v)", name, meta, ok)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestSessionToken' -v`
Expected: FAIL (nil pointer em `Get("tokens_in")`)

- [ ] **Step 3: Write `components_tokens.go`**

```go
package statusline

// sessionTokens lê os contadores da sessão: current_usage quando o Claude
// Code manda (turno atual), senão os totais acumulados.
func sessionTokens(in *Input) (input, output, cache int64) {
	cur := in.Context.Current
	input = int64(cur.InputTokens)
	if input == 0 {
		input = int64(in.Context.TotalInputTokens)
	}
	output = int64(cur.OutputTokens)
	if output == 0 {
		output = int64(in.Context.TotalOutputTokens)
	}
	return input, output, int64(cur.CacheReadInputTokens)
}

// tokenComp é o molde dos 4 chips: label fixo + valor formatado. value
// devolve (n, visible); visible=false esconde o chip.
type tokenComp struct {
	name  string
	label string
	value func(input, output, cache int64) (int64, bool)
}

func (t tokenComp) Name() string { return t.name }
func (t tokenComp) Render(c *RenderCtx, opts ComponentOpts) Segment {
	n, visible := t.value(sessionTokens(c.In))
	if !visible {
		return Segment{}
	}
	seg := c.Theme.SegOf(t.name)
	return Segment{Name: t.name, Text: opts.LabelPrefix + t.label + ": " + FormatTokens(n), FG: seg.FG, BG: seg.BG}
}

func init() {
	Register(tokenComp{name: "tokens_in", label: "In", value: func(in, _, _ int64) (int64, bool) { return in, true }})
	Register(tokenComp{name: "tokens_out", label: "Out", value: func(_, out, _ int64) (int64, bool) { return out, true }})
	Register(tokenComp{name: "tokens_total", label: "Total", value: func(in, out, _ int64) (int64, bool) { return in + out, true }})
	Register(tokenComp{name: "tokens_cache", label: "Cache", value: func(_, _, cache int64) (int64, bool) { return cache, cache > 0 }})
}
```

- [ ] **Step 4: Add metas and graphite colors**

Em `componentMetas` (`components.go`):
```go
	"tokens_in":    {Name: "tokens_in", Label: "Tokens in", Category: "context", Description: "Tokens de entrada da sessão (In: 3)"},
	"tokens_out":   {Name: "tokens_out", Label: "Tokens out", Category: "context", Description: "Tokens de saída da sessão (Out: 436)"},
	"tokens_total": {Name: "tokens_total", Label: "Tokens total", Category: "context", Description: "In + Out da sessão (Total: 439)"},
	"tokens_cache": {Name: "tokens_cache", Label: "Tokens cache", Category: "context", Description: "Tokens lidos do prompt cache (Cache: 38.6k), some quando 0"},
```

Em `graphiteTheme()` `Segs`:
```go
			"tokens_in":    {BG: Hex("#064e3b"), FG: Hex("#d1fae5")},
			"tokens_out":   {BG: Hex("#78350f"), FG: Hex("#fef3c7")},
			"tokens_total": {BG: Hex("#1e293b"), FG: Hex("#e2e8f0")},
			"tokens_cache": {BG: Hex("#312e81"), FG: Hex("#e0e7ff")},
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/statusline/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add internal/statusline/components_tokens.go internal/statusline/components_tokens_test.go internal/statusline/components.go internal/statusline/theme.go && git commit -m "feat: add session token components"
```

---

### Task 6: `auth_mode` reconhece gateway e o render preenche `Input.Gateway`

**Files:**
- Modify: `internal/statusline/components.go` (`authModeComp.Render`)
- Modify: `main.go:66-90` (`cmdRender`) e `main.go:249-281` (`detectAuthMode`)
- Test: `internal/statusline/components_authmode_test.go`, `main_test.go`

**Interfaces:**
- Consumes: `ProbeGateway`, `GatewayConfigured` (Task 3).
- Produces: `func detectAuthMode(stdinHadRateLimits bool, probe *statusline.ProbeResult, gateway bool) string` devolvendo `"gateway" | "oauth" | "api_key"`; `Input.AuthMode == "gateway"` renderiza `[Gateway]`.

- [ ] **Step 1: Write the failing tests**

`internal/statusline/components_authmode_test.go`:

```go
package statusline

import (
	"testing"
	"time"
)

func TestAuthModeRendersGateway(t *testing.T) {
	theme := GetTheme("graphite")
	cases := []struct {
		name   string
		mode   string
		want   string
		wantFG Color
	}{
		{"gateway", "gateway", "[Gateway]", theme.SegOf("auth_mode").FG},
		{"oauth", "oauth", "[OAuth]", theme.SegOf("auth_mode").FG},
		{"api key", "api_key", "[API key]", theme.Status.Warn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &RenderCtx{In: &Input{AuthMode: tc.mode}, Theme: theme, Now: time.Now()}
			seg := Get("auth_mode").Render(ctx, ComponentOpts{})
			if seg.Text != tc.want || seg.FG != tc.wantFG || !seg.Bold {
				t.Fatalf("got %+v, want text %q fg %+v bold", seg, tc.want, tc.wantFG)
			}
		})
	}
}
```

`main_test.go` (package `main`):

```go
package main

import (
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestAuthModeRendersGateway|TestDetectAuthMode' -v`
Expected: FAIL (`[Gateway]` não renderiza; `too many arguments` em `detectAuthMode`)

- [ ] **Step 3: Update `authModeComp.Render`**

Em `components.go`, substituir o bloco que começa em `text := "[OAuth]"` até o `return` por:

```go
	text, sev := "[OAuth]", SevOK
	switch mode {
	case "gateway":
		text = "[Gateway]"
	case "api_key":
		text, sev = "[API key]", SevWarn
	}
	seg := c.Theme.SegOf("auth_mode")
	fg := seg.FG
	if sev != SevOK {
		fg = c.Theme.SeverityFG(sev)
	}
	return Segment{Name: "auth_mode", Text: text, FG: fg, BG: seg.BG, Bold: true}
```

Atualizar o comentário do component: `// auth_mode — chip [Gateway] (LLM Gateway da empresa), [OAuth] ou [API key] (amarelo).`

- [ ] **Step 4: Update `main.go`**

`cmdRender` vira:

```go
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
```

`detectAuthMode` ganha o parâmetro e o item 0 na hierarquia do comentário (`0. Sessão via LLM Gateway da empresa (base URL + token do Auth0, ou probe do gateway respondeu) → gateway. Ganha de tudo: nesse modo o Claude Code não manda rate_limits e não há ANTHROPIC_API_KEY.`):

```go
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
```

Também atualizar a descrição de `auth_mode` em `componentMetas`: `"Chip [Gateway]/[OAuth]/[API key] indicando a auth ativa da sessão"`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add main.go main_test.go internal/statusline/components.go internal/statusline/components_authmode_test.go && git commit -m "feat: detect gateway auth mode and feed gateway usage into render"
```

---

### Task 7: Preset `gateway` e mocks com gateway

**Files:**
- Modify: `internal/statusline/presets.go`
- Modify: `main.go:288-316` (`mockInput`)
- Modify: `internal/server/server.go:180-208` (`defaultMockInput`)
- Test: `internal/statusline/presets_test.go`

**Interfaces:**
- Produces: `Presets["gateway"]`, `PresetNames = compact, max, powerline, gateway`; mocks com `Gateway` e `Context.Current` preenchidos.

- [ ] **Step 1: Write the failing test**

```go
package statusline

import "testing"

func TestGatewayPreset(t *testing.T) {
	cfg := Presets["gateway"]
	if cfg == nil {
		t.Fatal("preset gateway missing")
	}
	if len(cfg.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(cfg.Lines))
	}
	for _, name := range append(cfg.Lines[0].Components, cfg.Lines[1].Components...) {
		if Get(name) == nil {
			t.Fatalf("preset references unknown component %q", name)
		}
	}
	if cfg.Style != "plain" {
		t.Fatalf("style = %q, want plain", cfg.Style)
	}
	if PresetNames[len(PresetNames)-1] != "gateway" {
		t.Fatalf("PresetNames = %v, want gateway last", PresetNames)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/statusline/ -run TestGatewayPreset -v`
Expected: FAIL `preset gateway missing`

- [ ] **Step 3: Add the preset**

Em `presets.go`: adicionar `"gateway": gatewayPreset(),` no mapa, `"gateway"` no fim de `PresetNames`, e:

```go
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
```

- [ ] **Step 4: Enrich both mocks**

Em `main.go` `mockInput()` e em `internal/server/server.go` `defaultMockInput()`, substituir o bloco `Context: statusline.ContextWindow{...}` por:

```go
		Context: func() statusline.ContextWindow {
			cw := statusline.ContextWindow{UsedPercentage: 42, TotalInputTokens: 18432, TotalOutputTokens: 4521}
			cw.Current.InputTokens = 3
			cw.Current.OutputTokens = 436
			cw.Current.CacheReadInputTokens = 38630
			return cw
		}(),
```

e adicionar, depois de `RateLimits: ...`:

```go
		Gateway: &statusline.GatewayUsage{
			SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 520_000_000,
			Tokens: 9_700_000, WindowEnd: 1790812800, Scope: "user", CalendarPeriod: "monthly",
		},
```

(1790812800 = 2026-10-01T00:00:00Z.)

Atualizar o `usage` em `main.go`: `PRESETS: compact (default), max, powerline, gateway`.

- [ ] **Step 5: Run tests and preview**

Run: `go test ./... && go run . preview --theme graphite --style plain`
Expected: PASS; o preview mostra a linha com `🟢 R$ 73,53 / R$ 520,00 (14%)` só se o config local tiver o component (senão só confirma que roda sem erro).

Run: `go run . preview --all | head -5`
Expected: sem erro.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add internal/statusline/presets.go internal/statusline/presets_test.go main.go internal/server/server.go && git commit -m "feat: add gateway preset and gateway mock data"
```

---

### Task 8: Subcomando `budget`

**Files:**
- Create: `internal/statusline/budget_report.go`
- Modify: `main.go` (`usage`, switch, `cmdBudget`)
- Test: `internal/statusline/budget_report_test.go`

**Interfaces:**
- Consumes: `ProbeGatewayDetailed`, `GatewayProbeResult`, erros `ErrGateway*` (Task 3), `FormatBRLMicro`, `FormatTokens`.
- Produces:
  ```go
  type BudgetReport struct {
      SpentBRL, LimitBRL, BaseLimitBRL, Pct float64; Tokens int64
      WindowEnd, Period, Scope, Status, FetchedAt string; Exceeded, Stale bool
      Raw json.RawMessage
  }
  func NewBudgetReport(res *GatewayProbeResult, warn, crit float64) BudgetReport
  func (r BudgetReport) Text() string
  func BudgetErrorMessage(err error) string
  ```

- [ ] **Step 1: Write the failing tests**

```go
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
		name      string
		usage     GatewayUsage
		stale     bool
		mustHave  []string
		mustNot   []string
	}{
		{
			name:     "individual ok",
			usage:    GatewayUsage{SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 520_000_000, Tokens: 9_700_000, WindowEnd: end.Unix(), Scope: "user", CalendarPeriod: "monthly"},
			mustHave: []string{"Budget do LLM Gateway", "R$ 73,53 de R$ 520,00 (14%)", "mensal", "reseta em 01/10/2026", "individual", "9.7M no período", "status    OK"},
			mustNot:  []string{"licença", "dados de"},
		},
		{
			name:     "license pool and capped limit",
			usage:    GatewayUsage{SpentBRLMicro: 10, LimitBRLMicro: 100_000_000, BaseLimitBRLMicro: 200_000_000, Scope: "license", CalendarPeriod: "weekly"},
			mustHave: []string{"licença (pool compartilhado", "teto individual limitado pelo teto da licença", "semanal"},
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
		{errors.New("weird"), "weird"},
	}
	for _, tc := range cases {
		if got := BudgetErrorMessage(tc.err); !strings.Contains(got, tc.want) {
			t.Fatalf("BudgetErrorMessage(%v) = %q, want containing %q", tc.err, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestNewBudgetReport|TestBudgetReport|TestBudgetErrorMessage' -v`
Expected: FAIL `undefined: NewBudgetReport`

- [ ] **Step 3: Write `budget_report.go`**

```go
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
	SpentBRL     float64         `json:"spent_brl"`
	LimitBRL     float64         `json:"limit_brl"`
	BaseLimitBRL float64         `json:"base_limit_brl"`
	Pct          float64         `json:"pct"`
	Tokens       int64           `json:"tokens"`
	WindowEnd    string          `json:"window_end,omitempty"` // RFC3339 UTC
	Period       string          `json:"period,omitempty"`
	Scope        string          `json:"scope,omitempty"`
	Exceeded     bool            `json:"exceeded"`
	Status       string          `json:"status"` // ok | warn | crit | blocked
	FetchedAt    string          `json:"fetched_at"`
	Stale        bool            `json:"stale,omitempty"`
	Raw          json.RawMessage `json:"raw,omitempty"`

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
```

(Os erros tipados já carregam a frase em PT-BR; `BudgetErrorMessage` existe pra manter um único ponto de tradução se as mensagens mudarem.)

- [ ] **Step 4: Wire `cmdBudget` in `main.go`**

No `usage`, depois da linha do `studio`:
```
  claude-statusline budget [--json]            consumo no LLM Gateway da empresa (texto ou JSON pro /budget)
  claude-statusline version                    versão do binário
```

No `switch` de `main()`: `case "budget": cmdBudget(os.Args[2:])`.

Função nova:

```go
// cmdBudget imprime o consumo no gateway. Sempre exit 0: o /budget do
// Claude Code precisa do texto (inclusive do erro) pra explicar ao usuário.
func cmdBudget(args []string) {
	asJSON := len(args) > 0 && args[0] == "--json"
	cfg, err := statusline.LoadConfig(configPath())
	if err != nil {
		printBudgetError(asJSON, err)
		return
	}
	res, err := statusline.ProbeGatewayDetailed(cfg.Gateway)
	if err != nil {
		printBudgetError(asJSON, err)
		return
	}
	opts := cfg.Components["gateway_budget"]
	report := statusline.NewBudgetReport(res, opts.WarnAt, opts.CriticalAt)
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return
	}
	fmt.Print(report.Text())
}

func printBudgetError(asJSON bool, err error) {
	msg := statusline.BudgetErrorMessage(err)
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": msg})
		return
	}
	fmt.Println("Budget do LLM Gateway indisponível:", msg)
}
```

- [ ] **Step 5: Run tests and the command**

Run: `go test ./... && go run . budget && go run . budget --json`
Expected: PASS; sem gateway nesta máquina imprime `Budget do LLM Gateway indisponível: gateway não configurado: ANTHROPIC_BASE_URL ausente` e `{"error":"gateway não configurado: ANTHROPIC_BASE_URL ausente"}`.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add internal/statusline/budget_report.go internal/statusline/budget_report_test.go main.go && git commit -m "feat: add budget subcommand with text and json output"
```

---

### Task 9: `install` grava o `/budget` e usa refresh 60 no preset gateway

**Files:**
- Create: `internal/statusline/budget_command.go`
- Modify: `main.go` (`cmdInstall`)
- Test: `internal/statusline/budget_command_test.go`

**Interfaces:**
- Produces:
  ```go
  const budgetCommandMarker = "gerado por claude-statusline"
  func BudgetCommandContent(selfCmd string) string
  func WriteBudgetCommand(commandsDir, selfCmd string) (written bool, err error) // false = preservou arquivo de terceiros
  func RemoveBudgetCommand(commandsDir string) (removed bool, err error)         // só remove se tiver o marcador
  ```

- [ ] **Step 1: Write the failing tests**

```go
package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBudgetCommand(t *testing.T) {
	t.Run("creates file with marker and self path", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "commands")
		written, err := WriteBudgetCommand(dir, "C:/Users/dev/bin/claude-statusline.exe")
		if err != nil || !written {
			t.Fatalf("written=%v err=%v", written, err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "budget.md"))
		content := string(data)
		for _, s := range []string{"---\ndescription:", "allowed-tools: Bash(C:/Users/dev/bin/claude-statusline.exe budget:*)", budgetCommandMarker, "!`C:/Users/dev/bin/claude-statusline.exe budget --json`", "micro", "license", "exceeded"} {
			if !strings.Contains(content, s) {
				t.Fatalf("content missing %q:\n%s", s, content)
			}
		}
	})

	t.Run("quotes paths with spaces", func(t *testing.T) {
		content := BudgetCommandContent("C:/Program Files/cs/claude-statusline.exe")
		if !strings.Contains(content, `!`+"`"+`"C:/Program Files/cs/claude-statusline.exe" budget --json`+"`") {
			t.Fatalf("path not quoted:\n%s", content)
		}
	})

	t.Run("overwrites own file", func(t *testing.T) {
		dir := t.TempDir()
		_, _ = WriteBudgetCommand(dir, "/old/claude-statusline")
		written, err := WriteBudgetCommand(dir, "/new/claude-statusline")
		data, _ := os.ReadFile(filepath.Join(dir, "budget.md"))
		if err != nil || !written || !strings.Contains(string(data), "/new/claude-statusline") {
			t.Fatalf("written=%v err=%v content=%s", written, err, data)
		}
	})

	t.Run("preserves foreign file", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "budget.md"), []byte("meu comando"), 0644)
		written, err := WriteBudgetCommand(dir, "/x/claude-statusline")
		data, _ := os.ReadFile(filepath.Join(dir, "budget.md"))
		if err != nil || written || string(data) != "meu comando" {
			t.Fatalf("written=%v err=%v content=%s", written, err, data)
		}
	})
}

func TestRemoveBudgetCommand(t *testing.T) {
	dir := t.TempDir()
	if removed, err := RemoveBudgetCommand(dir); err != nil || removed {
		t.Fatalf("missing file: removed=%v err=%v", removed, err)
	}
	_ = os.WriteFile(filepath.Join(dir, "budget.md"), []byte("meu comando"), 0644)
	if removed, err := RemoveBudgetCommand(dir); err != nil || removed {
		t.Fatalf("foreign file: removed=%v err=%v", removed, err)
	}
	_, _ = WriteBudgetCommand(dir, "/x/claude-statusline")
	if removed, err := RemoveBudgetCommand(dir); err != nil || !removed {
		t.Fatalf("own file: removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "budget.md")); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/statusline/ -run 'TestWriteBudgetCommand|TestRemoveBudgetCommand' -v`
Expected: FAIL `undefined: WriteBudgetCommand`

- [ ] **Step 3: Write `budget_command.go`**

```go
package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const budgetCommandMarker = "gerado por claude-statusline"

// BudgetCommandContent é o slash command /budget do Claude Code: roda o
// binário em modo --json e pede pro modelo explicar em português.
func BudgetCommandContent(selfCmd string) string {
	if strings.Contains(selfCmd, " ") {
		selfCmd = `"` + selfCmd + `"`
	}
	return fmt.Sprintf(`---
description: Mostra seu limite e consumo no gateway LLM da Superlógica
allowed-tools: Bash(%s budget:*)
---
<!-- %s; reinstalar sobrescreve, apagar desinstala -->
Budget do usuário no gateway:

!`+"`"+`%s budget --json`+"`"+`

Apresente em português, sem tabela:
- Quanto já gastou e qual o teto, em reais (spent_brl e limit_brl já vêm em reais; o campo raw traz os valores originais em micro-reais). Diga o percentual (pct).
- Quando a janela reseta (window_end) e qual o período (period).
- Se scope for license, explique que não há teto individual e que os valores são o agregado da licença, não só dele.
- Se base_limit_brl for maior que limit_brl, explique que o teto individual está limitado pelo teto da licença.
- Se exceeded for true, diga que o acesso está bloqueado até a data de reset.
- Se vier stale true, avise que os dados são de fetched_at porque o gateway não respondeu agora.
- Se vier error, explique o que falta configurar e aponte a página "Ativando o Claude via LLM Gateway" no Confluence.
`, selfCmd, budgetCommandMarker, selfCmd)
}

// WriteBudgetCommand grava commands/budget.md. Preserva (written=false) um
// arquivo que não tenha o marcador: pode ser um comando do próprio usuário.
func WriteBudgetCommand(commandsDir, selfCmd string) (bool, error) {
	path := filepath.Join(commandsDir, "budget.md")
	if existing, err := os.ReadFile(path); err == nil && !strings.Contains(string(existing), budgetCommandMarker) {
		return false, nil
	}
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", commandsDir, err)
	}
	if err := os.WriteFile(path, []byte(BudgetCommandContent(selfCmd)), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// RemoveBudgetCommand apaga commands/budget.md só se foi gerado por nós.
func RemoveBudgetCommand(commandsDir string) (bool, error) {
	path := filepath.Join(commandsDir, "budget.md")
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !strings.Contains(string(existing), budgetCommandMarker) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
```

- [ ] **Step 4: Wire into `cmdInstall`**

Em `main.go` `cmdInstall`:

1. Depois do parse de args e antes do `home, err := os.UserHomeDir()`, nada muda. Depois de obter `home`, definir `commandsDir := filepath.Join(home, ".claude", "commands")`.
2. No bloco `if uninstall { ... }`, antes do `return` final (o que imprime `✓ statusLine removido`), adicionar:
   ```go
   		if removed, err := statusline.RemoveBudgetCommand(commandsDir); err == nil && removed {
   			fmt.Println("✓ /budget removido de", filepath.Join(commandsDir, "budget.md"))
   		}
   ```
3. Depois de calcular `cmd := self + " render"`, aplicar o default do preset:
   ```go
   	if preset == "gateway" && refresh == 0 {
   		refresh = 60 // budget muda sem turno novo; mesmo intervalo do script do time
   	}
   ```
4. Depois de imprimir `✓ statusLine instalado ...` e antes do `Próximo passo`, adicionar:
   ```go
   	written, err := statusline.WriteBudgetCommand(commandsDir, self)
   	switch {
   	case err != nil:
   		fmt.Println("⚠ não consegui gravar o /budget:", err)
   	case written:
   		fmt.Printf("✓ /budget instalado em %s\n", filepath.Join(commandsDir, "budget.md"))
   	default:
   		fmt.Printf("⚠ %s já existe e não é nosso — preservado\n", filepath.Join(commandsDir, "budget.md"))
   	}
   ```
5. Atualizar o `usage`: `claude-statusline install [--preset X]       escreve statusLine no ~/.claude/settings.json e o /budget em ~/.claude/commands` e a linha de opções `[--refresh N] [--force]      [--uninstall remove] (preset gateway usa --refresh 60 por padrão)`.

- [ ] **Step 5: Run tests and a dry install against a temp HOME**

Run: `go test ./...`
Expected: PASS

Run (Git Bash): `HOME=$(mktemp -d) USERPROFILE=$HOME go run . install --preset gateway && cat $HOME/.claude/commands/budget.md | head -5 && grep refreshInterval $HOME/.claude/settings.json`
Expected: mensagens `✓ config criado`, `✓ statusLine instalado`, `✓ /budget instalado`; frontmatter do budget.md; `"refreshInterval": 60`.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add internal/statusline/budget_command.go internal/statusline/budget_command_test.go main.go && git commit -m "feat: install /budget slash command and default refresh for gateway preset"
```

---

### Task 10: Studio — mock do gateway, badge e campos

**Files:**
- Modify: `web/src/types.ts`
- Modify: `web/src/App.tsx` (`DEFAULT_MOCK`, `mockToInput`, aviso do mock, "Como instalar", catálogo, `SortableChip`, `MockDataEditor`)
- Rebuild: `web/dist` (embarcado; ignorado pelo git, mas necessário pro build local)

**Interfaces:**
- Consumes: `/api/components` agora devolve `needs_gateway`; `/api/render` aceita `mock_input.gateway` e `mock_input.context_window.current_usage` (Tasks 2, 4, 5).

- [ ] **Step 1: Update `types.ts`**

Em `StatuslineComponentMeta`, depois de `needs_history: boolean`: `needs_gateway: boolean`.

Em `StatuslineConfig`, depois de `history?: ...`:
```ts
  gateway?: {
    enabled?: boolean
    base_url?: string
    token_file?: string
    cache_file?: string
    ttl?: string
    stale_ttl?: string
    timeout?: string
  }
```

Em `StatuslineMock`, depois de `cluster_name: string`:
```ts
  tokens_in: number
  tokens_out: number
  tokens_cache: number
  gateway_spent_brl: number
  gateway_limit_brl: number
  gateway_tokens: number
  gateway_reset: string // YYYY-MM-DD
  gateway_exceeded: boolean
```

- [ ] **Step 2: Update `DEFAULT_MOCK` and `mockToInput` in `App.tsx`**

Em `DEFAULT_MOCK`, depois de `cluster_name: 'auth-refactor',`:
```ts
  tokens_in: 3,
  tokens_out: 436,
  tokens_cache: 38630,
  gateway_spent_brl: 73.53,
  gateway_limit_brl: 520,
  gateway_tokens: 9_700_000,
  gateway_reset: '2026-10-01',
  gateway_exceeded: false,
```

Em `mockToInput`, substituir `context_window: { used_percentage: m.context_pct },` por:
```ts
    context_window: {
      used_percentage: m.context_pct,
      current_usage: {
        input_tokens: m.tokens_in,
        output_tokens: m.tokens_out,
        cache_read_input_tokens: m.tokens_cache,
      },
    },
```
e adicionar antes de `worktree:`:
```ts
    gateway: {
      spent_brl_micro: Math.round(m.gateway_spent_brl * 1_000_000),
      limit_brl_micro: Math.round(m.gateway_limit_brl * 1_000_000),
      base_limit_brl_micro: Math.round(m.gateway_limit_brl * 1_000_000),
      tokens: m.gateway_tokens,
      window_end: m.gateway_reset ? Math.floor(Date.parse(m.gateway_reset + 'T00:00:00Z') / 1000) : 0,
      exceeded: m.gateway_exceeded,
      scope: 'user',
      calendar_period: 'monthly',
    },
```

- [ ] **Step 3: Update helper texts, catalog and chip**

No aviso `⚠ Mock fields só aparecem...`, acrescentar ao fim da lista: `, <code>gateway_budget</code>, <code>gateway_tokens</code>, <code>gateway_reset</code>, <code>tokens_in</code>`.

Na seção "Como instalar", trocar o `<pre>` por:
```
# 1. salvar config (botão acima)
# 2. instalar entrada no settings.json (+ /budget em ~/.claude/commands):
claude-statusline install --preset gateway

# 3. reiniciar o Claude Code (statusLine só carrega no boot)
```

No catálogo, depois de `{c.needs_history && ...}`:
```tsx
                  {c.needs_gateway && <span className="text-sky-400">requer gateway</span>}
```

Em `SortableChip`: adicionar a prop `needsGateway: boolean` (tipo e destructuring), no `LineEditor` passar `needsGateway={meta?.needs_gateway ?? false}` junto de `needsHistory`, no tooltip acrescentar `+ (needsGateway ? '\n\n◈ requer LLM Gateway configurado (ANTHROPIC_BASE_URL + login Auth0)' : '')`, e depois do `⚡`:
```tsx
      {needsGateway && <span className="text-sky-400 text-[10px]" title="requer gateway">◈</span>}
```

- [ ] **Step 4: Add fields to `MockDataEditor`**

Dentro do grid de "Input (stdin do Claude Code)", depois do `Field` de `lines removed`, adicionar:
```tsx
            <Field label="tokens in" active={has('tokens_in') || has('tokens_total')}>
              <input type="number" min={0} value={mock.tokens_in} onChange={(e) => set('tokens_in', Number(e.target.value))}
                className="w-full bg-[var(--color-card)] border border-[var(--color-border)] rounded px-2 py-1 font-mono" />
            </Field>
            <Field label="tokens out" active={has('tokens_out') || has('tokens_total')}>
              <input type="number" min={0} value={mock.tokens_out} onChange={(e) => set('tokens_out', Number(e.target.value))}
                className="w-full bg-[var(--color-card)] border border-[var(--color-border)] rounded px-2 py-1 font-mono" />
            </Field>
            <Field label="tokens cache" active={has('tokens_cache')}>
              <input type="number" min={0} value={mock.tokens_cache} onChange={(e) => set('tokens_cache', Number(e.target.value))}
                className="w-full bg-[var(--color-card)] border border-[var(--color-border)] rounded px-2 py-1 font-mono" />
            </Field>
```

Antes do `<div className="border-t ... History (simula daemon)`, adicionar uma seção nova:
```tsx
      <div className="border-t border-[var(--color-border)] pt-3">
        <div className="text-[10px] text-[var(--color-muted)] uppercase tracking-wide mb-1.5">
          Gateway (simula /v1/usage do LLM Gateway)
        </div>
        <div className="space-y-2">
          <SliderField label="gasto R$" value={mock.gateway_spent_brl} min={0} max={1000} step={0.5}
            onChange={(v) => set('gateway_spent_brl', v)} active={has('gateway_budget')} />
          <div className="grid grid-cols-2 gap-2">
            <Field label="limite R$" active={has('gateway_budget')}>
              <input type="number" min={0} step={10} value={mock.gateway_limit_brl} onChange={(e) => set('gateway_limit_brl', Number(e.target.value))}
                className="w-full bg-[var(--color-card)] border border-[var(--color-border)] rounded px-2 py-1 font-mono" />
            </Field>
            <Field label="tokens período" active={has('gateway_tokens')}>
              <input type="number" min={0} step={100000} value={mock.gateway_tokens} onChange={(e) => set('gateway_tokens', Number(e.target.value))}
                className="w-full bg-[var(--color-card)] border border-[var(--color-border)] rounded px-2 py-1 font-mono" />
            </Field>
            <Field label="reset (data)" active={has('gateway_reset')}>
              <input type="date" value={mock.gateway_reset} onChange={(e) => set('gateway_reset', e.target.value)}
                className="w-full bg-[var(--color-card)] border border-[var(--color-border)] rounded px-2 py-1 font-mono" />
            </Field>
            <Field label="bloqueado" active={has('gateway_budget')}>
              <label className="flex items-center gap-2 py-1">
                <input type="checkbox" checked={mock.gateway_exceeded} onChange={(e) => set('gateway_exceeded', e.target.checked)} />
                <span>exceeded</span>
              </label>
            </Field>
          </div>
        </div>
      </div>
```

- [ ] **Step 5: Typecheck, build, and smoke test the Studio**

Run: `cd web && bun install && bun run build && cd ..`
Expected: `tsc --noEmit` sem erro, `vite build` gera `web/dist`.

Run: `go build -o /tmp/cs-test . && /tmp/cs-test studio --no-open --port 5599 &` e depois `curl -s localhost:5599/api/components | grep -c needs_gateway` (esperado `1`, o JSON vem numa linha) e:
```bash
curl -s -X POST localhost:5599/api/render -H 'Content-Type: application/json' -d '{"config":{"theme":"graphite","style":"plain","lines":[{"components":["gateway_budget","gateway_reset","tokens_cache"]}]},"mock_input":{"cwd":"/x","session_id":"s","model":{"display_name":"Opus"},"context_window":{"current_usage":{"cache_read_input_tokens":38630}},"gateway":{"spent_brl_micro":73530000,"limit_brl_micro":520000000,"window_end":1790812800}}}'
```
Expected: `ansi` contém `R$ 73,53 / R$ 520,00 (14%)`, `reset 01/10`, `Cache: 38.6k`. Matar o processo depois.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add web/src/types.ts web/src/App.tsx && git commit -m "feat(studio): gateway and session token mock fields with gateway badge"
```

---

### Task 11: `version`, CI, release e bootstrap

**Files:**
- Modify: `main.go` (`version`, subcomando, propaga pra `statusline.Version`)
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `install.sh`, `install.ps1`

**Interfaces:**
- Consumes: `statusline.Version` (Task 3).
- Produces: `claude-statusline version` imprime `claude-statusline <version>`; assets `claude-statusline_<os>_<arch>.tar.gz|zip` + `SHA256SUMS` na release.

- [ ] **Step 1: Version in `main.go`**

Depois dos imports: 
```go
// version é injetada no build da release: -ldflags "-X main.version=v1.2.3".
var version = "dev"
```

No início de `main()`, antes do `switch`: `statusline.Version = version`.

No `switch`: `case "version", "--version", "-v": fmt.Println("claude-statusline " + version)`.

Verificar: `go run . version` → `claude-statusline dev`; `go build -ldflags "-X main.version=v9.9.9" -o /tmp/cs . && /tmp/cs version` → `claude-statusline v9.9.9`.

- [ ] **Step 2: CI workflow**

`.github/workflows/ci.yml`:
```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - name: build studio
        working-directory: web
        run: bun install --frozen-lockfile && bun run build
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: gofmt -l . && test -z "$(gofmt -l .)"
      - run: go vet ./...
      - run: go test ./...
      - run: go build .
```

- [ ] **Step 3: Release workflow**

`.github/workflows/release.yml`:
```yaml
name: release
on:
  push:
    tags: ['v*']
permissions:
  contents: write
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - name: build studio
        working-directory: web
        run: bun install --frozen-lockfile && bun run build
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test ./...
      - name: cross-compile
        env:
          VERSION: ${{ github.ref_name }}
        run: |
          set -euo pipefail
          mkdir -p dist
          for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
            os="${target%/*}"; arch="${target#*/}"
            bin="claude-statusline"; [ "$os" = windows ] && bin="claude-statusline.exe"
            CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "dist/$bin" .
            name="claude-statusline_${os}_${arch}"
            if [ "$os" = windows ]; then
              (cd dist && zip -q "$name.zip" "$bin" && rm "$bin")
            else
              (cd dist && tar -czf "$name.tar.gz" "$bin" && rm "$bin")
            fi
          done
          (cd dist && sha256sum * > SHA256SUMS)
          ls -la dist
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
          generate_release_notes: true
```

- [ ] **Step 4: Bootstrap scripts**

`install.sh`:
```sh
#!/usr/bin/env sh
# Instala a última release do claude-statusline e pluga no Claude Code.
# Uso: curl -fsSL https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.sh | sh
#      ... | sh -s -- --preset compact   (default: gateway)
set -eu

REPO="Felipeness/claude-statusline"
PRESET="gateway"
while [ $# -gt 0 ]; do
  case "$1" in
    --preset) PRESET="$2"; shift 2 ;;
    *) echo "opção desconhecida: $1" >&2; exit 1 ;;
  esac
done

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  mingw*|msys*|cygwin*) os=windows ;;
  linux|darwin) ;;
  *) echo "sistema não suportado: $os" >&2; exit 1 ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "arquitetura não suportada: $arch" >&2; exit 1 ;;
esac
if [ "$os" = windows ] && [ "$arch" = arm64 ]; then
  echo "windows/arm64 ainda não tem binário na release" >&2; exit 1
fi

ext=tar.gz; bin=claude-statusline
if [ "$os" = windows ]; then ext=zip; bin=claude-statusline.exe; fi
url="https://github.com/$REPO/releases/latest/download/claude-statusline_${os}_${arch}.$ext"
bin_dir="${CLAUDE_STATUSLINE_BIN_DIR:-$HOME/.local/bin}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "baixando $url"
curl -fsSL "$url" -o "$tmp/pkg.$ext"
if [ "$ext" = zip ]; then unzip -q "$tmp/pkg.zip" -d "$tmp"; else tar -xzf "$tmp/pkg.tar.gz" -C "$tmp"; fi
mkdir -p "$bin_dir"
cp "$tmp/$bin" "$bin_dir/$bin"
chmod 755 "$bin_dir/$bin"

echo "instalado em $bin_dir/$bin"
"$bin_dir/$bin" install --preset "$PRESET" --force
```

`install.ps1`:
```powershell
# Instala a última release do claude-statusline e pluga no Claude Code (Windows).
# Uso: irm https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.ps1 | iex
param([string]$Preset = "gateway")
$ErrorActionPreference = "Stop"

$repo = "Felipeness/claude-statusline"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
if ($arch -eq "arm64") { throw "windows/arm64 ainda não tem binário na release" }
$url = "https://github.com/$repo/releases/latest/download/claude-statusline_windows_$arch.zip"
$binDir = if ($env:CLAUDE_STATUSLINE_BIN_DIR) { $env:CLAUDE_STATUSLINE_BIN_DIR } else { Join-Path $HOME "bin" }
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("claude-statusline-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp, $binDir | Out-Null

Write-Host "baixando $url"
Invoke-WebRequest -Uri $url -OutFile (Join-Path $tmp "pkg.zip")
Expand-Archive -Path (Join-Path $tmp "pkg.zip") -DestinationPath $tmp -Force
Copy-Item (Join-Path $tmp "claude-statusline.exe") (Join-Path $binDir "claude-statusline.exe") -Force
Remove-Item -Recurse -Force $tmp

$exe = Join-Path $binDir "claude-statusline.exe"
Write-Host "instalado em $exe"
& $exe install --preset $Preset --force
```

Marcar `install.sh` como executável no git: `git update-index --chmod=+x install.sh` (depois do `git add`).

- [ ] **Step 5: Verify locally**

Run: `go vet ./... && go test ./... && go build . && sh -n install.sh`
Expected: tudo verde, `sh -n` sem erro de sintaxe.

Run (cross-compile smoke): `GOOS=darwin GOARCH=arm64 go build -o /tmp/cs-darwin . && GOOS=linux GOARCH=amd64 go build -o /tmp/cs-linux . && echo ok`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add main.go .github/workflows/ci.yml .github/workflows/release.yml install.sh install.ps1 && git update-index --chmod=+x install.sh && git commit -m "ci: add test and release workflows with version command and bootstrap installers"
```

---

### Task 12: Docs (README PT/EN e página pro time)

**Files:**
- Modify: `README.md`, `README.en.md`
- Create: `docs/confluence-instalacao-time.md`

- [ ] **Step 1: README.md (PT-BR)**

1. Badge/linha de contagem: `**25 components** • **5 themes** • **3 styles** • **4 presets** • **0 deps em runtime**`. Trocar todas as ocorrências de "16 components" por "25 components" (inclusive no mermaid `components.go`, no Studio "Catálogo" e no sumário).
2. Sumário: adicionar item `Budget do LLM Gateway (Superlógica)` depois de "Components disponíveis" e `Instalação pro time` antes de "Quick Start".
3. Tabela de components: adicionar categoria `gateway` e `context`:
   ```
   | `gateway_budget` | gateway | `🟢 R$ 73,53 / R$ 520,00 (14%)` — consumo no LLM Gateway, 🟡 ≥70%, 🔴 ≥90%, `🚫 BLOQUEADO` quando excedido (requer gateway) |
   | `gateway_tokens` | gateway | `9.7M tokens` processados no período (requer gateway) |
   | `gateway_reset` | gateway | `reset 01/10`, dia em que o budget zera (requer gateway) |
   | `tokens_in` / `tokens_out` / `tokens_total` | context | `In: 3` `Out: 436` `Total: 439` da sessão atual |
   | `tokens_cache` | context | `Cache: 38.6k` tokens lidos do prompt cache (some quando 0) |
   ```
   e atualizar a linha do `auth_mode` na tabela (se existir) pra `[Gateway] / [OAuth] / [API key]`.
4. Seção nova `## ~ Budget do LLM Gateway (Superlógica)`:
   ```markdown
   Quem usa o Claude Code pelo [LLM Gateway da Superlógica](https://superlogica.atlassian.net/wiki/spaces/SPL/pages/4240015369) tem budget mensal em reais. O `claude-statusline` lê o mesmo `GET /v1/usage` que o gateway expõe, com o token do Auth0 que o Claude Code já guarda em `~/.claude/auth0-token-cache.json`, e mostra na linha:

   ```
   ~/projects/app  main  Sonnet 4.6 │ 🟢 R$ 73,53 / R$ 520,00 (14%) │ 9.7M tokens │ reset 01/10
   ▓▓▓░░░ 42% · In: 3 · Out: 436 · Total: 439 · Cache: 38.6k
   ```

   - Detecção automática: basta `ANTHROPIC_BASE_URL` no env do Claude Code e o login do gateway feito. Sem isso os components somem e nada quebra.
   - Cache de 60s em `~/.cache/claude-statusline-gateway.json` (só consumo, nunca o token). Se o gateway não responder, usa o último valor por até 1h.
   - `/budget` dentro do Claude Code: o `install` grava `~/.claude/commands/budget.md`, que roda `claude-statusline budget --json` e explica gasto, teto, reset, escopo (individual ou pool da licença) e bloqueio.
   - No terminal: `claude-statusline budget` (texto) ou `claude-statusline budget --json`.

   Config opcional em `~/.claude-statusline/config.toml`:

   ```toml
   [gateway]
   enabled = true       # false desliga o probe
   base_url = ""        # vazio = ANTHROPIC_BASE_URL
   token_file = ""      # vazio = ~/.claude/auth0-token-cache.json
   ttl = "60s"
   stale_ttl = "1h"
   timeout = "4s"
   ```
   ```
5. Seção nova `## ~ Instalação pro time` (antes do Quick Start de dev):
   ```markdown
   Sem Go, sem Bun, sem jq. Baixa o binário da release e pluga:

   ```bash
   # macOS / Linux / WSL / Git Bash
   curl -fsSL https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.sh | sh
   ```

   ```powershell
   # Windows (PowerShell)
   irm https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.ps1 | iex
   ```

   O script baixa `claude-statusline_<os>_<arch>` da [última release](https://github.com/Felipeness/claude-statusline/releases/latest), coloca em `~/.local/bin` (`~/bin` no Windows) e roda `claude-statusline install --preset gateway --force` (o `--force` substitui um statusline anterior, com backup do `settings.json`). Depois é só reiniciar o Claude Code.

   Prefere manual? Baixe o asset da release, extraia e rode `claude-statusline install --preset gateway`. Pra outro preset: `... | sh -s -- --preset compact`.
   ```
6. Quick Start de dev: renomear o título pra `## ~ Quick Start (build local)` e adicionar `claude-statusline install --preset gateway   # ou compact/max/powerline`.
7. Presets: onde o README cita `compact / max / powerline`, incluir `gateway`.
8. Estrutura do projeto: adicionar `format.go`, `gateway.go`, `gateway_probe.go`, `components_gateway.go`, `components_tokens.go`, `budget_report.go`, `budget_command.go`, `.github/workflows/`, `install.sh`, `install.ps1`.
9. Licença: trocar "A definir" por `MIT` (já existe `LICENSE`).

- [ ] **Step 2: README.en.md**

Espelhar as mesmas mudanças em inglês (mesmas seções, mesmos exemplos, mesmos comandos). Títulos: `LLM Gateway budget (Superlógica)`, `Team install`.

- [ ] **Step 3: `docs/confluence-instalacao-time.md`**

```markdown
# Acompanhando seu budget no LLM Gateway com o claude-statusline

> Rascunho pra publicar no Confluence (espaço SPL, ao lado de "Ativando o Claude via LLM Gateway"). Substitui a versão em bash.

## Objetivo

Depois desta configuração, a barra inferior do Claude Code mostra seu consumo mensal no LLM Gateway (gasto, teto, percentual e data de reset), os tokens da sessão, e o comando `/budget` traz o detalhamento. Tudo num binário único, sem `jq`, `python` ou scripts.

## Pré-requisitos

- Claude Code configurado com o LLM Gateway (página "Ativando o Claude via LLM Gateway") e login feito pelo menos uma vez.
- Nada mais. Não precisa de `jq`.

## Instalação

macOS, Linux, WSL ou Git Bash:

```bash
curl -fsSL https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.ps1 | iex
```

O instalador baixa a release, grava o `statusLine` no `~/.claude/settings.json` (com backup, substituindo o script antigo se existir) e cria o `/budget` em `~/.claude/commands/budget.md`.

Feche e reabra o Claude Code.

## O que aparece

```
~/projects/app  main  Sonnet 4.6 │ 🟢 R$ 73,53 / R$ 520,00 (14%) │ 9.7M tokens │ reset 01/10
▓▓▓░░░ 42% · In: 3 · Out: 436 · Total: 439 · Cache: 38.6k
```

- **Sonnet 4.6**: modelo ativo.
- **🟢 R$ 73,53 / R$ 520,00 (14%)**: gasto no mês, teto e percentual. 🟢 normal, 🟡 acima de 70%, 🔴 acima de 90%, 🚫 BLOQUEADO quando o teto foi excedido.
- **9.7M tokens**: tokens processados pelo gateway no período, todas as sessões.
- **reset 01/10**: dia em que o budget zera.
- **In / Out / Total / Cache**: tokens da sessão atual (entrada, saída, soma, lidos do cache de prompt).

Atualiza a cada 60 segundos e a cada mensagem.

## `/budget`

Digite `/budget` no Claude Code pra ver gasto, teto, percentual, data de reset, período, se o escopo é individual ou o pool da licença, e se o acesso está bloqueado. No terminal: `claude-statusline budget`.

## Personalizar

`claude-statusline studio` abre um editor visual no navegador: temas, estilo powerline, ordem dos chips, thresholds e preview ao vivo.

## Problemas

- Chips do gateway não aparecem: confira `ANTHROPIC_BASE_URL` no `settings.json` e faça login abrindo o Claude Code. `claude-statusline budget` diz o que falta.
- Quer voltar atrás: `claude-statusline install --uninstall` restaura o `settings.json` sem o statusline e remove o `/budget`.
- Dúvidas: Felipe Coelho no Slack.
```

- [ ] **Step 4: Commit**

```bash
git add README.md README.en.md docs/confluence-instalacao-time.md && git commit -m "docs: gateway budget, session tokens, team install guide"
```

---

## Self-Review

**Spec coverage**: RF1 → Task 3; RF2, RF3, RF4 → Task 4; RF5 → Task 5; RF6 → Task 6; RF7 → Tasks 7 e 9; RF8 → Task 8; RF9 → Task 9; RF10 → Task 10; RF11 → Task 1; RF12, RF13, RF14 → Task 11; RF15 → Task 12. Spec 5.3 (parse tolerante, fixture) → Task 2. Spec 6 (erros) → Tasks 3 e 8. Spec 7 (testes) → todas as tasks têm os testes listados. Primeira release `v1.0.0` (spec 5.10) fica com o orquestrador depois da Task 12, porque publica fora do repo.

**Placeholder scan**: nenhum `TBD`/`TODO`; todo passo de código traz o código.

**Type consistency**: `GatewayUsage` (campos e `Pct`/`ResetDate`) igual nas Tasks 2, 4, 7, 8, 10; `GatewayProbeResult{Usage, Raw, FetchedAt, Stale}` igual nas Tasks 3 e 8; `detectAuthMode(bool, *ProbeResult, bool)` igual nas Tasks 6 e teste; `WriteBudgetCommand(dir, self) (bool, error)` igual nas Tasks 9 e main; `ComponentMeta.NeedsGateway` → `needs_gateway` igual nas Tasks 4 e 10.
