package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tecnologer/warthunder/installer/installer"
	"github.com/tecnologer/warthunder/installer/schema"
)

const (
	labelColWidth = 28 // fixed label column width for form alignment
	rgbColWidth   = 10 // label column for RGB groups
	inputWidth    = 28 // textinput width for regular fields
	rgbInputWidth = 5  // textinput width for R/G/B sub-inputs

	// buffer enough queued progress updates to avoid stalling download IO.
	downloadMsgBufferSize = 128
)

// Model is the root Bubble Tea model for the installer wizard.
type Model struct {
	schema  *schema.Schema
	version string

	step       step
	sections   []section
	sectionIdx int // current section (during stepConfigFields)
	fieldIdx   int // focused field group within current section
	subFocus   int // for kindRGB: 0=R, 1=G, 2=B

	inputs      []textinput.Model
	selectIdxes []int // per-field select cursor position
	boolValues  map[string]bool

	installDir   textinput.Model
	configValues map[string]string

	// env var collection
	envVarFieldIndices []int
	envVarIndex        int
	envVarValues       map[string]string
	envVarInput        textinput.Model

	tmpBinPath string
	binaryPath string
	configPath string
	envPath    string

	spinner  spinner.Model
	progress progress.Model

	dlWritten int64
	dlTotal   int64
	dlMsgs    <-chan tea.Msg

	errMsg string
	width  int
}

// New creates a new installer Model seeded with the given schema and installer version.
func New(sch *schema.Schema, version string) *Model {
	newSpinner := spinner.New()
	newSpinner.Spinner = spinner.Dot
	newSpinner.Style = stylePrimary

	prog := progress.New(progress.WithDefaultGradient())

	installDir := textinput.New()
	installDir.Placeholder = installer.DefaultInstallDir()
	installDir.SetValue(installer.DefaultInstallDir())
	installDir.Focus()

	inputs := make([]textinput.Model, len(sch.Fields))
	selectIdxes := make([]int, len(sch.Fields))
	boolValues := map[string]bool{}
	configValues := map[string]string{}

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

		if field.Type == schema.FieldTypeSelect {
			for j, opt := range field.Options {
				if opt == field.Default {
					selectIdxes[i] = j

					break
				}
			}

			// Pre-populate so show_if conditions work from the first render.
			if field.Default != "" {
				configValues[field.Key] = field.Default
			} else if len(field.Options) > 0 {
				configValues[field.Key] = field.Options[0]
			}
		}

		if isRGBKey(field.Key) {
			textInput.Width = rgbInputWidth
		} else {
			textInput.Width = inputWidth
		}

		inputs[i] = textInput
	}

	envVarInput := textinput.New()
	envVarInput.EchoMode = textinput.EchoPassword
	envVarInput.Placeholder = "enter value…"

	return &Model{
		schema:       sch,
		version:      version,
		step:         stepWelcome,
		sections:     computeSections(sch.Fields),
		inputs:       inputs,
		selectIdxes:  selectIdxes,
		installDir:   installDir,
		configValues: configValues,
		boolValues:   boolValues,
		envVarValues: map[string]string{},
		envVarInput:  envVarInput,
		spinner:      newSpinner,
		progress:     prog,
	}
}

