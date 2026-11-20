$ErrorActionPreference='SilentlyContinue'
$here=Split-Path -Parent $MyInvocation.MyCommand.Path
$exe=Join-Path $here 'tools\tesseract\tesseract.exe'
Write-Host '=== HQTN_HSPO Tesseract CAS v1.9.3.5 ==='
if(-not (Test-Path -LiteralPath $exe)){
  Write-Host '[MISSING] Embedded tools\tesseract\tesseract.exe was not found.' -ForegroundColor Yellow
  Write-Host 'Run CAI_TESSERACT_NOI_BO.bat or start HQTN_HSPO once to deploy the bundled engine.'
  exit 2
}
$exe=(Resolve-Path -LiteralPath $exe).Path
Write-Host ('EXE: '+$exe)
& $exe --version | Select-Object -First 3
$td=Join-Path $here 'tools\tessdata\eng.traineddata'
if(Test-Path -LiteralPath $td){Write-Host ('HQTN TESSDATA: '+$td)}else{Write-Host '[WARN] Bundled eng.traineddata not found.' -ForegroundColor Yellow}
exit 0
