param(
  [string]$ResultPath = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$bundleRoot = (Resolve-Path $PSScriptRoot).Path
if ([string]::IsNullOrWhiteSpace($ResultPath)) {
  $ResultPath = Join-Path $bundleRoot "run\tokens-and-process.json"
}

if (-not (Test-Path $ResultPath)) {
  throw "Result file not found: $ResultPath"
}

$raw = Get-Content -Path $ResultPath -Raw
$data = $raw | ConvertFrom-Json

function To-IntOrNull {
  param($Value)
  if ($null -eq $Value) {
    return $null
  }
  $text = [string]$Value
  if ([string]::IsNullOrWhiteSpace($text)) {
    return $null
  }
  $n = 0
  if ([int]::TryParse($text, [ref]$n)) {
    return $n
  }
  return $null
}

function Stop-IfRunning {
  param(
    [string]$Name,
    [int]$ProcessId
  )
  $proc = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
  if ($null -eq $proc) {
    Write-Host "$Name already stopped (pid=$ProcessId)"
    return
  }
  Write-Host "stopping $Name (pid=$ProcessId)"
  Stop-Process -Id $ProcessId -Force
}

$ptyPid = To-IntOrNull $data.agents.pty.pid
$claudePid = To-IntOrNull $data.agents.chat_claude.pid
$echoPid = To-IntOrNull $data.agents.chat_echo.pid
$controlPid = To-IntOrNull $data.control.pid

# Backward compatibility with old format.
if ($null -eq $controlPid) { $controlPid = To-IntOrNull $data.control_pid }
if ($null -eq $echoPid) { $echoPid = To-IntOrNull $data.agent_pid }

if ($null -ne $echoPid) { Stop-IfRunning -Name "agent-chat-echo" -ProcessId $echoPid }
if ($null -ne $claudePid) { Stop-IfRunning -Name "agent-chat-claude" -ProcessId $claudePid }
if ($null -ne $ptyPid) { Stop-IfRunning -Name "agent-pty" -ProcessId $ptyPid }
if ($null -ne $controlPid) { Stop-IfRunning -Name "cc-control" -ProcessId $controlPid }

Write-Host "stop completed."
