//go:build !native

package main

import (
	"context"
	"os/exec"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

type SourceDTO struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Type  string `json:"type"`
}

type PreviewFrame struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Pixels string `json:"pixels"`
}

type AppState struct {
	Sources          []SourceDTO `json:"sources"`
	CurrentTab       int         `json:"currentTab"`
	SelectedIndex    int         `json:"selectedIndex"`
	IsRecording      bool        `json:"isRecording"`
	Timer            string      `json:"timer"`
	FFmpegAvailable  bool        `json:"ffmpegAvailable"`
	CaptureBackend   string      `json:"captureBackend"`
	BackendDetail    string      `json:"backendDetail"`
	RecorderMode     string      `json:"recorderMode"`
	MemoryTargetMB   int         `json:"memoryTargetMb"`
	MemoryCeilingMB  int         `json:"memoryCeilingMb"`
	RecordingQuality string      `json:"recordingQuality"`
	WGCCanCapture    bool        `json:"wgcCanCapture"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_, lookupErr := exec.LookPath("ffmpeg")
	ffmpegAvailable = lookupErr == nil
	refreshSourceListLight(0)
}

func (a *App) shutdown(ctx context.Context) {
	if isRecording {
		stopFFmpegRecording()
	}
	previewCapture.release()
}

func (a *App) GetState() AppState {
	return buildAppState()
}

func (a *App) RefreshSources(tab int) AppState {
	if tab != 0 && tab != 1 {
		tab = currentTab
	}
	previewCapture.release()
	refreshSourceListLight(tab)
	return buildAppState()
}

func (a *App) SelectSource(index int) AppState {
	if index >= 0 && index < len(activeSources) {
		selectedIndex = index
		selectedHWND = activeSources[index].HWND
	}
	return buildAppState()
}

func (a *App) StartNativePreview(index int, x int, y int, width int, height int) bool {
	if index < 0 || index >= len(activeSources) {
		nativeWGCPreviewStop()
		return false
	}
	src := activeSources[index]
	if src.Type != "window" || src.HWND == 0 {
		nativeWGCPreviewStop()
		return false
	}
	parent := findBitStreamWindow()
	if parent == 0 {
		return false
	}
	if selectedIndex != index {
		selectedIndex = index
		selectedHWND = src.HWND
	}
	return nativeWGCPreviewStart(parent, uintptr(src.HWND), x, y, width, height)
}

func (a *App) MoveNativePreview(x int, y int, width int, height int) {
	nativeWGCPreviewMove(x, y, width, height)
}

func (a *App) StopNativePreview() {
	nativeWGCPreviewStop()
}

func (a *App) GetPreviewFrame(index int, maxW int, maxH int) string {
	return capturePreviewDataURL(index, maxW, maxH)
}

func (a *App) GetPreviewFrameRaw(index int, maxW int, maxH int) PreviewFrame {
	return capturePreviewRawFrame(index, maxW, maxH)
}

func (a *App) StartRecording(index int) AppState {
	if index >= 0 && index < len(activeSources) {
		selectedIndex = index
		selectedHWND = activeSources[index].HWND
	}
	if selectedHWND != 0 && ffmpegAvailable && !isRecording {
		startFFmpegRecording(0)
	}
	return buildAppState()
}

func (a *App) StopRecording() AppState {
	if isRecording {
		stopFFmpegRecording()
	}
	return buildAppState()
}

func (a *App) OpenRecordingsFolder() {
	openRecordingsFolder()
}

func (a *App) Minimise() {
	if a.ctx != nil {
		wailsruntime.WindowMinimise(a.ctx)
	}
}

func (a *App) ToggleMaximise() {
	if a.ctx != nil {
		wailsruntime.WindowToggleMaximise(a.ctx)
	}
}

func (a *App) Close() {
	a.saveWindowState()
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

func (a *App) SaveWindowState() {
	a.saveWindowState()
}

func (a *App) saveWindowState() {
	defer func() {
		_ = recover()
	}()
	if a.ctx == nil {
		return
	}
	width, height := wailsruntime.WindowGetSize(a.ctx)
	x, y := wailsruntime.WindowGetPosition(a.ctx)
	saveWindowConfig(windowConfig{
		Width:     width,
		Height:    height,
		X:         x,
		Y:         y,
		Maximized: wailsruntime.WindowIsMaximised(a.ctx),
	})
}

func buildAppState() AppState {
	timer := "00:00"
	if isRecording && !recordingStart.IsZero() {
		elapsed := time.Since(recordingStart)
		timer = formatElapsed(elapsed)
	} else if timerString != "" {
		timer = timerString
	}

	backend := activeCaptureBackendStatus()
	wgcCanCapture := false
	if selectedIndex >= 0 && selectedIndex < len(activeSources) {
		wgcCanCapture = nativeWGCCanCreateItemForHWND(uintptr(activeSources[selectedIndex].HWND))
	}
	return AppState{
		Sources:          sourceDTOs(),
		CurrentTab:       currentTab,
		SelectedIndex:    selectedIndex,
		IsRecording:      isRecording,
		Timer:            timer,
		FFmpegAvailable:  ffmpegAvailable,
		CaptureBackend:   backend.Name,
		BackendDetail:    backend.Detail,
		RecorderMode:     "Native video preview",
		MemoryTargetMB:   25,
		MemoryCeilingMB:  35,
		RecordingQuality: "60 FPS, x264 ultrafast, CRF 20",
		WGCCanCapture:    wgcCanCapture,
	}
}

func sourceDTOs() []SourceDTO {
	out := make([]SourceDTO, 0, len(activeSources))
	for i, src := range activeSources {
		out = append(out, SourceDTO{
			Index: i,
			Name:  src.Name,
			Type:  src.Type,
		})
	}
	return out
}

func formatElapsed(elapsed time.Duration) string {
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	return twoDigits(minutes) + ":" + twoDigits(seconds)
}

func twoDigits(v int) string {
	if v < 10 {
		return "0" + string(rune('0'+v))
	}
	return string(rune('0'+(v/10))) + string(rune('0'+(v%10)))
}
