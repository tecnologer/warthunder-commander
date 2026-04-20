package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tecnologer/warthunder/internal/analyzer"
	"github.com/tecnologer/warthunder/internal/collector"
	"github.com/tecnologer/warthunder/internal/commander"
	"github.com/tecnologer/warthunder/internal/config"
	"github.com/tecnologer/warthunder/internal/lang"
	"github.com/tecnologer/warthunder/internal/matchlog"
	"github.com/tecnologer/warthunder/internal/tts"
	"github.com/tecnologer/warthunder/internal/tts/camb"
	"github.com/tecnologer/warthunder/internal/utils/closer"
	"github.com/tecnologer/warthunder/internal/wt"
	"github.com/urfave/cli/v2"
)

// version is set at build time via -ldflags "-X main.version=<version>".
var version = "dev"

const (
	alertCooldown    = 4 * time.Second
	minConfirmFrames = 6 // 3 s at 500 ms — avoids reacting to transient loading states
)

// isVersionFlag reports whether the user invoked the binary with --version or -v.
func isVersionFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			return true
		}
	}

	return false
}

// loadDotEnv looks for a .env file next to the running binary and sets any
// KEY=VALUE pairs found there as environment variables (skipping keys already set).
// It tries two candidate directories: the resolved executable path (via
// os.Executable / /proc/self/exe) and the path as invoked (os.Args[0]), which
// preserves symlinks so the .env placed beside a symlink is also found.
func loadDotEnv() {
	dirs := config.CandidateDirs()

	var envFile *os.File
	var path string
	for _, dir := range dirs {
		p := filepath.Join(dir, ".env")
		f, err := os.Open(p)
		if err == nil {
			envFile = f
			path = p
			break
		}
	}
	if envFile == nil {
		return
	}

	defer closer.Close(envFile)

	log.Printf("loading env from %s", path)

	scanner := bufio.NewScanner(envFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("error reading env file %s: %v", path, err)
	}
}

func main() {
	if !isVersionFlag() {
		loadDotEnv()
	}

	cfg, err := config.LoadAuto()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := &cli.App{
		Name:    "warthunder",
		Usage:   "War Thunder combat assistant",
		Version: version,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "write raw WT API responses to a JSONL file on every poll",
			},
		},
		Action: func(cliCtx *cli.Context) error {
			return runAssistant(cfg, cliCtx.Bool("debug"))
		},
		Commands: []*cli.Command{
			cambCommand(cfg),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// cambCommand returns the top-level "camb" subcommand.
func cambCommand(cfg config.Config) *cli.Command {
	return &cli.Command{
		Name:  "camb",
		Usage: "CAMB.AI utilities",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List CAMB.AI resources",
				Subcommands: []*cli.Command{
					{
						Name:  "voices",
						Usage: "List available voices",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "lang",
								Usage: "locale to filter voices (e.g. es-mx, en-us); defaults to tts.language in config.toml",
								Value: func() string {
									if cfg.TTS != nil {
										return cfg.TTS.Language
									}

									return ""
								}(),
							},
						},
						Action: func(cliCtx *cli.Context) error {
							return runCambListVoices(cliCtx, cfg)
						},
					},
				},
			},
		},
	}
}

func runCambListVoices(cliCtx *cli.Context, cfg config.Config) error {
	if cfg.TTS == nil || cfg.TTS.Engine != config.EngineCamb {
		return cli.Exit(`camb commands require engine = "camb" in [tts] section of config.toml`, 1)
	}

	apiKey := os.Getenv(cfg.TTS.APIKeyEnv)
	if apiKey == "" {
		return cli.Exit(fmt.Sprintf("env var %q is not set", cfg.TTS.APIKeyEnv), 1)
	}

	voices, err := camb.ListVoices(cliCtx.Context, apiKey, cliCtx.String("lang"))
	if err != nil {
		return fmt.Errorf("list voices: %w", err)
	}

	if len(voices) == 0 {
		fmt.Printf("no voices found for locale %q\n", cliCtx.String("lang"))

		return nil
	}

	fmt.Printf("%-10s  %-6s  %s\n", "ID", "Gender", "Name")
	fmt.Printf("%-10s  %-6s  %s\n", "----------", "------", "----------")

	for _, voice := range voices {
		fmt.Printf("%-10d  %-6s  %s\n", voice.ID, voice.GenderLabel(), voice.Name)
	}

	return nil
}

