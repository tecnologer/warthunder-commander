package commander

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tecnologer/warthunder/internal/collector"
	"github.com/tecnologer/warthunder/internal/config"
	"github.com/tecnologer/warthunder/internal/lang"
	"github.com/tecnologer/warthunder/internal/wt"
)

// ErrNoReport is returned by Advise when the LLM has nothing to report.
var ErrNoReport = errors.New("commander: nothing to report")

// backend sends a system prompt and a user prompt to an LLM and returns the text response.
type backend interface {
	complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Commander generates tactical reports via a pluggable LLM backend.
type Commander struct {
	llm        backend
	lang       lang.Language
	callsign   string
	mode       string
	windowSecs int

	mu              sync.Mutex
	lastAlert       string   // normalised form of the last emitted message
	alertHistory    []string // raw text of the last alertHistoryMax emitted alerts (oldest first)
	alertHistoryMax int
}

// Report holds a tactical recommendation.
type Report struct {
	Message string
}

// New returns a Commander, or nil if no API key is configured for the selected
// engine. A nil Commander is valid — callers should skip AI reports in that case.
// commanderInterval is used to set the window duration in the system prompt.
func New(aiCfg config.AIConfig, language lang.Language, commanderInterval time.Duration) *Commander {
	keyEnv := aiCfg.GroqEnv
	if aiCfg.Engine == config.AIEngineAnthropic {
		keyEnv = aiCfg.AnthropicEnv
	}

	if os.Getenv(keyEnv) == "" {
		log.Printf("[commander] env var %q not set — AI commander disabled", keyEnv)
		return nil
	}

	model := aiCfg.Model
	var llm backend
	if aiCfg.Engine == config.AIEngineAnthropic {
		if model == "" {
			model = config.DefaultAnthropicModel
		}
		llm = newAnthropicBackend(model)
	} else {
		if model == "" {
			model = config.DefaultGroqModel
		}
		llm = newGroqBackend(aiCfg.GroqEnv, model)
	}

	callsign := aiCfg.Callsign
	if callsign == "" {
		callsign = "Bronco"
	}

	histMax := aiCfg.AlertHistoryMax
	if histMax <= 0 {
		histMax = 3
	}

	return &Commander{
		llm:             llm,
		lang:            language,
		callsign:        callsign,
		mode:            aiCfg.Mode,
		windowSecs:      int(commanderInterval.Seconds()),
		alertHistoryMax: histMax,
	}
}

// Advise builds a tactical prompt from the summary, calls the backend, and
// returns a tactical report. Prompt is always populated even when ErrNoReport
// is returned, so callers can log it for debugging.
//
// Consecutive reports that normalise to the same string are suppressed so the
// player is not alerted to an unchanged situation every commander interval.
func (c *Commander) Advise(ctx context.Context, sum *collector.Summary, mapInfo *wt.MapInfo) (*Report, string, error) {
	systemPrompt := c.lang.SystemPrompt(c.callsign, c.mode, c.windowSecs, c.formattedHistory())
	prompt := c.buildPrompt(sum, mapInfo)

	text, err := c.llm.complete(ctx, systemPrompt, prompt)
	if err != nil {
		return nil, prompt, err
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, prompt, ErrNoReport
	}

	// Semantic deduplication: suppress reports that convey the same information
	// as the previous one, even if wording or grid refs differ slightly.
	norm := normalizeAlert(c.callsign, text)

	c.mu.Lock()

	dup := norm == c.lastAlert
	if !dup {
		c.lastAlert = norm
		c.appendHistory(text)
	}
	c.mu.Unlock()

	if dup {
		return nil, prompt, ErrNoReport
	}

	return &Report{Message: text}, prompt, nil
}

// ResetLastAlert clears the deduplication state and alert history so the next
// report is always emitted. Call this at the start of each new match.
// Safe to call on a nil Commander.
func (c *Commander) ResetLastAlert() {
	if c == nil {
		return
	}

	c.mu.Lock()
	c.lastAlert = ""
	c.alertHistory = c.alertHistory[:0]
	c.mu.Unlock()
}

// appendHistory appends text to alertHistory, keeping at most alertHistoryMax
// elements. Must be called with c.mu held.
func (c *Commander) appendHistory(text string) {
	c.alertHistory = append(c.alertHistory, text)
	if len(c.alertHistory) > c.alertHistoryMax {
		c.alertHistory = c.alertHistory[len(c.alertHistory)-c.alertHistoryMax:]
	}
}

// formattedHistory returns the alert history formatted as a "[N reports ago] ..."
// list for inclusion in the system prompt. Returns "" when the history is empty.
// Safe to call without holding c.mu.
func (c *Commander) formattedHistory() string {
	c.mu.Lock()
	history := make([]string, len(c.alertHistory))
	copy(history, c.alertHistory)
	c.mu.Unlock()

	if len(history) == 0 {
		return ""
	}

	var builder strings.Builder

	for i := len(history) - 1; i >= 0; i-- {
		count := len(history) - i

		label := "reports"
		if count == 1 {
			label = "report"
		}

		fmt.Fprintf(&builder, "[%d %s ago] %s\n", count, label, history[i])
	}

	return strings.TrimRight(builder.String(), "\n")
}

// gridRef converts normalised [0,1] map_obj coordinates to a minimap grid
// reference such as "D6" using the live grid geometry from map_info.json.
//
// Coordinate conventions (confirmed from API data):
//   - Positions (X, Y): Y=0 at south edge, increases north (mathematical convention).
//   - Headings (DX, DY): DY>0 = north (same mathematical convention as positions).
//   - GridZero: map-space coordinate of the north-west (top-left) grid corner.
//   - GridSteps: size of one grid cell in map-space units.
//   - Columns A, B, C… increase going east (positive X).
//   - Rows 1, 2, 3… increase going south (decreasing Y).
//
// Falls back to a legacy 8×8 grid when info is nil or lacks grid geometry.
func gridRef(coordinateX, coordinateY float64, info *wt.MapInfo) string {
	if info == nil || info.GridSteps[0] <= 0 || info.MapMax[0] <= 0 {
		return legacyGridRef(coordinateX, coordinateY)
	}

	mapW := info.MapMax[0] - info.MapMin[0]
	mapH := info.MapMax[1] - info.MapMin[1]

	worldX := coordinateX*mapW + info.MapMin[0]
	worldY := coordinateY*mapH + info.MapMin[1]

	col := int((worldX - info.GridZero[0]) / info.GridSteps[0])
	// Y increases north; GridZero is the north (high-Y) edge; rows go south.
	row := int((info.GridZero[1] - worldY) / info.GridSteps[1])

	numCols, numRows := info.GridDims()

	if col < 0 {
		col = 0
	}

	if row < 0 {
		row = 0
	}

	if col >= numCols {
		col = numCols - 1
	}

	if row >= numRows {
		row = numRows - 1
	}

	return string(rune('A'+col)) + strconv.Itoa(row+1)
}

// legacyGridRef is a fallback for when map_info grid geometry is unavailable.
func legacyGridRef(coordinateX, coordinateY float64) string {
	const (
		legacyCols = 8
		legacyRows = 8
	)

	col := int(coordinateX * legacyCols)
	row := int((1 - coordinateY) * legacyRows)

	if col >= legacyCols {
		col = legacyCols - 1
	}

	if row >= legacyRows {
		row = legacyRows - 1
	}

	if col < 0 {
		col = 0
	}

	if row < 0 {
		row = 0
	}

	return string(rune('A'+col)) + strconv.Itoa(row+1)
}

// buildPrompt serialises the 30-second battlefield summary into a situation
// report that gives the LLM movement history for flank and camping prediction.
func (c *Commander) buildPrompt(sum *collector.Summary, mapInfo *wt.MapInfo) string {
	phrases := c.lang.GetPhrases()

	if sum == nil {
		if hist := c.formattedHistory(); hist != "" {
			return phrases.NoData + "\n\nPrevious alerts:\n" + hist + "\n"
		}

		return phrases.NoData
	}

	var builder strings.Builder

	if mapInfo != nil && mapInfo.MapName != "" {
		builder.WriteString(phrases.MapPrefix + mapInfo.MapName + "\n")
	}

	fmt.Fprintf(&builder, phrases.MatchTypeFmt, sum.MatchType)

	windowSecs := int(sum.WindowEnd.Sub(sum.WindowStart).Seconds())
	fmt.Fprintf(&builder, phrases.SummaryFmt, windowSecs)

	if sum.Player != nil {
		// DY > 0 = north (math convention); negate before atan2 to get compass bearing.
		bearing := math.Mod(math.Atan2(-sum.Player.DY, sum.Player.DX)*180/math.Pi+90+360, 360)
		heading := c.lang.CompassDir(bearing)
		fmt.Fprintf(&builder, phrases.PlayerFmt, gridRef(sum.Player.X, sum.Player.Y, mapInfo), heading)
	} else {
		builder.WriteString(phrases.PlayerHidden)
	}

	c.buildAlliesSection(&builder, phrases, sum, mapInfo)
	c.buildSquadSection(&builder, phrases, sum, mapInfo)
	c.buildEnemiesSection(&builder, phrases, sum, mapInfo)
	c.buildZonesSection(&builder, phrases, sum, mapInfo)

	if hist := c.formattedHistory(); hist != "" {
		builder.WriteString("\nPrevious alerts:\n")
		builder.WriteString(hist)
		builder.WriteString("\n")
	}

	return builder.String()
}

func (c *Commander) buildAlliesSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary, mapInfo *wt.MapInfo) {
	if len(sum.Allies) == 0 {
		builder.WriteString(phrases.AlliesNone)

		return
	}

	fmt.Fprintf(builder, phrases.AlliesFmt, len(sum.Allies))

	for idx := range sum.Allies {
		ally := &sum.Allies[idx]
		line := fmt.Sprintf(phrases.AllyFmt, c.lang.IconName(ally.Icon), gridRef(ally.X, ally.Y, mapInfo))

		if sum.Player != nil {
			line += fmt.Sprintf(phrases.AllyDistFmt, wt.Dist(sum.Player, ally), c.relativeDir(sum.Player, ally))
		}

		builder.WriteString(line + "\n")
	}
}

