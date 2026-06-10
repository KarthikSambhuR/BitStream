package main

import (
	"context"
	"fmt"
	"image"
	draw "golang.org/x/image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Win32 API functions
var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	gdi32                      = syscall.NewLazyDLL("gdi32.dll")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetWindowRect        = user32.NewProc("GetWindowRect")
	procGetDC                = user32.NewProc("GetDC")
	procReleaseDC            = user32.NewProc("ReleaseDC")
	procGetDesktopWindow     = user32.NewProc("GetDesktopWindow")
	procCreateCompatibleDC   = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC             = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap= gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject         = gdi32.NewProc("SelectObject")
	procBitBlt               = gdi32.NewProc("BitBlt")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procGetDIBits            = gdi32.NewProc("GetDIBits")
)

type RECT struct {
	Left, Top, Right, Bottom int32
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	Bih BITMAPINFOHEADER
}

type CaptureSource struct {
	HWND     syscall.Handle
	Name     string
	Type     string // "screen" or "window"
	Snapshot image.Image
}

type AppState struct {
	selectedSource   *CaptureSource
	isRecording      bool
	startTime        time.Time
	timerTicker      *time.Ticker
	timerStopChan    chan bool
	previewCancel    context.CancelFunc
	previewMutex     sync.Mutex
	previewImgWidget *canvas.Image

	// Reusable buffers to minimize memory allocations and GC overhead
	gdiBuffer    []byte
	previewImage *image.RGBA
}

