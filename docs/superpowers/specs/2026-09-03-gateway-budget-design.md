# Spec: budget do LLM Gateway, tokens de sessão e distribuição pro time

**Data**: 2026-09-03 | **Status**: aprovado pelo Felipe em chat ("faz logo tudo, deixa pronto pro time, tudo em Go")

## 1. Contexto

O Marcelo Camargo publicou no Confluence ("Acompanhando seu budget no LLM Gateway") um statusline em bash + jq + python3 que mostra o consumo mensal do usuário no LLM Gateway da Superlógica (Kong em `kong-ai-gtw.idp.nexus.superlogica.net/llm-gtw`), mais os tokens da sessão, e um slash command `/budget`. O `claude-statusline` (este repo) vai absorver essas features em Go e substituir o script dele pro time.

O que o script do Marcelo faz, e que precisamos cobrir:

- Lê o JWT do Auth0 em `~/.claude/auth0-token-cache.json` (`access_token`, `expires_at` em epoch segundos).
- `GET {ANTHROPIC_BASE_URL}/v1/usage` com `Authorization: Bearer`, timeout 4s.
- Extrai de `entries[0].budget`: `effectiveLimit.brlLimitMicro` (fallback `limit.brlLimitMicro`), `spend.offerBrlMicro`, `spend.tokens`, `window.end`, `exceeded`. O `/budget` também cita `scope` (`license` = pool compartilhado) e `calendarPeriod`.
- Cache de 60s em arquivo. `refreshInterval: 60` no `settings.json`.
- Linha: `modelo | 🟢 R$ 73,53 / R$ 520,00 (14%) | 9.7M tokens | reset 01/10 | In: 3 | Out: 436 | Total: 439 · Cache: 38.6k`.
- Severidade: 🟡 a partir de 70%, 🔴 a partir de 90%, 🚫 `BLOQUEADO` quando `exceeded`.
- `--json` devolve a resposta crua pro `/budget` (arquivo `~/.claude/commands/budget.md`) que pede pro Claude explicar em português.

## 2. Objetivo

Depois desta entrega, alguém do time baixa um binário, roda `claude-statusline install --preset gateway`, reinicia o Claude Code e tem a mesma informação do script do Marcelo (e tudo que o `claude-statusline` já tinha), com `/budget` funcionando, sem jq, python ou bash.

### 2.1 Critérios de sucesso (mensuráveis)

| # | Critério | Como verificar |
|---|----------|----------------|
| CS1 | Instalação pro time em no máximo 2 comandos (baixar + `install`) e 1 reinício do Claude Code | Seguir o quick start do README numa máquina limpa |
| CS2 | Statusline mostra os 7 dados do script do Marcelo (budget, tokens do período, reset, in, out, total, cache) | Preset `gateway` renderizado com a fixture |
| CS3 | Nenhuma dependência externa em runtime (0 chamadas a jq/python/bash/curl) | `grep` por `exec.Command` só encontra `git` e o `openURL` do Studio |
| CS4 | Render com cache fresco não faz nenhuma request HTTP; no máximo 1 request por 60s por máquina | Teste com `httptest.Server` contando hits |
| CS5 | Render com gateway indisponível termina em menos de 5s (timeout 4s) e sem nada em stderr | Teste com servidor que dorme |
| CS6 | `/budget` responde em português com gasto, teto, %, reset e estado bloqueado quando aplicável | Rodar `/budget` com fixture no lugar do gateway |
| CS7 | Release publica 5 binários (linux amd64/arm64, darwin amd64/arm64, windows amd64) com checksums | Assets da release `v1.0.0` |
| CS8 | `go vet` e `go test ./...` verdes em CI a cada PR | Workflow de CI |

## 3. Fora de escopo

- Editar a página do Marcelo no Confluence (fica um rascunho em `docs/confluence-instalacao-time.md` pro Felipe publicar).
- Histórico de gasto no gateway ao longo do tempo (daemon).
- Suporte a outros gateways além do formato `/v1/usage` descrito.
- Refactor de components existentes.

## 4. Requisitos funcionais

