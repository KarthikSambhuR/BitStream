//go:build wgc

package main

/*
#cgo CXXFLAGS: -std=c++20 -DWIN32_LEAN_AND_MEAN -DNOMINMAX -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/cppwinrt -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/um -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/shared -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/winrt
#cgo LDFLAGS: -lole32 -lruntimeobject -lwindowsapp -ld3d11 -ldxgi -ld3dcompiler
int bitstream_wgc_is_supported();
int bitstream_wgc_can_create_item_for_hwnd(void* hwnd);
int bitstream_wgc_preview_start(void* parent, void* capture, int x, int y, int width, int height);
void bitstream_wgc_preview_move(int x, int y, int width, int height);
void bitstream_wgc_preview_stop();
*/
import "C"
import "unsafe"

func nativeWGCIsSupported() bool {
	return C.bitstream_wgc_is_supported() == 1
}

func nativeWGCCanCreateItemForHWND(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	return C.bitstream_wgc_can_create_item_for_hwnd(unsafe.Pointer(hwnd)) == 1
}

func nativeWGCPreviewStart(parent, capture uintptr, x, y, width, height int) bool {
	if parent == 0 || capture == 0 || width <= 0 || height <= 0 {
		return false
	}
	return C.bitstream_wgc_preview_start(
		unsafe.Pointer(parent),
		unsafe.Pointer(capture),
		C.int(x),
		C.int(y),
		C.int(width),
		C.int(height),
	) == 1
}

func nativeWGCPreviewMove(x, y, width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	C.bitstream_wgc_preview_move(C.int(x), C.int(y), C.int(width), C.int(height))
}

func nativeWGCPreviewStop() {
	C.bitstream_wgc_preview_stop()
}
