#!/usr/bin/env sh
# Instala a última release do claude-statusline e pluga no Claude Code.
# Uso: curl -fsSL https://raw.githubusercontent.com/Felipeness/claude-statusline/main/install.sh | sh
#      ... | sh -s -- --preset compact   (default: gateway)
set -eu

REPO="Felipeness/claude-statusline"
PRESET="gateway"
while [ $# -gt 0 ]; do
  case "$1" in
    --preset) [ $# -ge 2 ] || { echo "--preset precisa de um valor" >&2; exit 1; }; PRESET="$2"; shift 2 ;;
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
if [ "$ext" = zip ]; then
  if command -v unzip >/dev/null 2>&1; then
    unzip -q "$tmp/pkg.zip" -d "$tmp"
  elif command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "Expand-Archive -LiteralPath '$(cygpath -w "$tmp/pkg.zip")' -DestinationPath '$(cygpath -w "$tmp")' -Force"
  else
    echo "sem unzip nem powershell.exe pra extrair — roda o install.ps1 em vez deste script" >&2
    exit 1
  fi
else
  tar -xzf "$tmp/pkg.tar.gz" -C "$tmp"
fi
mkdir -p "$bin_dir"
cp "$tmp/$bin" "$bin_dir/$bin"
chmod 755 "$bin_dir/$bin"

echo "instalado em $bin_dir/$bin"
"$bin_dir/$bin" install --preset "$PRESET" --force
