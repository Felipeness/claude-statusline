# Pre-Implementation Audit Report

**Spec**: `docs/superpowers/specs/2026-09-03-gateway-budget-design.md` | **Plan**: `docs/superpowers/plans/2026-09-03-gateway-budget.md` | **Date**: 2026-09-03
**Constitution**: `docs/superpowers/constitution.md` (v1.0)

## Summary

| Metric | Count |
|--------|-------|
| Spec requirements (RF) | 15 |
| Spec success criteria (CS) | 8 |
| Plan tasks | 12 (+ release `v1.0.0` pelo orquestrador) |
| Forward coverage (RF → task) | 15/15 (100%) |
| Success criteria com verificação | 7/8 na primeira passagem, 8/8 após emenda |
| Reverse traceability (task → spec) | 12/12 (100%) |
| Constitution violations | 0 |

## Forward Traceability (Spec → Plan)

| Spec Item | Plan Task(s) | Status |
|-----------|-------------|--------|
| RF1 probe do gateway (TTL 60s, stale 1h, timeout 4s, fail-open) | Task 3 | ✅ |
| RF2 `gateway_budget` | Task 4 | ✅ |
| RF3 `gateway_tokens` | Task 4 | ✅ |
| RF4 `gateway_reset` | Task 4 | ✅ |
| RF5 `tokens_in/out/total/cache` | Task 5 | ✅ |
| RF6 `auth_mode` gateway | Task 6 | ✅ |
| RF7 preset `gateway` + refresh 60 | Tasks 7, 9 | ✅ |
| RF8 subcomando `budget` | Task 8 | ✅ |
| RF9 `install` grava `/budget` | Task 9 | ✅ |
| RF10 Studio | Task 10 | ✅ |
| RF11 formatação BRL/tokens | Task 1 | ✅ |
| RF12 release workflow | Task 11 | ✅ |
| RF13 `version` + bootstrap | Task 11 | ✅ |
| RF14 CI | Task 11 | ✅ |
| RF15 README + Confluence | Task 12 | ✅ |
| 5.3 parse tolerante + fixture | Task 2 | ✅ |
| 6 tratamento de erros | Tasks 3, 8 | ✅ (cache corrompido sem teste dedicado, coberto por `readGatewayCache` devolver nil) |
| CS1 instalação em 2 comandos | Tasks 11, 12 | ⚠️ verificação manual numa máquina limpa |
| CS2 7 dados do script | Tasks 4, 5, 7 (`TestGatewayPreset` + testes de render) | ✅ |
| CS3 zero deps em runtime | — | ⚠️ sem passo; orquestrador roda `grep -rn "exec.Command" --include=*.go` antes do release |
| CS4 no máximo 1 request/60s | Task 3 `fetches once and serves fresh cache without http` | ✅ |
| CS5 timeout < 5s, nada em stderr | — | ❌ **CRITICAL na 1ª passagem**: spec 7 pede "teste com servidor que dorme", plano não tinha |
| CS6 `/budget` em PT | Task 8 (`TestBudgetReportText`), Task 9 (conteúdo do comando) | ⚠️ execução real dentro do Claude Code é manual |
| CS7 5 binários + checksums | Task 11 | ⚠️ verificado pelo orquestrador após a tag |
| CS8 CI verde | Task 11 | ✅ |

## Reverse Traceability (Plan → Spec)

| Plan Task | Spec Item | Status |
|-----------|-----------|--------|
| Task 1 formatadores | RF11 | ✅ |
| Task 2 tipo + parse | 5.1, 5.3 | ✅ |
| Task 3 probe + config | RF1, 5.2, 6 | ✅ |
| Task 4 components gateway | RF2-4, 5.4 | ✅ (só graphite ganha cores; spec 5.4 dizia "os 5 themes". Deviação aceita e spec emendado: os outros 4 themes caem no `Default`, mesmo padrão dos components existentes) |
| Task 5 components tokens | RF5, 5.4 | ✅ |
| Task 6 auth_mode + render | RF6, 5.5 | ✅ |
| Task 7 preset + mocks | RF7, 5.6, 5.9 | ✅ (mock do `preview` e do server servem CS2) |
| Task 8 budget | RF8, 5.7 | ✅ |
| Task 9 install /budget | RF9, 5.8, 5.6 (refresh) | ✅ |
| Task 10 Studio | RF10, 5.9 | ✅ |
| Task 11 CI/release/bootstrap | RF12-14, 5.10 | ✅ |
| Task 12 docs | RF15, 5.11 | ✅ |

