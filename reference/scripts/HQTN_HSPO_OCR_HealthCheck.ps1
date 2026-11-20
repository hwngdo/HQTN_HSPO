param(
  [switch]$Wait
)
$ErrorActionPreference = 'Stop'
$PackageVersion = 'v1.9.6.9'
$ExpectedBridgeBuild = '1.9.6.9-lan'
$HealthUrl = 'http://127.0.0.1:8872/health'
$ExpectedExe = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'HQTN_HSPO_OCR_Bridge.exe'))

function Get-Health {
  try {
    return Invoke-RestMethod -Uri $HealthUrl -TimeoutSec 2
  } catch {
    return $null
  }
}

$health = $null
$tries = 1
if ($Wait) { $tries = 25 }
for ($i = 0; $i -lt $tries; $i++) {
  $health = Get-Health
  if ($health -and $health.ok) { break }
  if ($Wait) { Start-Sleep -Milliseconds 400 }
}

if (-not $health -or -not $health.ok) {
  Write-Host ('[ERROR] OCR bridge is not ready at ' + $HealthUrl) -ForegroundColor Red
  exit 9
}

if ([string]$health.build -ne $ExpectedBridgeBuild) {
  Write-Host ('[ERROR] Unexpected OCR bridge version: ' + [string]$health.build + '; expected ' + $ExpectedBridgeBuild) -ForegroundColor Red
  exit 10
}

$conn = Get-NetTCPConnection -LocalPort 8872 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $conn) {
  Write-Host '[ERROR] No LISTEN process found on port 8872.' -ForegroundColor Red
  exit 11
}

$proc = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $conn.OwningProcess) -ErrorAction SilentlyContinue
if (-not $proc -or -not $proc.ExecutablePath) {
  Write-Host ('[ERROR] Could not resolve process path for PID ' + $conn.OwningProcess) -ForegroundColor Red
  exit 12
}

$ActualExe = [System.IO.Path]::GetFullPath([string]$proc.ExecutablePath)
if (-not [string]::Equals($ActualExe, $ExpectedExe, [System.StringComparison]::OrdinalIgnoreCase)) {
  Write-Host '[ERROR] Port 8872 is owned by a bridge from another folder.' -ForegroundColor Red
  Write-Host ('Expected: ' + $ExpectedExe) -ForegroundColor Yellow
  Write-Host ('Actual:   ' + $ActualExe) -ForegroundColor Yellow
  exit 13
}

Write-Host ('=== HQTN_HSPO OCR package ' + $PackageVersion + ' ===')
$health | ConvertTo-Json -Depth 4
Write-Host ('PID 8872: ' + $proc.ProcessId)
Write-Host ('EXE: ' + $ActualExe)
Write-Host ('Package: ' + $PackageVersion + '; OCR bridge binary: ' + $ExpectedBridgeVersion)
exit 0