| ID | Requisito |
|----|-----------|
| RF1 | Probe do gateway: buscar `/v1/usage` com o JWT do Auth0, cache em disco com TTL 60s, fallback stale 1h, timeout 4s, fail-open |
| RF2 | Component `gateway_budget`: ícone de estado + `R$ gasto / R$ limite (pct%)`, severidade 70/90, estado bloqueado |
| RF3 | Component `gateway_tokens`: tokens do período (`9.7M tokens`) |
| RF4 | Component `gateway_reset`: `reset dd/mm` a partir de `window.end` |
| RF5 | Components `tokens_in`, `tokens_out`, `tokens_total`, `tokens_cache` a partir de `context_window.current_usage` (fallback totais) |
| RF6 | `auth_mode` reconhece o estado `gateway` (chip `[Gateway]`) |
| RF7 | Preset `gateway` com esses components e `refreshInterval` 60 por padrão |
| RF8 | Subcomando `claude-statusline budget` (humano) e `budget --json` (normalizado + raw) |
| RF9 | `install` grava `~/.claude/commands/budget.md` apontando pro binário |
| RF10 | Studio: categoria `gateway`, badge "requer gateway", campos de mock pro gateway e tokens |
| RF11 | Formatação BRL pt-BR a partir de micro-reais (`R$ 1.520,00`) e tokens `k`/`M` |
| RF12 | Release: workflow no GitHub Actions que, em tag `v*`, builda o Studio, cross-compila 5 alvos e publica no GitHub Releases com checksums |
| RF13 | `claude-statusline version` e scripts de bootstrap `install.sh` / `install.ps1` que baixam a release certa e rodam o `install` |
| RF14 | CI em PR: `go vet`, `go test ./...`, build do Studio |
| RF15 | README PT-BR e EN atualizados; rascunho de página Confluence pro time |

## 5. Design

### 5.1 Dados: `Input.Gateway`

Novo campo em `Input` (`internal/statusline/input.go`):

```go
Gateway *GatewayUsage `json:"gateway,omitempty"` // preenchido pelo probe (render) ou pelo mock (Studio)

type GatewayUsage struct {
    SpentBRLMicro     int64  `json:"spent_brl_micro"`
    LimitBRLMicro     int64  `json:"limit_brl_micro"`      // effectiveLimit, fallback limit
    BaseLimitBRLMicro int64  `json:"base_limit_brl_micro"` // limit (pra explicar teto da licença)
    Tokens            int64  `json:"tokens"`
    WindowEnd         int64  `json:"window_end"`           // unix epoch; 0 = desconhecido
    Exceeded          bool   `json:"exceeded"`
    Scope             string `json:"scope,omitempty"`           // user | license | ""
    CalendarPeriod    string `json:"calendar_period,omitempty"` // monthly | weekly | ""
}
```

Métodos puros: `Pct() float64` (0 quando limite 0), `Status() GatewayStatus` (`ok | warn | crit | blocked`, thresholds vêm do component).

Tem tag JSON (diferente de `AuthMode`) porque o Studio envia esse objeto dentro de `mock_input`.

### 5.2 Probe: `internal/statusline/gateway_probe.go`

Segue o molde do `oauth_probe.go`, com uma diferença: o cache em disco é o cache primário (cada `render` é um processo novo, cache em memória não ajuda).

Config em `Config.Gateway`:

```toml
[gateway]
enabled = true            # default true; só age se o token file existir
base_url = ""             # vazio = env ANTHROPIC_BASE_URL
token_file = ""           # vazio = ~/.claude/auth0-token-cache.json
ttl = "60s"
stale_ttl = "1h"
timeout = "4s"
```

Fluxo de `ProbeGateway(cfg GatewayConfig) *GatewayUsage`:

