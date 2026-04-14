package tts

import (
	"fmt"
	"log"
	"os"

	"github.com/tecnologer/warthunder/internal/config"
	"github.com/tecnologer/warthunder/internal/tts/camb"
	"github.com/tecnologer/warthunder/internal/tts/google"
	"github.com/tecnologer/warthunder/internal/tts/kokoro"
)

const (
	language = "es"
	audioDir = "/tmp/wt-tts"
)

// Speaker synthesises and plays text-to-speech audio.
type Speaker struct {
	speak func(msg string) error
}

// Speak synthesises msg and plays it, blocking until done.
func (s *Speaker) Speak(msg string) error {
	return s.speak(msg)
}

// New returns a Speaker for the engine specified in cfg, or a Google TTS speaker
// when cfg is nil. It validates credentials at construction time so failures
// are discovered on startup rather than on the first speech call.
func New(cfg config.TTSConfig) (*Speaker, error) {
	engine := cfg.Engine
	if engine == "" {
		engine = config.EngineGoogleTTS
	}

	switch engine {
	case config.EngineGoogleTTS:
		log.Printf("TTS: google-tts (language=%s)", language)

		return &Speaker{speak: google.New(language, audioDir, cfg.Volume).Speak}, nil

	case config.EngineKokoro:
		log.Printf("TTS: kokoro (voice=%s, model=%s, base_url=%s, volume=%d)", cfg.Voice, cfg.Model, cfg.BaseURL, cfg.Volume)
		key := os.Getenv(cfg.APIKeyEnv)
		spkr := kokoro.New(key, cfg.BaseURL, cfg.Voice, cfg.Model, audioDir, cfg.Volume)

		return &Speaker{speak: spkr.Speak}, nil

	case config.EngineCamb:
		log.Printf("TTS: camb (voice=%s, language=%s, volume=%d)", cfg.Voice, cfg.Language, cfg.Volume)
		key := os.Getenv(cfg.APIKeyEnv)

		spkr, err := camb.New(key, cfg.Voice, cfg.Language, audioDir, cfg.Volume)
		if err != nil {
			return nil, fmt.Errorf("tts: camb: %w", err)
		}

		return &Speaker{speak: spkr.Speak}, nil

	default:
		// Should not be reached — config.Validate() catches unknown engines.
		return nil, fmt.Errorf("tts: unknown engine %q", engine)
	}
}

// NewDefault returns a Google TTS speaker with no configuration.
func NewDefault() *Speaker {
	log.Printf("TTS: google-tts (language=%s)", language)

	return &Speaker{speak: google.New(language, audioDir, 100).Speak}
}
