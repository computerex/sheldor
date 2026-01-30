@echo off
setlocal enabledelayedexpansion

echo ==========================================
echo EmuBuddy Distribution Package Builder
echo ==========================================
echo.

:: Set version (can be passed as argument or defaulted)
set VERSION=%1
if "%VERSION%"=="" set VERSION=1.0.0

:: Create dist folder
set DIST_DIR=dist
if exist "%DIST_DIR%" rmdir /s /q "%DIST_DIR%"
mkdir "%DIST_DIR%"

:: Temp folders for each platform
set WIN_DIR=%DIST_DIR%\EmuBuddy-Windows-%VERSION%
set LINUX_DIR=%DIST_DIR%\EmuBuddy-Linux-%VERSION%
set MACOS_DIR=%DIST_DIR%\EmuBuddy-macOS-%VERSION%

echo [1/6] Building Windows Launcher...
cd launcher\gui
go build -ldflags="-s -w" -o ..\..\EmuBuddyLauncher.exe
cd ..\..
if not exist "EmuBuddyLauncher.exe" (
    echo ERROR: Failed to build Windows launcher
    exit /b 1
)

echo [2/6] Building Linux Launcher...
cd launcher\gui
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=1
go build -ldflags="-s -w" -o ..\..\EmuBuddyLauncher-linux 2>nul
cd ..\..
set GOOS=
set GOARCH=
set CGO_ENABLED=

echo [3/6] Building macOS Launcher...
cd launcher\gui
set GOOS=darwin
set GOARCH=amd64
set CGO_ENABLED=1
go build -ldflags="-s -w" -o ..\..\EmuBuddyLauncher-macos 2>nul
cd ..\..
set GOOS=
set GOARCH=
set CGO_ENABLED=

echo [4/6] Creating Windows distribution...
mkdir "%WIN_DIR%"
mkdir "%WIN_DIR%\1g1rsets"

:: Copy Windows files
copy EmuBuddyLauncher.exe "%WIN_DIR%\"
copy systems.json "%WIN_DIR%\"
copy README.md "%WIN_DIR%\"
xcopy /s /e /q 1g1rsets\*.json "%WIN_DIR%\1g1rsets\"

:: Create empty folders
mkdir "%WIN_DIR%\roms"
mkdir "%WIN_DIR%\Emulators"

:: Create Windows ZIP
echo Creating EmuBuddy-Windows-%VERSION%.zip...
cd "%DIST_DIR%"
powershell -Command "Compress-Archive -Path 'EmuBuddy-Windows-%VERSION%\*' -DestinationPath 'EmuBuddy-Windows-%VERSION%.zip' -Force"
cd ..

echo [5/6] Creating Linux distribution...
mkdir "%LINUX_DIR%"
mkdir "%LINUX_DIR%\1g1rsets"

:: Copy Linux files
if exist "EmuBuddyLauncher-linux" (
    copy EmuBuddyLauncher-linux "%LINUX_DIR%\"
) else (
    echo WARNING: Linux launcher not found, skipping...
)
copy systems.json "%LINUX_DIR%\"
copy README.md "%LINUX_DIR%\"
xcopy /s /e /q 1g1rsets\*.json "%LINUX_DIR%\1g1rsets\"

:: Create empty folders
mkdir "%LINUX_DIR%\roms"
mkdir "%LINUX_DIR%\Emulators"

:: Create run script for Linux
echo #!/bin/bash > "%LINUX_DIR%\run.sh"
echo chmod +x EmuBuddyLauncher-linux >> "%LINUX_DIR%\run.sh"
echo ./EmuBuddyLauncher-linux >> "%LINUX_DIR%\run.sh"

:: Create Linux ZIP
echo Creating EmuBuddy-Linux-%VERSION%.zip...
cd "%DIST_DIR%"
powershell -Command "Compress-Archive -Path 'EmuBuddy-Linux-%VERSION%\*' -DestinationPath 'EmuBuddy-Linux-%VERSION%.zip' -Force"
cd ..

echo [6/6] Creating macOS distribution...
mkdir "%MACOS_DIR%"
mkdir "%MACOS_DIR%\1g1rsets"

:: Copy macOS files
if exist "EmuBuddyLauncher-macos" (
    copy EmuBuddyLauncher-macos "%MACOS_DIR%\"
) else (
    echo WARNING: macOS launcher not found, skipping...
)
copy systems.json "%MACOS_DIR%\"
copy README.md "%MACOS_DIR%\"
xcopy /s /e /q 1g1rsets\*.json "%MACOS_DIR%\1g1rsets\"

:: Create empty folders
mkdir "%MACOS_DIR%\roms"
mkdir "%MACOS_DIR%\Emulators"

:: Create run script for macOS
echo #!/bin/bash > "%MACOS_DIR%\run.sh"
echo chmod +x EmuBuddyLauncher-macos >> "%MACOS_DIR%\run.sh"
echo ./EmuBuddyLauncher-macos >> "%MACOS_DIR%\run.sh"

:: Create macOS ZIP
echo Creating EmuBuddy-macOS-%VERSION%.zip...
cd "%DIST_DIR%"
powershell -Command "Compress-Archive -Path 'EmuBuddy-macOS-%VERSION%\*' -DestinationPath 'EmuBuddy-macOS-%VERSION%.zip' -Force"
cd ..

:: Cleanup temp folders
echo.
echo Cleaning up temporary folders...
rmdir /s /q "%WIN_DIR%"
rmdir /s /q "%LINUX_DIR%"
rmdir /s /q "%MACOS_DIR%"

echo.
echo ==========================================
echo Distribution packages created in dist\
echo ==========================================
echo.
dir /b "%DIST_DIR%\*.zip"
echo.
echo Done!

endlocal