1. `enabled` falso → nil.
2. Resolver `base_url` (config, senão env `ANTHROPIC_BASE_URL`). Vazio → nil.
3. Cache em disco `~/.cache/claude-statusline-gateway.json` com `fetched_at`: se idade < `ttl` → devolve sem HTTP.
4. Ler token file: `access_token` e `expires_at`. Ausente, vazio ou expirado → devolve cache stale (idade < `stale_ttl`) ou nil.
5. `GET {base_url}/v1/usage`, `Authorization: Bearer`, `Accept: application/json`, `User-Agent: claude-statusline/<version>`, `context.WithTimeout(timeout)`.
6. Status ≠ 200 ou JSON inválido → cache stale ou nil.
7. Parse defensivo (5.3), grava cache (0600, só os campos de `GatewayUsage`, nunca o token), devolve.

`ProbeGatewayRaw` faz o mesmo mas devolve também o `json.RawMessage` da resposta, pro `budget --json`. O cache em disco guarda o raw junto (é só dados de consumo, sem credencial).

### 5.3 Parse da resposta

Schema conhecido apenas pelo `jq` do script do Marcelo. Parse tolerante, todo campo opcional:

```json
{
  "entries": [{
    "scope": "user",
    "calendarPeriod": "monthly",
    "budget": {
      "limit":          { "brlLimitMicro": 520000000 },
      "effectiveLimit": { "brlLimitMicro": 520000000 },
      "spend":          { "offerBrlMicro": 73530000, "tokens": 9700000 },
      "window":         { "end": "2026-10-01T00:00:00Z" },
      "exceeded":       false,
      "scope": "...", "calendarPeriod": "..."
    }
  }]
}
```

- `entries` vazio ou sem `budget` → nil (component some).
- `scope` e `calendarPeriod` lidos no entry e no budget (o que existir).
- `window.end`: ISO 8601 com ou sem hora (`2006-01-02`), via `parseIsoEpoch` estendido.
- Limite 0 → `Pct()` 0 e component mostra só o gasto.

Fixture do JSON acima fica em `internal/statusline/testdata/gateway_usage.json`.

### 5.4 Components (`internal/statusline/components_gateway.go`)

Categoria `gateway`, `NeedsGateway: true` no `ComponentMeta` (novo campo, `json:"needs_gateway"`).

| Component | Texto | Regras |
|---|---|---|
| `gateway_budget` | `🟢 R$ 73,53 / R$ 520,00 (14%)` | `Classify(pct, warn 70, crit 90)` → 🟢/🟡/🔴 e cor de severidade. `Exceeded` → `🚫 BLOQUEADO R$ 73,53 / R$ 520,00`, cor crit, bold. Limite 0 → `R$ 73,53`. `HasWarnAt: true` |
| `gateway_tokens` | `9.7M tokens` | some se `Tokens == 0` |
| `gateway_reset` | `reset 01/10` | some se `WindowEnd == 0`. `label_prefix` respeitado |

Todos somem quando `In.Gateway == nil`. Cores via `Theme.SegOf(name)` com fallback em `Default`; o theme graphite (default do powerline) ganha entries pra `gateway_budget`, `gateway_tokens`, `gateway_reset`; os outros 4 themes usam `Default`, como já fazem pros components existentes.

Tokens de sessão (`internal/statusline/components_tokens.go`), categoria `context`:

| Component | Texto | Fonte |
|---|---|---|
| `tokens_in` | `In: 3` | `Context.Current.InputTokens`, fallback `TotalInputTokens` |
| `tokens_out` | `Out: 436` | `Context.Current.OutputTokens`, fallback `TotalOutputTokens` |
| `tokens_total` | `Total: 439` | in + out |
| `tokens_cache` | `Cache: 38.6k` | `Context.Current.CacheReadInputTokens`; some se 0 |

`tokens_in`, `tokens_out`, `tokens_total` aparecem mesmo com 0 (a linha não pula no início da sessão).

Helpers puros em `format.go`: `FormatBRLMicro(int64) string` (`R$ 1.520,00`, separador de milhar `.`, decimal `,`, arredondamento half-up nos centavos) e `FormatTokens(int64) string` (`999`, `38.6k`, `9.7M`).

### 5.5 `auth_mode`

