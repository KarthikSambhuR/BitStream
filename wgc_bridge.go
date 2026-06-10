//go:build wgc

package main

/*
#cgo CXXFLAGS: -std=c++20 -DWIN32_LEAN_AND_MEAN -DNOMINMAX -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/cppwinrt -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/um -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/shared -IC:/PROGRA~2/WI3CF2~1/10/Include/10.0.26100.0/winrt
#cgo LDFLAGS: -lole32 -lruntimeobject -lwindowsapp
int bitstream_wgc_is_supported();
int bitstream_wgc_can_create_item_for_hwnd(void* hwnd);
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
