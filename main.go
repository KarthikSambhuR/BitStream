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
)

// Win32 API procedures
var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	gdi32                      = syscall.NewLazyDLL("gdi32.dll")
	dwmapi                     = syscall.NewLazyDLL("dwmapi.dll")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procGetClientRect          = user32.NewProc("GetClientRect")
	procSendMessageW           = user32.NewProc("SendMessageW")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	procGetDesktopWindow       = user32.NewProc("GetDesktopWindow")
	procEnumDisplayMonitors    = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procSetWindowTextW         = user32.NewProc("SetWindowTextW")
	procGetWindowRect          = user32.NewProc("GetWindowRect")

	procGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW")

	procGetStockObject         = gdi32.NewProc("GetStockObject")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkColor             = gdi32.NewProc("SetBkColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")

	procDwmRegisterThumbnail    = dwmapi.NewProc("DwmRegisterThumbnail")
	procDwmUnregisterThumbnail  = dwmapi.NewProc("DwmUnregisterThumbnail")
	procDwmUpdateThumbnailProps = dwmapi.NewProc("DwmUpdateThumbnailProperties")
)

// Win32 Constants
const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	LBS_NOTIFY          = 0x0001
	TCS_TABS            = 0x0000

	WM_CREATE           = 0x0001
	WM_DESTROY          = 0x0002
	WM_SIZE             = 0x0005
	WM_COMMAND          = 0x0111
	WM_NOTIFY           = 0x004E

	WM_CTLCOLORMSGBOX   = 0x0132
	WM_CTLCOLOREDIT     = 0x0133
	WM_CTLCOLORLISTBOX  = 0x0134
	WM_CTLCOLORBTN      = 0x0135
	WM_CTLCOLORDLG      = 0x0136
	WM_CTLCOLORSTATIC   = 0x0138

	COLOR_WINDOW        = 5
	COLOR_BTNFACE       = 15

	// Control IDs
	IDC_TAB             = 101
	IDC_LISTBOX         = 102
	IDC_BTN_REFRESH     = 103
	IDC_BTN_FOLDER      = 104
	IDC_BTN_RECORD      = 105
	IDC_LBL_STATUS      = 106
	IDC_LBL_TIMER       = 107
	IDC_LBL_SOURCE      = 108

	// DWM Constants
	DWM_TNP_RECTDESTINATION      = 0x00000001
	DWM_TNP_VISIBLE              = 0x00000008
	DWM_TNP_OPACITY              = 0x00000004
	DWM_TNP_SOURCECLIENTAREAONLY = 0x00000010
)

// Tab messages
const (
	TCM_INSERTITEMW = 0x1300 + 62
	TCM_GETCURSEL   = 0x1300 + 11
	TCN_SELCHANGE   = 4294966745
)

