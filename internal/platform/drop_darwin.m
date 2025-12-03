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
        return NSDragOperationCopy;
    }
    return NSDragOperationNone;
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

// Setup function to be called from Go once we have the NSView pointer
void razor_setupExternalDrop(uintptr_t viewPtr) {
    if (viewPtr == 0 || razorDropSetupDone) {
        return;
    }

    NSView *view = (__bridge NSView *)(void *)viewPtr;
    Class viewClass = object_getClass(view);

    // Try to add or replace methods
    // NSView has default NSDraggingDestination stubs, so we need to replace them
    Method existingEntered = class_getInstanceMethod(viewClass, @selector(draggingEntered:));
    Method existingPerform = class_getInstanceMethod(viewClass, @selector(performDragOperation:));

    if (existingEntered != NULL) {
        method_setImplementation(existingEntered, (IMP)gioView_draggingEntered);
    } else {
        class_addMethod(viewClass, @selector(draggingEntered:), (IMP)gioView_draggingEntered, "Q@:@");
    }

    if (existingPerform != NULL) {
        method_setImplementation(existingPerform, (IMP)gioView_performDragOperation);
    } else {
        class_addMethod(viewClass, @selector(performDragOperation:), (IMP)gioView_performDragOperation, "B@:@");
    }

    // Register the view to accept file drops
    [view registerForDraggedTypes:@[NSPasteboardTypeFileURL]];

    razorDropSetupDone = YES;
}
