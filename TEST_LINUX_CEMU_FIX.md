# How to Test the Cemu Linux Fix

## Quick Test (5 minutes)

### On Your Linux System:

1. **Replace the launcher binary**:
   ```bash
   cd /path/to/your/EmuBuddy
   # Backup old launcher (optional)
   cp EmuBuddyLauncher-linux EmuBuddyLauncher-linux.backup
   # Copy new launcher from the project
   cp /path/to/project/EmuBuddyLauncher-linux .
   chmod +x EmuBuddyLauncher-linux
   ```

2. **Launch the GUI**:
   ```bash
   ./EmuBuddyLauncher-linux
   ```

3. **Test with Mario Maker** (or any Wii U game):
   - Select "Wii U" system from the left panel
   - Find "Super Mario Maker" in the game list
   - Double-click or press A to launch
   - **Expected**: Cemu should start and load the game

4. **Check the debug log** (if you want details):
   ```bash
   cat launcher_debug.log | grep "working dir"
   ```
   You should see: `Using base directory as working dir for AppImage`

## Command Line Test (Alternative)

If you prefer testing from command line:

```bash
cd /path/to/your/EmuBuddy
./EmuBuddyLauncher-linux --launch wiiu "roms/wiiu/Super Mario Maker (USA)/code/game.rpx"
```

Replace the ROM path with your actual game's RPX file path.

## What Should Happen

### Before the Fix ❌
- Launcher shows "Launch failed" or game doesn't start
- Cemu window doesn't appear
- No error message or silent failure

### After the Fix ✅
- Cemu window appears within 2-3 seconds
- Game loads normally
- Same experience as Windows

## Troubleshooting

### If Cemu Still Doesn't Launch:

1. **Check if Cemu is installed**:
   ```bash
   ls -la Emulators/Cemu/
   ```
   Should show a `.AppImage` file

2. **Check if Cemu is executable**:
   ```bash
   file Emulators/Cemu/*.AppImage
   ```
   Should show "executable"

3. **Test Cemu manually**:
   ```bash
   ./Emulators/Cemu/*.AppImage --help
   ```
   Should show Cemu help text

4. **Check ROM file exists**:
   ```bash
   find roms/wiiu -name "*.rpx"
   ```
   Should list your game RPX files

5. **Check debug log**:
   ```bash
   tail -50 launcher_debug.log
   ```
   Look for error messages or launch details

### Common Issues and Solutions:

| Issue | Solution |
|-------|----------|
| "No such file or directory" | Check Cemu is installed in `Emulators/Cemu/` |
| "Permission denied" | Run `chmod +x Emulators/Cemu/*.AppImage` |
| Cemu starts but crashes | Check if your system has required libraries |
| "FUSE not found" | Install FUSE: `sudo apt install fuse` or use `--appimage-extract-and-run` |

## Docker Test (For Development/Validation)

If you want to test in a clean environment:

```bash
cd /path/to/project
docker build -f test-final-cemu.dockerfile -t emubuddy-test .
docker run --rm emubuddy-test
```

Expected output: "ALL TESTS PASSED!"

## Reporting Issues

If the fix doesn't work for you, please provide:
1. Your Linux distribution and version: `cat /etc/os-release`
2. The contents of `launcher_debug.log`
3. Output of: `./Emulators/Cemu/*.AppImage --help`
4. Whether manual launch works: `./Emulators/Cemu/*.AppImage -g /full/path/to/game.rpx`

## Success Indicators

You'll know the fix is working when:
- ✅ Cemu launches within a few seconds of selecting a game
- ✅ Cemu window appears and game loads
- ✅ Debug log shows "Using base directory as working dir for AppImage"
- ✅ No "Launch failed" or other error messages

---

**Need Help?** Check the `LINUX_CEMU_INVESTIGATION_SUMMARY.md` for technical details and troubleshooting.