// initTTS creates and returns the TTS speaker from config, or a default Google TTS speaker.
func initTTS(cfg config.Config) (*tts.Speaker, error) {
	if cfg.TTS != nil {
		speaker, err := tts.New(*cfg.TTS)
		if err != nil {
			return nil, fmt.Errorf("tts: %w", err)
		}

		return speaker, nil
	}

	return tts.NewDefault(), nil
}

// anyPlayerPresent reports whether any object in objs is the local player.
func anyPlayerPresent(objs []wt.MapObject) bool {
	for idx := range objs {
		if objs[idx].IsPlayer() {
			return true
		}
	}

	return false
}

// notifyMatchEnded logs and resets state when a match is detected as ended.
func notifyMatchEnded(inMatch bool, resetFn func()) {
	if inMatch {
		resetFn()
		log.Println("Match ended. Waiting for next match...")
	}
}

// dispatchAlert fires a TTS alert if one is ready, passes the priority filter,
// and the cooldown has elapsed. Returns the updated lastAlert time.
func dispatchAlert(alert *analyzer.Alert, lastAlert time.Time, minPriority int, logger *matchlog.Logger, speak func(string)) time.Time {
	if alert == nil || alert.Priority < minPriority || time.Since(lastAlert) < alertCooldown {
		return lastAlert
	}

	log.Printf("[priority %d] %s", alert.Priority, alert.Message)
	logger.Alert(alert.Priority, alert.Message)

	go speak(alert.Message)

	return time.Now()
}

// invokeCommander cancels any in-flight commander call, then starts a new one.
// It returns the new cancel func so the caller can replace cmdCancel.
func invokeCommander(
	cmd *commander.Commander,
	col *collector.Collector,
	client *wt.Client,
	logger *matchlog.Logger,
	speak func(string),
	oldCancel context.CancelFunc,
) context.CancelFunc {
	oldCancel()

	mapInfo, _ := client.MapInfo()
	sum := col.Summary()

	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // cancel is returned to caller
	go func(ctx context.Context, sum *collector.Summary, info *wt.MapInfo) {
		report, prompt, err := cmd.Advise(ctx, sum, info)

		response := ""
		if report != nil {
			response = report.Message
		}

		logger.CommanderPrompt(prompt, response)

		if err != nil {
			if !errors.Is(err, commander.ErrNoReport) && ctx.Err() == nil {
				log.Printf("[commander] error: %v", err)
			}

			return
		}

		if report != nil {
			log.Printf("[commander] %s", report.Message)
			speak(report.Message)
		}
	}(ctx, sum, mapInfo)

	return cancel
}

// assistantState holds all mutable state for the polling loop.
type assistantState struct {
	client            *wt.Client
	alerter           *analyzer.Analyzer
	col               *collector.Collector
	cmd               *commander.Commander
	language          lang.Language
	notifications     config.NotificationsConfig
	commanderInterval time.Duration
	logDir            string
	logger            *matchlog.Logger
	inMatch           bool
	confirmFrames     int
	matchMap          string
	matchMode         wt.GameMode
	cmdCancel         context.CancelFunc
	lastAlert         time.Time
	lastCommand       time.Time
}

func newAssistantState(cfg config.Config) *assistantState {
	language := lang.Parse(cfg.Language)
	cmdInterval := cfg.CommanderInterval

	return &assistantState{
		client:            wt.NewClient(cfg.WTSource),
		alerter:           analyzer.New(language),
		col:               collector.New(cmdInterval),
		cmd:               commander.New(cfg.AI, language, cmdInterval),
		language:          language,
		notifications:     cfg.Notifications,
		commanderInterval: cmdInterval,
		logDir:            cfg.LogDir,
		cmdCancel:         func() {},
	}
}

func (s *assistantState) reset() {
	s.inMatch = false
	s.confirmFrames = 0
	s.matchMap = ""
	s.matchMode = 0

	s.cmdCancel()
	s.logger.VisibilitySummary(s.alerter.VisibilitySummary())
	s.logger.MatchEnd()
	s.logger = nil

	s.alerter = analyzer.New(s.language)
	s.col = collector.New(s.commanderInterval)
	s.cmd.ResetLastAlert()
	s.lastAlert = time.Time{}
	s.lastCommand = time.Time{}
}

