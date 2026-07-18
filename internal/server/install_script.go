package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// installScript is the POSIX installer served at /install.sh. It downloads the
// agent binary from THIS server (so it works self-hosted / offline), writes a
// starter agent.yaml seeded with MC_SERVER_URL / MC_ENROLL_TOKEN, and runs it.
//
// The literal "__BASE__" is replaced per-request with the server's public base.
const installScriptTmpl = `#!/usr/bin/env sh
set -eu
BASE="${MC_SERVER_URL:-__BASE__}"
# Normalize ws(s) -> http(s) for downloads.
case "$BASE" in
  ws://*)  BASE="http://${BASE#ws://}" ;;
  wss://*) BASE="https://${BASE#wss://}" ;;
esac
BIN="mission-control-agent"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${HOME}/.mission-control"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os (use install.ps1 on Windows)" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

# Guard: refuse to install/start a second agent on this machine. Running two
# agents (especially sharing one identity) makes them fight over the same
# server slot and the dashboard flaps. Set MC_FORCE=1 to override.
if [ -z "${MC_FORCE:-}" ]; then
  running=""
  if command -v pgrep >/dev/null 2>&1; then
    running="$(pgrep -f "${BIN} --config" 2>/dev/null || true)"
  else
    running="$(ps ax 2>/dev/null | grep "${BIN} --config" | grep -v grep || true)"
  fi
  if [ -n "$running" ]; then
    echo "error: a mission-control agent is already running on this machine:" >&2
    echo "$running" | sed 's/^/  /' >&2
    echo "" >&2
    echo "Only one agent per machine is supported. To reinstall/restart, first stop it:" >&2
    echo "  pkill -f '${BIN} --config'" >&2
    echo "then re-run this installer. Or set MC_FORCE=1 to override (not recommended)." >&2
    exit 1
  fi
fi

# Portable downloader: prefer curl, fall back to wget (minimal Ubuntu often ships
# only one of them).
download() { # download <url> <dest>
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    echo "need curl or wget to download the agent" >&2
    return 1
  fi
}

url="${BASE}/download/${BIN}-${os}-${arch}"
echo "Installing ${BIN} (${os}/${arch}) from ${url}..."
tmp="$(mktemp)"
if ! download "$url" "$tmp"; then
  echo "download failed: $url" >&2
  echo "The server has no binary for ${os}/${arch}. On the server, build them with:" >&2
  echo "  make agents   # then restart the server with MC_AGENT_BIN_DIR=bin/agents" >&2
  rm -f "$tmp"
  exit 1
fi
chmod +x "$tmp"
if [ -w "$INSTALL_DIR" ]; then mv "$tmp" "${INSTALL_DIR}/${BIN}"; else sudo mv "$tmp" "${INSTALL_DIR}/${BIN}"; fi

mkdir -p "$CONFIG_DIR"
CFG="${CONFIG_DIR}/agent.yaml"
if [ ! -f "$CFG" ]; then
  cat > "$CFG" <<EOF
serverUrl: "${MC_SERVER_URL:-__BASE__}"
enrollToken: "${MC_ENROLL_TOKEN:-}"
apiKey: ""
providers:
  - "claude-code"
  - "codex"
  - "gemini"
discoverEverySeconds: 5
metricsEverySeconds: 3
heartbeatEverySeconds: 10
logLevel: "info"
EOF
  echo "Wrote ${CFG}"
fi

if [ -n "${MC_ENROLL_TOKEN:-}" ]; then
  echo "Starting agent to enroll..."
  exec "${INSTALL_DIR}/${BIN}" --config "$CFG"
fi
echo "Done. Start with:"
echo "  ${BIN} --config ${CFG}"
`

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	script := strings.ReplaceAll(installScriptTmpl, "__BASE__", s.publicBase(r))
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(strings.TrimLeft(script, "\n")))
}