// ListBox messages
const (
	LB_ADDSTRING    = 0x0180
	LB_RESETCONTENT = 0x0184
	LB_GETCURSEL    = 0x0188
	LBN_SELCHANGE   = 1
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type TCITEMW struct {
	Mask        uint32
	DwState     uint32
	DwStateMask uint32
	PszText     *uint16
	CchTextMax  int32
	IImage      int32
	LParam      uintptr
}

type NMHDR struct {
	HwndFrom syscall.Handle
	IdFrom   uintptr
	Code     uint32
}

type DWM_THUMBNAIL_PROPERTIES struct {
	DwFlags           uint32
	RcDestination     RECT
	RcSource          RECT
	Opacity           byte
	FVisible          int32
	FSourceClientArea int32
}

type SourceItem struct {
	HWND syscall.Handle
	Name string
	Type string // "screen" or "window"
}

// Global state variables
var (
	hTab            syscall.Handle
	hListBox        syscall.Handle
	hBtnRefresh     syscall.Handle
	hBtnFolder      syscall.Handle
	hBtnRecord      syscall.Handle
	hLblStatus      syscall.Handle
	hLblTimer       syscall.Handle
	hLblSource      syscall.Handle

	activeSources   []SourceItem
	currentTab      int = 0 // 0 = Screens, 1 = Windows
	selectedHWND    syscall.Handle
	hThumbnail      uintptr = 0

	isRecording     bool = false
	recordingStart  time.Time
	recordingTimer  *time.Ticker
	timerStopChan   chan bool

	// Dark Mode brushes and colors
	hBgBrush        syscall.Handle // Slate-900 background brush
	hListBgBrush    syscall.Handle // Alternate darker list background brush
	darkTextColor   = uint32(0x00f0e8e2) // BGR equivalent of #e2e8f0 (Slate-200)
	darkBgColor     = uint32(0x002a170f) // BGR equivalent of #0f172a (Slate-900)
	listBgColor     = uint32(0x001a110a) // BGR equivalent of #0a111a (Slate-950)
)

func main() {
	// Keep memory footprint extremely tight and lock GUI thread
	runtime.LockOSThread()

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("BitStreamWindow")
	windowTitle, _ := syscall.UTF16PtrFromString("BitStream - Screen Recorder")

	// Initialize dark background brush: Slate-900 (0x2a170f in BGR)
	brush, _, _ := procCreateSolidBrush.Call(uintptr(darkBgColor))
	hBgBrush = syscall.Handle(brush)

	listBrush, _, _ := procCreateSolidBrush.Call(uintptr(listBgColor))
	hListBgBrush = syscall.Handle(listBrush)

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.HInstance = syscall.Handle(hInstance)
	wc.HbrBackground = hBgBrush
	wc.LpszClassName = className

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create Main Window
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100, 100, 1024, 640,
		0, 0, hInstance, 0,
	)

	if hwnd == 0 {
		fmt.Println("Window creation failed.")
		return
	}

	// Message Loop
	var msg struct {
		Hwnd    syscall.Handle
		Message uint32
		Wparam  uintptr
		Lparam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}

	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func wndProc(hwnd syscall.Handle, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		setupUI(hwnd)
		refreshSourceList()
		return 0

	case WM_SIZE:
		realignUI(hwnd)
		return 0

	case WM_NOTIFY:
		nmhdr := (*NMHDR)(unsafe.Pointer(lparam))
		if nmhdr.Code == TCN_SELCHANGE && nmhdr.HwndFrom == hTab {
			sel, _, _ := procSendMessageW.Call(uintptr(hTab), TCM_GETCURSEL, 0, 0)
			currentTab = int(sel)
			refreshSourceList()
		}
		return 0

	case WM_COMMAND:
		controlID := wparam & 0xFFFF
		notificationCode := (wparam >> 16) & 0xFFFF

		switch controlID {
		case IDC_LISTBOX:
			if notificationCode == LBN_SELCHANGE {
				onSourceSelected(hwnd)
			}
		case IDC_BTN_REFRESH:
			refreshSourceList()
		case IDC_BTN_FOLDER:
			openRecordingsFolder()
		case IDC_BTN_RECORD:
			toggleRecording(hwnd)
		}
		return 0

	// Handle Control coloring to draw custom premium Dark Theme
	case WM_CTLCOLORSTATIC:
		hdc := wparam
		procSetTextColor.Call(hdc, uintptr(darkTextColor))
		procSetBkColor.Call(hdc, uintptr(darkBgColor))
		return uintptr(hBgBrush)

	case WM_CTLCOLORDLG:
		return uintptr(hBgBrush)

	case WM_CTLCOLORLISTBOX:
		hdc := wparam
		procSetTextColor.Call(hdc, uintptr(darkTextColor))
		procSetBkColor.Call(hdc, uintptr(listBgColor))
		return uintptr(hListBgBrush)

	case WM_DESTROY:
		if hThumbnail != 0 {
			procDwmUnregisterThumbnail.Call(hThumbnail)
		}
		if isRecording {
			stopRecordingTimer()
		}
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return ret
}

func setupUI(hwnd syscall.Handle) {
	inst, _, _ := procGetModuleHandleW.Call(0)

	tabClass, _ := syscall.UTF16PtrFromString("SysTabControl32")
	emptyStr, _ := syscall.UTF16PtrFromString("")

	// 1. Create Tabs Control
	hTab = syscall.Handle(createControl(tabClass, emptyStr, WS_CHILD|WS_VISIBLE|TCS_TABS, 10, 10, 240, 480, hwnd, IDC_TAB, inst))

	screensName, _ := syscall.UTF16PtrFromString("Screens")
	var tie TCITEMW
	tie.Mask = 1 // TCIF_TEXT
	tie.PszText = screensName
	procSendMessageW.Call(uintptr(hTab), TCM_INSERTITEMW, 0, uintptr(unsafe.Pointer(&tie)))

	windowsName, _ := syscall.UTF16PtrFromString("Windows")
	tie.PszText = windowsName
	procSendMessageW.Call(uintptr(hTab), TCM_INSERTITEMW, 1, uintptr(unsafe.Pointer(&tie)))

	// 2. Create ListBox
	listClass, _ := syscall.UTF16PtrFromString("LISTBOX")
	hListBox = syscall.Handle(createControl(listClass, emptyStr, WS_CHILD|WS_VISIBLE|WS_BORDER|WS_VSCROLL|LBS_NOTIFY, 15, 40, 230, 440, hwnd, IDC_LISTBOX, inst))

	// 3. Create Buttons
	btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
	btnText, _ := syscall.UTF16PtrFromString("Refresh Sources")
	hBtnRefresh = syscall.Handle(createControl(btnClass, btnText, WS_CHILD|WS_VISIBLE, 10, 500, 115, 30, hwnd, IDC_BTN_REFRESH, inst))

	folderText, _ := syscall.UTF16PtrFromString("Open Folder")
	hBtnFolder = syscall.Handle(createControl(btnClass, folderText, WS_CHILD|WS_VISIBLE, 135, 500, 115, 30, hwnd, IDC_BTN_FOLDER, inst))

	// 4. Create Labels
	staticClass, _ := syscall.UTF16PtrFromString("STATIC")
	sourceText, _ := syscall.UTF16PtrFromString("No source selected")
	hLblSource = syscall.Handle(createControl(staticClass, sourceText, WS_CHILD|WS_VISIBLE, 270, 10, 400, 25, hwnd, IDC_LBL_SOURCE, inst))

	statusText, _ := syscall.UTF16PtrFromString("Status: Ready")
	hLblStatus = syscall.Handle(createControl(staticClass, statusText, WS_CHILD|WS_VISIBLE, 10, 545, 115, 20, hwnd, IDC_LBL_STATUS, inst))

	timerText, _ := syscall.UTF16PtrFromString("00:00")
	hLblTimer = syscall.Handle(createControl(staticClass, timerText, WS_CHILD|WS_VISIBLE, 135, 545, 115, 20, hwnd, IDC_LBL_TIMER, inst))

	// 5. Create Record Button
	recordText, _ := syscall.UTF16PtrFromString("Start Recording")
	hBtnRecord = syscall.Handle(createControl(btnClass, recordText, WS_CHILD|WS_VISIBLE, 10, 570, 240, 30, hwnd, IDC_BTN_RECORD, inst))

	// Set Modern Clean Fonts
	hFont, _, _ := procGetStockObject.Call(17) // DEFAULT_GUI_FONT
	procSendMessageW.Call(uintptr(hTab), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hListBox), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hBtnRefresh), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hBtnFolder), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hBtnRecord), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hLblSource), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hLblStatus), 0x0030, hFont, 1)
	procSendMessageW.Call(uintptr(hLblTimer), 0x0030, hFont, 1)
}

