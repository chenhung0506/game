package main

import "math"

// Game units: arena is 800x500, time in seconds.
const (
	arenaW        = 800.0
	arenaH        = 500.0
	speed         = 180.0
	jumpV         = 360.0 // initial vertical velocity; air time = 2*jumpV/gravity = 0.8s
	gravity       = 900.0
	groundZ       = 12.0 // a target above this height counts as airborne
	slamRadius    = 70.0
	slamDamage    = 20
	quakeRadius   = 100.0
	quakeDamage   = 10
	quakeCooldown = 0.8
	maxHP         = 100
)

// Input is the only thing a client controls: which keys are held.
type Input struct {
	DX     int  `json:"dx"`
	DY     int  `json:"dy"`
	Jump   bool `json:"jump"`
	Attack bool `json:"atk"`
}

type Player struct {
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"` // height above ground; the screen position is (X, Y-Z)
	VZ float64 `json:"-"`
	HP int     `json:"hp"`
	CD float64 `json:"cd"`
	In Input   `json:"-"`
}

// Event is a one-shot effect the client plays (ring, screen shake).
type Event struct {
	T string  `json:"t"` // "slam" | "quake"
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Game struct {
	P [2]Player
}

func NewGame() *Game {
	g := &Game{}
	g.Reset()
	return g
}

func (g *Game) Reset() {
	g.P[0] = Player{X: 200, Y: 250, HP: maxHP}
	g.P[1] = Player{X: 600, Y: 250, HP: maxHP}
}

// Winner returns the index of the surviving player, or -1 while both are alive.
func (g *Game) Winner() int {
	switch {
	case g.P[0].HP <= 0 && g.P[1].HP <= 0:
		return 0 // simultaneous KO: arbitrary but deterministic
	case g.P[0].HP <= 0:
		return 1
	case g.P[1].HP <= 0:
		return 0
	}
	return -1
}

func dist(a, b *Player) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

func (g *Game) hit(victim *Player, dmg int) {
	victim.HP -= dmg
	if victim.HP < 0 {
		victim.HP = 0
	}
}

// Step advances the simulation by dt seconds and returns the effects to show.
func (g *Game) Step(dt float64) []Event {
	var events []Event
	for i := range g.P {
		p, other := &g.P[i], &g.P[1-i]

		// Horizontal movement; normalise diagonals so they are not faster.
		dx, dy := float64(p.In.DX), float64(p.In.DY)
		if n := math.Hypot(dx, dy); n > 0 {
			p.X += dx / n * speed * dt
			p.Y += dy / n * speed * dt
		}
		p.X = math.Min(math.Max(p.X, 0), arenaW)
		p.Y = math.Min(math.Max(p.Y, 0), arenaH)

		// Vertical: jump, gravity, landing.
		if p.In.Jump && p.Z == 0 {
			p.VZ = jumpV
		}
		landed := false
		if p.Z > 0 || p.VZ > 0 {
			p.VZ -= gravity * dt
			p.Z += p.VZ * dt
			if p.Z <= 0 {
				p.Z, p.VZ, landed = 0, 0, true
			}
		}

		// The slam resolves on the landing frame, not while falling.
		if landed {
			events = append(events, Event{"slam", p.X, p.Y})
			if dist(p, other) < slamRadius && other.Z < groundZ {
				g.hit(other, slamDamage)
			}
		}

		// Ground attack: only from the ground, and it cannot reach airborne targets.
		if p.CD > 0 {
			p.CD = math.Max(0, p.CD-dt)
		}
		if p.In.Attack && p.CD == 0 && p.Z == 0 {
			p.CD = quakeCooldown
			events = append(events, Event{"quake", p.X, p.Y})
			if dist(p, other) < quakeRadius && other.Z < groundZ {
				g.hit(other, quakeDamage)
			}
		}
	}
	return events
}
