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
	Ball    bool `json:"ball"`    // Pikachu only: Electro Ball
}

type Player struct {
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"` // height above ground; the screen position is (X, Y-Z)
	VZ float64 `json:"-"`
	HP int     `json:"hp"`
	CD float64 `json:"cd"`
	In Input   `json:"-"`

	FX           int     `json:"fx"` // last non-zero direction held; a dash with no keys goes this way, and so does the ball
	FY           int     `json:"fy"`
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
	SweepID   int     `json:"-"`   // counts this player's sweeps
	SweptHit  bool    `json:"-"`   // the sweep has hit its target at least once
	BeamHitCD float64 `json:"-"`   // until the beam may hit again (hyper_beam.hit_interval); 0 = it may now
	BeamCD    float64 `json:"bcd"` // counted from the moment the beam is used
	JumpCD    float64 `json:"jcd"` // counted from the moment of take-off

	// Thunder: a shadow chases the opponent, then bolts hit wherever it is.
	// ThunderLeft is the number of bolts still to come (0 = no cast running).
	ThunderLeft int     `json:"tn"`
	ThunderIn   float64 `json:"ti"` // seconds until the next bolt
	ShadowX     float64 `json:"sx"`
	ShadowY     float64 `json:"sy"`
	ThunderCD   float64 `json:"tcd"` // counted from the moment the cast starts

	// Electro Ball: BallT is the charge time left (0 = not charging).
	BallT  float64 `json:"kt"`
	BallCD float64 `json:"ecd"` // counted from the moment the charge starts
}

// Ball is a thrown Electro Ball: it flies straight until it explodes.
type Ball struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	DX, DY    float64 `json:"-"` // unit direction
	HP        int     `json:"hp"`
	LastSweep int     `json:"-"` // the Hyper Beam sweep that last hit it, so one sweep hits it once
}