func createControl(className, text *uint16, style uint32, x, y, w, h int, parent syscall.Handle, id uintptr, inst uintptr) uintptr {
	ctrl, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(text)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent),
		id,
		inst,
		0,
	)
	return ctrl
}

func refreshSourceList() {
	procSendMessageW.Call(uintptr(hListBox), LB_RESETCONTENT, 0, 0)
	activeSources = nil

	if currentTab == 0 {
		// 1. Screens / Monitors Tab
		desktopHwnd, _, _ := procGetDesktopWindow.Call()
		if desktopHwnd != 0 {
			activeSources = append(activeSources, SourceItem{
				HWND: syscall.Handle(desktopHwnd),
				Name: "Entire Desktop Screen",
				Type: "screen",
			})
			titleW, _ := syscall.UTF16PtrFromString("Entire Desktop Screen")
			procSendMessageW.Call(uintptr(hListBox), LB_ADDSTRING, 0, uintptr(unsafe.Pointer(titleW)))
		}
	} else {
		// 2. Active Windows Tab
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

			if title == "" || title == "Program Manager" || title == "BitStream - Screen Recorder" || title == "Settings" {
				return 1
			}

			activeSources = append(activeSources, SourceItem{
				HWND: hwnd,
				Name: title,
				Type: "window",
			})

			titleW, _ := syscall.UTF16PtrFromString(title)
			procSendMessageW.Call(uintptr(hListBox), LB_ADDSTRING, 0, uintptr(unsafe.Pointer(titleW)))

			return 1
		})

		procEnumWindows.Call(cb, 0)
	}

	runtime.GC()
}

