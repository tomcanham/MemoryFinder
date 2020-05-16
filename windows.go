package main

import (
	"fmt"
	"image"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

/*
BOOL CALLBACK EnumWindowsProc(
  _In_ HWND   hwnd,
  _In_ LPARAM lParam
);
*/

func utf16PtrToString(p *uint16, max int) string {
	if p == nil {
		return ""
	}
	// Find NUL terminator.
	end := unsafe.Pointer(p)
	n := 0
	for *(*uint16)(end) != 0 && n < max {
		end = unsafe.Pointer(uintptr(end) + unsafe.Sizeof(*p))
		n++
	}
	s := (*[(1 << 30) - 1]uint16)(unsafe.Pointer(p))[:n:n]
	return string(utf16.Decode(s))
}

var user32 = syscall.NewLazyDLL("user32.dll")
var dwmAPI = syscall.NewLazyDLL("Dwmapi.dll")

var winEnumDesktopWindows = user32.NewProc("EnumDesktopWindows")
var winGetWindowTextW = user32.NewProc("GetWindowTextW")
var winGetWindowRect = user32.NewProc("GetWindowRect")
var winDwmGetWindowAttribute = dwmAPI.NewProc("DwmGetWindowAttribute")
var winSetForegroundWindow = user32.NewProc("SetForegroundWindow")
var winBringWindowToTop = user32.NewProc("BringWindowToTop")
var winGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")

type winRECT struct {
	left, top, right, bottom uint32
}

type windowInfo struct {
	bounds    image.Rectangle
	processID uint32
}

func tryBringWindowToTop(hwnd uintptr) {
	r1, _, lastError := winSetForegroundWindow.Call(hwnd)
	if r1 != 1 {
		fmt.Printf("SetForegroundWindow failed: %v\n", lastError)
	}
	r1, _, lastError = winBringWindowToTop.Call(hwnd)
	if r1 != 1 {
		fmt.Printf("BringWindowToTop failed: %v\n", lastError)
	}
}

func getWindowInfoByName(fileName string, setForeground bool) windowInfo {
	var (
		windowRect, boundsRect winRECT
		processID              uint32
	)

	callback := syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
		buffer, _ := syscall.UTF16PtrFromString("                                                                                                                     ")
		winGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(buffer)), 110)
		windowTitle := utf16PtrToString(buffer, 110)
		if windowTitle == fileName {
			if setForeground {
				tryBringWindowToTop(hwnd)
			}
			winGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
			winGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&windowRect)))
			r1, _, _ := winDwmGetWindowAttribute.Call(hwnd, 9, uintptr(unsafe.Pointer(&boundsRect)), uintptr(unsafe.Sizeof(boundsRect)))
			if r1 == 0 {
				windowRect = boundsRect
			}

			return 0
		}

		return 1
	})

	winEnumDesktopWindows.Call(0, callback, 0)

	var iRect image.Rectangle
	iRect.Min.X = int(windowRect.left)
	iRect.Min.Y = int(windowRect.top)
	iRect.Max.X = int(windowRect.right)
	iRect.Max.Y = int(windowRect.bottom)

	return windowInfo{iRect, processID}
}
