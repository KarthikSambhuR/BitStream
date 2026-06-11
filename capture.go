package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"sync"
	"syscall"
	"unsafe"
)

const (
	srccopy        = 0x00CC0020
	capTUREBLT     = 0x40000000
	biRGB          = 0
	dibRGBColors   = 0
	cursorShowing  = 0x00000001
	diNormal       = 0x0003
	pwRenderFull   = 0x00000002
	stretchQuality = 4
)

var previewCapture = previewCaptureState{}

type previewCaptureState struct {
	mu     sync.Mutex
	srcDC  uintptr
	memDC  uintptr
	bitmap uintptr
	oldObj uintptr
	width  int32
	height int32
	pixels []byte
	rgba   []byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

func refreshSourceListLight(tab int) {
	activeSources = nil
	currentTab = tab
	scrollOffset = 0
	selectedIndex = -1
	selectedHWND = 0

	progmanW, _ := syscall.UTF16PtrFromString("Progman")
	progmanHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(progmanW)), 0)

	if tab == 0 {
		if progmanHwnd != 0 {
			activeSources = append(activeSources, SourceItem{
				HWND: syscall.Handle(progmanHwnd),
				Name: "Entire Desktop Screen",
				Type: "screen",
			})
		}
		return
	}

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
		if title == "" || title == "Program Manager" || title == "BitStream" || title == "BitStream - Screen Recorder" || title == "Settings" {
			return 1
		}

		activeSources = append(activeSources, SourceItem{
			HWND: hwnd,
			Name: title,
			Type: "window",
		})
		return 1
	})
	procEnumWindows.Call(cb, 0)
}

func capturePreviewDataURL(index int, maxW, maxH int) string {
	previewCapture.mu.Lock()
	defer previewCapture.mu.Unlock()

	if index < 0 || index >= len(activeSources) {
		return ""
	}
	if maxW <= 0 {
		maxW = 960
	}
	if maxH <= 0 {
		maxH = 540
	}

	src := activeSources[index]
	x, y, srcW, srcH := previewSourceBounds(src)
	if srcW <= 0 || srcH <= 0 {
		return ""
	}

	dstW, dstH := fitInside(srcW, srcH, int32(maxW), int32(maxH))
	if dstW <= 0 || dstH <= 0 {
		return ""
	}

	if !previewCapture.ensure(dstW, dstH) {
		return ""
	}

	procSetStretchBltMode.Call(previewCapture.memDC, 3)
	ret, _, _ := procStretchBlt.Call(
		previewCapture.memDC, 0, 0, uintptr(dstW), uintptr(dstH),
		previewCapture.srcDC, uintptr(x), uintptr(y), uintptr(srcW), uintptr(srcH),
		uintptr(srccopy|capTUREBLT),
	)
	if ret == 0 {
		return ""
	}
	drawPreviewCursor(previewCapture.memDC, x, y, srcW, srcH, dstW, dstH)

	img := previewCapture.bitmapToRGBA()
	if img == nil {
		return ""
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 62}); err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

func capturePreviewRawFrame(index int, maxW, maxH int) PreviewFrame {
	previewCapture.mu.Lock()
	defer previewCapture.mu.Unlock()

	if index < 0 || index >= len(activeSources) {
		return PreviewFrame{}
	}
	if maxW <= 0 {
		maxW = 960
	}
	if maxH <= 0 {
		maxH = 540
	}

	src := activeSources[index]
	x, y, srcW, srcH := previewSourceBounds(src)
	if srcW <= 0 || srcH <= 0 {
		return PreviewFrame{}
	}

	dstW, dstH := fitInside(srcW, srcH, int32(maxW), int32(maxH))
	if dstW <= 0 || dstH <= 0 {
		return PreviewFrame{}
	}

	if !previewCapture.ensure(dstW, dstH) {
		return PreviewFrame{}
	}

	if src.Type == "window" {
		if !previewCapture.renderWindow(src.HWND, srcW, srcH, dstW, dstH) {
			return PreviewFrame{}
		}
		drawPreviewCursor(previewCapture.memDC, x, y, srcW, srcH, dstW, dstH)
	} else {
		procSetStretchBltMode.Call(previewCapture.memDC, stretchQuality)
		ret, _, _ := procStretchBlt.Call(
			previewCapture.memDC, 0, 0, uintptr(dstW), uintptr(dstH),
			previewCapture.srcDC, uintptr(x), uintptr(y), uintptr(srcW), uintptr(srcH),
			uintptr(srccopy|capTUREBLT),
		)
		if ret == 0 {
			return PreviewFrame{}
		}
		drawPreviewCursor(previewCapture.memDC, x, y, srcW, srcH, dstW, dstH)
	}

	pixels := previewCapture.bitmapToRGBABytes()
	if len(pixels) == 0 {
		return PreviewFrame{}
	}

	return PreviewFrame{
		Width:  int(dstW),
		Height: int(dstH),
		Pixels: base64.StdEncoding.EncodeToString(pixels),
	}
}

