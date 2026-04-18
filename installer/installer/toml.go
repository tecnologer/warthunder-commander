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

	for k, v := range values {
		parts := strings.SplitN(k, ".", 2)
		var section, key string
		if len(parts) == 2 {
			section, key = parts[0], parts[1]
		} else {
			section, key = "", parts[0]
		}
		sections[section] = append(sections[section], entry{section, key, v})
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
func tomlValue(v string) string {
	switch v {
	case "true", "false":
		return v
	}
	// If it looks like an integer or float, emit bare.
	if isNumeric(v) {
		return v
	}
	// Escape backslashes and quotes for TOML strings.
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return fmt.Sprintf("%q", v)
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	dotCount := 0
	for i, c := range s {
		if c == '-' && i == 0 {
			continue
		}
		if c == '.' {
			dotCount++
			if dotCount > 1 {
				return false
			}
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
