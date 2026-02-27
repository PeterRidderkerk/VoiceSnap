//go:build darwin

package overlay

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <dispatch/dispatch.h>
#include <string.h>

// ---- Constants (match Windows overlay_windows.go) ----
#define CAP_W  170.0f
#define CAP_H  48.0f
#define CAP_R  24.0f
#define BAR_W  5.0f
#define BAR_GAP 4.0f
#define BAR_R  2.5f
#define N_BARS 5
#define TXT_SZ 12.0f
#define TXT_GAP 12.0f

// ---- Shared state (written by Go, read on main thread) ----
typedef struct {
	float barHeights[N_BARS];
	float barR, barG, barB;
	char  text[128];
	int   status;
	float volumes[N_BARS];
	float animTime;
	float fadeProgress;
	float fadeTarget;
	int   posX, posY;
	int   needsPosition;
	int   hasPosition;
	int   dragEnded;
	int   dragX, dragY;
} OverlayState;

static OverlayState g_state = {
	.barHeights = {14, 14, 14, 14, 14},
	.barR = 0, .barG = 0.478f, .barB = 1.0f,
};

// ---- Custom NSView for CG drawing ----
@interface VoiceSnapOverlayView : NSView
@end

@implementation VoiceSnapOverlayView

- (void)drawRect:(NSRect)dirtyRect {
	CGContextRef ctx = [[NSGraphicsContext currentContext] CGContext];
	float w = self.bounds.size.width;
	float h = self.bounds.size.height;

	CGContextClearRect(ctx, self.bounds);

	// Capsule background (semi-transparent dark)
	CGRect capsuleRect = CGRectMake(0, 0, w, h);
	CGPathRef capsulePath = CGPathCreateWithRoundedRect(capsuleRect, CAP_R, CAP_R, NULL);
	CGContextAddPath(ctx, capsulePath);
	CGContextSetRGBFillColor(ctx, 28.0f/255, 28.0f/255, 30.0f/255, 0.85f);
	CGContextFillPath(ctx);

	// Inner border
	CGRect borderRect = CGRectInset(capsuleRect, 0.5f, 0.5f);
	CGPathRef borderPath = CGPathCreateWithRoundedRect(borderRect, CAP_R - 0.5f, CAP_R - 0.5f, NULL);
	CGContextAddPath(ctx, borderPath);
	CGContextSetRGBStrokeColor(ctx, 1, 1, 1, 32.0f/255);
	CGContextSetLineWidth(ctx, 1);
	CGContextStrokePath(ctx);
	CGPathRelease(capsulePath);
	CGPathRelease(borderPath);

	// Measure text
	float textW = 0;
	NSString* text = nil;
	NSDictionary* textAttrs = nil;
	if (g_state.text[0] != '\0') {
		text = [NSString stringWithUTF8String:g_state.text];
		textAttrs = @{
			NSFontAttributeName: [NSFont systemFontOfSize:TXT_SZ weight:NSFontWeightRegular],
			NSForegroundColorAttributeName: [NSColor colorWithRed:180.0f/255 green:180.0f/255 blue:180.0f/255 alpha:1.0f]
		};
		textW = [text sizeWithAttributes:textAttrs].width;
	}

	// Layout: bars + text, centered
	float barsW = N_BARS * BAR_W + (N_BARS - 1) * BAR_GAP;
	float totalW = barsW;
	if (textW > 0) totalW += TXT_GAP + textW;
	float startX = (w - totalW) / 2;

	// Draw bars
	for (int i = 0; i < N_BARS; i++) {
		float bx = startX + i * (BAR_W + BAR_GAP);
		float bh = g_state.barHeights[i];
		if (bh < 6) bh = 6;
		if (bh > 40) bh = 40;
		float by = (h - bh) / 2;

		CGRect barRect = CGRectMake(bx, by, BAR_W, bh);
		CGPathRef barPath = CGPathCreateWithRoundedRect(barRect, BAR_R, BAR_R, NULL);
		CGContextAddPath(ctx, barPath);
		CGContextSetRGBFillColor(ctx, g_state.barR, g_state.barG, g_state.barB, 1.0f);
		CGContextFillPath(ctx);
		CGPathRelease(barPath);
	}

	// Draw text
	if (text && textAttrs) {
		float tx = startX + barsW + TXT_GAP;
		NSSize sz = [text sizeWithAttributes:textAttrs];
		float ty = (h - sz.height) / 2;
		[text drawAtPoint:NSMakePoint(tx, ty) withAttributes:textAttrs];
	}
}

