package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/0xcafed00d/joystick"

	"github.com/emubuddy/gui/wiiu"
)

// Debug logging
var debugLog *os.File

func logDebug(format string, args ...interface{}) {
	if debugLog == nil {
		var err error
		debugLog, err = os.OpenFile(filepath.Join(baseDir, "launcher_debug.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
	}
	msg := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	debugLog.WriteString(msg)
	debugLog.Sync()
}

// TappableListItem is a container that captures taps and double-taps for list items
type TappableListItem struct {
	widget.BaseWidget
	Content       fyne.CanvasObject
	list          *widget.List
	itemID        widget.ListItemID
	onDoubleTap   func(widget.ListItemID)
	lastTapTime   time.Time
}

func NewTappableListItem(content fyne.CanvasObject) *TappableListItem {
	t := &TappableListItem{
		Content: content,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *TappableListItem) SetListInfo(list *widget.List, id widget.ListItemID, onDoubleTap func(widget.ListItemID)) {
	t.list = list
	t.itemID = id
	t.onDoubleTap = onDoubleTap
}

func (t *TappableListItem) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.Content)
}

func (t *TappableListItem) Tapped(e *fyne.PointEvent) {
	now := time.Now()
	
	// Check for double-tap
	if now.Sub(t.lastTapTime) < 400*time.Millisecond {
		logDebug("Double-tap on item %d!", t.itemID)
		if t.onDoubleTap != nil {
			t.onDoubleTap(t.itemID)
		}
		t.lastTapTime = time.Time{}
		return
	}
	
	t.lastTapTime = now
	
	// Select this item in the list
	if t.list != nil {
		t.list.Select(t.itemID)
	}
}

func (t *TappableListItem) TappedSecondary(e *fyne.PointEvent) {}

type ROM struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Size    string `json:"size"`
	Date    string `json:"date"`
	TitleID string `json:"titleId,omitempty"` // For Wii U games
	Region  string `json:"region,omitempty"`  // For Wii U games
}

type CoreConfig struct {
	Name   string `json:"name"`
	Dll    string `json:"dll"`              // Windows .dll (also used as fallback)
	So     string `json:"so,omitempty"`     // Linux .so override
	Dylib  string `json:"dylib,omitempty"`  // macOS .dylib override
}

// GetCorePath returns the appropriate core path for the current OS
func (c *CoreConfig) GetCorePath() string {
	switch runtime.GOOS {
	case "linux":
		if c.So != "" {
			return c.So
		}
		// Auto-convert from dll
		return strings.TrimSuffix(c.Dll, ".dll") + ".so"
	case "darwin":
		if c.Dylib != "" {
			return c.Dylib
		}
		// Auto-convert from dll
		return strings.TrimSuffix(c.Dll, ".dll") + ".dylib"
	default:
		return c.Dll
	}
}

type EmulatorConfig struct {
	Path  string       `json:"path"`
	Args  []string     `json:"args"`
	Cores []CoreConfig `json:"cores"`
	Name  string       `json:"name"`
}

type SystemConfig struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Dir                string          `json:"dir"`
	RomJsonFile        string          `json:"romJsonFile"`
	LibretroName       string          `json:"libretroName"`
	Emulator           EmulatorConfig  `json:"emulator"`
	StandaloneEmulator *EmulatorConfig `json:"standaloneEmulator"`
	FileExtensions     []string        `json:"fileExtensions"`
	NeedsExtract       bool            `json:"needsExtract"`
	SpecialDownload    string          `json:"specialDownload,omitempty"`
}

type SystemsConfig struct {
	Systems []SystemConfig `json:"systems"`
}

var systems map[string]SystemConfig
var systemsList []string
var favorites map[string]map[string]bool

var baseDir string
var romsDir string
var favoritesPath string

func init() {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}

	exeDir := filepath.Dir(exe)

	if fileExists(filepath.Join(exeDir, "1g1rsets")) {
		baseDir = exeDir
	} else if fileExists(filepath.Join(filepath.Dir(exeDir), "1g1rsets")) {
		baseDir = filepath.Dir(exeDir)
	} else if fileExists(filepath.Join(filepath.Dir(filepath.Dir(exeDir)), "1g1rsets")) {
		baseDir = filepath.Dir(filepath.Dir(exeDir))
	} else {
		baseDir = exeDir
	}

	romsDir = filepath.Join(baseDir, "roms")
	favoritesPath = filepath.Join(baseDir, "favorites.json")

	loadSystemsConfig()
	loadFavorites()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadSystemsConfig() {
	configPath := filepath.Join(baseDir, "systems.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load systems.json: %v", err))
	}

	var config SystemsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		panic(fmt.Sprintf("Failed to parse systems.json: %v", err))
	}

	systems = make(map[string]SystemConfig)
	systemsList = make([]string, 0, len(config.Systems))
	for _, sys := range config.Systems {
		systems[sys.ID] = sys
		systemsList = append(systemsList, sys.ID)
	}
}

func loadFavorites() {
	favorites = make(map[string]map[string]bool)
	data, err := os.ReadFile(favoritesPath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &favorites)
}

func saveFavorites() {
	data, _ := json.Marshal(favorites)
	os.WriteFile(favoritesPath, data, 0644)
}

// resolvePlatformPath converts Windows paths from systems.json to platform-specific paths
func resolvePlatformPath(windowsPath string) string {
	platform := runtime.GOOS

	if platform == "windows" {
		return windowsPath
	}

	// Convert to forward slashes
	path := filepath.ToSlash(windowsPath)

	if platform == "darwin" {
		// macOS-specific path resolution

		// Handle RetroArch
		if strings.Contains(path, "RetroArch/RetroArch-Win64/retroarch.exe") {
			return strings.Replace(path, "RetroArch/RetroArch-Win64/retroarch.exe", "RetroArch/RetroArch.app/Contents/MacOS/RetroArch", 1)
		}

		// Handle RetroArch cores - .dll -> .dylib
		// On macOS, cores are stored in ~/Library/Application Support/RetroArch/cores/
		if strings.Contains(path, "cores/") && strings.HasSuffix(path, ".dll") {
			coreName := filepath.Base(path)
			coreName = strings.TrimSuffix(coreName, ".dll") + ".dylib"
			homeDir, _ := os.UserHomeDir()
			return filepath.Join(homeDir, "Library/Application Support/RetroArch/cores", coreName)
		}

		// Handle Dolphin
		if strings.Contains(path, "Dolphin/Dolphin-x64/Dolphin.exe") {
			return strings.Replace(path, "Dolphin/Dolphin-x64/Dolphin.exe", "Dolphin/Dolphin.app/Contents/MacOS/Dolphin", 1)
		}

		// Handle PCSX2
		if strings.Contains(path, "PCSX2/pcsx2-qt.exe") {
			// Find the actual .app bundle (version may vary)
			pcsx2Dir := filepath.Join(baseDir, "Emulators", "PCSX2")
			if entries, err := os.ReadDir(pcsx2Dir); err == nil {
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), "PCSX2") && strings.HasSuffix(entry.Name(), ".app") {
						return fmt.Sprintf("Emulators/PCSX2/%s/Contents/MacOS/PCSX2-qt", entry.Name())
					}
				}
			}
			return strings.Replace(path, "PCSX2/pcsx2-qt.exe", "PCSX2/PCSX2.app/Contents/MacOS/PCSX2-qt", 1)
		}

		// Handle PPSSPP
		if strings.Contains(path, "PPSSPP/PPSSPPWindows64.exe") {
			return strings.Replace(path, "PPSSPP/PPSSPPWindows64.exe", "PPSSPP/PPSSPP.app/Contents/MacOS/PPSSPP", 1)
		}

		// Handle mGBA
		if strings.Contains(path, "mGBA/mGBA-0.10.5-win64/mGBA.exe") {
			return strings.Replace(path, "mGBA/mGBA-0.10.5-win64/mGBA.exe", "mGBA/mGBA.app/Contents/MacOS/mGBA", 1)
		}

		// Handle melonDS
		if strings.Contains(path, "melonDS/melonDS.exe") {
			return strings.Replace(path, "melonDS/melonDS.exe", "melonDS/melonDS.app/Contents/MacOS/melonDS", 1)
		}

		// Handle Azahar
		if strings.Contains(path, "Azahar/azahar.exe") {
			return strings.Replace(path, "Azahar/azahar.exe", "Azahar/azahar.app/Contents/MacOS/azahar", 1)
		}
	}

	if platform == "linux" {
		// Linux-specific path resolution

		// Handle RetroArch - it's an AppImage on Linux
		if strings.Contains(path, "RetroArch/RetroArch-Win64/retroarch.exe") {
			// Find the actual AppImage in the RetroArch directory
			retroarchDir := filepath.Join(baseDir, "Emulators", "RetroArch", "RetroArch-Linux-x86_64")
			if entries, err := os.ReadDir(retroarchDir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/RetroArch/RetroArch-Linux-x86_64/%s", entry.Name())
					}
				}
			}
			return "Emulators/RetroArch/RetroArch-Linux-x86_64/RetroArch-Linux-x86_64.AppImage"
		}

		// Handle RetroArch cores - .dll -> .so, and update path for Linux
		if strings.Contains(path, "cores/") && strings.HasSuffix(path, ".dll") {
			// Change extension and update the RetroArch path
			path = strings.TrimSuffix(path, ".dll") + ".so"
			path = strings.Replace(path, "RetroArch-Win64", "RetroArch-Linux-x86_64", 1)
			return path
		}

		// Handle PCSX2 - find the AppImage in the PCSX2 folder
		if strings.Contains(path, "PCSX2/pcsx2-qt.exe") {
			pcsx2Dir := filepath.Join(baseDir, "Emulators", "PCSX2")
			if entries, err := os.ReadDir(pcsx2Dir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/PCSX2/%s", entry.Name())
					}
				}
			}
			return "Emulators/PCSX2/pcsx2.AppImage"
		}

		// Handle PPSSPP - find the AppImage in the PPSSPP folder
		if strings.Contains(path, "PPSSPP/PPSSPPWindows64.exe") {
			ppssppDir := filepath.Join(baseDir, "Emulators", "PPSSPP")
			if entries, err := os.ReadDir(ppssppDir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/PPSSPP/%s", entry.Name())
					}
				}
			}
			return "Emulators/PPSSPP/ppsspp.AppImage"
		}

		// Handle mGBA - find the AppImage in the mGBA folder
		if strings.Contains(path, "mGBA/mGBA-0.10.5-win64/mGBA.exe") {
			mgbaDir := filepath.Join(baseDir, "Emulators", "mGBA")
			if entries, err := os.ReadDir(mgbaDir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/mGBA/%s", entry.Name())
					}
				}
			}
			return "Emulators/mGBA/mgba.AppImage"
		}

		// Handle melonDS - find the AppImage in the melonDS folder
		if strings.Contains(path, "melonDS/melonDS.exe") {
			melondsDir := filepath.Join(baseDir, "Emulators", "melonDS")
			if entries, err := os.ReadDir(melondsDir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/melonDS/%s", entry.Name())
					}
				}
			}
			return "Emulators/melonDS/melonDS.AppImage"
		}

		// Handle Azahar - find the AppImage in the Azahar folder
		if strings.Contains(path, "Azahar/azahar.exe") {
			azaharDir := filepath.Join(baseDir, "Emulators", "Azahar")
			if entries, err := os.ReadDir(azaharDir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/Azahar/%s", entry.Name())
					}
				}
			}
			return "Emulators/Azahar/azahar.AppImage"
		}

		// Handle Dolphin - currently Flatpak, but if installed as AppImage
		if strings.Contains(path, "Dolphin/Dolphin-x64/Dolphin.exe") {
			dolphinDir := filepath.Join(baseDir, "Emulators", "Dolphin")
			// Check for AppImage first
			if entries, err := os.ReadDir(dolphinDir); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
						return fmt.Sprintf("Emulators/Dolphin/%s", entry.Name())
					}
				}
			}
			// Dolphin is a Flatpak on Linux - return a special marker that launchGame can handle
			return "flatpak:org.DolphinEmu.dolphin-emu"
		}
	}

	return windowsPath
}

