package installer

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// BuildTOML takes a flat map of dot-notation keys to string values and
// renders a valid TOML string with nested sections.
//
// Example input:
//
//	{"server.host": "localhost", "server.port": "8080", "log.level": "info"}
//
// Example output:
//
//	[server]
//	host = "localhost"
//	port = "8080"
//
//	[log]
//	level = "info"
func BuildTOML(values map[string]string) string {
	type entry struct {
		section string
		key     string
		value   string
	}

	// Group by section (top-level key before first dot).
	// Keys with no dot go into the "" (root) section.
	sections := map[string][]entry{}
	order := []string{}
	seen := map[string]bool{}

	for key, value := range values {
		parts := strings.SplitN(key, ".", 2)

		var section, key string
		if len(parts) == 2 {
			section, key = parts[0], parts[1]
		} else {
			section, key = "", parts[0]
		}

		sections[section] = append(sections[section], entry{section, key, value})
		if !seen[section] {
			order = append(order, section)
			seen[section] = true
		}
	}

	// Sort sections for deterministic output; root section first.
	sort.SliceStable(order, func(i, j int) bool {
		if order[i] == "" {
			return true
		}

		if order[j] == "" {
			return false
		}

		return order[i] < order[j]
	})

	var buf bytes.Buffer

	for _, section := range order {
		entries := sections[section]
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].key < entries[j].key
		})

		if section != "" {
			fmt.Fprintf(&buf, "[%s]\n", section)
		}

		for _, e := range entries {
			fmt.Fprintf(&buf, "%s = %s\n", e.key, tomlValue(e.value))
		}

		buf.WriteString("\n")
	}

	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// tomlValue wraps string values in quotes, passes through booleans and numbers bare.
func tomlValue(value string) string {
	switch value {
	case "true", "false":
		return value
	}

	// If it looks like an integer or float, emit bare.
	if isNumeric(value) {
		return value
	}

	// Escape backslashes and quotes for TOML strings.
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)

	return fmt.Sprintf("%q", value)
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}

	dotCount := 0

	for i, char := range value {
		if char == '-' && i == 0 {
			continue
		}

		if char == '.' {
			dotCount++
			if dotCount > 1 {
				return false
			}

			continue
		}

		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
