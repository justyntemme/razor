// SPDX-License-Identifier: MIT
// +build darwin,!ios

#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>

#include "_cgo_export.h"

// Implementation of NSDraggingDestination methods that will be added to GioView at runtime

static NSDragOperation gioView_draggingEntered(id self, SEL _cmd, id<NSDraggingInfo> sender) {
    // Accept copy operations for files
    NSPasteboard *pboard = [sender draggingPasteboard];
    if ([[pboard types] containsObject:NSPasteboardTypeFileURL]) {
        // Notify Go that external drag started
        razor_onExternalDragStart();
        return NSDragOperationCopy;
    }
    return NSDragOperationNone;
}

static NSDragOperation gioView_draggingUpdated(id self, SEL _cmd, id<NSDraggingInfo> sender) {
    NSPasteboard *pboard = [sender draggingPasteboard];
    if ([[pboard types] containsObject:NSPasteboardTypeFileURL]) {
        // Get drag location in view coordinates
        NSPoint location = [sender draggingLocation];
        NSView *view = (NSView *)self;
        // Convert to upper-left origin (Gio uses upper-left, AppKit uses lower-left)
        CGFloat height = view.bounds.size.height;
        razor_onExternalDragUpdate((int)location.x, (int)(height - location.y));
        return NSDragOperationCopy;
    }
    return NSDragOperationNone;
}

static void gioView_draggingExited(id self, SEL _cmd, id<NSDraggingInfo> sender) {
    // Notify Go that external drag ended without drop
    razor_onExternalDragEnd();
}

static BOOL gioView_performDragOperation(id self, SEL _cmd, id<NSDraggingInfo> sender) {
    NSPasteboard *pboard = [sender draggingPasteboard];

    if ([[pboard types] containsObject:NSPasteboardTypeFileURL]) {
        // Read file URLs from pasteboard
        NSArray<NSURL *> *fileURLs = [pboard readObjectsForClasses:@[[NSURL class]]
                                                            options:@{NSPasteboardURLReadingFileURLsOnlyKey: @YES}];

        for (NSURL *fileURL in fileURLs) {
            NSString *filePath = [fileURL path];
            if (filePath != nil) {
                // Call back into Go for each dropped file
                razor_onExternalDrop((char *)[filePath UTF8String]);
            }
        }
        return YES;
    }
    return NO;
}

// Track if we've already set up (to avoid double-setup)
static BOOL razorDropSetupDone = NO;

// Helper to add or replace a method
static void addOrReplaceMethod(Class cls, SEL sel, IMP imp, const char *types) {
    Method existing = class_getInstanceMethod(cls, sel);
    if (existing != NULL) {
        method_setImplementation(existing, imp);
    } else {
        class_addMethod(cls, sel, imp, types);
    }
}

// Setup function to be called from Go once we have the NSView pointer
void razor_setupExternalDrop(uintptr_t viewPtr) {
    if (viewPtr == 0 || razorDropSetupDone) {
        return;
    }

    NSView *view = (__bridge NSView *)(void *)viewPtr;
    Class viewClass = object_getClass(view);

    // Add or replace NSDraggingDestination methods
    // Type encodings: Q = unsigned long long (NSDragOperation), B = BOOL, v = void, @ = object, : = SEL
    addOrReplaceMethod(viewClass, @selector(draggingEntered:), (IMP)gioView_draggingEntered, "Q@:@");
    addOrReplaceMethod(viewClass, @selector(draggingUpdated:), (IMP)gioView_draggingUpdated, "Q@:@");
    addOrReplaceMethod(viewClass, @selector(draggingExited:), (IMP)gioView_draggingExited, "v@:@");
    addOrReplaceMethod(viewClass, @selector(performDragOperation:), (IMP)gioView_performDragOperation, "B@:@");

    // Register the view to accept file drops
    [view registerForDraggedTypes:@[NSPasteboardTypeFileURL]];

    razorDropSetupDone = YES;
}