// App holds the application state
type App struct {
	window          fyne.Window
	windowFocused   bool
	currentSystem   string
	allGames        []ROM
	filteredGames   []ROM
	showFavsOnly    bool
	romCache        map[string]bool
	selectedGameIdx int
	selectedSysIdx  int
	focusOnGames    bool // true = game list focused, false = system list focused
	dialogOpen      bool

	// Emulator choice state
	choosingEmulator    bool
	emulatorChoices     []string
	emulatorPaths       []string
	emulatorArgs        [][]string
	selectedEmulatorIdx int
	pendingGame         ROM
	
	// Mouse double-click tracking
	lastClickTime time.Time
	lastClickIdx  int

	// UI elements
	systemList        *widget.List
	gameList          *widget.List
	statusBar         *widget.Label
	searchEntry       *widget.Entry
	searchQuery       string
	instructions      *widget.Label
	favsCheck         *widget.Check
	launchBtn         *widget.Button
	
	// Emulator choice UI
	emulatorList      *widget.List
	emulatorSelectBtn *widget.Button
	emulatorCancelBtn *widget.Button
	mainSplit         *container.Split
	gamePanel         *fyne.Container
	emulatorPanel     *fyne.Container
}

// isSetupComplete checks if emulators have been installed
func isSetupComplete() bool {
	emulatorsDir := filepath.Join(baseDir, "Emulators")
	
	// Check if Emulators directory exists
	info, err := os.Stat(emulatorsDir)
	if err != nil || !info.IsDir() {
		return false
	}
	
	// Check if there's at least one subdirectory (an installed emulator)
	entries, err := os.ReadDir(emulatorsDir)
	if err != nil {
		return false
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			// Found at least one emulator folder
			return true
		}
	}
	
	return false
}

// runSetupAndExit launches the setup program and exits the launcher
func runSetupAndExit() {
	var setupPath string
	
	switch runtime.GOOS {
	case "windows":
		setupPath = filepath.Join(baseDir, "EmuBuddySetup.exe")
	case "darwin":
		setupPath = filepath.Join(baseDir, "EmuBuddySetup-macos")
	default:
		setupPath = filepath.Join(baseDir, "EmuBuddySetup-linux")
	}
	
	// Check if setup exists
	if !fileExists(setupPath) {
		fmt.Println("Setup program not found:", setupPath)
		fmt.Println("Please run EmuBuddySetup first to install emulators.")
		os.Exit(1)
	}
	
	// Make executable on Unix
	if runtime.GOOS != "windows" {
		os.Chmod(setupPath, 0755)
	}
	
	fmt.Println("No emulators found. Launching setup...")
	
	cmd := exec.Command(setupPath)
	cmd.Dir = baseDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	
	os.Exit(0)
}

// launchROMHeadless launches a ROM without showing the GUI
func launchROMHeadless(systemID string, romPath string) {
	fmt.Printf("[DEBUG] Headless launch requested: system=%s, rom=%s\n", systemID, romPath)
	fmt.Printf("[DEBUG] Base directory: %s\n", baseDir)

	config, exists := systems[systemID]
	if !exists {
		fmt.Printf("Error: Unknown system '%s'\n", systemID)
		fmt.Println("Available systems:", systemsList)
		os.Exit(1)
	}

	if romPath == "" {
		fmt.Printf("Error: ROM path required\n")
		fmt.Printf("Usage: %s --launch <system> <rom_path>\n", os.Args[0])
		os.Exit(1)
	}

	if !fileExists(romPath) {
		fmt.Printf("Error: ROM not found: %s\n", romPath)
		os.Exit(1)
	}

	// Convert to absolute path for emulators that don't handle relative paths well
	if !filepath.IsAbs(romPath) {
		absPath, err := filepath.Abs(romPath)
		if err == nil {
			romPath = absPath
			fmt.Printf("[DEBUG] Converted to absolute path: %s\n", romPath)
		}
	}

	// Create a minimal game ROM struct
	game := ROM{
		Name: filepath.Base(romPath),
	}

	// Handle extraction if needed (for systems like Dolphin that can't read zips)
	actualRomPath := romPath
	if config.NeedsExtract && strings.HasSuffix(strings.ToLower(romPath), ".zip") {
		fmt.Printf("[DEBUG] System requires extraction, extracting ZIP...\n")
		romDir := filepath.Dir(romPath)
		extractedPath, err := extractZip(romPath, romDir)
		if err != nil {
			fmt.Printf("Error extracting ROM: %v\n", err)
			os.Exit(1)
		}
		if extractedPath != "" {
			actualRomPath = extractedPath
			fmt.Printf("[DEBUG] Extracted to: %s\n", actualRomPath)
			// Remove the zip after extraction to save space
			os.Remove(romPath)
		}
	}

	fmt.Printf("Launching %s: %s\n", config.Name, game.Name)

	// Use first emulator/core
	emuPath := config.Emulator.Path
	var emuArgs []string

	if len(config.Emulator.Cores) > 0 {
		// Use first core - GetCorePath() handles OS-specific paths
		corePath := config.Emulator.Cores[0].GetCorePath()
		emuArgs = []string{"-L", corePath}
		fmt.Printf("[DEBUG] Using RetroArch core: %s\n", corePath)
	} else {
		emuArgs = config.Emulator.Args
		fmt.Printf("[DEBUG] Using standalone emulator with args: %v\n", emuArgs)
	}

	// Launch the game (reuse existing logic)
	launchGameHeadless(game, actualRomPath, emuPath, emuArgs)
}

