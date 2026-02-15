package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/router"
)

// chatRequest is the JSON body for POST /chat.
type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// chatResponse is the JSON response from POST /chat.
type chatResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

// RunHTTP starts the HTTP API server. It blocks until ctx is cancelled.
func RunHTTP(ctx context.Context, cfg *config.Config, deps agent.Deps) {
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