// isRGBKey returns true for keys whose last component is "r", "g", or "b".
func isRGBKey(key string) bool {
	last := key[strings.LastIndex(key, ".")+1:]

	return last == "r" || last == "g" || last == "b"
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		next := cmdWaitDownloadMsg(m.dlMsgs)
		if m.dlTotal > 0 {
			cmd := m.progress.SetPercent(float64(m.dlWritten) / float64(m.dlTotal))

			return m, tea.Batch(next, cmd)
		}

		return m, next
	case msgDownloadDone:
		m.dlMsgs = nil
		m.tmpBinPath = msg.tmpPath
		m.step = stepInstalling

		return m, m.cmdInstall()
	case msgInstallDone:
		m.binaryPath = msg.binaryPath
		m.configPath = msg.configPath
		m.envPath = msg.envPath
		m.step = stepDone

		return m, nil
	case msgErr:
		m.dlMsgs = nil
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

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

			if len(m.sections) > 0 {
				m.step = stepConfigFields
				m.enterFirstVisibleSection()
			} else {
				m.step = stepConfirm
			}
		default:
			var cmd tea.Cmd

			m.installDir, cmd = m.installDir.Update(msg)

			return m, cmd
		}
	case stepConfigFields:
		return m.handleSectionKey(msg)
	case stepEnvVarPrompt:
		switch msg.String() {
		case "y", "Y":
			m.envVarInput.Reset()
			m.envVarInput.Focus()
			m.step = stepEnvVarValue
		case "n", "N", "enter":
			m.advanceEnvVar()
			return m, nil
		}

	case stepEnvVarValue:
		switch msg.Type {
		case tea.KeyEnter:
			if val := m.envVarInput.Value(); val != "" {
				m.envVarValues[m.currentEnvVarName()] = val
			}

			m.advanceEnvVar()

			return m, nil
		default:
			var cmd tea.Cmd

			m.envVarInput, cmd = m.envVarInput.Update(msg)

			return m, cmd
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

// handleSectionKey handles all input while the wizard is on a config section screen.
func (m *Model) handleSectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sec := m.sections[m.sectionIdx]
	group := sec.groups[m.fieldIdx]

	switch msg.Type {
	case tea.KeyTab:
		if group.kind == kindRGB && m.subFocus < 2 {
			m.subFocus++
			m.refocusSection()

			return m, nil
		}

		m.subFocus = 0

		return m, m.moveToNextField()
	case tea.KeyShiftTab:
		if group.kind == kindRGB && m.subFocus > 0 {
			m.subFocus--
			m.refocusSection()

			return m, nil
		}

		m.subFocus = 0
		m.moveToPrevField()

		return m, nil
	case tea.KeyUp:
		if group.kind == kindSingle {
			idx := group.indices[0]

			f := m.schema.Fields[idx]
			if f.Type == schema.FieldTypeSelect {
				if m.selectIdxes[idx] > 0 {
					m.selectIdxes[idx]--
					m.configValues[f.Key] = f.Options[m.selectIdxes[idx]]
				}

				return m, nil
			}
		}

		m.subFocus = 0
		m.moveToPrevField()

		return m, nil
	case tea.KeyDown:
		if group.kind == kindSingle {
			idx := group.indices[0]

			field := m.schema.Fields[idx]
			if field.Type == schema.FieldTypeSelect {
				if m.selectIdxes[idx] < len(field.Options)-1 {
					m.selectIdxes[idx]++
					m.configValues[field.Key] = field.Options[m.selectIdxes[idx]]
				}

				return m, nil
			}
		}

		m.subFocus = 0

		return m, m.moveToNextField()
	case tea.KeyLeft:
		if group.kind == kindSingle {
			idx := group.indices[0]

			field := m.schema.Fields[idx]
			if field.Type == schema.FieldTypeSelect && m.selectIdxes[idx] > 0 {
				m.selectIdxes[idx]--
				m.configValues[field.Key] = field.Options[m.selectIdxes[idx]]
			}
		}

		return m, nil
	case tea.KeyRight:
		if group.kind == kindSingle {
			idx := group.indices[0]

			f := m.schema.Fields[idx]
			if f.Type == schema.FieldTypeSelect && m.selectIdxes[idx] < len(f.Options)-1 {
				m.selectIdxes[idx]++
				m.configValues[f.Key] = f.Options[m.selectIdxes[idx]]
			}
		}

		return m, nil
	case tea.KeySpace:
		if group.kind == kindSingle {
			idx := group.indices[0]

			field := m.schema.Fields[idx]
			if field.Type == schema.FieldTypeBool {
				m.boolValues[field.Key] = !m.boolValues[field.Key]

				return m, nil
			}

			if field.Type == schema.FieldTypeText || field.Type == schema.FieldTypePassword {
				var cmd tea.Cmd

				m.inputs[idx], cmd = m.inputs[idx].Update(msg)

				return m, cmd
			}
		}
	case tea.KeyEnter:
		return m, m.handleEnter()
	default:
		switch group.kind {
		case kindSingle:
			idx := group.indices[0]

			f := m.schema.Fields[idx]
			if f.Type == schema.FieldTypeText || f.Type == schema.FieldTypePassword {
				var cmd tea.Cmd

				m.inputs[idx], cmd = m.inputs[idx].Update(msg)

				return m, cmd
			}
		case kindRGB:
			idx := group.indices[m.subFocus]

			var cmd tea.Cmd

			m.inputs[idx], cmd = m.inputs[idx].Update(msg)

			return m, cmd
		}
	}

	return m, nil
}

