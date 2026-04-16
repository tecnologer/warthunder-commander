package commander //nolint:testpackage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tecnologer/warthunder/internal/lang"
)

// newTestCommanderWithMax builds a Commander with a custom alertHistoryMax.
func newTestCommanderWithMax(callsign string, histMax int, responses ...string) *Commander {
	var llm backend
	if len(responses) == 1 {
		llm = &mockBackend{response: responses[0]}
	} else {
		llm = &sequentialBackend{responses: responses}
	}

	return &Commander{
		llm:             llm,
		callsign:        callsign,
		lang:            lang.EN,
		alertHistoryMax: histMax,
	}
}

// --- alertHistory update tests ---

func TestAlertHistoryUpdatesAfterEmit(t *testing.T) {
	t.Parallel()

	cmd := newTestCommanderWithMax("Iron Arm", 2,
		"Iron Arm, tango closing right flank.",
		"Iron Arm, contact — medium tank six o'clock.",
	)

	cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // first call
	cmd.mu.Lock()
	got := len(cmd.alertHistory)
	cmd.mu.Unlock()

	if got != 1 {
		t.Fatalf("after first alert: alertHistory len = %d, want 1", got)
	}

	cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // second call
	cmd.mu.Lock()
	got = len(cmd.alertHistory)
	cmd.mu.Unlock()

	if got != 2 {
		t.Fatalf("after second alert: alertHistory len = %d, want 2", got)
	}
}

func TestAlertHistoryNotUpdatedOnDuplicate(t *testing.T) {
	t.Parallel()

	msg := "White Horse, tango closing right flank."
	cmd := newTestCommanderWithMax("White Horse", 3, msg)

	cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // first (emitted)
	cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // second (suppressed duplicate)

	cmd.mu.Lock()
	got := len(cmd.alertHistory)
	cmd.mu.Unlock()

	if got != 1 {
		t.Errorf("duplicate suppressed: alertHistory len = %d, want 1", got)
	}
}

func TestAlertHistoryDoesNotExceedMax(t *testing.T) {
	t.Parallel()

	const histMax = 3

	responses := []string{
		"White Horse, contact — tango right flank.",
		"White Horse, tango still closing, medium range.",
		"White Horse, tango critical — immediate proximity.",
		"White Horse, fast mover buster, six o'clock.",
		"White Horse, left flank cold, threat gone.",
	}
	cmd := newTestCommanderWithMax("White Horse", histMax, responses...)

	for range responses {
		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck
	}

	cmd.mu.Lock()
	got := len(cmd.alertHistory)
	cmd.mu.Unlock()

	if got > histMax {
		t.Errorf("alertHistory len = %d, want <= %d", got, histMax)
	}
}

func TestAlertHistoryRolling(t *testing.T) {
	t.Parallel()

	responses := []string{
		"White Horse, contact — tango right flank.",
		"White Horse, tango still closing, medium range.",
		"White Horse, tango critical — immediate proximity.",
		"White Horse, fast mover buster, six o'clock.",
	}
	cmd := newTestCommanderWithMax("White Horse", 3, responses...)

	for range responses {
		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck
	}

	cmd.mu.Lock()
	history := make([]string, len(cmd.alertHistory))
	copy(history, cmd.alertHistory)
	cmd.mu.Unlock()

	// After 4 emits with max=3, oldest entry should be the second response.
	if len(history) != 3 {
		t.Fatalf("history len = %d, want 3", len(history))
	}

	if history[0] != responses[1] {
		t.Errorf("history[0] = %q, want %q (oldest retained)", history[0], responses[1])
	}

	if history[2] != responses[3] {
		t.Errorf("history[2] = %q, want %q (most recent)", history[2], responses[3])
	}
}

// --- ResetLastAlert tests ---

func TestResetLastAlertClearsHistory(t *testing.T) {
	t.Parallel()

	cmd := newTestCommanderWithMax("White Horse", 3,
		"White Horse, tango closing right flank.",
		"White Horse, contact — medium tank six o'clock.",
		"White Horse, tango critical — immediate proximity.",
	)

	for range 3 {
		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck
	}

	cmd.ResetLastAlert()

	cmd.mu.Lock()
	histLen := len(cmd.alertHistory)
	lastAlert := cmd.lastAlert
	cmd.mu.Unlock()

	if histLen != 0 {
		t.Errorf("after ResetLastAlert: alertHistory len = %d, want 0", histLen)
	}

	if lastAlert != "" {
		t.Errorf("after ResetLastAlert: lastAlert = %q, want empty", lastAlert)
	}
}

// --- buildPrompt Previous alerts section tests ---

