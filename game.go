package main

import "math"

// Game units: arena is 800x500, time in seconds.
const (
	arenaW        = 2000.0 // defaults: config.json court.length / court.width (x is the length)
	arenaH        = 1000.0
	speed         = 180.0 // default walk speed
	pikachuRadius = 16.0  // defaults of pikachu.radius / snorlax.radius: the size the placeholder art was drawn at
	snorlaxRadius = 24.0
	spawnHalfGap  = 200.0 // each player starts this far from the middle of the court
	groundZ       = 12.0  // a target above this height counts as airborne
	slamRadius    = 70.0  // the default of snorlax.slam_radius
	slamDamage    = 20
	quakeRadius   = 100.0
	quakeDamage   = 10
	quakeCooldown = 0.8
	maxHP         = 100 // default
)

// Player slots are fixed characters: the first to join is Pikachu.
const (
	pikachu = 0
	snorlax = 1
)

// Input is the only thing a client controls: which keys are held. Skill is the
// space bar; what it does depends on the character (dash or jump).
type Input struct {
	DX      int  `json:"dx"`
	DY      int  `json:"dy"`
	Skill   bool `json:"skill"`
	Attack  bool `json:"atk"`     // Pikachu only: earthquake
	Beam    bool `json:"beam"`    // Snorlax only: Hyper Beam
	Thunder bool `json:"thunder"` // Pikachu only: Thunder
}

type Player struct {
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"` // height above ground; the screen position is (X, Y-Z)
	VZ float64 `json:"-"`
	HP int     `json:"hp"`
	CD float64 `json:"cd"`
	In Input   `json:"-"`

	FX, FY       int     `json:"-"`   // last non-zero direction held; a dash with no keys goes this way
	DashT        float64 `json:"-"`   // seconds of dash left
	DashX, DashY float64 `json:"-"`   // unit dash direction
	DashCD       float64 `json:"dcd"` // cooldowns are sent so the client can draw their countdown

	// Hyper Beam: charge (BeamT) -> sweep (SweepT) -> recharge (RechargeT).
	// The client draws the warning fan, the rotating beam and "must recharge" from these.
	BeamT     float64 `json:"bt"` // seconds of charge left
	SweepT    float64 `json:"st"` // seconds of sweep left
	RechargeT float64 `json:"rt"` // seconds of recovery left
	BeamX     float64 `json:"bx"` // aim (unit vector toward the opponent), locked when the beam is used
	BeamY     float64 `json:"by"`
	BeamA     float64 `json:"ba"`  // beam angle from the facing in degrees, counter-clockwise on screen is positive
	SweptHit  bool    `json:"-"`   // a sweep hits a target at most once
	BeamCD    float64 `json:"bcd"` // counted from the moment the beam is used
	JumpCD    float64 `json:"jcd"` // counted from the moment of take-off

	// Thunder: a shadow chases the opponent, then bolts hit wherever it is.
	// ThunderLeft is the number of bolts still to come (0 = no cast running).
	ThunderLeft int     `json:"tn"`
	ThunderIn   float64 `json:"ti"` // seconds until the next bolt
	ShadowX     float64 `json:"sx"`
	ShadowY     float64 `json:"sy"`
	ThunderCD   float64 `json:"tcd"` // counted from the moment the cast starts
}

// Event is a one-shot effect the client plays (ring, screen shake).
type Event struct {
	T string  `json:"t"` // "slam" | "quake" | "thunder" | "skybolt"
	X float64 `json:"x"`
	Y float64 `json:"y"`
	R float64 `json:"r,omitempty"` // quake reach, so the effect can match it
}

// The view a room was opened from decides its court.
const (
	modeMobile = "mobile"
	modeWeb    = "web"
)

type Game struct {
	cfg   Config
	court CourtSize
	P     [2]Player
}

func NewGame(cfg Config) *Game { return NewGameFor(cfg, modeWeb) }

func NewGameFor(cfg Config, mode string) *Game {
	g := &Game{cfg: cfg, court: cfg.courtFor(mode)}
	g.Reset()
	return g
}

