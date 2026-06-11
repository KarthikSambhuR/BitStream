//go:build !wgc

package main

func nativeWGCIsSupported() bool {
	return false
}

func nativeWGCCanCreateItemForHWND(hwnd uintptr) bool {
	return false
}

func nativeWGCPreviewStart(parent, capture uintptr, x, y, width, height int) bool {
	return false
}

func nativeWGCPreviewMove(x, y, width, height int) {}

func nativeWGCPreviewStop() {}
