package commander //nolint:testpackage

import (
	"testing"

	"github.com/tecnologer/warthunder/internal/collector"
	"github.com/tecnologer/warthunder/internal/wt"
)

// playerNorth is a player at map centre facing north.
// DX=0, DY=+1 uses math convention (DY > 0 = north), matching the WT API and
// the relativeAngle() convention in analyzer.go.
var playerNorth = wt.MapObject{X: 0.5, Y: 0.5, DX: 0, DY: 1} //nolint:gochecknoglobals

func TestClassifyFlankThreat(t *testing.T) { //nolint:funlen
	t.Parallel()

	tests := []struct {
		name   string
		track  collector.EnemyTrack
		player wt.MapObject
		want   string
	}{
		{
			name: "closing on left flank",
			// Enemy west of north-facing player, moving east toward them.
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.35, Y: 0.5},
				Last:       wt.MapObject{X: 0.40, Y: 0.5},
				FrameCount: 5,
			},
			player: playerNorth,
			want:   "closing on left flank",
		},
		{
			name: "closing on right flank",
			// Enemy east of north-facing player, moving west toward them.
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.65, Y: 0.5},
				Last:       wt.MapObject{X: 0.60, Y: 0.5},
				FrameCount: 5,
			},
			player: playerNorth,
			want:   "closing on right flank",
		},
		{
			name: "approaching rear",
			// Enemy south of north-facing player (y increases north), moving north.
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.5, Y: 0.30},
				Last:       wt.MapObject{X: 0.5, Y: 0.35},
				FrameCount: 5,
			},
			player: playerNorth,
			want:   "approaching rear",
		},
		{
			name: "approaching front",
			// Enemy north of north-facing player, moving south toward them.
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.5, Y: 0.65},
				Last:       wt.MapObject{X: 0.5, Y: 0.60},
				FrameCount: 5,
			},
			player: playerNorth,
			want:   "approaching front",
		},
		{
			name: "moving away",
			// Enemy west of player, moving further west (away).
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.35, Y: 0.5},
				Last:       wt.MapObject{X: 0.30, Y: 0.5},
				FrameCount: 5,
			},
			player: playerNorth,
			want:   "moving away",
		},
		{
			name: "FrameCount below threshold returns empty string",
			// Same geometry as left-flank case but only 2 frames observed.
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.35, Y: 0.5},
				Last:       wt.MapObject{X: 0.40, Y: 0.5},
				FrameCount: 2,
			},
			player: playerNorth,
			want:   "",
		},
		{
			name: "stationary enemy returns empty string",
			// Net displacement well below campingThreshold (0.02).
			track: collector.EnemyTrack{
				First:      wt.MapObject{X: 0.5000, Y: 0.5000},
				Last:       wt.MapObject{X: 0.5005, Y: 0.5005},
				FrameCount: 10,
			},
			player: playerNorth,
			want:   "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := classifyFlankThreat(test.track, &test.player)
			if got != test.want {
				t.Errorf("classifyFlankThreat() = %q, want %q", got, test.want)
			}
		})
	}
}
