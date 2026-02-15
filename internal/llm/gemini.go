package llm

import "net/http"

// geminiAPIURL is Google's OpenAI-compatible Chat Completions endpoint.
const geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"

// NewGeminiClient creates an LLM client that talks to the Gemini API via
// Google's OpenAI-compatible endpoint. It reuses the OpenAIClient
// implementation with a different base URL.
func NewGeminiClient(apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		APIKey:        apiKey,
		Model:         model,
		BaseURL:       geminiAPIURL,
		ProviderLabel: "Gemini",
		HTTPClient:    &http.Client{},
	}
}