// handleEnter confirms the focused field and moves forward within the section or to the next section.
func (m *Model) handleEnter() tea.Cmd {
	var (
		sec   = m.sections[m.sectionIdx]
		group = sec.groups[m.fieldIdx]
	)

	switch group.kind {
	case kindRGB:
		var (
			idx   = group.indices[m.subFocus]
			field = m.schema.Fields[idx]
			val   = m.inputs[idx].Value()
		)

		if val == "" {
			val = field.Default
		}

		m.configValues[field.Key] = val

		if m.subFocus < 2 {
			m.subFocus++
			m.refocusSection()

			return nil
		}
		// Confirmed last sub-input; flush all three and advance.
		for _, i := range group.indices {
			field := m.schema.Fields[i]

			v := m.inputs[i].Value()
			if v == "" {
				v = field.Default
			}

			m.configValues[field.Key] = v
		}

		m.subFocus = 0

		return m.moveToNextField()
	case kindSingle:
		idx := group.indices[0]
		field := m.schema.Fields[idx]

		switch field.Type {
		case schema.FieldTypeBool:
			if m.boolValues[field.Key] {
				m.configValues[field.Key] = "true"
			} else {
				m.configValues[field.Key] = "false"
			}
		case schema.FieldTypeSelect:
			m.configValues[field.Key] = field.Options[m.selectIdxes[idx]]
		default:
			val := m.inputs[idx].Value()
			if val == "" {
				val = field.Default
			}

			if field.Required && val == "" {
				return nil
			}

			m.configValues[field.Key] = val
		}

		return m.moveToNextField()
	}

	return nil
}

func (m *Model) moveToNextField() tea.Cmd {
	sec := m.sections[m.sectionIdx]

	for i := m.fieldIdx + 1; i < len(sec.groups); i++ {
		if m.isGroupVisible(sec, i) {
			m.fieldIdx = i
			m.subFocus = 0
			m.refocusSection()

			return nil
		}
	}

	return m.advanceSection()
}

func (m *Model) moveToPrevField() {
	sec := m.sections[m.sectionIdx]

	for i := m.fieldIdx - 1; i >= 0; i-- {
		if m.isGroupVisible(sec, i) {
			m.fieldIdx = i
			m.subFocus = 0
			m.refocusSection()

			return
		}
	}
}

func (m *Model) advanceSection() tea.Cmd {
	for key, value := range m.boolValues {
		if value {
			m.configValues[key] = "true"
			continue
		}

		m.configValues[key] = "false"
	}

	for i := m.sectionIdx + 1; i < len(m.sections); i++ {
		if m.isSectionVisible(i) {
			m.sectionIdx = i
			m.fieldIdx = 0
			m.subFocus = 0
			m.enterFirstVisibleField()
			m.refocusSection()

			return nil
		}
	}

	return m.enterEnvVarPhase()
}

func (m *Model) enterFirstVisibleSection() {
	for i := range m.sections {
		if m.isSectionVisible(i) {
			m.sectionIdx = i
			m.fieldIdx = 0
			m.subFocus = 0
			m.enterFirstVisibleField()
			m.refocusSection()

			return
		}
	}

	m.step = stepConfirm
}

func (m *Model) enterFirstVisibleField() {
	sec := m.sections[m.sectionIdx]

	for gi := range sec.groups {
		if m.isGroupVisible(sec, gi) {
			m.fieldIdx = gi

			return
		}
	}
}