// launchGameHeadless launches a game without GUI
func launchGameHeadless(game ROM, romPath string, emuPath string, emuArgs []string) {
	// Resolve platform-specific path
	emuPath = resolvePlatformPath(emuPath)

	// Handle flatpak on Linux
	isFlatpak := strings.HasPrefix(emuPath, "flatpak:")
	var flatpakAppID string
	if isFlatpak {
		flatpakAppID = strings.TrimPrefix(emuPath, "flatpak:")
		emuPath = "flatpak"
	} else {
		emuPath = filepath.Join(baseDir, emuPath)
	}
	emuDir := filepath.Dir(emuPath)

	// On Linux, ensure AppImages are executable
	if runtime.GOOS == "linux" && strings.HasSuffix(strings.ToLower(emuPath), ".appimage") {
		os.Chmod(emuPath, 0755)
	}

	// Build args
	args := []string{}

	// For flatpak, add "run" and the app ID first
	if isFlatpak {
		args = append(args, "run", flatpakAppID)
	}

	for _, arg := range emuArgs {
		if strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
			// Resolve platform-specific core paths
			resolvedArg := resolvePlatformPath(arg)
			if filepath.IsAbs(resolvedArg) {
				args = append(args, resolvedArg)
			} else {
				resolvedPath := filepath.Join(emuDir, resolvedArg)

				// On Linux, verify core file exists
				if runtime.GOOS == "linux" && strings.HasSuffix(strings.ToLower(resolvedPath), ".so") {
					if !fileExists(resolvedPath) {
						fmt.Printf("ERROR: Core file not found: %s\n", resolvedPath)
						os.Exit(1)
					}
				}

				args = append(args, resolvedPath)
			}
		} else {
			args = append(args, arg)
		}
	}
	args = append(args, romPath)

	fmt.Printf("Command: %s %v\n", emuPath, args)

	cmd := exec.Command(emuPath, args...)
	if !isFlatpak {
		cmd.Dir = emuDir
	}

	// On Linux, set environment variables to fix AppImage compatibility
	if runtime.GOOS == "linux" {
		cmd.Env = append(os.Environ(),
			"SDL_VIDEODRIVER=x11",
			"QT_QPA_PLATFORM=xcb",
		)
	}

	// Use Start() instead of Run() so we don't wait for the emulator to exit
	// This allows the launcher to exit immediately after launching
	if err := cmd.Start(); err != nil {
		fmt.Printf("Launch failed: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("Emulator launched successfully")
}

func main() {
	// Print banner to confirm this version is running
	fmt.Println("========================================")
	fmt.Println("  EmuBuddy Launcher v2.0")
	fmt.Println("  Headless Mode Support")
	fmt.Println("========================================")

	// Debug: Print received arguments
	if len(os.Args) > 1 {
		fmt.Printf("[DEBUG] Received %d arguments: %v\n", len(os.Args), os.Args)
	} else {
		fmt.Println("[DEBUG] No arguments received - starting GUI mode")
	}

	// Check for CLI arguments for headless ROM launch FIRST (before setup check)
	// This allows testing even if setup isn't complete
	if len(os.Args) >= 2 && os.Args[1] == "--launch" {
		fmt.Println("[DEBUG] Headless mode activated")
		var systemID string
		var romPath string
		if len(os.Args) >= 3 {
			systemID = os.Args[2]
		}
		if len(os.Args) >= 4 {
			romPath = os.Args[3]
		}
		launchROMHeadless(systemID, romPath)
		return
	}

	// Check if setup has been run (Emulators folder should have content)
	if !isSetupComplete() {
		runSetupAndExit()
		return
	}

	myApp := app.New()
	myApp.Settings().SetTheme(theme.DarkTheme())

	myWindow := myApp.NewWindow("EmuBuddy")
	myWindow.Resize(fyne.NewSize(1000, 600))

	appState := &App{
		window:        myWindow,
		romCache:      make(map[string]bool),
		windowFocused: true,
	}

	appState.buildUI()
	appState.showDisclaimer()
	go appState.pollController()
	myWindow.ShowAndRun()
}

func (a *App) showDisclaimer() {
	disclaimerText := `LEGAL DISCLAIMER

EmuBuddy is a game launcher and does not include any copyrighted game files.

• We do not condone or encourage piracy
• You are solely responsible for ensuring you have the legal right to any games you use with this software
• Only use games that you legally own or have permission to use
• Downloading copyrighted games without authorization is illegal in most jurisdictions

By using this software, you acknowledge that you understand and accept these terms.`

	content := widget.NewLabel(disclaimerText)
	content.Wrapping = fyne.TextWrapWord
	
	d := dialog.NewCustomConfirm("Legal Notice", "I Understand", "Exit", content, func(accepted bool) {
		if !accepted {
			a.window.Close()
		}
	}, a.window)
	d.Resize(fyne.NewSize(500, 350))
	d.Show()
}

func (a *App) buildUI() {
	// System list on left
	a.systemList = widget.NewList(
		func() int { return len(systemsList) },
		func() fyne.CanvasObject {
			return widget.NewLabel("System Name Here")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(systemsList) {
				return
			}
			label := item.(*widget.Label)
			sysID := systemsList[id]
			name := systems[sysID].Name
			if !a.focusOnGames && id == a.selectedSysIdx {
				name = "> " + name
			}
			label.SetText(name)
		},
	)

	a.systemList.OnSelected = func(id widget.ListItemID) {
		a.selectedSysIdx = id
		a.focusOnGames = false
		a.selectSystem(systemsList[id])
		a.systemList.Refresh()
	}

	// Game list on right - use TappableListItem for double-click support
	a.gameList = widget.NewList(
		func() int { return len(a.filteredGames) },
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("Game Name Here")
			statusLabel := widget.NewLabel("Status")
			sizeLabel := widget.NewLabel("Size")
			content := container.NewBorder(nil, nil, nil,
				container.NewHBox(statusLabel, sizeLabel),
				nameLabel,
			)
			return NewTappableListItem(content)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(a.filteredGames) {
				return
			}
			game := a.filteredGames[id]
			tappable := item.(*TappableListItem)
			tappable.SetListInfo(a.gameList, id, func(itemID widget.ListItemID) {
				a.launchSelected()
			})
			
			box := tappable.Content.(*fyne.Container)
			nameLabel := box.Objects[0].(*widget.Label)
			rightBox := box.Objects[1].(*fyne.Container)
			statusLabel := rightBox.Objects[0].(*widget.Label)
			sizeLabel := rightBox.Objects[1].(*widget.Label)

			// Name with favorite indicator
			name := strings.TrimSuffix(game.Name, ".zip")
			name = strings.TrimSuffix(name, ".chd")
			if a.isFavorite(game.Name) {
				name = "[FAV] " + name
			}
			if a.focusOnGames && id == a.selectedGameIdx {
				name = "> " + name
			}
			if len(name) > 60 {
				name = name[:57] + "..."
			}
			nameLabel.SetText(name)

			// Status
			if a.romCache[game.Name] {
				statusLabel.SetText("[Ready]")
			} else {
				statusLabel.SetText("[DL]")
			}

			sizeLabel.SetText(game.Size)
		},
	)

	a.gameList.OnSelected = func(id widget.ListItemID) {
		a.selectedGameIdx = id
		a.focusOnGames = true
		a.updateStatus()
		a.updateLaunchButton()
		a.gameList.Refresh()
		a.systemList.Refresh()
	}

	// Search box
	a.searchEntry = widget.NewEntry()
	a.searchEntry.SetPlaceHolder("Type to search...")
	a.searchEntry.OnChanged = func(s string) {
		a.searchQuery = s
		a.filterGames()
	}

	// Status bar
	a.statusBar = widget.NewLabel("Select a system")

	// Instructions
	a.instructions = widget.NewLabel("Controller: L-Stick=Sys R-Stick=Games A=Select B=Back X=DL Y=Fav | Keyboard: Arrows/Enter/Esc/D=DL/F=Fav | Mouse: Double-click=Launch")
	a.instructions.TextStyle = fyne.TextStyle{Italic: true}

	// Title
	title := canvas.NewText("EmuBuddy", theme.ForegroundColor())
	title.TextSize = 24
	title.TextStyle = fyne.TextStyle{Bold: true}

	// System panel with header
	systemHeader := widget.NewLabel("SYSTEMS")
	systemHeader.TextStyle = fyne.TextStyle{Bold: true}
	systemPanel := container.NewBorder(
		systemHeader, nil, nil, nil,
		a.systemList,
	)

	// Emulator choice list - use TappableListItem for double-click support
	a.emulatorList = widget.NewList(
		func() int { return len(a.emulatorChoices) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("Emulator Option")
			return NewTappableListItem(label)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(a.emulatorChoices) {
				return
			}
			tappable := item.(*TappableListItem)
			tappable.SetListInfo(a.emulatorList, id, func(itemID widget.ListItemID) {
				a.confirmEmulatorChoice()
			})
			
			label := tappable.Content.(*widget.Label)
			name := a.emulatorChoices[id]
			if id == a.selectedEmulatorIdx {
				name = "> " + name
			}
			label.SetText(name)
		},
	)
	a.emulatorList.OnSelected = func(id widget.ListItemID) {
		a.selectedEmulatorIdx = id
		a.emulatorList.Refresh()
	}

	// Favorites checkbox
	a.favsCheck = widget.NewCheck("Favorites Only", func(checked bool) {
		a.showFavsOnly = checked
		a.filterGames()
	})
	
	// Launch/Download button - text changes based on game status
	a.launchBtn = widget.NewButton("Launch", func() {
		if a.selectedGameIdx < 0 || a.selectedGameIdx >= len(a.filteredGames) {
			return
		}
		game := a.filteredGames[a.selectedGameIdx]
		if a.romCache[game.Name] {
			logDebug("Launch button clicked - launching")
			a.launchSelected()
		} else {
			logDebug("Launch button clicked - downloading")
			a.downloadSelected()
		}
	})
	
	// Game panel with header, favorites checkbox, launch button, and search
	gamesLabel := widget.NewLabel("GAMES")
	gameHeader := container.NewBorder(nil, nil,
		container.NewHBox(gamesLabel, a.favsCheck, a.launchBtn),
		nil,
		a.searchEntry,
	)
	a.gamePanel = container.NewBorder(
		gameHeader, nil, nil, nil,
		a.gameList,
	)

	// Emulator choice panel
	emulatorHeader := widget.NewLabel("CHOOSE EMULATOR")
	emulatorHeader.TextStyle = fyne.TextStyle{Bold: true}
	
	a.emulatorSelectBtn = widget.NewButton("Select", func() {
		logDebug("Emulator Select button clicked")
		a.confirmEmulatorChoice()
	})
	a.emulatorCancelBtn = widget.NewButton("Cancel", func() {
		logDebug("Emulator Cancel button clicked")
		a.cancelEmulatorChoice()
	})
	
	emulatorButtons := container.NewHBox(a.emulatorSelectBtn, a.emulatorCancelBtn)
	emulatorHeaderRow := container.NewBorder(nil, nil, emulatorHeader, emulatorButtons)
	
	a.emulatorPanel = container.NewBorder(
		emulatorHeaderRow, nil, nil, nil,
		a.emulatorList,
	)

	// Split view
	a.mainSplit = container.NewHSplit(systemPanel, a.gamePanel)
	a.mainSplit.SetOffset(0.2)

	// Bottom bar
	bottomBar := container.NewBorder(nil, nil, nil, a.statusBar, a.instructions)

	// Main layout
	content := container.NewBorder(
		container.NewPadded(title),
		bottomBar,
		nil, nil,
		a.mainSplit,
	)

	a.window.SetContent(content)

	// Add keyboard shortcuts
	a.window.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
		// Don't handle keys if search box is focused or dialog is open
		if a.dialogOpen {
			return
		}
		
		switch ke.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			// Enter - Launch selected game or select emulator
			if a.choosingEmulator {
				a.confirmEmulatorChoice()
			} else if a.focusOnGames {
				a.launchSelected()
			} else {
				// Focus on games
				a.focusOnGames = true
				if len(a.filteredGames) > 0 {
					a.gameList.Select(0)
				}
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
			
		case fyne.KeyEscape, fyne.KeyBackspace:
			// Escape/Backspace - Go back
			if a.choosingEmulator {
				a.cancelEmulatorChoice()
			} else if a.focusOnGames {
				a.focusOnGames = false
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
			
		case fyne.KeyDown:
			// Down arrow - Move selection down
			if a.choosingEmulator {
				if a.selectedEmulatorIdx < len(a.emulatorChoices)-1 {
					a.selectedEmulatorIdx++
					a.emulatorList.Select(a.selectedEmulatorIdx)
				}
			} else if a.focusOnGames {
				if a.selectedGameIdx < len(a.filteredGames)-1 {
					a.selectedGameIdx++
					a.gameList.Select(a.selectedGameIdx)
				}
			} else {
				if a.selectedSysIdx < len(systemsList)-1 {
					a.selectedSysIdx++
					a.systemList.Select(a.selectedSysIdx)
				}
			}
			
		case fyne.KeyUp:
			// Up arrow - Move selection up
			if a.choosingEmulator {
				if a.selectedEmulatorIdx > 0 {
					a.selectedEmulatorIdx--
					a.emulatorList.Select(a.selectedEmulatorIdx)
				}
			} else if a.focusOnGames {
				if a.selectedGameIdx > 0 {
					a.selectedGameIdx--
					a.gameList.Select(a.selectedGameIdx)
				}
			} else {
				if a.selectedSysIdx > 0 {
					a.selectedSysIdx--
					a.systemList.Select(a.selectedSysIdx)
				}
			}
			
		case fyne.KeyLeft:
			// Left arrow - Focus on systems or download
			if !a.choosingEmulator && a.focusOnGames {
				a.focusOnGames = false
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
			
		case fyne.KeyRight:
			// Right arrow - Focus on games
			if !a.choosingEmulator && !a.focusOnGames {
				a.focusOnGames = true
				if len(a.filteredGames) > 0 && a.selectedGameIdx < 0 {
					a.selectedGameIdx = 0
					a.gameList.Select(0)
				}
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
			
		case fyne.KeyD:
			// D key - Download selected game
			if a.focusOnGames && !a.choosingEmulator {
				a.downloadSelected()
			}
			
		case fyne.KeyF:
			// F key - Toggle favorite
			if a.focusOnGames && !a.choosingEmulator {
				a.toggleSelectedFavorite()
			}
			
		case fyne.KeyTab:
			// Tab - Toggle between systems and games
			if !a.choosingEmulator {
				a.focusOnGames = !a.focusOnGames
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
			
		case fyne.KeyPageDown:
			// Page Down - Jump down 10 items
			if a.focusOnGames {
				newIdx := a.selectedGameIdx + 10
				if newIdx >= len(a.filteredGames) {
					newIdx = len(a.filteredGames) - 1
				}
				if newIdx >= 0 {
					a.selectedGameIdx = newIdx
					a.gameList.Select(a.selectedGameIdx)
				}
			}
			
		case fyne.KeyPageUp:
			// Page Up - Jump up 10 items
			if a.focusOnGames {
				newIdx := a.selectedGameIdx - 10
				if newIdx < 0 {
					newIdx = 0
				}
				a.selectedGameIdx = newIdx
				a.gameList.Select(a.selectedGameIdx)
			}
			
		case fyne.KeyHome:
			// Home - Jump to first item
			if a.focusOnGames && len(a.filteredGames) > 0 {
				a.selectedGameIdx = 0
				a.gameList.Select(0)
			}
			
		case fyne.KeyEnd:
			// End - Jump to last item
			if a.focusOnGames && len(a.filteredGames) > 0 {
				a.selectedGameIdx = len(a.filteredGames) - 1
				a.gameList.Select(a.selectedGameIdx)
			}
		}
	})

	// Select first system
	if len(systemsList) > 0 {
		a.systemList.Select(0)
	}
}

func (a *App) pollController() {
	// Try to find a working joystick
	var js joystick.Joystick
	var err error
	
	for i := 0; i < 4; i++ {
		js, err = joystick.Open(i)
		if err == nil {
			break
		}
	}
	
	if err != nil {
		// No controller found, that's fine
		return
	}
	defer js.Close()

	var lastButtons uint32
	var lastLeftY, lastRightY int
	leftRepeatTimer := time.Now()
	rightRepeatTimer := time.Now()
	rightHoldStart := time.Time{}
	const initialDelay = 300 * time.Millisecond
	const repeatDelay = 150 * time.Millisecond
	const fastRepeatDelay = 50 * time.Millisecond
	const fastScrollThreshold = 500 * time.Millisecond
	const deadzone = 10000

	// Log controller info once
	logDebug("Controller connected: %d axes, %d buttons", js.AxisCount(), js.ButtonCount())

	for {
		time.Sleep(16 * time.Millisecond) // ~60fps polling

		// Only process controller input when EmuBuddy window is focused
		// Temporarily disabled on macOS for debugging lag issues
		if runtime.GOOS != "darwin" && !isWindowFocused("EmuBuddy") {
			continue
		}

		state, err := js.Read()
		if err != nil {
			continue
		}

		// Debug: Log button presses and axis movements
		if state.Buttons != lastButtons {
			logDebug("Buttons changed: 0x%08X (was 0x%08X)", state.Buttons, lastButtons)
		}

		// Skip if dialog is open
		if a.dialogOpen {
			lastButtons = state.Buttons
			continue
		}

		buttons := state.Buttons

		// Left stick Y axis (axis 1) - controls system list
		leftY := 0
		// Right stick Y axis (axis 3 on most controllers) - controls game list
		rightY := 0

		if len(state.AxisData) >= 2 {
			// Invert Y axis for macOS (positive = down, we want down to scroll down)
			axisValue := state.AxisData[1]
			if runtime.GOOS == "darwin" {
				axisValue = -axisValue
			}
			if axisValue > deadzone {
				leftY = 1
			} else if axisValue < -deadzone {
				leftY = -1
			}
		}

		// Right stick Y axis - axis index differs by platform
		var rightAxisIndex int
		if runtime.GOOS == "linux" {
			rightAxisIndex = 4 // Linux: axis 4 is right Y
		} else {
			rightAxisIndex = 3 // Windows/macOS: axis 3 is right Y
		}

		if len(state.AxisData) > rightAxisIndex {
			axisValue := state.AxisData[rightAxisIndex]
			// Invert Y axis for macOS
			if runtime.GOOS == "darwin" {
				axisValue = -axisValue
			}
			if axisValue > deadzone {
				rightY = 1
			} else if axisValue < -deadzone {
				rightY = -1
			}
		}

		// Remap buttons for macOS (buttons are at different bit positions)
		if runtime.GOOS == "darwin" {
			// macOS button mapping (observed from Xbox controller):
			// bit 11 (0x0800) -> A (bit 0)
			// bit 12 (0x1000) -> B (bit 1)
			// bit 13 (0x2000) -> X (bit 2)
			// bit 14 (0x4000) -> Y (bit 3)
			remapped := uint32(0)
			if buttons&0x0800 != 0 { remapped |= 0x0001 } // A
			if buttons&0x1000 != 0 { remapped |= 0x0002 } // B
			if buttons&0x2000 != 0 { remapped |= 0x0004 } // X
			if buttons&0x4000 != 0 { remapped |= 0x0008 } // Y
			// Keep other bits as-is (Start, Select, etc.)
			remapped |= buttons & 0xFFFF00FF
			buttons = remapped
		}

		// Linux uses standard joystick API mapping (no remapping needed)
		// bit 0 = A, bit 1 = B, bit 2 = X, bit 3 = Y
		// bit 4 = LB, bit 5 = RB, bit 6 = Back/Select, bit 7 = Start

		// Check for new button presses
		justPressed := buttons &^ lastButtons

		// Handle emulator choice mode
		if a.choosingEmulator {
			// A button - confirm choice
			if justPressed&1 != 0 {
				a.confirmEmulatorChoice()
			}
			// B button - cancel
			if justPressed&2 != 0 {
				a.cancelEmulatorChoice()
			}
			// Right stick or D-pad to navigate emulator list
			if rightY != 0 && (rightY != lastRightY || time.Since(rightRepeatTimer) > repeatDelay) {
				newIdx := a.selectedEmulatorIdx + rightY
				if newIdx >= 0 && newIdx < len(a.emulatorChoices) {
					a.selectedEmulatorIdx = newIdx
					a.emulatorList.Select(newIdx)
					a.emulatorList.Refresh()
				}
				rightRepeatTimer = time.Now()
			}
			// D-pad up/down
			if justPressed&4096 != 0 && a.selectedEmulatorIdx > 0 {
				a.selectedEmulatorIdx--
				a.emulatorList.Select(a.selectedEmulatorIdx)
				a.emulatorList.Refresh()
			}
			if justPressed&8192 != 0 && a.selectedEmulatorIdx < len(a.emulatorChoices)-1 {
				a.selectedEmulatorIdx++
				a.emulatorList.Select(a.selectedEmulatorIdx)
				a.emulatorList.Refresh()
			}
			
			lastButtons = buttons
			lastLeftY = leftY
			lastRightY = rightY
			continue
		}

		// A button (bit 0) - Select/Launch
		if justPressed&1 != 0 {
			if a.focusOnGames {
				a.launchSelected()
			} else {
				a.focusOnGames = true
				if len(a.filteredGames) > 0 {
					a.gameList.Select(0)
				}
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
		}

		// B button (bit 1) - Back
		if justPressed&2 != 0 {
			if a.focusOnGames {
				a.focusOnGames = false
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
		}

		// X button (bit 2) - Download
		if justPressed&4 != 0 && a.focusOnGames {
			a.downloadSelected()
		}

		// Y button (bit 3) - Favorite
		if justPressed&8 != 0 && a.focusOnGames {
			a.toggleSelectedFavorite()
		}

		// Start button (bit 7) - Toggle favorites view
		if justPressed&128 != 0 {
			a.showFavsOnly = !a.showFavsOnly
			a.favsCheck.SetChecked(a.showFavsOnly) // Sync checkbox
			a.filterGames()
		}

		// Left stick - navigate systems
		if leftY != 0 {
			// Just started moving or repeat timer elapsed
			if leftY != lastLeftY || time.Since(leftRepeatTimer) > repeatDelay {
				newIdx := a.selectedSysIdx + leftY
				if newIdx >= 0 && newIdx < len(systemsList) {
					a.selectedSysIdx = newIdx
					a.systemList.Select(newIdx)
				}
				leftRepeatTimer = time.Now()
			}
		}

		// Right stick - navigate games with fast scroll
		if rightY != 0 {
			// Track how long stick has been held
			if rightY != lastRightY {
				rightHoldStart = time.Now()
			}
			
			holdDuration := time.Since(rightHoldStart)
			currentRepeatDelay := repeatDelay
			scrollAmount := 1
			
			// Fast scroll after holding for a bit
			if holdDuration > fastScrollThreshold {
				currentRepeatDelay = fastRepeatDelay
				scrollAmount = 5 // Jump 5 items at a time
			}
			
			// Just started moving or repeat timer elapsed
			if rightY != lastRightY || time.Since(rightRepeatTimer) > currentRepeatDelay {
				a.focusOnGames = true
				newIdx := a.selectedGameIdx + (rightY * scrollAmount)
				if newIdx < 0 {
					newIdx = 0
				}
				if newIdx >= len(a.filteredGames) {
					newIdx = len(a.filteredGames) - 1
				}
				if newIdx >= 0 && newIdx < len(a.filteredGames) {
					a.selectedGameIdx = newIdx
					a.gameList.Select(newIdx)
					a.updateStatus()
				}
				rightRepeatTimer = time.Now()
				a.systemList.Refresh()
				a.gameList.Refresh()
			}
		} else {
			rightHoldStart = time.Time{}
		}

		// D-pad navigation as fallback
		// D-pad Up (bit 12)
		if justPressed&4096 != 0 {
			a.navigate(-1)
		}
		// D-pad Down (bit 13)
		if justPressed&8192 != 0 {
			a.navigate(1)
		}
		// D-pad Left (bit 14)
		if justPressed&16384 != 0 {
			if a.selectedSysIdx > 0 {
				a.systemList.Select(a.selectedSysIdx - 1)
			}
		}
		// D-pad Right (bit 15)
		if justPressed&32768 != 0 {
			if a.selectedSysIdx < len(systemsList)-1 {
				a.systemList.Select(a.selectedSysIdx + 1)
			}
		}

		lastButtons = buttons
		lastLeftY = leftY
		lastRightY = rightY
	}
}

func (a *App) navigate(delta int) {
	if a.focusOnGames {
		newIdx := a.selectedGameIdx + delta
		if newIdx >= 0 && newIdx < len(a.filteredGames) {
			a.selectedGameIdx = newIdx
			a.gameList.Select(newIdx)
			a.updateStatus()
		}
	} else {
		newIdx := a.selectedSysIdx + delta
		if newIdx >= 0 && newIdx < len(systemsList) {
			a.systemList.Select(newIdx)
		}
	}
}

func (a *App) selectSystem(sysID string) {
	a.currentSystem = sysID
	config := systems[sysID]

	// Clear existing games before loading new ones
	a.allGames = nil
	
	// Load ROM JSON
	jsonFile := filepath.Join(baseDir, "1g1rsets", config.RomJsonFile)
	
	// Debug logging
	logFile, _ := os.OpenFile(filepath.Join(baseDir, "launcher_debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if logFile != nil {
		logFile.WriteString(fmt.Sprintf("[%s] Selecting system: %s, JSON: %s\n", time.Now().Format("15:04:05"), sysID, jsonFile))
		defer logFile.Close()
	}
	
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		a.statusBar.SetText(fmt.Sprintf("Error: %v", err))
		if logFile != nil {
			logFile.WriteString(fmt.Sprintf("[%s] ERROR reading file: %v\n", time.Now().Format("15:04:05"), err))
		}
		return
	}
	
	if logFile != nil {
		logFile.WriteString(fmt.Sprintf("[%s] Read %d bytes from JSON\n", time.Now().Format("15:04:05"), len(data)))
	}

	if err := json.Unmarshal(data, &a.allGames); err != nil {
		a.statusBar.SetText(fmt.Sprintf("Error: %v", err))
		if logFile != nil {
			logFile.WriteString(fmt.Sprintf("[%s] ERROR parsing JSON: %v\n", time.Now().Format("15:04:05"), err))
		}
		return
	}
	
	if logFile != nil {
		logFile.WriteString(fmt.Sprintf("[%s] Loaded %d games\n", time.Now().Format("15:04:05"), len(a.allGames)))
		// Debug: log first few games with TitleID for Wii U
		for i, game := range a.allGames {
			if i < 5 {
				logFile.WriteString(fmt.Sprintf("[%s] Game[%d]: Name=%s, TitleID=%s, URL=%s\n", 
					time.Now().Format("15:04:05"), i, game.Name, game.TitleID, game.URL))
			}
		}
	}

	// Build ROM cache
	a.buildROMCache()
	a.filterGames()
}

func (a *App) buildROMCache() {
	a.romCache = make(map[string]bool)
	config := systems[a.currentSystem]
	romDir := filepath.Join(romsDir, config.Dir)

	entries, err := os.ReadDir(romDir)
	if err != nil {
		return
	}

	existingFiles := make(map[string]bool)
	existingDirs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			existingDirs[strings.ToLower(entry.Name())] = true
		} else {
			existingFiles[strings.ToLower(entry.Name())] = true
		}
	}

	for _, game := range a.allGames {
		exists := false
		
		// For Wii U games, check for directory with sanitized name
		if config.SpecialDownload == "wiiu" {
			sanitizedName := strings.Map(func(r rune) rune {
				if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
					return '_'
				}
				return r
			}, game.Name)
			if existingDirs[strings.ToLower(sanitizedName)] {
				// Check if the directory has content (code or meta folder)
				gamePath := filepath.Join(romDir, sanitizedName)
				codePath := filepath.Join(gamePath, "code")
				metaPath := filepath.Join(gamePath, "meta")
				if _, err := os.Stat(codePath); err == nil {
					exists = true
				} else if _, err := os.Stat(metaPath); err == nil {
					exists = true
				}
			}
		} else {
			baseName := strings.TrimSuffix(game.Name, ".zip")

			for _, ext := range config.FileExtensions {
				if existingFiles[strings.ToLower(baseName+ext)] {
					exists = true
					break
				}
			}

			if !exists && !config.NeedsExtract {
				if existingFiles[strings.ToLower(game.Name)] {
					exists = true
				}
			}
		}

		a.romCache[game.Name] = exists
	}
}

func (a *App) filterGames() {
	a.filteredGames = []ROM{}
	query := strings.ToLower(a.searchQuery)

	for _, game := range a.allGames {
		// Search filter
		if query != "" && !strings.Contains(strings.ToLower(game.Name), query) {
			continue
		}

		// Favorites filter
		if a.showFavsOnly && !a.isFavorite(game.Name) {
			continue
		}

		a.filteredGames = append(a.filteredGames, game)
	}

	a.gameList.Refresh()
	a.statusBar.SetText(fmt.Sprintf("%d games", len(a.filteredGames)))

	if len(a.filteredGames) > 0 {
		a.gameList.Select(0)
	}
}

func (a *App) isFavorite(gameName string) bool {
	if favorites[a.currentSystem] == nil {
		return false
	}
	return favorites[a.currentSystem][gameName]
}

func (a *App) toggleSelectedFavorite() {
	if a.selectedGameIdx < 0 || a.selectedGameIdx >= len(a.filteredGames) {
		return
	}

	game := a.filteredGames[a.selectedGameIdx]
	if favorites[a.currentSystem] == nil {
		favorites[a.currentSystem] = make(map[string]bool)
	}

	if favorites[a.currentSystem][game.Name] {
		delete(favorites[a.currentSystem], game.Name)
		a.statusBar.SetText("Removed from favorites")
	} else {
		favorites[a.currentSystem][game.Name] = true
		a.statusBar.SetText("Added to favorites")
	}
	saveFavorites()
	a.gameList.Refresh()
}

func (a *App) updateStatus() {
	if a.selectedGameIdx < 0 || a.selectedGameIdx >= len(a.filteredGames) {
		return
	}
	game := a.filteredGames[a.selectedGameIdx]
	name := strings.TrimSuffix(game.Name, ".zip")
	name = strings.TrimSuffix(name, ".chd")

	if a.romCache[game.Name] {
		a.statusBar.SetText(fmt.Sprintf("Ready: %s", name))
	} else {
		a.statusBar.SetText(fmt.Sprintf("Not downloaded: %s (%s)", name, game.Size))
	}
}

func (a *App) updateLaunchButton() {
	if a.selectedGameIdx < 0 || a.selectedGameIdx >= len(a.filteredGames) {
		a.launchBtn.SetText("Launch")
		return
	}
	game := a.filteredGames[a.selectedGameIdx]
	if a.romCache[game.Name] {
		a.launchBtn.SetText("Launch")
	} else {
		a.launchBtn.SetText("Download")
	}
}

func (a *App) launchSelected() {
	if a.selectedGameIdx < 0 || a.selectedGameIdx >= len(a.filteredGames) {
		a.statusBar.SetText("No game selected")
		return
	}

	game := a.filteredGames[a.selectedGameIdx]
	if !a.romCache[game.Name] {
		a.statusBar.SetText("Game not downloaded yet")
		return
	}

	a.launchGame(game)
}

func (a *App) downloadSelected() {
	if a.selectedGameIdx < 0 || a.selectedGameIdx >= len(a.filteredGames) {
		a.statusBar.SetText("No game selected")
		return
	}

	game := a.filteredGames[a.selectedGameIdx]
	if a.romCache[game.Name] {
		a.statusBar.SetText("Already downloaded")
		return
	}

	a.downloadGame(game)
}

func (a *App) launchGame(game ROM) {
	config := systems[a.currentSystem]

	// Count total options
	totalOptions := 0
	
	// Main emulator: either has cores or is standalone
	if len(config.Emulator.Cores) > 0 {
		totalOptions += len(config.Emulator.Cores)
	} else if config.Emulator.Path != "" {
		totalOptions++
	}
	
	// Standalone emulator: either has cores or is standalone
	if config.StandaloneEmulator != nil {
		if len(config.StandaloneEmulator.Cores) > 0 {
			totalOptions += len(config.StandaloneEmulator.Cores)
		} else if config.StandaloneEmulator.Path != "" {
			totalOptions++
		}
	}

	if totalOptions > 1 {
		a.showEmulatorChoice(game, config)
	} else {
		// Single option - launch directly
		args := config.Emulator.Args
		if len(config.Emulator.Cores) == 1 {
			args = []string{"-L", config.Emulator.Cores[0].GetCorePath()}
		}
		a.launchWithEmulator(game, config.Emulator.Path, args)
	}
}

func (a *App) showEmulatorChoice(game ROM, config SystemConfig) {
	a.emulatorChoices = []string{}
	a.emulatorPaths = []string{}
	a.emulatorArgs = [][]string{}

	// Add main emulator options
	if len(config.Emulator.Cores) > 0 {
		// Has cores - add each core as an option
		for _, core := range config.Emulator.Cores {
			a.emulatorChoices = append(a.emulatorChoices, fmt.Sprintf("RetroArch (%s)", core.Name))
			a.emulatorPaths = append(a.emulatorPaths, config.Emulator.Path)
			a.emulatorArgs = append(a.emulatorArgs, []string{"-L", core.GetCorePath()})
		}
	} else if config.Emulator.Path != "" {
		// Standalone emulator (no cores)
		name := config.Emulator.Name
		if name == "" {
			name = "Default Emulator"
		}
		a.emulatorChoices = append(a.emulatorChoices, name)
		a.emulatorPaths = append(a.emulatorPaths, config.Emulator.Path)
		a.emulatorArgs = append(a.emulatorArgs, config.Emulator.Args)
	}

	// Add standalone emulator options
	if config.StandaloneEmulator != nil {
		if len(config.StandaloneEmulator.Cores) > 0 {
			// Has cores - add each core as an option
			for _, core := range config.StandaloneEmulator.Cores {
				a.emulatorChoices = append(a.emulatorChoices, fmt.Sprintf("RetroArch (%s)", core.Name))
				a.emulatorPaths = append(a.emulatorPaths, config.StandaloneEmulator.Path)
				a.emulatorArgs = append(a.emulatorArgs, []string{"-L", core.GetCorePath()})
			}
		} else if config.StandaloneEmulator.Path != "" {
			// Standalone (no cores)
			name := config.StandaloneEmulator.Name
			if name == "" {
				name = "Standalone"
			}
			a.emulatorChoices = append(a.emulatorChoices, name)
			a.emulatorPaths = append(a.emulatorPaths, config.StandaloneEmulator.Path)
			a.emulatorArgs = append(a.emulatorArgs, config.StandaloneEmulator.Args)
		}
	}

	if len(a.emulatorChoices) == 0 {
		return
	}

	// Store pending game and switch to emulator choice mode
	a.pendingGame = game
	a.selectedEmulatorIdx = 0
	a.choosingEmulator = true
	
	// Swap game panel for emulator panel
	a.mainSplit.Trailing = a.emulatorPanel
	a.mainSplit.Refresh()
	a.emulatorList.Select(0)
	a.emulatorList.Refresh()
	
	a.statusBar.SetText(fmt.Sprintf("Choose emulator for: %s", game.Name))
}

func (a *App) cancelEmulatorChoice() {
	a.choosingEmulator = false
	a.mainSplit.Trailing = a.gamePanel
	a.mainSplit.Refresh()
	a.updateStatus()
}

func (a *App) confirmEmulatorChoice() {
	if a.selectedEmulatorIdx >= 0 && a.selectedEmulatorIdx < len(a.emulatorPaths) {
		a.choosingEmulator = false
		a.mainSplit.Trailing = a.gamePanel
		a.mainSplit.Refresh()
		a.launchWithEmulator(a.pendingGame, a.emulatorPaths[a.selectedEmulatorIdx], a.emulatorArgs[a.selectedEmulatorIdx])
	}
}

func (a *App) launchWithEmulator(game ROM, emuPath string, emuArgs []string) {
	config := systems[a.currentSystem]
	romDir := filepath.Join(romsDir, config.Dir)

	// Resolve platform-specific path
	emuPath = resolvePlatformPath(emuPath)
	
	// Handle flatpak on Linux
	isFlatpak := strings.HasPrefix(emuPath, "flatpak:")
	var flatpakAppID string
	if isFlatpak {
		flatpakAppID = strings.TrimPrefix(emuPath, "flatpak:")
		emuPath = "flatpak"
	} else {
		emuPath = filepath.Join(baseDir, emuPath)
	}
	emuDir := filepath.Dir(emuPath)

	// On Linux, ensure AppImages are executable
	if runtime.GOOS == "linux" && strings.HasSuffix(strings.ToLower(emuPath), ".appimage") {
		os.Chmod(emuPath, 0755)
	}

	// Log the resolved path for debugging
	logDebug("Launching with emulator: %s", emuPath)
	logDebug("Emulator dir: %s", emuDir)

	// Find ROM file
	var romPath string
	
	// For Wii U games, the ROM is a directory
	if config.SpecialDownload == "wiiu" {
		sanitizedName := strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
				return '_'
			}
			return r
		}, game.Name)
		romPath = filepath.Join(romDir, sanitizedName)
		
		// For Cemu, we need to point to the rpx file in the code folder
		rpxPath := filepath.Join(romPath, "code")
		if entries, err := os.ReadDir(rpxPath); err == nil {
			for _, entry := range entries {
				if strings.HasSuffix(strings.ToLower(entry.Name()), ".rpx") {
					romPath = filepath.Join(rpxPath, entry.Name())
					break
				}
			}
		}
	} else if config.NeedsExtract {
		baseName := strings.TrimSuffix(game.Name, ".zip")
		for _, ext := range config.FileExtensions {
			testPath := filepath.Join(romDir, baseName+ext)
			if fileExists(testPath) {
				romPath = testPath
				break
			}
		}

		// If not found, look for any file starting with baseName
		if romPath == "" {
			entries, _ := os.ReadDir(romDir)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), baseName) {
					for _, ext := range config.FileExtensions {
						if strings.HasSuffix(strings.ToLower(entry.Name()), strings.ToLower(ext)) {
							romPath = filepath.Join(romDir, entry.Name())
							break
						}
					}
					if romPath != "" {
						break
					}
				}
			}
		}
	}

	if romPath == "" {
		romPath = filepath.Join(romDir, game.Name)
	}

	if !fileExists(romPath) {
		a.statusBar.SetText("ROM not found: " + game.Name)
		return
	}

	// Build args
	args := []string{}

	// For flatpak, add "run" and the app ID first
	if isFlatpak {
		args = append(args, "run", flatpakAppID)
	}

	for _, arg := range emuArgs {
		if strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
			// Resolve platform-specific core paths
			resolvedArg := resolvePlatformPath(arg)
			// If resolved path is absolute, use it directly; otherwise join with emuDir
			if filepath.IsAbs(resolvedArg) {
				args = append(args, resolvedArg)
			} else {
				resolvedPath := filepath.Join(emuDir, resolvedArg)

				// On Linux, verify core file exists
				if runtime.GOOS == "linux" && strings.HasSuffix(strings.ToLower(resolvedPath), ".so") {
					if !fileExists(resolvedPath) {
						logDebug("ERROR: Core file not found: %s", resolvedPath)
						a.statusBar.SetText(fmt.Sprintf("Core not found: %s", filepath.Base(resolvedPath)))
						return
					}
					logDebug("Core file found: %s", resolvedPath)
				}

				args = append(args, resolvedPath)
			}
		} else {
			args = append(args, arg)
		}
	}
	args = append(args, romPath)

	// Log launch command for debugging
	logDebug("Launch command: %s %v", emuPath, args)
	logDebug("Working directory: %s", emuDir)
	logDebug("ROM path: %s", romPath)

	cmd := exec.Command(emuPath, args...)
	if !isFlatpak {
		cmd.Dir = emuDir
	}

	// On Linux, set environment variables to fix AppImage compatibility
	if runtime.GOOS == "linux" {
		// Force SDL to use X11 instead of Wayland (fixes EGL symbol errors)
		cmd.Env = append(os.Environ(),
			"SDL_VIDEODRIVER=x11",
			"QT_QPA_PLATFORM=xcb",
		)

		// Capture stderr to debug log for troubleshooting
		if debugLog != nil {
			cmd.Stderr = debugLog
			cmd.Stdout = debugLog
		}
	}

	if err := cmd.Start(); err != nil {
		logDebug("Failed to start: %v", err)
		a.statusBar.SetText(fmt.Sprintf("Launch failed: %v", err))
		return
	}

	// On Linux, check if process exits immediately (indicates error)
	if runtime.GOOS == "linux" {
		go func() {
			err := cmd.Wait()
			if err != nil {
				logDebug("Process exited with error: %v", err)
			}
		}()
	}

	a.statusBar.SetText("Launched: " + game.Name)
}