Sem gold plating: nenhuma task fora do spec.

## Clarification Coverage

| Category | Status | Gap |
|----------|--------|-----|
| Functional Scope | ✅ Covered | objetivo, fora de escopo, persona (dev do time no gateway) |
| Domain & Data | ✅ Covered | `GatewayUsage`, schema, estados do cache (fresco / stale / ausente) |
| Interaction & UX | ⚠️ Partial | jornadas e estados de erro cobertos; acessibilidade depende de cor + emoji (mesmo padrão do resto do statusline) |
| Non-Functional | ⚠️ Partial | performance, reliability, segurança do token cobertos; observabilidade ausente por decisão (constitution III proíbe stderr no render; `budget` é a via de diagnóstico) |

## Constitution Compliance

| Principle | Status | Detail |
|-----------|--------|--------|
| I. Zero deps em runtime | ✅ | Nenhum `exec.Command` novo. `install.sh`/`install.ps1` são bootstrap de download, fora do runtime |
| II. Engine único em Go | ✅ | Studio só envia `mock_input.gateway`; render fica no Go |
| III. Fail-open | ✅ | `ProbeGateway` engole erro; `context.WithTimeout` 4s; `cmdRender` sem stderr |
| IV. Nada sai da máquina / token nunca persistido | ✅ | cache guarda `usage` + `raw` (0600). ⚠️ `raw` pode trazer identificadores do usuário além do consumo; local e 0600, aceito |
| V. Component com meta no catálogo | ✅ | 7 metas novas, `NeedsGateway` |
| Forbidden: HTTP sem timeout | ✅ | — |
| Tech constraints | ✅ | stdlib, sem dependência nova |

## Internal Consistency

- File structure ↔ tasks: ✅ (tabela do plano bate com `Files:` de cada task).
- Nomes entre tasks: ✅ (`GatewayUsage`, `GatewayProbeResult`, `detectAuthMode(bool, *ProbeResult, bool)`, `WriteBudgetCommand`, `needs_gateway`).
- Dependências: ✅ lineares 1→2→3→(4,5)→6→7→8→9→10→11→12, sem ciclo.
- TDD: ✅ Tasks 1-9; Tasks 10-12 têm verificação por build/smoke/leitura (UI, CI, docs), aceito.
- Nit: teste da Task 3 reimplementava `strings.Contains` (`contains`/`indexOf`). Corrigido na emenda.

## Verdict (1ª passagem): BLOCK

### Critical Issues
1. CS5 sem verificação: adicionar à Task 3 um subteste com servidor que dorme e `Timeout: "50ms"`, afirmando `ErrGatewayUnreachable` e duração < 250ms.

### Warnings
1. CS1, CS6, CS7: verificação manual pelo orquestrador (máquina limpa, `/budget` real, assets da release). Registrar no relatório final.
2. CS3: orquestrador roda `grep -rn "exec.Command" --include=*.go .` antes da tag e confirma que só `git` e `openURL` aparecem.
3. Spec 5.4 "5 themes" vs plano "graphite": alinhar o spec (emenda de uma linha).
4. Helpers `contains`/`indexOf` no teste da Task 3: usar `strings.Contains`.

## Emenda aplicada em seguida (mesma data)

- Task 3: subteste `slow gateway respects timeout` adicionado; helpers trocados por `strings.Contains`.
- Spec 5.4: "os 5 themes ganham entries" → "o theme graphite ganha entries; os demais usam `Default`".

## Verdict (2ª passagem, após emenda): WARN

0 critical, warnings de verificação manual (CS1, CS3, CS6, CS7) registradas pro orquestrador. Liberado pra `/phase-gate` pós-plano.
