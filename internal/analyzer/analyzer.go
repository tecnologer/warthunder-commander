package analyzer

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tecnologer/warthunder/internal/lang"
	"github.com/tecnologer/warthunder/internal/wt"
)

const (
	flankDistance       = 0.15
	captureZoneRadius   = 0.08
	flankAngleThreshold = 90.0 // degrees
	newEnemyRadius      = 0.05 // max distance an enemy can move per frame and still be considered the same unit

	trackDuration      = 30 * time.Second // how long to remember a spotted enemy
	trackDurationClose = 60 * time.Second // extended tracking when enemy was within flankDistance

	// alertSilence is the minimum time between repeated alerts for the same
	// ongoing event (same enemy flanking, same zone under pressure).
	alertSilence = 30 * time.Second

	// groupingWindow is how long to wait after the last new-enemy detection
	// before flushing all pending detections as a single grouped alert.
	groupingWindow = 1 * time.Second
)

// Priority levels for alerts.
const (
	PriorityInfo      = 1
	PriorityWarning   = 2
	PriorityCritical  = 3
	PriorityCommander = 4 // AI tactical-commander reports
)

// Alert represents a single voice alert to be spoken.
type Alert struct {
	Priority int
	Message  string
}

// trackedEnemy is a known enemy with position history and metadata.
type trackedEnemy struct {
	obj            wt.MapObject
	prevObj        wt.MapObject // position from the previous frame, used to compute movement direction
	lastSeen       time.Time
	wasClose       bool
	confirmed      bool      // false on the first frame; true once seen in a second consecutive frame
	lastFlankAlert time.Time // zero if not currently in a flank event
}

// pendingEnemy holds the data for a newly confirmed enemy waiting to be grouped.
type pendingEnemy struct {
	icon string
	dir  string // movement direction label, empty if stationary
}

// Analyzer detects tactical events across frames.
type Analyzer struct {
	lang       lang.Language
	tracked    []trackedEnemy
	zoneLabels map[string]string    // PosKey → "A", "B", "C"… assigned once per match
	zoneAlerts map[string]time.Time // PosKey → time of last zone pressure alert

	pendingNew   []pendingEnemy // newly confirmed enemies waiting to be grouped
	lastNewEnemy time.Time      // time the most recent enemy was added to pendingNew
}

// New returns a ready Analyzer for the given language.
func New(language lang.Language) *Analyzer {
	return &Analyzer{lang: language}
}

// allyDetectionRadius is the maximum distance an ally must be from an enemy for
// that enemy to be considered "spotted" in Realistic mode.
const allyDetectionRadius = 0.20

// scanObjects classifies map objects into player, enemies, allies, and zones.
func scanObjects(objs []wt.MapObject) (*wt.MapObject, []wt.MapObject, []wt.MapObject, []wt.MapObject) {
	var player *wt.MapObject

	var enemies, allies, zones []wt.MapObject

	for idx := range objs {
		obj := &objs[idx]
		switch {
		case obj.IsPlayer():
			player = obj
		case obj.IsEnemy():
			enemies = append(enemies, *obj)
		case obj.IsCaptureZone():
			zones = append(zones, *obj)
		case obj.Team() == wt.TeamAlly || obj.Team() == wt.TeamSquad:
			allies = append(allies, *obj)
		}
	}

	return player, enemies, allies, zones
}

// filterByMode applies mode-specific enemy filtering.
func (a *Analyzer) filterByMode(enemies, allies []wt.MapObject, player *wt.MapObject, mode wt.GameMode) []wt.MapObject {
	switch mode {
	case wt.GameModeSimulator:
		// Enemy positions are never reliable in Simulator — suppress all enemy alerts.
		return nil
	case wt.GameModeRealistic:
		// In Realistic, an enemy is only visible on the map when an ally is actively
		// spotting it. Keep only enemies with at least one ally within allyDetectionRadius.
		return filterSpottedEnemies(enemies, allies, player)
	case wt.GameModeArcade:
		// Arcade mode: all enemy positions are reliable; no filtering needed.
		return enemies
	default:
		return enemies
	}
}

