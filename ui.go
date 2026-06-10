package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Custom Paint loop for Main Dashboard
func drawUI(hwnd syscall.Handle, hdc uintptr) {
	var rect RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	width := rect.Right - rect.Left
	height := rect.Bottom - rect.Top

	// Create memory DC
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	memBitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
	oldObj, _, _ := procSelectObject.Call(memDC, memBitmap)

	// Apply default regular font
	procSelectObject.Call(memDC, hFontRegular)

	// 1. Draw Slate-900 Main Background
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&rect)), brushBg)

	// 2. Draw Slate-950 Sidebar Background
	sidebarRect := RECT{Left: 0, Top: 0, Right: 240, Bottom: height}
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&sidebarRect)), brushSidebar)

	// Draw decorative separator line
	lineRect := RECT{Left: 240, Top: 0, Right: 241, Bottom: height}
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&lineRect)), brushCard)

	// 3. Draw Brand Label with Bold logo font
	procSelectObject.Call(memDC, hFontLogo)
	procSetBkMode.Call(memDC, 1) // TRANSPARENT
	procSetTextColor.Call(memDC, uintptr(whiteCol))
	brandText, _ := syscall.UTF16PtrFromString("BitStream")
	brandRect := RECT{Left: 20, Top: 20, Right: 220, Bottom: 50}
	procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(brandText)), ^uintptr(0), uintptr(unsafe.Pointer(&brandRect)), 0)

	// Reset to default regular font
	procSelectObject.Call(memDC, hFontRegular)

	// 4. Draw Custom Toggle Tabs (Material You design)
	drawTabButton(memDC, "Screens", 15, 60, 105, 32, currentTab == 0, hoveredElement == "screens_tab")
	drawTabButton(memDC, "Windows", 125, 60, 100, 32, currentTab == 1, hoveredElement == "windows_tab")

	// 5. Draw Source Lists (Scrollable, max 7 items)
	visibleCount := 0
	for i := scrollOffset; i < len(activeSources); i++ {
		if visibleCount >= 7 {
			break
		}
		src := activeSources[i]
		yPos := 105 + (visibleCount * 56)
		isHovered := hoveredElement == fmt.Sprintf("item_%d", i)
		isSelected := selectedIndex == i
		drawSourceCard(memDC, src.Name, 15, yPos, 210, 48, isSelected, isHovered, src.HBitmap)
		visibleCount++
	}

	// 6. Draw Bottom Sidebar Controls
	drawFlatButton(memDC, "Refresh", 15, int(height)-95, 100, 30, hoveredElement == "refresh_btn", false)
	drawFlatButton(memDC, "Folder", 125, int(height)-95, 100, 30, hoveredElement == "folder_btn", false)

	// Timer indicator
	procSetTextColor.Call(memDC, uintptr(textMutedCol))
	statusText, _ := syscall.UTF16PtrFromString("Status: Ready")
	if isRecording {
		statusText, _ = syscall.UTF16PtrFromString("REC")
		procSetTextColor.Call(memDC, uintptr(accentCol))
	} else if !ffmpegAvailable {
		statusText, _ = syscall.UTF16PtrFromString("FFmpeg required")
		procSetTextColor.Call(memDC, uintptr(accentCol))
	}
	statusRect := RECT{Left: 20, Top: height - 55, Right: 120, Bottom: height - 40}
	procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(statusText)), ^uintptr(0), uintptr(unsafe.Pointer(&statusRect)), 0)

	procSetTextColor.Call(memDC, uintptr(textCol))
	timerText, _ := syscall.UTF16PtrFromString(timerString)
	timerRect := RECT{Left: 125, Top: height - 55, Right: 225, Bottom: height - 40}
	procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(timerText)), ^uintptr(0), uintptr(unsafe.Pointer(&timerRect)), 0)

	// Record button (pulsing red or dark slate accent)
	drawFlatButton(memDC, "Start Recording", 15, int(height)-35, 210, 30, hoveredElement == "record_btn", isRecording)

	// 7. Preview title overlay using Bold regular font
	procSelectObject.Call(memDC, hFontBold)
	selectedText := "Select a source to preview"
	if selectedIndex >= 0 && selectedIndex < len(activeSources) {
		selectedText = fmt.Sprintf("Previewing: %s", activeSources[selectedIndex].Name)
	}
	procSetTextColor.Call(memDC, uintptr(textMutedCol))
	selTextW, _ := syscall.UTF16PtrFromString(selectedText)
	selRect := RECT{Left: 260, Top: 15, Right: width - 20, Bottom: 35}
	procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(selTextW)), ^uintptr(0), uintptr(unsafe.Pointer(&selRect)), 0x00008000|0x00000004|0x00000020) // DT_END_ELLIPSIS | DT_SINGLELINE | DT_VCENTER

	// 8. Fixed Preview Card Container
	previewCardRect := RECT{Left: 260, Top: 40, Right: width - 20, Bottom: height - 20}
	procSelectObject.Call(memDC, brushSidebar)
	procSelectObject.Call(memDC, penTransparent)
	procRoundRect.Call(memDC, 260, 40, uintptr(width-20), uintptr(height-20), 12, 12)

	if selectedIndex < 0 {
		procSetTextColor.Call(memDC, uintptr(textMutedCol))
		placeholderText, _ := syscall.UTF16PtrFromString("Select a source to start preview")
		procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(placeholderText)), ^uintptr(0), uintptr(unsafe.Pointer(&previewCardRect)), 0x00000025|0x00000004) // DT_CENTER | DT_VCENTER | DT_SINGLELINE
	} else {
		// Calculate aspect ratio corrected coordinates inside container
		containerW := (width - 20) - 260 - 20
		containerH := (height - 20) - 40 - 20

		if containerW > 0 && containerH > 0 {
			var srcW, srcH int32
			progmanW, _ := syscall.UTF16PtrFromString("Progman")
			progmanHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(progmanW)), 0)

			if selectedHWND == syscall.Handle(progmanHwnd) {
				scrWVal, _, _ := procGetSystemMetrics.Call(0)
				scrHVal, _, _ := procGetSystemMetrics.Call(1)
				srcW = int32(scrWVal)
				srcH = int32(scrHVal)
			} else {
				var srcRect RECT
				ret, _, _ := procGetWindowRect.Call(uintptr(selectedHWND), uintptr(unsafe.Pointer(&srcRect)))
				if ret != 0 {
					srcW = srcRect.Right - srcRect.Left
					srcH = srcRect.Bottom - srcRect.Top
				}
			}

			if srcW > 0 && srcH > 0 {
				srcAspect := float64(srcW) / float64(srcH)
				destAspect := float64(containerW) / float64(containerH)

				var finalW, finalH int32
				if srcAspect > destAspect {
					finalW = containerW
					finalH = int32(float64(containerW) / srcAspect)
				} else {
					finalH = containerH
					finalW = int32(float64(containerH) * srcAspect)
				}

				leftOffset := int32(260 + 10) + (containerW-finalW)/2
				topOffset := int32(40 + 10) + (containerH-finalH)/2

				// Draw GDI Live Preview
				drawLivePreview(memDC, selectedHWND, leftOffset, topOffset, finalW, finalH)
			}
		}
	}

	// Copy buffer to screen DC
	procBitBlt.Call(hdc, 0, 0, uintptr(width), uintptr(height), memDC, 0, 0, 0x00CC0020)

	// Free memory objects
	procSelectObject.Call(memDC, oldObj)
	procDeleteObject.Call(memBitmap)
	procDeleteDC.Call(memDC)
}

