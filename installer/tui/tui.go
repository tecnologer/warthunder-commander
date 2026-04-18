package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tecnologer/warthunder/installer/installer"
	"github.com/tecnologer/warthunder/installer/schema"
)

// fieldGroup holds indices (into schema.Fields) that should be shown together.
type fieldGroup struct {
	indices []int
}

// computeGroups groups consecutive text/password fields (without show_if) that share
// the same parent key prefix into a single step; everything else is a singleton.
func computeGroups(fields []schema.Field) []fieldGroup {
	var groups []fieldGroup
	i := 0
	for i < len(fields) {
		gk := fieldParentKey(fields[i].Key)
		if gk != "" && isGroupableField(fields[i]) {
			j := i + 1
			for j < len(fields) && fieldParentKey(fields[j].Key) == gk && isGroupableField(fields[j]) {
				j++
			}
			if j > i+1 {
				idx := make([]int, j-i)
				for k := range idx {
					idx[k] = i + k
				}
				groups = append(groups, fieldGroup{idx})
				i = j
				continue
			}
		}
		groups = append(groups, fieldGroup{[]int{i}})
		i++
	}
	return groups
}

// fieldParentKey returns everything before the last dot in key, or "" if no dot.
func fieldParentKey(key string) string {
	dot := strings.LastIndex(key, ".")
	if dot < 0 {
		return ""
	}
	return key[:dot]
}

// isGroupableField reports whether f can participate in a multi-field group step.
func isGroupableField(f schema.Field) bool {
	return (f.Type == schema.FieldTypeText || f.Type == schema.FieldTypePassword) && f.ShowIf == nil
}

// formatGroupKey turns "colors.player" into "Colors > Player".
func formatGroupKey(gk string) string {
	parts := strings.Split(gk, ".")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " > ")
}

// Model is the root Bubble Tea model for the installer wizard.
type Model struct {
	schema  *schema.Schema
	version string

	step            step
	groups          []fieldGroup
	groupIndex      int // which group we are on (during stepConfigFields)
	groupInputFocus int // which input within the current group has focus
	inputs          []textinput.Model
	selectIndex     int // cursor for select-type fields
	installDir      textinput.Model
	configValues    map[string]string
	boolValues      map[string]bool // for type:bool fields

	release    *installer.Release
	tmpBinPath string
	binaryPath string
	configPath string

	spinner  spinner.Model
	progress progress.Model

	dlWritten int64
	dlTotal   int64

	errMsg string
	width  int
}

