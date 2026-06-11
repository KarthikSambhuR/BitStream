package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Win32 constants for CreateProcess with parent-process attribute
const (
	PROC_THREAD_ATTRIBUTE_PARENT_PROCESS uintptr = 0x00020000
	EXTENDED_STARTUPINFO_PRESENT                 = 0x00080000
)

type STARTUPINFOEX struct {
	StartupInfo   syscall.StartupInfo
	AttributeList uintptr // LPPROC_THREAD_ATTRIBUTE_LIST
}

type PROCESS_INFORMATION struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessId uint32
	ThreadId  uint32
}

var (
	procInitializeProcThreadAttributeList = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = kernel32.NewProc("DeleteProcThreadAttributeList")
	procCreateProcessW                    = kernel32.NewProc("CreateProcessW")
	procGetCurrentProcess                 = kernel32.NewProc("GetCurrentProcess")
	procWriteFile                         = kernel32.NewProc("WriteFile")
	procCreatePipe                        = kernel32.NewProc("CreatePipe")
	procSetHandleInformation              = kernel32.NewProc("SetHandleInformation")
	procTerminateProcess                  = kernel32.NewProc("TerminateProcess")
	procWaitForSingleObject               = kernel32.NewProc("WaitForSingleObject")
)

// Raw handles for direct stdin pipe communication
var (
	ffmpegStdinWrite syscall.Handle
	ffmpegProcHandle syscall.Handle
	ffmpegProcID     uint32
)

func openRecordingsFolder() {
	dir, _ := filepath.Abs("./recordings")
	_ = os.MkdirAll(dir, 0755)
	_ = exec.Command("explorer", dir).Start()
}

func toggleRecording(hwnd syscall.Handle) {
	if selectedHWND == 0 || !ffmpegAvailable {
		return
	}

	if !isRecording {
		startFFmpegRecording(hwnd)
	} else {
		stopFFmpegRecording()
	}
}

// buildFFmpegCmdLine constructs the full ffmpeg.exe command line string for CreateProcessW
func buildFFmpegCmdLine(outputPath string) string {
	src := activeSources[selectedIndex]

	args := []string{
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "gdigrab",
		"-framerate", "60",
		"-rtbufsize", "10M",
		"-draw_mouse", "1",
	}

	if src.Type == "screen" {
		args = append(args, "-i", "desktop")
	} else {
		args = append(args, "-i", "title="+src.Name)
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-crf", "20",
		"-threads", "1",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		outputPath,
	)

	// Quote any argument that contains spaces
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = quoteCmdArg(a)
	}
	return strings.Join(quoted, " ")
}

func quoteCmdArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
}