// refocusSection updates Focus/Blur on all text inputs so that only the
// currently focused field (and sub-input for RGB) has an active cursor.
func (m *Model) refocusSection() {
	if m.sectionIdx >= len(m.sections) {
		return
	}

	sec := m.sections[m.sectionIdx]

	for i, group := range sec.groups {
		for subIdx, schIdx := range group.indices {
			f := m.schema.Fields[schIdx]
			if f.Type != schema.FieldTypeText && f.Type != schema.FieldTypePassword {
				continue
			}

			if i == m.fieldIdx {
				if group.kind == kindRGB {
					if subIdx == m.subFocus {
						m.inputs[schIdx].Focus()
					} else {
						m.inputs[schIdx].Blur()
					}
				} else {
					m.inputs[schIdx].Focus()
				}
			} else {
				m.inputs[schIdx].Blur()
			}
		}
	}
}

func (m *Model) isFieldVisible(i int) bool {
	field := m.schema.Fields[i]
	if field.ShowIf == nil {
		return true
	}

	val, ok := m.configValues[field.ShowIf.Key]
	if !ok {
		for _, field := range m.schema.Fields {
			if field.Key == field.ShowIf.Key {
				val = field.Default

				break
			}
		}
	}

	return slices.Contains(field.ShowIf.Values, val)
}

func (m *Model) isGroupVisible(sec section, gi int) bool {
	return slices.ContainsFunc(sec.groups[gi].indices, m.isFieldVisible)
}

func (m *Model) isSectionVisible(si int) bool {
	sec := m.sections[si]

	for gi := range sec.groups {
		if m.isGroupVisible(sec, gi) {
			return true
		}
	}

	return false
}

func (m *Model) enterEnvVarPhase() tea.Cmd {
	m.envVarFieldIndices = m.visibleEnvVarFieldIndices()
	if len(m.envVarFieldIndices) == 0 {
		m.step = stepConfirm

		return nil
	}

	m.envVarIndex = 0
	m.step = stepEnvVarPrompt

	return nil
}

func (m *Model) visibleEnvVarFieldIndices() []int {
	var result []int

	for i, f := range m.schema.Fields {
		if f.EnvVar && m.isFieldVisible(i) {
			result = append(result, i)
		}
	}

	return result
}

// envVarPlaceholders returns the env var names for all visible env-var fields,
// used to write commented-out stubs in the .env for vars the user skipped.
func (m *Model) envVarPlaceholders() []string {
	var result []string

	for i, field := range m.schema.Fields {
		if !field.EnvVar || !m.isFieldVisible(i) {
			continue
		}

		name := m.configValues[field.Key]

		if name == "" {
			name = field.Default
		}

		if name != "" {
			result = append(result, name)
		}
	}

	return result
}

func (m *Model) currentEnvVarName() string {
	idx := m.envVarFieldIndices[m.envVarIndex]
	key := m.schema.Fields[idx].Key

	if val, ok := m.configValues[key]; ok && val != "" {
		return val
	}

	return m.schema.Fields[idx].Default
}

func (m *Model) advanceEnvVar() {
	m.envVarIndex++

	if m.envVarIndex >= len(m.envVarFieldIndices) {
		m.step = stepConfirm

		return
	}

	m.step = stepEnvVarPrompt
}

func (m *Model) cmdResolveAndDownload() tea.Cmd {
	msgs := make(chan tea.Msg, downloadMsgBufferSize)
	m.dlMsgs = msgs

	go func() {
		defer close(msgs)

		rel, err := installer.Resolve(m.schema.GithubRepo, m.schema.BinaryName, m.version)
		if err != nil {
			msgs <- msgErr{err}

			return
		}

		tmpPath, err := installer.DownloadBinary(rel, func(written, total int64) {
			select {
			case msgs <- msgProgress{written: written, total: total}:
			default:
			}
		})
		if err != nil {
			msgs <- msgErr{err}

			return
		}

		msgs <- msgDownloadDone{tmpPath: tmpPath}
	}()

	return cmdWaitDownloadMsg(msgs)
}

