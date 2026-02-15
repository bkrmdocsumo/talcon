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

static NSStatusItem *statusItem = nil;
static NSImage *iconIdle         = nil;

static NSImage* createIdleIcon(void) {
    if (@available(macOS 11.0, *)) {
        NSImage *img = [NSImage imageWithSystemSymbolName:@"waveform"
                                 accessibilityDescription:@"Talon"];
        if (img) {
            [img setTemplate:YES];
            return img;
        }
    }
    // Fallback: draw a small waveform (5 bars) for older macOS.
    NSImage *img = [[NSImage alloc] initWithSize:NSMakeSize(18, 18)];
    [img lockFocus];
    [[NSColor labelColor] setFill];
    CGFloat barW = 2, gap = 1.5;
    CGFloat heights[] = {5, 9, 13, 9, 5};
    CGFloat totalW = 5 * barW + 4 * gap;
    CGFloat startX = (18 - totalW) / 2.0;
    for (int i = 0; i < 5; i++) {
        CGFloat h = heights[i];
        CGFloat x = startX + i * (barW + gap);
        CGFloat y = (18 - h) / 2.0;
        [[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(x, y, barW, h)
                                         xRadius:1 yRadius:1] fill];
    }
    [img unlockFocus];
    [img setTemplate:YES];
    return img;
}

void ShowMenuBar(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem) return;

        iconIdle = createIdleIcon();

        statusItem = [[NSStatusBar systemStatusBar]
            statusItemWithLength:NSVariableStatusItemLength];
        statusItem.button.image   = iconIdle;
        statusItem.button.toolTip = @"Talon — Voice Input";
    });
}

void HideMenuBar(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem) {
            [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
            statusItem = nil;
        }
    });
}

// state: 0 = idle, 1 = recording, 2 = transcribing
// Icon stays the same waveform — macOS shows its own orange mic indicator
// automatically when the microphone is in use.  We only update the tooltip.
void SetMenuBarState(int state) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!statusItem) return;
        switch (state) {
            case 1:
                statusItem.button.toolTip = @"Talon — Recording…";
                break;
            case 2:
                statusItem.button.toolTip = @"Talon — Transcribing…";
                break;
            default:
                statusItem.button.toolTip = @"Talon — Voice Input";
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
        NSSound *sound = [NSSound soundNamed:soundName];
        if (sound) {
            [sound play];
        }
    });
}