// expireTracked removes tracked enemies that haven't been seen within their retention window.
func (a *Analyzer) expireTracked(now time.Time) {
	active := a.tracked[:0]
	for _, tracked := range a.tracked {
		ttl := trackDuration
		if tracked.wasClose {
			ttl = trackDurationClose
		}

		if now.Sub(tracked.lastSeen) <= ttl {
			active = append(active, tracked)
		}
	}

	a.tracked = active
}

// processFlankEnemy handles flank detection for a single enemy, updating tracking
// state and returning a flank alert if the enemy is in a new or re-entered flank position.
func (a *Analyzer) processFlankEnemy(player, enemy *wt.MapObject, now time.Time, mode wt.GameMode, best *Alert) *Alert {
	if mode == wt.GameModeRealistic && !lang.IsIdentifiableIcon(enemy.Icon) {
		return best
	}

	inRange := wt.Dist(player, enemy) <= flankDistance
	angle := relativeAngle(player, enemy)
	inFlank := inRange && math.Abs(angle) > flankAngleThreshold

	idx := a.findTracked(enemy)
	if idx >= 0 {
		if !inFlank {
			// Enemy left the flank zone — reset so next entry fires fresh.
			a.tracked[idx].lastFlankAlert = time.Time{}

			return best
		}

		if !a.tracked[idx].lastFlankAlert.IsZero() &&
			now.Sub(a.tracked[idx].lastFlankAlert) < alertSilence {
			return best // still within the silence window
		}

		a.tracked[idx].lastFlankAlert = now
	} else if !inFlank {
		return best
	}

	side := a.lang.FlankSide(angle)

	return highest(best, &Alert{
		Priority: PriorityCritical,
		Message:  a.lang.FlankAlert(side, a.lang.IconName(enemy.Icon)),
	})
}

// processFlankAlerts runs flank detection for all enemies and returns the best alert found.
func (a *Analyzer) processFlankAlerts(player *wt.MapObject, enemies []wt.MapObject, now time.Time, mode wt.GameMode) *Alert {
	if player == nil {
		return nil
	}

	var best *Alert

	for idx := range enemies {
		best = a.processFlankEnemy(player, &enemies[idx], now, mode, best)
	}

	return best
}

// updateKnownEnemy refreshes position tracking for an already-tracked enemy
// and queues it in pendingNew once confirmed on a second frame.
func (a *Analyzer) updateKnownEnemy(trackIdx int, enemy *wt.MapObject, isClose bool, now time.Time, mode wt.GameMode) {
	prev := a.tracked[trackIdx].obj
	a.tracked[trackIdx].prevObj = prev
	a.tracked[trackIdx].obj = *enemy
	a.tracked[trackIdx].lastSeen = now

	if isClose {
		a.tracked[trackIdx].wasClose = true
	}

	if a.tracked[trackIdx].confirmed {
		return
	}

	a.tracked[trackIdx].confirmed = true

	// In Realistic mode, only alert when the enemy type can be identified.
	if mode != wt.GameModeRealistic || lang.IsIdentifiableIcon(enemy.Icon) {
		a.pendingNew = append(a.pendingNew, pendingEnemy{
			icon: enemy.Icon,
			dir:  a.movementDir(prev, *enemy),
		})
		a.lastNewEnemy = now
	}
}

// updateTrackedEnemies updates position tracking for all current enemies
// and queues newly confirmed enemies in pendingNew.
func (a *Analyzer) updateTrackedEnemies(player *wt.MapObject, enemies []wt.MapObject, now time.Time, mode wt.GameMode) {
	for idx := range enemies {
		enemy := &enemies[idx]
		isClose := player != nil && wt.Dist(player, enemy) <= flankDistance

		if trackIdx := a.findTracked(enemy); trackIdx >= 0 {
			a.updateKnownEnemy(trackIdx, enemy, isClose, now, mode)
		} else {
			// First time seeing this enemy — register but wait one frame for direction.
			a.tracked = append(a.tracked, trackedEnemy{
				obj:      *enemy,
				prevObj:  *enemy,
				lastSeen: now,
				wasClose: isClose,
			})
		}
	}
}

