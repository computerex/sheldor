@echo off
:: Quick Windows installer build for development/testing
cd /d "%~dp0installer"
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o ..\EmuBuddySetup.exe main.go
if %ERRORLEVEL% EQU 0 (
    echo Built EmuBuddySetup.exe
) else (
    echo Build failed
)
