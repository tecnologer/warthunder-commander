package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/tecnologer/warthunder/internal/analyzer"
	"github.com/tecnologer/warthunder/internal/collector"
	"github.com/tecnologer/warthunder/internal/commander"
	"github.com/tecnologer/warthunder/internal/config"
	"github.com/tecnologer/warthunder/internal/lang"
	"github.com/tecnologer/warthunder/internal/tts"
	"github.com/tecnologer/warthunder/internal/tts/camb"
	"github.com/tecnologer/warthunder/internal/wt"
	"github.com/urfave/cli/v2"
)

// version is set at build time via -ldflags "-X main.version=<version>".
var version = "dev"

const (
	pollInterval      = 500 * time.Millisecond
	alertCooldown     = 4 * time.Second
	commanderInterval = 30 * time.Second
	minConfirmFrames  = 6 // 3 s at 500 ms — avoids reacting to transient loading states
)

func main() {
	cfg, err := config.Load("config.toml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := &cli.App{
		Name:    "warthunder",
		Usage:   "War Thunder combat assistant",
		Version: version,
		Action: func(_ *cli.Context) error {
			return runAssistant(cfg)
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

// dispatchAlert fires a TTS alert if one is ready and the cooldown has elapsed.
// Returns the updated lastAlert time.
func dispatchAlert(alert *analyzer.Alert, lastAlert time.Time, speak func(string)) time.Time {
	if alert == nil || time.Since(lastAlert) < alertCooldown {
		return lastAlert
	}

	log.Printf("[priority %d] %s", alert.Priority, alert.Message)

	go speak(alert.Message)

	return time.Now()
}

// invokeCommander cancels any in-flight commander call, then starts a new one.
// It returns the new cancel func so the caller can replace cmdCancel.
func invokeCommander(
	cmd *commander.Commander,
	col *collector.Collector,
	client *wt.Client,
	speak func(string),
	oldCancel context.CancelFunc,
) context.CancelFunc {
	oldCancel()

	mapInfo, _ := client.MapInfo()
	sum := col.Summary()

	ctx, cancel := context.WithCancel(context.Background())
	go func(ctx context.Context, sum *collector.Summary, info *wt.MapInfo) {
		report, err := cmd.Advise(ctx, sum, info)
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
	client        *wt.Client
	alerter       *analyzer.Analyzer
	col           *collector.Collector
	cmd           *commander.Commander
	language      lang.Language
	inMatch       bool
	confirmFrames int
	cmdCancel     context.CancelFunc
	lastAlert     time.Time
	lastCommand   time.Time
}

func newAssistantState(cfg config.Config) *assistantState {
	language := lang.Parse(cfg.Language)

	return &assistantState{
		client:    wt.NewClient(),
		alerter:   analyzer.New(language),
		col:       collector.New(commanderInterval),
		cmd:       commander.New(cfg.AI, language),
		language:  language,
		cmdCancel: func() {},
	}
}

func (s *assistantState) reset() {
	s.inMatch = false
	s.confirmFrames = 0

	s.cmdCancel()

	s.alerter = analyzer.New(s.language)
	s.col = collector.New(commanderInterval)
	s.lastAlert = time.Time{}
	s.lastCommand = time.Time{}
}

func (s *assistantState) pollFrame(speak func(string)) {
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

	if !s.inMatch {
		s.confirmFrames++
		if s.confirmFrames < minConfirmFrames {
			return
		}

		s.inMatch = true
		s.lastCommand = time.Now()

		log.Println("Match started.")
	}

	s.col.Add(objs)

	mode := s.client.GameMode()

	alert := s.alerter.Analyze(objs, mode)
	s.lastAlert = dispatchAlert(alert, s.lastAlert, speak)

	if time.Since(s.lastCommand) >= commanderInterval {
		s.lastCommand = time.Now()
		s.cmdCancel = invokeCommander(s.cmd, s.col, s.client, speak, s.cmdCancel)
	}
}

func runAssistant(cfg config.Config) error {
	wt.SetColors(cfg.Colors)

	state := newAssistantState(cfg)

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
		time.Sleep(pollInterval)
		state.pollFrame(speak)
	}
}