// New creates a new installer Model seeded with the given schema and installer version.
func New(sch *schema.Schema, version string) Model {
	newSpinner := spinner.New()
	newSpinner.Spinner = spinner.Dot
	newSpinner.Style = stylePrimary

	prog := progress.New(progress.WithDefaultGradient())

	installDir := textinput.New()
	installDir.Placeholder = installer.DefaultInstallDir()
	installDir.SetValue(installer.DefaultInstallDir())
	installDir.Focus()

	// Build one textinput per schema field.
	inputs := make([]textinput.Model, len(sch.Fields))
	boolValues := map[string]bool{}

	for i, field := range sch.Fields {
		textInput := textinput.New()
		textInput.Placeholder = field.Default

		if field.Default != "" {
			textInput.SetValue(field.Default)
		}

		if field.Type == schema.FieldTypePassword {
			textInput.EchoMode = textinput.EchoPassword
		}

		if field.Type == schema.FieldTypeBool {
			boolValues[field.Key] = field.Default == "true"
		}

		inputs[i] = textInput
	}

	return Model{
		schema:       sch,
		version:      version,
		step:         stepWelcome,
		groups:       computeGroups(sch.Fields),
		inputs:       inputs,
		installDir:   installDir,
		configValues: map[string]string{},
		boolValues:   boolValues,
		spinner:      newSpinner,
		progress:     prog,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.progress.Width = msg.Width - 8
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case msgProgress:
		m.dlWritten = msg.written
		m.dlTotal = msg.total

		if m.dlTotal > 0 {
			cmd := m.progress.SetPercent(float64(m.dlWritten) / float64(m.dlTotal))
			return m, cmd
		}

		return m, nil
	case msgDownloadDone:
		m.tmpBinPath = msg.tmpPath
		m.step = stepInstalling

		return m, m.cmdInstall()
	case msgInstallDone:
		m.binaryPath = msg.binaryPath
		m.configPath = msg.configPath
		m.step = stepDone

		return m, nil
	case msgErr:
		m.errMsg = msg.err.Error()
		m.step = stepError

		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	case progress.FrameMsg:
		pm, cmd := m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global cancel: Esc or Ctrl+C on any step
	if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.step {
	case stepWelcome:
		switch {
		case msg.String() == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyEnter:
			m.step = stepInstallDir
			m.installDir.Focus()
		}

	case stepInstallDir:
		switch msg.Type {
		case tea.KeyEnter:
			if m.installDir.Value() == "" {
				m.installDir.SetValue(installer.DefaultInstallDir())
			}

			m.installDir.Blur()

			if len(m.groups) > 0 {
				m.step = stepConfigFields
				m.groupIndex = 0
				m.groupInputFocus = 0
				for m.groupIndex < len(m.groups) && !m.isGroupVisible(m.groupIndex) {
					m.groupIndex++
				}
				if m.groupIndex >= len(m.groups) {
					m.step = stepConfirm
				} else {
					m.focusCurrentField()
				}
			} else {
				m.step = stepConfirm
			}
		default:
			var cmd tea.Cmd
			m.installDir, cmd = m.installDir.Update(msg)
			return m, cmd
		}

	case stepConfigFields:
		g := m.groups[m.groupIndex]

		if len(g.indices) > 1 {
			return m.handleGroupKey(msg, g)
		}

		// Singleton group
		fieldIdx := g.indices[0]
		field := m.schema.Fields[fieldIdx]

		switch field.Type {
		case schema.FieldTypeBool:
			switch msg.String() {
			case " ", "enter":
				m.boolValues[field.Key] = !m.boolValues[field.Key]
				if msg.Type == tea.KeyEnter {
					return m, m.advanceGroup()
				}
			}

		case schema.FieldTypeSelect:
			switch msg.Type {
			case tea.KeyUp:
				if m.selectIndex > 0 {
					m.selectIndex--
				}
			case tea.KeyDown:
				if m.selectIndex < len(field.Options)-1 {
					m.selectIndex++
				}
			case tea.KeyEnter:
				m.configValues[field.Key] = field.Options[m.selectIndex]
				return m, m.advanceGroup()
			}

		default: // text / password
			switch msg.Type {
			case tea.KeyEnter:
				val := m.inputs[fieldIdx].Value()
				if val == "" {
					val = field.Default
				}

				if field.Required && val == "" {
					return m, nil
				}

				m.configValues[field.Key] = val

				return m, m.advanceGroup()
			default:
				var cmd tea.Cmd
				m.inputs[fieldIdx], cmd = m.inputs[fieldIdx].Update(msg)
				return m, cmd
			}
		}

	case stepConfirm:
		switch msg.String() {
		case "y", "Y", "enter":
			m.step = stepDownloading
			return m, m.cmdResolveAndDownload()
		case "n", "N", "q":
			return m, tea.Quit
		}

	case stepDownloading, stepInstalling:
		if msg.String() == "q" {
			return m, tea.Quit
		}

	case stepDone, stepError:
		return m, tea.Quit
	}

	return m, nil
}

// handleGroupKey handles key events when the current group has multiple inputs.
func (m Model) handleGroupKey(msg tea.KeyMsg, g fieldGroup) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		next := m.groupInputFocus + 1
		if next < len(g.indices) {
			m.groupInputFocus = next
			m.focusCurrentField()
		}
		return m, nil

	case tea.KeyShiftTab, tea.KeyUp:
		if m.groupInputFocus > 0 {
			m.groupInputFocus--
			m.focusCurrentField()
		}
		return m, nil

	case tea.KeyEnter:
		for _, idx := range g.indices {
			field := m.schema.Fields[idx]
			val := m.inputs[idx].Value()
			if val == "" {
				val = field.Default
			}
			if field.Required && val == "" {
				return m, nil
			}
			m.configValues[field.Key] = val
		}
		return m, m.advanceGroup()

	default:
		focusedIdx := g.indices[m.groupInputFocus]
		var cmd tea.Cmd
		m.inputs[focusedIdx], cmd = m.inputs[focusedIdx].Update(msg)
		return m, cmd
	}
}

// isFieldVisible reports whether the field at index i should be shown.
func (m *Model) isFieldVisible(i int) bool {
	f := m.schema.Fields[i]
	if f.ShowIf == nil {
		return true
	}
	val, ok := m.configValues[f.ShowIf.Key]
	if !ok {
		return false
	}
	for _, v := range f.ShowIf.Values {
		if v == val {
			return true
		}
	}
	return false
}

// isGroupVisible reports whether at least one field in the group is visible.
func (m *Model) isGroupVisible(gi int) bool {
	for _, idx := range m.groups[gi].indices {
		if m.isFieldVisible(idx) {
			return true
		}
	}
	return false
}

// visibleGroupCount returns how many groups are currently visible.
func (m *Model) visibleGroupCount() int {
	n := 0
	for i := range m.groups {
		if m.isGroupVisible(i) {
			n++
		}
	}
	return n
}

// visibleGroupPos returns the 1-based position of groupIndex among visible groups.
func (m *Model) visibleGroupPos() int {
	pos := 0
	for i := 0; i <= m.groupIndex; i++ {
		if m.isGroupVisible(i) {
			pos++
		}
	}
	return pos
}

