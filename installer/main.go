package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
)

type EmulatorURL struct {
	Windows string
	Linux   string
	MacOS   string
}

type Emulator struct {
	Name        string
	URLs        EmulatorURL
	ArchiveName map[string]string // platform -> filename
	ExtractDir  string
}

type RetroArchCore struct {
	Name string
	URLs EmulatorURL
}

var emulators = []Emulator{
	{
		Name: "PCSX2 (PS2)",
		URLs: EmulatorURL{
			Windows: "https://github.com/PCSX2/pcsx2/releases/download/v2.2.0/pcsx2-v2.2.0-windows-x64-Qt.7z",
			Linux:   "https://github.com/PCSX2/pcsx2/releases/download/v2.2.0/pcsx2-v2.2.0-linux-appimage-x64-Qt.AppImage",
			MacOS:   "https://github.com/PCSX2/pcsx2/releases/download/v2.2.0/pcsx2-v2.2.0-macos-Qt.tar.xz",
		},
		ArchiveName: map[string]string{
			"windows": "pcsx2.7z",
			"linux":   "pcsx2.AppImage",
			"darwin":  "pcsx2.tar.xz",
		},
		ExtractDir: "PCSX2",
	},
	{
		Name: "PPSSPP (PSP)",
		URLs: EmulatorURL{
			Windows: "https://www.ppsspp.org/files/1_19_3/ppsspp_win.zip",
			Linux:   "", // Flatpak - installed separately
			MacOS:   "https://www.ppsspp.org/files/1_19_3/PPSSPP_macOS.dmg",
		},
		ArchiveName: map[string]string{
			"windows": "ppsspp.zip",
			"linux":   "",
			"darwin":  "ppsspp.dmg",
		},
		ExtractDir: "PPSSPP",
	},
	{
		Name: "Dolphin (GameCube/Wii)",
		URLs: EmulatorURL{
			Windows: "https://dl.dolphin-emu.org/releases/2512/dolphin-2512-x64.7z",
			Linux:   "https://dl.dolphin-emu.org/releases/2512/dolphin-2512-x86_64.flatpak",
			MacOS:   "https://dl.dolphin-emu.org/releases/2512/dolphin-2512-universal.dmg",
		},
		ArchiveName: map[string]string{
			"windows": "dolphin.7z",
			"linux":   "dolphin.flatpak",
			"darwin":  "dolphin.dmg",
		},
		ExtractDir: "Dolphin",
	},
	{
		Name: "DeSmuME (Nintendo DS)",
		URLs: EmulatorURL{
			Windows: "https://github.com/TASEmulators/desmume/releases/download/release_0_9_13/desmume-0.9.13-win64.zip",
			Linux:   "", // Snap - installed separately
			MacOS:   "https://github.com/TASEmulators/desmume/releases/download/release_0_9_13/desmume-0.9.13-macOS.dmg",
		},
		ArchiveName: map[string]string{
			"windows": "desmume.zip",
			"linux":   "",
			"darwin":  "desmume.dmg",
		},
		ExtractDir: "DeSmuME",
	},
	{
		Name: "Azahar (Nintendo 3DS)",
		URLs: EmulatorURL{
			Windows: "https://github.com/azahar-emu/azahar/releases/download/2124.3/azahar-2124.3-windows-msvc.zip",
			Linux:   "", // Not available
			MacOS:   "https://github.com/azahar-emu/azahar/releases/download/2124.3/azahar-2124.3-macos-universal.zip",
		},
		ArchiveName: map[string]string{
			"windows": "azahar.zip",
			"linux":   "",
			"darwin":  "azahar.zip",
		},
		ExtractDir: "Lime3DS",
	},
	{
		Name: "mGBA (Game Boy Advance)",
		URLs: EmulatorURL{
			Windows: "https://github.com/mgba-emu/mgba/releases/download/0.10.5/mGBA-0.10.5-win64.7z",
			Linux:   "https://github.com/mgba-emu/mgba/releases/download/0.10.5/mGBA-0.10.5-appimage-x64.appimage",
			MacOS:   "https://github.com/mgba-emu/mgba/releases/download/0.10.5/mGBA-0.10.5-macos.dmg",
		},
		ArchiveName: map[string]string{
			"windows": "mgba.7z",
			"linux":   "mgba.AppImage",
			"darwin":  "mgba.dmg",
		},
		ExtractDir: "mGBA",
	},
	{
		Name: "RetroArch (Multi-System)",
		URLs: EmulatorURL{
			Windows: "https://buildbot.libretro.com/stable/1.19.1/windows/x86_64/RetroArch.7z",
			Linux:   "https://buildbot.libretro.com/stable/1.19.1/linux/x86_64/RetroArch.7z",
			MacOS:   "https://buildbot.libretro.com/stable/1.19.1/apple/osx/universal/RetroArch_Metal.dmg",
		},
		ArchiveName: map[string]string{
			"windows": "retroarch.7z",
			"linux":   "retroarch.7z",
			"darwin":  "retroarch.dmg",
		},
		ExtractDir: "RetroArch",
	},
}

