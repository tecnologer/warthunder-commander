package commander

import (
	"regexp"
	"strings"
)

var (
	// reGrid matches grid references in LLM output in several formats:
	//   "at grid Delta-Six", "grid Delta-Six", "at D5", "at C4".
	// The (?i) flag makes the match case-insensitive throughout.
	reGrid = regexp.MustCompile(
		`(?i)(?:at\s+)?grid\s+[a-z][a-z0-9]*(?:[-\s][a-z][a-z0-9]*)?` +
			`|at\s+[a-h]\d\b`,
	)

	// reNonAlnum removes everything that is not an ASCII letter, digit, or space.
	reNonAlnum = regexp.MustCompile(`[^a-z0-9 ]`)

	// reSpaces collapses runs of whitespace into a single space.
	reSpaces = regexp.MustCompile(`\s+`)
)

// normalizeAlert normalises a commander message for semantic deduplication.
// It strips the callsign prefix, grid references, and punctuation, then folds
// to lowercase. Two messages with the same normalised form describe the same
// tactical situation and the second should be suppressed.
//
// normalizeAlert is pure and has no side effects.
func normalizeAlert(callsign, alert string) string {
	alert = strings.ToLower(alert)

	callsign = strings.ToLower(callsign)

	// Remove the callsign prefix. The LLM uses several separator styles.
	for _, sep := range []string{", ", " \u2014 ", " - ", ","} {
		if rest, ok := strings.CutPrefix(alert, callsign+sep); ok {
			alert = rest
			break
		}
	}

	// Strip grid references — position-specific and not semantically meaningful
	// for deduplication (same threat can be reported at slightly different grids).
	alert = reGrid.ReplaceAllString(alert, "")

	// Strip punctuation; keep only lowercase letters, digits, and spaces.
	alert = reNonAlnum.ReplaceAllString(alert, "")

	// Collapse runs of whitespace introduced by the removals above.
	return strings.TrimSpace(reSpaces.ReplaceAllString(alert, " "))
}
