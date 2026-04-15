package google

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/tecnologer/warthunder/internal/tts/player"
)

// Speaker downloads MP3 from Google Translate TTS and plays it via an
// external player. It caches files by a hash of the text so identical phrases
// are only downloaded once.
type Speaker struct {
	language string
	dir      string
	volume   int
	speed    float64
}

// New returns a Speaker for the given language (BCP-47, e.g. "es") that
// caches audio files under dir at the given volume (0–200, 100=normal) and
// speed (0.25–4.0, 1.0=normal).
func New(language, dir string, volume int, speed float64) *Speaker {
	return &Speaker{language: language, dir: dir, volume: volume, speed: speed}
}

// Speak synthesises msg and plays it, blocking until done.
func (g *Speaker) Speak(msg string) error {
	if err := os.MkdirAll(g.dir, 0o755); err != nil {
		return fmt.Errorf("google-tts: mkdir: %w", err)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(g.language+msg)))
	path := filepath.Join(g.dir, hash+".mp3")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := g.fetch(msg, path); err != nil {
			return err
		}
	}

	if err := player.PlayFile(path, g.volume, g.speed); err != nil {
		return fmt.Errorf("google-tts: play: %w", err)
	}

	return nil
}

func (g *Speaker) fetch(text, dest string) error {
	ttsURL := "https://translate.google.com/translate_tts?ie=UTF-8&client=tw-ob&tl=" +
		url.QueryEscape(g.language) + "&q=" + url.QueryEscape(text)

	req, err := http.NewRequest(http.MethodGet, ttsURL, nil) //nolint:noctx
	if err != nil {
		return fmt.Errorf("google-tts: build request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("google-tts: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("google-tts: status %d: %s", resp.StatusCode, body)
	}

	audioFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("google-tts: create %s: %w", dest, err)
	}
	defer audioFile.Close()

	if _, err := io.Copy(audioFile, resp.Body); err != nil {
		os.Remove(dest)

		return fmt.Errorf("google-tts: write audio: %w", err)
	}

	return nil
}
