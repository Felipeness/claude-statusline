# claude-statusline

> A custom statusline for [Claude Code](https://claude.ai/code) **with a visual editor in the browser**.

```
~/Desktop/Projects/my-app  feat/CC-1234✱  Opus 4.7  ▓▓░░░░ 42%  $0.32  5h:73%
```

Configure 16 components, 5 themes, 3 styles (plain · powerline · capsule), thresholds per metric — all from a drag-and-drop web editor. Single Go binary, ~11MB. No dependencies.

## Why

Claude Code's default statusline shows `branch · model · mode`. Other tools (`ccstatusline`, `claude-powerline`) give you more, but configuring them is a JSON dance. **Studio** (open in your browser) lets you compose, theme, threshold, and preview live with mock data.

[Inspired by Powerline Studio for vim, ported to Claude Code.]

## Install

```bash
# 1. Build
git clone https://github.com/felipeness/claude-statusline ~/.local/src/claude-statusline
cd ~/.local/src/claude-statusline
cd web && bun install && bun run build && cd ..
go build -o ~/.local/bin/claude-statusline .

# 2. Plug into Claude Code (backups settings.json automatically)
claude-statusline install --preset compact
# If you have another statusline already: --force

# 3. Restart Claude Code (statusLine only loads on boot)
```

## Customize visually

```bash
claude-statusline studio        # opens http://localhost:5556 in browser
```

In the Studio:

- **Theme picker** — graphite, nord, dracula, sakura, mono
- **Style picker** — plain · powerline (Nerd Font) · capsule
- **Lines** — drag-and-drop chips, multi-line support, per-line separator
- **Threshold editor** — click the ⚙ on any chip with `has_warn_at` to tune warn/critical for severity colors
- **Mock data** — sliders to simulate `context %`, `cost USD`, `burn rate`, `rate 5h/7d %` and see the bar react live
- **Reset to preset** — compact / max / powerline

Save persists to `~/.claude-statusline/config.toml`. Reload Claude Code to apply.

## CLI

```bash
claude-statusline render             # consumed by Claude Code via stdin
claude-statusline install [--preset compact|max|powerline] [--force] [--uninstall]
claude-statusline preview --all      # all 15 theme×style combos in your terminal
claude-statusline studio [--port 5556]
```

## Components

| Component | Category | Shows |
|---|---|---|
| `cwd` | path | Current path shortened with `~` |
| `git` | git | Branch + dirty marker (`✱`) + ahead/behind (`↑1↓2`) |
| `ticket` | git | Auto-extracts `TICKET-NNNN` from branch (Jira/Linear) |
| `model` | model | Display name (e.g., "Opus 4.7") |
| `vim_mode` | system | NORMAL / INSERT |
| `context_pct` | context | Bar `▓▓░░░░ 42%` with severity color |
| `cost_session` | cost | `$X.XX` with optional `(N×p90)` badge |
| `burn_rate` | cost | Tokens/min with rising arrow `⬆` |
| `cost_today` | cost | Today's accumulated cost (needs daemon) |
| `cost_month` | cost | Monthly total + projection (needs daemon) |
| `lines_changed` | git | `+45/-12` lines |
| `rate_5h` | limits | % of 5h block + reset countdown |
| `rate_7d` | limits | % of 7-day block |
| `cluster` | history | AI-clustered session label (needs daemon) |
| `time` | system | `hh:mm` |
| `mcp_status` | system | MCP servers health (placeholder) |

## Severity (color) system

Components flagged `has_warn_at` react with color:

| Severity | Color | Trigger |
|---|---|---|
| **OK** | green | `value < warn_at` |
| **Warn** | amber | `warn_at ≤ value < critical_at` |
| **Crit** | red | `value ≥ critical_at` |

Defaults (configurable in Studio ⚙):

- `context_pct`: warn=50, critical=80 (% of context window)
- `cost_session`: warn=0.8, critical=1.2 (multiplier of historical p90 — needs daemon)
- `burn_rate`: warn=1500, critical=3000 (tokens/min)
- `rate_5h` / `rate_7d`: warn=70, critical=90 (% of block)

## Optional: history daemon

Some components (`cost_today`, `cost_month`, `cluster`, `cost_session` p90 badge, `burn_rate`) need a sidecar daemon that aggregates your past sessions. We support [`claude-history`](https://github.com/felipeness/claude-history) — point at it via:

```toml
# ~/.claude-statusline/config.toml
[history]
endpoint = "http://localhost:5555"
timeout = "80ms"
```

Without a daemon, those components render nothing — but everything else (cwd, git, model, context %, cost session, rate limits) works from stdin alone.

## Themes & styles

5 themes × 3 styles = 15 combinations. Run `claude-statusline preview --all` to see them all in your terminal at once.

**Style notes:**

- `plain` — separator between segments (`│`). Works in any terminal.
- `powerline` — pill segments with smooth color transitions (` arrow). Requires a [Nerd Font](https://www.nerdfonts.com/).
- `capsule` — independent pills with rounded edges (` ` ` `). Requires a Nerd Font.

## Architecture

```
claude-statusline/
├── main.go                       # 4 CLI subcommands
├── embed.go                      # //go:embed all:web/dist
├── internal/
│   ├── statusline/
│   │   ├── input.go              # JSON shape of Claude Code stdin
│   │   ├── config.go             # TOML config + defaults
│   │   ├── theme.go              # 5 themes embedded
│   │   ├── ansi.go               # truecolor helpers
│   │   ├── components.go         # 16 components with metadata
│   │   ├── render.go             # plain/powerline/capsule renderers
│   │   ├── html.go               # ANSI → HTML (for Studio preview)
│   │   ├── history.go            # optional daemon fetch (best-effort)
│   │   ├── presets.go            # compact/max/powerline
│   │   └── install.go            # atomic settings.json merge
│   └── server/
│       └── server.go             # 5 endpoints powering the Studio
└── web/                          # Vite + React + Tailwind v4
    └── src/
        ├── App.tsx               # the Studio
        ├── api.ts
        └── types.ts
```

**Single source of truth**: render engine lives in Go. Studio web sends `{config, mock_input, mock_history}` via POST and gets back `{ansi, html}` (Go converts ANSI to HTML — no JS rendering lib). What you see in the Studio is exactly what Claude Code will see.

## Tech

**Backend**: Go 1.26 · [BurntSushi/toml](https://github.com/BurntSushi/toml) · stdlib only.
**Frontend**: Vite 8 · React 19 · TypeScript · Tailwind v4 · [@dnd-kit](https://dndkit.com/) for drag-drop.

## Privacy

Runs entirely local. The Studio binds to `127.0.0.1:5556`. Statusline render reads stdin from Claude Code, optionally pings a local daemon for history. Nothing leaves your machine.

## License

MIT
