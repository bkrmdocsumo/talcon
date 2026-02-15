package main

import (
	"fmt"
	"log"
	"os"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/llm"
	"github.com/user/talon/internal/speech"
)

// SettingsPayload is the structure exposed to the frontend for reading/writing settings.
type SettingsPayload struct {
	AnthropicKey   string `json:"anthropic_key"`
	TelegramToken  string `json:"telegram_token"`
	SpeechProvider string `json:"speech_provider"` // "whisper" or "deepgram"
	SpeechModel    string `json:"speech_model"`    // "gpt-4o-mini-transcribe", "gpt-4o-transcribe", "whisper-1"
	OpenAIKey      string `json:"openai_key"`
	GeminiKey      string `json:"gemini_key"`
	DeepgramKey    string `json:"deepgram_key"`
	HotkeyEnabled  bool   `json:"hotkey_enabled"`
	HotkeyModifier string `json:"hotkey_modifier"`
}

// ChangeModel switches the active LLM to a different model. The provider
// (Anthropic or OpenAI) is detected automatically from the model name.
// Called from the frontend when the user picks a model from the dropdown.
func (a *App) ChangeModel(modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model ID is empty")
	}

	// Load config to get API keys.
	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	anthropicKey := cfg.AnthropicKey
	if anthropicKey == "" {
		anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	openaiKey := cfg.OpenAIKey
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}
	geminiKey := cfg.GeminiKey
	if geminiKey == "" {
		geminiKey = os.Getenv("GEMINI_API_KEY")
	}

	client, err := llm.NewClientForModel(modelID, anthropicKey, openaiKey, geminiKey)
	if err != nil {
		return err
	}

	a.deps.LLMClient = client
	a.agentCfg.Model = modelID
	log.Printf("Model changed to %s (provider=%s)", modelID, llm.DetectProvider(modelID))
	return nil
}

// GetSettings returns the current API key and Telegram token for the settings UI.
func (a *App) GetSettings() (*SettingsPayload, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	modifier := cfg.HotkeyModifier
	if modifier == "" {
		modifier = speech.DefaultModifier
	}

	return &SettingsPayload{
		AnthropicKey:   cfg.AnthropicKey,
		TelegramToken:  cfg.TelegramToken,
		SpeechProvider: cfg.SpeechProvider,
		SpeechModel:    cfg.SpeechModel,
		OpenAIKey:      cfg.OpenAIKey,
		GeminiKey:      cfg.GeminiKey,
		DeepgramKey:    cfg.DeepgramKey,
		HotkeyEnabled:  cfg.HotkeyEnabled,
		HotkeyModifier: modifier,
	}, nil
}

// SaveSettings persists the API key and Telegram token, then re-initialises
// the LLM client so the change takes effect immediately.
func (a *App) SaveSettings(payload SettingsPayload) error {
	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.AnthropicKey = payload.AnthropicKey
	cfg.TelegramToken = payload.TelegramToken
	cfg.SpeechProvider = payload.SpeechProvider
	cfg.SpeechModel = payload.SpeechModel
	cfg.OpenAIKey = payload.OpenAIKey
	cfg.GeminiKey = payload.GeminiKey
	cfg.DeepgramKey = payload.DeepgramKey
	cfg.HotkeyEnabled = payload.HotkeyEnabled
	cfg.HotkeyModifier = payload.HotkeyModifier

	if err := config.Save(baseDir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if !a.ready {
		// First-time setup: app could not fully initialise at launch (e.g. no
		// API key was configured). Attempt full initialisation now.
		if err := a.initCore(cfg, baseDir); err != nil {
			log.Printf("Post-save init: %v", err)
		} else {
			log.Printf("Settings saved — app initialised (agent=%s, model=%s)", a.agentCfg.Name, a.agentCfg.Model)
			speech.ShowMenuBarIcon()
		}
	} else if a.agentCfg.Model != "" {
		// Hot-reload: rebuild the LLM client with the (possibly updated) keys.
		reloadKey := cfg.OpenAIKey
		if reloadKey == "" {
			reloadKey = os.Getenv("OPENAI_API_KEY")
		}
		anthropicKey := cfg.AnthropicKey
		if anthropicKey == "" {
			anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		reloadGeminiKey := cfg.GeminiKey
		if reloadGeminiKey == "" {
			reloadGeminiKey = os.Getenv("GEMINI_API_KEY")
		}
		if newClient, err := llm.NewClientForModel(a.agentCfg.Model, anthropicKey, reloadKey, reloadGeminiKey); err == nil {
			a.deps.LLMClient = newClient
			log.Printf("Settings saved — LLM client reloaded (model=%s)", a.agentCfg.Model)
		}
	}

	// Restart Telegram bot if token is provided, stop if removed.
	if cfg.TelegramToken != "" {
		a.startTelegram(cfg)
	} else {
		a.stopTelegram()
	}

	// Enable or disable push-to-talk dictation.
	if cfg.HotkeyEnabled {
		a.startDictation(cfg)
	} else {
		speech.TeardownDictation()
	}

	return nil
}

// loadSpeechConfig builds a TranscribeConfig from the current app configuration.
func (a *App) loadSpeechConfig() (speech.TranscribeConfig, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return speech.TranscribeConfig{}, fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return speech.TranscribeConfig{}, fmt.Errorf("load config: %w", err)
	}

	provider := speech.Provider(cfg.SpeechProvider)
	if provider == "" {
		provider = speech.ProviderWhisper
	}

	var apiKey string
	switch provider {
	case speech.ProviderWhisper:
		apiKey = cfg.OpenAIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	case speech.ProviderDeepgram:
		apiKey = cfg.DeepgramKey
		if apiKey == "" {
			apiKey = os.Getenv("DEEPGRAM_API_KEY")
		}
	}

	model := cfg.SpeechModel
	if model == "" {
		model = speech.ModelGPT4oMiniTranscribe
	}

	return speech.TranscribeConfig{
		Provider: provider,
		APIKey:   apiKey,
		Language: "en",
		Model:    model,
		Prompt:   "Transcribe the following audio cleanly. Remove filler words such as um, uh, like, you know, so, and basically. Fix any grammatical errors and produce well-structured, punctuated sentences.",
	}, nil
}