`detectAuthMode` ganha o valor `gateway`, com prioridade máxima: se o probe do gateway devolveu dados, ou se `ANTHROPIC_BASE_URL` está setado e o token file do Auth0 existe, é `gateway`. Chip `[Gateway]` em severidade OK, bold. Fallback antigo (sem `AuthMode`) inalterado.

### 5.6 Preset `gateway`

```go
Style: "plain"
Lines[0]: cwd, git, model, gateway_budget, gateway_tokens, gateway_reset   (sep " │ ")
Lines[1]: context_pct, tokens_in, tokens_out, tokens_total, tokens_cache   (sep " · ")
Components: gateway_budget {70, 90} + defaults existentes
```

`install --preset gateway` usa `refreshInterval` 60 quando `--refresh` não é passado (o budget muda sem turno novo).

### 5.7 Subcomando `budget`

`claude-statusline budget`:

```
Budget do LLM Gateway
  gasto     R$ 73,53 de R$ 520,00 (14%)
  período   mensal, reseta em 01/10/2026
  escopo    individual
  tokens    9.7M no período
  status    OK
```

- `status` em `ATENÇÃO` (≥70), `CRÍTICO` (≥90), `BLOQUEADO até 01/10/2026` (`exceeded`).
- `escopo` `licença (pool compartilhado, valores são o agregado da licença)` quando `scope == license`; nota `teto individual limitado pelo teto da licença` quando `LimitBRLMicro < BaseLimitBRLMicro`.
- Sem gateway configurado ou probe falhou: mensagem em stdout explicando (`gateway não configurado: ANTHROPIC_BASE_URL ausente` / `token do Auth0 ausente ou expirado, abra o Claude Code pra renovar` / `gateway inacessível`), exit 0.

`budget --json` imprime:

```json
{
  "spent_brl": 73.53, "limit_brl": 520.0, "base_limit_brl": 520.0, "pct": 14,
  "tokens": 9700000, "window_end": "2026-10-01T00:00:00Z", "period": "monthly",
  "scope": "user", "exceeded": false, "fetched_at": "...", "raw": { ...resposta do gateway... }
}
```

Em erro: `{"error": "<mesma mensagem humana>"}`, exit 0 (o `/budget` precisa do texto pra explicar).

### 5.8 `install` grava o `/budget`

`install` (qualquer preset) escreve `~/.claude/commands/budget.md`:

```markdown
---
description: Mostra seu limite e consumo no gateway LLM da Superlógica
allowed-tools: Bash("<self>" budget:*)
---
<!-- gerado por claude-statusline; reinstalar sobrescreve -->
Budget do usuário no gateway:

!`"<self>" budget --json`

Apresente em português, sem tabela: quanto já gastou e o teto em reais, o percentual,
quando a janela reseta e o período. Se `scope` for `license`, explique que não há teto
individual e os valores são o agregado da licença. Se `base_limit_brl` for maior que
`limit_brl`, explique que o teto individual está limitado pelo teto da licença. Se
`exceeded` for true, diga que o acesso está bloqueado até o reset. Se vier `error`,
explique o que falta configurar.
```

`<self>` é o path do binário com forward slashes (mesma normalização do `command`). Sobrescreve só se o arquivo não existe ou contém o marcador `gerado por claude-statusline`; caso contrário avisa e preserva. `install --uninstall` remove o arquivo se tiver o marcador.

### 5.9 Studio

- `types.ts`: `needs_gateway` em `StatuslineComponentMeta`; `StatuslineConfig.gateway`; campos novos em `StatuslineMock`: `gateway_spent_brl`, `gateway_limit_brl`, `gateway_tokens`, `gateway_reset` (`YYYY-MM-DD`), `gateway_exceeded`, `tokens_in`, `tokens_out`, `tokens_cache`.
- `mockToInput` monta `Input.Gateway` (micro-reais = BRL × 1e6) e `Context.Current`.
- Badge `requer gateway` (mesmo estilo do `requer daemon`) no catálogo e no chip.
- `MockDataEditor` ganha os campos, com o mesmo aviso de "só aparece se o component estiver na linha".
- Texto do card de ajuda cita o preset `gateway` e o `/budget`.
- Rebuild do `web/dist` faz parte da entrega (é embarcado no binário).

