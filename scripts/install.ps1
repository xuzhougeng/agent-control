# agent-control cc-agent installer (Windows)
# Usage: irm https://cc-remote.app/install.ps1 | iex
#    or: irm https://raw.githubusercontent.com/xuzhougeng/agent-control/main/scripts/install.ps1 | iex
$ErrorActionPreference = 'Stop'

$GITHUB_REPO = if ($env:GITHUB_REPO) { $env:GITHUB_REPO } else { "xuzhougeng/agent-control" }
$VERSION = if ($env:VERSION) { $env:VERSION } else { "latest" }
$INSTALL_DIR = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "cc-agent\bin" }

function Get-Platform {
  $arch = switch -Wildcard ($env:PROCESSOR_ARCHITECTURE) { "ARM64" { "arm64" } default { "amd64" } }
  "windows-$arch"
}

function Get-DownloadUrl {
  param([string]$Binary, [string]$Platform)
  if ($VERSION -eq "latest") {
    "https://github.com/$GITHUB_REPO/releases/latest/download/$Binary-$Platform.exe"
  } else {
    "https://github.com/$GITHUB_REPO/releases/download/$VERSION/$Binary-$Platform.exe"
  }
}

function Install-Binary {
  param([string]$Binary, [string]$Platform)
  $url = Get-DownloadUrl -Binary $Binary -Platform $Platform
  $dest = Join-Path $env:TEMP "$Binary-$Platform.exe"
  Write-Host "==> Downloading $url"
  try {
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
  } catch {
    Write-Host "Download failed. Ensure a release exists with asset: $Binary-$Platform.exe" -ForegroundColor Red
    Write-Host "See: https://github.com/$GITHUB_REPO/releases" -ForegroundColor Red
    exit 1
  }
  $final = Join-Path $INSTALL_DIR "$Binary.exe"
  Copy-Item -Path $dest -Destination $final -Force
  Remove-Item -Path $dest -Force -ErrorAction SilentlyContinue
  Write-Host "==> Installed: $final"
}

$platform = Get-Platform
Write-Host "==> Agent Control installer"
Write-Host "==> Platform: $platform"

New-Item -ItemType Directory -Force -Path $INSTALL_DIR | Out-Null
Install-Binary -Binary "cc-agent" -Platform $platform
Install-Binary -Binary "cc-chat-claude" -Platform $platform
Write-Host ""
Write-Host "Next: get an agent token from your control plane, then run:"
Write-Host "  cc-agent.exe -control-url wss://YOUR_CONTROL/ws/agent -agent-token YOUR_TOKEN -server-id srv-01 -allow-root C:\projects -claude-path C:\path\to\claude.exe"
Write-Host ""
$path = [Environment]::GetEnvironmentVariable("Path", "User")
if ($path -notlike "*cc-agent*") {
  Write-Host "Add to PATH (optional):" -ForegroundColor Yellow
  Write-Host "  [Environment]::SetEnvironmentVariable('Path', `$env:Path + ';$INSTALL_DIR', 'User')"
}
Write-Host "Docs: https://github.com/$GITHUB_REPO/blob/main/docs/getting-started.md"
