package commander

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

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
	llm          backend
	lang         lang.Language
	systemPrompt string
}

// Report holds a tactical recommendation.
type Report struct {
	Message string
}

// New returns a Commander. It selects the backend specified by aiCfg.Engine
// ("groq" by default, "anthropic" as the alternative).
func New(aiCfg config.AIConfig, language lang.Language) *Commander {
	var llm backend
	if aiCfg.Engine == config.AIEngineAnthropic {
		llm = newAnthropicBackend(aiCfg.AnthropicEnv)
	} else {
		llm = newGroqBackend(aiCfg.GroqEnv)
	}

	callsign := aiCfg.Callsign
	if callsign == "" {
		callsign = "Bronco"
	}

	return &Commander{llm: llm, lang: language, systemPrompt: language.SystemPrompt(callsign)}
}

// Advise builds a tactical prompt from the 30-second summary, calls the
// backend, and returns a tactical report. Returns nil, ErrNoReport when there
// is nothing actionable to report.
func (c *Commander) Advise(ctx context.Context, sum *collector.Summary, mapInfo *wt.MapInfo) (*Report, error) {
	prompt := c.buildPrompt(sum, mapInfo)

	text, err := c.llm.complete(ctx, c.systemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrNoReport
	}

	return &Report{Message: text}, nil
}

// gridCols and gridRows define the standard War Thunder minimap grid size.
// Most maps use 8×8; adjust per-map if needed in the future.
const (
	gridCols = 8
	gridRows = 8
)

// gridRef converts normalised [0,1] coordinates to a minimap grid reference
// such as "E3". Clamps values that fall on or past the boundary.
func gridRef(x, y float64) string {
	col := int(x * gridCols)
	row := int(y * gridRows)

	if col >= gridCols {
		col = gridCols - 1
	}

	if row >= gridRows {
		row = gridRows - 1
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
		return phrases.NoData
	}

	var builder strings.Builder

	if mapInfo != nil && mapInfo.MapName != "" {
		builder.WriteString(phrases.MapPrefix + mapInfo.MapName + "\n")
	}

	windowSecs := int(sum.WindowEnd.Sub(sum.WindowStart).Seconds())
	fmt.Fprintf(&builder, phrases.SummaryFmt, windowSecs)

	if sum.Player != nil {
		bearing := math.Mod(math.Atan2(sum.Player.DY, sum.Player.DX)*180/math.Pi+90+360, 360)
		heading := c.lang.CompassDir(bearing)
		fmt.Fprintf(&builder, phrases.PlayerFmt, gridRef(sum.Player.X, sum.Player.Y), heading)
	} else {
		builder.WriteString(phrases.PlayerHidden)
	}

	c.buildAlliesSection(&builder, phrases, sum)
	c.buildSquadSection(&builder, phrases, sum)
	c.buildEnemiesSection(&builder, phrases, sum)
	c.buildZonesSection(&builder, phrases, sum)

	return builder.String()
}

func (c *Commander) buildAlliesSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary) {
	if len(sum.Allies) == 0 {
		builder.WriteString(phrases.AlliesNone)

		return
	}

	fmt.Fprintf(builder, phrases.AlliesFmt, len(sum.Allies))

	for idx := range sum.Allies {
		ally := &sum.Allies[idx]
		line := fmt.Sprintf(phrases.AllyFmt, c.lang.IconName(ally.Icon), gridRef(ally.X, ally.Y))

		if sum.Player != nil {
			line += fmt.Sprintf(phrases.AllyDistFmt, wt.Dist(sum.Player, ally), c.relativeDir(sum.Player, ally))
		}

		builder.WriteString(line + "\n")
	}
}

func (c *Commander) buildSquadSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary) {
	if len(sum.Squad) == 0 {
		builder.WriteString(phrases.SquadNone)

		return
	}

	fmt.Fprintf(builder, phrases.SquadFmt, len(sum.Squad))

	for _, squadTrack := range sum.Squad {
		line := fmt.Sprintf(phrases.SquadMbrFmt, c.lang.IconName(squadTrack.Icon), gridRef(squadTrack.Last.X, squadTrack.Last.Y))

		if sum.Player != nil {
			line += fmt.Sprintf(phrases.SquadDistFmt, wt.Dist(sum.Player, &squadTrack.Last), c.relativeDir(sum.Player, &squadTrack.Last))
		}

		if squadTrack.IsStationary() {
			line += phrases.Stationary
		} else {
			dx, dy := squadTrack.Displacement()
			dist := math.Hypot(dx, dy)
			bearing := math.Mod(math.Atan2(dy, dx)*180/math.Pi+90+360, 360)
			dir := c.lang.CompassDir(bearing)
			line += fmt.Sprintf(phrases.MovingFmt, dir, dist)
		}

		builder.WriteString(line + "\n")
	}
}

func (c *Commander) buildEnemiesSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary) {
	if len(sum.Enemies) == 0 {
		builder.WriteString(phrases.EnemiesNone)

		return
	}

	fmt.Fprintf(builder, phrases.EnemiesFmt, len(sum.Enemies))

	for _, enemyTrack := range sum.Enemies {
		line := fmt.Sprintf(phrases.EnemyFmt, c.lang.IconName(enemyTrack.Icon), gridRef(enemyTrack.Last.X, enemyTrack.Last.Y))

		if sum.Player != nil {
			line += fmt.Sprintf(phrases.EnemyDistFmt, wt.Dist(sum.Player, &enemyTrack.Last), c.relativeDir(sum.Player, &enemyTrack.Last))
		}

		if enemyTrack.IsStationary() {
			line += phrases.Stationary
		} else {
			dx, dy := enemyTrack.Displacement()
			dist := math.Hypot(dx, dy)
			bearing := math.Mod(math.Atan2(dy, dx)*180/math.Pi+90+360, 360)
			dir := c.lang.CompassDir(bearing)
			line += fmt.Sprintf(phrases.MovingFmt, dir, dist)
		}

		builder.WriteString(line + "\n")
	}
}

func (c *Commander) buildZonesSection(builder *strings.Builder, phrases lang.Phrases, sum *collector.Summary) {
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

		fmt.Fprintf(builder, phrases.ZoneFmt, string(rune('A'+idx)), status, gridRef(zone.X, zone.Y))
	}
}

// relativeDir returns the enemy's bearing relative to the player's heading as
// a localised label.
func (c *Commander) relativeDir(player, enemy *wt.MapObject) string {
	ex := enemy.X - player.X
	ey := enemy.Y - player.Y
	dot := player.DX*ex + player.DY*ey
	cross := player.DX*ey - player.DY*ex
	angle := math.Atan2(cross, dot) * 180 / math.Pi

	return c.lang.RelativeDir(angle)
}