// isNewMatch returns true when the current map name or game mode differs from
// what was recorded at match start. When a change is detected it resets state
// and logs the transition so the caller can start fresh confirmation.
func (s *assistantState) isNewMatch(_ *wt.MapInfo, mode wt.GameMode) bool {
	currentMap := s.client.MapName()

	mapChanged := currentMap != "" && currentMap != s.matchMap
	modeChanged := mode != s.matchMode

	if !mapChanged && !modeChanged {
		return false
	}

	log.Printf("New match detected (map %q→%q, mode %s→%s). Resetting.", s.matchMap, currentMap, s.matchMode, mode)
	s.reset()

	return true
}

func (s *assistantState) pollFrame(speak func(string)) { //nolint:cyclop
	objs, err := s.client.MapObjects()
	if err != nil {
		notifyMatchEnded(s.inMatch, s.reset)
		s.inMatch = false
		s.confirmFrames = 0

		return
	}

	if !anyPlayerPresent(objs) {
		notifyMatchEnded(s.inMatch, s.reset)
		s.inMatch = false
		s.confirmFrames = 0

		return
	}

	mapInfo, _ := s.client.MapInfo()
	if mapInfo == nil || !mapInfo.Valid {
		notifyMatchEnded(s.inMatch, s.reset)
		s.inMatch = false
		s.confirmFrames = 0

		return
	}

	mode := s.client.GameMode()

	if !s.inMatch {
		s.confirmFrames++
		if s.confirmFrames < minConfirmFrames {
			return
		}

		mapName := s.client.MapName()
		s.inMatch = true
		s.matchMap = mapName
		s.matchMode = mode
		s.lastCommand = time.Now()
		s.logger = matchlog.New(s.logDir)
		s.logger.MatchStart(mapName)

		if mapName != "" {
			log.Printf("Match started. Map: %q, Mode: %s", mapName, mode)
		} else {
			log.Printf("Match started. Mode: %s (map name unavailable)", mode)
		}
	} else if s.isNewMatch(mapInfo, mode) {
		return
	}

	s.col.Add(objs)

	alert := s.alerter.Analyze(objs, mode)
	s.lastAlert = dispatchAlert(alert, s.lastAlert, int(s.notifications.MinPriority), s.logger, speak)

	if s.cmd != nil && int(s.notifications.MinPriority) <= analyzer.PriorityCommander && time.Since(s.lastCommand) >= s.commanderInterval {
		s.lastCommand = time.Now()
		s.cmdCancel = invokeCommander(s.cmd, s.col, s.client, s.logger, speak, s.cmdCancel)
	}
}

// logTTSConfig prints the [tts] section of the configuration.
func logTTSConfig(tts *config.TTSConfig) {
	log.Println("[tts]")

	if tts == nil {
		log.Printf("  Engine       : google-tts (default)")
		log.Printf("  Volume       : 100%%")
		log.Printf("  Speed        : 1.0x")

		return
	}

	log.Printf("  Engine       : %s", tts.Engine)
	log.Printf("  Volume       : %d%%", tts.Volume)
	log.Printf("  Speed        : %.2fx", tts.Speed)

	switch tts.Engine {
	case config.EngineKokoro:
		log.Printf("  Base URL     : %s", tts.BaseURL)
		log.Printf("  Voice        : %s", tts.Voice)
		log.Printf("  Model        : %s", tts.Model)

		if tts.APIKeyEnv != "" {
			log.Printf("  Env var      : %s %s", tts.APIKeyEnv, envStatus(tts.APIKeyEnv))
		}
	case config.EngineCamb:
		log.Printf("  Voice        : %s", tts.Voice)
		log.Printf("  Language     : %s", tts.Language)
		log.Printf("  Env var      : %s %s", tts.APIKeyEnv, envStatus(tts.APIKeyEnv))
	}
}

// envStatus returns "(set)" or "(not set)" for the given env var name.
func envStatus(name string) string {
	if os.Getenv(name) != "" {
		return "(set)"
	}

	return "(not set)"
}

