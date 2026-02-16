// +build darwin

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

// Go-exported callbacks (defined in dictation_darwin.go).
extern void goDictationPressed(void);
extern void goDictationReleased(void);

// ═══════════════════════════════════════════════════════════════════════
// Menu Bar Status Item (NSStatusItem)
//
// Shows a mic icon in the macOS menu bar when the app is running.
// Changes appearance to reflect recording / transcribing state.
// ═══════════════════════════════════════════════════════════════════════

static NSStatusItem *statusItem  = nil;
static NSImage      *iconIdle   = nil;
static NSMenu       *statusMenu = nil;
static NSMenuItem   *hotkeyMenuItem  = nil;
static NSMenuItem   *statusMenuItem  = nil;

// ─── Menu Action Handler ───
@interface TalonMenuHandler : NSObject
- (void)quitApp:(id)sender;
- (void)showApp:(id)sender;
@end

@implementation TalonMenuHandler
- (void)quitApp:(id)sender {
    [[NSApplication sharedApplication] terminate:nil];
}
- (void)showApp:(id)sender {
    [NSApp activateIgnoringOtherApps:YES];
    // Bring the main window to front.
    for (NSWindow *w in [NSApp windows]) {
        if ([w isKindOfClass:[NSWindow class]] && w.isVisible) {
            [w makeKeyAndOrderFront:nil];
            break;
        }
    }
}
@end

static TalonMenuHandler *menuHandler = nil;

// Create a monochrome pinwheel/turbine icon for the macOS menu bar,
// matching the Talon app icon shape.  Drawn as a template image so
// macOS automatically renders it in white on dark menu bars and black
// on light menu bars — matching the native look of other status icons.
static NSImage* createMenuBarIcon(void) {
    CGFloat size = 18.0;
    NSImage *icon = [[NSImage alloc] initWithSize:NSMakeSize(size, size)];
    [icon lockFocus];

    CGContextRef cg = [[NSGraphicsContext currentContext] CGContext];
    [[NSColor blackColor] setFill];

    CGFloat cx = size / 2.0;
    CGFloat cy = size / 2.0;
    int blades = 6;

    for (int i = 0; i < blades; i++) {
        CGFloat angle = (2.0 * M_PI * i) / blades;

        CGContextSaveGState(cg);
        CGContextTranslateCTM(cg, cx, cy);
        CGContextRotateCTM(cg, angle);

        // One blade pointing up (+Y), curving clockwise.
        // Extra bold to match the visual weight of other
        // macOS menu-bar icons.
        NSBezierPath *b = [NSBezierPath bezierPath];
        [b moveToPoint:NSMakePoint(-1.4, 1.2)];
        [b curveToPoint:NSMakePoint(3.4, 7.8)
              controlPoint1:NSMakePoint(-0.6, 4.5)
              controlPoint2:NSMakePoint(1.4, 6.8)];
        [b curveToPoint:NSMakePoint(1.4, 1.2)
              controlPoint1:NSMakePoint(2.2, 5.6)
              controlPoint2:NSMakePoint(1.6, 3.2)];
        [b closePath];
        [b fill];

        CGContextRestoreGState(cg);
    }

    [icon unlockFocus];
    [icon setTemplate:YES];  // Let macOS handle light/dark appearance.
    return icon;
}

