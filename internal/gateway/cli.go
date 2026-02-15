package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/config"
)

// RunCLI starts an interactive REPL that reads user input from stdin
// and sends it to the agent loop.
func RunCLI(ctx context.Context, agentCfg config.AgentConfig, deps agent.Deps) error {
	scanner := bufio.NewScanner(os.Stdin)
	sessionID := agentCfg.SessionPrefix + "_cli"

	fmt.Printf("Talon (%s) — type your message, or 'exit' to quit.\n", agentCfg.Name)
	fmt.Println(strings.Repeat("─", 50))

	for {
		fmt.Print("\nyou> ")
		if !scanner.Scan() {
			break // EOF
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
			fmt.Println("Goodbye.")
			return nil
		}

		content, _ := json.Marshal(input)
		result, err := agent.RunAgentTurn(ctx, sessionID, content, agentCfg, deps)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
			continue
		}

		fmt.Printf("\n%s> %s\n", agentCfg.Name, result.FinalText)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return nil
}
