//go:build darwin

package hotkey

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#import <Cocoa/Cocoa.h>
#include <ApplicationServices/ApplicationServices.h>
#include <pthread.h>

static volatile int g_keysDown[128];
static CFMachPortRef g_eventTap = NULL;

// CGEventTap callback — runs on dedicated thread
static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type,
                              CGEventRef event, void* refcon) {
	(void)proxy; (void)refcon;

	// macOS auto-disables event taps that are slow. Re-enable immediately.
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		if (g_eventTap) CGEventTapEnable(g_eventTap, true);
		return event;
	}

	int keyCode = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
	if (keyCode < 0 || keyCode >= 128) return event;

	if (type == kCGEventKeyDown) {
		g_keysDown[keyCode] = 1;
	} else if (type == kCGEventKeyUp) {
		g_keysDown[keyCode] = 0;
	} else if (type == kCGEventFlagsChanged) {
		CGEventFlags flags = CGEventGetFlags(event);
		int down = 0;
		switch (keyCode) {
		case 0x3B: case 0x3E: down = (flags & kCGEventFlagMaskControl)   ? 1 : 0; break;
		case 0x38: case 0x3C: down = (flags & kCGEventFlagMaskShift)     ? 1 : 0; break;
		case 0x3A: case 0x3D: down = (flags & kCGEventFlagMaskAlternate) ? 1 : 0; break;
		case 0x37: case 0x36: down = (flags & kCGEventFlagMaskCommand)   ? 1 : 0; break;
		case 0x39:            down = (flags & kCGEventFlagMaskAlphaShift) ? 1 : 0; break;
		}
		g_keysDown[keyCode] = down;
	}

	return event;
}

// Dedicated thread for the CGEventTap run loop
static void* tapThreadFunc(void* arg) {
	(void)arg;

	CGEventMask mask = (1 << kCGEventKeyDown) | (1 << kCGEventKeyUp) | (1 << kCGEventFlagsChanged);
	g_eventTap = CGEventTapCreate(
		kCGHIDEventTap,
		kCGHeadInsertEventTap,
		kCGEventTapOptionListenOnly,
		mask,
		tapCallback,
		NULL
	);

	if (!g_eventTap) {
		NSLog(@"[hotkey] CGEventTapCreate failed — Accessibility permission not granted?");
		return NULL;
	}

	CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, g_eventTap, 0);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), source, kCFRunLoopCommonModes);
	CGEventTapEnable(g_eventTap, true);

	NSLog(@"[hotkey] CGEventTap started (AXTrusted=%d)", AXIsProcessTrusted());

	CFRunLoopRun(); // blocks forever, processing events

	CFRelease(source);
	return NULL;
}

static int ensureAccessibility(void) {
	NSDictionary* opts = @{(__bridge NSString*)kAXTrustedCheckOptionPrompt: @YES};
	return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts) ? 1 : 0;
}

static void startKeyMonitor(void) {
	static int started = 0;
	if (started) return;
	started = 1;

	pthread_t thread;
	pthread_create(&thread, NULL, tapThreadFunc, NULL);
	pthread_detach(thread);
}

static int isKeyDown(int keyCode) {
	if (keyCode >= 0 && keyCode < 128) return g_keysDown[keyCode];
	return 0;
}
*/
import "C"

import "voicesnap/internal/logger"

// macOS key code mapping from VK (Windows-style) to macOS CGKeyCode.
var vkToMac = map[int]int{
	0x11: 0x3B, // Ctrl -> kVK_Control
	0xA2: 0x3B, // L-Ctrl -> kVK_Control
	0xA3: 0x3E, // R-Ctrl -> kVK_RightControl
	0x12: 0x3A, // Alt -> kVK_Option
	0xA4: 0x3A, // L-Alt -> kVK_Option
	0xA5: 0x3D, // R-Alt -> kVK_RightOption
	0x10: 0x38, // Shift -> kVK_Shift
	0xA0: 0x38, // L-Shift -> kVK_Shift
	0xA1: 0x3C, // R-Shift -> kVK_RightShift
	0x14: 0x39, // Caps Lock
	0x20: 0x31, // Space
	0x09: 0x30, // Tab
	0x0D: 0x24, // Enter/Return
	0x5B: 0x37, // L-Cmd (Win)
	0x5C: 0x36, // R-Cmd
	0x1B: 0x35, // Esc
}

type darwinListener struct{}

func newPlatformListener() Listener {
	trusted := C.ensureAccessibility()
	if trusted == 0 {
		logger.Info("Accessibility permission not granted — hotkey won't work until granted")
	} else {
		logger.Info("Accessibility permission granted")
	}
	C.startKeyMonitor()
	return &darwinListener{}
}

func (l *darwinListener) IsKeyDown(vk int) bool {
	macKey, ok := vkToMac[vk]
	if !ok {
		if vk >= 0x41 && vk <= 0x5A {
			macKey = vkToMacAlpha(vk)
		} else {
			return false
		}
	}
	return C.isKeyDown(C.int(macKey)) != 0
}

func (l *darwinListener) IsAnyOtherKeyPressed(excludeVK int) bool {
	modifiers := []int{0x11, 0xA2, 0xA3, 0x12, 0xA4, 0xA5, 0x10, 0xA0, 0xA1}
	for _, k := range modifiers {
		if k == excludeVK {
			continue
		}
		if l.IsKeyDown(k) {
			return true
		}
	}
	for vk := 0x41; vk <= 0x5A; vk++ {
		if vk == excludeVK {
			continue
		}
		if l.IsKeyDown(vk) {
			return true
		}
	}
	return false
}

func vkToMacAlpha(vk int) int {
	alphaMap := map[int]int{
		0x41: 0x00, 0x42: 0x0B, 0x43: 0x08, 0x44: 0x02,
		0x45: 0x0E, 0x46: 0x03, 0x47: 0x05, 0x48: 0x04,
		0x49: 0x22, 0x4A: 0x26, 0x4B: 0x28, 0x4C: 0x25,
		0x4D: 0x2E, 0x4E: 0x2D, 0x4F: 0x1F, 0x50: 0x23,
		0x51: 0x0C, 0x52: 0x0F, 0x53: 0x01, 0x54: 0x11,
		0x55: 0x20, 0x56: 0x09, 0x57: 0x0D, 0x58: 0x07,
		0x59: 0x10, 0x5A: 0x06,
	}
	if code, ok := alphaMap[vk]; ok {
		return code
	}
	return 0
}