func (a *App) downloadGame(game ROM) {
	config := systems[a.currentSystem]
	romDir := filepath.Join(romsDir, config.Dir)
	os.MkdirAll(romDir, 0755)

	// Debug logging
	debugLog, _ := os.OpenFile(filepath.Join(baseDir, "launcher_debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if debugLog != nil {
		debugLog.WriteString(fmt.Sprintf("[%s] downloadGame: Name=%s, TitleID=%s, SpecialDownload=%s\n", 
			time.Now().Format("15:04:05"), game.Name, game.TitleID, config.SpecialDownload))
		debugLog.Close()
	}

	// Handle Wii U special download
	if config.SpecialDownload == "wiiu" && game.TitleID != "" {
		a.downloadWiiUGame(game)
		return
	}

	outputPath := filepath.Join(romDir, game.Name)

	progressBar := widget.NewProgressBar()
	progressLabel := widget.NewLabel("Starting download...")

	progressContent := container.NewVBox(
		widget.NewLabel(game.Name),
		progressBar,
		progressLabel,
	)

	progressDialog := dialog.NewCustom("Downloading", "Cancel", progressContent, a.window)
	cancelled := false
	progressDialog.SetOnClosed(func() {
		cancelled = true
	})
	progressDialog.Show()

	go func() {
		err := downloadWithProgress(game.URL, outputPath, func(downloaded, total int64) {
			if cancelled {
				return
			}
			if total > 0 {
				pct := float64(downloaded) / float64(total)
				progressBar.SetValue(pct)
				progressLabel.SetText(fmt.Sprintf("%.1f MB / %.1f MB", float64(downloaded)/1024/1024, float64(total)/1024/1024))
			}
		})

		if cancelled {
			os.Remove(outputPath)
			return
		}

		if err != nil {
			progressDialog.Hide()
			dialog.ShowError(err, a.window)
			return
		}

		// Extract if needed
		if config.NeedsExtract && strings.HasSuffix(game.Name, ".zip") {
			progressLabel.SetText("Extracting...")
			extractZip(outputPath, romDir)
			os.Remove(outputPath)
		}

		progressDialog.Hide()
		a.romCache[game.Name] = true
		a.gameList.Refresh()
		a.statusBar.SetText("Downloaded: " + game.Name)
	}()
}

// WiiUProgressReporter implements the wiiu.ProgressReporter interface
type WiiUProgressReporter struct {
	progressBar    *widget.ProgressBar
	progressLabel  *widget.Label
	downloadLabel  *widget.Label
	gameTitle      string
	cancelled      bool
	downloadSize   int64
	mu             sync.Mutex
	fileProgress   map[string]int64
	totalDownloaded int64
	startTime      time.Time
}

func NewWiiUProgressReporter(progressBar *widget.ProgressBar, progressLabel, downloadLabel *widget.Label) *WiiUProgressReporter {
	return &WiiUProgressReporter{
		progressBar:   progressBar,
		progressLabel: progressLabel,
		downloadLabel: downloadLabel,
		fileProgress:  make(map[string]int64),
	}
}

func (r *WiiUProgressReporter) SetGameTitle(title string) {
	r.mu.Lock()
	r.gameTitle = title
	r.mu.Unlock()
}

func (r *WiiUProgressReporter) UpdateDownloadProgress(downloaded int64, filename string) {
	r.mu.Lock()
	r.fileProgress[filename] = downloaded
	var total int64
	for _, v := range r.fileProgress {
		total += v
	}
	r.totalDownloaded = total
	r.mu.Unlock()

	if r.downloadSize > 0 {
		pct := float64(total) / float64(r.downloadSize)
		r.progressBar.SetValue(pct)
		r.progressLabel.SetText(fmt.Sprintf("%.1f MB / %.1f MB", float64(total)/1024/1024, float64(r.downloadSize)/1024/1024))
	}
}

func (r *WiiUProgressReporter) UpdateDecryptionProgress(progress float64) {
	r.progressBar.SetValue(progress)
	r.progressLabel.SetText(fmt.Sprintf("Decrypting... %.0f%%", progress*100))
}

func (r *WiiUProgressReporter) Cancelled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelled
}

