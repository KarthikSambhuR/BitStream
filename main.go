package main

import (
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

func main() {
	runtime.LockOSThread()

	// Check if FFmpeg is installed on System PATH
	_, lookupErr := exec.LookPath("ffmpeg")
	ffmpegAvailable = lookupErr == nil

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("BitStreamWindow")
	windowTitle, _ := syscall.UTF16PtrFromString("BitStream - Screen Recorder")

	// Pre-build solid colors for GDI Double Buffer rendering
	brushBg, _, _ = procCreateSolidBrush.Call(uintptr(bgCol))
	brushSidebar, _, _ = procCreateSolidBrush.Call(uintptr(sidebarBgCol))
	brushCard, _, _ = procCreateSolidBrush.Call(uintptr(cardBgCol))
	brushCardHover, _, _ = procCreateSolidBrush.Call(uintptr(cardHoverCol))
	brushAccent, _, _ = procCreateSolidBrush.Call(uintptr(accentCol))

	// Pens for borders
	penTransparent, _, _ = procCreatePen.Call(5, 1, 0) // PS_NULL (transparent border)
	penAccent, _, _ = procCreatePen.Call(0, 1, uintptr(accentCol)) // Solid accent pen

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.HInstance = syscall.Handle(hInstance)
	wc.HbrBackground = syscall.Handle(brushBg)
	wc.LpszClassName = className

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create Window
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100, 100, 1024, 640,
		0, 0, hInstance, 0,
	)

	if hwnd == 0 {
		return
	}

	// Apply dark titlebar frame
	darkMode := int32(1)
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 20, uintptr(unsafe.Pointer(&darkMode)), 4) // DWMWA_USE_IMMERSIVE_DARK_MODE

	// Start Font Downloader routine
	setupFonts(syscall.Handle(hwnd))

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
		procSetTimer.Call(uintptr(hwnd), 1, 50, 0) // 50ms = 20 FPS live preview timer
		return 0

	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		if isDownloadingFont {
			drawDownloadScreen(hwnd, uintptr(hdc))
		} else {
			drawUI(hwnd, uintptr(hdc))
		}
		procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0

	case WM_SIZE:
		procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		return 0

	case WM_MOUSEMOVE:
		if !isDownloadingFont {
			onMouseMove(hwnd, lparam)
		}
		return 0

	case WM_LBUTTONDOWN:
		if !isDownloadingFont {
			onLButtonDown(hwnd, lparam)
		}
		return 0

	case 0x020A: // WM_MOUSEWHEEL
		if !isDownloadingFont {
			delta := int16(wparam >> 16)
			// Scroll when mouse is over left sidebar
			if scrollOffset >= 0 {
				if delta > 0 {
					if scrollOffset > 0 {
						scrollOffset--
						procInvalidateRect.Call(uintptr(hwnd), 0, 0)
					}
				} else {
					if scrollOffset < len(activeSources)-7 {
						scrollOffset++
						procInvalidateRect.Call(uintptr(hwnd), 0, 0)
					}
				}
			}
		}
		return 0

	case 0x0113: // WM_TIMER
		if wparam == 1 && selectedHWND != 0 {
			var rect RECT
			procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
			// Invalidate only the preview container rect to keep CPU usage low
			previewRect := RECT{Left: 260, Top: 40, Right: rect.Right - 20, Bottom: rect.Bottom - 20}
			procInvalidateRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&previewRect)), 0)
		}
		return 0

	case WM_DESTROY:
		procKillTimer.Call(uintptr(hwnd), 1)
		if isRecording {
			stopFFmpegRecording()
		}
		// Unload Font Resource dynamically from process
		if fontLoaded {
			fontPathW, _ := syscall.UTF16PtrFromString(fontPath)
			procRemoveFontResourceExW.Call(uintptr(unsafe.Pointer(fontPathW)), 0x10, 0) // FR_PRIVATE = 0x10
		}

		// Delete Fonts
		if hFontRegular != 0 {
			procDeleteObject.Call(hFontRegular)
			procDeleteObject.Call(hFontBold)
			procDeleteObject.Call(hFontLogo)
		}

		// Clean up GDI bitmaps in active sources
		for _, src := range activeSources {
			if src.HBitmap != 0 {
				procDeleteObject.Call(src.HBitmap)
			}
		}

		// Clean up GDI Brushes & Pens
		procDeleteObject.Call(brushBg)
		procDeleteObject.Call(brushSidebar)
		procDeleteObject.Call(brushCard)
		procDeleteObject.Call(brushCardHover)
		procDeleteObject.Call(brushAccent)
		procDeleteObject.Call(penTransparent)
		procDeleteObject.Call(penAccent)

		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return ret
}
