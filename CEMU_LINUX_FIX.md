# Cemu Linux Launch Fix

## Problem
Cemu AppImage was not launching correctly from the EmuBuddy launcher on Linux, even though:
- Cemu could be launched manually from the command line
- Cemu could run games when launched directly
- The installation was correct and files were in place

## Root Cause
The launcher was setting the working directory to the emulator's directory (`Emulators/Cemu/`) when launching AppImages. This caused Cemu to be unable to properly access configuration files and other resources that it expects to find relative to a specific base directory.

## Solution
Modified the launcher to set the working directory to the EmuBuddy root directory (instead of the emulator directory) when launching AppImages on Linux. This allows Cemu and other AppImages to:
1. Access their configuration directories properly
2. Find ROM files with correct paths
3. Access all necessary resources relative to the project root

## Changes Made
File: `launcher/gui/main.go`

### In `launchGameHeadless()` function (~line 742):
- Added logic to detect AppImages on Linux
- Set working directory to `baseDir` instead of `emuDir` for AppImages
- Added debug logging to track working directory selection

### In `launchWithEmulator()` function (~line 2157):
- Added logic to detect AppImages on Linux
- Set working directory to `baseDir` instead of `emuDir` for AppImages
- Added debug logging to track working directory selection

## Testing
Comprehensive Docker-based tests confirm:
1. ✓ Cemu launches correctly when run from EmuBuddy root
2. ✓ Cemu fails when run from wrong directory (validation)
3. ✓ Launcher now sets correct working directory for AppImages
4. ✓ ROM files are accessible from the correct working directory
5. ✓ Both GUI and headless modes work correctly

## Impact
- **Cemu on Linux**: Now launches correctly from the launcher
- **Other AppImages**: Also benefit from the fix (PPSSPP, mGBA, etc.)
- **Windows/Mac**: No impact - only affects Linux AppImages
- **RetroArch and Flatpak emulators**: No change - use their own working directories as before

## How to Test
On a Linux system:
1. Install Cemu via the EmuBuddy setup
2. Download a Wii U game (e.g., Mario Maker)
3. Launch the game from the EmuBuddy launcher
4. Cemu should now start correctly

If issues persist, check `launcher_debug.log` for working directory information.