// logConfig prints every configuration value that the assistant will use,
// including fields that were left at their defaults.
func logConfig(cfg config.Config) {
	log.Println("──── Configuration ────────────────────────────────────")
	log.Printf("  Language     : %s", cfg.Language)
	log.Printf("  WT source    : %s", cfg.WTSource)
	log.Printf("  Poll         : %s", cfg.PollInterval)
	log.Printf("  Alert cooldown: %s", alertCooldown)
	log.Printf("  Commander    : every %s", cfg.CommanderInterval)

	if cfg.LogDir != "" {
		log.Printf("  Match logs   : %s", cfg.LogDir)
	} else {
		log.Printf("  Match logs   : disabled")
	}

	log.Println("[ai]")
	log.Printf("  Engine       : %s", cfg.AI.Engine)
	log.Printf("  Model        : %s", cfg.AI.Model)
	log.Printf("  Mode         : %s", cfg.AI.Mode)
	log.Printf("  Callsign     : %s", cfg.AI.Callsign)

	switch cfg.AI.Engine {
	case config.AIEngineAnthropic:
		log.Printf("  Env var      : %s %s", cfg.AI.AnthropicEnv, envStatus(cfg.AI.AnthropicEnv))
	default:
		log.Printf("  Env var      : %s %s", cfg.AI.GroqEnv, envStatus(cfg.AI.GroqEnv))
	}

	logTTSConfig(cfg.TTS)

	log.Println("[notifications]")
	log.Printf("  Min priority : %d (1=Info 2=Warning 3=Critical 4=Commander only)", cfg.Notifications.MinPriority)

	log.Println("[colors]")
	log.Printf("  Tolerance    : %.0f", cfg.Colors.Tolerance)
	log.Printf("  Player       : RGB(%.0f, %.0f, %.0f)", cfg.Colors.Player.R, cfg.Colors.Player.G, cfg.Colors.Player.B)
	log.Printf("  Ally         : RGB(%.0f, %.0f, %.0f)", cfg.Colors.Ally.R, cfg.Colors.Ally.G, cfg.Colors.Ally.B)
	log.Printf("  Enemy        : RGB(%.0f, %.0f, %.0f)", cfg.Colors.Enemy.R, cfg.Colors.Enemy.G, cfg.Colors.Enemy.B)
	log.Printf("  Squad        : RGB(%.0f, %.0f, %.0f)", cfg.Colors.Squad.R, cfg.Colors.Squad.G, cfg.Colors.Squad.B)
	log.Println("───────────────────────────────────────────────────────")
}

// openDebugFile creates a JSONL file for raw WT API responses.
// The file is placed in cfg.LogDir when set, otherwise in the working directory.
// The caller is responsible for closing the returned file.
func openDebugFile(cfg config.Config) (*os.File, error) {
	dir := cfg.LogDir
	if dir == "" {
		dir = "."
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create debug dir %q: %w", dir, err)
	}

	name := fmt.Sprintf("wt_debug_%s.jsonl", time.Now().UTC().Format("20060102T150405Z"))
	path := dir + "/" + name

	debugFile, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create debug file: %w", err)
	}

	log.Printf("[debug] raw API data → %s", path)

	return debugFile, nil
}

func runAssistant(cfg config.Config, debug bool) error {
	if !debug {
		cfg.LogDir = ""
	}

	logConfig(cfg)

	wt.SetColors(cfg.Colors)

	state := newAssistantState(cfg)

	if state.cmd == nil && int(cfg.Notifications.MinPriority) >= analyzer.PriorityCommander {
		const red = "\033[31m"
		const reset = "\033[0m"
		fmt.Fprintf(os.Stderr, "%sERROR: min_priority is set to %d (commander only) but no AI API key is configured — no alerts will be delivered.%s\n",
			red, cfg.Notifications.MinPriority, reset)
		return cli.Exit("", 1)
	}

	if debug {
		debugFile, err := openDebugFile(cfg)
		if err != nil {
			return err
		}
		defer closer.Close(debugFile)

		state.client.SetDebugWriter(debugFile)
	}

	speech, err := initTTS(cfg)
	if err != nil {
		return err
	}

	var ttsMu sync.Mutex

	// speak serialises TTS calls so alert and commander voices don't overlap.
	speak := func(msg string) {
		ttsMu.Lock()
		defer ttsMu.Unlock()

		if err := speech.Speak(msg); err != nil {
			log.Printf("TTS error: %v", err)
		}
	}

	log.Println("War Thunder assistant started. Waiting for a match...")

	for {
		time.Sleep(cfg.PollInterval)
		state.pollFrame(speak)
	}
}
