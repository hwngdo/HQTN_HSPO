$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist\qa"
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
Push-Location (Join-Path $root "apps\core-recovered")
try {
  go test ./...; if ($LASTEXITCODE -ne 0) { throw "Core test failed" }
  go build -trimpath -o (Join-Path $dist "HQTN_HSPO_Recovered_Core_REFERENCE_NOT_PRODUCTION.exe") ./cmd/hspo-core
  if ($LASTEXITCODE -ne 0) { throw "Core reference build failed" }
  go build -trimpath -o (Join-Path $dist "HQTN_HSPO_Recovered_Launcher_REFERENCE_NOT_PRODUCTION.exe") ./cmd/hspo-launcher
  if ($LASTEXITCODE -ne 0) { throw "Launcher reference build failed" }
} finally { Pop-Location }
Push-Location (Join-Path $root "apps\qa-gateway")
try {
  go test ./...; if ($LASTEXITCODE -ne 0) { throw "QA test failed" }
  go build -trimpath -o (Join-Path $dist "HQTN_HSPO_Gateway_REFERENCE_NOT_PRODUCTION.exe") ./cmd/hspo-qa-gateway
  if ($LASTEXITCODE -ne 0) { throw "Gateway QA build failed" }
} finally { Pop-Location }
Push-Location (Join-Path $root "apps\uat-preview-8970")
try {
  go test ./...; if ($LASTEXITCODE -ne 0) { throw "UAT preview test failed" }
  go build -trimpath -o (Join-Path $dist "HQTN_HSPO_UAT_Preview_REFERENCE_NOT_PRODUCTION.exe") ./cmd/uat-server
  if ($LASTEXITCODE -ne 0) { throw "UAT preview build failed" }
} finally { Pop-Location }
Push-Location (Join-Path $root "apps\release-v198")
try {
  $previousTestCore = $env:HSPO_TRAY_TEST_CORE
  try {
    $env:HSPO_TRAY_TEST_CORE = Join-Path $dist "HQTN_HSPO_Recovered_Core_REFERENCE_NOT_PRODUCTION.exe"
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "v1.9.8 component or QA Tray tests failed" }
  } finally { $env:HSPO_TRAY_TEST_CORE = $previousTestCore }
  $targets = @(
    @{ Command = "./cmd/update-gateway"; Name = "HQTN_HSPO_UpdateGateway" },
    @{ Command = "./cmd/launcher"; Name = "HQTN_HSPO"; LinkerFlags = "-H=windowsgui" },
    @{ Command = "./cmd/tray"; Name = "HQTN_HSPO_Tray"; LinkerFlags = "-H=windowsgui" }
  )
  foreach ($target in $targets) {
    $buildArgs = @("build", "-trimpath")
    if ($target.LinkerFlags) { $buildArgs += @("-ldflags", $target.LinkerFlags) }
    $buildArgs += @("-o", (Join-Path $dist ($target.Name + "_QA_NOT_PRODUCTION.exe")), $target.Command)
    go @buildArgs
    if ($LASTEXITCODE -ne 0) { throw "Windows build failed: $($target.Name)" }
  }
  $qaTray = Join-Path $dist "tray_runtime"
  New-Item -ItemType Directory -Force -Path $qaTray | Out-Null
  Copy-Item (Join-Path $dist "HQTN_HSPO_Tray_QA_NOT_PRODUCTION.exe") (Join-Path $qaTray "HQTN_HSPO_Tray_QA.exe")
  Copy-Item (Join-Path $dist "HQTN_HSPO_Recovered_Core_REFERENCE_NOT_PRODUCTION.exe") (Join-Path $qaTray "HQTN_HSPO_Core_QA.exe")
  Copy-Item (Join-Path $PSScriptRoot "MO_BAN_QA.cmd") $qaTray
} finally { Pop-Location }
Write-Host "QA only. No production binaries have been replaced."
