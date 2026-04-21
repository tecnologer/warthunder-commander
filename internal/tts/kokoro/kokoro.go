package kokoro

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tecnologer/warthunder/internal/tts/player"
	"github.com/tecnologer/warthunder/internal/utils/closer"
)

// Speaker synthesises speech via the Kokoro HTTP API and plays it locally.
type Speaker struct {
	apiKey  string
	baseURL string
	voice   string
	model   string
	dir     string
	volume  int
	speed   float64
}

// New returns a Speaker configured for the Kokoro TTS API.
func New(apiKey, baseURL, voice, model, dir string, volume int, speed float64) *Speaker {
	return &Speaker{
		apiKey:  apiKey,
		baseURL: baseURL,
		voice:   voice,
		model:   model,
		dir:     dir,
		volume:  volume,
		speed:   speed,
	}
}

type request struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// Speak synthesises msg and plays it, blocking until done.
func (k *Speaker) Speak(msg string) error {
	if err := os.MkdirAll(k.dir, 0o755); err != nil {
		return fmt.Errorf("kokoro: mkdir: %w", err)
	}

	// Cache by voice+message to avoid redundant API calls.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(k.voice+msg)))
	path := filepath.Join(k.dir, "kokoro-"+hash+".mp3")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := k.fetch(msg, path); err != nil {
			return err
		}
	}

	if err := player.PlayFile(path, k.volume, k.speed); err != nil {
		return fmt.Errorf("kokoro: play: %w", err)
	}

	return nil
}

func (k *Speaker) fetch(msg, dest string) error {
	body, err := json.Marshal(request{
		Model:          k.model,
		Input:          msg,
		Voice:          k.voice,
		ResponseFormat: "mp3",
	})
	if err != nil {
		return fmt.Errorf("kokoro: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, k.baseURL+"/v1/audio/speech", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("kokoro: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("kokoro: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("kokoro: status %d: %s", resp.StatusCode, bodyBytes)
	}

	audioFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("kokoro: create %s: %w", dest, err)
	}
	defer closer.Close(audioFile)

	if _, err := io.Copy(audioFile, resp.Body); err != nil {
		os.Remove(dest) // remove partial file on write failure

		return fmt.Errorf("kokoro: write audio: %w", err)
	}

	return nil
}