func cmdWaitDownloadMsg(msgs <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if msgs == nil {
			return nil
		}

		msg, ok := <-msgs
		if !ok {
			return nil
		}

		return msg
	}
}

func (m *Model) cmdInstall() tea.Cmd {
	return func() tea.Msg {
		appDir := m.installDir.Value()
		if m.schema.InstallSubdir != "" {
			appDir = filepath.Join(appDir, m.schema.InstallSubdir)
		}

		binPath, err := installer.InstallBinary(m.tmpBinPath, appDir, m.schema.BinaryName)
		if err != nil {
			return msgErr{fmt.Errorf("installing binary: %w", err)}
		}

		tomlContent := installer.BuildTOML(m.configValues)

		cfgPath, err := installer.WriteConfig(appDir, m.schema.AppName+".toml", tomlContent)
		if err != nil {
			return msgErr{fmt.Errorf("writing config: %w", err)}
		}

		var envPath string
		if envPlaceholders := m.envVarPlaceholders(); len(m.envVarValues) > 0 || len(envPlaceholders) > 0 {
			envPath, err = installer.WriteEnvFile(appDir, m.envVarValues, envPlaceholders)
			if err != nil {
				return msgErr{fmt.Errorf("writing .env file: %w", err)}
			}
		}

		return msgInstallDone{binaryPath: binPath, configPath: cfgPath, envPath: envPath}
	}
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m *Model) View() string {
	switch m.step {
	case stepWelcome:
		return m.viewWelcome()
	case stepInstallDir:
		return m.viewInstallDir()
	case stepConfigFields:
		return m.viewSection()
	case stepEnvVarPrompt:
		return m.viewEnvVarPrompt()
	case stepEnvVarValue:
		return m.viewEnvVarValue()
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

func (m *Model) viewWelcome() string {
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

func (m *Model) viewInstallDir() string {
	return m.viewStep(
		"Install Directory",
		"Where should the binary be installed?",
		m.installDir.View(),
		"Press Enter to confirm  •  Esc to cancel",
	)
}

// viewSection renders the current section as a form with all visible fields.
func (m *Model) viewSection() string {
	sec := m.sections[m.sectionIdx]

	total, current := 0, 0

	for i := range m.sections {
		if m.isSectionVisible(i) {
			total++

			if i <= m.sectionIdx {
				current++
			}
		}
	}

	sectionProgress := styleSubtle.Render(fmt.Sprintf("Section %d of %d", current, total))

	var rows []string

	for gi, g := range sec.groups {
		if !m.isGroupVisible(sec, gi) {
			continue
		}

		rows = append(rows, m.renderFieldGroup(g, gi == m.fieldIdx))
	}

	form := strings.Join(rows, "\n")
	controls := styleSubtle.Render("Tab/↑↓ to switch fields  •  ←→ to cycle select  •  Enter to confirm  •  Esc to quit")

	return m.viewStep(sec.name, sectionProgress, form+"\n\n"+controls, "")
}

func (m *Model) renderFieldGroup(g fieldGroup, focused bool) string {
	if g.kind == kindRGB {
		return m.renderRGBGroup(g, focused)
	}
	return m.renderSingleGroup(g, focused)
}

func (m *Model) renderSingleGroup(g fieldGroup, focused bool) string {
	idx := g.indices[0]
	field := m.schema.Fields[idx]

	cursor := "  "
	if focused {
		cursor = stylePrimary.Render("▶") + " "
	}

	raw := field.Label + ":"
	raw += strings.Repeat(" ", max(0, labelColWidth-len(raw)))

	labelStyle := styleLabelUnfocused
	descStyle := styleDescUnfocused

	if focused {
		labelStyle = styleLabelFocused
		descStyle = styleDescFocused
	}

	label := labelStyle.Render(raw)

	descLine := ""

	if field.Description != "" {
		descLine = "\n" + strings.Repeat(" ", 2+labelColWidth) + descStyle.Render(field.Description)
	}

	switch field.Type {
	case schema.FieldTypeBool:
		check := "[ ]"
		if m.boolValues[field.Key] {
			check = styleSuccess.Render("[✓]")
		}
		return cursor + check + " " + field.Label + descLine
	case schema.FieldTypeSelect:
		return cursor + label + m.renderSelect(idx, field, focused) + descLine
	default:
		return cursor + label + m.inputs[idx].View() + descLine
	}
}

// renderSelect renders a select field's current value with directional arrows when focused.
func (m *Model) renderSelect(schIdx int, field schema.Field, focused bool) string {
	sel := m.selectIdxes[schIdx]
	value := selectLabel(field, sel)

	if !focused {
		return styleSubtle.Render(value)
	}

	var render strings.Builder

	if sel > 0 {
		render.WriteString(styleSubtle.Render("◀ "))
	} else {
		render.WriteString("  ")
	}

	render.WriteString(stylePrimary.Render(value))

	if sel < len(field.Options)-1 {
		render.WriteString(styleSubtle.Render(" ▶"))
	}

	return render.String()
}

// selectLabel returns the human-readable label for option sel, falling back to the raw value.
func selectLabel(f schema.Field, sel int) string {
	if sel < len(f.OptionLabels) && f.OptionLabels[sel] != "" {
		return f.OptionLabels[sel]
	}

	return f.Options[sel]
}

func (m *Model) renderRGBGroup(g fieldGroup, focused bool) string {
	cursor := "  "
	if focused {
		cursor = stylePrimary.Render("▶") + " "
	}

	raw := g.label + ":"
	raw += strings.Repeat(" ", max(0, rgbColWidth-len(raw)))

	rVal, gVal, bVal := 0, 0, 0
	hasValue := false

	for _, schIdx := range g.indices {
		field := m.schema.Fields[schIdx]
		cur := m.inputs[schIdx].Value()

		if cur == "" {
			cur = field.Default
		}

		if cur != "" {
			hasValue = true
		}

		value, _ := strconv.Atoi(cur)

		if value < 0 {
			value = 0
		} else if value > 255 {
			value = 255
		}

		switch field.Key[strings.LastIndex(field.Key, ".")+1:] {
		case "r":
			rVal = value
		case "g":
			gVal = value
		case "b":
			bVal = value
		}
	}

	labelStyle := styleDimLabel

	if hasValue {
		hex := fmt.Sprintf("#%02X%02X%02X", rVal, gVal, bVal)
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
	}

	label := labelStyle.Render(raw)

	var parts []string

	for _, schIdx := range g.indices {
		f := m.schema.Fields[schIdx]
		letter := strings.ToUpper(f.Key[strings.LastIndex(f.Key, ".")+1:])
		lbl := styleDimLabel.Render(letter + ":")

		parts = append(parts, lbl+m.inputs[schIdx].View())
	}

	return cursor + label + strings.Join(parts, "  ")
}

func (m *Model) viewEnvVarPrompt() string {
	name := m.currentEnvVarName()
	pos := fmt.Sprintf("%d of %d", m.envVarIndex+1, len(m.envVarFieldIndices))
	return m.viewStep(
		"Set Environment Variable",
		styleSubtle.Render(pos)+"\n\nDo you want to set the value for "+stylePrimary.Render(name)+"?\nThe value will be written to a "+stylePrimary.Render(".env")+" file beside the binary.",
		styleSubtle.Render("y = yes  •  n / Enter = skip  •  Esc to cancel"),
		"",
	)
}

func (m *Model) viewEnvVarValue() string {
	name := m.currentEnvVarName()

	return m.viewStep(
		name,
		"Enter the API key value (stored masked in .env)",
		m.envVarInput.View(),
		"Enter to confirm  •  Esc to cancel",
	)
}

func (m *Model) viewConfirm() string {
	var lines []string

	lines = append(lines,
		styleBold.Render("Review your settings:"),
		"",
		styleDimLabel.Render("Install directory:"),
		"  "+m.installDir.Value(),
	)

	if len(m.configValues) > 0 {
		type kv struct{ k, v string }
		var kvs []kv

		for k, v := range m.configValues {
			kvs = append(kvs, kv{k, v})
		}

		sort.Slice(kvs, func(i, j int) bool { return kvs[i].k < kvs[j].k })

		prevSection := ""

		for _, pair := range kvs {
			secName := sectionDisplayName(topLevelKey(pair.k))

			if secName != prevSection {
				lines = append(lines, "", styleDimLabel.Render(secName+":"))
				prevSection = secName
			}

			display := pair.v

			for _, f := range m.schema.Fields {
				if f.Key == pair.k && f.Type == schema.FieldTypePassword {
					display = strings.Repeat("•", len(pair.v))
				}
			}

			lines = append(lines, fmt.Sprintf("  %s = %s", styleDimLabel.Render(pair.k), display))
		}
	}

	if len(m.envVarValues) > 0 {
		lines = append(lines, "", styleDimLabel.Render("Environment variables (.env):"))
		names := make([]string, 0, len(m.envVarValues))

		for k := range m.envVarValues {
			names = append(names, k)
		}

		sort.Strings(names)

		for _, name := range names {
			lines = append(lines, fmt.Sprintf("  %s = %s", styleDimLabel.Render(name), strings.Repeat("•", 8)))
		}
	}

	lines = append(lines, "", styleSubtle.Render("Press Enter or y to install  •  n/q to cancel"))
	body := styleBox.Render(strings.Join(lines, "\n"))

	return "\n" + styleStepTitle.Render("Confirm Installation") + "\n" + body + "\n"
}

func (m *Model) viewDownloading() string {
	var pct string

	if m.dlTotal > 0 {
		pct = fmt.Sprintf(" %.0f%%", float64(m.dlWritten)/float64(m.dlTotal)*100)
	}

	return "\n" + m.spinner.View() +
		stylePrimary.Render(fmt.Sprintf("  Downloading latest %s release%s…", m.schema.AppName, pct)) +
		"\n\n  " + m.progress.View() +
		"\n\n  " + m.progress.View() +
		"\n\n" + styleSubtle.Render("  This may take a moment") + "\n"
}

func (m *Model) viewInstalling() string {
	return "\n" + m.spinner.View() + stylePrimary.Render("  Installing…") + "\n"
}

func (m *Model) viewDone() string {
	content := styleSuccess.Render("✓  Installation complete!") + "\n\n" +
		styleDimLabel.Render("Binary:") + "\n  " + m.binaryPath + "\n\n" +
		styleDimLabel.Render("Config:") + "\n  " + m.configPath + "\n\n"

	if m.envPath != "" {
		content += styleDimLabel.Render("Env file:") + "\n  " + m.envPath + "\n\n"
	}

	pathDir := m.installDir.Value()
	if m.schema.InstallSubdir != "" {
		pathDir = filepath.Join(pathDir, m.schema.InstallSubdir)
	}

	content += styleSubtle.Render("Make sure "+pathDir+" is in your PATH.\n") +
		styleSubtle.Render("Press any key to exit.")

	return "\n" + styleBox.Render(content) + "\n"
}

func (m *Model) viewError() string {
	body := styleBox.Render(
		styleError.Render("✗  Installation failed") + "\n\n" +
			m.errMsg + "\n\n" +
			styleSubtle.Render("Press any key to exit."),
	)

	return "\n" + body + "\n"
}

// viewStep is a generic layout helper for wizard screens.
func (m *Model) viewStep(title, subtitle, input, hint string) string {
	var view strings.Builder

	view.WriteString("\n")
	view.WriteString(styleStepTitle.Render(title))
	view.WriteString("\n")

	if subtitle != "" {
		view.WriteString(styleSubtle.Render(subtitle))
		view.WriteString("\n\n")
	}

	view.WriteString("  ")
	view.WriteString(strings.ReplaceAll(input, "\n", "\n  "))
	view.WriteString("\n")

	if hint != "" {
		view.WriteString("\n")
		view.WriteString(styleSubtle.Render("  " + hint))
		view.WriteString("\n")
	}

	return view.String()
}
