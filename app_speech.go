package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/speech"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startDictation sets up the global push-to-talk hotkey and pipeline.
func (a *App) startDictation(cfg *config.Config) {
	// Tear down any existing registration first.
	if speech.IsDictationEnabled() {
		speech.TeardownDictation()
	}

	modifier := cfg.HotkeyModifier
	if modifier == "" {
		modifier = speech.DefaultModifier
	}

	speech.SetupDictation(
		modifier,
		a.loadSpeechConfig,
		func(state speech.DictationState, text string) {
			// Track recording start time for duration calculation.
			if state == speech.DictationRecording {
				a.dictStartTime = time.Now()
			}

			// When dictation completes with text, auto-save to flow history.
			if state == speech.DictationIdle && text != "" {
				duration := int(time.Since(a.dictStartTime).Seconds())
				if duration < 1 {
					duration = 1
				}
				go func() {
					id, err := a.SaveFlowTranscript(text, duration)
					if err != nil {
						log.Printf("[dictation] failed to save flow transcript: %v", err)
					} else {
						log.Printf("[dictation] auto-saved flow transcript %s", id)
						// Notify frontend to refresh flow history.
						wailsRuntime.EventsEmit(a.ctx, "flow:saved", map[string]interface{}{
							"id": id,
						})
					}
				}()
			}

			wailsRuntime.EventsEmit(a.ctx, "dictation:status", map[string]interface{}{
				"state": string(state),
				"text":  text,
			})
		},
		func(errMsg string) {
			log.Printf("[dictation] error: %s", errMsg)
			wailsRuntime.EventsEmit(a.ctx, "dictation:error", map[string]interface{}{
				"error": errMsg,
			})
		},
	)
}

// GetDictationStatus returns the current dictation status for the frontend.
func (a *App) GetDictationStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":       speech.IsDictationEnabled(),
		"state":         string(speech.GetDictationState()),
		"accessibility": speech.HasAccessibilityPermission(false),
	}
}

// RequestAccessibility prompts macOS to show the Accessibility permission dialog.
// Returns true if the app already has permission.
func (a *App) RequestAccessibility() bool {
	return speech.HasAccessibilityPermission(true)
}

// TranscribeAudio takes base64-encoded audio data and transcribes it using
// the configured speech-to-text API (OpenAI Whisper or Deepgram).
// mimeType should be the audio format, e.g. "audio/webm", "audio/wav".
func (a *App) TranscribeAudio(audioBase64 string, mimeType string) (string, error) {
	cfg, err := a.loadSpeechConfig()
	if err != nil {
		return "", err
	}

	log.Printf("[voice] transcribing audio (%s, provider=%s, %d bytes base64)",
		mimeType, cfg.Provider, len(audioBase64))

	result, err := speech.Transcribe(cfg, audioBase64, mimeType)
	if err != nil {
		return "", err
	}

	log.Printf("[voice] transcription complete: %d chars", len(result.Text))
	return result.Text, nil
}

// StartVoiceRecording begins capturing audio from the native macOS microphone.
// The audio is saved to a temporary m4a file. Permission dialogs are handled
// by macOS automatically on first use.
func (a *App) StartVoiceRecording() {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("talon-voice-%d.m4a", time.Now().UnixMilli()))

	a.voiceMu.Lock()
	a.voiceRecordingPath = tmpFile
	a.voiceMu.Unlock()

	log.Printf("[voice] starting native recording → %s", tmpFile)

	speech.StartRecording(tmpFile, func(errMsg string) {
		log.Printf("[voice] recording error: %s", errMsg)
		wailsRuntime.EventsEmit(a.ctx, "voice:error", map[string]interface{}{
			"error": errMsg,
		})
	})
}

// StopVoiceAndTranscribe stops the current recording, sends the audio file
// to the configured speech-to-text API, and returns the transcribed text.
func (a *App) StopVoiceAndTranscribe() (string, error) {
	log.Println("[voice] stopping recording and transcribing...")

	speech.StopRecording()

	// Brief pause to let the file be fully flushed.
	time.Sleep(150 * time.Millisecond)

	a.voiceMu.Lock()
	path := a.voiceRecordingPath
	a.voiceRecordingPath = ""
	a.voiceMu.Unlock()

	if path == "" {
		return "", fmt.Errorf("no recording in progress")
	}

	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read recording file: %w", err)
	}

	if len(data) == 0 {
		return "", fmt.Errorf("recorded file is empty — try speaking louder or closer to the microphone")
	}

	log.Printf("[voice] recorded %d bytes, sending to API...", len(data))

	// Build speech config and transcribe.
	cfg, err := a.loadSpeechConfig()
	if err != nil {
		return "", err
	}

	audioBase64 := base64.StdEncoding.EncodeToString(data)
	result, err := speech.Transcribe(cfg, audioBase64, "audio/m4a")
	if err != nil {
		return "", err
	}

	log.Printf("[voice] transcription complete: %d chars", len(result.Text))
	return result.Text, nil
}

// IsVoiceRecording returns whether a native voice recording is currently active.
func (a *App) IsVoiceRecording() bool {
	return speech.IsRecording()
}
