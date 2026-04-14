package collector

import (
	"math"
	"sync"
	"time"

	"github.com/tecnologer/warthunder/internal/wt"
)

const (
	// matchRadius is the max distance to associate the same enemy across frames.
	matchRadius = 0.03
	// campingThreshold: net displacement below this over the window = camping.
	campingThreshold = 0.02
)

// Frame is a single poll snapshot.
type Frame struct {
	At      time.Time
	Objects []wt.MapObject
}

// EnemyTrack summarises one enemy's movement over the collection window.
type EnemyTrack struct {
	Icon       string
	First      wt.MapObject // earliest position in window
	Last       wt.MapObject // most recent position
	FrameCount int          // number of frames this enemy was visible
}

// IsStationary reports whether the enemy barely moved (camping behaviour).
func (track EnemyTrack) IsStationary() bool {
	return wt.Dist(&track.First, &track.Last) < campingThreshold
}

// Displacement returns the net (dx, dy) movement vector over the window.
func (track EnemyTrack) Displacement() (float64, float64) {
	return track.Last.X - track.First.X, track.Last.Y - track.First.Y
}

// SquadTrack summarises one squad member's movement over the collection window.
type SquadTrack struct {
	Icon       string
	First      wt.MapObject
	Last       wt.MapObject
	FrameCount int
}

// IsStationary reports whether the squad member barely moved.
func (track SquadTrack) IsStationary() bool {
	return wt.Dist(&track.First, &track.Last) < campingThreshold
}

// Displacement returns the net (dx, dy) movement vector over the window.
func (track SquadTrack) Displacement() (float64, float64) {
	return track.Last.X - track.First.X, track.Last.Y - track.First.Y
}

// Summary is the aggregated battlefield state over the collection window.
type Summary struct {
	Player      *wt.MapObject  // from the most recent frame
	Enemies     []EnemyTrack   // one entry per distinct enemy tracked
	Allies      []wt.MapObject // non-squad allies from the most recent frame
	Squad       []SquadTrack   // squad members tracked over the window
	Zones       []wt.MapObject // capture zones from the most recent frame
	WindowStart time.Time
	WindowEnd   time.Time
}

// Collector accumulates frames over a rolling time window.
type Collector struct {
	mu     sync.Mutex
	frames []Frame
	window time.Duration
}

// New returns a Collector with the given rolling window duration.
func New(window time.Duration) *Collector {
	return &Collector{window: window}
}

// Add appends a new frame and expires frames that fall outside the window.
func (c *Collector) Add(objs []wt.MapObject) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.frames = append(c.frames, Frame{At: now, Objects: objs})

	cutoff := now.Add(-c.window)
	idx := 0

	for idx < len(c.frames) && c.frames[idx].At.Before(cutoff) {
		idx++
	}

	if idx > 0 {
		c.frames = c.frames[idx:]
	}
}

// Summary builds an aggregated report from the current window.
// Returns nil if no frames have been collected yet.
func (c *Collector) Summary() *Summary {
	c.mu.Lock()
	frames := make([]Frame, len(c.frames))
	copy(frames, c.frames)
	c.mu.Unlock()

	if len(frames) == 0 {
		return nil
	}

	sum := &Summary{
		WindowStart: frames[0].At,
		WindowEnd:   frames[len(frames)-1].At,
	}

	// Player, allies, and zones come from the most recent frame.
	last := frames[len(frames)-1]
	for idx := range last.Objects {
		obj := &last.Objects[idx]
		switch {
		case obj.IsPlayer():
			sum.Player = new(*obj)
		case obj.IsCaptureZone():
			sum.Zones = append(sum.Zones, *obj)
		case obj.Team() == wt.TeamAlly:
			sum.Allies = append(sum.Allies, *obj)
		}
	}

	sum.Enemies = buildTracks(frames)
	sum.Squad = buildSquadTracks(frames)

	return sum
}

