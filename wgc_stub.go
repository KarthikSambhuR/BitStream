//go:build !wgc

package main

func nativeWGCIsSupported() bool {
	return false
}

func nativeWGCCanCreateItemForHWND(hwnd uintptr) bool {
	return false
}
