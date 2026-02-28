param(
  [int]$ControlPort = 18080,
  [string]$AdminToken = "admin-dev-token",
  [string]$ServerID = "srv-win",
  [string]$ServerIDPrefix = "",
  [string]$AllowRoot = "",
  [string]$ClaudePath = "",
  [string]$ChatProfileFile = "",
  $StartAgent = $true
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$bundleRoot = (Resolve-Path $PSScriptRoot).Path
if ([string]::IsNullOrWhiteSpace($AllowRoot)) {
  $AllowRoot = $bundleRoot
}
if ([string]::IsNullOrWhiteSpace($ServerIDPrefix)) {
  $ServerIDPrefix = $ServerID
}
if ([string]::IsNullOrWhiteSpace($ServerIDPrefix)) {
  $ServerIDPrefix = "srv-win"
}

$ptyServerID = "$ServerIDPrefix-pty"
$chatClaudeServerID = "$ServerIDPrefix-chat-claude"
$chatEchoServerID = "$ServerIDPrefix-chat-echo"

$binDir = Join-Path $bundleRoot "bin"
$uiDir = Join-Path $bundleRoot "cc-web"
$runDir = Join-Path $bundleRoot "run"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

$controlExe = Join-Path $binDir "cc-control.exe"
$agentExe = Join-Path $binDir "cc-agent.exe"
$chatEchoWorkerExe = Join-Path $binDir "cc-chat-echo.exe"
$chatClaudeWorkerExe = Join-Path $binDir "cc-chat-claude.exe"

$controlLogOut = Join-Path $runDir "cc-control.stdout.log"
$controlLogErr = Join-Path $runDir "cc-control.stderr.log"
$ptyAgentLogOut = Join-Path $runDir "cc-agent-pty.stdout.log"
$ptyAgentLogErr = Join-Path $runDir "cc-agent-pty.stderr.log"
$chatClaudeAgentLogOut = Join-Path $runDir "cc-agent-chat-claude.stdout.log"
$chatClaudeAgentLogErr = Join-Path $runDir "cc-agent-chat-claude.stderr.log"
$chatEchoAgentLogOut = Join-Path $runDir "cc-agent-chat-echo.stdout.log"
$chatEchoAgentLogErr = Join-Path $runDir "cc-agent-chat-echo.stderr.log"
$resultPath = Join-Path $runDir "tokens-and-process.json"

$controlProc = $null
$ptyAgentProc = $null
$chatClaudeAgentProc = $null
$chatEchoAgentProc = $null
$resolvedClaudePath = ""

function Convert-ToBool {
  param(
    [Parameter(Mandatory = $true)]$Value
  )
  if ($Value -is [bool]) {
    return [bool]$Value
  }
  if ($Value -is [int] -or $Value -is [long]) {
    return ([int64]$Value) -ne 0
  }
  $text = [string]$Value
  $normalized = $text.Trim().ToLowerInvariant()
  switch ($normalized) {
    "1" { return $true }
    "0" { return $false }
    "true" { return $true }
    "false" { return $false }
    "`$true" { return $true }
    "`$false" { return $false }
    "yes" { return $true }
    "no" { return $false }
    "on" { return $true }
    "off" { return $false }
    default {
      throw "Invalid -StartAgent value: '$text'. Use true/false/\$true/\$false/1/0."
    }
  }
}

function Wait-HttpHealth {
  param(
    [Parameter(Mandatory = $true)][string]$Url,
    [int]$Retries = 80,
    [int]$SleepMS = 500
  )
  for ($i = 0; $i -lt $Retries; $i++) {
    try {
      $resp = Invoke-RestMethod -Method Get -Uri $Url -TimeoutSec 2
      if ($resp.ok -eq $true) {
        return
      }
    }
    catch {
      # keep waiting
    }
    Start-Sleep -Milliseconds $SleepMS
  }
  throw "Health check failed: $Url"
}

function Test-TcpPortOpen {
  param(
    [string]$TargetHost = "127.0.0.1",
    [Parameter(Mandatory = $true)][int]$Port
  )
  $client = New-Object System.Net.Sockets.TcpClient
  try {
    $iar = $client.BeginConnect($TargetHost, $Port, $null, $null)
    if (-not $iar.AsyncWaitHandle.WaitOne(300)) {
      return $false
    }
    $client.EndConnect($iar) | Out-Null
    return $true
  }
  catch {
    return $false
  }
  finally {
    $client.Dispose()
  }
}

function Get-TailIfExists {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [int]$Lines = 20
  )
  if (Test-Path $Path) {
    return (Get-Content -Path $Path -Tail $Lines -ErrorAction SilentlyContinue) -join "`n"
  }
  return ""
}

