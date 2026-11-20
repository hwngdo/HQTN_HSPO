param([switch]$Force)
$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$tools = Join-Path $here 'tools'
$target = Join-Path $tools 'tesseract'
$exe = Join-Path $target 'tesseract.exe'
$installer = Join-Path $tools 'bootstrap\tesseract-ocr-w64-setup-5.5.3.20260724.exe'
$expectedSha256 = 'BEE9E3434BD94FD65387D9BE28CD467A41F61B1275383B55B0F59A1331270AE4'

Write-Host '=== HQTN_HSPO embedded Tesseract bootstrap v1.9.3.2 ==='
if ((Test-Path -LiteralPath $exe) -and -not $Force) {
  Write-Host ('[OK] Embedded Tesseract already exists: ' + $exe) -ForegroundColor Green
  & $exe --version | Select-Object -First 3
  exit 0
}
if (-not (Test-Path -LiteralPath $installer)) {
  Write-Host ('[ERROR] Bundled installer missing: ' + $installer) -ForegroundColor Red
  exit 20
}
$hash = (Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToUpperInvariant()
if ($hash -ne $expectedSha256) {
  Write-Host '[ERROR] Bundled Tesseract installer SHA256 mismatch.' -ForegroundColor Red
  Write-Host ('Expected: ' + $expectedSha256)
  Write-Host ('Actual:   ' + $hash)
  exit 21
}

if (Test-Path -LiteralPath $target) {
  try { Remove-Item -LiteralPath $target -Recurse -Force -ErrorAction Stop } catch {
    Write-Host ('[ERROR] Cannot prepare local Tesseract folder: ' + $_.Exception.Message) -ForegroundColor Red
    exit 22
  }
}

Write-Host '[INFO] Deploying bundled Tesseract locally. No Internet is used.'
Write-Host ('Target: ' + $target)
# NSIS standard silent switch. /D must be the final command-line option.
$argLine = '/S /D=' + $target
$p = Start-Process -FilePath $installer -ArgumentList $argLine -PassThru -Wait
if ($p.ExitCode -ne 0) {
  Write-Host ('[WARN] Installer exit code: ' + $p.ExitCode) -ForegroundColor Yellow
}

if (-not (Test-Path -LiteralPath $exe)) {
  # Fallback only if this installer ignored the custom directory.
  $fallbacks = New-Object System.Collections.Generic.List[string]
  if ($env:ProgramFiles) { $fallbacks.Add((Join-Path $env:ProgramFiles 'Tesseract-OCR')) }
  if (${env:ProgramFiles(x86)}) { $fallbacks.Add((Join-Path ${env:ProgramFiles(x86)} 'Tesseract-OCR')) }
  if ($env:LOCALAPPDATA) { $fallbacks.Add((Join-Path $env:LOCALAPPDATA 'Programs\Tesseract-OCR')) }
  $source = $null
  foreach ($d in $fallbacks) {
    if ($d -and (Test-Path -LiteralPath (Join-Path $d 'tesseract.exe'))) { $source = $d; break }
  }
  if ($source) {
    Write-Host ('[INFO] Copying installed runtime into package: ' + $source)
    [System.IO.Directory]::CreateDirectory($target) | Out-Null
    Copy-Item -LiteralPath (Join-Path $source '*') -Destination $target -Recurse -Force
  }
}

if (-not (Test-Path -LiteralPath $exe)) {
  Write-Host '[ERROR] Tesseract could not be deployed into tools\tesseract.' -ForegroundColor Red
  Write-Host 'The bundled installer may require an elevation prompt on this Windows installation.' -ForegroundColor Yellow
  exit 23
}

Write-Host ('[OK] Embedded Tesseract ready: ' + $exe) -ForegroundColor Green
& $exe --version | Select-Object -First 3
exit 0