func (c *Commander) buildSquadSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary, mapInfo *wt.MapInfo) {
	if len(sum.Squad) == 0 {
		builder.WriteString(phrases.SquadNone)

		return
	}

	fmt.Fprintf(builder, phrases.SquadFmt, len(sum.Squad))

	for _, squadTrack := range sum.Squad {
		line := fmt.Sprintf(phrases.SquadMbrFmt, c.lang.IconName(squadTrack.Icon), gridRef(squadTrack.Last.X, squadTrack.Last.Y, mapInfo))

		if sum.Player != nil {
			line += fmt.Sprintf(phrases.SquadDistFmt, wt.Dist(sum.Player, &squadTrack.Last), c.relativeDir(sum.Player, &squadTrack.Last))
		}

		if squadTrack.IsStationary() {
			line += phrases.Stationary
		} else {
			dx, dy := squadTrack.Displacement()
			dist := math.Hypot(dx, dy)
			bearing := math.Mod(math.Atan2(-dy, dx)*180/math.Pi+90+360, 360)
			dir := c.lang.CompassDir(bearing)
			line += fmt.Sprintf(phrases.MovingFmt, dir, dist)
		}

		builder.WriteString(line + "\n")
	}
}

func (c *Commander) buildEnemiesSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary, mapInfo *wt.MapInfo) {
	if len(sum.Enemies) == 0 {
		builder.WriteString(phrases.EnemiesNone)

		return
	}

	fmt.Fprintf(builder, phrases.EnemiesFmt, len(sum.Enemies))

	for _, enemyTrack := range sum.Enemies {
		stationary := enemyTrack.IsStationary()

		var gridPart string
		if stationary {
			gridPart = gridRef(enemyTrack.Last.X, enemyTrack.Last.Y, mapInfo)
		} else {
			gridPart = fmt.Sprintf("%s (from %s, %d frames)",
				gridRef(enemyTrack.Last.X, enemyTrack.Last.Y, mapInfo),
				gridRef(enemyTrack.First.X, enemyTrack.First.Y, mapInfo),
				enemyTrack.FrameCount)
		}

		line := fmt.Sprintf(phrases.EnemyFmt, c.lang.IconName(enemyTrack.Icon), gridPart)

		if sum.Player != nil {
			line += fmt.Sprintf(phrases.EnemyDistFmt, wt.Dist(sum.Player, &enemyTrack.Last), c.relativeDir(sum.Player, &enemyTrack.Last))
		}

		if stationary {
			line += phrases.Stationary
		} else {
			dx, dy := enemyTrack.Displacement()
			dist := math.Hypot(dx, dy)
			bearing := math.Mod(math.Atan2(-dy, dx)*180/math.Pi+90+360, 360)
			dir := c.lang.CompassDir(bearing)
			line += fmt.Sprintf(phrases.MovingFmt, dir, dist)

			if sum.Player != nil {
				if threat := classifyFlankThreat(enemyTrack, sum.Player); threat != "" {
					line += " → " + threat
				}
			}
		}

		builder.WriteString(line + "\n")
	}
}