func drawTabButton(hdc uintptr, label string, x, y, w, h int, isActive bool, isHovered bool) {
	rect := RECT{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}

	brush := brushSidebar
	tColor := textMutedCol
	if isActive {
		brush = brushAccent
		tColor = whiteCol
	} else if isHovered {
		brush = brushCard
		tColor = textCol
	}

	procSelectObject.Call(hdc, brush)
	procSelectObject.Call(hdc, penTransparent)
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 12, 12)

	// Draw label
	procSetTextColor.Call(hdc, uintptr(tColor))
	labelW, _ := syscall.UTF16PtrFromString(label)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(labelW)), ^uintptr(0), uintptr(unsafe.Pointer(&rect)), 0x00000025)
}

func drawSourceCard(hdc uintptr, label string, x, y, w, h int, isSelected bool, isHovered bool, hBitmap uintptr) {
	rect := RECT{Left: int32(x + 76), Top: int32(y), Right: int32(x + w - 6), Bottom: int32(y + h)}

	brush := brushSidebar
	tColor := textCol
	if isSelected {
		brush = brushCardHover
		tColor = whiteCol
	} else if isHovered {
		brush = brushCard
		tColor = whiteCol
	}

	procSelectObject.Call(hdc, brush)
	if isSelected {
		procSelectObject.Call(hdc, penAccent)
	} else {
		procSelectObject.Call(hdc, penTransparent)
	}
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 10, 10)

	// Draw static preview thumbnail inside the card (X: x+6, Y: y+6, W: 64, H: 36)
	if hBitmap != 0 {
		drawBitmap(hdc, hBitmap, x+6, y+6, 64, 36)
	} else {
		// Draw a placeholder dark gray rectangle
		placeholderRect := RECT{Left: int32(x + 6), Top: int32(y + 6), Right: int32(x + 6 + 64), Bottom: int32(y + 6 + 36)}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&placeholderRect)), brushSidebar)
	}

	// Draw label with end ellipsis to avoid shifting or overflow
	procSetTextColor.Call(hdc, uintptr(tColor))
	labelW, _ := syscall.UTF16PtrFromString(label)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(labelW)), ^uintptr(0), uintptr(unsafe.Pointer(&rect)), 0x00000024|0x00000004|0x00008000)
}

