package main

import "unsafe"

func windowsVersion() (uint32, uint32, uint32) {
	var info RTL_OSVERSIONINFOW
	info.OSVersionInfoSize = uint32(unsafe.Sizeof(info))
	ret, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	if ret != 0 {
		return 0, 0, 0
	}
	return info.MajorVersion, info.MinorVersion, info.BuildNumber
}
