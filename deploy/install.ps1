# Mission Control agent installer for Windows (PowerShell).
# Usage:
#   irm https://raw.githubusercontent.com/avi-pathak/mission-control.ai/main/deploy/install.ps1 | iex
#
# Env: MC_SERVER_URL, MC_API_KEY, MC_VERSION

$ErrorActionPreference = 'Stop'

$Repo = 'avi-pathak/mission-control.ai'
$Bin = 'mission-control-agent.exe'
$InstallDir = Join-Path $env:LOCALAPPDATA 'MissionControl'
$ConfigDir = Join-Path $env:USERPROFILE '.mission-control'

$arch = if ([Environment]::Is64BitOperatingSystem) { 'amd64' } else { '386' }
$version = if ($env:MC_VERSION) { $env:MC_VERSION } else { 'latest' }

Write-Host "Installing $Bin (windows/$arch, $version)..."

if ($version -eq 'latest') {
  $url = "https://github.com/$Repo/releases/latest/download/mission-control-agent-windows-$arch.exe"
} else {
  $url = "https://github.com/$Repo/releases/download/$version/mission-control-agent-windows-$arch.exe"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir $Bin
Invoke-WebRequest -Uri $url -OutFile $target

# Add to user PATH if missing.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
  Write-Host "Added $InstallDir to PATH (restart your shell)."
}

# Starter config.
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
$cfg = Join-Path $ConfigDir 'agent.yaml'
if (-not (Test-Path $cfg)) {
  $serverUrl = if ($env:MC_SERVER_URL) { $env:MC_SERVER_URL } else { 'ws://localhost:8080' }
  $apiKey = if ($env:MC_API_KEY) { $env:MC_API_KEY } else { '' }
  $enrollToken = if ($env:MC_ENROLL_TOKEN) { $env:MC_ENROLL_TOKEN } else { '' }
  @"
serverUrl: "$serverUrl"
enrollToken: "$enrollToken"
apiKey: "$apiKey"
providers:
  - "claude-code"
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
  exit $LASTEXITCODE
}

Write-Host "Done. Start with:"
Write-Host "  $target --config $cfg"
