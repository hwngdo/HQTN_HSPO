@echo off
setlocal
cd /d "%~dp0"
if not exist "%~dp0HQTN_HSPO_Tray_QA.exe" (
    echo Khong tim thay HQTN_HSPO_Tray_QA.exe.
    pause
    exit /b 1
)
start "" "%~dp0HQTN_HSPO_Tray_QA.exe" -qa