func drawFlatButton(hdc uintptr, label string, x, y, w, h int, isHovered bool, isRecordActive bool) {
	rect := RECT{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}

	brush := brushCard
	tColor := textCol
	if isRecordActive {
		redBrush, _, _ := procCreateSolidBrush.Call(uintptr(0x002424ef)) // Red
		brush = redBrush
		tColor = whiteCol
		defer procDeleteObject.Call(redBrush)
	} else if isHovered {
		brush = brushCardHover
		tColor = whiteCol
	}

	procSelectObject.Call(hdc, brush)
	procSelectObject.Call(hdc, penTransparent)
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 12, 12)

	// Draw Label
	procSetTextColor.Call(hdc, uintptr(tColor))
	labelW, _ := syscall.UTF16PtrFromString(label)
	if isRecordActive {
		labelW, _ = syscall.UTF16PtrFromString("Stop Recording")
	}
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(labelW)), ^uintptr(0), uintptr(unsafe.Pointer(&rect)), 0x00000025)
}

func drawBitmap(hdc uintptr, hBitmap uintptr, x, y, w, h int) {
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	oldObj, _, _ := procSelectObject.Call(memDC, hBitmap)

	procSetStretchBltMode.Call(hdc, 4) // HALFTONE
	procStretchBlt.Call(
		hdc, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		memDC, 0, 0, 128, 72,
		0x00CC0020, // SRCCOPY
	)

	procSelectObject.Call(memDC, oldObj)
	procDeleteDC.Call(memDC)
}