func TestBuildPromptOmitsPreviousAlertsWhenEmpty(t *testing.T) {
	t.Parallel()

	cmd := &Commander{
		llm:             &mockBackend{},
		callsign:        "White Horse",
		lang:            lang.EN,
		alertHistoryMax: 3,
	}

	prompt := cmd.buildPrompt(nil, nil)

	if strings.Contains(prompt, "Previous alerts") {
		t.Errorf("buildPrompt with empty history must not include 'Previous alerts' section, got:\n%s", prompt)
	}
}

func TestBuildPromptIncludesPreviousAlertsWithRelativeLabels(t *testing.T) {
	t.Parallel()

	cmd := &Commander{
		llm:      &mockBackend{},
		callsign: "White Horse",
		lang:     lang.EN,
		alertHistory: []string{
			"White Horse, traffic — light tank six o'clock.",
			"White Horse, contact — tango closing right flank.",
		},
		alertHistoryMax: 3,
	}

	prompt := cmd.buildPrompt(nil, nil)

	if !strings.Contains(prompt, "Previous alerts") {
		t.Fatalf("buildPrompt with history must include 'Previous alerts' section, got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "[1 report ago]") {
		t.Errorf("most recent entry must be labelled '[1 report ago]', got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "[2 reports ago]") {
		t.Errorf("second entry must be labelled '[2 reports ago]', got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "tango closing right flank") {
		t.Errorf("most recent alert text must appear in prompt, got:\n%s", prompt)
	}
}

// --- formattedHistory tests ---

func TestFormattedHistoryEmptyReturnsEmpty(t *testing.T) {
	t.Parallel()

	cmd := &Commander{alertHistoryMax: 3}
	if got := cmd.formattedHistory(); got != "" {
		t.Errorf("formattedHistory() with empty history = %q, want empty", got)
	}
}

func TestFormattedHistoryOrder(t *testing.T) {
	t.Parallel()

	cmd := &Commander{
		alertHistory: []string{
			"White Horse, traffic — light tank six o'clock.",
			"White Horse, contact — tango closing right flank.",
		},
		alertHistoryMax: 3,
	}

	got := cmd.formattedHistory()

	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 {
		t.Fatalf("formattedHistory returned %d lines, want 2:\n%s", len(lines), got)
	}

	// First line = most recent = "[1 report ago]"
	if !strings.HasPrefix(lines[0], "[1 report ago]") {
		t.Errorf("line 0 = %q, want prefix '[1 report ago]'", lines[0])
	}

	// Second line = older = "[2 reports ago]"
	if !strings.HasPrefix(lines[1], "[2 reports ago]") {
		t.Errorf("line 1 = %q, want prefix '[2 reports ago]'", lines[1])
	}
}

// --- Integration: history in system prompt ---

func TestAdviseSystemPromptIncludesHistory(t *testing.T) {
	t.Parallel()

	var capturedSystem string

	cmd := &Commander{
		llm: &capturingBackend{
			capture:  &capturedSystem,
			response: "White Horse, tango still hot, medium range.",
		},
		callsign:   "White Horse",
		lang:       lang.EN,
		mode:       "warning",
		windowSecs: 30,
		alertHistory: []string{
			"White Horse, contact — tango closing right flank.",
		},
		alertHistoryMax: 3,
	}

	r, _, err := cmd.Advise(context.Background(), nil, nil)
	if err != nil || r == nil {
		t.Fatalf("Advise: want report, got err=%v report=%v", err, r)
	}

	if !strings.Contains(capturedSystem, "[1 report ago]") {
		t.Errorf("system prompt must include alert history, got:\n%s", capturedSystem)
	}
}

// capturingBackend records the system prompt passed to complete().
type capturingBackend struct {
	capture  *string
	response string
	err      error
}

func (b *capturingBackend) complete(_ context.Context, systemPrompt, _ string) (string, error) {
	*b.capture = systemPrompt
	return b.response, b.err
}

// TestAdviseDoesNotEmitWhenLLMReturnsEmpty ensures ErrNoReport on empty response.
func TestAdviseDoesNotEmitWhenLLMReturnsEmpty(t *testing.T) {
	t.Parallel()

	cmd := newTestCommanderWithMax("White Horse", 3, "")

	_, _, err := cmd.Advise(context.Background(), nil, nil)
	if !errors.Is(err, ErrNoReport) {
		t.Errorf("empty LLM response: want ErrNoReport, got %v", err)
	}

	cmd.mu.Lock()
	got := len(cmd.alertHistory)
	cmd.mu.Unlock()

	if got != 0 {
		t.Errorf("empty LLM response must not update alertHistory, len = %d", got)
	}
}