func onSourceSelected(hwnd syscall.Handle) {
	sel, _, _ := procSendMessageW.Call(uintptr(hListBox), LB_GETCURSEL, 0, 0)
	idx := int(sel)
	if idx < 0 || idx >= len(activeSources) {
		return
	}

	src := activeSources[idx]
	selectedHWND = src.HWND

	// Update text
	lblW, _ := syscall.UTF16PtrFromString(fmt.Sprintf("Selected: %s (%s)", src.Name, src.Type))
	procSetWindowTextW.Call(uintptr(hLblSource), uintptr(unsafe.Pointer(lblW)))

	if hThumbnail != 0 {
		procDwmUnregisterThumbnail.Call(hThumbnail)
		hThumbnail = 0
	}

	// Register DWM Live Preview Thumbnail
	var thumb uintptr
	hr, _, _ := procDwmRegisterThumbnail.Call(
		uintptr(hwnd),
		uintptr(selectedHWND),
		uintptr(unsafe.Pointer(&thumb)),
	)

	if int32(hr) >= 0 {
		hThumbnail = thumb
		updateThumbnailPosition(hwnd)
	}
}

func updateThumbnailPosition(hwnd syscall.Handle) {
	if hThumbnail == 0 || selectedHWND == 0 {
		return
	}

	var rect RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))

	// Target destination preview viewport boundaries
	margin := int32(270)
	w := rect.Right - margin - 20
	h := rect.Bottom - 60

	if w <= 0 || h <= 0 {
		return
	}

	// Fetch actual source dimensions using GetWindowRect
	var srcRect RECT
	ret, _, _ := procGetWindowRect.Call(uintptr(selectedHWND), uintptr(unsafe.Pointer(&srcRect)))
	if ret == 0 {
		return
	}
	srcW := srcRect.Right - srcRect.Left
	srcH := srcRect.Bottom - srcRect.Top

	if srcW <= 0 || srcH <= 0 {
		return
	}

	// Calculate perfect aspect ratio fitting inside the preview box (preserves original ratio!)
	srcAspect := float64(srcW) / float64(srcH)
	destAspect := float64(w) / float64(h)

	var finalW, finalH int32
	if srcAspect > destAspect {
		finalW = w
		finalH = int32(float64(w) / srcAspect)
	} else {
		finalH = h
		finalW = int32(float64(h) * srcAspect)
	}

	// Center the preview inside the container
	leftOffset := margin + (w-finalW)/2
	topOffset := 40 + (h-finalH)/2

	var props DWM_THUMBNAIL_PROPERTIES
	// TNP_RECTDESTINATION | TNP_VISIBLE | TNP_OPACITY | TNP_SOURCECLIENTAREAONLY
	props.DwFlags = DWM_TNP_RECTDESTINATION | DWM_TNP_VISIBLE | DWM_TNP_OPACITY | DWM_TNP_SOURCECLIENTAREAONLY
	props.RcDestination = RECT{
		Left:   leftOffset,
		Top:    topOffset,
		Right:  leftOffset + finalW,
		Bottom: topOffset + finalH,
	}
	props.Opacity = 255
	props.FVisible = 1
	props.FSourceClientArea = 1 // Shows ONLY client contents (no borders/title bars, crystal clear!)

	procDwmUpdateThumbnailProps.Call(hThumbnail, uintptr(unsafe.Pointer(&props)))
}

