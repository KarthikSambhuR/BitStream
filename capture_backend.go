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
			Detail: "Windows Graphics Capture is available. Browser capture prompts are disabled while the native WGC preview renderer is being wired in.",
			Ready:  true,
		}
	}
	return captureBackendStatus{
		Name:   string(captureBackendDesktopCrop),
		Detail: "Using the app-owned desktop capture preview so WebView2 does not show sharing prompts.",
		Ready:  false,
	}
}

func windowsGraphicsCaptureAvailable() bool {
	// WGC requires Windows 10 1903+. The frame-pool recorder stays isolated from
	// the Wails UI so capture-specific state does not leak into the frontend.
	major, _, build := windowsVersion()
	return major >= 10 && build >= 18362 && nativeWGCIsSupported()
}