### 5.10 Distribuição

- `main.go`: `var version = "dev"` via `-ldflags "-X main.version=vX.Y.Z"`; subcomando `version`; probe usa no `User-Agent`.
- `.github/workflows/ci.yml`: em push/PR: `bun install && bun run build` em `web/`, `go vet ./...`, `go test ./...`, `go build`.
- `.github/workflows/release.yml`: em tag `v*`: build do Studio, cross-compile `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` com `-s -w`, arquivos `claude-statusline_<os>_<arch>.tar.gz` (`.zip` no Windows), `SHA256SUMS`, release via `softprops/action-gh-release` com notas automáticas.
- `install.sh` (macOS/Linux/WSL/Git Bash) e `install.ps1` (PowerShell): detectam OS/arch, baixam o asset da última release pra `~/.local/bin` (ou `~/bin` no Windows), rodam `claude-statusline install --preset gateway --force` (o `--force` é intencional: substitui o statusline do Marcelo, com backup). Aceitam `--preset X` pra sobrescrever.
- Primeira release: tag `v1.0.0` depois de `go test` e build local verdes.

### 5.11 Docs

- README PT-BR e EN: contagem de components (25), tabela com os 7 novos, preset `gateway`, seção "Budget do LLM Gateway (Superlógica)" com `/budget`, quick start pro time (release + one-liner), config `[gateway]`.
- `docs/confluence-instalacao-time.md`: rascunho da página de instalação pro time, no formato da página do Marcelo (objetivo, pré-requisitos, passo a passo, o que cada chip mostra, `/budget`, dúvidas).

## 6. Tratamento de erros

| Situação | Comportamento |
|---|---|
| Token file ausente | probe devolve cache stale ou nil; components somem; `budget` explica |
| Token expirado | idem; não chama o gateway (evita 401 em loop) |
| Gateway 401/403/5xx ou timeout | cache stale (< 1h) ou nil; nunca stderr no render |
| JSON sem `entries[0].budget` | nil; `budget` diz "resposta sem budget" |
| Limite 0 | `gateway_budget` mostra só o gasto |
| `window.end` inválido | `gateway_reset` some; `budget` omite a data |
| Cache em disco corrompido | ignorado e sobrescrito no próximo fetch OK |

## 7. Testes (Go, table-driven, colocados)

- `format_test.go`: BRL (0, 73530000, 1520000000, arredondamento 999999 → R$ 1,00) e tokens (999, 1000, 38630, 9700000).
- `gateway_probe_test.go`: parse da fixture completa, entry sem budget, `effectiveLimit` ausente, `window.end` só data, `scope` no entry vs budget; cache TTL fresco não chama HTTP (`httptest.Server` contando hits); token expirado não chama HTTP; 500 devolve stale.
- `components_gateway_test.go`: `gateway_budget` em OK/warn/crit/blocked/limite 0/nil; `gateway_tokens` e `gateway_reset` visíveis e ocultos.
- `components_tokens_test.go`: current vs fallback totals, cache oculto em 0.
- `main_test.go` ou `authmode_test.go`: `detectAuthMode` com gateway.
- `install_test.go`: `WriteBudgetCommand` cria, sobrescreve com marcador, preserva sem marcador; `Uninstall` remove com marcador.
- `config_test.go`: merge de `[gateway]` sobre defaults.

Teste ao vivo com o gateway real fica pro Felipe (ele ainda não tem `ANTHROPIC_BASE_URL` nem o cache do Auth0 nesta máquina).

## 8. Riscos e premissas

- **Schema do `/v1/usage`** inferido do `jq` do Marcelo. Parse tolerante e fixture documentada; se o gateway mudar, só `gateway_probe.go` muda.
- **`expires_at`** assumido em epoch segundos (o script compara com `date +%s`).
- **Emoji no terminal**: 🟢🟡🔴🚫 são os mesmos que o time já vê hoje. `Charset` do config ainda não é aplicado em nenhum component; fica como está.
- **Windows**: paths com espaço no `commands/budget.md` vão entre aspas.