var retroarchCores = EmulatorURL{
	Windows: "https://buildbot.libretro.com/stable/1.19.1/windows/x86_64/RetroArch_cores.7z",
	Linux:   "https://buildbot.libretro.com/stable/1.19.1/linux/x86_64/RetroArch_cores.7z",
	MacOS:   "", // Cores included in DMG
}

// BIOS files URLs
var retroarchBIOSURL = "https://github.com/Abdess/retroarch_system/releases/download/v20220308/libretro_31-01-22.zip"

// PS2 BIOS - USA version for best compatibility
var ps2BIOSURL = "https://myrient.erista.me/files/Redump/Sony%20-%20PlayStation%202%20-%20BIOS%20Images%20%28DoM%20Version%29/ps2-0220a-20060905-125923.zip"

// Additional cores that need to be downloaded separately (not in the main cores pack)
var additionalCores = []RetroArchCore{
	{
		Name: "Citra (3DS)",
		URLs: EmulatorURL{
			Windows: "https://buildbot.libretro.com/nightly/windows/x86_64/latest/citra_libretro.dll.zip",
			Linux:   "https://buildbot.libretro.com/nightly/linux/x86_64/latest/citra_libretro.so.zip",
			MacOS:   "",
		},
	},
}

func main() {
	printHeader()

	// Detect OS
	platform := runtime.GOOS
	platformName := getPlatformName(platform)

	printInfo(fmt.Sprintf("Detected platform: %s", platformName))
	fmt.Println()

	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		printError("Failed to get executable path: " + err.Error())
		waitForExit(1)
		return
	}
	baseDir := filepath.Dir(exePath)

	// Create necessary directories
	emuDir := filepath.Join(baseDir, "Emulators")
	downloadDir := filepath.Join(baseDir, "Downloads")

	for _, dir := range []string{emuDir, downloadDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			printError("Failed to create directory " + dir + ": " + err.Error())
			waitForExit(1)
			return
		}
	}

	// Download and setup 7-Zip for all platforms
	printSection("Step 1: Setting up 7-Zip")
	extractorPath := get7ZipPath(baseDir)
	if !fileExists(extractorPath) {
		if err := setup7Zip(baseDir); err != nil {
			printError("Failed to setup 7-Zip: " + err.Error())
			waitForExit(1)
			return
		}
	} else {
		printSuccess("7-Zip already installed")
	}
	
	// On non-Windows, also check for tar (needed for .tar.xz files)
	if platform != "windows" {
		if !commandExists("tar") {
			printError("'tar' command not found. Please install tar utilities.")
			waitForExit(1)
			return
		}
	}

	// Download emulators
	printSection("Step 2: Downloading Emulators")
	installedCount := 0
	skippedCount := 0
	failedEmulators := []string{}
	linuxManualInstalls := []string{}

	for i, emu := range emulators {
		fmt.Printf("[%d/%d] %s\n", i+1, len(emulators), emu.Name)

		// Get platform-specific URL
		url := getURLForPlatform(emu.URLs, platform)
		if url == "" {
			printWarning("  Not available for " + platformName)
			if platform == "linux" {
				linuxManualInstalls = append(linuxManualInstalls, emu.Name)
			} else {
				failedEmulators = append(failedEmulators, emu.Name)
			}
			skippedCount++
			continue
		}

		archiveName := emu.ArchiveName[platform]
		downloadPath := filepath.Join(downloadDir, archiveName)
		extractPath := filepath.Join(emuDir, emu.ExtractDir)

		// Skip if already extracted/installed
		if fileExists(extractPath) || (platform == "linux" && strings.HasSuffix(archiveName, ".AppImage")) {
			printInfo("  Already installed, skipping...")
			installedCount++
			continue
		}

		// Download
		if !fileExists(downloadPath) {
			printInfo("  Downloading...")
			if err := downloadFile(url, downloadPath); err != nil {
				printWarning("  Download failed: " + err.Error())
				printWarning("  Skipping " + emu.Name)
				failedEmulators = append(failedEmulators, emu.Name)
				continue
			}
		} else {
			printInfo("  Archive already downloaded")
		}

		// Extract/Install based on file type
		printInfo("  Installing...")
		if err := extractFile(extractorPath, downloadPath, extractPath, platform); err != nil {
			printWarning("  Installation failed: " + err.Error())
			failedEmulators = append(failedEmulators, emu.Name)
			continue
		}

		printSuccess("  ✓ Installed")
		installedCount++
	}

	fmt.Println()
	printInfo(fmt.Sprintf("Successfully installed: %d/%d emulators", installedCount, len(emulators)))
	if skippedCount > 0 {
		printInfo(fmt.Sprintf("Skipped (already installed): %d", skippedCount-len(failedEmulators)))
	}
	if len(failedEmulators) > 0 {
		printWarning("Failed to install: " + strings.Join(failedEmulators, ", "))
	}
	if len(linuxManualInstalls) > 0 {
		fmt.Println()
		printInfo("Linux: Install these via package manager:")
		for _, name := range linuxManualInstalls {
			if strings.Contains(name, "PPSSPP") {
				printInfo("  • PPSSPP: flatpak install flathub org.ppsspp.PPSSPP")
			} else if strings.Contains(name, "DeSmuME") {
				printInfo("  • DeSmuME: sudo snap install desmume-emulator")
			} else if strings.Contains(name, "Azahar") {
				printInfo("  • Azahar: Not available for Linux (compile from source)")
			}
		}
	}

	// Download RetroArch cores
	if platform != "darwin" { // macOS DMG includes cores
		printSection("Step 3: Downloading RetroArch Cores")
		coresURL := getURLForPlatform(retroarchCores, platform)
		if coresURL != "" {
			coresArchive := filepath.Join(downloadDir, "RetroArch_cores.7z")
			retroarchDir := filepath.Join(emuDir, "RetroArch")

			if !fileExists(coresArchive) {
				printInfo("Downloading RetroArch cores package...")
				if err := downloadFile(coresURL, coresArchive); err != nil {
					printWarning("Failed to download cores: " + err.Error())
				} else {
					printInfo("Extracting cores...")
					// The cores 7z contains RetroArch-Win64/cores/ structure
					// Extract directly to RetroArch/ so cores end up in RetroArch/RetroArch-Win64/cores/
					coresExtractPath := retroarchDir
					if err := extractFile(extractorPath, coresArchive, coresExtractPath, platform); err != nil {
						printWarning("Failed to extract cores: " + err.Error())
					} else {
						printSuccess("✓ RetroArch cores installed")
					}
				}
			} else {
				printSuccess("RetroArch cores already installed")
			}
		}

		// Download additional cores (like Citra) that aren't in the main pack
		printInfo("Downloading additional cores...")
		coresDir := filepath.Join(emuDir, "RetroArch", "RetroArch-Win64", "cores")
		if platform == "linux" {
			coresDir = filepath.Join(emuDir, "RetroArch", "RetroArch-Linux-x86_64", "cores")
		}
		os.MkdirAll(coresDir, 0755)

		for _, core := range additionalCores {
			coreURL := getURLForPlatform(core.URLs, platform)
			if coreURL == "" {
				continue
			}
			
			// Determine expected dll/so name from URL
			coreName := filepath.Base(coreURL)
			coreName = strings.TrimSuffix(coreName, ".zip")
			coreFile := filepath.Join(coresDir, coreName)
			
			if fileExists(coreFile) {
				printSuccess(fmt.Sprintf("  ✓ %s already installed", core.Name))
				continue
			}
			
			printInfo(fmt.Sprintf("  Downloading %s core...", core.Name))
			coreArchive := filepath.Join(downloadDir, filepath.Base(coreURL))
			if err := downloadFile(coreURL, coreArchive); err != nil {
				printWarning(fmt.Sprintf("  Failed to download %s: %s", core.Name, err.Error()))
				continue
			}
			
			// Extract the core zip directly to cores folder
			if err := extractZipToDir(coreArchive, coresDir); err != nil {
				printWarning(fmt.Sprintf("  Failed to extract %s: %s", core.Name, err.Error()))
			} else {
				printSuccess(fmt.Sprintf("  ✓ %s core installed", core.Name))
			}
		}
	} else {
		printSection("Step 3: RetroArch Cores")
		printSuccess("Cores included in RetroArch Metal DMG")
	}

	// Download BIOS files
	printSection("Step 4: Downloading BIOS Files")
	biosDir := filepath.Join(emuDir, "RetroArch", "RetroArch-Win64", "system")
	if platform == "linux" {
		biosDir = filepath.Join(emuDir, "RetroArch", "RetroArch-Linux-x86_64", "system")
	} else if platform == "darwin" {
		biosDir = filepath.Join(emuDir, "RetroArch", "system")
	}
	os.MkdirAll(biosDir, 0755)

	// Download RetroArch system/BIOS files
	printInfo("Downloading RetroArch BIOS/System files...")
	retroarchBiosArchive := filepath.Join(downloadDir, "retroarch_bios.zip")
	if !fileExists(retroarchBiosArchive) {
		if err := downloadFile(retroarchBIOSURL, retroarchBiosArchive); err != nil {
			printWarning("Failed to download RetroArch BIOS: " + err.Error())
		} else {
			printInfo("Extracting RetroArch BIOS files...")
			if err := extractZip(retroarchBiosArchive, biosDir); err != nil {
				printWarning("Failed to extract RetroArch BIOS: " + err.Error())
			} else {
				printSuccess("✓ RetroArch BIOS files installed")
			}
		}
	} else {
		printSuccess("RetroArch BIOS files already downloaded")
	}

	// Download PS2 BIOS
	printInfo("Downloading PS2 BIOS...")
	ps2BiosArchive := filepath.Join(downloadDir, "ps2_bios.zip")
	pcsx2BiosDir := filepath.Join(emuDir, "PCSX2", "bios")
	os.MkdirAll(pcsx2BiosDir, 0755)

	if !fileExists(ps2BiosArchive) {
		if err := downloadFromMyrient(ps2BIOSURL, ps2BiosArchive); err != nil {
			printWarning("Failed to download PS2 BIOS: " + err.Error())
		} else {
			printInfo("Extracting PS2 BIOS files...")
			if err := extractZip(ps2BiosArchive, pcsx2BiosDir); err != nil {
				printWarning("Failed to extract PS2 BIOS: " + err.Error())
			} else {
				printSuccess("✓ PS2 BIOS files installed")
			}
		}
	} else {
		printSuccess("PS2 BIOS files already downloaded")
	}

	// Configure PCSX2 to use the BIOS directory
	if platform == "windows" {
		printInfo("Configuring PCSX2...")
		if err := configurePCSX2(emuDir, pcsx2BiosDir); err != nil {
			printWarning("Failed to configure PCSX2: " + err.Error())
		} else {
			printSuccess("✓ PCSX2 configured")
		}
	}

	// Configure RetroArch system directory
	printInfo("Configuring RetroArch...")
	if err := configureRetroArch(emuDir, biosDir, platform); err != nil {
		printWarning("Failed to configure RetroArch: " + err.Error())
	} else {
		printSuccess("✓ RetroArch configured")
	}

	// Cleanup
	printSection("Step 5: Cleanup")
	printInfo("Removing downloaded archives...")
	os.RemoveAll(downloadDir)
	printSuccess("✓ Cleanup complete")

	// Final summary
	fmt.Println()
	if len(failedEmulators) == 0 && len(linuxManualInstalls) == 0 {
		printSuccess("═══════════════════════════════════════")
		printSuccess("  Installation Complete!")
		printSuccess("═══════════════════════════════════════")
		fmt.Println()
		printInfo("All emulators installed successfully!")
	} else {
		printSuccess("═══════════════════════════════════════")
		printSuccess("  Installation Mostly Complete!")
		printSuccess("═══════════════════════════════════════")
		fmt.Println()
		printInfo(fmt.Sprintf("Installed %d/%d emulators successfully.", installedCount, len(emulators)))
		if len(linuxManualInstalls) > 0 {
			printInfo("Some emulators require manual installation (see above).")
		}
		if len(failedEmulators) > 0 {
			printWarning("Some emulators failed to download.")
		}
	}

	fmt.Println()
	printInfo("Next steps:")
	if platform == "windows" {
		printInfo("  Launching EmuBuddy...")
	} else if platform == "linux" {
		printInfo("  1. Run: ./start_launcher.sh (web interface)")
		printInfo("  2. Or run: ./start_gui.sh (desktop app)")
	} else if platform == "darwin" {
		printInfo("  1. Run: ./start_launcher.sh (web interface)")
		printInfo("  2. Or run: ./start_gui.sh (desktop app)")
		printInfo("  3. Mount DMG files and drag apps to Applications folder")
	}
	fmt.Println()

	// Launch the GUI on Windows
	if platform == "windows" {
		launcherPath := filepath.Join(baseDir, "EmuBuddyLauncher.exe")
		if fileExists(launcherPath) {
			exec.Command(launcherPath).Start()
		}
	}

	os.Exit(0)
}

