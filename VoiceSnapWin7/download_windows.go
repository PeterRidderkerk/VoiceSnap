package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"voicesnap/internal/engine"
	"voicesnap/internal/logger"
)

// Win32 constants for progress dialog
const (
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsVisible     = 0x10000000
	wsChild       = 0x40000000
	ssCenter      = 0x00000001
	pbmSetRange32 = 0x0406
	pbmSetPos     = 0x0402
	pbsSmooth     = 0x01

	progressClassName = "msctls_progress32"
	staticClassName   = "STATIC"
)

var (
	comctl32           = syscall.NewLazyDLL("comctl32.dll")
	procInitCommonCtls = comctl32.NewProc("InitCommonControls")
	procSendMessageW   = user32T.NewProc("SendMessageW")
	procUpdateWindow   = user32T.NewProc("UpdateWindow")
	procShowWindow     = user32T.NewProc("ShowWindow")
	procEnableWindow   = user32T.NewProc("EnableWindow")
	procMoveWindow     = user32T.NewProc("MoveWindow")
	procSetWindowTextW = user32T.NewProc("SetWindowTextW")
	procPeekMessageW   = user32T.NewProc("PeekMessageW")
	procMessageBoxW    = user32T.NewProc("MessageBoxW")
	procGetDesktopWin  = user32T.NewProc("GetDesktopWindow")
)

const (
	mbOK          = 0x00000000
	mbOKCancel    = 0x00000001
	mbIconInfo    = 0x00000040
	mbIconWarning = 0x00000030
	mbIconError   = 0x00000010
	idOK          = 1
	idCancel      = 2
)

func messageBox(hwnd uintptr, text, caption string, flags uint32) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	ret, _, _ := procMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), uintptr(flags))
	return int(ret)
}

// pumpMessages processes pending Windows messages to keep UI responsive.
func pumpMessages() {
	var m trayMsg
	for {
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1) // PM_REMOVE=1
		if ret == 0 {
			break
		}
		procTranslateMessageT.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageT.Call(uintptr(unsafe.Pointer(&m)))
	}
}

type progressDialog struct {
	hwnd       uintptr
	hProgress  uintptr
	hLabel     uintptr
	totalBytes int64
}

func newProgressDialog(title string) *progressDialog {
	procInitCommonCtls.Call()

	titleU, _ := syscall.UTF16PtrFromString(title)
	clsName, _ := syscall.UTF16PtrFromString("VoiceSnapDlg")

	hInst, _, _ := kGetModuleHandleT.Call(0)

	// Register a simple window class using DefWindowProc
	wc := trayWndClassEx{
		cbSize:    uint32(unsafe.Sizeof(trayWndClassEx{})),
		wndProc:   procDefWindowProcW.Addr(),
		hInstance: hInst,
		className: clsName,
		hbrBg:     15 + 1, // COLOR_3DFACE + 1
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create main window (400x130, centered)
	desktop, _, _ := procGetDesktopWin.Call()
	var wa rect
	procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	wx := int(wa.left) + (int(wa.right-wa.left)-400)/2
	wy := int(wa.top) + (int(wa.bottom-wa.top)-130)/2

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(titleU)),
		wsOverlapped|wsCaption|wsSysMenu|wsVisible,
		uintptr(wx), uintptr(wy), 400, 130,
		desktop, 0, hInst, 0,
	)

	// Create label
	staticCls, _ := syscall.UTF16PtrFromString(staticClassName)
	waitText, _ := syscall.UTF16PtrFromString("preparing...")
	hLabel, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(staticCls)),
		uintptr(unsafe.Pointer(waitText)),
		wsVisible|wsChild|ssCenter,
		10, 10, 370, 20,
		hwnd, 0, hInst, 0,
	)

	// Create progress bar
	progCls, _ := syscall.UTF16PtrFromString(progressClassName)
	hProg, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(progCls)),
		0,
		wsVisible|wsChild|pbsSmooth,
		10, 40, 370, 25,
		hwnd, 0, hInst, 0,
	)

	procUpdateWindow.Call(hwnd)

	return &progressDialog{
		hwnd:      hwnd,
		hProgress: hProg,
		hLabel:    hLabel,
	}
}

func (d *progressDialog) setRange(max int64) {
	d.totalBytes = max
	procSendMessageW.Call(d.hProgress, pbmSetRange32, 0, uintptr(max/1024))
}

func (d *progressDialog) setPos(pos int64) {
	procSendMessageW.Call(d.hProgress, pbmSetPos, uintptr(pos/1024), 0)

	// Update label text
	var label string
	if d.totalBytes > 0 {
		pct := pos * 100 / d.totalBytes
		label = fmt.Sprintf("%d%%  (%s / %s)", pct, humanSize(pos), humanSize(d.totalBytes))
	} else {
		label = fmt.Sprintf("Downloaded %s", humanSize(pos))
	}
	labelU, _ := syscall.UTF16PtrFromString(label)
	procSetWindowTextW.Call(d.hLabel, uintptr(unsafe.Pointer(labelU)))
}

func (d *progressDialog) close() {
	if d.hwnd != 0 {
		procDestroyWindowT.Call(d.hwnd)
		d.hwnd = 0
	}
}

func humanSize(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}

