package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

type windowConfig struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximized bool `json:"maximized"`
}

func loadWindowConfig() (windowConfig, bool) {
	path, err := windowConfigPath()
	if err != nil {
		return windowConfig{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return windowConfig{}, false
	}
	var cfg windowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return windowConfig{}, false
	}
	if cfg.Width < 980 || cfg.Height < 620 {
		return windowConfig{}, false
	}
	return cfg, true
}

func saveWindowConfig(cfg windowConfig) {
	if cfg.Width < 980 || cfg.Height < 620 {
		return
	}
	path, err := windowConfigPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

func windowConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "BitStream", "window.json"), nil
}

func findBitStreamWindow() uintptr {
	className, _ := syscall.UTF16PtrFromString("BitStreamWailsWindow")
	title, _ := syscall.UTF16PtrFromString("BitStream")
	hwnd, _, _ := procFindWindowW.Call(
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
	)
	return hwnd
}