func (g *Game) Reset() {
	// 400 apart, centred in the arena.
	c := g.court
	g.P[pikachu] = Player{X: c.Length/2 - spawnHalfGap, Y: c.Width / 2, HP: g.cfg.Pikachu.MaxHP, FX: 1}
	g.P[snorlax] = Player{X: c.Length/2 + spawnHalfGap, Y: c.Width / 2, HP: g.cfg.Snorlax.MaxHP, FX: -1}
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

// radius is how big a character's body is: a skill hits when its area touches the body.
func (g *Game) radius(i int) float64 {
	if i == pikachu {
		return g.cfg.Pikachu.Radius
	}
	return g.cfg.Snorlax.Radius
}

func dist(a, b *Player) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

// sweepHits reports whether a beam fired from src toward the facing (fx, fy)
// touched the target while it rotated from angle a0 down to a1 (degrees from
// the facing; counter-clockwise on screen is positive, so a0 >= a1).
//
// It tests the whole angular interval of the tick instead of the beam's final
// position, so a fast sweep cannot jump over a target between two ticks. The
// beam has width, so it reaches half-width/r degrees past its centre line.
func sweepHits(src, target *Player, fx, fy, a0, a1, length, width float64) bool {
	dx, dy := target.X-src.X, target.Y-src.Y
	r := math.Hypot(dx, dy)
	if r <= width/2 {
		return true // standing on the muzzle
	}
	if r > length {
		return false
	}
	// Screen y points down, so negate it to get counter-clockwise angles.
	bearing := math.Remainder(math.Atan2(-dy, dx)-math.Atan2(-fy, fx), 2*math.Pi) * 180 / math.Pi
	half := math.Asin(math.Min(1, width/2/r)) * 180 / math.Pi
	return bearing >= a1-half && bearing <= a0+half
}

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

		if p.In.DX != 0 || p.In.DY != 0 {
			p.FX, p.FY = p.In.DX, p.In.DY
		}
		if p.DashCD > 0 {
			p.DashCD = math.Max(0, p.DashCD-dt)
		}

		// Pikachu's skill: dash toward the held direction (or where it faces).
		if i == pikachu && p.In.Skill && p.DashT == 0 && p.DashCD == 0 && p.ThunderLeft == 0 {
			dx, dy := float64(p.In.DX), float64(p.In.DY)
			if dx == 0 && dy == 0 {
				dx, dy = float64(p.FX), float64(p.FY)
			}
			n := math.Hypot(dx, dy)
			p.DashX, p.DashY = dx/n, dy/n
			p.DashT = g.cfg.Pikachu.Dash.Duration
			p.DashCD = g.cfg.Pikachu.Dash.Cooldown // counted from the start of the dash
		}

		// Snorlax's Hyper Beam: charge -> sweep -> recharge. It is rooted through all
		// three. The aim is locked on the opponent's position when the beam is used.
		busy := false
		casting := false // Pikachu is rooted while Thunder runs
		if i == snorlax {
			hb := g.cfg.Snorlax.HyperBeam
			if p.JumpCD > 0 {
				if p.JumpCD -= dt; p.JumpCD < 1e-9 {
					p.JumpCD = 0
				}
			}
			if p.RechargeT > 0 {
				if p.RechargeT -= dt; p.RechargeT < 1e-9 {
					p.RechargeT = 0
				}
			}
			if p.BeamCD > 0 {
				if p.BeamCD -= dt; p.BeamCD < 1e-9 {
					p.BeamCD = 0
				}
			}
			if p.BeamT == 0 && p.SweepT == 0 && p.RechargeT == 0 && p.BeamCD == 0 && p.In.Beam && p.Z == 0 {
				dx, dy := other.X-p.X, other.Y-p.Y
				n := math.Hypot(dx, dy)
				if n < 1e-9 { // standing on the opponent: there is no direction, so use the facing
					dx, dy = float64(p.FX), float64(p.FY)
					n = math.Hypot(dx, dy)
				}
				p.BeamX, p.BeamY = dx/n, dy/n
				p.BeamT = hb.ChargeDuration
				p.BeamCD = hb.Cooldown
			}
			if p.BeamT > 0 {
				if p.BeamT -= dt; p.BeamT < 1e-9 { // charged: the beam starts at +angle
					p.BeamT = 0
					p.SweepT = hb.SweepDuration
					p.BeamA = hb.SweepAngle
					p.SweptHit = false
				}
			} else if p.SweepT > 0 {
				prev := p.BeamA
				if p.SweepT -= dt; p.SweepT < 1e-9 {
					p.SweepT = 0
				}
				p.BeamA = hb.SweepAngle * (2*p.SweepT/hb.SweepDuration - 1) // +angle -> -angle
				// Like the quake, the beam travels along the ground and misses airborne targets.
				if !p.SweptHit && other.Z < groundZ && sweepHits(p, other, p.BeamX, p.BeamY, prev, p.BeamA, hb.Length+g.radius(1-i), hb.Width+2*g.radius(1-i)) {
					g.hit(other, hb.Damage)
					p.SweptHit = true
				}
				if p.SweepT == 0 {
					p.RechargeT = hb.RechargeDuration
				}
			}
			busy = p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0
		}

		// Pikachu's Thunder. Pikachu cannot move until the last bolt has struck, and a
		// new cast waits for the current one to finish. Bolts come from the sky, so
		// height gives no cover. Each strike is also fired up into the sky from Pikachu.
		if i == pikachu {
			th := g.cfg.Pikachu.Thunder
			if p.ThunderCD > 0 {
				if p.ThunderCD -= dt; p.ThunderCD < 1e-9 {
					p.ThunderCD = 0
				}
			}
			if p.In.Thunder && p.ThunderLeft == 0 && p.ThunderCD == 0 {
				p.ThunderLeft, p.ThunderIn = th.Strikes, th.Delay
				p.ThunderCD = th.Cooldown
				p.DashT = 0                             // a dash in progress (or begun this tick) stops: casting roots Pikachu at once
				p.ShadowX, p.ShadowY = other.X, other.Y // appears right where the opponent stands
			}
			casting = p.ThunderLeft > 0 // rooted through the tick of the last bolt, free from the next
			if p.ThunderLeft > 0 {
				dx, dy := other.X-p.ShadowX, other.Y-p.ShadowY
				if d := math.Hypot(dx, dy); d > 0 { // chase, without overshooting
					step := math.Min(d, th.FollowSpeed*dt)
					p.ShadowX += dx / d * step
					p.ShadowY += dy / d * step
				}
				p.ThunderIn -= dt
				for p.ThunderLeft > 0 && p.ThunderIn < 1e-9 { // a loop so a tiny interval cannot lose bolts
					events = append(events, Event{T: "skybolt", X: p.X, Y: p.Y}, Event{T: "thunder", X: p.ShadowX, Y: p.ShadowY})
					if math.Hypot(other.X-p.ShadowX, other.Y-p.ShadowY) <= th.Radius+g.radius(1-i) {
						g.hit(other, th.Damage)
					}
					p.ThunderLeft--
					p.ThunderIn += th.Interval
				}
			}
		}

		if p.DashT > 0 {
			// Steering is locked while dashing. The last step is clipped so the
			// total distance is exact whatever the tick length.
			step := math.Min(dt, p.DashT)
			v := g.cfg.Pikachu.Dash.Distance / g.cfg.Pikachu.Dash.Duration
			p.X += p.DashX * v * step
			p.Y += p.DashY * v * step
			if p.DashT -= step; p.DashT < 1e-9 {
				p.DashT = 0
			}
		} else if n := math.Hypot(float64(p.In.DX), float64(p.In.DY)); !busy && !casting && n > 0 {
			// Normalise diagonals so they are not faster. Snorlax steers at its own speeds in
			// the air: rising is the first half of the jump, falling the second.
			v := g.cfg.Pikachu.WalkSpeed
			if i == snorlax {
				v = g.cfg.Snorlax.WalkSpeed
			}
			if i == snorlax && p.Z > 0 {
				v = g.cfg.Snorlax.Jump.AirSpeedSecondHalf
				if p.VZ > 0 {
					v = g.cfg.Snorlax.Jump.AirSpeedFirstHalf
				}
			}
			p.X += float64(p.In.DX) / n * v * dt
			p.Y += float64(p.In.DY) / n * v * dt
		}
		p.X = math.Min(math.Max(p.X, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y, 0), g.court.Width)

		// Snorlax's skill: jump. Exact parabola so height and air time match the config.
		if i == snorlax && p.In.Skill && p.Z == 0 && !busy && p.JumpCD == 0 {
			p.VZ = g.cfg.jumpV()
			p.JumpCD = g.cfg.Snorlax.Jump.Cooldown // counted from the moment of take-off
		}
		landed := false
		if p.Z > 0 || p.VZ > 0 {
			gr := g.cfg.gravity()
			p.Z += p.VZ*dt - 0.5*gr*dt*dt
			p.VZ -= gr * dt
			if p.Z <= 1e-9 { // not 0: the parabola ends at 0 on the tick, but rounding can leave +3e-15
				p.Z, p.VZ, landed = 0, 0, true
			}
		}

		// The slam resolves on the landing frame, not while falling.
		if landed {
			events = append(events, Event{T: "slam", X: p.X, Y: p.Y, R: g.cfg.Snorlax.Jump.SlamRadius}) // the client draws the real reach
			if dist(p, other) < g.cfg.Snorlax.Jump.SlamRadius+g.radius(1-i) && other.Z < groundZ {
				g.hit(other, slamDamage)
			}
		}

		// Pikachu's earthquake: only from the ground, and it cannot reach airborne targets.
		// Snorlax has none; its jump landing is its ground shock.
		if p.CD > 0 {
			p.CD = math.Max(0, p.CD-dt)
		}
		if i == pikachu && p.In.Attack && p.CD == 0 && p.Z == 0 && !busy {
			p.CD = quakeCooldown
			events = append(events, Event{T: "quake", X: p.X, Y: p.Y, R: quakeRadius}) // the client draws the real reach
			if dist(p, other) < quakeRadius+g.radius(1-i) && other.Z < groundZ {
				g.hit(other, quakeDamage)
			}
		}
	}
	return events
}
