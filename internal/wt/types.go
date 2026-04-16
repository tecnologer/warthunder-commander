package wt

import (
	"fmt"
	"math"
	"strings"

	"github.com/tecnologer/warthunder/internal/config"
)

// GameMode identifies the battle ruleset.
type GameMode int

const (
	// GameModeArcade is the default; all alerts active.
	GameModeArcade GameMode = iota
	// GameModeRealistic only show enemies spotted by a nearby ally (< 0.20 units).
	GameModeRealistic
	// GameModeSimulator all enemy-position alerts are disabled.
	GameModeSimulator
)

// ParseGameMode converts an API string to a GameMode.
// Unrecognised values return GameModeArcade.
func ParseGameMode(s string) GameMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "realistic", "rb", "realistic_battle":
		return GameModeRealistic
	case "simulator", "sim", "sb", "simulator_battle":
		return GameModeSimulator
	default:
		return GameModeArcade
	}
}

// Team identifies which side an object belongs to.
type Team int

const (
	TeamUnknown Team = iota
	TeamPlayer
	TeamAlly
	TeamEnemy
	TeamSquad
)

// MapObject represents a single entry from /map_obj.json.
type MapObject struct {
	Type   string    `json:"type"`
	Color  string    `json:"color"`
	ColorR []float64 `json:"color[]"`
	Blink  int       `json:"blink"`
	Icon   string    `json:"icon"`
	IconBg string    `json:"icon_bg"`
	X      float64   `json:"x"`
	Y      float64   `json:"y"`
	DX     float64   `json:"dx"`
	DY     float64   `json:"dy"`
}

// MapInfo represents /map_info.json metadata.
type MapInfo struct {
	Valid         bool       `json:"valid"`
	MapGeneration int        `json:"map_generation"`
	HudType       int        `json:"hud_type"`
	GridSize      [2]float64 `json:"grid_size"`  // [width, height] of the labelled grid area in map units
	GridSteps     [2]float64 `json:"grid_steps"` // [col_width, row_height] — size of one grid cell in map units
	GridZero      [2]float64 `json:"grid_zero"`  // map-space coordinate of the north-west (top-left) grid corner
	MapMin        [2]float64 `json:"map_min"`    // south-west corner of the map coordinate space
	MapMax        [2]float64 `json:"map_max"`    // north-east corner of the map coordinate space
	// Legacy: present in older API versions only.
	MapName  string  `json:"map_name"`
	MapSizeX float64 `json:"map_size_x"`
	MapSizeY float64 `json:"map_size_y"`
}

// GridDims returns the number of grid columns and rows.
// Derived as floor(grid_size / grid_steps). Falls back to 8×8 when grid data
// is unavailable (e.g. game not in a match).
func (m *MapInfo) GridDims() (int, int) {
	if m == nil || m.GridSteps[0] <= 0 || m.GridSteps[1] <= 0 {
		return 8, 8
	}

	cols := int(m.GridSize[0] / m.GridSteps[0])
	rows := int(m.GridSize[1] / m.GridSteps[1])

	if cols <= 0 {
		cols = 8
	}

	if rows <= 0 {
		rows = 8
	}

	return cols, rows
}

// teamColors holds the active color configuration; set via SetColors.
//
//nolint:gochecknoglobals // intentional package-level state updated by SetColors
var teamColors = config.ColorsConfig{
	Tolerance: 30,
	Player:    config.RGBColor{R: 250, G: 200, B: 30},
	Ally:      config.RGBColor{R: 23, G: 77, B: 255},
	Enemy:     config.RGBColor{R: 250, G: 12, B: 0},
	Squad:     config.RGBColor{R: 103, G: 215, B: 86},
}

// SetColors replaces the color thresholds used for team identification.
func SetColors(c config.ColorsConfig) { teamColors = c }

// Team derives the team from the object's color array.
func (o *MapObject) Team() Team {
	if len(o.ColorR) < 3 {
		return TeamUnknown
	}

	red, green, blue := o.ColorR[0], o.ColorR[1], o.ColorR[2]
	tol := teamColors.Tolerance

	switch {
	case colorClose(red, green, blue, teamColors.Player.R, teamColors.Player.G, teamColors.Player.B, tol):
		return TeamPlayer
	case colorClose(red, green, blue, teamColors.Ally.R, teamColors.Ally.G, teamColors.Ally.B, tol):
		return TeamAlly
	case colorClose(red, green, blue, teamColors.Enemy.R, teamColors.Enemy.G, teamColors.Enemy.B, tol):
		return TeamEnemy
	case colorClose(red, green, blue, teamColors.Squad.R, teamColors.Squad.G, teamColors.Squad.B, tol):
		return TeamSquad
	default:
		return TeamUnknown
	}
}

const (
	// RespawnBaseTank is the icon for respawn bases in tank battles.
	RespawnBaseTank = "respawn_base_tank"
	// RespawnBaseFighter is the icon for respawn bases in air battles.
	RespawnBaseFighter = "respawn_base_fighter"
	// CaptureZone is the icon for capture zones in conquest battles.
	CaptureZone = "capture_zone"
)

// IsEnemy returns true when the object belongs to the enemy team.
// Respawn bases share the enemy team color but are map objects, not combatants.
func (o *MapObject) IsEnemy() bool {
	return o.Team() == TeamEnemy &&
		o.Icon != RespawnBaseTank &&
		o.Icon != RespawnBaseFighter &&
		o.Icon != CaptureZone
}

// IsAlly returns true when the object is an allied combatant.
// Respawn bases share the ally team color but are map objects, not combatants.
func (o *MapObject) IsAlly() bool {
	return o.Team() == TeamAlly &&
		o.Icon != RespawnBaseTank &&
		o.Icon != RespawnBaseFighter &&
		o.Icon != CaptureZone
}

// IsPlayer returns true when this object is the local player.
func (o *MapObject) IsPlayer() bool { return o.Team() == TeamPlayer && o.Icon == "Player" }

// IsCaptureZone returns true for capture zone objects.
func (o *MapObject) IsCaptureZone() bool { return o.Icon == "capture_zone" }

// PosKey returns a compact string key for deduplication across frames.
func (o *MapObject) PosKey() string {
	return fmt.Sprintf("%.4f:%.4f:%s", o.X, o.Y, o.Icon)
}

// Dist returns the normalized Euclidean distance between two objects.
func Dist(a, b *MapObject) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y

	return math.Sqrt(dx*dx + dy*dy)
}

// colorClose checks whether RGB values are within the given tolerance.
func colorClose(r, g, b, tr, tg, tb, tol float64) bool {
	return math.Abs(r-tr) < tol && math.Abs(g-tg) < tol && math.Abs(b-tb) < tol
}