func (p *previewCaptureState) ensure(width, height int32) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if p.srcDC == 0 {
		p.srcDC, _, _ = procGetDC.Call(0)
		if p.srcDC == 0 {
			return false
		}
	}
	if p.memDC != 0 && p.bitmap != 0 && p.width == width && p.height == height {
		return true
	}
	p.releaseBitmap()
	p.memDC, _, _ = procCreateCompatibleDC.Call(p.srcDC)
	if p.memDC == 0 {
		return false
	}
	p.bitmap, _, _ = procCreateCompatibleBitmap.Call(p.srcDC, uintptr(width), uintptr(height))
	if p.bitmap == 0 {
		procDeleteDC.Call(p.memDC)
		p.memDC = 0
		return false
	}
	p.oldObj, _, _ = procSelectObject.Call(p.memDC, p.bitmap)
	p.width = width
	p.height = height
	p.pixels = make([]byte, int(width*height*4))
	p.rgba = make([]byte, int(width*height*4))
	return true
}

func (p *previewCaptureState) renderWindow(hwnd syscall.Handle, srcW, srcH, dstW, dstH int32) bool {
	if hwnd == 0 || srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 || p.srcDC == 0 || p.memDC == 0 {
		return false
	}

	fullDC, _, _ := procCreateCompatibleDC.Call(p.srcDC)
	if fullDC == 0 {
		return false
	}
	defer procDeleteDC.Call(fullDC)

	fullBitmap, _, _ := procCreateCompatibleBitmap.Call(p.srcDC, uintptr(srcW), uintptr(srcH))
	if fullBitmap == 0 {
		return false
	}
	defer procDeleteObject.Call(fullBitmap)

	oldObj, _, _ := procSelectObject.Call(fullDC, fullBitmap)
	defer procSelectObject.Call(fullDC, oldObj)

	printed, _, _ := procPrintWindow.Call(uintptr(hwnd), fullDC, pwRenderFull)
	if printed == 0 {
		return false
	}

	procSetStretchBltMode.Call(p.memDC, stretchQuality)
	scaled, _, _ := procStretchBlt.Call(
		p.memDC, 0, 0, uintptr(dstW), uintptr(dstH),
		fullDC, 0, 0, uintptr(srcW), uintptr(srcH),
		uintptr(srccopy),
	)
	return scaled != 0
}

func previewSourceBounds(src SourceItem) (int32, int32, int32, int32) {
	if src.Type == "window" && src.HWND != 0 {
		var rect RECT
		ret, _, _ := procGetWindowRect.Call(uintptr(src.HWND), uintptr(unsafe.Pointer(&rect)))
		if ret != 0 {
			w := rect.Right - rect.Left
			h := rect.Bottom - rect.Top
			if w > 0 && h > 0 {
				return rect.Left, rect.Top, w, h
			}
		}
	}
	return sourceBounds(src)
}

func (p *previewCaptureState) release() {
	p.releaseBitmap()
	if p.srcDC != 0 {
		procReleaseDC.Call(0, p.srcDC)
		p.srcDC = 0
	}
}

func (p *previewCaptureState) releaseBitmap() {
	if p.memDC != 0 {
		if p.oldObj != 0 {
			procSelectObject.Call(p.memDC, p.oldObj)
			p.oldObj = 0
		}
		procDeleteDC.Call(p.memDC)
		p.memDC = 0
	}
	if p.bitmap != 0 {
		procDeleteObject.Call(p.bitmap)
		p.bitmap = 0
	}
	p.width = 0
	p.height = 0
	p.pixels = nil
	p.rgba = nil
}

func (p *previewCaptureState) bitmapToRGBA() *image.RGBA {
	rgba := p.bitmapToRGBABytes()
	if len(rgba) == 0 {
		return nil
	}
	width := int(p.width)
	height := int(p.height)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, rgba)
	return img
}

