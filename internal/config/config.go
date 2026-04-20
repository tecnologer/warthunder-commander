package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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

// Valid AI commander mode identifiers.
const (
	AIModeWarning     = "warning"     // situational alerts only — describes what is happening, no action verbs
	AIModeOrders      = "orders"      // direct tactical orders ("Reposition to B4", "Engage from the right")
	AIModeSuggestions = "suggestions" // soft recommendations ("Consider repositioning to B4")
)

// DefaultWTSource is the default War Thunder local API address.
const DefaultWTSource = "http://localhost:8111"

// Config holds all user-tunable settings.
type Config struct {
	Language          string              `toml:"language"`           // "es" (default) or "en"
	WTSource          string              `toml:"wt_source"`          // War Thunder API base URL (default: http://localhost:8111)
	PollInterval      time.Duration       `toml:"poll_interval"`      // how often to query the WT API (default: 500ms)
	CommanderInterval time.Duration       `toml:"commander_interval"` // how often to invoke the AI commander (default: 30s)
	LogDir            string              `toml:"log_dir"`            // directory for per-match debug logs; empty = disabled
	AI                AIConfig            `toml:"ai"`
	Colors            ColorsConfig        `toml:"colors"`
	TTS               *TTSConfig          `toml:"tts"`
	Notifications     NotificationsConfig `toml:"notifications"`
}

// MinPriority is the lowest notification level that will be delivered.
// It accepts either an integer (1–4) or the string "commander" (≡ 4).
//
//	1 = Info      — all alerts + commander (default)
//	2 = Warning   — Warning, Critical, and Commander
//	3 = Critical  — Critical and Commander only
//	4 / "commander" — Commander reports only (regular alerts silenced)
type MinPriority int

// UnmarshalTOML satisfies toml.Unmarshaler so the field can be set as either
// an integer or the string "commander" in config.toml.
func (p *MinPriority) UnmarshalTOML(v any) error { //nolint:varnamelen // 'v' matches the toml.Unmarshaler interface convention
	switch val := v.(type) {
	case int64:
		*p = MinPriority(val)
	case string:
		if val == "commander" {
			*p = MinPriority(4)
			return nil
		}

		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("min_priority: expected integer (1–4) or \"commander\", got %q", val)
		}

		*p = MinPriority(n)
	default:
		return fmt.Errorf("min_priority: expected integer or \"commander\", got %T", v)
	}

	return nil
}

// NotificationsConfig controls which alerts are delivered to the player.
type NotificationsConfig struct {
	// MinPriority is the lowest alert priority that will be spoken aloud.
	// Accepts 1–4 or the string "commander". Default: 1 (all alerts).
	MinPriority MinPriority `toml:"min_priority"`
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
	// Speed controls playback speed as a multiplier (0.25–4.0, default 1.0).
	// Values below 1.0 slow down the voice; values above 1.0 speed it up.
	Speed float64 `toml:"speed"`
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

	if c.Speed == 0 {
		c.Speed = 1.0
	}

	if c.Speed < 0.25 || c.Speed > 4.0 {
		return fmt.Errorf("tts.speed %.2f is out of range; must be between 0.25 and 4.0", c.Speed)
	}

	if c.Engine == EngineKokoro && c.Model == "" {
		c.Model = "kokoro"
	}

	if c.Engine == EngineCamb {
		return c.validateCamb()
	}

	return nil
}

// validateCamb fills CAMB.AI defaults and checks that the API key is set.
func (c *TTSConfig) validateCamb() error {
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

	return nil
}

const (
	maxCallsignWords = 3
	maxCallsignChars = 24
)

// Validate fills in the default AI engine and returns an error if the engine
// name is invalid or the callsign exceeds the allowed length.
func (a *AIConfig) Validate() error {
	if a.Engine == "" {
		a.Engine = AIEngineGroq
	}

	validEngines := []string{AIEngineGroq, AIEngineAnthropic}
	if !slices.Contains(validEngines, a.Engine) {
		return fmt.Errorf("ai.engine %q is not valid; must be one of: %s",
			a.Engine, strings.Join(validEngines, ", "))
	}

	if a.Model == "" {
		if a.Engine == AIEngineAnthropic {
			a.Model = DefaultAnthropicModel
		} else {
			a.Model = DefaultGroqModel
		}
	}

	if a.Mode == "" {
		a.Mode = AIModeWarning
	}

	validModes := []string{AIModeWarning, AIModeOrders, AIModeSuggestions}
	if !slices.Contains(validModes, a.Mode) {
		return fmt.Errorf("ai.mode %q is not valid; must be one of: %s",
			a.Mode, strings.Join(validModes, ", "))
	}

	if a.Callsign != "" {
		if len([]rune(a.Callsign)) > maxCallsignChars {
			return fmt.Errorf("ai.callsign %q exceeds %d characters", a.Callsign, maxCallsignChars)
		}

		if len(strings.Fields(a.Callsign)) > maxCallsignWords {
			return fmt.Errorf("ai.callsign %q exceeds %d words", a.Callsign, maxCallsignWords)
		}
	}

	return nil
}

