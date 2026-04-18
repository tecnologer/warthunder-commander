# Kokoro TTS

Synthesises speech via the [Kokoro](https://github.com/remsky/Kokoro-FastAPI) HTTP API (OpenAI-compatible `/v1/audio/speech` endpoint) and plays it locally through `mpv`, `mplayer`, `ffplay`, or `vlc`.

Audio files are cached on disk by voice + message hash, so identical phrases are never re-fetched.

---

## Prerequisites

One of the following audio players must be installed and on `$PATH`:

| Player    | Install (Arch)       | Install (Ubuntu/Debian)  |
|-----------|----------------------|--------------------------|
| `mpv`     | `pacman -S mpv`      | `apt install mpv`        |
| `mplayer` | `pacman -S mplayer`  | `apt install mplayer`    |
| `ffplay`  | `pacman -S ffmpeg`   | `apt install ffmpeg`     |
| `vlc`     | `pacman -S vlc`      | `apt install vlc`        |

On **Windows**, playback falls back to the built-in MCI API — no extra software needed.

---

## Option A — Local service (self-hosted)

Run the Kokoro FastAPI server on your machine. No API key is required.

### 1. Start the server

The easiest path is Docker:

```bash
# CPU-only
docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest

# NVIDIA GPU (much faster)
docker run --gpus all -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-gpu:latest
```

The server listens on `http://localhost:8880` by default.

### Workaround — Docker cannot see the GPU

If the GPU container fails with an error like `unknown or invalid runtime name: nvidia`
or `could not select device driver "nvidia"`, the NVIDIA Container Toolkit is missing
or not configured.

**1. Install the toolkit**

```bash
# Arch Linux
sudo pacman -S nvidia-container-toolkit

# Ubuntu / Debian
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
  | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
  | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
  | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

sudo apt update && sudo apt install -y nvidia-container-toolkit
```

**2. Configure the Docker runtime**

```bash
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

**3. Verify**

```bash
docker run --rm --gpus all nvidia/cuda:12.0-base-ubuntu22.04 nvidia-smi
```

You should see your GPU listed. If `nvidia-smi` reports an error, make sure the
host NVIDIA driver is installed (`nvidia-smi` outside Docker should work first).

**4. Re-run the GPU container**

```bash
docker run --gpus all -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-gpu:latest
```

If GPU access is not available, fall back to the CPU image — quality is identical,
only speed differs or try building a custom image with the workaround below.

#### GPU Blackwell

The Kokoro Docker image ships with PyTorch built for CUDA 12.4, which does not include support for NVIDIA's Blackwell architecture (SM 120). If you have an RTX 50-series GPU, the container will fail to start with:

```shell
CUDA error: no kernel image is available for execution on the device
```

##### Workaround

The included `Dockerfile` rebuilds the image with a Blackwell-compatible version of PyTorch (CUDA 12.8).

Build the image:

```bash
docker build -t kokoro-blackwell .
```

Run the container:

```bash
docker run --gpus all -p 8880:8880 kokoro-blackwell
```

### 2. `config.toml`

```toml
[tts]
engine   = "kokoro"
base_url = "http://localhost:8880"   # default; omit if unchanged
voice    = "af_sky"                  # see Voice reference below
model    = "kokoro"                  # only model currently supported
volume   = 100                       # 0–200, 100 = original level
```

No `api_key_env` is needed for a local server — the field is ignored when the
server does not require authentication.

---

## Option B — Cloud service

### 1. Obtain an API key

Sign up at the Kokoro cloud provider of your choice and copy the API key.

### 2. Export the key

```bash
export KOKORO_API_KEY="sk-..."
```

Add that line to `~/.bashrc` / `~/.zshrc` to make it permanent.

### 3. `config.toml`

```toml
[tts]
engine      = "kokoro"
base_url    = "https://api.example-kokoro-cloud.com"   # cloud endpoint
api_key_env = "KOKORO_API_KEY"                          # env var that holds the key
voice       = "af_sky"
model       = "kokoro"
volume      = 100
```

Set `base_url` to the URL provided by your cloud provider. The client appends
`/v1/audio/speech` to whatever `base_url` you supply.

---

## Full `config.toml` reference

```toml
[tts]
# Required
engine = "kokoro"

# URL of the Kokoro HTTP API. Default: "http://localhost:8880".
base_url = "http://localhost:8880"

# Voice identifier (see Voice reference below). Required.
voice = "af_sky"

# Model name sent in each request. Default / only supported value: "kokoro".
model = "kokoro"

# Name of the environment variable that holds the API key.
# Omit or leave blank for unauthenticated local servers.
# api_key_env = "KOKORO_API_KEY"

# Playback volume as a percentage (0–200). 100 = original level, 150 = +50%.
volume = 100
```

---

## Voice reference

Voices follow the pattern `<accent><gender>_<name>`. The prefix encodes accent
and gender:

| Prefix | Accent          | Gender |
|--------|-----------------|--------|
| `af_`  | American        | Female |
| `am_`  | American        | Male   |
| `bf_`  | British         | Female |
| `bm_`  | British         | Male   |

Examples:

| Voice        | Description           |
|--------------|-----------------------|
| `af_sky`     | American female       |
| `af_bella`   | American female       |
| `am_adam`    | American male         |
| `bf_emma`    | British female        |
| `bm_daniel`  | British male          |
| `bm_lewis`   | British male          |

Run the local server and call `GET /v1/voices` to list all voices it supports.

### Voice combinations

Kokoro supports blending multiple voices by joining them with `+` and optionally weighting each one with a parenthesised number:

```
am_v0adam(2)+bf_alice(1)
```

The number controls the relative weight of each voice. Higher weight means that voice has more influence on the output. You can mix any number of voices this way.

To explore and audition available voices interactively, open the web UI that ships with the local server:

```
http://localhost:8880/web
```

---

## Audio cache

Synthesised MP3 files are stored in the directory configured via the `Speaker`
constructor (`dir` parameter), which in practice is set by `cmd/main.go`. Files
are named `kokoro-<sha256>.mp3` where the hash covers the voice and message
text. Delete the directory to clear the cache and force re-synthesis.