func (r *WiiUProgressReporter) SetCancelled() {
	r.mu.Lock()
	r.cancelled = true
	r.mu.Unlock()
}

func (r *WiiUProgressReporter) SetDownloadSize(size int64) {
	r.mu.Lock()
	r.downloadSize = size
	r.mu.Unlock()
}

func (r *WiiUProgressReporter) ResetTotals() {
	r.mu.Lock()
	r.fileProgress = make(map[string]int64)
	r.totalDownloaded = 0
	r.mu.Unlock()
}

func (r *WiiUProgressReporter) MarkFileAsDone(filename string) {
	// No-op for now
}

func (r *WiiUProgressReporter) SetTotalDownloadedForFile(filename string, downloaded int64) {
	r.mu.Lock()
	r.fileProgress[filename] = downloaded
	r.mu.Unlock()
}

func (r *WiiUProgressReporter) SetStartTime(startTime time.Time) {
	r.mu.Lock()
	r.startTime = startTime
	r.mu.Unlock()
}

func (a *App) downloadWiiUGame(game ROM) {
	config := systems[a.currentSystem]
	
	// Use a sanitized directory name based on the game name
	sanitizedName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, game.Name)
	
	romDir := filepath.Join(romsDir, config.Dir, sanitizedName)
	os.MkdirAll(romDir, 0755)

	progressBar := widget.NewProgressBar()
	progressLabel := widget.NewLabel("Starting download...")
	downloadLabel := widget.NewLabel(game.Name)

	progressContent := container.NewVBox(
		downloadLabel,
		progressBar,
		progressLabel,
	)

	progressDialog := dialog.NewCustom("Downloading Wii U Title", "Cancel", progressContent, a.window)
	reporter := NewWiiUProgressReporter(progressBar, progressLabel, downloadLabel)

	progressDialog.SetOnClosed(func() {
		reporter.SetCancelled()
	})
	progressDialog.Show()

	go func() {
		client := &http.Client{Timeout: 0} // No timeout for large downloads

		// Download and decrypt
		progressLabel.SetText("Downloading from Nintendo CDN...")
		err := wiiu.DownloadTitle(game.TitleID, romDir, true, reporter, true, client)

		if reporter.Cancelled() {
			os.RemoveAll(romDir)
			return
		}

		if err != nil {
			progressDialog.Hide()
			dialog.ShowError(err, a.window)
			os.RemoveAll(romDir)
			return
		}

		progressDialog.Hide()
		a.romCache[game.Name] = true
		a.gameList.Refresh()
		a.statusBar.SetText("Downloaded: " + game.Name)
	}()
}

