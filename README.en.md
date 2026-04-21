# War Thunder Commander

A real-time combat assistant for War Thunder. It reads the game's local HTTP API
(`localhost:8111`) and emits voice alerts for enemy detection, flanking threats,
and contested capture zones. Every 30 seconds it also calls an LLM (Groq or
Anthropic) to deliver a one-line tactical situation report over voice.

## Features

- **Flank detection** — warns when an enemy closes within 15% of the map width at
  an angle greater than 90° from your heading (Critical priority).
- **Enemy spotted** — announces new enemies by unit type, count, and movement
  direction. Groups detections within a 1-second window into a single alert.
- **Zone pressure** — fires when a capture zone is contested and an enemy is
  within 8% of map width of it.
- **AI commander** — every 30 seconds a Groq or Anthropic LLM reads the last
  30-second battlefield summary and speaks a short tactical report (≤15 words).
  Three commander modes: `warning` (situational alerts), `orders` (direct
  tactical commands), or `suggestions` (soft recommendations).
- **Game-mode awareness** — Arcade shows all enemies; Realistic only shows
  enemies actively spotted by a nearby ally; Simulator suppresses all enemy
  alerts.
- **Enemy tracking** — tracks enemies and squad members across frames by
  proximity, with 30-second (60-second for close contacts) retention. Avoids
  re-alerting on the same ongoing threat within a 30-second silence window.
- **Notification filter** — configurable `min_priority` silences low-priority
  alerts (e.g. `min_priority = 3` delivers only Critical and Commander reports).
- **Bilingual** — full English and Spanish support for all alerts, commander
  voice, and LLM responses.
- **Pluggable TTS** — Google Translate (free, no key), Kokoro (local
  OpenAI-compatible API), or CAMB.AI.
- **Audio cache** — synthesised MP3s are cached in `/tmp/wt-tts/` and reused for
  identical strings.
- **Silent when idle** — when the game is not running the loop skips silently
  with no log spam.

## Requirements

- Go 1.21+
- `mplayer` or `mpv` installed and on `$PATH`
- War Thunder running (the local API is only active in-game)
- For Groq commander: `GROQ_API_KEY` environment variable
- For Anthropic commander: `ANTHROPIC_API_KEY` environment variable
- For CAMB.AI TTS: `CAMB_API_KEY` environment variable
- For Kokoro TTS: a local Kokoro server at `http://localhost:8880` (or any
  OpenAI-compatible `/v1/audio/speech` endpoint)

## Installation

### Recommended: setup wizard

Download `warthunder-setup` from the [Releases](../../releases) page and run it:

> `warthunder-commander` is a separate release asset — the wizard downloads and installs it for you.

Run the wizard and follow the on-screen steps:

```bash
./warthunder-setup
```

The wizard will:

1. Ask where to install the binary (default: `~/.local/bin/wtcommander`).
2. Walk you through every config option (language, AI engine/key, TTS engine, colors, etc.).
3. Download the correct `warthunder-commander` binary for your OS and architecture from GitHub Releases.
4. Write `warthunder-commander.toml` next to the binary.

### Manual build (developers)

```bash
git clone https://github.com/tecnologer/warthunder
cd warthunder
go build -o warthunder-commander ./cmd/main.go
```

## Usage

```bash
# Run with default config.toml in the current directory
./warthunder-commander

# Run with debug mode (writes raw WT API responses + per-match JSONL logs)
./warthunder-commander --debug

# Specific version info
./warthunder-commander --version
```

The assistant starts polling the game API every 500 ms and waits silently until a
match begins. Once a player object appears in 6 consecutive frames (≈3 s) it
announces "Match started" and begins alerting.

Pass `--debug` to write raw API responses to a timestamped JSONL file. Match
logs (alerts and commander prompts/responses) are also written when `log_dir` is
set in `config.toml` and `--debug` is active.

## Configuration

Copy `config.toml` into the same directory as the binary and edit it. All fields
are optional; the defaults are shown below.

