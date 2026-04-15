// Package matchlog writes per-match debug logs so that incorrect alerts and
// commander reports can be reproduced offline.
package matchlog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Logger writes match events to a timestamped file in logDir.
// A nil Logger is valid — all methods are no-ops.
type Logger struct {
	file *os.File
}

// New creates a new log file inside logDir named by the current timestamp.
// If logDir is empty or the file cannot be created, New logs a warning and
// returns nil (the caller continues without logging).
func New(logDir string) *Logger {
	if logDir == "" {
		return nil
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Printf("[matchlog] cannot create log directory %q: %v", logDir, err)
		return nil
	}

	name := filepath.Join(logDir, time.Now().Format("2006-01-02T15-04-05")+".log")

	file, err := os.Create(name)
	if err != nil {
		log.Printf("[matchlog] cannot create log file %q: %v", name, err)
		return nil
	}

	log.Printf("[matchlog] logging match to %s", name)

	return &Logger{file: file}
}

// MatchStart writes the match-start header.
func (l *Logger) MatchStart(mapName string) {
	if l == nil {
		return
	}

	l.writef("=== MATCH START %s  map=%q ===\n", time.Now().Format(time.RFC3339), mapName)
}

// MatchEnd writes the match-end footer and closes the file.
func (l *Logger) MatchEnd() {
	if l == nil {
		return
	}

	l.writef("=== MATCH END %s ===\n", time.Now().Format(time.RFC3339))
	_ = l.file.Close()
}

// Alert logs a fired alert.
func (l *Logger) Alert(priority int, message string) {
	if l == nil {
		return
	}

	l.writef("[ALERT p%d] %s  %s\n", priority, time.Now().Format("15:04:05.000"), message)
}

// CommanderPrompt logs the prompt sent to the LLM and its response.
func (l *Logger) CommanderPrompt(prompt, response string) {
	if l == nil {
		return
	}

	l.writef("[COMMANDER] %s\n--- prompt ---\n%s\n--- response ---\n%s\n--------------\n",
		time.Now().Format("15:04:05.000"), prompt, response)
}

func (l *Logger) writef(format string, args ...any) {
	if _, err := fmt.Fprintf(l.file, format, args...); err != nil {
		log.Printf("[matchlog] write error: %v", err)
	}
}