// EnsureModel checks if model exists; if not, prompts user and downloads.
// Must be called from the main OS thread (runtime.LockOSThread).
// Returns true if model is ready, false if user cancelled or download failed.
func EnsureModel() bool {
	if engine.ModelExists() {
		return true
	}

	// Lock to OS thread for Win32 UI
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Prompt user
	var prompt string
	if isChinese() {
		prompt = "\u8bed\u97f3\u6a21\u578b\u672a\u627e\u5230\uff0c\u9700\u8981\u4e0b\u8f7d (~152MB)\u3002\n\u662f\u5426\u7acb\u5373\u4e0b\u8f7d\uff1f"
	} else {
		prompt = "Voice model not found. Need to download (~152MB).\nDownload now?"
	}

	ret := messageBox(0, prompt, "VoiceSnap", mbOKCancel|mbIconInfo)
	if ret != idOK {
		return false
	}

	return downloadModel()
}

func downloadModel() bool {
	cfg, _ := loadDownloadURLs()

	urls := []string{cfg.ModelDownloadUrl, cfg.FallbackModelDownloadUrl}
	for _, url := range urls {
		if url == "" {
			continue
		}
		logger.Info("Downloading model from %s", url)
		if err := downloadAndExtract(url); err != nil {
			logger.Error("Download failed from %s: %v", url, err)
			continue
		}
		if engine.ModelExists() {
			logger.Info("Model downloaded successfully")
			return true
		}
	}

	var errMsg string
	if isChinese() {
		errMsg = "\u6a21\u578b\u4e0b\u8f7d\u5931\u8d25\uff0c\u8bf7\u624b\u52a8\u5c06\u6a21\u578b\u6587\u4ef6\u653e\u5165 models/sensevoice/ \u76ee\u5f55\u3002"
	} else {
		errMsg = "Model download failed. Please manually place model files in models/sensevoice/ directory."
	}
	messageBox(0, errMsg, "VoiceSnap", mbOK|mbIconError)
	return false
}

func loadDownloadURLs() (struct{ ModelDownloadUrl, FallbackModelDownloadUrl string }, error) {
	cfg := struct{ ModelDownloadUrl, FallbackModelDownloadUrl string }{
		ModelDownloadUrl:         "http://www.maikami.com/voicesnap/sensevoice.zip",
		FallbackModelDownloadUrl: "https://modelscope.cn/models/sherpa-onnx/sherpa-onnx-sense-voice-zh-en-ja-ko-yue/resolve/master/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2",
	}
	return cfg, nil
}

// downloadState is shared between the UI goroutine and the download goroutine.
type downloadState struct {
	mu         sync.Mutex
	downloaded int64
	totalSize  int64
	done       bool
	err        error
}

func downloadAndExtract(url string) error {
	// Determine target directory
	modelDir := engine.ModelDir()
	os.MkdirAll(modelDir, 0755)

	// Download to temp file
	tmpFile := filepath.Join(os.TempDir(), "voicesnap_model_dl.tmp")
	defer os.Remove(tmpFile)

	var dlTitle string
	if isChinese() {
		dlTitle = "\u4e0b\u8f7d\u8bed\u97f3\u6a21\u578b"
	} else {
		dlTitle = "Downloading Voice Model"
	}

	// Create progress dialog on the current (UI) thread
	dlg := newProgressDialog(dlTitle)
	defer dlg.close()

	// Shared state between download goroutine and UI thread
	var state downloadState

	// Start download in a background goroutine
	go func() {
		err := doHTTPDownload(url, tmpFile, &state)
		state.mu.Lock()
		state.err = err
		state.done = true
		state.mu.Unlock()
	}()

	// UI thread: pump messages and update progress until download completes
	for {
		pumpMessages()

		state.mu.Lock()
		downloaded := state.downloaded
		totalSize := state.totalSize
		done := state.done
		dlErr := state.err
		state.mu.Unlock()

		if totalSize > 0 && dlg.totalBytes == 0 {
			dlg.setRange(totalSize)
		}
		dlg.setPos(downloaded)

		if done {
			if dlErr != nil {
				return dlErr
			}
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	dlg.close()

	// Extract based on extension
	if strings.HasSuffix(url, ".zip") {
		return extractZip(tmpFile, modelDir)
	}
	return extractTarBz2(tmpFile, modelDir)
}

// doHTTPDownload runs in a background goroutine, updating state atomically.
func doHTTPDownload(url, tmpFile string, state *downloadState) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	state.mu.Lock()
	state.totalSize = resp.ContentLength
	state.mu.Unlock()

	out, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("create temp file: %v", err)
	}
	defer out.Close()

	var downloaded int64
	buf := make([]byte, 64*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("write error: %v", wErr)
			}
			downloaded += int64(n)
			state.mu.Lock()
			state.downloaded = downloaded
			state.mu.Unlock()
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("download error: %v", readErr)
		}
	}
	return nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		// We only need model.int8.onnx (or model.onnx) and tokens.txt
		if name != "model.int8.onnx" && name != "model.onnx" && name != "tokens.txt" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s in zip: %v", name, err)
		}

		outPath := filepath.Join(destDir, name)
		outFile, err := os.Create(outPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %s: %v", outPath, err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("extract %s: %v", name, err)
		}
		logger.Info("Extracted %s", outPath)
	}
	return nil
}

func extractTarBz2(archivePath, destDir string) error {
	logger.Error("tar.bz2 extraction not implemented, please use zip format")
	return fmt.Errorf("tar.bz2 format not supported, please download zip format")
}

// suppress unused import warning for sync/atomic
var _ = atomic.LoadInt32