// advanceGroup moves to the next group or to the confirm step.
func (m *Model) advanceGroup() tea.Cmd {
	for k, v := range m.boolValues {
		if v {
			m.configValues[k] = "true"
		} else {
			m.configValues[k] = "false"
		}
	}

	m.groupIndex++
	m.groupInputFocus = 0
	m.selectIndex = 0

	for m.groupIndex < len(m.groups) && !m.isGroupVisible(m.groupIndex) {
		m.groupIndex++
	}

	if m.groupIndex >= len(m.groups) {
		m.step = stepConfirm
		return nil
	}

	m.focusCurrentField()
	return nil
}

func (m *Model) focusCurrentField() {
	if m.groupIndex >= len(m.groups) {
		return
	}
	g := m.groups[m.groupIndex]
	for i, idx := range g.indices {
		field := m.schema.Fields[idx]
		if field.Type == schema.FieldTypeText || field.Type == schema.FieldTypePassword {
			if i == m.groupInputFocus {
				m.inputs[idx].Focus()
			} else {
				m.inputs[idx].Blur()
			}
		}
	}
}

func (m Model) cmdResolveAndDownload() tea.Cmd {
	return func() tea.Msg {
		rel, err := installer.Resolve(m.schema.GithubRepo, m.schema.BinaryName, m.version)
		if err != nil {
			return msgErr{err}
		}

		tmpPath, err := installer.DownloadBinary(rel, func(written, total int64) {
			_ = written
			_ = total
		})
		if err != nil {
			return msgErr{err}
		}

		return msgDownloadDone{tmpPath: tmpPath}
	}
}

func (m Model) cmdInstall() tea.Cmd {
	return func() tea.Msg {
		binPath, err := installer.InstallBinary(m.tmpBinPath, m.installDir.Value(), m.schema.BinaryName)
		if err != nil {
			return msgErr{fmt.Errorf("installing binary: %w", err)}
		}

		tomlContent := installer.BuildTOML(m.configValues)

		cfgPath, err := installer.WriteConfig(m.installDir.Value(), m.schema.AppName+".toml", tomlContent)
		if err != nil {
			return msgErr{fmt.Errorf("writing config: %w", err)}
		}

		return msgInstallDone{binaryPath: binPath, configPath: cfgPath}
	}
}

func (m Model) View() string {
	switch m.step {
	case stepWelcome:
		return m.viewWelcome()
	case stepInstallDir:
		return m.viewInstallDir()
	case stepConfigFields:
		return m.viewConfigField()
	case stepConfirm:
		return m.viewConfirm()
	case stepDownloading:
		return m.viewDownloading()
	case stepInstalling:
		return m.viewInstalling()
	case stepDone:
		return m.viewDone()
	case stepError:
		return m.viewError()
	}
	return ""
}

func (m Model) viewWelcome() string {
	title := stylePrimary.Render(fmt.Sprintf("  %s Installer", m.schema.AppName))
	versionLine := styleSubtle.Render(fmt.Sprintf("  version %s", m.version))
	body := styleBox.Render(
		title + "\n" + versionLine + "\n\n" +
			"This wizard will guide you through:\n\n" +
			"  • Choosing an install directory\n" +
			"  • Setting up your configuration file\n" +
			"  • Downloading and installing the latest binary\n\n" +
			styleSubtle.Render("Press Enter to continue  •  q to quit"),
	)
	return "\n" + body + "\n"
}

func (m Model) viewInstallDir() string {
	return m.viewStep(
		"Install Directory",
		"Where should the binary be installed?",
		m.installDir.View(),
		"Press Enter to confirm  •  Esc to cancel",
	)
}

func (m Model) viewConfigField() string {
	g := m.groups[m.groupIndex]

	progress := styleSubtle.Render(
		fmt.Sprintf("Step %d of %d", m.visibleGroupPos(), m.visibleGroupCount()),
	)

	if len(g.indices) > 1 {
		return m.viewMultiFieldGroup(g, progress)
	}

	return m.viewSingleField(g.indices[0], progress)
}

