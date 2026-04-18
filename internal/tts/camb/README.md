# CAMB.AI TTS Engine

This package integrates [CAMB.AI](https://camb.ai) cloud voices into the War Thunder commander.

## Requirements

- A CAMB.AI account and API key
- `mplayer` or `mpv` installed for audio playback

## Configuration

Edit `config.toml` and set the `[tts]` section:

```toml
[tts]
engine = "camb"

# Name of the environment variable that holds your CAMB.AI API key.
# Default: "CAMB_API_KEY"
api_key_env = "CAMB_API_KEY"

# Voice: numeric CAMB.AI voice ID or exact voice name.
# Run the list-voices command (see below) to discover available voices.
voice = "165304"

# BCP-47 locale code for synthesis. Default: "es-mx".
# Examples: "en-us", "es-mx", "es-es", "pt-br"
language = "en-us"

# Playback volume as a percentage (0–200, default 100).
volume = 100

# Playback speed multiplier (0.25–4.0, default 1.0).
speed = 1.0
```

### API key

Export your CAMB.AI key before running the application:

```bash
export CAMB_API_KEY=your_key_here
go run cmd/main.go
```

Or add it to your shell profile (`.bashrc`, `.zshrc`, etc.) to make it permanent.

## Discovering voices

List all voices available for a locale:

```bash
CAMB_API_KEY=your_key_here ./warthunder-commander camb list voices --lang en-us
```

Omit `--lang` to list every voice regardless of language.

The command prints each voice's numeric ID, name, and gender — copy the ID or name directly into `config.toml`.

## How it works

1. On startup the package resolves the `language` string to a numeric CAMB.AI language ID via `/source-languages`.
2. The `voice` value is resolved to a numeric voice ID via `/list-voices` (accepts either an integer string or an exact voice name).
3. Each call to `Speak` submits a TTS job to `/apis/tts`, polls `/apis/tts/{task_id}` until the job succeeds (up to 120 s), then downloads the resulting MP3 from `/apis/tts-result/{run_id}`.
4. The MP3 is cached locally (by a SHA-256 hash of voice + language + text) so repeated phrases never hit the network twice.
5. The cached file is played with `mplayer`/`mpv`.

## Cache location

Audio files are stored in the directory configured by `log_dir` in `config.toml` (or the default `/tmp/wt-tts/`). Each file is named `camb-<sha256>.mp3`.
