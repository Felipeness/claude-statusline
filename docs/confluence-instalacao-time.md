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
