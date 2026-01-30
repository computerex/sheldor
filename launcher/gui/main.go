package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/0xcafed00d/joystick"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
)

func isWindowFocused(windowTitle string) bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	
	buf := make([]uint16, 256)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	title := syscall.UTF16ToString(buf)
	
	return strings.Contains(title, windowTitle)
}

// DoubleTappableList wraps a widget.List to add double-tap support
type DoubleTappableList struct {
	widget.BaseWidget
	List          *widget.List
	OnDoubleTap   func()
	lastTapTime   time.Time
}

func NewDoubleTappableList(list *widget.List, onDoubleTap func()) *DoubleTappableList {
	d := &DoubleTappableList{
		List:        list,
		OnDoubleTap: onDoubleTap,
	}
	d.ExtendBaseWidget(d)
	return d
}

func (d *DoubleTappableList) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(d.List)
}

func (d *DoubleTappableList) Tapped(e *fyne.PointEvent) {
	now := time.Now()
	if now.Sub(d.lastTapTime) < 400*time.Millisecond {
		if d.OnDoubleTap != nil {
			d.OnDoubleTap()
		}
		d.lastTapTime = time.Time{} // Reset
	} else {
		d.lastTapTime = now
	}
}

func (d *DoubleTappableList) TappedSecondary(e *fyne.PointEvent) {}

type ROM struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size string `json:"size"`
	Date string `json:"date"`
}

type CoreConfig struct {
	Name string `json:"name"`
	Dll  string `json:"dll"`
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
	tappableGameList  *DoubleTappableList
	statusBar         *widget.Label
	searchEntry       *widget.Entry
	searchQuery       string
	instructions      *widget.Label
	favsCheck         *widget.Check
	
	// Emulator choice UI
	emulatorList *widget.List
	mainSplit    *container.Split
	gamePanel    *fyne.Container
	emulatorPanel *fyne.Container
}

func main() {
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
	go appState.pollController()
	myWindow.ShowAndRun()
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

	// Game list on right
	a.gameList = widget.NewList(
		func() int { return len(a.filteredGames) },
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("Game Name Here")
			statusLabel := widget.NewLabel("Status")
			sizeLabel := widget.NewLabel("Size")
			return container.NewBorder(nil, nil, nil,
				container.NewHBox(statusLabel, sizeLabel),
				nameLabel,
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(a.filteredGames) {
				return
			}
			game := a.filteredGames[id]
			box := item.(*fyne.Container)

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
		a.gameList.Refresh()
		a.systemList.Refresh()
	}
	
	// Wrap game list with double-tap support
	a.tappableGameList = NewDoubleTappableList(a.gameList, func() {
		a.launchSelected()
	})

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

	// Emulator choice list
	a.emulatorList = widget.NewList(
		func() int { return len(a.emulatorChoices) },
		func() fyne.CanvasObject {
			return widget.NewLabel("Emulator Option")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(a.emulatorChoices) {
				return
			}
			label := item.(*widget.Label)
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
	
	// Game panel with header, favorites checkbox, and search
	gamesLabel := widget.NewLabel("GAMES")
	gameHeader := container.NewBorder(nil, nil,
		container.NewHBox(gamesLabel, a.favsCheck),
		nil,
		a.searchEntry,
	)
	a.gamePanel = container.NewBorder(
		gameHeader, nil, nil, nil,
		a.tappableGameList,
	)

	// Emulator choice panel
	emulatorHeader := widget.NewLabel("CHOOSE EMULATOR (A=Select, B=Cancel)")
	emulatorHeader.TextStyle = fyne.TextStyle{Bold: true}
	a.emulatorPanel = container.NewBorder(
		emulatorHeader, nil, nil, nil,
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

	for {
		time.Sleep(16 * time.Millisecond) // ~60fps polling

		// Only process controller input when EmuBuddy window is focused
		if !isWindowFocused("EmuBuddy") {
			continue
		}

		state, err := js.Read()
		if err != nil {
			continue
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
			if state.AxisData[1] > deadzone {
				leftY = 1
			} else if state.AxisData[1] < -deadzone {
				leftY = -1
			}
		}
		
		if len(state.AxisData) >= 4 {
			if state.AxisData[3] > deadzone {
				rightY = 1
			} else if state.AxisData[3] < -deadzone {
				rightY = -1
			}
		}

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
	for _, entry := range entries {
		if !entry.IsDir() {
			existingFiles[strings.ToLower(entry.Name())] = true
		}
	}

	for _, game := range a.allGames {
		exists := false
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
			args = []string{"-L", config.Emulator.Cores[0].Dll}
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
			a.emulatorArgs = append(a.emulatorArgs, []string{"-L", core.Dll})
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
				a.emulatorArgs = append(a.emulatorArgs, []string{"-L", core.Dll})
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
	emuPath = filepath.Join(baseDir, emuPath)
	emuDir := filepath.Dir(emuPath)

	// Find ROM file
	var romPath string
	if config.NeedsExtract {
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
	for _, arg := range emuArgs {
		if strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
			args = append(args, filepath.Join(emuDir, arg))
		} else {
			args = append(args, arg)
		}
	}
	args = append(args, romPath)

	cmd := exec.Command(emuPath, args...)
	cmd.Dir = emuDir

	if err := cmd.Start(); err != nil {
		a.statusBar.SetText(fmt.Sprintf("Launch failed: %v", err))
		return
	}

	a.statusBar.SetText("Launched: " + game.Name)
}

func (a *App) downloadGame(game ROM) {
	config := systems[a.currentSystem]
	romDir := filepath.Join(romsDir, config.Dir)
	os.MkdirAll(romDir, 0755)

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

func downloadWithProgress(url, outputPath string, progress func(downloaded, total int64)) error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

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

	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
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