@end

// ---- Custom NSWindow for drag tracking ----
@interface VoiceSnapOverlayWindow : NSWindow {
	NSPoint _dragStartOrigin;
}
@end

@implementation VoiceSnapOverlayWindow

- (void)sendEvent:(NSEvent *)event {
	if (event.type == NSEventTypeLeftMouseDown) {
		_dragStartOrigin = self.frame.origin;
	}
	[super sendEvent:event];
	if (event.type == NSEventTypeLeftMouseUp) {
		NSPoint cur = self.frame.origin;
		if (!NSEqualPoints(_dragStartOrigin, cur)) {
			NSScreen* screen = [NSScreen mainScreen];
			if (screen) {
				float screenH = screen.frame.size.height;
				g_state.dragX = (int)cur.x;
				g_state.dragY = (int)(screenH - cur.y - CAP_H);
				g_state.posX = g_state.dragX;
				g_state.posY = g_state.dragY;
				g_state.dragEnded = 1;
			}
		}
	}
}

- (BOOL)canBecomeKeyWindow { return NO; }

@end

// ---- Timer target ----
static NSWindow* g_window = nil;
static VoiceSnapOverlayView* g_view = nil;
static NSTimer* g_timer = nil;

static void animateBars(void) {
	g_state.animTime += 0.033f;
	float t = g_state.animTime;

	switch (g_state.status) {
	case 1: // loading
		for (int i = 0; i < N_BARS; i++) {
			float pulse = sinf(t * 2 + i * 0.3f) * 0.5f + 0.5f;
			g_state.barHeights[i] = 10 + pulse * 20;
		}
		break;
	case 2: // ready
	case 6: // done
		for (int i = 0; i < N_BARS; i++)
			g_state.barHeights[i] = g_state.barHeights[i] * 0.85f + 14 * 0.15f;
		break;
	case 3: // recording
	case 4: { // freetalking
		int hasReal = 0;
		for (int i = 0; i < N_BARS; i++)
			if (g_state.volumes[i] > 0.01f) { hasReal = 1; break; }
		for (int i = 0; i < N_BARS; i++) {
			float target;
			if (hasReal) {
				target = 8 + g_state.volumes[i] * 32;
			} else {
				float wave = sinf(t * 4 + i * 0.8f) * 0.5f + 0.5f;
				float rnd = sinf(t * 7 + i * 1.5f) * 0.3f;
				target = 10 + (wave + rnd) * 25;
			}
			g_state.barHeights[i] = g_state.barHeights[i] * 0.6f + target * 0.4f;
		}
		break;
	}
	case 5: // processing
		for (int i = 0; i < N_BARS; i++) {
			float wave = sinf(t * 3 + i * 0.6f) * 0.5f + 0.5f;
			g_state.barHeights[i] = 12 + wave * 18;
		}
		break;
	default:
		for (int i = 0; i < N_BARS; i++)
			g_state.barHeights[i] = g_state.barHeights[i] * 0.85f + 14 * 0.15f;
		break;
	}
}

@interface OverlayTimerTarget : NSObject
- (void)tick:(NSTimer*)timer;
@end

@implementation OverlayTimerTarget

