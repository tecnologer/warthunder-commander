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
- **Game-mode awareness** — Arcade shows all enemies; Realistic only shows
  enemies actively spotted by a nearby ally; Simulator suppresses all enemy
  alerts.
- **Enemy tracking** — tracks enemies across frames by proximity, with 30-second
  (60-second for close contacts) retention. Avoids re-alerting on the same
  ongoing threat within a 30-second silence window.
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

```bash
git clone https://github.com/tecnologer/warthunder
cd warthunder
go build -o warthunder-commander ./cmd/main.go
```

Or download a pre-built binary from the [Releases](../../releases) page.

## Usage

```bash
# Run with default config.toml in the current directory
./warthunder-commander

# Specific version info
./warthunder-commander --version
```

The assistant starts polling the game API every 500 ms and waits silently until a
match begins. Once a player object appears in 6 consecutive frames (≈3 s) it
announces "Match started" and begins alerting.

## Configuration

Copy `config.toml` into the same directory as the binary and edit it. All fields
are optional; the defaults are shown below.

```toml
# UI language for alerts, commander voice, and LLM responses.
# Valid values: "es" (Spanish, default), "en" (English).
language = "en"

[ai]
# Environment variable that holds the Groq API key.
# When set, Groq is used; otherwise Anthropic is used.
groq_env = "GROQ_API_KEY"

# The assistant's callsign — how the LLM addresses you.
callsign = "White Horse"

# Environment variable that holds the Anthropic API key.
anthropic_env = "ANTHROPIC_API_KEY"

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

# --- Kokoro settings ---
base_url = "http://localhost:8880"
voice    = "bm_lewis"   # bm_ = British Male; am_ = American Male; af_ = American Female
model    = "kokoro"

# --- CAMB.AI settings ---
# api_key_env = "CAMB_API_KEY"
# voice = "165304"
# language = "en-us"
```

### TTS engines

| Engine | Key required | Notes |
|--------|-------------|-------|
| `google-tts` | No | Uses Google Translate; requires internet access |
| `kokoro` | Optional | Local OpenAI-compatible `/v1/audio/speech` server |
| `camb` | Yes (`CAMB_API_KEY`) | Cloud voices; run `warthunder-commander camb list voices` to browse |

### Listing CAMB.AI voices

```bash
CAMB_API_KEY=<key> ./warthunder-commander camb list voices
CAMB_API_KEY=<key> ./warthunder-commander camb list voices --lang en-us
```

## Architecture

```
main.go  →  wt.Client.MapObjects()  →  analyzer.Analyze()  →  tts.Speaker.Speak()
                                    →  commander.Advise()   ↗
```

| Package | Responsibility |
|---------|----------------|
| `cmd/main.go` | Polling loop (500 ms), cooldown gate (4 s), TTS dispatch, AI commander timer (30 s) |
| `internal/wt` | HTTP client for `localhost:8111`; team identification by RGB color tolerance |
| `internal/analyzer` | Stateful per-frame detection: flanks, new enemies, zone pressure |
| `internal/collector` | Rolling 30-second window of map objects for the LLM prompt |
| `internal/commander` | Builds a situation-report prompt and calls Groq or Anthropic |
| `internal/tts` | Pluggable TTS: Google Translate, Kokoro, CAMB.AI; audio file cache |
| `internal/lang` | Localised strings and compass/direction logic (EN / ES) |
| `internal/config` | TOML config loader with defaults |

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