func getPlatformName(platform string) string {
	switch platform {
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "darwin":
		return "macOS"
	default:
		return platform
	}
}

func getURLForPlatform(urls EmulatorURL, platform string) string {
	switch platform {
	case "windows":
		return urls.Windows
	case "linux":
		return urls.Linux
	case "darwin":
		return urls.MacOS
	default:
		return ""
	}
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func printHeader() {
	platform := getPlatformName(runtime.GOOS)
	fmt.Println(colorCyan + "╔═══════════════════════════════════════╗" + colorReset)
	fmt.Println(colorCyan + "║   EmuBuddy Installer v2.1            ║" + colorReset)
	fmt.Println(colorCyan + "║   Cross-Platform Edition             ║" + colorReset)
	fmt.Println(colorCyan + "╚═══════════════════════════════════════╝" + colorReset)
	fmt.Println()
	fmt.Println("Platform: " + platform)
	fmt.Println()
	fmt.Println("This installer will download and set up:")
	fmt.Println("  • 7 Emulators (~350 MB)")
	fmt.Println("  • RetroArch Cores (~468 MB)")
	fmt.Println("  • BIOS Files (~600 MB)")
	fmt.Println("  • Total download: ~1.4 GB")
	fmt.Println()
	fmt.Println("Press Ctrl+C to cancel, or Enter to continue...")
	fmt.Scanln()
	fmt.Println()
}

func printSection(title string) {
	fmt.Println()
	fmt.Println(colorCyan + "═══════════════════════════════════════" + colorReset)
	fmt.Println(colorCyan + "  " + title + colorReset)
	fmt.Println(colorCyan + "═══════════════════════════════════════" + colorReset)
}

func printSuccess(msg string) {
	fmt.Println(colorGreen + msg + colorReset)
}

func printInfo(msg string) {
	fmt.Println(msg)
}

func printWarning(msg string) {
	fmt.Println(colorYellow + msg + colorReset)
}

func printError(msg string) {
	fmt.Println(colorRed + "ERROR: " + msg + colorReset)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func downloadFile(url, destPath string) error {
	return downloadFileWithReferer(url, destPath, "")
}

// downloadFileWithReferer downloads a file with an optional Referer header
func downloadFileWithReferer(url, destPath, referer string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	client := &http.Client{
		Timeout: 30 * time.Minute,
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	
	// Set headers to avoid rate limiting
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	totalSize := resp.ContentLength
	downloaded := int64(0)
	lastPrint := time.Now()

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)

			if time.Since(lastPrint) > time.Second {
				if totalSize > 0 {
					pct := float64(downloaded) / float64(totalSize) * 100
					fmt.Printf("\r  Progress: %.1f%% (%s / %s)", pct, formatBytes(downloaded), formatBytes(totalSize))
				} else {
					fmt.Printf("\r  Downloaded: %s", formatBytes(downloaded))
				}
				lastPrint = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}

// downloadFromMyrient downloads a file from Myrient with proper headers to avoid rate limiting
func downloadFromMyrient(url, destPath string) error {
	return downloadFileWithReferer(url, destPath, "https://myrient.erista.me/")
}

func extractFile(extractorPath, archivePath, destDir string, platform string) error {
	ext := filepath.Ext(archivePath)

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, destDir)

	case strings.HasSuffix(archivePath, ".7z"):
		// Use our bundled 7-Zip on all platforms
		return extract7z(extractorPath, archivePath, destDir)

	case strings.HasSuffix(archivePath, ".tar.xz"):
		return extractTarXz(archivePath, destDir)

	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGz(archivePath, destDir)

	case strings.HasSuffix(archivePath, ".AppImage") || strings.HasSuffix(archivePath, ".appimage"):
		// Make AppImage executable and move to destination
		if err := os.Chmod(archivePath, 0755); err != nil {
			return err
		}
		finalPath := filepath.Join(destDir, filepath.Base(archivePath))
		return os.Rename(archivePath, finalPath)

	case strings.HasSuffix(archivePath, ".dmg"):
		printInfo("  DMG file downloaded. Mount it manually and drag to Applications folder.")
		// Move DMG to a visible location
		dmgPath := filepath.Join(filepath.Dir(destDir), filepath.Base(archivePath))
		return os.Rename(archivePath, dmgPath)

	case strings.HasSuffix(archivePath, ".flatpak"):
		printInfo("  Flatpak downloaded. Install with: flatpak install " + archivePath)
		return nil

	default:
		return fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func extract7z(sevenZipPath, archivePath, destDir string) error {
	cmd := exec.Command(sevenZipPath, "x", archivePath, "-o"+destDir, "-y")
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// extractZipToDir extracts a zip file directly to destDir without stripping root folders
// Used for simple core zip files that contain just the dll/so file
func extractZipToDir(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(destDir, 0755)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		
		// Just use the base filename, ignore any folder structure in the zip
		destPath := filepath.Join(destDir, filepath.Base(f.Name))
		
		rc, err := f.Open()
		if err != nil {
			return err
		}
		
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}
		
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		
		if err != nil {
			return err
		}
	}
	return nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// Clean and normalize destDir for consistent path handling on Windows
	destDir = filepath.Clean(destDir)

	// Ensure destination directory exists first
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %v", destDir, err)
	}

	// Find if there's a common root folder
	var rootFolder string
	if len(r.File) > 0 {
		// Check first file's path
		firstPath := r.File[0].Name
		parts := strings.Split(filepath.ToSlash(firstPath), "/")
		if len(parts) > 1 {
			// Potential root folder
			potentialRoot := parts[0] + "/"
			hasRoot := true
			for _, f := range r.File {
				if !strings.HasPrefix(filepath.ToSlash(f.Name), potentialRoot) {
					hasRoot = false
					break
				}
			}
			if hasRoot {
				rootFolder = potentialRoot
			}
		}
	}

	// Helper function to check if a ZIP entry is a directory
	isDir := func(f *zip.File) bool {
		// Check the mode flag
		if f.FileInfo().IsDir() {
			return true
		}
		// Also check for trailing slash (some ZIPs mark dirs this way)
		if strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, "\\") {
			return true
		}
		// Check if uncompressed size is 0 and name looks like a directory
		if f.UncompressedSize64 == 0 && !strings.Contains(filepath.Base(f.Name), ".") {
			return true
		}
		return false
	}

	// First pass: collect all directories that need to be created
	dirsToCreate := make(map[string]bool)
	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		if rootFolder != "" {
			name = strings.TrimPrefix(name, rootFolder)
		}
		if name == "" {
			continue
		}

		// Remove trailing slashes before processing
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}

		// Security check
		if strings.Contains(name, "..") {
			continue
		}

		// Convert to OS-specific path and clean it
		name = filepath.Clean(filepath.FromSlash(name))
		fpath := filepath.Join(destDir, name)

		if isDir(f) {
			dirsToCreate[fpath] = true
		} else {
			// Add parent directory
			parentDir := filepath.Dir(fpath)
			if parentDir != destDir {
				dirsToCreate[parentDir] = true
			}
		}
	}

	// Create all directories upfront, sorted by depth (shortest paths first)
	var sortedDirs []string
	for dir := range dirsToCreate {
		sortedDirs = append(sortedDirs, dir)
	}
	// Sort by path length to ensure parent dirs are created first
	for i := 0; i < len(sortedDirs); i++ {
		for j := i + 1; j < len(sortedDirs); j++ {
			if len(sortedDirs[i]) > len(sortedDirs[j]) {
				sortedDirs[i], sortedDirs[j] = sortedDirs[j], sortedDirs[i]
			}
		}
	}

	for _, dir := range sortedDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %v", dir, err)
		}
	}

	// Second pass: extract files
	for _, f := range r.File {
		// Skip directories (already created)
		if isDir(f) {
			continue
		}

		name := filepath.ToSlash(f.Name)
		if rootFolder != "" {
			name = strings.TrimPrefix(name, rootFolder)
		}

		if name == "" {
			continue
		}

		// Security check
		if strings.Contains(name, "..") {
			continue
		}

		// Convert to OS-specific path and clean it
		name = filepath.Clean(filepath.FromSlash(name))
		fpath := filepath.Join(destDir, name)

		// Create file
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("create file %s: %v", fpath, err)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if copyErr != nil {
			return fmt.Errorf("write file %s: %v", fpath, copyErr)
		}
	}

	return nil
}

