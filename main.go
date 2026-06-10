package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetDesktopWindow     = user32.NewProc("GetDesktopWindow")
)

type CaptureSource struct {
	ID   string
	Name string
	Type string // "screen" or "window"
}

type AppState struct {
	selectedSource *CaptureSource
	isRecording    bool
	startTime      time.Time
	timerTicker    *time.Ticker
	timerStopChan  chan bool
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("BitStream - Screen Recorder")
	myWindow.Resize(fyne.NewSize(900, 600))

	state := &AppState{
		timerStopChan: make(chan bool),
	}

	// 1. Sidebar Controls
	statusLabel := widget.NewLabel("Status: Ready")
	statusLabel.Alignment = fyne.TextAlignCenter

	selectedLabel := widget.NewLabel("No source selected")
	selectedLabel.TextStyle = fyne.TextStyle{Bold: true}

	// 2. Main content area (Source list)
	sourceList := widget.NewList(
		func() int { return 0 },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.ComputerIcon()),
				widget.NewLabel("Source Name"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {},
	)

	var sources []CaptureSource

	refreshSources := func() {
		sources = getActiveSources()
		sourceList.Length = func() int { return len(sources) }
		sourceList.UpdateItem = func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(sources) {
				return
			}
			src := sources[id]
			box := obj.(*fyne.Container)
			icon := box.Objects[0].(*widget.Icon)
			label := box.Objects[1].(*widget.Label)

			label.SetText(src.Name)
			if src.Type == "screen" {
				icon.SetResource(theme.ComputerIcon())
			} else {
				icon.SetResource(theme.WindowIcon())
			}
		}
		sourceList.Refresh()
	}

	refreshSources()

	// 3. Setup interaction on source selection
	sourceList.OnSelected = func(id widget.ListItemID) {
		if id < len(sources) {
			src := sources[id]
			state.selectedSource = &src
			selectedLabel.SetText(fmt.Sprintf("Selected: %s (%s)", src.Name, src.Type))
		}
	}

	btnRefresh := widget.NewButtonWithIcon("Refresh Sources", theme.ViewRefreshIcon(), func() {
		refreshSources()
	})

	btnOpenFolder := widget.NewButtonWithIcon("Recordings Folder", theme.FolderOpenIcon(), func() {
		dir, _ := filepath.Abs("./recordings")
		_ = os.MkdirAll(dir, 0755)
		if runtime.GOOS == "windows" {
			_ = exec.Command("explorer", dir).Start()
		} else {
			_ = exec.Command("open", dir).Start()
		}
	})

	// 4. Timer & Record controls
	timerLabel := widget.NewLabel("00:00")
	timerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	btnRecord := widget.NewButtonWithIcon("Start Recording", theme.MediaRecordIcon(), nil)
	btnRecord.Importance = widget.HighImportance

	btnRecord.OnTapped = func() {
		if state.selectedSource == nil {
			statusLabel.SetText("Error: Please select a source first")
			return
		}

		if !state.isRecording {
			// Start Recording
			state.isRecording = true
			state.startTime = time.Now()
			btnRecord.SetText("Stop Recording")
			btnRecord.SetIcon(theme.MediaStopIcon())
			statusLabel.SetText("Recording...")

			// Start UI Timer
			state.timerTicker = time.NewTicker(1 * time.Second)
			go func() {
				for {
					select {
					case <-state.timerTicker.C:
						elapsed := time.Since(state.startTime)
						minutes := int(elapsed.Minutes()) % 60
						seconds := int(elapsed.Seconds()) % 60
						timerLabel.SetText(fmt.Sprintf("%02d:%02d", minutes, seconds))
					case <-state.timerStopChan:
						return
					}
				}
			}()

			// TODO: Trigger native recorder capture logic
		} else {
			// Stop Recording
			state.isRecording = false
			state.timerTicker.Stop()
			state.timerStopChan <- true
			btnRecord.SetText("Start Recording")
			btnRecord.SetIcon(theme.MediaRecordIcon())
			statusLabel.SetText("Recording Saved")
			timerLabel.SetText("00:00")
		}
	}

	// 5. Sidebar layout
	sidebar := container.NewVBox(
		widget.NewLabelWithStyle("BitStream", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		btnRefresh,
		btnOpenFolder,
		widget.NewSeparator(),
		selectedLabel,
		statusLabel,
	)

	// Bottom control bar layout
	bottomBar := container.NewHBox(
		layout.NewSpacer(),
		timerLabel,
		btnRecord,
		layout.NewSpacer(),
	)

	// Border layout
	content := container.NewBorder(
		nil,
		bottomBar,
		sidebar,
		nil,
		sourceList,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

func getActiveSources() []CaptureSource {
	var sources []CaptureSource

	if runtime.GOOS != "windows" {
		sources = append(sources, CaptureSource{ID: "screen_0", Name: "Primary Monitor (Fallback)", Type: "screen"})
		return sources
	}

	// 1. Desktop Monitor
	desktopHwnd, _, _ := procGetDesktopWindow.Call()
	if desktopHwnd != 0 {
		sources = append(sources, CaptureSource{
			ID:   fmt.Sprintf("screen_%d", desktopHwnd),
			Name: "Entire Desktop Screen",
			Type: "screen",
		})
	}

	// 2. Enumerate Windows
	cb := syscall.NewCallback(func(hwnd syscall.Handle, lparam uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if visible == 0 {
			return 1
		}

		length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
		if length == 0 {
			return 1
		}

		buf := make([]uint16, length+1)
		procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), length+1)
		title := syscall.UTF16ToString(buf)

		if title == "" || title == "Program Manager" || title == "BitStream" {
			return 1
		}

		sources = append(sources, CaptureSource{
			ID:   fmt.Sprintf("window_%d", hwnd),
			Name: title,
			Type: "window",
		})

		return 1
	})

	procEnumWindows.Call(cb, 0)

	return sources
}
