//go:build darwin

package speech

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices

#include <stdlib.h>

// Defined in dictation_darwin.m
void ShowMenuBar(void);
void HideMenuBar(void);
void SetMenuBarState(int state);

void SetHotkeyModifier(int keyCode);
void StartHotkeyMonitor(void);
void StopHotkeyMonitor(void);

void TypeTextViaClipboard(const char *text);
int  CheckAccessibilityPermission(int promptUser);
void PlayDictationSound(int soundType);

// Defined in overlay_darwin.m
void PreCreateDictationOverlay(void);   // warm up — call once at startup
void ShowDictationOverlay(int state);   // 1 = recording, 2 = thinking
void HideDictationOverlay(void);
*/
import "C"
import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

// ─── Dictation State Machine ───

// DictationState represents the current state of the dictation pipeline.
type DictationState string

const (
	DictationIdle         DictationState = "idle"
	DictationRecording    DictationState = "recording"
	DictationTranscribing DictationState = "transcribing"
)

// DictationStatusHandler is called when the dictation state changes.
type DictationStatusHandler func(state DictationState, text string)

var (
	dictMu        sync.Mutex
	dictState     DictationState = DictationIdle
	dictTempPath  string
	dictCfgLoader func() (TranscribeConfig, error)
	dictOnStatus  DictationStatusHandler
	dictOnError   func(string)
	dictEnabled   bool
	dictPressTime time.Time // tracks when the modifier key was pressed
)

// ─── Modifier Key Codes ───
// Must match the switch in SetHotkeyModifier() in dictation_darwin.m.
const (
	ModLeftOption  = 0
	ModRightOption = 1
	ModLeftCmd     = 2
	ModRightCmd    = 3
	ModLeftCtrl    = 4
	ModRightCtrl   = 5
)

// ModifierCodeFromString converts a config string to an integer code.
func ModifierCodeFromString(s string) int {
	switch s {
	case "left_option":
		return ModLeftOption
	case "right_option":
		return ModRightOption
	case "left_cmd":
		return ModLeftCmd
	case "right_cmd":
		return ModRightCmd
	case "left_ctrl":
		return ModLeftCtrl
	case "right_ctrl":
		return ModRightCtrl
	default:
		return ModLeftOption
	}
}

// DefaultModifier is the default hotkey modifier string.
const DefaultModifier = "right_option"

// ─── Public API ───

// SetupDictation configures the push-to-talk dictation pipeline.
//
// Parameters:
//   - modifier: human-readable modifier key string (e.g. "left_option"). Use "" for default.
//   - cfgLoader: called each time transcription is needed to get current speech config.
//   - onStatus: called when dictation state changes (may be called from background goroutine).
//   - onError: called when an error occurs (may be called from background goroutine).
func SetupDictation(modifier string, cfgLoader func() (TranscribeConfig, error), onStatus DictationStatusHandler, onError func(string)) {
	if modifier == "" {
		modifier = DefaultModifier
	}
	modCode := ModifierCodeFromString(modifier)

	dictMu.Lock()
	dictCfgLoader = cfgLoader
	dictOnStatus = onStatus
	dictOnError = onError
	dictEnabled = true
	dictMu.Unlock()

	C.SetHotkeyModifier(C.int(modCode))
	C.PreCreateDictationOverlay() // warm up NSPanel so first press is instant
	C.StartHotkeyMonitor()

	log.Printf("[dictation] enabled — hold %q to record, release to transcribe & paste", modifier)
}

// TeardownDictation stops the hotkey monitor.
// The menu bar icon is managed separately (shown/hidden with the app lifecycle).
func TeardownDictation() {
	C.StopHotkeyMonitor()

	dictMu.Lock()
	dictEnabled = false
	dictState = DictationIdle
	dictMu.Unlock()

	log.Println("[dictation] disabled")
}

// ShowMenuBarIcon shows the Talon waveform icon in the macOS menu bar.
// Call this once at app startup so the icon is visible while the app is running.
func ShowMenuBarIcon() {
	C.ShowMenuBar()
}

// HideMenuBarIcon removes the Talon icon from the macOS menu bar.
// Call this at app shutdown.
func HideMenuBarIcon() {
	C.HideMenuBar()
}

// IsDictationEnabled returns whether the dictation hotkey is active.
func IsDictationEnabled() bool {
	dictMu.Lock()
	defer dictMu.Unlock()
	return dictEnabled
}

// GetDictationState returns the current dictation pipeline state.
func GetDictationState() DictationState {
	dictMu.Lock()
	defer dictMu.Unlock()
	return dictState
}

// HasAccessibilityPermission checks whether the app has macOS Accessibility
// permission (required for pasting text into other apps via ⌘V simulation).
func HasAccessibilityPermission(promptUser bool) bool {
	prompt := 0
	if promptUser {
		prompt = 1
	}
	return C.CheckAccessibilityPermission(C.int(prompt)) != 0
}

// SetMenuState updates the menu bar icon to reflect the current dictation state.
// 0 = idle, 1 = recording, 2 = transcribing.
func SetMenuState(state int) {
	C.SetMenuBarState(C.int(state))
}

// ─── Hotkey Callbacks (called from ObjC via CGO) ───

//export goDictationPressed
func goDictationPressed() {
	dictMu.Lock()
	if !dictEnabled {
		dictMu.Unlock()
		return
	}
	// If already recording or transcribing, ignore.
	if dictState != DictationIdle {
		dictMu.Unlock()
		return
	}
	dictPressTime = time.Now()
	dictMu.Unlock()

	go startDictation()
}