// classifyFlankThreat returns a string describing how an enemy track threatens
// the player based on its trajectory relative to the player's heading.
// Returns "" when there is insufficient data: FrameCount < 3 or the enemy is
// stationary (net displacement below campingThreshold).
//
// The five possible return values are:
//
//	"closing on left flank", "closing on right flank",
//	"approaching rear", "approaching front", "moving away"
func classifyFlankThreat(track collector.EnemyTrack, player *wt.MapObject) string { //nolint:cyclop
	if track.FrameCount < 3 || track.IsStationary() {
		return ""
	}

	// Use distance change over the window to determine approach direction.
	// This is more robust than a dot-product check, which can be falsely positive
	// when diagonal movement has a small converging component in one axis while
	// the dominant motion is away from the player.
	if wt.Dist(&track.Last, player) >= wt.Dist(&track.First, player) {
		return "moving away"
	}

	// Classify the approach direction relative to the player's heading.
	// Both positions and heading use math convention (DY > 0 = north).
	ex := track.Last.X - player.X
	ey := track.Last.Y - player.Y
	dot := player.DX*ex + player.DY*ey
	cross := player.DY*ex - player.DX*ey
	angle := math.Atan2(cross, dot) * 180 / math.Pi

	switch {
	case angle > -22.5 && angle <= 22.5:
		return "approaching front"
	case angle > 22.5 && angle <= 112.5:
		return "closing on right flank"
	case angle > -112.5 && angle <= -22.5:
		return "closing on left flank"
	default:
		return "approaching rear"
	}
}

