// Amostra do Zone. Arquivo original: escritorio/office/gateway-go/world/world.go
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

// Package world owns movement rules, independently of HTTP and the Zone.
package world

import (
	"fmt"
	"time"
)

const StepInterval = 150 * time.Millisecond

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Map struct {
	Version string   `json:"version"`
	Name    string   `json:"name"`
	Spawn   Position `json:"spawn"`
	Rows    []string `json:"rows"`
	Zones   []Zone   `json:"zones,omitempty"`
	Desks   []Desk   `json:"desks,omitempty"`
}

func (m Map) Validate() error {
	if m.Version == "" || m.Name == "" || len(m.Rows) == 0 || len(m.Rows) > 100 {
		return fmt.Errorf("invalid map metadata or height")
	}
	width := len(m.Rows[0])
	if width == 0 || width > 100 {
		return fmt.Errorf("invalid map width")
	}
	for _, row := range m.Rows {
		if len(row) != width {
			return fmt.Errorf("map must be rectangular")
		}
		for _, tile := range row {
			if tile != '.' && tile != '#' && tile != 'T' {
				return fmt.Errorf("unknown tile")
			}
		}
	}
	if !m.Walkable(m.Spawn) {
		return fmt.Errorf("spawn must be walkable")
	}
	if err := m.validateZones(); err != nil {
		return err
	}
	return m.validateDesks()
}

func (m Map) Walkable(p Position) bool {
	return p.Y >= 0 && p.Y < len(m.Rows) && p.X >= 0 && p.X < len(m.Rows[p.Y]) && m.Rows[p.Y][p.X] == '.'
}

// Arrival chooses the nearest free floor using an orthogonal breadth-first
// search. People may still cross each other after entering the office.
func (m Map) Arrival(occupied map[Position]bool) Position {
	queue := []Position{m.Spawn}
	seen := map[Position]bool{m.Spawn: true}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if !occupied[p] {
			return p
		}
		for _, delta := range []Position{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
			next := Position{p.X + delta.X, p.Y + delta.Y}
			if !seen[next] && m.Walkable(next) && m.ZoneAt(next) == nil {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return m.Spawn
}

type State struct {
	Position   Position `json:"position"`
	Sequence   uint64   `json:"sequence"`
	MapVersion string   `json:"map_version"`
	ZoneID     string   `json:"zone_id"`
	lastStep   time.Time
}

func NewState(m Map) State { return State{Position: m.Spawn, MapVersion: m.Version} }

// Move accepts one intent, never a client-provided position. Valid ordered
// attempts consume a sequence even when blocked, so stale retries cannot move.
func (s *State) Move(m Map, direction string, sequence uint64, now time.Time) string {
	return s.MoveWithOccupancy(m, direction, sequence, now, nil)
}

func (s *State) MoveWithOccupancy(m Map, direction string, sequence uint64, now time.Time, occupants map[string]int) string {
	var dx, dy int
	switch direction {
	case "up":
		dy = -1
	case "down":
		dy = 1
	case "left":
		dx = -1
	case "right":
		dx = 1
	default:
		return "invalid_direction"
	}
	if sequence != s.Sequence+1 || sequence == 0 {
		return "out_of_order"
	}
	s.Sequence = sequence
	if !s.lastStep.IsZero() && now.Sub(s.lastStep) < StepInterval {
		return "rate_limited"
	}
	s.lastStep = now
	next := Position{s.Position.X + dx, s.Position.Y + dy}
	if !m.Walkable(next) {
		return "collision"
	}
	if code := m.ZoneTransition(s.Position, next, occupants); code != "" {
		return code
	}
	s.Position = next
	s.ZoneID = ""
	if z := m.ZoneAt(next); z != nil {
		s.ZoneID = z.ID
	}
	return ""
}
