@echo off
:: Quick Windows launcher build for development/testing
cd /d "%~dp0launcher\gui"
go build -ldflags="-s -w -H windowsgui" -o ..\..\EmuBuddyLauncher.exe .
if %ERRORLEVEL% EQU 0 (
    echo Built EmuBuddyLauncher.exe
) else (
    echo Build failed
)
