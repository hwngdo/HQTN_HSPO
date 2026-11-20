@echo off
setlocal
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
if not exist dist mkdir dist
go build -trimpath -o dist\HQTN_HSPO_Core_Recovered.exe .\cmd\hspo-core
if errorlevel 1 exit /b 1
go build -trimpath -o dist\HQTN_HSPO_Recovered.exe .\cmd\hspo-launcher
if errorlevel 1 exit /b 1
echo Built dist\HQTN_HSPO_Core_Recovered.exe and dist\HQTN_HSPO_Recovered.exe
