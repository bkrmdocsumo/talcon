# Talon

A local AI assistant that runs on your Mac — chat via a native desktop app, CLI, HTTP API, or Telegram. Powered by Claude (Anthropic) with tools for shell commands, file access, web browsing, and persistent memory.

## Features

- **Native Mac app** — Dark-themed Wails app with Svelte frontend
- **CLI mode** — Interactive terminal chat
- **Server mode** — HTTP API and optional Telegram bot
- **Voice input** — Push-to-talk dictation and voice recording with transcription (Whisper or Deepgram)
- **Tools** — Run commands, read/write files, browse the web (Chromium), manage persistent memory
- **Sessions** — Conversation history with session switching
- **Configurable agents** — Customize personality via `SOUL.md` in `~/.talon/workspace/`
- **Scheduled tasks** — Cron-based morning briefings and reminders

## Requirements

- **macOS** (Apple Silicon or Intel)
- **Go 1.24+**
- **Node.js** and npm (for frontend)
- [Wails v2](https://wails.io/)
- **Anthropic API key** (Claude)

### Optional

- **OpenAI API key** — For Whisper/GPT-4o transcription
- **Deepgram API key** — Alternative speech-to-text
- **Telegram Bot Token** — For the Telegram interface

## Quick Start

### 1. Install dependencies

```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Install frontend dependencies
cd frontend && npm install && cd ..
```

### 2. Configure

On first run, Talon creates `~/.talon/` with default config. Add your API key:

```bash
# Edit ~/.talon/config.json
{
  "anthropic_key": "sk-ant-...",
  ...
}
```

Or set the environment variable:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

### 3. Run

```bash
# Development (hot-reload)
./talon.sh dev

# Build Mac app
./talon.sh build

# Run CLI
./talon.sh cli
```

## Commands (`talon.sh`)

| Command   | Description                                             |
| --------- | ------------------------------------------------------- |
| `dev`     | Start in dev mode (hot-reload frontend + Go recompile)  |
| `build`   | Build production Mac app (Apple Silicon)                |
| `universal` | Build universal binary (Intel + Apple Silicon)        |
| `dmg`     | Build app and create DMG installer                      |
| `open`    | Launch the built `.app`                                 |
| `cli`     | Build and run CLI mode                                  |
| `doctor`  | Check if system is ready for Wails development         |

## Configuration

Config lives in `~/.talon/config.json`. Key options:

| Option           | Description                                      |
| ---------------- | ------------------------------------------------ |
| `anthropic_key`  | Anthropic API key (Claude)                        |
| `telegram_token` | Optional Telegram bot token                       |
| `port`           | HTTP server port (default: 8080)                  |
| `browser_headless` | Run Chromium headless (default: true)          |
| `speech_provider` | `"whisper"` or `"deepgram"` for transcription  |
| `hotkey_enabled` | Enable push-to-talk dictation                    |
| `hotkey_modifier` | Modifier key (e.g. `"left_option"`)            |

## Project Structure

```
talon/
├── main.go           # Wails GUI entry point
├── app.go            # App logic, bindings, voice, settings
├── cmd/talon/        # CLI and server entry point
├── frontend/         # Svelte + Vite UI
├── internal/
│   ├── agent/        # LLM agent orchestration
│   ├── browser/      # Chromium automation
│   ├── config/       # Config loading
│   ├── gateway/      # CLI, HTTP, Telegram
│   ├── llm/          # Anthropic client
│   ├── scheduler/    # Cron tasks
│   ├── session/      # Conversation history
│   ├── speech/       # Voice recording, transcription, dictation
│   └── tools/        # Command, file, memory, browser tools
├── talon.sh          # Build and run scripts
└── wails.json        # Wails config
```

## License

MIT (or as specified in the repo)
