package llm

import (
	"fmt"
	"strings"
)

// Provider identifies which LLM API backend to use.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "gemini"
)

// DetectProvider returns the API provider for a given model ID.
func DetectProvider(model string) Provider {
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "gemini-") {
		return ProviderGemini
	}
	if strings.HasPrefix(lower, "gpt-") ||
		strings.HasPrefix(lower, "o1-") ||
		strings.HasPrefix(lower, "o3-") ||
		strings.HasPrefix(lower, "o4-") {
		return ProviderOpenAI
	}
	// Default to Anthropic (claude-* models and anything else).
	return ProviderAnthropic
}

// NewClientForModel creates the appropriate LLM client based on the model name.
// anthropicKey, openaiKey, and geminiKey are the respective API keys; the
// function selects which one to use based on the detected provider.
func NewClientForModel(model, anthropicKey, openaiKey, geminiKey string) (LLMClient, error) {
	provider := DetectProvider(model)
	switch provider {
	case ProviderGemini:
		if geminiKey == "" {
			return nil, fmt.Errorf("Gemini API key not configured — add it in Settings to use %s", model)
		}
		return NewGeminiClient(geminiKey, model), nil
	case ProviderOpenAI:
		if openaiKey == "" {
			return nil, fmt.Errorf("OpenAI API key not configured — add it in Settings to use %s", model)
		}
		return NewOpenAIClient(openaiKey, model), nil
	default:
		if anthropicKey == "" {
			return nil, fmt.Errorf("Anthropic API key not configured — add it in Settings to use %s", model)
		}
		return NewClient(anthropicKey, model), nil
	}
}