//export goDictationReleased
func goDictationReleased() {
	dictMu.Lock()
	if !dictEnabled {
		dictMu.Unlock()
		return
	}
	dur := time.Since(dictPressTime)
	state := dictState
	dictMu.Unlock()

	// Ignore very short taps (< 300ms) — likely an accidental modifier press.
	if dur < 300*time.Millisecond {
		if state == DictationRecording {
			// Cancel the recording silently.
			go cancelDictation()
		}
		return
	}

	if state == DictationRecording {
		go stopDictationAndType()
	}
}

// ─── Internal State Machine ───

func startDictation() {
	dictMu.Lock()
	dictState = DictationRecording
	onStatus := dictOnStatus
	dictMu.Unlock()

	// Start recording FIRST — this is the latency-critical path.
	// The recording dispatch_async block should land on the main queue
	// before the overlay/sound blocks so it runs with minimal delay.
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("talon-dictate-%d.m4a", time.Now().UnixMilli()))
	dictMu.Lock()
	dictTempPath = tmpFile
	dictMu.Unlock()

	log.Printf("[dictation] recording started → %s", tmpFile)

	StartRecording(tmpFile, func(errMsg string) {
		log.Printf("[dictation] recording error: %s", errMsg)
		C.PlayDictationSound(3)
		C.SetMenuBarState(0)

		dictMu.Lock()
		dictState = DictationIdle
		onErr := dictOnError
		onSt := dictOnStatus
		dictMu.Unlock()

		if onErr != nil {
			onErr(errMsg)
		}
		if onSt != nil {
			onSt(DictationIdle, "")
		}
	})

	// UI feedback comes after — the microphone is already capturing audio.
	C.SetMenuBarState(1)          // recording menu bar icon
	C.PlayDictationSound(0)       // "Pop" — start
	// No overlay for recording — the red menu bar dot is sufficient.

	if onStatus != nil {
		onStatus(DictationRecording, "")
	}
}

func cancelDictation() {
	StopRecording()
	C.SetMenuBarState(0)

	dictMu.Lock()
	path := dictTempPath
	dictTempPath = ""
	dictState = DictationIdle
	dictMu.Unlock()

	if path != "" {
		os.Remove(path)
	}
	log.Println("[dictation] short press — recording cancelled")
}

func stopDictationAndType() {
	dictMu.Lock()
	dictState = DictationTranscribing
	path := dictTempPath
	dictTempPath = ""
	cfgLoader := dictCfgLoader
	onStatus := dictOnStatus
	onError := dictOnError
	dictMu.Unlock()

	C.PlayDictationSound(1)       // "Tink" — stop
	C.ShowDictationOverlay(2)     // switch overlay to "Thinking…"
	C.SetMenuBarState(2)          // transcribing menu bar icon

	if onStatus != nil {
		onStatus(DictationTranscribing, "")
	}

	log.Println("[dictation] stopping recording, starting transcription...")

	StopRecording()
	time.Sleep(150 * time.Millisecond)

	// Helper to go back to idle on error.
	fail := func(msg string) {
		log.Printf("[dictation] error: %s", msg)
		C.PlayDictationSound(3)
		C.HideDictationOverlay()
		C.SetMenuBarState(0)
		dictMu.Lock()
		dictState = DictationIdle
		dictMu.Unlock()
		if onError != nil {
			onError(msg)
		}
		if onStatus != nil {
			onStatus(DictationIdle, "")
		}
	}

	if path == "" {
		fail("No recording in progress")
		return
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		fail(fmt.Sprintf("Failed to read recording: %v", err))
		return
	}
	if len(data) == 0 {
		fail("Recording was empty — try speaking louder or closer to the microphone")
		return
	}

	log.Printf("[dictation] recorded %d bytes, transcribing...", len(data))

	cfg, err := cfgLoader()
	if err != nil {
		fail(fmt.Sprintf("Speech config error: %v", err))
		return
	}

	audioBase64 := base64.StdEncoding.EncodeToString(data)
	result, err := Transcribe(cfg, audioBase64, "audio/m4a")
	if err != nil {
		fail(fmt.Sprintf("Transcription failed: %v", err))
		return
	}

	if result.Text == "" {
		fail("No speech detected — try speaking louder")
		return
	}

	log.Printf("[dictation] transcribed %d chars, pasting into focused app...", len(result.Text))

	// Check accessibility permission before attempting to paste.
	if !HasAccessibilityPermission(false) {
		log.Println("[dictation] accessibility permission not granted — prompting user")
		HasAccessibilityPermission(true) // show macOS dialog
		C.PlayDictationSound(3)
		C.HideDictationOverlay()
		C.SetMenuBarState(0)
		dictMu.Lock()
		dictState = DictationIdle
		dictMu.Unlock()
		if onError != nil {
			onError("Grant Accessibility permission in System Settings → Privacy & Security → Accessibility, then try again")
		}
		if onStatus != nil {
			onStatus(DictationIdle, result.Text)
		}
		return
	}

	// Paste the transcribed text into the currently focused text field.
	C.HideDictationOverlay()
	cText := C.CString(result.Text)
	defer C.free(unsafe.Pointer(cText))
	C.TypeTextViaClipboard(cText)

	C.PlayDictationSound(2) // "Glass" — success
	C.SetMenuBarState(0)    // back to idle

	log.Println("[dictation] text pasted successfully")

	dictMu.Lock()
	dictState = DictationIdle
	dictMu.Unlock()

	if onStatus != nil {
		onStatus(DictationIdle, result.Text)
	}
}
