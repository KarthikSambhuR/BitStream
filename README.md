# BitStream

BitStream is a native Windows screen and window recording application written in Go. By avoiding heavy web views or Electron or Wails frames, it maintains an exceptionally small memory footprint of approximately 3.5 MB RAM during idle states.

## Key Features

* Native Win32 UI: Built directly using Win32 API, GDI double-buffered graphics, and Desktop Window Manager (DWM) interfaces for an efficient user experience.
* Low Memory Footprint: Runs on approximately 3.5 MB of RAM by utilizing native OS rendering.
* Dynamic Font Loading: Automatically downloads and caches the Urbanist typeface on first launch, showing download progress directly in the interface.
* Live Video Preview: Displays a real-time preview of the target display or selected window, complete with cursor visibility.
* Nested FFmpeg Process: Launches the FFmpeg recording process directly nested under BitStream in Task Manager. This is achieved by utilizing the raw Win32 CreateProcess API with parent-process attributes.
* Clean Recording Stop: Sends a graceful quit command directly to FFmpeg to ensure video files are saved correctly without corruption.

## Prerequisites

To build and run BitStream, you need:

1. Windows Operating System.
2. Go compiler (version 1.16 or higher is recommended).
3. FFmpeg installed and available on the system PATH.

## Building and Running

You can compile BitStream as a standard Windows GUI application:

```cmd
go build -ldflags "-H windowsgui" -o BitStream.exe .
```

After building, run the generated executable:

```cmd
BitStream.exe
```

## How It Works

* Rendering: Uses GDI double-buffered drawing to prevent flickering when resizing or updating the interface.
* Window Management: Queries the system using EnumWindows and related Win32 APIs to list active application windows.
* Process Nesting: Uses the Windows API PROC_THREAD_ATTRIBUTE_PARENT_PROCESS attribute so that the FFmpeg subprocess appears under the BitStream process tree in Windows Task Manager, rather than as a separate background process.
