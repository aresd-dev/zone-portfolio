// Amostra do Zone. Arquivo original: escritorio/office/gateway-go/communication/policy_test.go
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

package communication

import (
	"math"
	"testing"

	"zone-office/world"
)

func policyGrid() world.Map {
	return world.Map{
		Rows: []string{
			"####################",
			"#..................#",
			"#..................#",
			"#..................#",
			"#..................#",
			"#..................#",
			"#..................#",
			"#..................#",
			"####################",
		},
		Zones: []world.Zone{
			{ID: "meeting-a", Kind: "meeting", X: 10, Y: 1, Width: 8, Height: 2},
			{ID: "meeting-b", Kind: "meeting", X: 10, Y: 4, Width: 3, Height: 2},
			{ID: "focus", Kind: "focus", X: 15, Y: 4, Width: 3, Height: 2},
		},
	}
}
func person(id string, x, y int) Participant {
	return Participant{ID: id, Position: world.Position{X: x, Y: y}, Available: true}
}
func closeFloat(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func TestPolicyContextMatrixAndSymmetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		a, b        Participant
		text, media bool
		gain        float64
		kind, zone  string
	}{
		{"self has text without media loopback", person("a", 2, 2), person("a", 2, 2), true, false, 0, "self", ""},
		{"open same tile", person("a", 2, 2), person("b", 2, 2), true, true, 1, "proximity", ""},
		{"open two cells full gain", person("a", 2, 2), person("b", 4, 2), true, true, 1, "proximity", ""},
		{"open three cells", person("a", 2, 2), person("b", 5, 2), true, true, .75, "proximity", ""},
		{"open five cells", person("a", 2, 2), person("b", 7, 2), true, true, .25, "proximity", ""},
		{"six cells preserves text boundary", person("a", 2, 2), person("b", 8, 2), true, false, 0, "proximity", ""},
		{"beyond six no link", person("a", 2, 2), person("b", 9, 2), false, false, 0, "none", ""},
		{"diagonal uses Euclidean distance", person("a", 2, 2), person("b", 5, 6), true, true, .25, "proximity", ""},
		{"same meeting far apart uniform gain", person("a", 10, 1), person("b", 17, 2), true, true, 1, "meeting", "meeting-a"},
		{"same focus only text", person("a", 15, 4), person("b", 17, 5), true, false, 0, "focus", "focus"},
		{"focus meeting isolated", person("a", 15, 4), person("b", 12, 4), false, false, 0, "none", ""},
		{"different meeting isolated", person("a", 10, 2), person("b", 10, 4), false, false, 0, "none", ""},
		{"meeting corridor isolated", person("a", 10, 1), person("b", 9, 1), false, false, 0, "none", ""},
		{"focus corridor isolated", person("a", 15, 4), person("b", 14, 4), false, false, 0, "none", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := Policy{Grid: policyGrid()}
			got := policy.Between(tc.a, tc.b)
			if got.Text != tc.text || got.Media != tc.media || !closeFloat(got.Gain, tc.gain) || got.Kind != tc.kind || got.ZoneID != tc.zone {
				t.Fatalf("unexpected decision: %+v", got)
			}
			if reverse := policy.Between(tc.b, tc.a); reverse != got {
				t.Fatalf("not symmetric: %+v versus %+v", got, reverse)
			}
		})
	}
}

func TestPolicyRequiresCurrentPresenceAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Participant)
	}{
		{"offline or disconnected", func(p *Participant) { p.Available = false }},
		{"access revoked", func(p *Participant) { p.Available = false }},
		{"expired session", func(p *Participant) { p.Available = false }},
		{"empty identity", func(p *Participant) { p.ID = "" }},
		{"outside map", func(p *Participant) { p.Position.X = -1 }},
		{"wall position", func(p *Participant) { p.Position.Y = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := person("a", 2, 2), person("b", 3, 2)
			tc.change(&a)
			p := Policy{Grid: policyGrid(), Locks: map[string]string{"a": "b", "b": "a"}}
			if got := p.Between(a, b); got.Text || got.Media || got.Gain != 0 {
				t.Fatalf("unavailable source authorized: %+v", got)
			}
			if got := p.Between(b, a); got.Text || got.Media || got.Gain != 0 {
				t.Fatalf("unavailable recipient authorized: %+v", got)
			}
		})
	}
}

func TestPolicyConsentedPairExcludesEveryThirdPerson(t *testing.T) {
	p := Policy{Grid: policyGrid(), Locks: map[string]string{"a": "b", "b": "a"}}
	a, b, c := person("a", 2, 2), person("b", 3, 2), person("c", 2, 3)
	for _, pair := range [][2]Participant{{a, b}, {b, a}} {
		if got := p.Between(pair[0], pair[1]); !got.Text || !got.Media || got.Gain != 1 || got.Kind != "locked" {
			t.Fatalf("consented pair denied: %+v", got)
		}
	}
	for _, pair := range [][2]Participant{{a, c}, {c, a}, {b, c}, {c, b}} {
		if got := p.Between(pair[0], pair[1]); got.Text || got.Media || got.Gain != 0 {
			t.Fatalf("third party received locked conversation: %+v", got)
		}
	}
	b.Position = world.Position{X: 6, Y: 2}
	if got := p.Between(a, b); !got.Text || !got.Media || got.Gain != 1 || got.Kind != "locked" {
		t.Fatalf("locked pair should retain uniform gain while eligible: %+v", got)
	}
	delete(p.Locks, "a")
	delete(p.Locks, "b")
	if got := p.Between(a, c); !got.Text || !got.Media || got.Kind != "proximity" {
		t.Fatalf("released pair did not restore proximity: %+v", got)
	}
}

func TestPolicyMalformedOrStaleLocksFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		locks map[string]string
		a, b  Participant
	}{
		{"one sided", map[string]string{"a": "b"}, person("a", 2, 2), person("b", 3, 2)},
		{"conflicting pair", map[string]string{"a": "b", "b": "c"}, person("a", 2, 2), person("b", 3, 2)},
		{"empty peer", map[string]string{"a": ""}, person("a", 2, 2), person("b", 3, 2)},
		{"six cells closes pair", map[string]string{"a": "b", "b": "a"}, person("a", 2, 2), person("b", 8, 2)},
		{"meeting cannot retain proximity lock", map[string]string{"a": "b", "b": "a"}, person("a", 10, 1), person("b", 11, 1)},
		{"focus cannot retain proximity lock", map[string]string{"a": "b", "b": "a"}, person("a", 15, 4), person("b", 16, 4)},
		{"one peer enters zone", map[string]string{"a": "b", "b": "a"}, person("a", 9, 1), person("b", 10, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Policy{Grid: policyGrid(), Locks: tc.locks}
			got := p.Between(tc.a, tc.b)
			if got.Text || got.Media || got.Gain != 0 {
				t.Fatalf("stale lock authorized: %+v", got)
			}
			if reverse := p.Between(tc.b, tc.a); reverse != got {
				t.Fatal("asymmetric stale lock")
			}
		})
	}
}

func TestOpaqueWallsAndDiagonalCornersBlockBothChannels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		row   int
		tiles string
		a, b  Participant
	}{
		{"straight opaque wall", 2, "#..#...............#", person("a", 2, 2), person("b", 4, 2)},
		{"diagonal side wall", 1, "#.#................#", person("a", 1, 1), person("b", 3, 3)},
		{"opposite diagonal side wall", 2, "##.................#", person("a", 1, 1), person("b", 3, 3)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grid := policyGrid()
			grid.Rows[tc.row] = tc.tiles
			for _, locks := range []map[string]string{nil, {"a": "b", "b": "a"}} {
				p := Policy{Grid: grid, Locks: locks}
				got := p.Between(tc.a, tc.b)
				if got.Text || got.Media || got.Gain != 0 {
					t.Fatalf("wall leaked: %+v", got)
				}
				if ClearLineOfSight(grid, tc.a.Position, tc.b.Position) || ClearLineOfSight(grid, tc.b.Position, tc.a.Position) {
					t.Fatal("line of sight leaked in either direction")
				}
			}
		})
	}
}

func TestFurnitureDoesNotBecomeOpaque(t *testing.T) {
	grid := policyGrid()
	grid.Rows[2] = "#..T...............#"
	a, b := person("a", 2, 2), person("b", 4, 2)
	if got := (Policy{Grid: grid}).Between(a, b); !got.Text || !got.Media || got.Gain != 1 {
		t.Fatalf("furniture blocked sight: %+v", got)
	}
	b.Position.X = 3
	if got := (Policy{Grid: grid}).Between(a, b); got.Text || got.Media {
		t.Fatal("nonwalkable endpoint admitted")
	}
}

func TestOpenProximityAdmissionBoundariesAndImmutablePolicy(t *testing.T) {
	grid := policyGrid()
	a, b := person("a", 2, 2), person("b", 7, 2)
	locks := map[string]string{"a": "b", "b": "a"}
	policy := Policy{Grid: grid, Locks: locks}
	if gain, ok := OpenProximity(grid, a.Position, b.Position); !ok || gain != .25 {
		t.Fatal("five-cell pair rejected", gain, ok)
	}
	if gain, ok := OpenProximity(grid, a.Position, world.Position{X: 8, Y: 2}); ok || gain != 0 {
		t.Fatal("six-cell pair admitted", gain, ok)
	}
	if gain, ok := OpenProximity(grid, world.Position{X: 10, Y: 1}, world.Position{X: 11, Y: 1}); ok || gain != 0 {
		t.Fatal("meeting pair admitted")
	}
	before := policy.Between(a, b)
	for i := 0; i < 100; i++ {
		if got := policy.Between(a, b); got != before {
			t.Fatal("pure policy mutated result")
		}
	}
	if len(locks) != 2 || locks["a"] != "b" || locks["b"] != "a" {
		t.Fatal("policy mutated locked membership")
	}
	if grid.Rows[2] != "#..................#" {
		t.Fatal("policy mutated map")
	}
}
