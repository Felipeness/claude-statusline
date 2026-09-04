# claude-statusline Constitution

**Version**: 1.0 | **Created**: 2026-09-03 | **Last Amended**: 2026-09-03

## Core Principles

### I. Binário único, zero dependências em runtime
Tudo que o statusline precisa pra renderizar vive dentro do binário Go. Nenhum `jq`, `python`, `node` ou script shell é chamado em runtime.

**Rationale**: o valor do projeto sobre os concorrentes é instalar um arquivo e pronto. Cada dependência externa vira um "Permission denied" ou "command not found" na máquina de alguém do time.
**Violation policy**: BLOCK

### II. Engine único em Go
O render no terminal e a preview no Studio saem do mesmo código (`internal/statusline`). O frontend só envia `{config, mock}` e exibe o HTML que o Go devolve.

**Rationale**: qualquer lógica duplicada em TypeScript diverge do terminal em semanas. O que a preview mostra tem que ser o que o Claude Code vê.
**Violation policy**: BLOCK

### III. Fail-open, sempre
O `render` nunca falha, nunca trava e nunca escreve em stderr. Toda chamada externa (daemon, OAuth, gateway, git) tem timeout explícito e, em erro, o component some ou usa cache stale.

**Rationale**: o statusline roda a cada turno do Claude Code. Um erro visível ou um hang de 30s quebra a experiência inteira do editor.
**Violation policy**: BLOCK

### IV. Nada sai da máquina sem o usuário mandar
Só chamamos endpoints locais ou os que o próprio usuário configurou (daemon de histórico, API da Anthropic, gateway da empresa). Tokens nunca vão pra log, cache em disco ou stdout.

**Rationale**: o binário lê credenciais do Claude Code (`.credentials.json`, cache do Auth0). Um vazamento aqui compromete a conta do usuário.
**Violation policy**: BLOCK

### V. Component é um chip independente com metadata no catálogo
Todo component se registra no `registry`, tem entry em `componentMetas` (label, categoria, badges) e renderiza sozinho a partir do `RenderCtx`. Sem estado compartilhado entre components.

**Rationale**: o Studio (drag-and-drop, thresholds, badges) é gerado do catálogo. Um component fora do padrão fica invisível ou quebra a UI.
**Violation policy**: WARN

## Forbidden Patterns

| Pattern | Why Forbidden | Exception |
|---------|--------------|-----------|
| Chamar CLI externa pra obter dados (jq, python, curl, claude) | Viola I e III | `git` (best-effort, com timeout, já existente) |
| Lógica de render em TypeScript | Viola II | none |
| Chamada HTTP sem `context.WithTimeout` | Viola III | none |
| Gravar token/credencial em cache de disco ou output | Viola IV | none |
| `panic`, `os.Exit` ou stderr dentro do path de `render` | Viola III | `install`, `studio`, `budget` podem falhar com mensagem |
| Component que renderiza a partir de estado global mutável | Viola V | caches de probe protegidos por mutex |

## Quality Attribute Priorities

1. **Confiabilidade** — o statusline nunca pode piorar a sessão do Claude Code. Em dúvida, esconder o component.
2. **Latência** — render típico abaixo de 50ms. Rede só com cache em disco e TTL; nunca bloquear o turno esperando resposta.
3. **DX de instalação** — um binário, um comando (`install --preset X`), reiniciar o Claude Code. Sem editar JSON na mão.

## Technology Constraints

| Area | Decision | Status | Rationale |
|------|----------|--------|-----------|
| Language (engine + CLI) | Go 1.26, stdlib + `BurntSushi/toml` + `x/term` | Non-negotiable | Binário único, cross-compile trivial |
| Studio frontend | Vite + React 19 + Tailwind v4 | Flexible | Só UI; embarcado via `go:embed` no build |
| Build do frontend | Bun | Flexible | Build-time only, nunca runtime |
| Config | TOML em `~/.claude-statusline/config.toml` | Non-negotiable | Tipado, legível, editável pelo Studio |
| Testes | `go test`, table-driven, colocados ao lado do source | Non-negotiable | Padrão global do Felipe |

## Operational Boundaries

- **Max binários/pacotes**: 1 binário; `internal/statusline` (engine) + `internal/server` (Studio)
- **Latency budget**: render < 50ms típico, < 100ms com cache hit de probe; timeouts de rede entre 80ms (daemon local) e 4s (gateway)
- **Uptime SLA**: N/A (ferramenta local)
- **Data residency**: 100% local; único tráfego é para endpoints configurados pelo usuário

## Amendment Log

| Date | Change | Rationale | Author |
|------|--------|-----------|--------|
| 2026-09-03 | Initial constitution | Derivada do README antes da feature de budget do LLM Gateway | Felipe (via Claude) |
