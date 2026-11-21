param(
  [Parameter(Mandatory)][string]$Baseline,
  [Parameter(Mandatory)][string]$InnoCompiler,
  [Parameter(Mandatory)][string]$AppRelease,
  [Parameter(Mandatory)][string]$SigningKey,
  [Parameter(Mandatory)][string]$ReleaseNotes
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$baseline = (Resolve-Path -LiteralPath $Baseline).Path
$compiler = (Resolve-Path -LiteralPath $InnoCompiler).Path
if ($AppRelease -notmatch '^(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})$') { throw 'Use a numeric application version such as 1.9.9.' }
$SigningKey = (Resolve-Path -LiteralPath $SigningKey).Path
$ReleaseNotes = (Resolve-Path -LiteralPath $ReleaseNotes).Path
$output = Join-Path $root "dist\installer_qa\$AppRelease"
New-Item -ItemType Directory -Force -Path $output | Out-Null
$output = (Resolve-Path -LiteralPath $output).Path
$stage = Join-Path $output ('.stage-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
$stage = (Resolve-Path -LiteralPath $stage).Path
$utf8 = [Text.UTF8Encoding]::new($false)

Push-Location $root
try {
  $publicKey = go run ./apps/release-v198/cmd/hspo-release -action public -key $SigningKey
  if ($LASTEXITCODE -ne 0 -or $publicKey -notmatch '^[a-f0-9]{64}$') { throw 'Could not read the release public key.' }
  $keyFlags = "-X hspo/release-v198/appupdate.releasePublicKey=$publicKey"
  $manifest = Get-Content -LiteralPath (Join-Path $baseline 'SHA256SUMS.txt') -Encoding UTF8
  $items = [ordered]@{}
  foreach ($line in $manifest) {
    if ($line -notmatch '^([a-fA-F0-9]{64})  (.+)$') { continue }
    $name, $digest = $Matches[2], $Matches[1]
    if ($name -notmatch '^(static/|policy_data/|tools/tesseract/|HQTN_HSPO_OCR_Worker\.ps1$)') { continue }
    if ($name.StartsWith('/') -or $name.Contains('\') -or $name.Contains(':') -or
      ($name -split '/' | Where-Object { $_ -in '', '.', '..' }) -or $items.Contains($name)) {
      throw "Invalid path in the baseline manifest: $name"
    }
    $source = Join-Path $baseline $name
    $target = Join-Path $stage $name
    if ((Get-FileHash -LiteralPath $source).Hash -ne $digest) { throw "Baseline SHA-256 mismatch: $name" }
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
    Copy-Item -LiteralPath $source -Destination $target
    $items[$name] = $digest.ToLowerInvariant()
  }
  foreach ($required in 'static/index.html','static/VERSION.txt','static/hspo.ico','static/tariff_2026.json.gz',
    'policy_data/chemical_policy.json','tools/tesseract/tesseract.exe','HQTN_HSPO_OCR_Worker.ps1') {
    if (!$items.Contains($required)) { throw "Missing installer asset: $required" }
  }
  $version = (Get-Content -LiteralPath (Join-Path $stage 'static/VERSION.txt') -Raw).Trim()
  if ($version -ne '1.9.8') { throw "Unexpected QA baseline version: $version" }
  # The root version matches the data format. The public version follows the program release.
  Copy-Item -LiteralPath (Join-Path $stage 'static/VERSION.txt') -Destination (Join-Path $stage 'VERSION.txt')
  [IO.File]::WriteAllLines((Join-Path $stage 'static/VERSION.txt'), @($AppRelease), $utf8)
  [IO.File]::WriteAllLines((Join-Path $stage 'APP_RELEASE.txt'), @($AppRelease), $utf8)
  $webRoot = (Resolve-Path -LiteralPath 'apps/release-v198/web/static').Path
  foreach ($resource in Get-ChildItem -LiteralPath $webRoot -File -Recurse) {
    if ($resource.Extension -notin '.html','.js','.css','.svg','.ico','.png','.webmanifest') { continue }
    $relative = $resource.FullName.Substring($webRoot.Length + 1)
    if ($relative -like 'documents\*') { continue }
    $target = Join-Path (Join-Path $stage 'static') $relative
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
    Copy-Item -LiteralPath $resource.FullName -Destination $target -Force
  }
  [IO.File]::WriteAllLines((Join-Path $stage 'HQTN_HSPO_QA_INSTALL.txt'), @('HQTN_HSPO QA installer'), $utf8)

  $env:GOOS = 'windows'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'
  go build -trimpath -ldflags $keyFlags -o (Join-Path $stage 'HQTN_HSPO_Core_QA.exe') ./apps/core-recovered/cmd/hspo-core
  if ($LASTEXITCODE -ne 0) { throw 'QA Core build failed.' }
  go build -trimpath -ldflags "-H=windowsgui $keyFlags" -o (Join-Path $stage 'HQTN_HSPO_Tray_QA.exe') ./apps/release-v198/cmd/tray
  if ($LASTEXITCODE -ne 0) { throw 'QA Tray build failed.' }

  $scriptFile = Join-Path $output ('.inno-' + [Guid]::NewGuid().ToString('N') + '.iss')
  $scriptContent = [IO.File]::ReadAllText((Join-Path $root 'installer\HQTN_HSPO_QA.iss'))
  [IO.File]::WriteAllText($scriptFile, $scriptContent, [Text.UTF8Encoding]::new($true))
  try {
    & $compiler /Q "/DStage=$stage" "/DOut=$output" "/DAppRelease=$AppRelease" $scriptFile
    if ($LASTEXITCODE -ne 0) { throw 'Windows installer build failed.' }
  } finally {
    Remove-Item -LiteralPath $scriptFile -Force
  }
  $setup = Join-Path $output 'HQTN_HSPO_QA_Setup_Win64.exe'
  $releaseManifest = Join-Path $output 'HQTN_HSPO_QA_Release.json'
  go run ./apps/release-v198/cmd/hspo-release -action sign -key $SigningKey -version $AppRelease -setup $setup -notes $ReleaseNotes -out $releaseManifest
  if ($LASTEXITCODE -ne 0) { throw 'Release manifest signing failed.' }
  $hash = (Get-FileHash -LiteralPath $setup).Hash.ToLowerInvariant()
  $manifestHash = (Get-FileHash -LiteralPath $releaseManifest).Hash.ToLowerInvariant()
  [IO.File]::WriteAllLines((Join-Path $output 'SHA256SUMS.txt'), @("$hash  HQTN_HSPO_QA_Setup_Win64.exe", "$manifestHash  HQTN_HSPO_QA_Release.json"), $utf8)
  Write-Host "Setup: $setup"
  Write-Host "SHA-256: $hash"
} finally {
  Pop-Location
  if ([IO.Path]::GetDirectoryName($stage) -ieq $output -and [IO.Path]::GetFileName($stage) -match '^\.stage-[a-f0-9]{32}$') {
    Remove-Item -LiteralPath $stage -Recurse -Force
  }
}
