package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

// Loads fonts, downloading if not cached
func setupFonts(hwnd syscall.Handle) {
	os.MkdirAll("cache", 0755)
	fontPath = filepath.Join("cache", "Urbanist-Regular.ttf")

	if _, err := os.Stat(fontPath); os.IsNotExist(err) {
		// Start async downloader
		isDownloadingFont = true
		go downloadFontFile(hwnd)
	} else {
		loadRegisteredFont(hwnd)
	}
}

func downloadFontFile(hwnd syscall.Handle) {
	url := "https://github.com/google/fonts/raw/main/ofl/urbanist/Urbanist-Regular.ttf"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Font download error: %v\n", err)
		isDownloadingFont = false
		procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(fontPath)
	if err != nil {
		isDownloadingFont = false
		procInvalidateRect.Call(uintptr(hwnd), 0, 1)
		return
	}
	defer out.Close()

	totalSize := float64(resp.ContentLength)
	if totalSize <= 0 {
		totalSize = 400000 // Approximate fallback size
	}

	buffer := make([]byte, 4096)
	var downloadedSize float64 = 0

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			out.Write(buffer[:n])
			downloadedSize += float64(n)
			downloadProgress = downloadedSize / totalSize

			// Repaint progress bar on UI thread
			procInvalidateRect.Call(uintptr(hwnd), 0, 0)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		time.Sleep(2 * time.Millisecond) // Smooth visual progress bar fill
	}

	isDownloadingFont = false
	loadRegisteredFont(hwnd)
}

func loadRegisteredFont(hwnd syscall.Handle) {
	absPath, _ := filepath.Abs(fontPath)
	fontPathW, _ := syscall.UTF16PtrFromString(absPath)
	ret, _, _ := procAddFontResourceExW.Call(uintptr(unsafe.Pointer(fontPathW)), 0x10, 0) // FR_PRIVATE = 0x10
	if ret > 0 {
		fontLoaded = true
	}

	// Create GDI Fonts using "Urbanist"
	fontNameW, _ := syscall.UTF16PtrFromString("Urbanist")

	// Standard Regular Font
	hFontRegular, _, _ = procCreateFontW.Call(16, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(fontNameW)))
	// Bold labels
	hFontBold, _, _ = procCreateFontW.Call(16, 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(fontNameW)))
	// Large Title Font
	hFontLogo, _, _ = procCreateFontW.Call(26, 0, 0, 0, 800, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(fontNameW)))

	// Load listbox sources
	refreshSourceList()

	// Invalidate window to force render dashboard using Urbanist font
	procInvalidateRect.Call(uintptr(hwnd), 0, 1)
}
