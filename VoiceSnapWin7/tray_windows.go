package main

import (
	"runtime"
	"syscall"
	"unsafe"

	"voicesnap/internal/config"
	"voicesnap/internal/startup"
)

// Win32 Shell_NotifyIcon constants
const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	wmApp        = 0x8000
	wmTrayMsg    = wmApp + 1
	wmCommand    = 0x0111
	wmLButtonUp  = 0x0202
	wmRButtonUp  = 0x0205
	wmDestroy    = 0x0002
	wmClose      = 0x0010

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmRightAlign = 0x0008
	tpmBottomAlign = 0x0020
	tpmReturnCmd  = 0x0100

	idIconRes      = 3 // must match .rc
	idMenuShow     = 1001
	idMenuExit     = 1002
	idMenuHotkey   = 1003
	idMenuSound    = 1004
	idMenuStartup  = 1005
	idMenuGithub   = 1006

	mfChecked   = 0x00000008
	mfUnchecked = 0x00000000
	mfGrayed    = 0x00000001

	imageIcon   = 1
	lrDefaultSize = 0x00000040
	lrShared      = 0x00008000
)

// NOTIFYICONDATAW (simplified for Win7 compat — V2 structure)
type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
}

var (
	shell32 = syscall.NewLazyDLL("shell32.dll")
	user32T = syscall.NewLazyDLL("user32.dll")
	kern32T = syscall.NewLazyDLL("kernel32.dll")

	procShellNotifyIcon   = shell32.NewProc("Shell_NotifyIconW")
	procRegisterClassExW  = user32T.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32T.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32T.NewProc("DefWindowProcW")
	procGetMessageW       = user32T.NewProc("GetMessageW")
	procTranslateMessageT = user32T.NewProc("TranslateMessage")
	procDispatchMessageT  = user32T.NewProc("DispatchMessageW")
	procPostQuitMessage   = user32T.NewProc("PostQuitMessage")
	procDestroyWindowT    = user32T.NewProc("DestroyWindow")
	procCreatePopupMenu   = user32T.NewProc("CreatePopupMenu")
	procAppendMenuW       = user32T.NewProc("AppendMenuW")
	procTrackPopupMenu    = user32T.NewProc("TrackPopupMenu")
	procDestroyMenu       = user32T.NewProc("DestroyMenu")
	procGetCursorPos      = user32T.NewProc("GetCursorPos")
	procSetForegroundWin  = user32T.NewProc("SetForegroundWindow")
	procPostMessageW      = user32T.NewProc("PostMessageW")
	procLoadImageW        = user32T.NewProc("LoadImageW")
	kGetModuleHandleT     = kern32T.NewProc("GetModuleHandleW")
)

type trayWndClassEx struct {
	cbSize    uint32
	style     uint32
	wndProc   uintptr
	clsExtra  int32
	wndExtra  int32
	hInstance uintptr
	hIcon     uintptr
	hCursor   uintptr
	hbrBg     uintptr
	menuName  *uint16
	className *uint16
	hIconSm   uintptr
}

type trayMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type trayPoint struct{ x, y int32 }

var gTray *Tray

type Tray struct {
	app  *App
	hwnd uintptr
	nid  notifyIconData
}

func NewTray(app *App) *Tray {
	return &Tray{app: app}
}

func trayWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	t := gTray
	switch msg {
	case wmTrayMsg:
		switch lp {
		case wmRButtonUp:
			if t != nil {
				t.showContextMenu()
			}
		case wmLButtonUp:
			// Left click on tray — could show a tooltip or do nothing
		}
		return 0
	case wmCommand:
		switch wp {
		case idMenuShow:
			// No settings window in Win7 version
		case idMenuExit:
			if t != nil {
				t.removeTrayIcon()
			}
			procPostQuitMessage.Call(0)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func (t *Tray) Run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	gTray = t

	hInst, _, _ := kGetModuleHandleT.Call(0)

	// Load icon from exe resource (ID 3)
	hIcon, _, _ := procLoadImageW.Call(
		hInst,
		uintptr(idIconRes),
		imageIcon,
		16, 16,
		lrDefaultSize|lrShared,
	)

	cls, _ := syscall.UTF16PtrFromString("VoiceSnapTrayWin7")
	wc := trayWndClassEx{
		cbSize:    uint32(unsafe.Sizeof(trayWndClassEx{})),
		wndProc:   syscall.NewCallback(trayWndProc),
		hInstance: hInst,
		className: cls,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	t.hwnd, _, _ = procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cls)), 0, 0,
		0, 0, 0, 0, 0, 0, hInst, 0,
	)

	// Add tray icon
	t.nid = notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             t.hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayMsg,
		hIcon:            hIcon,
	}
	tip := "VoiceSnap Win7"
	tipU := syscall.StringToUTF16(tip)
	for i := 0; i < len(tipU) && i < 127; i++ {
		t.nid.szTip[i] = tipU[i]
	}

	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))

	// Message loop
	var m trayMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 || int32(ret) == -1 {
			break
		}
		procTranslateMessageT.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageT.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *Tray) showContextMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	zh := isChinese()

	// --- App name (disabled, just a label) ---
	nameLabel := "VoiceSnap (win7)"
	nameU, _ := syscall.UTF16PtrFromString(nameLabel)
	procAppendMenuW.Call(hMenu, mfString|mfGrayed, 0, uintptr(unsafe.Pointer(nameU)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)

	// --- Set Hotkey ---
	hotkeyLabel := "\u8bbe\u7f6e\u70ed\u952e"
	if !zh {
		hotkeyLabel = "Set Hotkey"
	}
	hotkeyU, _ := syscall.UTF16PtrFromString(hotkeyLabel)
	procAppendMenuW.Call(hMenu, mfString, idMenuHotkey, uintptr(unsafe.Pointer(hotkeyU)))

	// --- Sound feedback (checked/unchecked) ---
	soundLabel := "\u63d0\u793a\u97f3"
	if !zh {
		soundLabel = "Sound Feedback"
	}
	soundU, _ := syscall.UTF16PtrFromString(soundLabel)
	soundFlag := mfString | mfUnchecked
	if t.app != nil {
		t.app.mu.Lock()
		if t.app.cfg.SoundFeedback {
			soundFlag = mfString | mfChecked
		}
		t.app.mu.Unlock()
	}
	procAppendMenuW.Call(hMenu, uintptr(soundFlag), idMenuSound, uintptr(unsafe.Pointer(soundU)))

	// --- Startup (checked/unchecked) ---
	startupLabel := "\u5f00\u673a\u81ea\u542f\u52a8"
	if !zh {
		startupLabel = "Start at Login"
	}
	startupU, _ := syscall.UTF16PtrFromString(startupLabel)
	startupFlag := mfString | mfUnchecked
	if startup.IsEnabled() {
		startupFlag = mfString | mfChecked
	}
	procAppendMenuW.Call(hMenu, uintptr(startupFlag), idMenuStartup, uintptr(unsafe.Pointer(startupU)))

	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)

	// --- GitHub ---
	githubU, _ := syscall.UTF16PtrFromString("GitHub")
	procAppendMenuW.Call(hMenu, mfString, idMenuGithub, uintptr(unsafe.Pointer(githubU)))

	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)

	// --- Exit ---
	exitLabel := "\u9000\u51fa"
	if !zh {
		exitLabel = "Exit"
	}
	exitU, _ := syscall.UTF16PtrFromString(exitLabel)
	procAppendMenuW.Call(hMenu, mfString, idMenuExit, uintptr(unsafe.Pointer(exitU)))

	var pt trayPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWin.Call(t.hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(hMenu,
		tpmRightAlign|tpmBottomAlign|tpmReturnCmd,
		uintptr(pt.x), uintptr(pt.y), 0, t.hwnd, 0)

	// Post a WM_NULL to dismiss the menu
	procPostMessageW.Call(t.hwnd, 0, 0, 0)

	// Handle the selected command directly (TPM_RETURNCMD returns the ID)
	switch cmd {
	case idMenuHotkey:
		if t.app != nil {
			ShowHotkeyDialog(t.app)
		}
	case idMenuSound:
		if t.app != nil {
			t.app.mu.Lock()
			t.app.cfg.SoundFeedback = !t.app.cfg.SoundFeedback
			t.app.mu.Unlock()
			config.Save(t.app.cfg)
		}
	case idMenuStartup:
		if startup.IsEnabled() {
			startup.SetEnabled(false)
		} else {
			startup.SetEnabled(true)
		}
	case idMenuGithub:
		openURL("https://github.com/vorojar/VoiceSnap")
	case idMenuExit:
		t.removeTrayIcon()
		procPostQuitMessage.Call(0)
	}
}

func openURL(url string) {
	verb, _ := syscall.UTF16PtrFromString("open")
	urlU, _ := syscall.UTF16PtrFromString(url)
	shell32.NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(urlU)),
		0, 0, 1, // SW_SHOWNORMAL
	)
}

func (t *Tray) removeTrayIcon() {
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
}