// flushPendingAlerts flushes pending new-enemy detections as a single grouped
// alert once the grouping window has elapsed since the last addition.
// Returns the flush alert if it should be emitted (and clears pendingNew), nil otherwise.
func (a *Analyzer) flushPendingAlerts(now time.Time) *Alert {
	if len(a.pendingNew) == 0 || now.Sub(a.lastNewEnemy) < groupingWindow {
		return nil
	}

	alert := &Alert{Priority: PriorityWarning, Message: a.groupedDetectionMsg()}
	a.pendingNew = nil

	return alert
}

// processZoneAlerts checks contested capture zones for nearby enemies and
// returns the best zone pressure alert, respecting the alertSilence window.
func (a *Analyzer) processZoneAlerts(zones, enemies []wt.MapObject, now time.Time) *Alert {
	if len(zones) > 0 && len(a.zoneLabels) == 0 {
		a.zoneLabels = assignZoneLabels(zones)
	}

	if a.zoneAlerts == nil {
		a.zoneAlerts = make(map[string]time.Time)
	}

	var best *Alert

	for idx := range zones {
		zone := &zones[idx]
		if zone.Blink != 1 {
			// Zone is no longer contested — reset its silence timer.
			delete(a.zoneAlerts, zone.PosKey())

			continue
		}

		enemy := closestEnemyInZone(zone, enemies)
		if enemy == nil {
			continue
		}

		key := zone.PosKey()
		if lastAlert, seen := a.zoneAlerts[key]; seen && now.Sub(lastAlert) < alertSilence {
			continue // still within the silence window
		}

		a.zoneAlerts[key] = now
		label := a.zoneLabels[key]
		best = highest(best, &Alert{
			Priority: PriorityWarning,
			Message:  a.lang.ZoneEnemyAlert(label, a.lang.IconName(enemy.Icon)),
		})
	}

	return best
}

// closestEnemyInZone returns the closest enemy within captureZoneRadius of zone,
// or nil when no such enemy exists.
func closestEnemyInZone(zone *wt.MapObject, enemies []wt.MapObject) *wt.MapObject {
	var closest *wt.MapObject

	minDist := captureZoneRadius

	for idx := range enemies {
		if d := wt.Dist(zone, &enemies[idx]); d < minDist {
			minDist = d
			closest = &enemies[idx]
		}
	}

	return closest
}

// Analyze computes the highest-priority alert for the current frame.
// mode controls which alerts are active (see GameMode constants).
// Returns nil when there is nothing to report.
func (a *Analyzer) Analyze(objs []wt.MapObject, mode wt.GameMode) *Alert {
	player, enemies, allies, zones := scanObjects(objs)

	enemies = a.filterByMode(enemies, allies, player, mode)

	now := time.Now()
	a.expireTracked(now)

	best := a.processFlankAlerts(player, enemies, now, mode)
	a.updateTrackedEnemies(player, enemies, now, mode)

	if flushAlert := a.flushPendingAlerts(now); flushAlert != nil {
		best = highest(best, flushAlert)
	}

	return highest(best, a.processZoneAlerts(zones, enemies, now))
}

// iconEntry tracks occurrences of a single icon type in pendingNew.
type iconEntry struct {
	icon  string
	count int
}

// countPendingIcons counts occurrences per icon type in pendingNew,
// preserving first-seen order.
func (a *Analyzer) countPendingIcons() ([]string, map[string]*iconEntry) {
	order := make([]string, 0, len(a.pendingNew))
	counts := make(map[string]*iconEntry, len(a.pendingNew))

	for _, pending := range a.pendingNew {
		if _, exists := counts[pending.icon]; !exists {
			order = append(order, pending.icon)
			counts[pending.icon] = &iconEntry{icon: pending.icon}
		}

		counts[pending.icon].count++
	}

	return order, counts
}

// buildDetectionSegments turns the ordered icon counts into human-readable
// segments such as "Fighter" or "three Medium tanks".
func (a *Analyzer) buildDetectionSegments(order []string, counts map[string]*iconEntry) []string {
	segments := make([]string, 0, len(order))

	for _, icon := range order {
		entry := counts[icon]
		if entry.count == 1 {
			segments = append(segments, a.lang.IconName(icon))
		} else {
			segments = append(segments, a.lang.Count(entry.count)+" "+a.lang.IconNamePlural(icon))
		}
	}

	return segments
}