func main() {
	// Restrict GC behavior to run frequently and keep memory usage extremely low (target 20-30MB)
	debug.SetGCPercent(20)

	myApp := app.New()
	myWindow := myApp.NewWindow("BitStream - Screen Recorder")
	myWindow.Resize(fyne.NewSize(1000, 650))

	state := &AppState{
		timerStopChan: make(chan bool),
		gdiBuffer:     make([]byte, 0, 1920*1080*4), // Pre-allocate buffer for full HD
	}

	// Stable Left Sidebar Layout
	sidebarTitle := widget.NewLabelWithStyle("BitStream", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	statusLabel := widget.NewLabel("Status: Ready")
	statusLabel.Alignment = fyne.TextAlignCenter

	selectedLabel := widget.NewLabel("No source selected")
	selectedLabel.TextStyle = fyne.TextStyle{Bold: true}
	selectedLabel.Wrapping = fyne.TextTruncate

	// Selected live preview canvas
	state.previewImgWidget = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	state.previewImgWidget.FillMode = canvas.ImageFillContain
	state.previewImgWidget.SetMinSize(fyne.NewSize(480, 270))

	previewContainer := container.NewMax(state.previewImgWidget)

	// Grid containers for tabs
	screensGrid := container.New(layout.NewGridWrapLayout(fyne.NewSize(220, 160)))
	windowsGrid := container.New(layout.NewGridWrapLayout(fyne.NewSize(220, 160)))

	screensScroll := container.NewVScroll(screensGrid)
	windowsScroll := container.NewVScroll(windowsGrid)

	// Callback to update selected source and spawn live preview loop
	onSourceClicked := func(src CaptureSource) {
		state.previewMutex.Lock()
		defer state.previewMutex.Unlock()

		state.selectedSource = &src
		selectedLabel.SetText(fmt.Sprintf("Selected: %s", src.Name))

		// Cancel any existing preview loop
		if state.previewCancel != nil {
			state.previewCancel()
		}

		// Setup new cancellation context for live loop
		ctx, cancel := context.WithCancel(context.Background())
		state.previewCancel = cancel

		// Live preview loop: pulls frames at ~20 FPS (50ms interval)
		go func(targetCtx context.Context, hwnd syscall.Handle) {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()

			// Force immediate GC to clear unused structures on source selection change
			runtime.GC()

			for {
				select {
				case <-targetCtx.Done():
					return
				case <-ticker.C:
					// Run GDI capture using reusable state buffers
					img, err := state.captureSourceFrame(hwnd, 960) // High quality 960 max dimension with correct aspect ratio
					if err == nil && img != nil {
						// Thread-safe Fyne UI updates wrapped in fyne.Do
						fyne.Do(func() {
							state.previewImgWidget.Image = img
							state.previewImgWidget.Refresh()
						})
					}
				}
			}
		}(ctx, src.HWND)
	}

	refreshSources := func() {
		sources := getActiveSources()

		screensGrid.Objects = nil
		windowsGrid.Objects = nil

		for _, src := range sources {
			currentSrc := src

			var imgWidget *canvas.Image
			if src.Snapshot != nil {
				imgWidget = canvas.NewImageFromImage(src.Snapshot)
			} else {
				imgWidget = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
			}
			imgWidget.FillMode = canvas.ImageFillContain
			imgWidget.SetMinSize(fyne.NewSize(200, 112))

			nameLabel := widget.NewLabel(src.Name)
			nameLabel.Alignment = fyne.TextAlignCenter
			nameLabel.Wrapping = fyne.TextTruncate

			cardBtn := widget.NewButton("", func() {
				onSourceClicked(currentSrc)
			})

			cardContent := container.NewBorder(
				nil,
				nameLabel,
				nil,
				nil,
				container.NewMax(imgWidget, cardBtn),
			)

			if src.Type == "screen" {
				screensGrid.Add(cardContent)
			} else {
				windowsGrid.Add(cardContent)
			}
		}

		screensGrid.Refresh()
		windowsGrid.Refresh()
		runtime.GC() // Tidy up memory after loading snapshots
	}

	// Initial Refresh
	refreshSources()

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

	// Sidebar container (fixed width elements)
	sidebar := container.NewVBox(
		sidebarTitle,
		widget.NewSeparator(),
		btnRefresh,
		btnOpenFolder,
		widget.NewSeparator(),
		selectedLabel,
		statusLabel,
	)

	// Fixed width layout constraint to stop layout shifting around
	sidebarBox := container.NewHBox(
		container.NewGridWrap(fyne.NewSize(220, 500), sidebar),
		widget.NewSeparator(),
	)

	// Setup Screen and Window tabs
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Screens", theme.ComputerIcon(), screensScroll),
		container.NewTabItemWithIcon("Windows", theme.FileApplicationIcon(), windowsScroll),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Center splits: tabs selector on the left, active live preview on the right
	centerSplit := container.NewHSplit(
		tabs,
		previewContainer,
	)
	centerSplit.SetOffset(0.6) // 60% list, 40% preview

	// Timer & Recording Controls
	timerLabel := widget.NewLabel("00:00")
	timerLabel.TextStyle = fyne.TextStyle{Monospace: true}

	btnRecord := widget.NewButtonWithIcon("Start Recording", theme.MediaRecordIcon(), nil)
	btnRecord.Importance = widget.HighImportance

	btnRecord.OnTapped = func() {
		if state.selectedSource == nil {
			statusLabel.SetText("Error: Select a source")
			return
		}

		if !state.isRecording {
			state.isRecording = true
			state.startTime = time.Now()
			btnRecord.SetText("Stop Recording")
			btnRecord.SetIcon(theme.MediaStopIcon())
			statusLabel.SetText("Recording...")

			state.timerTicker = time.NewTicker(1 * time.Second)
			go func() {
				for {
					select {
					case <-state.timerTicker.C:
						elapsed := time.Since(state.startTime)
						minutes := int(elapsed.Minutes()) % 60
						seconds := int(elapsed.Seconds()) % 60
						fyne.Do(func() {
							timerLabel.SetText(fmt.Sprintf("%02d:%02d", minutes, seconds))
						})
					case <-state.timerStopChan:
						return
					}
				}
			}()
		} else {
			state.isRecording = false
			state.timerTicker.Stop()
			state.timerStopChan <- true
			btnRecord.SetText("Start Recording")
			btnRecord.SetIcon(theme.MediaRecordIcon())
			statusLabel.SetText("Saved")
			fyne.Do(func() {
				timerLabel.SetText("00:00")
			})
		}
	}

	bottomBar := container.NewHBox(
		layout.NewSpacer(),
		timerLabel,
		btnRecord,
		layout.NewSpacer(),
	)

	content := container.NewBorder(
		nil,
		bottomBar,
		sidebarBox,
		nil,
		centerSplit,
	)

	myWindow.SetContent(content)
	
	// Terminate any preview threads cleanly when window closes
	myWindow.SetOnClosed(func() {
		state.previewMutex.Lock()
		if state.previewCancel != nil {
			state.previewCancel()
		}
		state.previewMutex.Unlock()
	})

	// Run memory collector in background periodically to maintain <25MB RAM footprint
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runtime.GC()
			debug.FreeOSMemory()
		}
	}()

	myWindow.ShowAndRun()
}

