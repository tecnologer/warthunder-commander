package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tecnologer/warthunder/installer/schema"
	"github.com/tecnologer/warthunder/installer/tui"
)

//go:embed schema.yaml
var embeddedFS embed.FS

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	defer dumpOnPanic()

	flag.Parse()

	data, err := embeddedFS.ReadFile("schema.yaml")
	if err != nil {
		return fmt.Errorf("reading embedded schema: %w", err)
	}

	schemaFields, err := schema.LoadBytes(data)
	if err != nil {
		return fmt.Errorf("loading schema: %w", err)
	}

	model := tui.New(schemaFields, version)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running installer: %w", err)
	}

	return nil
}

func dumpOnPanic() {
	recovered := recover()
	if recovered == nil {
		return
	}

	stack := debug.Stack()
	filename := fmt.Sprintf("panic_%s.txt", time.Now().Format("20060102_150405"))

	f, createErr := os.Create(filename)
	if createErr == nil {
		_, _ = fmt.Fprintf(f, "panic: %v\n\n%s\n", recovered, stack)
		_ = f.Close()
		_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n\nStack trace written to %s\n", recovered, filename)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n\n%s\n", recovered, stack)
	}

	os.Exit(2)
}