func drawLivePreview(hdc uintptr, targetHWND syscall.Handle, destX, destY, destW, destH int32) {
	progmanW, _ := syscall.UTF16PtrFromString("Progman")
	progmanHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(progmanW)), 0)

	var srcDC uintptr
	var srcX, srcY, srcW, srcH int32

	// Capture from full desktop screen so we get all windows and compositor layers
	srcDC, _, _ = procGetDC.Call(0)
	if srcDC == 0 {
		return
	}
	defer procReleaseDC.Call(0, srcDC)

	if targetHWND == 0 || targetHWND == syscall.Handle(progmanHwnd) {
		srcX = 0
		srcY = 0
		scrWVal, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN = 0
		scrHVal, _, _ := procGetSystemMetrics.Call(1) // SM_CYSCREEN = 1
		srcW = int32(scrWVal)
		srcH = int32(scrHVal)
	} else {
		var rect RECT
		ret, _, _ := procGetWindowRect.Call(uintptr(targetHWND), uintptr(unsafe.Pointer(&rect)))
		if ret == 0 {
			return
		}
		srcX = rect.Left
		srcY = rect.Top
		srcW = rect.Right - rect.Left
		srcH = rect.Bottom - rect.Top
	}

	if srcW <= 0 || srcH <= 0 {
		return
	}

	// Copy from screen to the double buffer
	procSetStretchBltMode.Call(hdc, 4) // HALFTONE
	procStretchBlt.Call(
		hdc, uintptr(destX), uintptr(destY), uintptr(destW), uintptr(destH),
		srcDC, uintptr(srcX), uintptr(srcY), uintptr(srcW), uintptr(srcH),
		0x00CC0020, // SRCCOPY
	)

	// Draw the scaled cursor on top of the preview
	type CURSORINFO struct {
		CbSize      uint32
		Flags       uint32
		HCursor     syscall.Handle
		PtScreenPos struct{ X, Y int32 }
	}

	var ci CURSORINFO
	ci.CbSize = uint32(unsafe.Sizeof(ci))
	ret, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci)))
	if ret != 0 && (ci.Flags&0x00000001) != 0 { // CURSOR_SHOWING = 0x00000001
		cursorX := ci.PtScreenPos.X
		cursorY := ci.PtScreenPos.Y

		if cursorX >= srcX && cursorX <= srcX+srcW && cursorY >= srcY && cursorY <= srcY+srcH {
			relX := cursorX - srcX
			relY := cursorY - srcY

			scaledX := destX + int32(float64(relX)*float64(destW)/float64(srcW))
			scaledY := destY + int32(float64(relY)*float64(destH)/float64(srcH))

			procDrawIconEx.Call(
				hdc,
				uintptr(scaledX),
				uintptr(scaledY),
				uintptr(ci.HCursor),
				0, 0, 0, 0,
				0x0003, // DI_NORMAL
			)
		}
	}
}

// Paints the download progress screen in GDI
func drawDownloadScreen(hwnd syscall.Handle, hdc uintptr) {
	var rect RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	width := rect.Right - rect.Left
	height := rect.Bottom - rect.Top

	// Double buffering
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	memBitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
	oldObj, _, _ := procSelectObject.Call(memDC, memBitmap)

	// Slate background fill
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&rect)), brushBg)

	// Draw loading title
	procSetBkMode.Call(memDC, 1) // TRANSPARENT
	procSetTextColor.Call(memDC, uintptr(whiteCol))

	hFont, _, _ := procGetStockObject.Call(17) // default
	procSelectObject.Call(memDC, hFont)

	loadText, _ := syscall.UTF16PtrFromString("Setting Up Fonts...")
	loadRect := RECT{Left: 0, Top: height/2 - 40, Right: width, Bottom: height/2 - 10}
	procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(loadText)), ^uintptr(0), uintptr(unsafe.Pointer(&loadRect)), 0x00000025)

	// Progress bar container (flat card outline)
	barW := int32(320)
	barH := int32(10)
	barX := width/2 - barW/2
	barY := height/2 + 5

	borderRect := RECT{Left: barX - 2, Top: barY - 2, Right: barX + barW + 2, Bottom: barY + barH + 2}
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&borderRect)), brushCard)

	// Fill progress bar (Material indigo fill)
	fillW := int32(float64(barW) * downloadProgress)
	fillRect := RECT{Left: barX, Top: barY, Right: barX + fillW, Bottom: barY + barH}
	procFillRect.Call(memDC, uintptr(unsafe.Pointer(&fillRect)), brushAccent)

	// Flush double-buffered DC to screen
	procBitBlt.Call(hdc, 0, 0, uintptr(width), uintptr(height), memDC, 0, 0, 0x00CC0020)

	procSelectObject.Call(memDC, oldObj)
	procDeleteObject.Call(memBitmap)
	procDeleteDC.Call(memDC)
}