func moveDir(src, dst string) error {
	// Ensure destination exists
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := moveDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := os.Rename(srcPath, dstPath); err != nil {
				// If rename fails, try copy
				if copyErr := copyFile(srcPath, dstPath); copyErr != nil {
					return copyErr
				}
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func extractTarXz(tarXzPath, destDir string) error {
	f, err := os.Open(tarXzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	xzReader, err := xz.NewReader(f)
	if err != nil {
		return err
	}

	return extractTar(xzReader, destDir)
}

func extractTarGz(tarGzPath, destDir string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	return extractTar(gzReader, destDir)
}

func extractTar(reader io.Reader, destDir string) error {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

func setup7Zip(baseDir string) error {
	toolsDir := filepath.Join(baseDir, "Tools", "7zip")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		return err
	}

	platform := runtime.GOOS
	
	switch platform {
	case "windows":
		sevenZipPath := filepath.Join(toolsDir, "7za.exe")
		printInfo("Downloading 7-Zip for Windows...")
		url := "https://www.7-zip.org/a/7zr.exe"
		if err := downloadFile(url, sevenZipPath); err != nil {
			return err
		}
		
	case "linux":
		printInfo("Downloading 7-Zip for Linux...")
		tarPath := filepath.Join(toolsDir, "7z-linux.tar.xz")
		url := "https://github.com/ip7z/7zip/releases/download/25.01/7z2501-linux-x64.tar.xz"
		if err := downloadFile(url, tarPath); err != nil {
			return err
		}
		// Extract tar.xz using system tar (more reliable for complex xz files)
		cmd := exec.Command("tar", "-xf", tarPath, "-C", toolsDir)
		if err := cmd.Run(); err != nil {
			// Fallback to Go implementation
			if err := extractTarXz(tarPath, toolsDir); err != nil {
				return fmt.Errorf("failed to extract 7-Zip: %v", err)
			}
		}
		// Make 7zz executable
		sevenZipPath := filepath.Join(toolsDir, "7zz")
		if err := os.Chmod(sevenZipPath, 0755); err != nil {
			return err
		}
		// Clean up tarball
		os.Remove(tarPath)
		
	case "darwin":
		printInfo("Downloading 7-Zip for macOS...")
		tarPath := filepath.Join(toolsDir, "7z-mac.tar.xz")
		url := "https://github.com/ip7z/7zip/releases/download/25.01/7z2501-mac.tar.xz"
		if err := downloadFile(url, tarPath); err != nil {
			return err
		}
		// Extract tar.xz using system tar (more reliable for complex xz files)
		cmd := exec.Command("tar", "-xf", tarPath, "-C", toolsDir)
		if err := cmd.Run(); err != nil {
			// Fallback to Go implementation
			if err := extractTarXz(tarPath, toolsDir); err != nil {
				return fmt.Errorf("failed to extract 7-Zip: %v", err)
			}
		}
		// Make 7zz executable
		sevenZipPath := filepath.Join(toolsDir, "7zz")
		if err := os.Chmod(sevenZipPath, 0755); err != nil {
			return err
		}
		// Clean up tarball
		os.Remove(tarPath)
		
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}

	printSuccess("✓ 7-Zip installed")
	return nil
}

func get7ZipPath(baseDir string) string {
	toolsDir := filepath.Join(baseDir, "Tools", "7zip")
	platform := runtime.GOOS
	
	switch platform {
	case "windows":
		return filepath.Join(toolsDir, "7za.exe")
	case "linux", "darwin":
		return filepath.Join(toolsDir, "7zz")
	default:
		return ""
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func waitForExit(code int) {
	if runtime.GOOS == "windows" {
		fmt.Println()
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
	}
	os.Exit(code)
}

// configurePCSX2 sets up PCSX2 to use the provided BIOS directory in portable mode
func configurePCSX2(emuDir, biosDir string) error {
	pcsx2Dir := filepath.Join(emuDir, "PCSX2")
	
	// Create portable.txt to make PCSX2 use local config (Qt version uses portable.txt)
	portableFile := filepath.Join(pcsx2Dir, "portable.txt")
	if err := os.WriteFile(portableFile, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to create portable.txt: %v", err)
	}
	
	// Create the inis directory for config files
	inisDir := filepath.Join(pcsx2Dir, "inis")
	if err := os.MkdirAll(inisDir, 0755); err != nil {
		return fmt.Errorf("failed to create inis directory: %v", err)
	}
	
	// Create necessary directories for portable mode
	dirsToCreate := []string{"bios", "snaps", "sstates", "memcards", "logs", "cheats", "patches", "cache", "textures", "inputprofiles", "covers", "gamesettings"}
	for _, dir := range dirsToCreate {
		os.MkdirAll(filepath.Join(pcsx2Dir, dir), 0755)
	}
	
	// PCSX2 Qt version uses relative paths in portable mode
	// The bios folder is relative to the PCSX2 directory
	pcsx2Config := `[UI]
SettingsVersion = 1
InhibitScreensaver = true
StartFullscreen = false
SetupWizardIncomplete = false

[Folders]
Bios = bios
Snapshots = snaps
Savestates = sstates
MemoryCards = memcards
Logs = logs
Cheats = cheats
Patches = patches
Cache = cache
Textures = textures
InputProfiles = inputprofiles
Covers = covers

[EmuCore]
EnablePatches = true
EnableFastBoot = true
EnableGameFixes = true

[BIOS]
SearchDirectory = bios
`
	
	configPath := filepath.Join(inisDir, "PCSX2.ini")
	if err := os.WriteFile(configPath, []byte(pcsx2Config), 0644); err != nil {
		return fmt.Errorf("failed to write PCSX2.ini: %v", err)
	}
	
	return nil
}

// configureRetroArch sets up RetroArch to use the provided system/BIOS directory
func configureRetroArch(emuDir, systemDir string, platform string) error {
	var retroarchDir string
	switch platform {
	case "windows":
		retroarchDir = filepath.Join(emuDir, "RetroArch", "RetroArch-Win64")
	case "linux":
		retroarchDir = filepath.Join(emuDir, "RetroArch", "RetroArch-Linux-x86_64")
	case "darwin":
		retroarchDir = filepath.Join(emuDir, "RetroArch")
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	
	configPath := filepath.Join(retroarchDir, "retroarch.cfg")
	
	// Convert paths to use forward slashes (RetroArch prefers this even on Windows)
	systemPath := filepath.ToSlash(systemDir)
	
	// Check if config already exists
	existingConfig := ""
	if data, err := os.ReadFile(configPath); err == nil {
		existingConfig = string(data)
	}
	
	// Key settings to ensure BIOS is found
	settings := map[string]string{
		"system_directory":        `"` + systemPath + `"`,
		"systemfiles_in_content_dir": `"false"`,
	}
	
	// Update or add settings
	lines := strings.Split(existingConfig, "\n")
	settingsFound := make(map[string]bool)
	
	for i, line := range lines {
		for key := range settings {
			if strings.HasPrefix(strings.TrimSpace(line), key+" ") || strings.HasPrefix(strings.TrimSpace(line), key+"=") {
				lines[i] = key + " = " + settings[key]
				settingsFound[key] = true
			}
		}
	}
	
	// Append any settings that weren't found
	for key, value := range settings {
		if !settingsFound[key] {
			lines = append(lines, key+" = "+value)
		}
	}
	
	// Write the updated config
	if err := os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write retroarch.cfg: %v", err)
	}
	
	return nil
}