void ShowMenuBar(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem) return;

        iconIdle = createMenuBarIcon();

        if (!menuHandler) {
            menuHandler = [[TalonMenuHandler alloc] init];
        }

        statusItem = [[NSStatusBar systemStatusBar]
            statusItemWithLength:NSVariableStatusItemLength];
        statusItem.button.image   = iconIdle;
        statusItem.button.toolTip = @"Talon — Voice Input";

        // ─── Build menu ───
        statusMenu = [[NSMenu alloc] init];

        // Status line (idle by default) — with a small app icon.
        statusMenuItem = [[NSMenuItem alloc]
            initWithTitle:@"Talon — Voice Input"
            action:nil keyEquivalent:@""];
        [statusMenuItem setEnabled:NO];
        {
            NSImage *menuIcon = createMenuBarIcon();
            if (menuIcon) {
                [menuIcon setSize:NSMakeSize(16, 16)];
                [statusMenuItem setImage:menuIcon];
            }
        }
        [statusMenu addItem:statusMenuItem];

        [statusMenu addItem:[NSMenuItem separatorItem]];

        // Hotkey info line (disabled — informational only).
        hotkeyMenuItem = [[NSMenuItem alloc]
            initWithTitle:@"Hotkey: not configured"
            action:nil keyEquivalent:@""];
        [hotkeyMenuItem setEnabled:NO];
        [statusMenu addItem:hotkeyMenuItem];

        [statusMenu addItem:[NSMenuItem separatorItem]];

        // Show Talon window.
        NSMenuItem *showItem = [[NSMenuItem alloc]
            initWithTitle:@"Show Talon"
            action:@selector(showApp:) keyEquivalent:@""];
        [showItem setTarget:menuHandler];
        [statusMenu addItem:showItem];

        [statusMenu addItem:[NSMenuItem separatorItem]];

        // Quit.
        NSMenuItem *quitItem = [[NSMenuItem alloc]
            initWithTitle:@"Quit Talon"
            action:@selector(quitApp:) keyEquivalent:@"q"];
        [quitItem setTarget:menuHandler];
        [statusMenu addItem:quitItem];

        statusItem.menu = statusMenu;
    });
}

void HideMenuBar(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem) {
            [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
            statusItem = nil;
            statusMenu = nil;
            hotkeyMenuItem = nil;
            statusMenuItem = nil;
        }
    });
}

// Update the hotkey label shown in the menu.
// label: e.g. "Hold ⌥ Right Option and speak"
void SetMenuBarHotkeyLabel(const char *label) {
    NSString *str = [NSString stringWithUTF8String:label];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (hotkeyMenuItem) {
            [hotkeyMenuItem setTitle:str];
        }
    });
}

// state: 0 = idle, 1 = recording, 2 = transcribing
// Icon stays the same waveform — macOS shows its own orange mic indicator
// automatically when the microphone is in use.  We update the tooltip
// and the status menu item text.
void SetMenuBarState(int state) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!statusItem) return;
        switch (state) {
            case 1:
                statusItem.button.toolTip = @"Talon — Recording…";
                if (statusMenuItem) [statusMenuItem setTitle:@"Recording…"];
                break;
            case 2:
                statusItem.button.toolTip = @"Talon — Transcribing…";
                if (statusMenuItem) [statusMenuItem setTitle:@"Transcribing…"];
                break;
            default:
                statusItem.button.toolTip = @"Talon — Voice Input";
                if (statusMenuItem) [statusMenuItem setTitle:@"Talon — Voice Input"];
                break;
        }
    });
}

// ═══════════════════════════════════════════════════════════════════════
// Global Modifier-Key Push-to-Talk Monitor
//
// Uses NSEvent global/local monitors to detect when a specific modifier
// key (e.g. Left Option) is pressed and released. Fires Go callbacks
// on press and release.
//
// Does NOT require Accessibility permission — modifier flag monitoring
// is allowed for all apps.
// ═══════════════════════════════════════════════════════════════════════

// Device-specific modifier masks (lower bits of CGEventFlags).
#define DEV_LCTRL   0x00000001
#define DEV_LSHIFT  0x00000002
#define DEV_RSHIFT  0x00000004
#define DEV_LCMD    0x00000008
#define DEV_RCMD    0x00000010
#define DEV_LALT    0x00000020
#define DEV_RALT    0x00000040
#define DEV_RCTRL   0x00002000

static id  globalMonitor = nil;
static id  localMonitor  = nil;
static uint64_t hotkeyMask = DEV_LALT;  // default: Left Option
static BOOL isHotkeyDown  = NO;

static void handleModifierEvent(NSEvent *event) {
    CGEventRef cgEvt = event.CGEvent;
    if (!cgEvt) return;

    CGEventFlags flags = CGEventGetFlags(cgEvt);
    BOOL keyIsDown = (flags & hotkeyMask) != 0;

    if (keyIsDown && !isHotkeyDown) {
        isHotkeyDown = YES;
        goDictationPressed();
    } else if (!keyIsDown && isHotkeyDown) {
        isHotkeyDown = NO;
        goDictationReleased();
    }
}