// internalSquad is a mutable squad track used only inside buildSquadTracks.
type internalSquad struct {
	icon       string
	first      wt.MapObject
	last       wt.MapObject
	frameCount int
	active     bool
}

// findNearestSquad finds the index of the nearest non-active squad track
// within matchRadius of pos. Returns -1 if none found.
func findNearestSquad(tracks []*internalSquad, pos *wt.MapObject) int {
	best := -1
	bestDist := math.MaxFloat64

	for trackIdx, track := range tracks {
		if track.active {
			continue
		}

		dist := wt.Dist(pos, &track.last)
		if dist < matchRadius && dist < bestDist {
			best = trackIdx
			bestDist = dist
		}
	}

	return best
}

// buildSquadTracks runs the same greedy nearest-neighbour tracker as
// buildTracks but for squad members.
func buildSquadTracks(frames []Frame) []SquadTrack {
	var tracks []*internalSquad

	for _, frame := range frames {
		var members []wt.MapObject

		for idx := range frame.Objects {
			if frame.Objects[idx].Team() == wt.TeamSquad {
				members = append(members, frame.Objects[idx])
			}
		}

		for _, track := range tracks {
			track.active = false
		}

		for idx := range members {
			member := members[idx]
			best := findNearestSquad(tracks, &member)

			if best >= 0 {
				tracks[best].last = member
				tracks[best].frameCount++
				tracks[best].active = true
			} else {
				tracks = append(tracks, &internalSquad{
					icon:       member.Icon,
					first:      member,
					last:       member,
					frameCount: 1,
					active:     true,
				})
			}
		}
	}

	result := make([]SquadTrack, 0, len(tracks))
	for _, track := range tracks {
		result = append(result, SquadTrack{
			Icon:       track.icon,
			First:      track.first,
			Last:       track.last,
			FrameCount: track.frameCount,
		})
	}

	return result
}

// internalTrack is a mutable enemy track used only inside buildTracks.
type internalTrack struct {
	icon       string
	first      wt.MapObject
	last       wt.MapObject
	frameCount int
	active     bool // matched in the current frame pass
}

// findNearestInternalTrack finds the index of the nearest non-active track
// within matchRadius of pos. Returns -1 if none found.
func findNearestInternalTrack(tracks []*internalTrack, pos *wt.MapObject) int {
	best := -1
	bestDist := math.MaxFloat64

	for trackIdx, track := range tracks {
		if track.active {
			continue
		}

		dist := wt.Dist(pos, &track.last)
		if dist < matchRadius && dist < bestDist {
			best = trackIdx
			bestDist = dist
		}
	}

	return best
}

// buildTracks runs a greedy nearest-neighbour tracker over the frame sequence
// and returns one EnemyTrack per distinct enemy observed.
func buildTracks(frames []Frame) []EnemyTrack {
	var tracks []*internalTrack

	for _, frame := range frames {
		var enemies []wt.MapObject

		for idx := range frame.Objects {
			if frame.Objects[idx].IsEnemy() {
				enemies = append(enemies, frame.Objects[idx])
			}
		}

		for _, track := range tracks {
			track.active = false
		}

		for idx := range enemies {
			enemy := enemies[idx]
			best := findNearestInternalTrack(tracks, &enemy)

			if best >= 0 {
				tracks[best].last = enemy
				tracks[best].frameCount++
				tracks[best].active = true
			} else {
				tracks = append(tracks, &internalTrack{
					icon:       enemy.Icon,
					first:      enemy,
					last:       enemy,
					frameCount: 1,
					active:     true,
				})
			}
		}
	}

	result := make([]EnemyTrack, 0, len(tracks))
	for _, track := range tracks {
		result = append(result, EnemyTrack{
			Icon:       track.icon,
			First:      track.first,
			Last:       track.last,
			FrameCount: track.frameCount,
		})
	}

	return result
}