func getActiveSources() []CaptureSource {
	var sources []CaptureSource

	if runtime.GOOS != "windows" {
		sources = append(sources, CaptureSource{HWND: 0, Name: "Primary Screen (Mock)", Type: "screen"})
		return sources
	}

	// 1. Screens / Monitors
	desktopHwnd, _, _ := procGetDesktopWindow.Call()
	if desktopHwnd != 0 {
		// Temporary instance for snapshot refresh (not locked to loop)
		tempState := &AppState{gdiBuffer: make([]byte, 0, 1920*1080*4)}
		snap, _ := tempState.captureSourceFrame(syscall.Handle(desktopHwnd), 320)
		sources = append(sources, CaptureSource{
			HWND:     syscall.Handle(desktopHwnd),
			Name:     "Entire Desktop Screen",
			Type:     "screen",
			Snapshot: snap,
		})
	}

	// 2. Active Windows
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

		if title == "" || title == "Program Manager" || title == "BitStream" || title == "Settings" {
			return 1
		}

		tempState := &AppState{gdiBuffer: make([]byte, 0, 1280*720*4)}
		snap, _ := tempState.captureSourceFrame(hwnd, 320)
		sources = append(sources, CaptureSource{
			HWND:     hwnd,
			Name:     title,
			Type:     "window",
			Snapshot: snap,
		})

		return 1
	})

	procEnumWindows.Call(cb, 0)

	return sources
}

// captureSourceFrame grabs a GDI snapshot, handles aspect ratio dynamically, and uses reusable buffers.
func (state *AppState) captureSourceFrame(hwnd syscall.Handle, maxDimension int) (image.Image, error) {
	var rect RECT
	ret, _, _ := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return nil, fmt.Errorf("rect failed")
	}

	width := rect.Right - rect.Left
	height := rect.Bottom - rect.Top

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid size")
	}

	// Calculate correct aspect-ratio scaling
	aspect := float64(height) / float64(width)
	var targetWidth, targetHeight int
	if width > height {
		targetWidth = maxDimension
		targetHeight = int(float64(maxDimension) * aspect)
	} else {
		targetHeight = maxDimension
		targetWidth = int(float64(maxDimension) / aspect)
	}

	if targetWidth <= 0 {
		targetWidth = 1
	}
	if targetHeight <= 0 {
		targetHeight = 1
	}

	hdcSrc, _, _ := procGetDC.Call(uintptr(hwnd))
	if hdcSrc == 0 {
		return nil, fmt.Errorf("get DC failed")
	}
	defer procReleaseDC.Call(uintptr(hwnd), hdcSrc)

	hdcDest, _, _ := procCreateCompatibleDC.Call(hdcSrc)
	if hdcDest == 0 {
		return nil, fmt.Errorf("compatible DC failed")
	}
	defer procDeleteDC.Call(hdcDest)

	hBitmap, _, _ := procCreateCompatibleBitmap.Call(hdcSrc, uintptr(width), uintptr(height))
	if hBitmap == 0 {
		return nil, fmt.Errorf("bitmap failed")
	}
	defer procDeleteObject.Call(hBitmap)

	oldObj, _, _ := procSelectObject.Call(hdcDest, hBitmap)
	defer procSelectObject.Call(hdcDest, oldObj)

	ret, _, _ = procBitBlt.Call(hdcDest, 0, 0, uintptr(width), uintptr(height), hdcSrc, 0, 0, 0x00CC0020)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	var bi BITMAPINFO
	bi.Bih.BiSize = uint32(unsafe.Sizeof(bi.Bih))
	bi.Bih.BiWidth = width
	bi.Bih.BiHeight = -height
	bi.Bih.BiPlanes = 1
	bi.Bih.BiBitCount = 32
	bi.Bih.BiCompression = 0

	// Reuse slice buffer instead of re-allocating
	size := int(width * height * 4)
	if cap(state.gdiBuffer) < size {
		state.gdiBuffer = make([]byte, size)
	} else {
		state.gdiBuffer = state.gdiBuffer[:size]
	}

	ret, _, _ = procGetDIBits.Call(
		hdcDest,
		hBitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&state.gdiBuffer[0])),
		uintptr(unsafe.Pointer(&bi)),
		0,
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	// Normalize color channels in-place: BGRA -> RGBA
	for i := 0; i < size; i += 4 {
		b := state.gdiBuffer[i]
		state.gdiBuffer[i] = state.gdiBuffer[i+2]
		state.gdiBuffer[i+2] = b
	}

	// Build raw RGBA structure pointing to our pre-allocated slice
	rgba := &image.RGBA{
		Pix:    state.gdiBuffer,
		Stride: int(width) * 4,
		Rect:   image.Rect(0, 0, int(width), int(height)),
	}

	// Reuse target previewImage canvas buffer instead of re-allocating
	if state.previewImage == nil || state.previewImage.Bounds().Dx() != targetWidth || state.previewImage.Bounds().Dy() != targetHeight {
		state.previewImage = image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	}

	// Bilinear downscaling directly into the reusable canvas image
	draw.ApproxBiLinear.Scale(state.previewImage, state.previewImage.Bounds(), rgba, rgba.Bounds(), draw.Over, nil)

	return state.previewImage, nil
}
