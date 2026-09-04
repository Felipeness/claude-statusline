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
