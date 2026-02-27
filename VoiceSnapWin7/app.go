package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"voicesnap/internal/audio"
	"voicesnap/internal/config"
	"voicesnap/internal/engine"
	"voicesnap/internal/hotkey"
	"voicesnap/internal/input"
	"voicesnap/internal/logger"
	"voicesnap/internal/overlay"
	"voicesnap/internal/sound"
	"voicesnap/internal/textproc"
)

const (
	appVersion = "1.0.0-win7"
	appName    = "VoiceSnap"

	silenceThreshold       = 0.05
	silenceTimeoutDuration = 3 * time.Second
	silenceGracePeriod     = 2 * time.Second
)

// App holds all application state and orchestration logic.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	cfg      *config.Config
	recorder *audio.Recorder
	eng      engine.Engine
	hk       hotkey.Listener
	paster   input.Paster

	indicator overlay.Overlay
	tray      *Tray

	// State
	mu                sync.Mutex
	isRecording       bool
	isFreetalking     bool
	hotkeyActive      bool
	hotkeyPressTime   time.Time
	isCombination     bool
	hideGen           uint64 // atomic via sync/atomic functions (Go 1.20 compat)
	lastStopTime      time.Time
	silenceSince      time.Time
	freeTalkStart     time.Time
}

func RunApp() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := &App{
		ctx:    ctx,
		cancel: cancel,
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config: %v", err)
		cfg = config.Default()
	}
	app.cfg = cfg

	// Initialize audio recorder
	app.recorder = audio.NewRecorder()

	// Initialize hotkey listener
	app.hk = hotkey.New()

	// Initialize paster
	app.paster = input.NewPaster()

	// Create overlay indicator (native GDI+ layered window)
	app.indicator = overlay.New()
	app.indicator.OnDragged(func(x, y int) {
		app.cfg.IndicatorX = x
		app.cfg.IndicatorY = y
		config.Save(app.cfg)
	})

	// Volume callback
	app.recorder.OnVolume(func(vol float64) {
		app.indicator.SetVolume(vol)
		app.checkSilenceTimeout(vol)
	})

	// Ensure model files exist (download if needed)
	if !EnsureModel() {
		logger.Error("Model not available, exiting")
		return fmt.Errorf("model not available")
	}

	// Start hotkey loop
	app.wg.Add(1)
	go app.hotkeyLoop()

	// Initialize engine asynchronously
	go app.initEngine()

	// Create and run system tray (blocks until quit)
	app.tray = NewTray(app)
	app.tray.Run() // blocks

	// Cleanup
	app.cleanup()
	return nil
}

// hotkeyLoop polls the hotkey state at 30ms intervals.
func (a *App) hotkeyLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.pollHotkey()
		}
	}
}

func (a *App) pollHotkey() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.eng == nil {
		return
	}

	// Escape cancels any active recording
	if a.cfg.HotkeyVK != 0x1B && (a.isRecording || a.isFreetalking) && a.hk.IsKeyDown(0x1B) {
		a.isFreetalking = false
		a.isRecording = false
		a.lastStopTime = time.Now()
		a.recorder.Stop()
		logger.Info("Recording cancelled (Escape)")
		a.indicator.SetStatus(overlay.StatusCancelled, "\u5df2\u53d6\u6d88")
		if a.cfg.SoundFeedback {
			sound.PlayCancel()
		}
		a.delayedHide(1000)
		return
	}

	isDown := a.hk.IsKeyDown(a.cfg.HotkeyVK)

	if isDown {
		if !a.hotkeyActive {
			a.hotkeyActive = true
			a.isCombination = false
			a.hotkeyPressTime = time.Now()

			if a.isFreetalking {
				a.stopFreetalkLocked()
				return
			}
		} else {
			if !a.isCombination && a.hk.IsAnyOtherKeyPressed(a.cfg.HotkeyVK) {
				a.isCombination = true
				if a.isRecording {
					a.stopRecordingLocked(true)
				}
			}

			if !a.isRecording && !a.isCombination && time.Since(a.hotkeyPressTime) > 300*time.Millisecond && time.Since(a.lastStopTime) > 500*time.Millisecond {
				a.startRecordingLocked()
			}
		}
	} else if a.hotkeyActive {
		a.hotkeyActive = false

		if a.isRecording {
			a.stopRecordingLocked(a.isCombination)
		} else if !a.isCombination && time.Since(a.hotkeyPressTime) < 300*time.Millisecond && time.Since(a.lastStopTime) > 500*time.Millisecond {
			a.startFreetalkLocked()
		}
	}
}

