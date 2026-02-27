package main

import (
	"syscall"
	"unsafe"
)

type rect struct {
	left, top, right, bottom int32
}

var (
	user32S                   = syscall.NewLazyDLL("user32.dll")
	procSystemParametersInfoW = user32S.NewProc("SystemParametersInfoW")
)

const spiGetWorkArea = 0x0030

// getScreenCenter returns coordinates for the overlay at bottom-center of the work area.
func getScreenCenter() (int, int) {
	var wa rect
	procSystemParametersInfoW.Call(
		spiGetWorkArea, 0,
		uintptr(unsafe.Pointer(&wa)), 0,
	)

	areaW := int(wa.right - wa.left)
	areaH := int(wa.bottom - wa.top)
	if areaW == 0 || areaH == 0 {
		// Fallback: use GetSystemMetrics
		sm := user32S.NewProc("GetSystemMetrics")
		w, _, _ := sm.Call(0)  // SM_CXSCREEN
		h, _, _ := sm.Call(1)  // SM_CYSCREEN
		areaW = int(w)
		areaH = int(h)
	}

	cx := int(wa.left) + (areaW-170)/2
	cy := int(wa.top) + areaH - 48 - 100
	return cx, cy
}

func isChinese() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultUILanguage")
	langID, _, _ := proc.Call()
	return (langID & 0x3FF) == 0x04
}