// installPowershellTmpl is the Windows installer served at /install.ps1.
const installPowershellTmpl = `$ErrorActionPreference = 'Stop'
$Base = if ($env:MC_SERVER_URL) { $env:MC_SERVER_URL } else { '__BASE__' }
$Base = $Base -replace '^ws://','http://' -replace '^wss://','https://'
$Bin = 'mission-control-agent.exe'
$InstallDir = Join-Path $env:LOCALAPPDATA 'MissionControl'
$ConfigDir = Join-Path $env:USERPROFILE '.mission-control'

# Guard: refuse to install/start a second agent on this machine. Set MC_FORCE=1
# to override.
if (-not $env:MC_FORCE) {
  $existing = Get-Process -Name 'mission-control-agent' -ErrorAction SilentlyContinue
  if ($existing) {
    Write-Error "a mission-control agent is already running on this machine (PID $($existing.Id -join ', '))."
    Write-Host "Only one agent per machine is supported. To reinstall/restart, first stop it:"
    Write-Host "  Stop-Process -Name mission-control-agent"
    Write-Host "then re-run this installer. Or set MC_FORCE=1 to override (not recommended)."
    exit 1
  }
}

# Detect CPU architecture (Windows on ARM reports ARM64).
$archRaw = $env:PROCESSOR_ARCHITECTURE
if (-not $archRaw) { $archRaw = (Get-CimInstance Win32_Processor | Select-Object -First 1).Architecture }
switch -Regex ("$archRaw") {
  'ARM64|^12$' { $arch = 'arm64' }
  'AMD64|x86_64|^9$' { $arch = 'amd64' }
  default { $arch = 'amd64' }
}

$url = "$Base/download/mission-control-agent-windows-$arch.exe"
Write-Host "Installing $Bin (windows/$arch) from $url..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir $Bin
try {
  Invoke-WebRequest -Uri $url -OutFile $target -UseBasicParsing
} catch {
  Write-Error "download failed: $url"
  Write-Host "The server has no binary for windows/$arch. On the server run 'make agents' and restart it with MC_AGENT_BIN_DIR=bin/agents."
  exit 1
}

New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
$cfg = Join-Path $ConfigDir 'agent.yaml'
if (-not (Test-Path $cfg)) {
  $token = if ($env:MC_ENROLL_TOKEN) { $env:MC_ENROLL_TOKEN } else { '' }
  $srv = if ($env:MC_SERVER_URL) { $env:MC_SERVER_URL } else { '__BASE__' }
  @"
serverUrl: "$srv"
enrollToken: "$token"
apiKey: ""
providers:
  - "claude-code"
  - "codex"
  - "gemini"
discoverEverySeconds: 5
metricsEverySeconds: 3
heartbeatEverySeconds: 10
logLevel: "info"
"@ | Set-Content -Path $cfg -Encoding UTF8
  Write-Host "Wrote $cfg"
}

if ($env:MC_ENROLL_TOKEN) {
  Write-Host "Starting agent to enroll..."
  & $target --config $cfg
} else {
  Write-Host "Done. Start with: $target --config $cfg"
}
`

func (s *Server) handleInstallPowershell(w http.ResponseWriter, r *http.Request) {
	script := strings.ReplaceAll(installPowershellTmpl, "__BASE__", s.publicBase(r))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(script))
}

// handleDownloadBinary serves prebuilt agent binaries from AgentBinDir at
// /download/<name>. Names are restricted to mission-control-agent-<os>-<arch>
// (optionally .exe) to prevent path traversal or serving arbitrary files.
func (s *Server) handleDownloadBinary(w http.ResponseWriter, r *http.Request, name string) {
	if s.cfg.AgentBinDir == "" {
		http.Error(w, "no agent binaries configured on this server", http.StatusNotFound)
		return
	}
	if !isAllowedBinaryName(name) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(s.cfg.AgentBinDir, name)
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		http.Error(w, "binary not found for this platform", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, full)
}

func isAllowedBinaryName(name string) bool {
	if !strings.HasPrefix(name, "mission-control-agent-") {
		return false
	}
	// No path separators or traversal.
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	return true
}