// keyCode: 0=LOption 1=ROption 2=LCmd 3=RCmd 4=LCtrl 5=RCtrl
void SetHotkeyModifier(int keyCode) {
    switch (keyCode) {
        case 0: hotkeyMask = DEV_LALT;  break;
        case 1: hotkeyMask = DEV_RALT;  break;
        case 2: hotkeyMask = DEV_LCMD;  break;
        case 3: hotkeyMask = DEV_RCMD;  break;
        case 4: hotkeyMask = DEV_LCTRL; break;
        case 5: hotkeyMask = DEV_RCTRL; break;
        default: hotkeyMask = DEV_LALT; break;
    }
}

void StartHotkeyMonitor(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (globalMonitor) return; // already running

        // Monitor when OTHER apps are focused.
        globalMonitor = [NSEvent
            addGlobalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
            handler:^(NSEvent *event) {
                handleModifierEvent(event);
            }];

        // Monitor when THIS app is focused.
        localMonitor = [NSEvent
            addLocalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
            handler:^NSEvent *(NSEvent *event) {
                handleModifierEvent(event);
                return event;
            }];

        NSLog(@"[dictation] hotkey monitor started");
    });
}

void StopHotkeyMonitor(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (globalMonitor) {
            [NSEvent removeMonitor:globalMonitor];
            globalMonitor = nil;
        }
        if (localMonitor) {
            [NSEvent removeMonitor:localMonitor];
            localMonitor = nil;
        }
        isHotkeyDown = NO;
        NSLog(@"[dictation] hotkey monitor stopped");
    });
}

// ═══════════════════════════════════════════════════════════════════════
// Text Injection — paste transcribed text into the focused application.
//
// Strategy: save clipboard → set text → simulate Cmd+V → restore clipboard.
// Requires Accessibility permission for CGEventPost.
// ═══════════════════════════════════════════════════════════════════════

void TypeTextViaClipboard(const char *text) {
    NSString *str = [NSString stringWithUTF8String:text];
    NSPasteboard *pb = [NSPasteboard generalPasteboard];

    // Save the current clipboard text so we can restore it after pasting.
    NSString *savedText = [[pb stringForType:NSPasteboardTypeString] copy];

    // Place our transcribed text on the clipboard.
    [pb clearContents];
    [pb setString:str forType:NSPasteboardTypeString];

    // Brief pause to ensure the pasteboard update is visible to other apps.
    usleep(50000); // 50 ms

    // Simulate Cmd+V (paste) via CGEvent.
    // Virtual key code 9 = 'V' on macOS.
    CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);

    CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)9, true);
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);

    CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)9, false);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    CGEventPost(kCGHIDEventTap, keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);

    CFRelease(keyDown);
    CFRelease(keyUp);
    if (source) CFRelease(source);

    // Restore the original clipboard content after a delay.
    if (savedText) {
        NSString *textToRestore = savedText;
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1.0 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            NSPasteboard *rpb = [NSPasteboard generalPasteboard];
            [rpb clearContents];
            [rpb setString:textToRestore forType:NSPasteboardTypeString];
        });
    }
}

// ═══════════════════════════════════════════════════════════════════════
// Focus Management — save and restore the frontmost application.
//
// During the grammar-fix flow the Talon overlay / main window may steal
// focus while the LLM call is in flight. These helpers let us remember
// which app the user was actually working in and bring it back before we
// try to paste / replace text.
// ═══════════════════════════════════════════════════════════════════════

static NSRunningApplication *savedFrontApp = nil;
static pid_t savedFrontPid = 0;

void SaveFocusedApp(void) {
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (app) {
        savedFrontApp = app;
        savedFrontPid = app.processIdentifier;
    }
}

void RestoreFocusedApp(void) {
    if (savedFrontApp) {
        [savedFrontApp activateWithOptions:NSApplicationActivateIgnoringOtherApps];
        // Give the app a moment to come to front.
        usleep(150000); // 150 ms
        savedFrontApp = nil;
        savedFrontPid = 0;
    }
}