// Parallel download configuration
const (
	numDownloadWorkers = 4               // Number of parallel connections (reduced to avoid rate limiting)
	minChunkSize       = 4 * 1024 * 1024 // 4MB minimum chunk size
	maxChunkRetries    = 3               // Retries per chunk on failure
)

func downloadWithProgress(url, outputPath string, progress func(downloaded, total int64)) error {
	// First, get file size and check for Range support
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		WriteBufferSize:     1024 * 1024,
		ReadBufferSize:      1024 * 1024,
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,
	}
	client := &http.Client{Transport: transport}

	// HEAD request to get file info
	headReq, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	headReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	
	headResp, err := client.Do(headReq)
	if err != nil {
		return err
	}
	headResp.Body.Close()

	totalSize := headResp.ContentLength
	supportsRange := headResp.Header.Get("Accept-Ranges") == "bytes"

	// Use parallel download for large files that support Range requests
	if supportsRange && totalSize > minChunkSize*2 {
		return downloadParallel(client, url, outputPath, totalSize, progress)
	}

	// Fall back to single-threaded download
	return downloadSingle(client, url, outputPath, progress)
}

func downloadParallel(client *http.Client, url, outputPath string, totalSize int64, progress func(downloaded, total int64)) error {
	// Calculate chunk size
	chunkSize := totalSize / int64(numDownloadWorkers)
	if chunkSize < minChunkSize {
		chunkSize = minChunkSize
	}

	// Create output file
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Pre-allocate file
	if err := out.Truncate(totalSize); err != nil {
		return err
	}

	// Progress tracking - track per-chunk progress to handle retries correctly
	chunkProgress := make(map[int64]int64) // start position -> bytes downloaded
	var progressMu sync.Mutex
	
	updateProgress := func(chunkStart int64, totalForChunk int64) {
		progressMu.Lock()
		chunkProgress[chunkStart] = totalForChunk
		var total int64
		for _, v := range chunkProgress {
			total += v
		}
		progressMu.Unlock()
		progress(total, totalSize)
	}

	// Create worker pool
	type chunk struct {
		start, end int64
	}
	chunks := make(chan chunk, numDownloadWorkers*2)
	errChan := make(chan error, numDownloadWorkers)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numDownloadWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range chunks {
				if err := downloadChunk(client, url, out, c.start, c.end, updateProgress); err != nil {
					errChan <- err
					return
				}
			}
		}()
	}

	// Queue chunks
	for start := int64(0); start < totalSize; start += chunkSize {
		end := start + chunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		chunks <- chunk{start, end}
	}
	close(chunks)

	// Wait for completion
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			os.Remove(outputPath)
			return err
		}
	}

	return nil
}

