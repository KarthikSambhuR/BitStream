package main

type captureBackendKind string

const (
	captureBackendDesktopCrop captureBackendKind = "Desktop crop fallback"
	captureBackendWGC         captureBackendKind = "Windows Graphics Capture"
)

type captureBackendStatus struct {
	Name   string
	Detail string
	Ready  bool
}

func activeCaptureBackendStatus() captureBackendStatus {
	if windowsGraphicsCaptureAvailable() {
		return captureBackendStatus{
			Name:   string(captureBackendWGC),
			Detail: "Windows Graphics Capture is available; native frame-pool recording is the next implementation layer.",
			Ready:  true,
		}
	}
	return captureBackendStatus{
		Name:   string(captureBackendDesktopCrop),
		Detail: "Using visible desktop-region capture until the native WGC frame-pool recorder is complete.",
		Ready:  false,
	}
}

func windowsGraphicsCaptureAvailable() bool {
	// WGC requires Windows 10 1903+. The frame-pool recorder stays isolated from
	// the Wails UI so capture-specific state does not leak into the frontend.
	major, _, build := windowsVersion()
	return major >= 10 && build >= 18362 && nativeWGCIsSupported()
}