// ═══════════════════════════════════════════════════════════════════════
// Text Extraction — read the currently selected text from the focused
// application.
//
// Primary strategy: use the Accessibility API (kAXSelectedTextAttribute)
// to read the selected text directly — no clipboard manipulation needed
// and no interference from physically-held modifier keys.
//
// Fallback: simulate Cmd+C using a *private* event source (so held
// modifier keys don't leak into the synthesised keystroke) and read
// from the clipboard.
//
// Returns a strdup'd C string the caller must free(), or NULL if nothing
// was selected.
// Requires Accessibility permission.
// ═══════════════════════════════════════════════════════════════════════

// Try reading selected text via the Accessibility API.
static NSString* getSelectedTextViaAX(void) {
    if (!AXIsProcessTrusted()) return nil;

    AXUIElementRef sysWide = AXUIElementCreateSystemWide();
    if (!sysWide) return nil;

    AXUIElementRef focusedApp = NULL;
    AXError err = AXUIElementCopyAttributeValue(sysWide, kAXFocusedApplicationAttribute,
                                                (CFTypeRef *)&focusedApp);
    if (err != kAXErrorSuccess || !focusedApp) {
        CFRelease(sysWide);
        return nil;
    }

    AXUIElementRef focusedEl = NULL;
    err = AXUIElementCopyAttributeValue(focusedApp, kAXFocusedUIElementAttribute,
                                        (CFTypeRef *)&focusedEl);
    if (err != kAXErrorSuccess || !focusedEl) {
        CFRelease(focusedApp);
        CFRelease(sysWide);
        return nil;
    }

    CFTypeRef selectedTextRef = NULL;
    err = AXUIElementCopyAttributeValue(focusedEl, kAXSelectedTextAttribute, &selectedTextRef);

    CFRelease(focusedEl);
    CFRelease(focusedApp);
    CFRelease(sysWide);

    if (err != kAXErrorSuccess || !selectedTextRef) return nil;

    NSString *text = (__bridge_transfer NSString *)selectedTextRef;
    return (text && text.length > 0) ? text : nil;
}

// Fallback: simulate Cmd+C with a private event source and read the clipboard.
static NSString* getSelectedTextViaCmdC(void) {
    NSPasteboard *pb = [NSPasteboard generalPasteboard];

    // Save the current clipboard text so we can restore it afterwards.
    NSString *savedText = [[pb stringForType:NSPasteboardTypeString] copy];

    // Clear the clipboard so we can detect whether Cmd+C actually copied anything.
    [pb clearContents];
    usleep(50000); // 50 ms

    // Use kCGEventSourceStatePrivate so the synthesised keystroke does NOT
    // inherit the physical modifier-key state (the hotkey modifier may still
    // be held down at this point).
    CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStatePrivate);

    CGEventRef keyDown = CGEventCreateKeyboardEvent(source, (CGKeyCode)8, true);  // 8 = 'C'
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);

    CGEventRef keyUp = CGEventCreateKeyboardEvent(source, (CGKeyCode)8, false);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    CGEventPost(kCGHIDEventTap, keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);

    CFRelease(keyDown);
    CFRelease(keyUp);
    if (source) CFRelease(source);

    // Wait for the target app to place content on the clipboard.
    usleep(200000); // 200 ms

    // Read the clipboard.
    NSString *selectedText = [pb stringForType:NSPasteboardTypeString];

    // Restore the original clipboard content.
    [pb clearContents];
    if (savedText) {
        [pb setString:savedText forType:NSPasteboardTypeString];
    }

    return (selectedText && selectedText.length > 0) ? selectedText : nil;
}

char* CopySelectedText(void) {
    // Primary: Accessibility API — fast, reliable, no clipboard side-effects.
    NSString *text = getSelectedTextViaAX();

    // Fallback: Cmd+C simulation with private event source.
    if (!text) {
        text = getSelectedTextViaCmdC();
    }

    if (text && text.length > 0) {
        return strdup([text UTF8String]); // caller must free()
    }
    return NULL;
}

