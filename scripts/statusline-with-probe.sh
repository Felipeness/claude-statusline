#!/usr/bin/env bash
# Wrapper do claude-statusline: probe a quota OAuth e injeta no stdin antes
# de passar pro binario, pra status line sempre mostrar 5h/7d (mesmo quando
# Claude Code esta em API key mode e nao envia rate_limits).

set -u

CACHE="/tmp/claude-statusline-probe-$UID.json"
TTL=30
CREDS="$HOME/.claude/.credentials.json"
BIN="$HOME/.local/bin/claude-statusline"

input=$(cat)

now=$(date +%s)
need_refresh=1
if [[ -f "$CACHE" ]]; then
  cached_at=$(stat -c %Y "$CACHE" 2>/dev/null || echo 0)
  (( now - cached_at < TTL )) && need_refresh=0
fi

if (( need_refresh )) && [[ -r "$CREDS" ]] \
   && command -v jq >/dev/null 2>&1 && command -v curl >/dev/null 2>&1; then
  tok=$(jq -r '.claudeAiOauth.accessToken // empty' "$CREDS" 2>/dev/null)
  if [[ -n "$tok" ]]; then
    body=$(curl -sS -m 3 \
      -H "Authorization: Bearer $tok" \
      -H "anthropic-beta: oauth-2025-04-20" \
      -H "User-Agent: claude-cli/2.1.123" \
      "https://api.anthropic.com/api/oauth/usage" 2>/dev/null)
    if printf '%s' "$body" | jq -e '.five_hour' >/dev/null 2>&1; then
      printf '%s' "$body" > "$CACHE"
    fi
  fi
fi

if [[ -s "$CACHE" ]]; then
  # claude-statusline espera resets_at como unix epoch (int).
  # /api/oauth/usage devolve ISO string. Converte.
  injected=$(jq --argjson probe "$(<"$CACHE")" '
    def to_epoch: if . == null or . == "" then 0
                  else (sub("\\.[0-9]+"; "") | sub("\\+00:00"; "Z") | fromdateiso8601)
                  end;
    .rate_limits = {
      five_hour: ($probe.five_hour | if . then {
        used_percentage: .utilization,
        resets_at: (.resets_at | to_epoch)
      } else null end),
      seven_day: ($probe.seven_day | if . then {
        used_percentage: .utilization,
        resets_at: (.resets_at | to_epoch)
      } else null end)
    }' <<<"$input" 2>/dev/null)
  [[ -n "$injected" ]] && input="$injected"
fi

# Chip [OAuth X%] ou [API key]. Determina o modo a partir do util cacheado:
# >=90 OU env tem ANTHROPIC_API_KEY -> API; senao -> OAuth.
util=""
if [[ -s "$CACHE" ]]; then
  util=$(jq -r '.five_hour.utilization // empty' "$CACHE" 2>/dev/null)
fi

if [[ -n "${ANTHROPIC_API_KEY:-}" ]] \
   || { [[ -n "$util" ]] && awk -v u="$util" 'BEGIN{exit !(u+0 >= 90)}'; }; then
  chip=$'\033[1;33m[API key]\033[0m'  # amarelo bold
else
  chip=$'\033[1;32m[OAuth]\033[0m'    # verde bold
fi

printf '%s ' "$chip"
# Injeta labels: "API: $X" e "OAuth 5h X% / 7d X%"
printf '%s' "$input" | "$BIN" render \
  | sed -E 's/(\x1b\[[0-9;:]*m)\$/\1API: $/g; s/(\x1b\[[0-9;:]*m)5h /\1OAuth 5h /g'
