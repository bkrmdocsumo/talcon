package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/router"
	"github.com/user/talon/internal/util"
)

// TelegramNotify is a callback invoked when a Telegram message is received or
// a reply is sent. The frontend uses these notifications to refresh the
// Telegram chat list in real time.
type TelegramNotify func(sessionID string, userText string, replyText string)

// RunTelegram starts the Telegram bot long-poller. It blocks until ctx is cancelled.
// If notify is non-nil it is called when a message is received (replyText=="")
// and when a reply is sent (userText=="").
func RunTelegram(ctx context.Context, cfg *config.Config, deps agent.Deps, notify TelegramNotify) {
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		safeErr := regexp.MustCompile(`/bot[^/]+/`).ReplaceAllString(err.Error(), "/bot****/")
		log.Printf("[telegram] failed to init bot: %s", safeErr)
		return
	}

	log.Printf("[telegram] authorized as @%s", bot.Self.UserName)

	// Force-close any stale long-poll connections from previous instances
	// by issuing a short getUpdates with offset -1 and timeout 0. This
	// "steals" the connection and lets us start cleanly.
	clearReq := tgbotapi.NewUpdate(-1)
	clearReq.Timeout = 0
	if cleared, err := bot.GetUpdates(clearReq); err != nil {
		log.Printf("[telegram] warning: failed to clear stale updates: %v", err)
	} else if len(cleared) > 0 {
		log.Printf("[telegram] cleared %d stale update(s)", len(cleared))
	}

	// Use a custom polling loop instead of GetUpdatesChan so we can
	// properly cancel the HTTP requests via context and avoid 409
	// Conflict errors during hot-reloads.
	offset := 0
	for {
		select {
		case <-ctx.Done():
			log.Println("[telegram] shutting down...")
			return
		default:
		}

		updateCfg := tgbotapi.NewUpdate(offset)
		updateCfg.Timeout = 30 // shorter timeout for faster shutdown response

		// Run getUpdates in a goroutine so we can cancel via context.
		type updatesResult struct {
			updates []tgbotapi.Update
			err     error
		}
		ch := make(chan updatesResult, 1)
		go func() {
			updates, err := bot.GetUpdates(updateCfg)
			ch <- updatesResult{updates, err}
		}()

		select {
		case <-ctx.Done():
			log.Println("[telegram] shutting down...")
			return
		case res := <-ch:
			if res.err != nil {
				log.Printf("[telegram] getUpdates error: %v", res.err)
				// Wait before retrying, but respect context cancellation.
				select {
				case <-ctx.Done():
					log.Println("[telegram] shutting down...")
					return
				case <-time.After(3 * time.Second):
				}
				continue
			}

			for _, update := range res.updates {
				if update.UpdateID >= offset {
					offset = update.UpdateID + 1
				}

				if update.Message == nil {
					continue
				}

				userID := update.Message.From.ID
				chatID := update.Message.Chat.ID
				sessionID := fmt.Sprintf("tg_%d", userID)

				// Determine the text: regular messages use Text, attachments use Caption.
				text := update.Message.Text
				if text == "" {
					text = update.Message.Caption
				}

				log.Printf("[telegram] message from %d: %s", userID, util.Truncate(text, 80))

				// Route to the correct agent.
				agentCfg := router.Route(text, cfg)

				// Build the content payload for the agent.
				content, err := buildTelegramContent(bot, update.Message, text)
				if err != nil {
					log.Printf("[telegram] failed to build content for %d: %v", userID, err)
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Sorry, failed to process your message: %v", err))
					bot.Send(msg)
					continue
				}

			// Notify frontend that a new Telegram message was received.
			if notify != nil {
				notify(sessionID, text, "")
			}

			// Handle in a goroutine so we don't block polling.
			go func() {
				result, err := agent.RunAgentTurn(ctx, sessionID, content, agentCfg, deps)
				var response string
				if err != nil {
					log.Printf("[telegram] error for %d: %v", userID, err)
					response = fmt.Sprintf("Sorry, an error occurred: %v", err)
				} else {
					response = result.FinalText
				}

				// Telegram has a 4096 char limit per message.
				for len(response) > 0 {
					chunk := response
					if len(chunk) > 4000 {
						chunk = response[:4000]
						response = response[4000:]
					} else {
						response = ""
					}

					msg := tgbotapi.NewMessage(chatID, chunk)
					if _, err := bot.Send(msg); err != nil {
						log.Printf("[telegram] send error for %d: %v", userID, err)
					}
				}

				// Notify frontend that a reply was sent.
				if notify != nil {
					notify(sessionID, "", result.FinalText)
				}
			}()
			}
		}
	}
}

// buildTelegramContent constructs the JSON content payload for a Telegram message.
// For plain text messages it returns a JSON string; for messages with photos or
// documents it returns an array of Anthropic content blocks.
func buildTelegramContent(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, text string) (json.RawMessage, error) {
	var fileID string
	var mimeType string

	// Check for photo attachments (array of sizes; last is largest).
	if msg.Photo != nil && len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		fileID = largest.FileID
		mimeType = "image/jpeg" // Telegram always converts photos to JPEG.
	}

	// Check for document attachments (files, images sent as documents, PDFs, etc.).
	if fileID == "" && msg.Document != nil {
		fileID = msg.Document.FileID
		mimeType = msg.Document.MimeType
	}

	// No attachment — plain text message.
	if fileID == "" {
		if text == "" {
			// Unsupported message type (sticker, voice, etc.) — provide a fallback.
			text = "[unsupported message type]"
		}
		data, _ := json.Marshal(text)
		return data, nil
	}

	// Download the file from Telegram.
	fileData, err := downloadTelegramFile(bot, fileID)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(fileData)

	// Build multimodal content blocks.
	var blocks []map[string]interface{}

	if util.IsImageMime(mimeType) {
		blocks = append(blocks, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": mimeType,
				"data":       b64,
			},
		})
	} else if mimeType == "application/pdf" {
		blocks = append(blocks, map[string]interface{}{
			"type": "document",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": mimeType,
				"data":       b64,
			},
		})
	} else {
		// Other file types — include as text.
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("File (%s):\n```\n%s\n```", mimeType, string(fileData)),
		})
	}

	// Always include a text block — Anthropic requires non-empty content.
	if text == "" {
		text = "Describe this."
	}
	blocks = append(blocks, map[string]interface{}{
		"type": "text",
		"text": text,
	})

	data, _ := json.Marshal(blocks)
	return data, nil
}

// downloadTelegramFile downloads a file from Telegram's servers by file ID.
func downloadTelegramFile(bot *tgbotapi.BotAPI, fileID string) ([]byte, error) {
	url, err := bot.GetFileDirectURL(fileID)
	if err != nil {
		return nil, fmt.Errorf("get file URL: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