// ═══════════════════════════════════════════════════════════════════════
// Text Replacement — replace the currently selected text in the focused
// application with new text.
//
// Primary strategy: use the Accessibility API (kAXSelectedTextAttribute)
// to write directly — avoids clipboard manipulation and focus issues.
//
// Fallback: use TypeTextViaClipboard (Cmd+V simulation).
//
// Returns 1 on success, 0 on failure.
// ═══════════════════════════════════════════════════════════════════════

int ReplaceSelectedText(const char *newText) {
    NSString *replacement = [NSString stringWithUTF8String:newText];

    // Try writing via the Accessibility API first.
    if (AXIsProcessTrusted()) {
        AXUIElementRef sysWide = AXUIElementCreateSystemWide();
        if (sysWide) {
            AXUIElementRef focusedApp = NULL;
            AXError err = AXUIElementCopyAttributeValue(sysWide, kAXFocusedApplicationAttribute,
                                                        (CFTypeRef *)&focusedApp);
            if (err == kAXErrorSuccess && focusedApp) {
                AXUIElementRef focusedEl = NULL;
                err = AXUIElementCopyAttributeValue(focusedApp, kAXFocusedUIElementAttribute,
                                                    (CFTypeRef *)&focusedEl);
                if (err == kAXErrorSuccess && focusedEl) {
                    // Try to set the selected text directly.
                    err = AXUIElementSetAttributeValue(focusedEl, kAXSelectedTextAttribute,
                                                      (__bridge CFTypeRef)replacement);
                    CFRelease(focusedEl);
                    CFRelease(focusedApp);
                    CFRelease(sysWide);

                    if (err == kAXErrorSuccess) {
                        return 1; // success via AX API
                    }
                    // Fall through to clipboard fallback.
                } else {
                    CFRelease(focusedApp);
                    CFRelease(sysWide);
                }
            } else {
                CFRelease(sysWide);
            }
        }
    }

    // Fallback: use clipboard paste.
    TypeTextViaClipboard(newText);
    return 1;
}

// ═══════════════════════════════════════════════════════════════════════
// Accessibility Permission Check
//
// CGEventPost (used for pasting) requires the app to be trusted for
// Accessibility. This function checks the status and optionally prompts
// the user with macOS's permission dialog.
// ═══════════════════════════════════════════════════════════════════════

int CheckAccessibilityPermission(int promptUser) {
    NSDictionary *options = @{
        (__bridge NSString *)kAXTrustedCheckOptionPrompt: @(promptUser ? YES : NO)
    };
    return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options) ? 1 : 0;
}

// ═══════════════════════════════════════════════════════════════════════
// Audio Feedback — play system sounds for dictation state changes.
//
// 0 = start recording (Pop)
// 1 = stop recording  (Tink)
// 2 = done / success  (Glass)
// 3 = error           (Basso)
// ═══════════════════════════════════════════════════════════════════════

static BOOL audioSystemWarmedUp = NO;

void WarmUpAudioSystem(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (audioSystemWarmedUp) return;
        @try {
            // Trigger CoreAudio HAL initialization by loading (but not playing)
            // a system sound. This ensures the audio subsystem is ready before
            // the first hotkey press, avoiding a cold-start crash if Info.plist
            // keys are missing or the audio daemon is slow to respond.
            NSSound *warmup = [NSSound soundNamed:@"Pop"];
            (void)warmup;
            audioSystemWarmedUp = YES;
            NSLog(@"[dictation] audio system warmed up");
        } @catch (NSException *e) {
            NSLog(@"[dictation] audio warm-up failed: %@", e);
        }
    });
}

void PlayDictationSound(int soundType) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSString *soundName;
        switch (soundType) {
            case 0: soundName = @"Pop";   break;
            case 1: soundName = @"Tink";  break;
            case 2: soundName = @"Glass"; break;
            case 3: soundName = @"Basso"; break;
            default: return;
        }
        @try {
            NSSound *sound = [NSSound soundNamed:soundName];
            if (sound) {
                [sound play];
            }
        } @catch (NSException *e) {
            NSLog(@"[dictation] PlayDictationSound failed: %@", e);
        }
    });
}
