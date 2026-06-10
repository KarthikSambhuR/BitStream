package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

func captureStaticThumbnail(targetHWND syscall.Handle, destW, destH int) uintptr {
	progmanW, _ := syscall.UTF16PtrFromString("Progman")
	progmanHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(progmanW)), 0)

	var srcDC uintptr
	var srcW, srcH int32

	if targetHWND == 0 || targetHWND == syscall.Handle(progmanHwnd) {
		// Capture desktop screen
		srcDC, _, _ = procGetDC.Call(0)
		if srcDC == 0 {
			return 0
		}
		defer procReleaseDC.Call(0, srcDC)

		scrWVal, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN = 0
		scrHVal, _, _ := procGetSystemMetrics.Call(1) // SM_CYSCREEN = 1
		srcW = int32(scrWVal)
		srcH = int32(scrHVal)
	} else {
		srcDC, _, _ = procGetWindowDC.Call(uintptr(targetHWND))
		if srcDC == 0 {
			srcDC, _, _ = procGetDC.Call(uintptr(targetHWND))
		}
		if srcDC == 0 {
			return 0
		}
		defer procReleaseDC.Call(uintptr(targetHWND), srcDC)

		var rect RECT
		ret, _, _ := procGetWindowRect.Call(uintptr(targetHWND), uintptr(unsafe.Pointer(&rect)))
		if ret == 0 {
			return 0
		}
		srcW = rect.Right - rect.Left
		srcH = rect.Bottom - rect.Top
	}

	if srcW <= 0 || srcH <= 0 {
		return 0
	}

	// Create compatible DC and bitmap for thumbnail
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return 0
	}
	defer procReleaseDC.Call(0, screenDC)

	thumbDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if thumbDC == 0 {
		return 0
	}
	defer procDeleteDC.Call(thumbDC)

	thumbBitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(destW), uintptr(destH))
	if thumbBitmap == 0 {
		return 0
	}

	oldObj, _, _ := procSelectObject.Call(thumbDC, thumbBitmap)
	procSetStretchBltMode.Call(thumbDC, 4) // HALFTONE

	ret, _, _ := procStretchBlt.Call(
		thumbDC, 0, 0, uintptr(destW), uintptr(destH),
		srcDC, 0, 0, uintptr(srcW), uintptr(srcH),
		0x00CC0020, // SRCCOPY
	)

	procSelectObject.Call(thumbDC, oldObj)

	if ret == 0 {
		procDeleteObject.Call(thumbBitmap)
		return 0
	}

	return thumbBitmap
}

func refreshSourceList() {
	// First, clean up old GDI thumbnail bitmaps to prevent leaks
	for _, src := range activeSources {
		if src.HBitmap != 0 {
			procDeleteObject.Call(src.HBitmap)
		}
	}
	activeSources = nil
	scrollOffset = 0

	progmanW, _ := syscall.UTF16PtrFromString("Progman")
	progmanHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(progmanW)), 0)

	if currentTab == 0 {
		if progmanHwnd != 0 {
			hBmp := captureStaticThumbnail(syscall.Handle(progmanHwnd), 128, 72)
			activeSources = append(activeSources, SourceItem{
				HWND:    syscall.Handle(progmanHwnd),
				Name:    "Entire Desktop Screen",
				Type:    "screen",
				HBitmap: hBmp,
			})
		}
	} else {
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

			// Capture a small static thumbnail at the point of refresh
			hBmp := captureStaticThumbnail(hwnd, 128, 72)

			activeSources = append(activeSources, SourceItem{
				HWND:    hwnd,
				Name:    title,
				Type:    "window",
				HBitmap: hBmp,
			})

			return 1
		})

		procEnumWindows.Call(cb, 0)
	}

	runtime.GC()
}

func onSourceSelected(hwnd syscall.Handle) {
	if selectedIndex < 0 || selectedIndex >= len(activeSources) {
		return
	}

	src := activeSources[selectedIndex]
	selectedHWND = src.HWND
}

func resetSelection(hwnd syscall.Handle) {
	selectedIndex = -1
	selectedHWND = 0
}

func onMouseMove(hwnd syscall.Handle, lparam uintptr) {
	x := int(lparam & 0xFFFF)
	y := int((lparam >> 16) & 0xFFFF)

	var rect RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	height := int(rect.Bottom - rect.Top)

	prevHover := hoveredElement
	hoveredElement = ""

	// Check tabs coordinates
	if y >= 60 && y <= 92 {
		if x >= 15 && x <= 120 {
			hoveredElement = "screens_tab"
		} else if x >= 125 && x <= 225 {
			hoveredElement = "windows_tab"
		}
	}

	// Check items list coordinates (scroll-aware)
	visibleCount := 0
	for i := scrollOffset; i < len(activeSources); i++ {
		if visibleCount >= 7 {
			break
		}
		yPos := 105 + (visibleCount * 56)
		if x >= 15 && x <= 225 && y >= yPos && y <= yPos+48 {
			hoveredElement = fmt.Sprintf("item_%d", i)
		}
		visibleCount++
	}

	// Check buttons coordinates
	if y >= height-95 && y <= height-65 {
		if x >= 15 && x <= 115 {
			hoveredElement = "refresh_btn"
		} else if x >= 125 && x <= 225 {
			hoveredElement = "folder_btn"
		}
	}

	if x >= 15 && x <= 225 && y >= height-35 && y <= height-5 {
		hoveredElement = "record_btn"
	}

	if prevHover != hoveredElement {
		procInvalidateRect.Call(uintptr(hwnd), 0, 0)
	}
}

func onLButtonDown(hwnd syscall.Handle, lparam uintptr) {
	x := int(lparam & 0xFFFF)
	y := int((lparam >> 16) & 0xFFFF)

	var rect RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	height := int(rect.Bottom - rect.Top)

	// Clicks inside tabs
	if y >= 60 && y <= 92 {
		if x >= 15 && x <= 120 {
			currentTab = 0
			resetSelection(hwnd)
			refreshSourceList()
			procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		} else if x >= 125 && x <= 225 {
			currentTab = 1
			resetSelection(hwnd)
			refreshSourceList()
			procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		}
	}

	// Clicks inside source cards (scroll-aware)
	visibleCount := 0
	for i := scrollOffset; i < len(activeSources); i++ {
		if visibleCount >= 7 {
			break
		}
		yPos := 105 + (visibleCount * 56)
		if x >= 15 && x <= 225 && y >= yPos && y <= yPos+48 {
			selectedIndex = i
			onSourceSelected(hwnd)
			procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		}
		visibleCount++
	}

	// Clicks inside control buttons
	if y >= height-95 && y <= height-65 {
		if x >= 15 && x <= 115 {
			refreshSourceList()
			procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		} else if x >= 125 && x <= 225 {
			openRecordingsFolder()
		}
	}

	if x >= 15 && x <= 225 && y >= height-35 && y <= height-5 {
		toggleRecording(hwnd)
		procInvalidateRect.Call(uintptr(hwnd), 0, 1)
	}
}
