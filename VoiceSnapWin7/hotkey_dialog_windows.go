package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"voicesnap/internal/config"
	"voicesnap/internal/hotkey"
	"voicesnap/internal/logger"
)

// Win32 message constants for hotkey dialog
const (
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
	wmPaint      = 0x000F
	wmSetFont    = 0x0030

	gwlWndProc = -4 // GWL_WNDPROC (32-bit) / GWLP_WNDPROC (64-bit)

	swShow = 5
)

var (
	procSetWindowLongPtrW = user32T.NewProc("SetWindowLongPtrW")
	procCallWindowProcW   = user32T.NewProc("CallWindowProcW")
	procGetStockObject    = syscall.NewLazyDLL("gdi32.dll").NewProc("GetStockObject")
	procSetFocus          = user32T.NewProc("SetFocus")
)

const defaultFont = 17 // DEFAULT_GUI_FONT

// hotkeyDialog state
var hkDlg struct {
	hwnd       uintptr
	hLabel     uintptr
	hKeyLabel  uintptr
	oldWndProc uintptr
	app        *App
	captured   bool
}

// validHotkeys are the virtual key codes that can be used as hotkeys.
var validHotkeys = map[int]bool{
	0xA2: true, // L-Ctrl
	0xA3: true, // R-Ctrl
	0xA4: true, // L-Alt
	0xA5: true, // R-Alt
	0xA0: true, // L-Shift
	0xA1: true, // R-Shift
	0x14: true, // Caps Lock
	0x20: true, // Space
}

func init() {
	// F1-F12
	for i := 0x70; i <= 0x7B; i++ {
		validHotkeys[i] = true
	}
}

func hotkeyDlgProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmKeyDown, wmSysKeyDown:
		vk := int(wp)
		// Map generic modifier to specific side
		switch vk {
		case 0x11: // VK_CONTROL → check which side
			if getAsyncKeyStateDl(0xA3)&0x8000 != 0 {
				vk = 0xA3
			} else {
				vk = 0xA2
			}
		case 0x12: // VK_MENU → check which side
			if getAsyncKeyStateDl(0xA5)&0x8000 != 0 {
				vk = 0xA5
			} else {
				vk = 0xA4
			}
		case 0x10: // VK_SHIFT → check which side
			if getAsyncKeyStateDl(0xA1)&0x8000 != 0 {
				vk = 0xA1
			} else {
				vk = 0xA0
			}
		}

		if validHotkeys[vk] {
			hkDlg.captured = true
			applyHotkey(vk)
			return 0
		}
		// Ignore invalid keys
		return 0

	case wmDestroy:
		hkDlg.hwnd = 0
		return 0
	}

	if hkDlg.oldWndProc != 0 {
		r, _, _ := procCallWindowProcW.Call(hkDlg.oldWndProc, hwnd, uintptr(msg), wp, lp)
		return r
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func getAsyncKeyStateDl(vk int) uint16 {
	ret, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetAsyncKeyState").Call(uintptr(vk))
	return uint16(ret)
}

func applyHotkey(vk int) {
	keyName := hotkey.GetKeyName(vk)

	// Update the label
	var text string
	if isChinese() {
		text = fmt.Sprintf("\u5df2\u8bbe\u7f6e\u4e3a: %s", keyName)
	} else {
		text = fmt.Sprintf("Set to: %s", keyName)
	}
	textU, _ := syscall.UTF16PtrFromString(text)
	procSetWindowTextW.Call(hkDlg.hKeyLabel, uintptr(unsafe.Pointer(textU)))

	// Save to config
	if hkDlg.app != nil {
		hkDlg.app.mu.Lock()
		hkDlg.app.cfg.HotkeyVK = vk
		hkDlg.app.mu.Unlock()
		config.Save(hkDlg.app.cfg)
		logger.Info("Hotkey changed to %s (0x%02X)", keyName, vk)
	}

	// Close dialog after a brief moment to show the result
	go func() {
		// Small delay so user can see the confirmation
		time.Sleep(600 * time.Millisecond)
		if hkDlg.hwnd != 0 {
			procDestroyWindowT.Call(hkDlg.hwnd)
		}
	}()
}

// ShowHotkeyDialog opens a small window that captures a key press to set as hotkey.
func ShowHotkeyDialog(app *App) {
	if hkDlg.hwnd != 0 {
		// Already open, bring to front
		procSetForegroundWin.Call(hkDlg.hwnd)
		return
	}

	hkDlg.app = app
	hkDlg.captured = false

	hInst, _, _ := kGetModuleHandleT.Call(0)

	clsName, _ := syscall.UTF16PtrFromString("VoiceSnapHotkeyDlg")

	wc := trayWndClassEx{
		cbSize:    uint32(unsafe.Sizeof(trayWndClassEx{})),
		wndProc:   syscall.NewCallback(hotkeyDlgProc),
		hInstance: hInst,
		className: clsName,
		hbrBg:     15 + 1, // COLOR_3DFACE + 1
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Title
	var title string
	if isChinese() {
		title = "\u8bbe\u7f6e\u70ed\u952e"
	} else {
		title = "Set Hotkey"
	}
	titleU, _ := syscall.UTF16PtrFromString(title)

	// Center on screen
	var wa rect
	procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	wx := int(wa.left) + (int(wa.right-wa.left)-340)/2
	wy := int(wa.top) + (int(wa.bottom-wa.top)-160)/2

	hkDlg.hwnd, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(titleU)),
		wsOverlapped|wsCaption|wsSysMenu|wsVisible,
		uintptr(wx), uintptr(wy), 340, 160,
		0, 0, hInst, 0,
	)

	// Get default font
	hFont, _, _ := procGetStockObject.Call(defaultFont)

	// Current hotkey label
	app.mu.Lock()
	currentVK := app.cfg.HotkeyVK
	app.mu.Unlock()
	currentName := hotkey.GetKeyName(currentVK)

	var currentLabel string
	if isChinese() {
		currentLabel = fmt.Sprintf("\u5f53\u524d\u70ed\u952e: %s", currentName)
	} else {
		currentLabel = fmt.Sprintf("Current hotkey: %s", currentName)
	}
	currentU, _ := syscall.UTF16PtrFromString(currentLabel)

	staticCls, _ := syscall.UTF16PtrFromString(staticClassName)
	hCurrent, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(staticCls)),
		uintptr(unsafe.Pointer(currentU)),
		wsVisible|wsChild|ssCenter,
		10, 15, 310, 20,
		hkDlg.hwnd, 0, hInst, 0,
	)
	procSendMessageW.Call(hCurrent, wmSetFont, hFont, 1)

	// Instruction label
	var instrLabel string
	if isChinese() {
		instrLabel = "\u8bf7\u6309\u4e0b\u65b0\u7684\u70ed\u952e...\n(Ctrl/Alt/Shift/CapsLock/F1-F12)"
	} else {
		instrLabel = "Press a new hotkey...\n(Ctrl/Alt/Shift/CapsLock/F1-F12)"
	}
	instrU, _ := syscall.UTF16PtrFromString(instrLabel)
	hkDlg.hKeyLabel, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(staticCls)),
		uintptr(unsafe.Pointer(instrU)),
		wsVisible|wsChild|ssCenter,
		10, 55, 310, 45,
		hkDlg.hwnd, 0, hInst, 0,
	)
	procSendMessageW.Call(hkDlg.hKeyLabel, wmSetFont, hFont, 1)

	procSetFocus.Call(hkDlg.hwnd)
	procUpdateWindow.Call(hkDlg.hwnd)
}
