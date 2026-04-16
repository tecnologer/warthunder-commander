package commander //nolint:testpackage

import (
	"context"
	"errors"
	"testing"

	"github.com/tecnologer/warthunder/internal/lang"
)

// mockBackend is a single-response LLM stub for testing.
type mockBackend struct {
	response string
	err      error
}

func (m *mockBackend) complete(_ context.Context, _, _ string) (string, error) {
	return m.response, m.err
}

// sequentialBackend returns each response in order.
type sequentialBackend struct {
	responses []string
	idx       int
}

func (s *sequentialBackend) complete(_ context.Context, _, _ string) (string, error) {
	if s.idx >= len(s.responses) {
		return "", errors.New("sequentialBackend: no more responses")
	}

	r := s.responses[s.idx]
	s.idx++

	return r, nil
}

// newTestCommander builds a minimal Commander without requiring real API keys.
func newTestCommander(callsign, response string) *Commander {
	return &Commander{
		llm:      &mockBackend{response: response},
		callsign: callsign,
		lang:     lang.EN,
	}
}

// --- normalizeAlert unit tests ---

func TestNormalizeAlert(t *testing.T) { //nolint:funlen
	t.Parallel()

	tests := []struct {
		name     string
		callsign string
		input    string
		want     string
	}{
		{
			name:     "strips callsign with comma separator",
			callsign: "White Horse",
			input:    "White Horse, tango stationary, three o'clock.",
			want:     "tango stationary three oclock",
		},
		{
			name:     "strips callsign with em-dash separator",
			callsign: "White Horse",
			input:    "White Horse \u2014 contact, tango closing.",
			want:     "contact tango closing",
		},
		{
			name:     "strips callsign case-insensitively",
			callsign: "White Horse",
			input:    "WHITE HORSE, tango closing six o'clock.",
			want:     "tango closing six oclock",
		},
		{
			name:     "strips 'at grid Delta-Six' reference",
			callsign: "White Horse",
			input:    "White Horse, tango at grid Delta-Six, hot.",
			want:     "tango hot",
		},
		{
			name:     "strips 'grid Delta-Six' without 'at'",
			callsign: "White Horse",
			input:    "White Horse, tango grid Delta-Six cold.",
			want:     "tango cold",
		},
		{
			name:     "strips 'at C4' short grid reference",
			callsign: "White Horse",
			input:    "White Horse, enemies at C4, closing.",
			want:     "enemies closing",
		},
		{
			name:     "strips 'at D5' short grid reference",
			callsign: "White Horse",
			input:    "White Horse, tango at D5, stationary.",
			want:     "tango stationary",
		},
		{
			name:     "eliminates punctuation",
			callsign: "White Horse",
			input:    "White Horse, contact — medium tango!",
			want:     "contact medium tango",
		},
		{
			name:     "identical semantics different grid — normalises to same string",
			callsign: "White Horse",
			input:    "White Horse, tango stationary at grid Echo-Five, hot angle.",
			want:     "tango stationary hot angle",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeAlert(test.callsign, test.input)
			if got != test.want {
				t.Errorf("normalizeAlert(%q, %q)\n  got  %q\n  want %q", test.callsign, test.input, got, test.want)
			}
		})
	}
}

// --- Deduplication integration tests ---

func TestCommanderDeduplication(t *testing.T) { //nolint:cyclop,funlen
	t.Parallel()

	t.Run("same message twice — second suppressed", func(t *testing.T) {
		t.Parallel()

		msg := "White Horse, tango closing six o'clock."
		cmd := newTestCommander("White Horse", msg)

		r1, _, err1 := cmd.Advise(context.Background(), nil, nil)
		if err1 != nil || r1 == nil {
			t.Fatalf("first Advise: want report, got err=%v report=%v", err1, r1)
		}

		// Same LLM response on the second call.
		r2, _, err2 := cmd.Advise(context.Background(), nil, nil)
		if !errors.Is(err2, ErrNoReport) {
			t.Errorf("second Advise: want ErrNoReport, got err=%v report=%v", err2, r2)
		}
	})

	t.Run("semantically identical different grid — second suppressed", func(t *testing.T) {
		t.Parallel()

		cmd := &Commander{
			llm: &sequentialBackend{responses: []string{
				"White Horse, tango stationary at grid Delta-Six, three o'clock, hot angle.",
				"White Horse, tango stationary at grid Echo-Five, three o'clock, hot angle.",
			}},
			callsign: "White Horse",
			lang:     lang.EN,
		}

		r1, _, err1 := cmd.Advise(context.Background(), nil, nil)
		if err1 != nil || r1 == nil {
			t.Fatalf("first Advise: want report, got err=%v report=%v", err1, r1)
		}

		r2, _, err2 := cmd.Advise(context.Background(), nil, nil)
		if !errors.Is(err2, ErrNoReport) {
			t.Errorf("second Advise (same semantic): want ErrNoReport, got err=%v report=%v", err2, r2)
		}
	})

	t.Run("different message after duplicate — emitted", func(t *testing.T) {
		t.Parallel()

		cmd := &Commander{
			llm: &sequentialBackend{responses: []string{
				"White Horse, tango closing six o'clock.",
				"White Horse, tango closing six o'clock.", // duplicate
				"White Horse, fast mover, buster, left flank.",
			}},
			callsign: "White Horse",
			lang:     lang.EN,
		}

		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // first call, not under test
		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // duplicate, not under test

		r3, _, err3 := cmd.Advise(context.Background(), nil, nil)
		if err3 != nil || r3 == nil {
			t.Errorf("third Advise (different): want report, got err=%v report=%v", err3, r3)
		}
	})

	t.Run("ResetLastAlert — first alert after reset always emitted", func(t *testing.T) {
		t.Parallel()

		msg := "White Horse, tango closing six o'clock."
		cmd := newTestCommander("White Horse", msg)

		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // prime state
		cmd.Advise(context.Background(), nil, nil) //nolint:errcheck // suppressed

		cmd.ResetLastAlert()

		r, _, err := cmd.Advise(context.Background(), nil, nil)
		if err != nil || r == nil {
			t.Errorf("Advise after ResetLastAlert: want report, got err=%v report=%v", err, r)
		}
	})

	t.Run("ResetLastAlert on nil Commander — no panic", func(t *testing.T) {
		t.Parallel()

		var cmd *Commander
		cmd.ResetLastAlert() // must not panic
	})
}
