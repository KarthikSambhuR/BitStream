package main

import (
	"syscall"
	"time"
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
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procFindWindowW            = user32.NewProc("FindWindowW")
	procInvalidateRect         = user32.NewProc("InvalidateRect")
	procBeginPaint             = user32.NewProc("BeginPaint")
	procEndPaint               = user32.NewProc("EndPaint")
	procDrawTextW              = user32.NewProc("DrawTextW")
	procFillRect               = user32.NewProc("FillRect")

	procGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW")

	procGetStockObject         = gdi32.NewProc("GetStockObject")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkColor             = gdi32.NewProc("SetBkColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procRoundRect              = gdi32.NewProc("RoundRect")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procAddFontResourceExW     = gdi32.NewProc("AddFontResourceExW")
	procRemoveFontResourceExW  = gdi32.NewProc("RemoveFontResourceExW")

	procGetDC                  = user32.NewProc("GetDC")
	procGetWindowDC            = user32.NewProc("GetWindowDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procStretchBlt             = gdi32.NewProc("StretchBlt")
	procSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")

	procGetCursorInfo          = user32.NewProc("GetCursorInfo")
	procDrawIconEx             = user32.NewProc("DrawIconEx")
	procSetTimer               = user32.NewProc("SetTimer")
	procKillTimer              = user32.NewProc("KillTimer")

	procDwmSetWindowAttribute   = dwmapi.NewProc("DwmSetWindowAttribute")
)

// Win32 Constants
const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000

	WM_CREATE           = 0x0001
	WM_DESTROY          = 0x0002
	WM_SIZE             = 0x0005
	WM_PAINT            = 0x000F
	WM_MOUSEMOVE        = 0x0200
	WM_LBUTTONDOWN      = 0x0201
	WM_LBUTTONUP        = 0x0202

	COLOR_BTNFACE       = 15
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

type PAINTSTRUCT struct {
	Hdc         syscall.Handle
	FErase      int32
	RcPaint     RECT
	Restore     int32
	IncUpdate   int32
	RowVal      [32]byte
}

type SourceItem struct {
	HWND     syscall.Handle
	Name     string
	Type     string // "screen" or "window"
	HBitmap  uintptr
}

// Global state variables
var (
	activeSources   []SourceItem
	currentTab      int = 0 // 0 = Screens, 1 = Windows
	selectedIndex   int = -1
	selectedHWND    syscall.Handle
	scrollOffset    int = 0

	isRecording     bool = false
	recordingStart  time.Time
	recordingTimer  *time.Ticker
	timerStopChan   chan bool
	timerString     string = "00:00"

	// Native FFmpeg capture subprocess
	ffmpegAvailable bool = false

	// Font Loading State
	isDownloadingFont bool    = false
	downloadProgress  float64 = 0.0
	fontPath          string
	fontLoaded        bool    = false

	// Custom Fonts
	hFontRegular      uintptr
	hFontBold         uintptr
	hFontLogo         uintptr

	// Premium Custom Color Palette (Material You - Slate & Indigo theme)
	bgCol           = uint32(0x00170f0b) // Slate-900 (#0f172a)
	sidebarBgCol    = uint32(0x000b0806) // Slate-950 (#06080b)
	cardBgCol       = uint32(0x001d130f) // Slate-800 (#0f131d)
	cardHoverCol    = uint32(0x00271c17) // Slate-700 (#171c27)
	accentCol       = uint32(0x00e5464f) // Indigo-600 (#4f46e5)
	textCol         = uint32(0x00f0e8e2) // Slate-200 (#e2e8f0)
	textMutedCol    = uint32(0x009ca3af) // Gray-400 (#9ca3af)
	whiteCol        = uint32(0x00ffffff) // White

	// GDI Brushes & Pens stored as uintptr for Win32 API direct usage
	brushBg         uintptr
	brushSidebar    uintptr
	brushCard       uintptr
	brushCardHover  uintptr
	brushAccent     uintptr
	penTransparent  uintptr
	penAccent       uintptr

	// Hover tracking states
	hoveredElement  string = "" // "screens_tab", "windows_tab", "refresh_btn", "folder_btn", "record_btn", "item_0", etc.
)
