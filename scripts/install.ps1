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

function Convert-ToControlUrl {
  param([Parameter(Mandatory = $true)][string]$InputUrl)

  $value = $InputUrl.Trim().TrimEnd('/')
  if ([string]::IsNullOrWhiteSpace($value)) {
    throw "Control URL cannot be empty."
  }

  if ($value -match '^wss?://') {
    $url = $value
  } elseif ($value -match '^https://') {
    $url = "wss://" + $value.Substring(8)
  } elseif ($value -match '^http://') {
    $url = "ws://" + $value.Substring(7)
  } else {
    $url = "wss://$value"
  }

  if (-not $url.EndsWith('/ws/agent')) {
    $url = "$url/ws/agent"
  }
  return $url
}

function Read-NonEmptyLine {
  param([Parameter(Mandatory = $true)][string]$Prompt)
  while ($true) {
    $value = Read-Host $Prompt
    if (-not [string]::IsNullOrWhiteSpace($value)) {
      return $value.Trim()
    }
    Write-Host "$Prompt cannot be empty." -ForegroundColor Yellow
  }
}

function Read-Token {
  param([Parameter(Mandatory = $true)][string]$Prompt)
  while ($true) {
    $secure = Read-Host $Prompt -AsSecureString
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
      $value = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
    } finally {
      [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }

    if (-not [string]::IsNullOrWhiteSpace($value)) {
      return $value.Trim()
    }
    Write-Host "$Prompt cannot be empty." -ForegroundColor Yellow
  }
}

function Resolve-ClaudePath {
  $which = Get-Command which -ErrorAction SilentlyContinue
  if ($which) {
    $whichOutput = & which claude 2>$null
    foreach ($line in $whichOutput) {
      if (-not [string]::IsNullOrWhiteSpace($line)) {
        $candidate = $line.Trim()
        if ($candidate -match 'aliased to') {
          $aliasTarget = ($candidate -replace "^.*aliased to\s+", "").Trim("'")
          if ($aliasTarget -notmatch '\s') {
            $resolved = Get-Command $aliasTarget -ErrorAction SilentlyContinue
            if ($resolved -and $resolved.Source) {
              return $resolved.Source
            }
          }
          continue
        }
        if (Test-Path $candidate) {
          return (Resolve-Path $candidate).Path
        }
      }
    }
  }

  $command = Get-Command claude -ErrorAction SilentlyContinue
  if (-not $command) {
    return $null
  }

  while ($command -and $command.CommandType -eq "Alias") {
    $nextName = $command.Definition.Split(' ')[0]
    $command = Get-Command $nextName -ErrorAction SilentlyContinue
  }

  if ($command -and $command.Source) {
    return $command.Source
  }
  return $null
}

function Build-RunCommand {
  param(
    [Parameter(Mandatory = $true)][string]$ControlUrl,
    [Parameter(Mandatory = $true)][string]$ServerId,
    [Parameter(Mandatory = $true)][string]$AllowRoot,
    [string]$ClaudePath
  )
  $cmd = "cc-agent.exe -control-url `"$ControlUrl`" -agent-token <YOUR_TOKEN> -server-id `"$ServerId`" -allow-root `"$AllowRoot`""
  if ($ClaudePath) {
    $cmd += " -claude-path `"$ClaudePath`""
  }
  return $cmd
}

$platform = Get-Platform
Write-Host "==> Agent Control installer"
Write-Host "==> Platform: $platform"

New-Item -ItemType Directory -Force -Path $INSTALL_DIR | Out-Null
Install-Binary -Binary "cc-agent" -Platform $platform
Install-Binary -Binary "cc-chat-claude" -Platform $platform

$controlInput = Read-NonEmptyLine -Prompt "Control URL (e.g. https://cc-remote.app)"
$CONTROL_URL = Convert-ToControlUrl -InputUrl $controlInput
$AGENT_TOKEN = Read-Token -Prompt "Agent Token"

$CLAUDE_PATH = Resolve-ClaudePath
if ($CLAUDE_PATH) {
  Write-Host "==> Claude detected: $CLAUDE_PATH"
} else {
  Write-Host "==> Claude not found via 'which claude' / command lookup." -ForegroundColor Yellow
  Write-Host "    Install Claude CLI, or run later with -claude-path C:\path\to\claude.exe" -ForegroundColor Yellow
}

$serverHost = if ($env:COMPUTERNAME) { $env:COMPUTERNAME } else { "windows" }
$SERVER_ID = if ($env:SERVER_ID) { $env:SERVER_ID } else { ("srv-" + ($serverHost.ToLower() -replace '[^a-z0-9]+', '-')) }
$ALLOW_ROOT = if ($env:ALLOW_ROOT) { $env:ALLOW_ROOT } else { (Get-Location).Path }
$agentExe = Join-Path $INSTALL_DIR "cc-agent.exe"
$env:Path = "$INSTALL_DIR;$env:Path"

Write-Host ""
Write-Host "Reusable command:"
Write-Host ("  " + (Build-RunCommand -ControlUrl $CONTROL_URL -ServerId $SERVER_ID -AllowRoot $ALLOW_ROOT -ClaudePath $CLAUDE_PATH))
Write-Host ""
Write-Host "Add to PATH (optional):" -ForegroundColor Yellow
Write-Host "  [Environment]::SetEnvironmentVariable('Path', `$env:Path + ';$INSTALL_DIR', 'User')"
Write-Host "Docs: https://github.com/$GITHUB_REPO/blob/main/docs/getting-started.md"
Write-Host ""
Write-Host "==> Launching cc-agent..."

$args = @(
  "-control-url", $CONTROL_URL,
  "-agent-token", $AGENT_TOKEN,
  "-server-id", $SERVER_ID,
  "-allow-root", $ALLOW_ROOT
)
if ($CLAUDE_PATH) {
  $args += @("-claude-path", $CLAUDE_PATH)
}

& $agentExe @args