func (c *Commander) buildZonesSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary, mapInfo *wt.MapInfo) {
	if len(sum.Zones) == 0 {
		return
	}

	sorted := make([]wt.MapObject, len(sum.Zones))
	copy(sorted, sum.Zones)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].X < sorted[j].X })
	builder.WriteString(phrases.ZonesHeader)

	for idx, zone := range sorted {
		status := phrases.Neutral
		if zone.Blink == 1 {
			status = phrases.Contested
		}

		fmt.Fprintf(builder, phrases.ZoneFmt, string(rune('A'+idx)), status, gridRef(zone.X, zone.Y, mapInfo))
	}
}

// relativeDir returns the enemy's bearing relative to the player's heading as
// a localised label suitable for LLM prompts.
// For English it returns a clock-position string ("twelve o'clock", "three o'clock"…).
// Uses math convention: DY > 0 = north, consistent with relativeAngle in analyzer.go.
func (c *Commander) relativeDir(player, enemy *wt.MapObject) string {
	ex := enemy.X - player.X
	ey := enemy.Y - player.Y
	dot := player.DX*ex + player.DY*ey
	cross := player.DY*ex - player.DX*ey
	angle := math.Atan2(cross, dot) * 180 / math.Pi

	return c.lang.PromptRelativeDir(angle)
}
