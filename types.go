package main

import (
	"syscall"
	"time"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procEnumWindows          = user32.NewProc("EnumWindows")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetWindowRect        = user32.NewProc("GetWindowRect")
	procFindWindowW          = user32.NewProc("FindWindowW")
	procGetDC                = user32.NewProc("GetDC")
	procReleaseDC            = user32.NewProc("ReleaseDC")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procGetCursorInfo        = user32.NewProc("GetCursorInfo")
	procDrawIconEx           = user32.NewProc("DrawIconEx")
	procRtlGetVersion        = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")

	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procStretchBlt             = gdi32.NewProc("StretchBlt")
	procSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
)

type RECT struct {
	Left, Top, Right, Bottom int32
}

type SourceItem struct {
	HWND syscall.Handle
	Name string
	Type string // "screen" or "window"
}

var (
	activeSources []SourceItem
	currentTab    int = 0
	selectedIndex int = -1
	selectedHWND  syscall.Handle
	scrollOffset  int = 0

	isRecording    bool = false
	recordingStart time.Time
	recordingTimer *time.Ticker
	timerStopChan  chan bool
	timerString    string = "00:00"

	ffmpegAvailable bool = false
)

type CURSORINFO struct {
	CbSize      uint32
	Flags       uint32
	HCursor     syscall.Handle
	PtScreenPos struct{ X, Y int32 }
}

type RTL_OSVERSIONINFOW struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformID        uint32
	CSDVersion        [128]uint16
}