func startFFmpegRecording(hwnd syscall.Handle) {
	os.MkdirAll("recordings", 0755)

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("BitStream_%s.mp4", timestamp)
	outputPath := filepath.Join("recordings", filename)

	cmdLine := buildFFmpegCmdLine(outputPath)

	// ── Step 1: Create anonymous stdin pipe ─────────────────────────────────
	// The read end goes to ffmpeg's STDIN; we keep the write end to send "q".
	var stdinRead, stdinWrite syscall.Handle
	var sa syscall.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.InheritHandle = 1 // child must inherit read end

	ret, _, _ := procCreatePipe.Call(
		uintptr(unsafe.Pointer(&stdinRead)),
		uintptr(unsafe.Pointer(&stdinWrite)),
		uintptr(unsafe.Pointer(&sa)),
		0,
	)
	if ret == 0 {
		return
	}
	// Make write end non-inheritable so ffmpeg doesn't hold a ref to it
	procSetHandleInformation.Call(uintptr(stdinWrite), 1, 0) // HANDLE_FLAG_INHERIT=1, clear it

	// ── Step 2: Build PROC_THREAD_ATTRIBUTE_LIST with parent-process attr ───
	// First call: get required size
	var attrListSize uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrListSize)))

	attrListBuf := make([]byte, attrListSize)
	attrListPtr := uintptr(unsafe.Pointer(&attrListBuf[0]))

	procInitializeProcThreadAttributeList.Call(attrListPtr, 1, 0, uintptr(unsafe.Pointer(&attrListSize)))

	// Get our own process handle as the desired parent
	selfHandle, _, _ := procGetCurrentProcess.Call()

	// Set PROC_THREAD_ATTRIBUTE_PARENT_PROCESS
	procUpdateProcThreadAttribute.Call(
		attrListPtr,
		0,
		PROC_THREAD_ATTRIBUTE_PARENT_PROCESS,
		uintptr(unsafe.Pointer(&selfHandle)), // pointer to the handle value
		unsafe.Sizeof(selfHandle),
		0, 0,
	)

	// ── Step 3: Fill STARTUPINFOEX ──────────────────────────────────────────
	var siex STARTUPINFOEX
	siex.StartupInfo.Cb = uint32(unsafe.Sizeof(siex))
	siex.StartupInfo.Flags = syscall.STARTF_USESTDHANDLES | 0x00000100 // STARTF_USESHOWWINDOW
	siex.StartupInfo.ShowWindow = 0                                    // SW_HIDE
	siex.StartupInfo.StdInput = stdinRead
	siex.StartupInfo.StdOutput = syscall.Handle(0)
	siex.StartupInfo.StdErr = syscall.Handle(0)
	siex.AttributeList = attrListPtr

	// ── Step 4: CreateProcessW ──────────────────────────────────────────────
	cmdLineW, _ := syscall.UTF16PtrFromString(cmdLine)

	var pi PROCESS_INFORMATION
	creationFlags := uint32(EXTENDED_STARTUPINFO_PRESENT | 0x08000000) // +CREATE_NO_WINDOW

	r, _, _ := procCreateProcessW.Call(
		0,                                 // lpApplicationName (nil = use cmdline)
		uintptr(unsafe.Pointer(cmdLineW)), // lpCommandLine
		0,                                 // lpProcessAttributes
		0,                                 // lpThreadAttributes
		1,                                 // bInheritHandles = TRUE (stdin pipe)
		uintptr(creationFlags),
		0, // lpEnvironment
		0, // lpCurrentDirectory
		uintptr(unsafe.Pointer(&siex)),
		uintptr(unsafe.Pointer(&pi)),
	)

	// Clean up attribute list and child-side of pipe (ffmpeg owns it now)
	procDeleteProcThreadAttributeList.Call(attrListPtr)
	syscall.CloseHandle(stdinRead)

	if r == 0 {
		syscall.CloseHandle(stdinWrite)
		return
	}

	// Keep write handle; close the thread handle we don't need
	syscall.CloseHandle(pi.Thread)

	ffmpegStdinWrite = stdinWrite
	ffmpegProcHandle = pi.Process
	ffmpegProcID = pi.ProcessId
	// Keep a reference so stopFFmpegRecording can Wait on it
	isRecording = true
	recordingStart = time.Now()

	// ── Step 5: Recording timer ─────────────────────────────────────────────
	timerStopChan = make(chan bool)
	recordingTimer = time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-recordingTimer.C:
				elapsed := time.Since(recordingStart)
				minutes := int(elapsed.Minutes()) % 60
				seconds := int(elapsed.Seconds()) % 60
				timerString = fmt.Sprintf("%02d:%02d", minutes, seconds)
			case <-timerStopChan:
				return
			}
		}
	}()
}

func stopFFmpegRecording() {
	// Send "q\n" through the stdin pipe — FFmpeg's graceful quit signal
	if ffmpegStdinWrite != 0 {
		quit := []byte("q\n")
		var written uint32
		procWriteFile.Call(
			uintptr(ffmpegStdinWrite),
			uintptr(unsafe.Pointer(&quit[0])),
			uintptr(len(quit)),
			uintptr(unsafe.Pointer(&written)),
			0,
		)
		syscall.CloseHandle(ffmpegStdinWrite)
		ffmpegStdinWrite = 0
	}

	// Wait up to 8 seconds for ffmpeg to finish writing the file cleanly
	if ffmpegProcHandle != 0 {
		procWaitForSingleObject.Call(uintptr(ffmpegProcHandle), 8000)
		syscall.CloseHandle(ffmpegProcHandle)
		ffmpegProcHandle = 0
	}

	ffmpegProcID = 0
	isRecording = false
	stopRecordingTimer()
	timerString = "00:00"
}

func stopRecordingTimer() {
	if recordingTimer != nil {
		recordingTimer.Stop()
		close(timerStopChan)
		recordingTimer = nil
	}
}