// groupedDetectionMsg builds a single alert string for all pending new enemies.
// Same vehicle types are counted and named in plural
// (e.g. "three Medium tanks and Fighter detected, moving northeast").
func (a *Analyzer) groupedDetectionMsg() string {
	order, counts := a.countPendingIcons()
	segments := a.buildDetectionSegments(order, counts)

	// Assemble "A, B and C" / "A, B y C".
	conjunction := " y "
	if a.lang == lang.EN {
		conjunction = " and "
	}

	var builder strings.Builder

	for idx, seg := range segments {
		if idx > 0 {
			if idx == len(segments)-1 {
				builder.WriteString(conjunction)
			} else {
				builder.WriteString(", ")
			}
		}

		builder.WriteString(seg)
	}

	builder.WriteString(a.lang.DetectedSuffix(len(a.pendingNew)))

	// Append direction only when every pending enemy shares the same one.
	commonDir := a.pendingNew[0].dir
	allSame := commonDir != ""

	for _, pending := range a.pendingNew[1:] {
		if pending.dir != commonDir {
			allSame = false

			break
		}
	}

	if allSame {
		builder.WriteString(a.lang.MovingLabel(commonDir))
	}

	return builder.String()
}

// findTracked returns the index of the tracked enemy within newEnemyRadius of e,
// or -1 if none found.
func (a *Analyzer) findTracked(enemy *wt.MapObject) int {
	for idx := range a.tracked {
		if wt.Dist(enemy, &a.tracked[idx].obj) <= newEnemyRadius {
			return idx
		}
	}

	return -1
}

// movementDir returns the localised compass direction of movement from src to dst,
// or an empty string when movement is below minMovement.
// Assumes map coordinates where +x = east and +y = south (y increases downward).
func (a *Analyzer) movementDir(src, dst wt.MapObject) string {
	return a.lang.MovementDir(dst.X-src.X, dst.Y-src.Y)
}

// relativeAngle returns the bearing of enemy relative to player's heading in degrees.
// 0° = straight ahead, positive = right, negative = left, ±180° = behind.
//
// Coordinate system note: map positions use math convention (y=0 south, y
// increases north) while the heading vector (DX, DY) uses screen convention
// (DY > 0 means heading south). The dot and cross products below account for
// this mismatch by negating the Y component of the heading when projecting
// onto the map-position offset.
func relativeAngle(player, enemy *wt.MapObject) float64 {
	ex := enemy.X - player.X
	ey := enemy.Y - player.Y
	// Convert heading to map convention: map_hy = -DY
	dot := player.DX*ex - player.DY*ey
	cross := -player.DY*ex - player.DX*ey

	return math.Atan2(cross, dot) * 180 / math.Pi
}

// assignZoneLabels sorts zones left-to-right by X coordinate and assigns
// letters A, B, C… to each. Returns a map from PosKey to label.
func assignZoneLabels(zones []wt.MapObject) map[string]string {
	sorted := make([]wt.MapObject, len(zones))
	copy(sorted, zones)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].X < sorted[j].X
	})

	labels := make(map[string]string, len(sorted))
	for idx, zone := range sorted {
		labels[zone.PosKey()] = string(rune('A' + idx))
	}

	return labels
}

func highest(current, candidate *Alert) *Alert {
	if candidate == nil {
		return current
	}

	if current == nil || candidate.Priority > current.Priority {
		return candidate
	}

	return current
}

// filterSpottedEnemies returns only those enemies that have at least one ally
// (or the player) within allyDetectionRadius.  This models Realistic-mode
// spotting: an enemy dot appears on the minimap only while someone on your team
// has line-of-sight to it.
func filterSpottedEnemies(enemies, allies []wt.MapObject, player *wt.MapObject) []wt.MapObject {
	spotted := enemies[:0:0] // nil-safe empty slice, shares no backing array

	for idx := range enemies {
		enemy := &enemies[idx]
		if player != nil && wt.Dist(player, enemy) <= allyDetectionRadius {
			spotted = append(spotted, *enemy)

			continue
		}

		for allyIdx := range allies {
			if wt.Dist(&allies[allyIdx], enemy) <= allyDetectionRadius {
				spotted = append(spotted, *enemy)

				break
			}
		}
	}

	return spotted
}