func (m Model) viewSingleField(fieldIdx int, progressLine string) string {
	field := m.schema.Fields[fieldIdx]

	var inputView string
	switch field.Type {
	case schema.FieldTypeBool:
		checked := "[ ]"
		if m.boolValues[field.Key] {
			checked = styleSuccess.Render("[✓]")
		}

		inputView = fmt.Sprintf("%s %s", checked, field.Label) +
			"\n" + styleSubtle.Render("Space to toggle  •  Enter to confirm  •  Esc to cancel")

	case schema.FieldTypeSelect:
		var lines []string

		for i, opt := range field.Options {
			if i == m.selectIndex {
				lines = append(lines, stylePrimary.Render("▶ "+opt))
			} else {
				lines = append(lines, styleSubtle.Render("▷ "+opt))
			}
		}

		inputView = strings.Join(lines, "\n") +
			"\n\n" + styleSubtle.Render("↑/↓ to select  •  Enter to confirm  •  Esc to cancel")

	default:
		inputView = m.inputs[fieldIdx].View()
	}

	hint := ""
	if field.Description != "" {
		hint = "\n" + styleSubtle.Render(field.Description)
	}
	if field.Required {
		hint += " " + styleWarning.Render("(required)")
	}

	return m.viewStep(
		field.Label,
		progressLine+hint,
		inputView,
		"",
	)
}

func (m Model) viewMultiFieldGroup(g fieldGroup, progressLine string) string {
	firstField := m.schema.Fields[g.indices[0]]
	gk := fieldParentKey(firstField.Key)
	title := formatGroupKey(gk)

	var rows []string
	for i, idx := range g.indices {
		field := m.schema.Fields[idx]
		label := styleDimLabel.Render(field.Label + ":")
		inputStr := m.inputs[idx].View()
		if i == m.groupInputFocus {
			rows = append(rows, label+" "+inputStr)
		} else {
			rows = append(rows, label+" "+inputStr)
		}
	}

	inputArea := strings.Join(rows, "\n")
	controls := styleSubtle.Render("Tab/↑↓ to switch fields  •  Enter to confirm  •  Esc to cancel")

	return m.viewStep(
		title,
		progressLine,
		inputArea+"\n\n"+controls,
		"",
	)
}

func (m Model) viewConfirm() string {
	var lines []string

	lines = append(lines,
		styleBold.Render("Review your settings:"),
		"",
		styleDimLabel.Render("Install directory:"),
		"  "+m.installDir.Value(),
	)

	if len(m.configValues) > 0 {
		lines = append(lines, "", styleDimLabel.Render("Configuration:"))

		keys := make([]string, 0, len(m.configValues))
		for k := range m.configValues {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			value := m.configValues[key]
			display := value
			for _, f := range m.schema.Fields {
				if f.Key == key && f.Type == schema.FieldTypePassword {
					display = strings.Repeat("•", len(value))
				}
			}
			lines = append(lines, fmt.Sprintf("  %s = %s", styleDimLabel.Render(key), display))
		}
	}

	lines = append(lines, "", styleSubtle.Render("Press Enter or y to install  •  n/q to cancel"))

	body := styleBox.Render(strings.Join(lines, "\n"))

	return "\n" + styleStepTitle.Render("Confirm Installation") + "\n" + body + "\n"
}

func (m Model) viewDownloading() string {
	var pct string
	if m.dlTotal > 0 {
		pct = fmt.Sprintf(" %.0f%%", float64(m.dlWritten)/float64(m.dlTotal)*100)
	}
	return "\n" + m.spinner.View() +
		stylePrimary.Render(fmt.Sprintf("  Downloading latest %s release%s…", m.schema.AppName, pct)) +
		"\n\n" + styleSubtle.Render("  This may take a moment") + "\n"
}

func (m Model) viewInstalling() string {
	return "\n" + m.spinner.View() +
		stylePrimary.Render("  Installing…") + "\n"
}

func (m Model) viewDone() string {
	body := styleBox.Render(
		styleSuccess.Render("✓  Installation complete!") + "\n\n" +
			styleDimLabel.Render("Binary:") + "\n  " + m.binaryPath + "\n\n" +
			styleDimLabel.Render("Config:") + "\n  " + m.configPath + "\n\n" +
			styleSubtle.Render("Make sure "+m.installDir.Value()+" is in your PATH.\n") +
			styleSubtle.Render("Press any key to exit."),
	)
	return "\n" + body + "\n"
}

func (m Model) viewError() string {
	body := styleBox.Render(
		styleError.Render("✗  Installation failed") + "\n\n" +
			m.errMsg + "\n\n" +
			styleSubtle.Render("Press any key to exit."),
	)
	return "\n" + body + "\n"
}

// viewStep is a generic layout for wizard steps with a title, subtitle, input area, and hint.
func (m Model) viewStep(title, subtitle, input, hint string) string {
	var builder strings.Builder

	builder.WriteString("\n")
	builder.WriteString(styleStepTitle.Render(title))
	builder.WriteString("\n")

	if subtitle != "" {
		builder.WriteString(styleSubtle.Render(subtitle))
		builder.WriteString("\n\n")
	}

	builder.WriteString("  ")
	builder.WriteString(input)
	builder.WriteString("\n")

	if hint != "" {
		builder.WriteString("\n")
		builder.WriteString(styleSubtle.Render("  " + hint))
		builder.WriteString("\n")
	}
	return builder.String()
}