func (a *App) startRecordingLocked() {
	if a.isRecording {
		return
	}
	a.isRecording = true
	atomic.AddUint64(&a.hideGen, 1)

	logger.Info("Recording started")
	a.positionIndicator()
	a.indicator.SetStatus(overlay.StatusRecording, "0:00")
	a.indicator.Show()
	if a.cfg.SoundFeedback {
		sound.PlayStart()
	}

	if err := a.recorder.Start(); err != nil {
		logger.Error("Failed to start recording: %v", err)
		a.isRecording = false
		return
	}
	a.startRecordingTimer(overlay.StatusRecording)
}

func (a *App) stopRecordingLocked(cancel bool) {
	if !a.isRecording {
		return
	}
	a.isRecording = false
	a.lastStopTime = time.Now()

	if cancel {
		a.recorder.Stop()
		logger.Info("Recording cancelled (combination key)")
		a.indicator.SetStatus(overlay.StatusCancelled, "\u5df2\u53d6\u6d88")
		if a.cfg.SoundFeedback {
			sound.PlayCancel()
		}
		a.delayedHide(1000)
		return
	}

	hasVoice := a.recorder.HasVoiceActivity()
	samples := a.recorder.StopAndGetSamples()

	a.indicator.SetStatus(overlay.StatusProcessing, "\u8bc6\u522b\u4e2d")
	go a.recognizeAndPaste(hasVoice, samples)
}

func (a *App) startFreetalkLocked() {
	if a.isRecording || a.isFreetalking {
		return
	}
	a.isFreetalking = true
	a.isRecording = true
	a.freeTalkStart = time.Now()
	a.silenceSince = time.Time{}
	atomic.AddUint64(&a.hideGen, 1)

	logger.Info("Free talk started")
	a.positionIndicator()
	a.indicator.SetStatus(overlay.StatusFreetalking, "0:00")
	a.indicator.Show()
	if a.cfg.SoundFeedback {
		sound.PlayStart()
	}

	if err := a.recorder.Start(); err != nil {
		logger.Error("Failed to start free talk recording: %v", err)
		a.isFreetalking = false
		a.isRecording = false
		return
	}
	a.startRecordingTimer(overlay.StatusFreetalking)
}

func (a *App) startRecordingTimer(status overlay.Status) {
	start := time.Now()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.mu.Lock()
				recording := a.isRecording
				a.mu.Unlock()
				if !recording {
					return
				}
				d := time.Since(start)
				a.indicator.SetStatus(status, fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60))
			}
		}
	}()
}

func (a *App) stopFreetalkLocked() {
	if !a.isFreetalking {
		return
	}
	a.isFreetalking = false
	a.isRecording = false
	a.lastStopTime = time.Now()

	hasVoice := a.recorder.HasVoiceActivity()
	samples := a.recorder.StopAndGetSamples()

	logger.Info("Free talk stopped")
	a.indicator.SetStatus(overlay.StatusProcessing, "\u8bc6\u522b\u4e2d")
	go a.recognizeAndPaste(hasVoice, samples)
}

func (a *App) checkSilenceTimeout(vol float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.isFreetalking {
		a.silenceSince = time.Time{}
		return
	}

	if time.Since(a.freeTalkStart) < silenceGracePeriod {
		return
	}

	if vol > silenceThreshold {
		a.silenceSince = time.Time{}
		return
	}

	if a.silenceSince.IsZero() {
		a.silenceSince = time.Now()
		return
	}

	if time.Since(a.silenceSince) >= silenceTimeoutDuration {
		logger.Info("Silence timeout in free talk mode, auto-stopping")
		a.silenceSince = time.Time{}
		a.isFreetalking = false
		a.isRecording = false
		a.lastStopTime = time.Now()
		go a.silenceAutoStop()
	}
}

func (a *App) silenceAutoStop() {
	hasVoice := a.recorder.HasVoiceActivity()
	samples := a.recorder.StopAndGetSamples()

	logger.Info("Free talk stopped (silence auto-stop)")
	a.indicator.SetStatus(overlay.StatusProcessing, "\u8bc6\u522b\u4e2d")
	a.recognizeAndPaste(hasVoice, samples)
}