- (void)tick:(NSTimer*)timer {
	if (!g_window) return;

	// Fade
	if (g_state.fadeProgress < g_state.fadeTarget) {
		g_state.fadeProgress += 0.16f;
		if (g_state.fadeProgress > 1.0f) g_state.fadeProgress = 1.0f;
	} else if (g_state.fadeProgress > g_state.fadeTarget) {
		g_state.fadeProgress -= 0.22f;
		if (g_state.fadeProgress < 0) g_state.fadeProgress = 0;
	}

	float eased = g_state.fadeProgress * (2 - g_state.fadeProgress);
	[g_window setAlphaValue:eased];

	if (g_state.fadeProgress <= 0 && g_state.fadeTarget == 0) {
		[g_window orderOut:nil];
		return;
	}

	if (g_state.fadeTarget > 0 && ![g_window isVisible]) {
		[g_window orderFrontRegardless];
	}

	// Reposition
	if (g_state.needsPosition) {
		g_state.needsPosition = 0;
		NSScreen* screen = [NSScreen mainScreen];
		if (screen) {
			float screenH = screen.frame.size.height;
			float macY = screenH - g_state.posY - CAP_H;
			[g_window setFrameOrigin:NSMakePoint(g_state.posX, macY)];
		}
	}

	animateBars();
	[g_view setNeedsDisplay:YES];
}

@end

static OverlayTimerTarget* g_timerTarget = nil;

// ---- C API for Go ----

static void doCreateOverlay(void* ctx) {
	(void)ctx;

	VoiceSnapOverlayWindow* window = [[VoiceSnapOverlayWindow alloc]
		initWithContentRect:NSMakeRect(0, 0, CAP_W, CAP_H)
		styleMask:NSWindowStyleMaskBorderless
		backing:NSBackingStoreBuffered
		defer:NO];

	[window setOpaque:NO];
	[window setBackgroundColor:[NSColor clearColor]];
	[window setLevel:NSFloatingWindowLevel];
	[window setCollectionBehavior:
		NSWindowCollectionBehaviorCanJoinAllSpaces |
		NSWindowCollectionBehaviorStationary |
		NSWindowCollectionBehaviorFullScreenAuxiliary];
	[window setIgnoresMouseEvents:NO];
	[window setMovableByWindowBackground:YES];
	[window setHasShadow:NO];
	[window setAlphaValue:0];

	VoiceSnapOverlayView* view = [[VoiceSnapOverlayView alloc]
		initWithFrame:NSMakeRect(0, 0, CAP_W, CAP_H)];
	[window setContentView:view];

	g_window = window;
	g_view = view;

	g_timerTarget = [[OverlayTimerTarget alloc] init];
	g_timer = [NSTimer timerWithTimeInterval:0.033
		target:g_timerTarget
		selector:@selector(tick:)
		userInfo:nil
		repeats:YES];
	[[NSRunLoop currentRunLoop] addTimer:g_timer forMode:NSRunLoopCommonModes];
}

static void overlayCreate(void) {
	dispatch_async_f(dispatch_get_main_queue(), NULL, doCreateOverlay);
}

static void doAutoPosition(void* ctx) {
	(void)ctx;
	if (!g_window) return;
	NSScreen* screen = [NSScreen mainScreen];
	if (!screen) return;
	NSRect wa = [screen visibleFrame];
	int cx = (int)(wa.origin.x + (wa.size.width - CAP_W) / 2);
	int cy = (int)(wa.origin.y + 100);
	[g_window setFrameOrigin:NSMakePoint(cx, cy)];
	g_state.hasPosition = 1;
}

static void overlayShow(void) {
	g_state.fadeTarget = 1.0f;
	if (!g_state.hasPosition) {
		dispatch_async_f(dispatch_get_main_queue(), NULL, doAutoPosition);
	}
}

static void overlayHide(void) {
	g_state.fadeTarget = 0.0f;
}

static void overlaySetStatus(int status, const char* text, float r, float g, float b) {
	g_state.status = status;
	g_state.barR = r;
	g_state.barG = g;
	g_state.barB = b;
	if (text) {
		strncpy(g_state.text, text, sizeof(g_state.text) - 1);
		g_state.text[sizeof(g_state.text) - 1] = '\0';
	} else {
		g_state.text[0] = '\0';
	}
}

static void overlaySetVolume(float vol) {
	for (int i = 0; i < N_BARS - 1; i++)
		g_state.volumes[i] = g_state.volumes[i + 1];
	g_state.volumes[N_BARS - 1] = vol;
}