// Default model identifiers per engine.
const (
	DefaultGroqModel      = "llama-3.3-70b-versatile"
	DefaultAnthropicModel = "claude-sonnet-4-6"
)

// AIConfig controls which environment variable is read for each LLM backend.
type AIConfig struct {
	// Engine selects the AI backend. Valid values: "groq" (default), "anthropic".
	Engine       string `toml:"engine"`
	GroqEnv      string `toml:"groq_env"`
	AnthropicEnv string `toml:"anthropic_env"`
	Callsign     string `toml:"callsign"` // how the commander addresses the player; default "Bronco"
	Model        string `toml:"model"`    // LLM model name; defaults per engine if omitted
	// Mode controls how the AI frames its output.
	// Valid values: "warning" (default), "orders", "suggestions".
	Mode string `toml:"mode"`
	// AlertHistoryMax is the number of recent alerts included as context in the
	// system prompt so the LLM can escalate urgency and vary phrasing.
	// Default: 3. Set to 0 to disable.
	AlertHistoryMax int `toml:"alert_history_max"`
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
		Language:          "es",
		WTSource:          DefaultWTSource,
		PollInterval:      500 * time.Millisecond,
		CommanderInterval: 30 * time.Second,
		AI: AIConfig{
			Engine:       AIEngineGroq,
			GroqEnv:      "GROQ_API_KEY",
			AnthropicEnv: "ANTHROPIC_API_KEY",
			Callsign:     "Bronco",
			Mode:         AIModeWarning,
		},
		Colors: ColorsConfig{
			Tolerance: 30,
			Player:    RGBColor{R: 250, G: 200, B: 30},
			Ally:      RGBColor{R: 23, G: 77, B: 255},
			Enemy:     RGBColor{R: 250, G: 12, B: 0},
			Squad:     RGBColor{R: 103, G: 215, B: 86},
		},
		TTS: nil,
		Notifications: NotificationsConfig{
			MinPriority: 1,
		},
	}
}

// configFileNames lists the TOML file names searched in order by LoadAuto.
var configFileNames = []string{"config.toml", "warthunder-commander.toml"} //nolint:gochecknoglobals

// CandidateDirs returns unique directories to search for config/env files.
// It tries the resolved executable path (os.Executable) then the invocation
// path (os.Args[0]) which preserves symlinks, so files placed beside a symlink
// are also found.
func CandidateDirs() []string {
	seen := map[string]bool{}

	var dirs []string

	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			dirs = append(dirs, p)
		}
	}

	if exe, err := os.Executable(); err == nil {
		add(filepath.Dir(exe))
	}

	if len(os.Args) > 0 {
		if abs, err := filepath.Abs(os.Args[0]); err == nil {
			add(filepath.Dir(abs))
		}
	}

	if len(dirs) == 0 {
		dirs = append(dirs, ".")
	}

	return dirs
}

func candidateDirs() []string {
	return CandidateDirs()
}

// LoadAuto searches for a config file next to the running executable, trying
// each name in configFileNames in order. Falls back to the working directory
// when the executable path cannot be resolved. Missing file is not an error.
func LoadAuto() (Config, error) {
	for _, dir := range candidateDirs() {
		for _, name := range configFileNames {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return Load(path)
			}
		}
	}

	dir := candidateDirs()[0]

	return Load(filepath.Join(dir, configFileNames[0]))
}

// Load reads config.toml from path. Missing file is not an error — defaults
// are returned instead. Malformed TOML or invalid TTS config is always an error.
func Load(path string) (Config, error) {
	cfg := defaults()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("config: decode %s: %w", path, err)
	}

	_ = meta // used for future key-presence checks if needed

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
