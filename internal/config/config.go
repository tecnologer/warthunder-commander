package config

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Valid TTS engine identifiers.
const (
	EngineGoogleTTS = "google-tts"
	EngineKokoro    = "kokoro"
	EngineCamb      = "camb"
)

// Valid AI engine identifiers.
const (
	AIEngineGroq      = "groq"
	AIEngineAnthropic = "anthropic"
)

// Config holds all user-tunable settings.
type Config struct {
	Language string       `toml:"language"` // "es" (default) or "en"
	AI       AIConfig     `toml:"ai"`
	Colors   ColorsConfig `toml:"colors"`
	TTS      *TTSConfig   `toml:"tts"`
}

// TTSConfig controls text-to-speech synthesis.
type TTSConfig struct {
	// Engine selects the TTS backend. Valid values: "google-tts", "kokoro", "camb".
	// Defaults to "google-tts" when omitted.
	Engine string `toml:"engine"`
	// APIKeyEnv is the name of the environment variable that holds the API key
	// for engines that require one (kokoro, camb).
	APIKeyEnv string `toml:"api_key_env"`
	// BaseURL is the root of the Kokoro HTTP API (default: http://localhost:8880).
	// Unused for other engines.
	BaseURL string `toml:"base_url"`
	// Voice is the voice identifier. For Kokoro: e.g. "af_sky", "am_adam".
	// For CAMB.AI: numeric voice ID (e.g. "20305") or voice name. Required —
	// run once without it to see available voices for your chosen language.
	Voice string `toml:"voice"`
	// Model is the model name sent in each request (Kokoro only, default: "kokoro").
	Model string `toml:"model"`
	// Language is the BCP-47 locale code sent to CAMB.AI, e.g. "es-es", "es-mx", "en-us".
	// Resolved to a numeric ID at startup via the /source-languages endpoint.
	Language string `toml:"language"`
	// Volume controls playback volume as a percentage (0–200, default 100).
	// Values above 100 amplify the audio beyond the original level.
	Volume int `toml:"volume"`
}

// cambDefaults holds the default values applied when engine = "camb" and the
// corresponding field is absent from config.toml.
const (
	cambDefaultAPIKeyEnv = "CAMB_API_KEY" //nolint:gosec // env var name, not a credential
	cambDefaultLanguage  = "es-mx"
)

// Validate fills in engine-specific defaults and returns an error if the engine
// name is invalid or required credentials are missing.
func (c *TTSConfig) Validate() error {
	if c.Engine == "" {
		c.Engine = EngineGoogleTTS
	}

	validEngines := []string{EngineGoogleTTS, EngineKokoro, EngineCamb}
	if !slices.Contains(validEngines, c.Engine) {
		return fmt.Errorf("tts.engine %q is not valid; must be one of: %s",
			c.Engine, strings.Join(validEngines, ", "))
	}

	if c.Volume == 0 {
		c.Volume = 100
	}

	if c.Engine == EngineCamb {
		if c.APIKeyEnv == "" {
			c.APIKeyEnv = cambDefaultAPIKeyEnv
		}
		// Voice is intentionally left empty when unconfigured so that
		// cambResolveVoice can print available voices and guide the user.
		if c.Language == "" {
			c.Language = cambDefaultLanguage
		}

		if os.Getenv(c.APIKeyEnv) == "" {
			return fmt.Errorf("tts.engine \"camb\": environment variable %q is not set or empty", c.APIKeyEnv)
		}
	}

	return nil
}

// Validate fills in the default AI engine and returns an error if the engine
// name is invalid.
func (a *AIConfig) Validate() error {
	if a.Engine == "" {
		a.Engine = AIEngineGroq
	}

	validEngines := []string{AIEngineGroq, AIEngineAnthropic}
	if !slices.Contains(validEngines, a.Engine) {
		return fmt.Errorf("ai.engine %q is not valid; must be one of: %s",
			a.Engine, strings.Join(validEngines, ", "))
	}

	return nil
}

// AIConfig controls which environment variable is read for each LLM backend.
type AIConfig struct {
	// Engine selects the AI backend. Valid values: "groq" (default), "anthropic".
	Engine       string `toml:"engine"`
	GroqEnv      string `toml:"groq_env"`
	AnthropicEnv string `toml:"anthropic_env"`
	Callsign     string `toml:"callsign"` // how the commander addresses the player; default "Bronco"
}

// ColorsConfig defines the RGB reference values used to identify each team.
type ColorsConfig struct {
	Tolerance float64  `toml:"tolerance"`
	Player    RGBColor `toml:"player"`
	Ally      RGBColor `toml:"ally"`
	Enemy     RGBColor `toml:"enemy"`
	Squad     RGBColor `toml:"squad"`
}

// RGBColor holds a single RGB reference color.
type RGBColor struct {
	R float64 `toml:"r"`
	G float64 `toml:"g"`
	B float64 `toml:"b"`
}

// defaults returns a Config with the values that were previously hardcoded.
func defaults() Config {
	return Config{
		Language: "es",
		AI: AIConfig{
			Engine:       AIEngineGroq,
			GroqEnv:      "GROQ_API_KEY",
			AnthropicEnv: "ANTHROPIC_API_KEY",
			Callsign:     "Bronco",
		},
		Colors: ColorsConfig{
			Tolerance: 30,
			Player:    RGBColor{R: 250, G: 200, B: 30},
			Ally:      RGBColor{R: 23, G: 77, B: 255},
			Enemy:     RGBColor{R: 250, G: 12, B: 0},
			Squad:     RGBColor{R: 103, G: 215, B: 86},
		},
		TTS: nil,
	}
}

// Load reads config.toml from path. Missing file is not an error — defaults
// are returned instead. Malformed TOML or invalid TTS config is always an error.
func Load(path string) (Config, error) {
	cfg := defaults()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("config: decode %s: %w", path, err)
	}

	if cfg.TTS != nil {
		if err := cfg.TTS.Validate(); err != nil {
			return cfg, fmt.Errorf("config: %w", err)
		}
	}

	if err := cfg.AI.Validate(); err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
}