```toml
# UI language for alerts, commander voice, and LLM responses.
# Valid values: "es" (Spanish, default), "en" (English).
language = "en"

[ai]
# AI backend: "groq" (default) or "anthropic".
engine = "groq"

# LLM model name; defaults per engine when omitted.
# Groq default: "llama-3.3-70b-versatile"
# Anthropic default: "claude-sonnet-4-6"
# model = "llama-3.3-70b-versatile"

# Commander personality mode.
# "warning" (default) — situational alerts, no action verbs
# "orders"            — direct tactical commands ("Reposition to B4")
# "suggestions"       — soft recommendations ("Consider repositioning")
mode = "warning"

# The assistant's callsign — how the LLM addresses you (max 3 words / 24 chars).
callsign = "Bronco"

# Number of recent alerts included as context so the LLM can vary phrasing.
# Default: 3. Set to 0 to disable.
alert_history_max = 3

# Environment variable that holds the Groq API key.
groq_env = "GROQ_API_KEY"

# Environment variable that holds the Anthropic API key.
anthropic_env = "ANTHROPIC_API_KEY"

[notifications]
# Minimum priority level to deliver. Accepts 1–4 or the string "commander".
#   1 = Info      — all alerts + commander (default)
#   2 = Warning   — Warning, Critical, and Commander
#   3 = Critical  — Critical and Commander only
#   4 / "commander" — Commander reports only (regular alerts silenced)
min_priority = 1

[colors]
# Per-channel RGB tolerance for team identification.
tolerance = 30

[colors.player]
r = 250
g = 200
b = 30

[colors.ally]
r = 23
g = 77
b = 255

[colors.enemy]
r = 250
g = 12
b = 0

[colors.squad]
r = 103
g = 215
b = 86

[tts]
# Engine: "google-tts" (default, no key), "kokoro", or "camb".
engine = "kokoro"

# Playback volume as a percentage (0–200, default 100).
volume = 150

# Playback speed multiplier (0.25–4.0, default 1.0).
speed = 1.0

# --- Kokoro settings ---
# See internal/tts/kokoro/README.md for full Kokoro configuration details.

# --- CAMB.AI settings ---
# See internal/tts/camb/README.md for full CAMB.AI configuration details.
```

### TTS engines

| Engine | Key required | Notes |
|--------|-------------|-------|
| `google-tts` | No | Uses Google Translate; requires internet access |
| `kokoro` | Optional | Local OpenAI-compatible `/v1/audio/speech` server; see [`internal/tts/kokoro/README.md`](internal/tts/kokoro/README.md) for setup and voice reference |
| `camb` | Yes (`CAMB_API_KEY`) | Cloud voices; see [`internal/tts/camb/README.md`](internal/tts/camb/README.md) for setup and voice discovery |

## Architecture

```
main.go  →  wt.Client.MapObjects()  →  analyzer.Analyze()  →  tts.Speaker.Speak()
                                    →  collector.Add()
                                    →  commander.Advise()   ↗
```

### Detection rules (priority order)

1. **Critical** — enemy within 15% of map width at >90° from player heading → flank alert with side.
2. **Warning** — newly confirmed enemy (seen in two consecutive frames) → grouped detection alert.
3. **Warning** — contested capture zone with enemy within 8% of map width.

At most one alert fires per 4-second cooldown window; highest priority wins.

### Team identification

Teams are identified by matching the `color[]` RGB array from `map_obj.json`
against configurable reference colors with a per-channel tolerance (default ±30).
There is no team ID field in the API.

## Development

```bash
# Run tests
go test ./...

# Run tests with output
go test -v ./...

# Lint
golangci-lint run

# Build with version
go build -ldflags "-X main.version=v1.0.0" -o warthunder-commander ./cmd/main.go
```

## War Thunder local API

`localhost:8111` is only active while the game is running. Key endpoints:

| Endpoint | Used for |
|----------|----------|
| `/map_obj.json` | Player, ally, enemy, and capture-zone positions each frame |
| `/map_info.json` | Map name and dimensions |

Coordinates in `map_obj.json` are normalised to `[0.0, 1.0]`. The player object
includes `dx`/`dy` heading vectors; other objects do not.
