package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/user/talon/internal/access"
	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/claw"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/router"
)

// chatRequest is the JSON body for POST /chat.
type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	SenderID  string `json:"sender_id,omitempty"` // optional sender identity for access control
}

// chatResponse is the JSON response from POST /chat.
type chatResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

// webhookRequest is the JSON body for POST /webhook.
type webhookRequest struct {
	Source    string            `json:"source"`
	Message   string            `json:"message"`
	SessionID string            `json:"session_id"`
	AgentName string            `json:"agent_name"`
	Metadata  map[string]string `json:"metadata"`
}

// RunHTTP starts the HTTP API server. It blocks until ctx is cancelled.
// If gw is non-nil the /webhook endpoint routes events through the Claw gateway.
// accessMgr may be nil, in which case all requests are allowed.
func RunHTTP(ctx context.Context, cfg *config.Config, deps agent.Deps, gw *claw.Gateway, accessMgr *access.Manager) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, chatResponse{Error: "invalid JSON body"})
			return
		}

		if req.SessionID == "" {
			req.SessionID = "http:default"
		}
		if req.Message == "" {
			writeJSON(w, http.StatusBadRequest, chatResponse{Error: "message is required"})
			return
		}

		// Access control check.
		if accessMgr != nil && req.SenderID != "" {
			sender := access.SenderInfo{
				ID:          "http:" + req.SenderID,
				DisplayName: req.SenderID,
				Channel:     "http",
				MessageText: req.Message,
			}
			decision := accessMgr.Check(sender)
			if !decision.Allowed {
				writeJSON(w, http.StatusForbidden, chatResponse{Error: "access denied: " + decision.Reason})
				return
			}
		}

		// Route to the correct agent.
		agentCfg := router.Route(req.Message, cfg)

		content, _ := json.Marshal(req.Message)
		result, err := agent.RunAgentTurn(r.Context(), req.SessionID, content, agentCfg, deps)
		if err != nil {
			log.Printf("[http] error: %v", err)
			writeJSON(w, http.StatusInternalServerError, chatResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, chatResponse{Response: result.FinalText})
	})

	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if gw == nil {
			writeJSON(w, http.StatusServiceUnavailable, chatResponse{Error: "claw gateway not enabled"})
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, chatResponse{Error: "failed to read body"})
			return
		}

		var req webhookRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, chatResponse{Error: "invalid JSON body"})
			return
		}

		source := req.Source
		if source == "" {
			source = "webhook"
		}
		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = "claw:webhook"
		}
		agentName := req.AgentName
		if agentName == "" {
			agentName = "main"
		}

		payload := req.Message
		if payload == "" {
			payload = fmt.Sprintf("Incoming webhook from %s:\n```json\n%s\n```\nAnalyse this payload and take appropriate action.", source, string(body))
		}

		evt := claw.NewEvent(claw.EventWebhook, payload, source, sessionID, agentName)
		evt.Metadata = req.Metadata
		gw.Push(evt)

		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":   "accepted",
			"event_id": evt.ID,
		})
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		log.Println("[http] shutting down...")
		server.Close()
	}()

	log.Printf("[http] listening on %s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("[http] server error: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