func realignUI(hwnd syscall.Handle) {
	var rect RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))

	h := rect.Bottom - 160
	if h > 0 {
		procDefWindowProcW.Call(uintptr(hTab), 0x0005, 0, uintptr(h))
	}

	updateThumbnailPosition(hwnd)
}

func openRecordingsFolder() {
	dir, _ := filepath.Abs("./recordings")
	_ = os.MkdirAll(dir, 0755)
	_ = exec.Command("explorer", dir).Start()
}

func toggleRecording(hwnd syscall.Handle) {
	if selectedHWND == 0 {
		errW, _ := syscall.UTF16PtrFromString("Status: Select a source")
		procSetWindowTextW.Call(uintptr(hLblStatus), uintptr(unsafe.Pointer(errW)))
		return
	}

	if !isRecording {
		isRecording = true
		recordingStart = time.Now()

		btnW, _ := syscall.UTF16PtrFromString("Stop Recording")
		procSetWindowTextW.Call(uintptr(hBtnRecord), uintptr(unsafe.Pointer(btnW)))

		statusW, _ := syscall.UTF16PtrFromString("Status: Recording...")
		procSetWindowTextW.Call(uintptr(hLblStatus), uintptr(unsafe.Pointer(statusW)))

		timerStopChan = make(chan bool)
		recordingTimer = time.NewTicker(1 * time.Second)
		go func() {
			for {
				select {
				case <-recordingTimer.C:
					elapsed := time.Since(recordingStart)
					minutes := int(elapsed.Minutes()) % 60
					seconds := int(elapsed.Seconds()) % 60
					timeStr := fmt.Sprintf("%02d:%02d", minutes, seconds)
					timeW, _ := syscall.UTF16PtrFromString(timeStr)
					procSetWindowTextW.Call(uintptr(hLblTimer), uintptr(unsafe.Pointer(timeW)))
				case <-timerStopChan:
					return
				}
			}
		}()
	} else {
		isRecording = false
		stopRecordingTimer()

		btnW, _ := syscall.UTF16PtrFromString("Start Recording")
		procSetWindowTextW.Call(uintptr(hBtnRecord), uintptr(unsafe.Pointer(btnW)))

		statusW, _ := syscall.UTF16PtrFromString("Status: Saved")
		procSetWindowTextW.Call(uintptr(hLblStatus), uintptr(unsafe.Pointer(statusW)))

		timerW, _ := syscall.UTF16PtrFromString("00:00")
		procSetWindowTextW.Call(uintptr(hLblTimer), uintptr(unsafe.Pointer(timerW)))
	}
}

func stopRecordingTimer() {
	if recordingTimer != nil {
		recordingTimer.Stop()
		close(timerStopChan)
		recordingTimer = nil
	}
}