func downloadChunk(client *http.Client, url string, out *os.File, start, end int64, updateProgress func(chunkStart int64, totalForChunk int64)) error {
	var lastErr error
	
	for attempt := 0; attempt < maxChunkRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second) // Backoff: 1s, 2s
			// Reset progress for this chunk on retry
			updateProgress(start, 0)
		}
		
		err := downloadChunkAttempt(client, url, out, start, end, updateProgress)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("chunk %d-%d failed after %d retries: %w", start, end, maxChunkRetries, lastErr)
}

func downloadChunkAttempt(client *http.Client, url string, out *os.File, start, end int64, updateProgress func(chunkStart int64, totalForChunk int64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for range %d-%d", resp.StatusCode, start, end)
	}

	buf := make([]byte, 256*1024) // 256KB read buffer
	pos := start
	var chunkDownloaded int64
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.WriteAt(buf[:n], pos)
			if writeErr != nil {
				return writeErr
			}
			pos += int64(n)
			chunkDownloaded += int64(n)
			updateProgress(start, chunkDownloaded)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func downloadSingle(client *http.Client, url, outputPath string, progress func(downloaded, total int64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	bufferedOut := bufio.NewWriterSize(out, 1024*1024)
	defer bufferedOut.Flush()

	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 1024*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			bufferedOut.Write(buf[:n])
			downloaded += int64(n)
			progress(downloaded, total)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func extractZip(zipPath, destDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	var extractedFile string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		destPath := filepath.Join(destDir, f.Name)
		os.MkdirAll(filepath.Dir(destPath), 0755)

		outFile, err := os.Create(destPath)
		if err != nil {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			continue
		}

		io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		extractedFile = destPath
	}
	return extractedFile, nil
}

// Ensure Windows doesn't need console
func init() {
	if runtime.GOOS == "windows" {
		// Hide console on Windows - handled by ldflags
	}
}
