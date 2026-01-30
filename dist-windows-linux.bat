@echo off
setlocal enabledelayedexpansion

:: ==========================================
:: EmuBuddy Distribution - Windows & Linux
:: ==========================================
:: Creates distribution ZIPs for Windows and Linux
:: Usage: dist-windows-linux.bat [version]
:: ==========================================

set VERSION=%1
if "%VERSION%"=="" set VERSION=1.0.0

echo.
echo ==========================================
echo   EmuBuddy Distribution (Windows/Linux)
echo   Version: %VERSION%
echo ==========================================
echo.

:: Create dist folder
if not exist dist mkdir dist

:: ==========================================
:: Build Installers
:: ==========================================
echo Building Installers...
cd installer
set CGO_ENABLED=0

echo   Windows...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o ..\EmuBuddySetup.exe main.go
if %ERRORLEVEL% NEQ 0 (echo   [FAILED] & goto :error)
echo   [OK] EmuBuddySetup.exe

echo   Linux...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o ..\EmuBuddySetup-linux main.go
if %ERRORLEVEL% NEQ 0 (echo   [FAILED] & goto :error)
echo   [OK] EmuBuddySetup-linux

cd ..

:: ==========================================
:: Build Launchers
:: ==========================================
echo.
echo Building Launchers...

:: Check Docker for Linux build
docker info >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Docker is not running. Start Docker Desktop for Linux launcher build.
    goto :error
)

cd launcher\gui

:: Windows native
echo   Windows...
set GOOS=
set GOARCH=
set CGO_ENABLED=
go build -ldflags="-s -w -H windowsgui" -o ..\..\EmuBuddyLauncher.exe .
if %ERRORLEVEL% NEQ 0 (echo   [FAILED] & goto :error)
echo   [OK] EmuBuddyLauncher.exe

:: Linux via fyne-cross
echo   Linux (Docker)...
fyne-cross linux -arch=amd64 -app-id=com.emubuddy.launcher >nul 2>&1
if exist "fyne-cross\bin\linux-amd64\gui" (
    copy /y "fyne-cross\bin\linux-amd64\gui" "..\..\EmuBuddyLauncher-linux" >nul
    echo   [OK] EmuBuddyLauncher-linux
) else (
    echo   [FAILED] Linux launcher
    goto :error
)

cd ..\..

:: ==========================================
:: Create Windows ZIP
:: ==========================================
echo.
echo Creating Windows distribution...
set WIN_DIR=dist\EmuBuddy-Windows-v%VERSION%
if exist "%WIN_DIR%" rmdir /s /q "%WIN_DIR%"
mkdir "%WIN_DIR%"
mkdir "%WIN_DIR%\1g1rsets"

copy EmuBuddyLauncher.exe "%WIN_DIR%\" >nul
copy EmuBuddySetup.exe "%WIN_DIR%\" >nul
copy systems.json "%WIN_DIR%\" >nul
copy README.md "%WIN_DIR%\" >nul
xcopy 1g1rsets\*.json "%WIN_DIR%\1g1rsets\" /q >nul

cd dist
powershell -Command "Compress-Archive -Path 'EmuBuddy-Windows-v%VERSION%' -DestinationPath 'EmuBuddy-Windows-v%VERSION%.zip' -Force"
cd ..
echo   [OK] dist\EmuBuddy-Windows-v%VERSION%.zip

:: ==========================================
:: Create Linux ZIP
:: ==========================================
echo Creating Linux distribution...
set LIN_DIR=dist\EmuBuddy-Linux-v%VERSION%
if exist "%LIN_DIR%" rmdir /s /q "%LIN_DIR%"
mkdir "%LIN_DIR%"
mkdir "%LIN_DIR%\1g1rsets"

copy EmuBuddyLauncher-linux "%LIN_DIR%\" >nul
copy EmuBuddySetup-linux "%LIN_DIR%\" >nul
copy systems.json "%LIN_DIR%\" >nul
copy README.md "%LIN_DIR%\" >nul
xcopy 1g1rsets\*.json "%LIN_DIR%\1g1rsets\" /q >nul

:: Create run scripts
echo #!/bin/bash > "%LIN_DIR%\start-emubuddy.sh"
echo cd "$(dirname "$0")" >> "%LIN_DIR%\start-emubuddy.sh"
echo ./EmuBuddyLauncher-linux >> "%LIN_DIR%\start-emubuddy.sh"

echo #!/bin/bash > "%LIN_DIR%\run-setup.sh"
echo cd "$(dirname "$0")" >> "%LIN_DIR%\run-setup.sh"
echo ./EmuBuddySetup-linux >> "%LIN_DIR%\run-setup.sh"

cd dist
powershell -Command "Compress-Archive -Path 'EmuBuddy-Linux-v%VERSION%' -DestinationPath 'EmuBuddy-Linux-v%VERSION%.zip' -Force"
cd ..
echo   [OK] dist\EmuBuddy-Linux-v%VERSION%.zip

:: ==========================================
:: Summary
:: ==========================================
echo.
echo ==========================================
echo   Distribution Complete!
echo ==========================================
echo.
dir dist\*.zip /b
echo.
goto :end

:error
echo.
echo Build failed!
exit /b 1

:end