static void overlaySetPosition(int x, int y) {
	g_state.posX = x;
	g_state.posY = y;
	g_state.hasPosition = 1;
	g_state.needsPosition = 1;
}

static void overlayGetPosition(int* x, int* y) {
	*x = g_state.posX;
	*y = g_state.posY;
}

static int overlayCheckDrag(int* x, int* y) {
	if (g_state.dragEnded) {
		g_state.dragEnded = 0;
		*x = g_state.dragX;
		*y = g_state.dragY;
		return 1;
	}
	return 0;
}

static void doCloseOverlay(void* ctx) {
	(void)ctx;
	if (g_timer) {
		[g_timer invalidate];
		g_timer = nil;
	}
	if (g_window) {
		[g_window close];
		g_window = nil;
	}
	g_view = nil;
	g_timerTarget = nil;
}

static void overlayClose(void) {
	dispatch_async_f(dispatch_get_main_queue(), NULL, doCloseOverlay);
}
*/
import "C"

import (
	"time"
	"unsafe"
)

// darwinOverlay uses a native NSWindow + Core Graphics to render
// the floating indicator, matching the Windows GDI+ implementation.
type darwinOverlay struct {
	dragCb  func(int, int)
	created bool
	done    chan struct{}
}

func New() Overlay {
	return &darwinOverlay{
		done: make(chan struct{}),
	}
}

func (o *darwinOverlay) ensureCreated() {
	if !o.created {
		C.overlayCreate()
		o.created = true
		time.Sleep(50 * time.Millisecond)
		go o.pollDrag()
	}
}

func (o *darwinOverlay) Show() {
	o.ensureCreated()
	C.overlayShow()
}

func (o *darwinOverlay) Hide() {
	C.overlayHide()
}

func (o *darwinOverlay) SetStatus(status Status, text string) {
	r, g, b := statusColorRGB(status)
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	C.overlaySetStatus(statusToInt(status), cText, C.float(r), C.float(g), C.float(b))
}

func (o *darwinOverlay) SetVolume(vol float64) {
	C.overlaySetVolume(C.float(vol))
}

func (o *darwinOverlay) SetPosition(x, y int) {
	C.overlaySetPosition(C.int(x), C.int(y))
}

func (o *darwinOverlay) GetPosition() (int, int) {
	var x, y C.int
	C.overlayGetPosition(&x, &y)
	return int(x), int(y)
}

func (o *darwinOverlay) OnDragged(fn func(int, int)) {
	o.dragCb = fn
}

func (o *darwinOverlay) Close() {
	select {
	case <-o.done:
	default:
		close(o.done)
	}
	C.overlayClose()
}

func (o *darwinOverlay) pollDrag() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-o.done:
			return
		case <-ticker.C:
			var x, y C.int
			if C.overlayCheckDrag(&x, &y) != 0 {
				if o.dragCb != nil {
					o.dragCb(int(x), int(y))
				}
			}
		}
	}
}

func statusToInt(s Status) C.int {
	switch s {
	case StatusLoading:
		return 1
	case StatusReady:
		return 2
	case StatusRecording:
		return 3
	case StatusFreetalking:
		return 4
	case StatusProcessing:
		return 5
	case StatusDone:
		return 6
	case StatusCancelled:
		return 7
	case StatusNoVoice:
		return 8
	case StatusNoContent:
		return 9
	case StatusError:
		return 10
	default:
		return 0
	}
}

func statusColorRGB(s Status) (float32, float32, float32) {
	switch s {
	case StatusLoading, StatusFreetalking:
		return 0, 0.478, 1.0 // #007AFF
	case StatusReady, StatusDone:
		return 0.204, 0.780, 0.349 // #34C759
	case StatusRecording, StatusError:
		return 1.0, 0.231, 0.188 // #FF3B30
	case StatusProcessing, StatusCancelled, StatusNoVoice, StatusNoContent:
		return 1.0, 0.584, 0.0 // #FF9500
	default:
		return 0, 0.478, 1.0
	}
}