func (p *previewCaptureState) bitmapToRGBABytes() []byte {
	if p.width <= 0 || p.height <= 0 || p.memDC == 0 || p.bitmap == 0 {
		return nil
	}
	bi := bitmapInfo{
		Header: bitmapInfoHeader{
			Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			Width:       p.width,
			Height:      -p.height,
			Planes:      1,
			BitCount:    32,
			Compression: biRGB,
			SizeImage:   uint32(len(p.pixels)),
		},
	}

	ret, _, _ := procGetDIBits.Call(
		p.memDC,
		p.bitmap,
		0,
		uintptr(p.height),
		uintptr(unsafe.Pointer(&p.pixels[0])),
		uintptr(unsafe.Pointer(&bi)),
		dibRGBColors,
	)
	if ret == 0 {
		return nil
	}

	width := int(p.width)
	height := int(p.height)
	if len(p.rgba) != width*height*4 {
		p.rgba = make([]byte, width*height*4)
	}
	for i := 0; i < width*height; i++ {
		b := p.pixels[i*4+0]
		g := p.pixels[i*4+1]
		r := p.pixels[i*4+2]
		p.rgba[i*4+0] = r
		p.rgba[i*4+1] = g
		p.rgba[i*4+2] = b
		p.rgba[i*4+3] = 255
	}
	return p.rgba
}

func drawPreviewCursor(hdc uintptr, srcX, srcY, srcW, srcH, dstW, dstH int32) {
	var ci CURSORINFO
	ci.CbSize = uint32(unsafe.Sizeof(ci))
	ret, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci)))
	if ret == 0 || (ci.Flags&cursorShowing) == 0 {
		return
	}
	cursorX := ci.PtScreenPos.X
	cursorY := ci.PtScreenPos.Y
	if cursorX < srcX || cursorX > srcX+srcW || cursorY < srcY || cursorY > srcY+srcH {
		return
	}
	relX := cursorX - srcX
	relY := cursorY - srcY
	scaledX := int32(float64(relX) * float64(dstW) / float64(srcW))
	scaledY := int32(float64(relY) * float64(dstH) / float64(srcH))
	procDrawIconEx.Call(
		hdc,
		uintptr(scaledX),
		uintptr(scaledY),
		uintptr(ci.HCursor),
		0, 0, 0, 0,
		diNormal,
	)
}

func selectedSourceBounds() (int32, int32, int32, int32) {
	if selectedIndex < 0 || selectedIndex >= len(activeSources) {
		return 0, 0, 640, 480
	}
	return sourceBounds(activeSources[selectedIndex])
}

func sourceBounds(src SourceItem) (int32, int32, int32, int32) {
	progmanW, _ := syscall.UTF16PtrFromString("Progman")
	progmanHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(progmanW)), 0)
	if src.HWND == 0 || src.HWND == syscall.Handle(progmanHwnd) || src.Type == "screen" {
		scrWVal, _, _ := procGetSystemMetrics.Call(0)
		scrHVal, _, _ := procGetSystemMetrics.Call(1)
		return 0, 0, int32(scrWVal), int32(scrHVal)
	}

	var rect RECT
	ret, _, _ := procGetWindowRect.Call(uintptr(src.HWND), uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return 0, 0, 640, 480
	}

	x := rect.Left
	y := rect.Top
	w := rect.Right - rect.Left
	h := rect.Bottom - rect.Top

	scrWVal, _, _ := procGetSystemMetrics.Call(0)
	scrHVal, _, _ := procGetSystemMetrics.Call(1)
	screenW := int32(scrWVal)
	screenH := int32(scrHVal)
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > screenW {
		w = screenW - x
	}
	if y+h > screenH {
		h = screenH - y
	}
	if w%2 != 0 {
		w--
	}
	if h%2 != 0 {
		h--
	}
	if w <= 0 {
		w = 640
	}
	if h <= 0 {
		h = 480
	}
	return x, y, w, h
}

func fitInside(srcW, srcH, maxW, maxH int32) (int32, int32) {
	if srcW <= 0 || srcH <= 0 || maxW <= 0 || maxH <= 0 {
		return 0, 0
	}
	dstW := maxW
	dstH := int32(float64(dstW) * float64(srcH) / float64(srcW))
	if dstH > maxH {
		dstH = maxH
		dstW = int32(float64(dstH) * float64(srcW) / float64(srcH))
	}
	if dstW < 2 {
		dstW = 2
	}
	if dstH < 2 {
		dstH = 2
	}
	return dstW, dstH
}
