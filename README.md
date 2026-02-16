<div align="center">

# Talon

**A local AI assistant that lives on your Mac.**

Chat through a native desktop app, the terminal, an HTTP API, or Telegram.
Powered by Claude, GPT, and Gemini — with tools for shell commands, file access, web browsing, code execution, and persistent memory.

[Getting Started](#getting-started) · [Features](#features) · [Commands](#commands) · [Troubleshooting](#troubleshooting) · [Configuration](#configuration) · [Architecture](#architecture)

</div>

---

## Features

### Multiple Interfaces

| Interface | Description |
|-----------|-------------|
| **Desktop App** | Native macOS app built with Wails + Svelte. Dark theme, streaming responses, session management. |
| **CLI** | Interactive terminal REPL. Lightweight, scriptable. |
| **HTTP API** | REST server for programmatic access (`POST /chat`, `GET /health`). |
| **Telegram Bot** | Chat from your phone. Supports text, images, and PDFs. |

### Built-in Tools

The agent can autonomously use tools to accomplish tasks:

- **Shell commands** — Run any terminal command with configurable approval rules
- **File read/write** — Session-scoped file access with workspace isolation
- **Web browsing** — Full Chromium automation (navigate, click, type, scroll) via semantic DOM snapshots
- **Persistent memory** — Save, search, list, and delete long-term knowledge as Markdown files
- **Code execution** — Run JavaScript, Python, or Go in a sandboxed WASM environment (no network, no filesystem, 30s timeout)
- **Task planning** — Structured todo lists with real-time progress updates in the UI

### Voice & Dictation

- **Push-to-talk dictation** — Hold a modifier key to record, release to transcribe and paste anywhere on your Mac
- **Double-tap grammar fix** — Double-tap the modifier key to fix grammar, spelling, and punctuation of selected text via LLM (uses `gpt-4o-mini`)
- **Voice recording** — Record and transcribe audio directly in the app
- **Transcription providers** — OpenAI Whisper / GPT-4o Transcribe or Deepgram Nova-2
- **Snippet expansion** — Define trigger phrases that auto-expand to full text during dictation (e.g., "my email" → "john@example.com")
- **Menu bar indicator** — Waveform icon shows recording/transcription state
- **Flow history** — All voice transcriptions are saved and browsable in the Flow tab

### Agent Tasks

A dedicated workspace mode for complex, multi-step tasks:

- The agent plans its work with a visible todo checklist
- Code and files are written to an isolated workspace directory
- Created files are listed in the sidebar and can be opened in your default editor
- Follow-up messages let you iterate on the agent's work

### Other

- **Session management** — Multiple conversation threads with full history
- **Customisable personality** — Edit `Master_prompt.md` in `~/.talon/workspace/` to change how the agent behaves
- **Multi-model support** — Switch between Claude (Anthropic), GPT (OpenAI), and Gemini (Google) models from the UI; provider is auto-detected from the model name
- **Scheduled tasks** — Cron-based automation (e.g., morning briefings)
- **Streaming responses** — Real-time text and thinking-block streaming
- **Extended thinking** — See the agent's reasoning process as it works

---

## Getting Started

### Prerequisites

| Requirement | Notes |
|------------|-------|
| **macOS** | Apple Silicon or Intel |
| **Go 1.24+** | [golang.org/dl](https://golang.org/dl/) |
| **Node.js + npm** | For the Svelte frontend |
| **Wails v2** | [wails.io](https://wails.io/) |
| **LLM API key** | At least one of: [Anthropic](https://console.anthropic.com/), [OpenAI](https://platform.openai.com/), or [Google Gemini](https://ai.google.dev/) |

### Install

```bash
# Install the Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Clone the repo
git clone https://github.com/user/talon.git
cd talon

# Install frontend dependencies
cd frontend && npm install && cd ..
```

### Configure

On first run, Talon creates `~/.talon/` with default config files. Set your API key using one of these methods:

**Option A — Config file** (`~/.talon/config.json`):

```json
{
  "anthropic_key": "sk-ant-..."
}
```

**Option B — Environment variable**:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

**Option C — Settings UI**: Open the app and add your key in the Settings modal.

### Run

```bash
# Development mode (hot-reload frontend + live Go recompilation)
./talon.sh dev

# Build the production Mac app
./talon.sh build

# Run in CLI mode
./talon.sh cli
```

---

## Commands

All build and run operations go through `talon.sh`:

| Command | Description |
|---------|-------------|
| `./talon.sh dev` | Start development mode with hot-reload |
| `./talon.sh build` | Build production `.app` (Apple Silicon) |
| `./talon.sh universal` | Build universal binary (Intel + Apple Silicon) |
| `./talon.sh dmg` | Build and package as DMG installer |
| `./talon.sh open` | Launch the built `.app` |
| `./talon.sh cli` | Build and run the CLI |
| `./talon.sh doctor` | Check if your system is ready for Wails development |

### CLI & Server Modes

The standalone binary (`cmd/talon/`) supports additional flags:

```bash
# Interactive CLI
./talon-cli --mode cli

# HTTP server + Telegram bot
./talon-cli --mode server

# Use a specific agent profile
./talon-cli --agent researcher

# Disable browser tools
./talon-cli --no-browser
```

---

## Troubleshooting

### macOS Gatekeeper / Quarantine

If macOS blocks the app with a message like **"Talon.app is damaged and can't be opened"** or **"Apple cannot check it for malicious software"**, run the included quarantine removal script:

```bash
./remove_quarantine.sh
```

The script automatically looks for `Talon.app` in `/Applications` and `~/Downloads`. If the app is elsewhere, it will prompt you for the full path. It removes the quarantine extended attribute (`xattr -cr`) so macOS allows the app to launch normally.

> **Note:** The script requires `sudo` to clear the quarantine attributes.

---

## Configuration

All config lives in `~/.talon/config.json`. Key options:

### API Keys

| Key | Description |
|-----|-------------|
| `anthropic_key` | Anthropic API key (required for Claude models) |
| `openai_key` | OpenAI API key (for GPT chat models, Whisper transcription, and grammar fix) |
| `gemini_key` | Google Gemini API key (for Gemini models) |
| `deepgram_key` | Deepgram API key (alternative transcription provider) |
| `telegram_token` | Telegram Bot API token |

> **Provider auto-detection:** The model name prefix determines which provider and API key are used — `claude-*` → Anthropic, `gpt-*` / `o1-*` / `o3-*` / `o4-*` → OpenAI, `gemini-*` → Google.

### General

| Key | Default | Description |
|-----|---------|-------------|
| `port` | `8080` | HTTP server port |
| `browser_headless` | `true` | Run Chromium in headless mode |
| `speech_provider` | `"whisper"` | Transcription backend: `"whisper"` or `"deepgram"` |
| `speech_model` | `"gpt-4o-mini-transcribe"` | OpenAI transcription model |
| `hotkey_enabled` | `false` | Enable system-wide push-to-talk dictation |
| `hotkey_modifier` | `"right_option"` | Modifier key for push-to-talk |

### Agent Profiles

Agents are configured under the `agents` map. Each agent has:

| Key | Description |
|-----|-------------|
| `name` | Display name |
| `model` | LLM model identifier (e.g., `claude-sonnet-4-20250514`) |
| `prompt_path` | Path to the system prompt file (relative to `~/.talon/`) |
| `session_prefix` | Prefix for session IDs |
| `enable_thinking` | Show extended thinking blocks |

### Command Approvals

`~/.talon/exec-approvals.json` controls which shell commands the agent can run:

- **Allowlist** — Commands that run without confirmation
- **Blocklist** — Commands that are always rejected

---

## Architecture

### Project Structure

```
talon/
├── main.go                # Wails GUI entry point
├── app.go                 # App initialisation and lifecycle
├── app_chat.go            # Chat bindings (GUI ↔ Go)
├── app_agent.go           # Agent task bindings
├── app_speech.go          # Voice recording, transcription, grammar fix
├── app_telegram.go        # Telegram bot lifecycle
├── app_settings.go        # Settings management
├── app_flow.go            # Flow (voice) bindings
├── app_snippets.go        # Snippet CRUD operations
├── cmd/talon/             # CLI and server entry point
│   └── main.go
├── frontend/              # Svelte + Vite UI
│   └── src/
│       ├── App.svelte
│       ├── components/
│       │   ├── SnippetsPanel.svelte   # Snippet management UI
│       │   ├── LoadingSpinner.svelte  # Shared loading indicator
│       │   └── ...                    # Chat, Agent, Flow, Settings, etc.
│       └── lib/
│           ├── stores/
│           │   ├── chatStore.js       # Chat, streaming, Telegram chat list
│           │   ├── agentStore.js      # Agent tasks, progress, files
│           │   ├── flowStore.js       # Voice transcription history
│           │   └── snippetsStore.js   # Snippet list and panel state
│           └── utils/
│               └── formatters.js      # Tool labels, path/URL shortening
├── internal/
│   ├── agent/             # LLM agent loop (tool calls, streaming, thinking)
│   ├── browser/           # Chromium automation (chromedp)
│   ├── config/            # Config loading, bootstrap, defaults
│   ├── gateway/           # CLI, HTTP, and Telegram interfaces
│   ├── llm/               # Multi-provider LLM clients
│   │   ├── anthropic.go   # Claude (Anthropic)
│   │   ├── openai.go      # GPT / o-series (OpenAI)
│   │   ├── gemini.go      # Gemini (Google, via OpenAI-compatible API)
│   │   └── provider.go    # Auto-detection by model name prefix
│   ├── router/            # Message routing to agent profiles
│   ├── sandbox/           # WASM code execution (wazero)
│   ├── scheduler/         # Cron-based task scheduling
│   ├── session/           # Conversation persistence (JSONL)
│   ├── speech/            # Recording, transcription, dictation (macOS)
│   ├── tools/             # Tool registry and implementations
│   └── util/              # Shared utilities
├── talon.sh               # Build and run scripts
├── remove_quarantine.sh   # Fix macOS Gatekeeper quarantine
└── wails.json             # Wails config
```

### Core Flow

```
User Input
    │
    ▼
┌─────────────────┐     ┌───────────────┐
│   Interface      │     │   Router      │
│  (GUI/CLI/HTTP/  │────▶│  Selects agent│
│   Telegram)      │     │  profile      │
└─────────────────┘     └───────┬───────┘
                                │
                                ▼
                       ┌────────────────┐
                       │   Agent Loop   │
                       │  (max 25 iter) │
                       └───────┬────────┘
                               │
                    ┌──────────┼──────────┐
                    ▼          ▼          ▼
              ┌──────────┐ ┌───────┐ ┌──────┐
              │  LLM API │ │ Tools │ │Session│
              │(Anthropic│ │(shell,│ │(JSONL │
              │ /OpenAI/ │ │file,  │ │history│
              │ Gemini)  │ │browse,│ └──────┘
              └──────────┘ │memory,│
                           │code)  │
                           └───────┘
```

### Data Storage

All data is stored locally in `~/.talon/`:

| Path | Contents |
|------|----------|
| `sessions/` | Conversation history (JSONL files) |
| `sessions/tg_*/` | Telegram user sessions |
| `memory/` | Persistent knowledge (Markdown files) |
| `agents/` | Agent task workspaces (one directory per task) |
| `workspace/` | Shared workspace, home of `Master_prompt.md` |
| `flow/` | Voice recording transcripts |
| `snippets.json` | Text expansion triggers and replacements |

---

## Optional Integrations

### Telegram Bot

1. Create a bot via [@BotFather](https://t.me/BotFather)
2. Add the token to config or set `TELEGRAM_BOT_TOKEN`
3. The bot auto-starts with the GUI, or run in server mode

Supports text, images, and PDF attachments. Each Telegram user gets an isolated session (stored in `~/.talon/sessions/tg_{userID}/`).

The desktop app also shows a live **Telegram chat list** in the sidebar — browse and review all Telegram conversations without leaving the app.

### Snippets (Text Expansion)

1. Open the Snippets panel from Settings
2. Add trigger → expansion pairs (e.g., "sig" → "Best regards, John")
3. Snippets auto-expand during voice dictation and push-to-talk

Snippets are stored in `~/.talon/snippets.json` and matching is case-insensitive.

### Push-to-Talk Dictation

1. Enable in Settings (`hotkey_enabled: true`)
2. Choose a modifier key (e.g., Right Option)
3. Hold the key to record, release to transcribe
4. Transcribed text is pasted at your cursor position — works in any app

Requires microphone and accessibility permissions on macOS.

**Double-tap shortcut:** Quickly double-tap the modifier key (within 400ms) to fix grammar and spelling of the currently selected text in any app. Requires an OpenAI API key.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Desktop framework | [Wails v2](https://wails.io/) |
| Frontend | [Svelte 4](https://svelte.dev/) + [Vite 5](https://vitejs.dev/) |
| Backend | Go 1.24 |
| LLM | [Anthropic Claude](https://anthropic.com/) · [OpenAI GPT](https://openai.com/) · [Google Gemini](https://ai.google.dev/) |
| Browser automation | [chromedp](https://github.com/chromedp/chromedp) |
| Code sandbox | [wazero](https://wazero.io/) (WASM) |
| Telegram | [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api) |
| Scheduling | [robfig/cron](https://github.com/robfig/cron) |
| Speech | OpenAI Whisper / Deepgram + macOS AVFoundation |