func (a *App) recognizeAndPaste(hasVoice bool, samples []float32) {
	a.mu.Lock()
	soundFeedback := a.cfg.SoundFeedback
	hotkeyVK := a.cfg.HotkeyVK
	autoHide := a.cfg.AutoHide
	a.mu.Unlock()

	if !hasVoice {
		logger.Info("No voice activity detected")
		a.indicator.SetStatus(overlay.StatusNoVoice, "\u65e0\u8bed\u97f3")
		a.delayedHideIf(autoHide, 1500)
		return
	}

	if len(samples) == 0 {
		a.indicator.SetStatus(overlay.StatusNoContent, "\u65e0\u5185\u5bb9")
		a.delayedHideIf(autoHide, 1500)
		return
	}

	text, err := a.eng.Recognize(samples)
	if err != nil {
		logger.Error("Recognition failed: %v", err)
		a.indicator.SetStatus(overlay.StatusError, "\u9519\u8bef")
		a.delayedHideIf(autoHide, 2000)
		return
	}

	text = textproc.PostProcess(text)

	if text == "" {
		a.indicator.SetStatus(overlay.StatusNoContent, "\u65e0\u5185\u5bb9")
		a.delayedHideIf(autoHide, 1500)
		return
	}

	logger.Info("Recognized: %s", text)

	a.waitForHotkeyRelease(hotkeyVK)

	if err := a.paster.Paste(text); err != nil {
		logger.Error("Paste failed, trying fallback: %v", err)
		if err := a.paster.TypeText(text); err != nil {
			logger.Error("Fallback type also failed: %v", err)
		}
	}

	a.indicator.SetStatus(overlay.StatusDone, "\u5b8c\u6210")
	if soundFeedback {
		sound.PlayDone()
	}
	a.delayedHideIf(autoHide, 2000)
}

func (a *App) waitForHotkeyRelease(hotkeyVK int) {
	for i := 0; i < 50; i++ {
		if !a.hk.IsKeyDown(hotkeyVK) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
}

func (a *App) delayedHide(delayMs int) {
	if a.cfg.AutoHide {
		a.scheduleHide(delayMs)
	}
}

func (a *App) delayedHideIf(autoHide bool, delayMs int) {
	if autoHide {
		a.scheduleHide(delayMs)
	}
}

func (a *App) scheduleHide(delayMs int) {
	gen := atomic.LoadUint64(&a.hideGen)
	go func() {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
		if atomic.LoadUint64(&a.hideGen) == gen {
			a.indicator.Hide()
		}
	}()
}

func (a *App) initEngine() {
	logger.Info("Initializing ASR engine...")

	eng, err := engine.New()
	if err != nil {
		logger.Error("Engine initialization failed: %v", err)
		logger.Error("Please ensure model files exist in models/sensevoice/ directory")
		return
	}

	a.mu.Lock()
	a.eng = eng
	a.mu.Unlock()

	logger.Info("ASR engine ready: %s", eng.HardwareInfo())

	a.mu.Lock()
	hotkeyVK := a.cfg.HotkeyVK
	autoHide := a.cfg.AutoHide
	indX := a.cfg.IndicatorX
	indY := a.cfg.IndicatorY
	a.mu.Unlock()

	keyName := hotkey.GetKeyName(hotkeyVK)
	a.positionIndicatorAt(indX, indY)
	a.indicator.SetStatus(overlay.StatusReady, "\u6309\u4f4f"+keyName+"\u8bf4\u8bdd")
	a.indicator.Show()
	a.delayedHideIf(autoHide, 2000)
}

func (a *App) positionIndicator() {
	a.positionIndicatorAt(a.cfg.IndicatorX, a.cfg.IndicatorY)
}

func (a *App) positionIndicatorAt(x, y int) {
	if x != 0 || y != 0 {
		a.indicator.SetPosition(x, y)
		return
	}
	// Use Win32 API to get screen work area
	cx, cy := getScreenCenter()
	a.indicator.SetPosition(cx, cy)
}

func (a *App) cleanup() {
	logger.Info("Cleaning up...")
	a.cancel()
	if a.indicator != nil {
		a.indicator.Close()
	}
	if a.recorder != nil {
		a.recorder.Close()
	}
	if a.eng != nil {
		a.eng.Close()
	}
	a.wg.Wait()
	logger.Info("Cleanup complete")
}
