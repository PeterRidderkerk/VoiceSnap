package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static inline void _showDockIcon() {
    if ([NSThread isMainThread]) {
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    } else {
        dispatch_async(dispatch_get_main_queue(), ^{
            [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        });
    }
}

static inline void _hideDockIcon() {
    if ([NSThread isMainThread]) {
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    } else {
        dispatch_async(dispatch_get_main_queue(), ^{
            [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        });
    }
}
*/
import "C"

func showDockIcon() { C._showDockIcon() }
func hideDockIcon() { C._hideDockIcon() }
