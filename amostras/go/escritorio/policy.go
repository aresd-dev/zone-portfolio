// Amostra do Zone. Arquivo original: escritorio/office/gateway-go/communication/policy.go
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

// Package communication owns the office's transport-independent communication rules.
// Its decisions are eligibility only; they do not admit a WebRTC session or relay media.
package communication

import (
	"math"

	"zone-office/world"
)

const (
	// LocalTextRadius preserves the inclusive E06a speech boundary.
	LocalTextRadius = 6
	// MediaRadius is exclusive: a peer six cells away has no media link.
	MediaRadius    = 6
	FullGainRadius = 2
)

// Participant describes an authenticated participant whose office presence is live.
// The transport supplies Available after its admission and expiry checks.
type Participant struct {
	ID        string
	Position  world.Position
	Available bool
}

// Policy is an immutable view for one world snapshot. Locks maps each participant
// to their consented peer. Both reciprocal entries must exist to grant access.
// Callers must synchronize access to the map or provide a copy.
type Policy struct {
	Grid  world.Map
	Locks map[string]string
}

// Decision describes a symmetric pair, never a persisted grant. Media reports the
// current eligibility for future audio and screen links; E08 transmits no media.
type Decision struct {
	Text   bool
	Media  bool
	Gain   float64
	Kind   string
	ZoneID string
}

// Between computes text and media access from server-owned presence and positions.
// An active lock excludes every third person from both text and media.
func (p Policy) Between(a, b Participant) Decision {
	none := Decision{Kind: "none"}
	if a.ID == "" || b.ID == "" || !a.Available || !b.Available ||
		!p.Grid.Walkable(a.Position) || !p.Grid.Walkable(b.Position) {
		return none
	}
	if a.ID == b.ID {
		return Decision{Text: true, Kind: "self"}
	}

	peerA, lockedA := p.Locks[a.ID]
	peerB, lockedB := p.Locks[b.ID]
	if lockedA || lockedB {
		if !lockedA || !lockedB || peerA != b.ID || peerB != a.ID {
			return none
		}
		_, allowed := OpenProximity(p.Grid, a.Position, b.Position)
		if !allowed {
			return none
		}
		return Decision{Text: true, Media: true, Gain: 1, Kind: "locked"}
	}

	za, zb := p.Grid.ZoneAt(a.Position), p.Grid.ZoneAt(b.Position)
	if za != nil || zb != nil {
		if za == nil || zb == nil || za.ID != zb.ID || za.Kind != zb.Kind {
			return none
		}
		switch za.Kind {
		case "meeting":
			return Decision{Text: true, Media: true, Gain: 1, Kind: "meeting", ZoneID: za.ID}
		case "focus":
			return Decision{Text: true, Kind: "focus", ZoneID: za.ID}
		default:
			return none
		}
	}

	if !ClearLineOfSight(p.Grid, a.Position, b.Position) {
		return none
	}
	distance := Distance(a.Position, b.Position)
	if distance > LocalTextRadius {
		return none
	}
	result := Decision{Text: true, Kind: "proximity"}
	if distance < MediaRadius {
		result.Media = true
		result.Gain = gainAt(distance)
	}
	return result
}

// Distance uses Euclidean distance in map cells; it is symmetric.
func Distance(a, b world.Position) float64 {
	return math.Hypot(float64(b.X)-float64(a.X), float64(b.Y)-float64(a.Y))
}

// OpenProximity is the admission condition for a consented pair on open floor.
// Meetings and focus zones cannot create a locked proximity conversation.
func OpenProximity(grid world.Map, a, b world.Position) (float64, bool) {
	if !grid.Walkable(a) || !grid.Walkable(b) || grid.ZoneAt(a) != nil || grid.ZoneAt(b) != nil {
		return 0, false
	}
	distance := Distance(a, b)
	if distance >= MediaRadius || !ClearLineOfSight(grid, a, b) {
		return 0, false
	}
	return gainAt(distance), true
}

func gainAt(distance float64) float64 {
	if distance <= FullGainRadius {
		return 1
	}
	return (MediaRadius - distance) / (MediaRadius - FullGainRadius)
}

// ClearLineOfSight checks every cell touched by the segment. Opaque walls block;
// furniture is not opaque. At a diagonal corner both side cells are checked,
// preventing communication through a gap formed by touching walls.
func ClearLineOfSight(grid world.Map, source, target world.Position) bool {
	if !grid.Walkable(source) || !grid.Walkable(target) {
		return false
	}
	dx, dy := target.X-source.X, target.Y-source.Y
	nx, ny := abs(dx), abs(dy)
	sx, sy := sign(dx), sign(dy)
	x, y, ix, iy := source.X, source.Y, 0, 0
	wall := func(x, y int) bool {
		return y < 0 || y >= len(grid.Rows) || x < 0 || x >= len(grid.Rows[y]) || grid.Rows[y][x] == '#'
	}
	for ix < nx || iy < ny {
		decision := (1+2*ix)*ny - (1+2*iy)*nx
		if decision == 0 {
			if wall(x+sx, y) || wall(x, y+sy) {
				return false
			}
			x += sx
			y += sy
			ix++
			iy++
		} else if decision < 0 {
			x += sx
			ix++
		} else {
			y += sy
			iy++
		}
		if wall(x, y) {
			return false
		}
	}
	return true
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}