// Event is a one-shot effect the client plays (ring, screen shake).
type Event struct {
	T string  `json:"t"` // "slam" | "quake" | "thunder" | "skybolt" | "explode"
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
	Balls []Ball
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
	g.Balls = nil
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

// knockBack pushes target d units straight away from (sx, sy), stopping at the walls. Standing exactly
// on it there is no "away", so it goes along (fx, fy).
func (g *Game) knockBack(sx, sy float64, target *Player, d, fx, fy float64) {
	dx, dy := target.X-sx, target.Y-sy
	n := math.Hypot(dx, dy)
	if n < 1e-9 {
		dx, dy, n = fx, fy, math.Hypot(fx, fy)
	}
	target.X = math.Min(math.Max(target.X+dx/n*d, 0), g.court.Length)
	target.Y = math.Min(math.Max(target.Y+dy/n*d, 0), g.court.Width)
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
		if i == pikachu && p.In.Skill && p.DashT == 0 && p.DashCD == 0 && p.ThunderLeft == 0 && p.BallT == 0 {
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
					p.SweptHit, p.BeamHitCD = false, 0
					p.SweepID++
				}
			} else if p.SweepT > 0 {
				prev := p.BeamA
				if p.SweepT -= dt; p.SweepT < 1e-9 {
					p.SweepT = 0
				}
				p.BeamA = hb.SweepAngle * (2*p.SweepT/hb.SweepDuration - 1) // +angle -> -angle
				if p.BeamHitCD > 0 {
					if p.BeamHitCD -= dt; p.BeamHitCD < 1e-9 {
						p.BeamHitCD = 0
					}
				}
				// Like the quake, the beam travels along the ground and misses airborne targets.
				// It hits again every hit_interval for as long as the target stays in the beam (0 = once per sweep).
				ready := p.BeamHitCD == 0 && (hb.HitInterval > 0 || !p.SweptHit)
				if ready && other.Z < groundZ && sweepHits(p, other, p.BeamX, p.BeamY, prev, p.BeamA, hb.Length+g.radius(1-i), hb.Width+2*g.radius(1-i)) {
					g.hit(other, hb.Damage)
					p.SweptHit, p.BeamHitCD = true, hb.HitInterval
					g.knockBack(p.X, p.Y, other, hb.Knockback, p.BeamX, p.BeamY)
				}
				br := g.cfg.Pikachu.ElectroBall.Radius
				for j := range g.Balls { // the beam hits a ball once per sweep
					b := &g.Balls[j]
					if b.LastSweep != p.SweepID && sweepHits(p, &Player{X: b.X, Y: b.Y}, p.BeamX, p.BeamY, prev, p.BeamA, hb.Length+br, hb.Width+2*br) {
						b.HP -= hb.Damage
						b.LastSweep = p.SweepID
					}
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
			if p.In.Thunder && p.ThunderLeft == 0 && p.ThunderCD == 0 && p.BallT == 0 {
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

		// Pikachu's Electro Ball: charge rooted, then throw it where Pikachu faces. Holding a direction
		// while charging turns the aim. Only one charge or Thunder cast at a time.
		if i == pikachu {
			bc := g.cfg.Pikachu.ElectroBall
			if p.BallCD > 0 {
				if p.BallCD -= dt; p.BallCD < 1e-9 {
					p.BallCD = 0
				}
			}
			if p.In.Ball && p.BallT == 0 && p.ThunderLeft == 0 && p.BallCD == 0 {
				p.BallT, p.BallCD = bc.ChargeTime, bc.Cooldown
				p.DashT = 0 // charging roots Pikachu at once
			}
			if p.BallT > 0 {
				casting = true // rooted through the tick of the throw, free from the next
				if p.BallT -= dt; p.BallT < 1e-9 {
					p.BallT = 0
					fx, fy := float64(p.FX), float64(p.FY)
					n := math.Hypot(fx, fy)
					off := g.radius(pikachu) + bc.Radius + 1 // start just clear of Pikachu's body
					g.Balls = append(g.Balls, Ball{X: p.X + fx/n*off, Y: p.Y + fy/n*off, DX: fx / n, DY: fy / n, HP: bc.HP})
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
			for j := range g.Balls { // the shock wave is a ground attack: it hits balls too
				b := &g.Balls[j]
				if math.Hypot(b.X-p.X, b.Y-p.Y) < g.cfg.Snorlax.Jump.SlamRadius+g.cfg.Pikachu.ElectroBall.Radius {
					b.HP -= slamDamage
				}
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
	return append(events, g.stepBalls(dt)...)
}

// stepBalls flies the balls and blows up those that touched the opponent, reached a wall or ran out of HP.
func (g *Game) stepBalls(dt float64) []Event {
	var events []Event
	bc := g.cfg.Pikachu.ElectroBall
	foe := &g.P[snorlax]
	kept := g.Balls[:0]
	for _, b := range g.Balls {
		boom := b.HP <= 0 // shot down where it is
		if !boom {
			x0, y0 := b.X, b.Y
			b.X += b.DX * bc.Speed * dt
			b.Y += b.DY * bc.Speed * dt
			if b.X <= 0 || b.X >= g.court.Length || b.Y <= 0 || b.Y >= g.court.Width {
				b.X = math.Min(math.Max(b.X, 0), g.court.Length)
				b.Y = math.Min(math.Max(b.Y, 0), g.court.Width)
				boom = true
			}
			// Along the whole step, so a fast ball cannot jump over the opponent between two ticks.
			// Like the ground attacks it flies under an airborne opponent.
			if foe.Z < groundZ && segDist(foe.X, foe.Y, x0, y0, b.X, b.Y) <= bc.Radius+g.radius(snorlax) {
				b.X, b.Y = closestOnSegment(foe.X, foe.Y, x0, y0, b.X, b.Y) // it blows up where it met the opponent
				boom = true
			}
		}
		if !boom {
			kept = append(kept, b)
			continue
		}
		events = append(events, Event{T: "explode", X: b.X, Y: b.Y, R: bc.ExplosionRadius})
		if foe.Z < groundZ && math.Hypot(foe.X-b.X, foe.Y-b.Y) <= bc.ExplosionRadius+g.radius(snorlax) {
			g.hit(foe, bc.ExplosionDamage)
			g.knockBack(b.X, b.Y, foe, bc.Knockback, b.DX, b.DY)
		}
	}
	g.Balls = kept
	return events
}

// closestOnSegment is the point of the segment (x0, y0)-(x1, y1) nearest to (px, py).
func closestOnSegment(px, py, x0, y0, x1, y1 float64) (float64, float64) {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = math.Min(math.Max(((px-x0)*dx+(py-y0)*dy)/l2, 0), 1)
	}
	return x0 + t*dx, y0 + t*dy
}

// segDist is how far the point (px, py) is from the segment (x0, y0)-(x1, y1).
func segDist(px, py, x0, y0, x1, y1 float64) float64 {
	cx, cy := closestOnSegment(px, py, x0, y0, x1, y1)
	return math.Hypot(px-cx, py-cy)
}
