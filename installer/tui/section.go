package tui

import (
	"strings"

	"github.com/tecnologer/warthunder/installer/schema"
)

type fieldGroupKind int

const (
	kindSingle fieldGroupKind = iota
	kindRGB
)

// fieldGroup is one "row" in a section form: a single field or an RGB triplet.
type fieldGroup struct {
	kind    fieldGroupKind
	indices []int  // indices into schema.Fields; len=1 for kindSingle, 3 for kindRGB
	label   string // display name; used for kindRGB (e.g. "Player")
}

// section maps to one TOML table and becomes one wizard screen.
type section struct {
	name   string
	groups []fieldGroup
}

// computeSections groups schema fields by their top-level TOML key, then detects
// consecutive R/G/B triplets within the same parent key and fuses them into kindRGB groups.
func computeSections(fields []schema.Field) []section {
	var order []string
	byKey := map[string][]int{}

	for i, f := range fields {
		tk := topLevelKey(f.Key)
		if _, ok := byKey[tk]; !ok {
			order = append(order, tk)
		}
		byKey[tk] = append(byKey[tk], i)
	}

	sections := make([]section, 0, len(order))
	for _, key := range order {
		sec := section{name: sectionDisplayName(key)}
		sec.groups = buildGroups(fields, byKey[key])
		sections = append(sections, sec)
	}
	return sections
}

// topLevelKey returns everything before the first dot, or "" for root-level keys.
func topLevelKey(key string) string {
	if i := strings.Index(key, "."); i >= 0 {
		return key[:i]
	}
	return ""
}

// sectionDisplayName converts a top-level key to a human-readable title.
func sectionDisplayName(key string) string {
	if key == "" {
		return "General"
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

// buildGroups converts a flat list of field indices into fieldGroups, merging
// consecutive R/G/B triplets that share a parent key.
func buildGroups(fields []schema.Field, indices []int) []fieldGroup {
	var groups []fieldGroup
	for i := 0; i < len(indices); {
		if i+2 < len(indices) && isRGBTriplet(fields, indices[i], indices[i+1], indices[i+2]) {
			triplet := make([]int, 3)
			copy(triplet, indices[i:i+3])
			groups = append(groups, fieldGroup{
				kind:    kindRGB,
				indices: triplet,
				label:   rgbLabel(fields[indices[i]].Key),
			})
			i += 3
			continue
		}
		groups = append(groups, fieldGroup{kind: kindSingle, indices: []int{indices[i]}})
		i++
	}
	return groups
}

// isRGBTriplet returns true when three consecutive fields share the same parent
// key prefix and their final components are exactly "r", "g", "b" in any order.
func isRGBTriplet(fields []schema.Field, i0, i1, i2 int) bool {
	parent := fieldParentKey(fields[i0].Key)
	if parent == "" {
		return false
	}
	if fieldParentKey(fields[i1].Key) != parent || fieldParentKey(fields[i2].Key) != parent {
		return false
	}
	last := func(k string) string { return k[strings.LastIndex(k, ".")+1:] }
	parts := map[string]bool{
		last(fields[i0].Key): true,
		last(fields[i1].Key): true,
		last(fields[i2].Key): true,
	}
	return parts["r"] && parts["g"] && parts["b"]
}

// rgbLabel derives a display name for an RGB group from the first field's key.
// e.g. "colors.player.r" → "Player"
func rgbLabel(key string) string {
	parent := fieldParentKey(key) // "colors.player"
	i := strings.LastIndex(parent, ".")
	name := parent[i+1:]
	if name == "" {
		return "RGB"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// fieldParentKey returns everything before the last dot, or "" if there is no dot.
func fieldParentKey(key string) string {
	if i := strings.LastIndex(key, "."); i >= 0 {
		return key[:i]
	}
	return ""
}
