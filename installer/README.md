# warthunder-setup

A [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI installer for `warthunder-commander`.

## How it works

```
User downloads: warthunder-commander_{version}_{os}_{arch}.tar.gz
  └── warthunder-commander        ← the CLI (downloaded from GitHub by the TUI)
  └── warthunder-setup            ← the TUI installer (run this first)
  └── schema.yaml                 ← defines the config wizard fields
```

Running `./warthunder-setup` walks the user through:

1. **Install directory** — where to put the binary (default: `~/.local/bin`)
2. **Config fields** — driven by `schema.yaml`; supports text, password, bool, and select
3. **Confirm** — review all settings before proceeding
4. **Download** — fetches the correct binary for the current OS/ARCH from GitHub Releases
5. **Install** — copies the binary and writes `warthunder-commander.toml` beside the binary

## schema.yaml

The wizard is fully driven by `schema.yaml`. The current schema configures `warthunder-commander` with fields for:

- **General**: `language`, `poll_interval`, `commander_interval`, `wt_source`
- **Colors**: `colors.player|ally|enemy|squad.(r|g|b)`
- **AI**: `ai.engine` (groq/anthropic), API key env vars, `ai.model`, `ai.callsign`, `ai.mode`
- **Notifications**: `notifications.min_priority`
- **TTS**: `tts.engine` (google-tts/kokoro/camb), `tts.voice`, `tts.volume`, `tts.speed`, `tts.base_url`

### Field types

| type       | UI                          |
|------------|-----------------------------|
| `text`     | text input                  |
| `password` | masked text input           |
| `bool`     | space-to-toggle checkbox    |
| `select`   | arrow-key menu              |

## Project structure

```
installer/
├── main.go          ← entry point; embeds schema.yaml and launches TUI
├── schema.yaml      ← warthunder-commander config field definitions
├── schema/
│   └── schema.go    ← schema.yaml loader and validator
├── tui/
│   ├── tui.go       ← Bubble Tea wizard (all steps)
│   ├── message.go   ← tea.Msg types
│   ├── step.go      ← step enum
│   └── styles.go    ← lipgloss styles
└── installer/
    ├── installer.go ← GitHub release resolver, binary copy, config writer
    └── toml.go      ← TOML builder from dot-notation key/value map
```

## Building

```bash
# Run the installer locally
go run .

# Build the installer binary
go build -o warthunder-setup .

# Full release build (both CLI + setup binaries bundled)
goreleaser release --clean
```

`schema.yaml` is embedded at build time in `main.go` via `//go:embed`.
