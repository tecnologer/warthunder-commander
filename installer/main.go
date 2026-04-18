package main

import (
	"embed"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tecnologer/warthunder/installer/schema"
	"github.com/tecnologer/warthunder/installer/tui"
)

//go:embed schema.yaml
var embeddedFS embed.FS

var version = "dev"

func main() {
	flag.Parse()

	var (
		err error
	)

	data, readErr := embeddedFS.ReadFile("schema.yaml")
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "Error reading embedded schema: %v\n", readErr)
		os.Exit(1)
	}

	schemaFields, err := schema.LoadBytes(data)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading schema: %v\n", err)
		os.Exit(1)
	}

	m := tui.New(schemaFields, version)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running installer: %v\n", err)
		os.Exit(1)
	}
}