function Resolve-ClaudePath {
  param([string]$Preferred)
  $candidates = @()
  if (-not [string]::IsNullOrWhiteSpace($Preferred)) {
    $candidates += $Preferred.Trim()
  }
  $candidates += @("claude-code", "claude")
  $seen = @{}
  foreach ($c in $candidates) {
    if ([string]::IsNullOrWhiteSpace($c)) {
      continue
    }
    if ($seen.ContainsKey($c)) {
      continue
    }
    $seen[$c] = $true

    $cmd = Get-Command $c -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
      return $cmd.Source
    }
    if (Test-Path $c) {
      return (Resolve-Path $c).Path
    }
  }
  throw "Cannot find Claude executable. Set -ClaudePath, or ensure 'claude-code'/'claude' is available in PATH."
}

function Start-AgentProcess {
  param(
    [Parameter(Mandatory = $true)][string]$StdoutPath,
    [Parameter(Mandatory = $true)][string]$StderrPath,
    [Parameter(Mandatory = $true)][string[]]$Args,
    [hashtable]$ExtraEnv
  )
  $backup = @{}
  if ($ExtraEnv) {
    foreach ($key in $ExtraEnv.Keys) {
      $backup[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
      [Environment]::SetEnvironmentVariable($key, [string]$ExtraEnv[$key], "Process")
    }
  }
  try {
    return Start-Process `
      -FilePath $agentExe `
      -ArgumentList $Args `
      -WorkingDirectory $bundleRoot `
      -RedirectStandardOutput $StdoutPath `
      -RedirectStandardError $StderrPath `
      -PassThru
  }
  finally {
    if ($ExtraEnv) {
      foreach ($key in $ExtraEnv.Keys) {
        [Environment]::SetEnvironmentVariable($key, $backup[$key], "Process")
      }
    }
  }
}

function Stop-ProcIfRunning {
  param($Proc)
  if ($Proc -and -not $Proc.HasExited) {
    Stop-Process -Id $Proc.Id -Force
  }
}

$startAgentEnabled = Convert-ToBool -Value $StartAgent

foreach ($required in @($controlExe, $uiDir)) {
  if (-not (Test-Path $required)) {
    throw "Missing required path in bundle: $required"
  }
}
if ($startAgentEnabled) {
  foreach ($required in @($agentExe, $chatEchoWorkerExe, $chatClaudeWorkerExe)) {
    if (-not (Test-Path $required)) {
      throw "Missing required path in bundle: $required"
    }
  }
}

try {
  if (Test-TcpPortOpen -Port $ControlPort) {
    throw "Port $ControlPort is already in use. Use another -ControlPort (for example 18081), or stop the existing service first."
  }

  $controlArgs = @(
    "-addr", ":$ControlPort",
    "-ui-dir", $uiDir,
    "-ui-token=",
    "-agent-token=",
    "-admin-token", $AdminToken,
    "-audit-path", (Join-Path $runDir "audit.jsonl"),
    "-offline-after-sec", "30"
  )

  Write-Host "Starting cc-control..."
  $controlProc = Start-Process `
    -FilePath $controlExe `
    -ArgumentList $controlArgs `
    -WorkingDirectory $bundleRoot `
    -RedirectStandardOutput $controlLogOut `
    -RedirectStandardError $controlLogErr `
    -PassThru

  $baseUrl = "http://127.0.0.1:$ControlPort"
  Wait-HttpHealth -Url "$baseUrl/api/healthz"
  if ($controlProc.HasExited) {
    $stderrTail = Get-TailIfExists -Path $controlLogErr
    throw "cc-control exited unexpectedly right after start. stderr:`n$stderrTail"
  }
  Write-Host "cc-control is healthy."

  try {
    Invoke-RestMethod `
      -Method Get `
      -Uri "$baseUrl/admin/verify" `
      -Headers @{ Authorization = "Bearer $AdminToken" } | Out-Null
  }
  catch {
    throw "Admin token verification failed (token mismatch or wrong service on this port). Current -AdminToken='$AdminToken', port=$ControlPort."
  }

  Write-Host "Creating tenant token..."
  $tenantResp = Invoke-RestMethod `
    -Method Post `
    -Uri "$baseUrl/admin/tokens" `
    -Headers @{ Authorization = "Bearer $AdminToken" } `
    -ContentType "application/json" `
    -Body (@{ type = "tenant" } | ConvertTo-Json -Compress)

  $tenantToken = [string]$tenantResp.token
  $tenantID = [string]$tenantResp.tenant_id
  if ([string]::IsNullOrWhiteSpace($tenantToken) -or [string]::IsNullOrWhiteSpace($tenantID)) {
    throw "Failed to create tenant token: missing token or tenant_id"
  }

  Write-Host "Generating UI + Agent tokens from tenant token..."
  $tenantIssueResp = Invoke-RestMethod `
    -Method Post `
    -Uri "$baseUrl/tenant/tokens" `
    -Headers @{ Authorization = "Bearer $tenantToken" } `
    -ContentType "application/json" `
    -Body (@{ role = "owner" } | ConvertTo-Json -Compress)

  $uiToken = [string]$tenantIssueResp.ui.token
  $agentToken = [string]$tenantIssueResp.agent.token
  if ([string]::IsNullOrWhiteSpace($uiToken) -or [string]::IsNullOrWhiteSpace($agentToken)) {
    throw "Failed to issue ui/agent tokens from tenant token"
  }

  if ($startAgentEnabled) {
    $resolvedClaudePath = Resolve-ClaudePath -Preferred $ClaudePath
    if ([string]::IsNullOrWhiteSpace($ChatProfileFile)) {
      $defaultProfile = Join-Path $bundleRoot "chat-profile.md"
      if (Test-Path $defaultProfile) {
        $ChatProfileFile = $defaultProfile
      }
    }
    if (-not [string]::IsNullOrWhiteSpace($ChatProfileFile) -and -not (Test-Path $ChatProfileFile)) {
      throw "Chat profile file not found: $ChatProfileFile"
    }
    Write-Host "Claude path: $resolvedClaudePath"
    if (-not [string]::IsNullOrWhiteSpace($ChatProfileFile)) {
      Write-Host "Chat profile file: $ChatProfileFile"
    }

    $commonArgs = @(
      "-control-url", "ws://127.0.0.1:$ControlPort/ws/agent",
      "-agent-token", $agentToken,
      "-allow-root", $AllowRoot,
      "-claude-path", $resolvedClaudePath
    )

    Write-Host "Starting cc-agent #1 PTY ($ptyServerID)..."
    $ptyArgs = @($commonArgs + @("-server-id", $ptyServerID))
    $ptyAgentProc = Start-AgentProcess -StdoutPath $ptyAgentLogOut -StderrPath $ptyAgentLogErr -Args $ptyArgs

    Write-Host "Starting cc-agent #2 chat-claude ($chatClaudeServerID)..."
    $chatClaudeArgs = @($commonArgs + @("-server-id", $chatClaudeServerID, "-chat-worker", $chatClaudeWorkerExe))
    $chatClaudeAgentProc = Start-AgentProcess `
      -StdoutPath $chatClaudeAgentLogOut `
      -StderrPath $chatClaudeAgentLogErr `
      -Args $chatClaudeArgs `
      -ExtraEnv @{
        "CC_CLAUDE_CMD" = $resolvedClaudePath
        "CC_CLAUDE_PROFILE_FILE" = $ChatProfileFile
      }

    Write-Host "Starting cc-agent #3 chat-echo ($chatEchoServerID)..."
    $chatEchoArgs = @($commonArgs + @("-server-id", $chatEchoServerID, "-chat-worker", $chatEchoWorkerExe))
    $chatEchoAgentProc = Start-AgentProcess -StdoutPath $chatEchoAgentLogOut -StderrPath $chatEchoAgentLogErr -Args $chatEchoArgs
  }

  $result = [ordered]@{
    base_url = $baseUrl
    admin_token = $AdminToken
    tenant_id = $tenantID
    tenant_token = $tenantToken
    ui_token = $uiToken
    agent_token = $agentToken
    allow_root = $AllowRoot
    control = [ordered]@{
      pid = if ($controlProc) { $controlProc.Id } else { $null }
      logs = [ordered]@{
        stdout = $controlLogOut
        stderr = $controlLogErr
      }
    }
    agents = [ordered]@{
      pty = [ordered]@{
        server_id = $ptyServerID
        pid = if ($ptyAgentProc) { $ptyAgentProc.Id } else { $null }
        logs = [ordered]@{
          stdout = $ptyAgentLogOut
          stderr = $ptyAgentLogErr
        }
      }
      chat_claude = [ordered]@{
        server_id = $chatClaudeServerID
        pid = if ($chatClaudeAgentProc) { $chatClaudeAgentProc.Id } else { $null }
        logs = [ordered]@{
          stdout = $chatClaudeAgentLogOut
          stderr = $chatClaudeAgentLogErr
        }
      }
      chat_echo = [ordered]@{
        server_id = $chatEchoServerID
        pid = if ($chatEchoAgentProc) { $chatEchoAgentProc.Id } else { $null }
        logs = [ordered]@{
          stdout = $chatEchoAgentLogOut
          stderr = $chatEchoAgentLogErr
        }
      }
    }
    resolved_claude_path = if ([string]::IsNullOrWhiteSpace($resolvedClaudePath)) { $null } else { $resolvedClaudePath }
    chat_profile_file = if ([string]::IsNullOrWhiteSpace($ChatProfileFile)) { $null } else { $ChatProfileFile }
    urls = [ordered]@{
      web_terminal = "$baseUrl/"
      web_chat = "$baseUrl/chat"
      admin = "$baseUrl/admin"
      tenant = "$baseUrl/tenant"
    }
  }
  $result | ConvertTo-Json -Depth 8 | Set-Content -Path $resultPath -Encoding UTF8

  Write-Host ""
  Write-Host "Done."
  Write-Host "Tenant ID:    $tenantID"
  Write-Host "Tenant Token: $tenantToken"
  Write-Host "UI Token:     $uiToken"
  Write-Host "Agent Token:  $agentToken"
  Write-Host ""
  Write-Host "Server IDs:"
  Write-Host "  PTY:         $ptyServerID"
  Write-Host "  chat-claude: $chatClaudeServerID"
  Write-Host "  chat-echo:   $chatEchoServerID"
  Write-Host ""
  Write-Host "Allow Root:   $AllowRoot"
  Write-Host ""
  Write-Host "Result file:  $resultPath"
  Write-Host "Chat URL:     $baseUrl/chat"
  Write-Host ""
  Write-Host "Stop commands:"
  if ($chatEchoAgentProc) {
    Write-Host "  Stop-Process -Id $($chatEchoAgentProc.Id) -Force"
  }
  if ($chatClaudeAgentProc) {
    Write-Host "  Stop-Process -Id $($chatClaudeAgentProc.Id) -Force"
  }
  if ($ptyAgentProc) {
    Write-Host "  Stop-Process -Id $($ptyAgentProc.Id) -Force"
  }
  if ($controlProc) {
    Write-Host "  Stop-Process -Id $($controlProc.Id) -Force"
  }
}
catch {
  Write-Error $_
  Stop-ProcIfRunning -Proc $chatEchoAgentProc
  Stop-ProcIfRunning -Proc $chatClaudeAgentProc
  Stop-ProcIfRunning -Proc $ptyAgentProc
  Stop-ProcIfRunning -Proc $controlProc
  exit 1
}
