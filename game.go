package main

import (
	"math"
	"math/rand/v2"
	"time"
)

// Game units: arena is 800x500, time in seconds.
const (
	arenaW        = 2000.0 // defaults: config.json court.length / court.width (x is the length)
	arenaH        = 1000.0
	speed         = 180.0 // default walk speed
	pikachuRadius = 16.0  // defaults of pikachu.radius / snorlax.radius: the size the placeholder art was drawn at
	snorlaxRadius = 24.0
	spawnHalfGap  = 200.0 // each player starts this far from the middle of the court
	groundZ       = 12.0  // a target above this height counts as airborne
	flyRiseTime   = 0.25  // seconds Charizard takes to climb to its flying height, and to come down
	slamRadius    = 70.0  // the default of snorlax.slam_radius
	slamDamage    = 20
	maxHP         = 100 // default
)

// Characters ("kinds"). A player slot (0 or 1, the order of joining) holds whichever one its player picked;
// with the default NewGame, slot 0 is Pikachu and slot 1 is Snorlax.
const (
	pikachu   = 0
	snorlax   = 1
	charizard = 2
	chikorita = 3
	kinds     = 4
)

// Input is the only thing a client controls: which keys are held. Skill is the
// space bar; what it does depends on the character (dash or jump).
type Input struct {
	DX      int  `json:"dx"`
	DY      int  `json:"dy"`
	Skill   bool `json:"skill"`
	Beam    bool `json:"beam"`    // Snorlax only: Hyper Beam
	Thunder bool `json:"thunder"` // Pikachu only: Thunder
	Ball    bool `json:"ball"`    // Pikachu only: Electro Ball
	Punch   bool `json:"punch"`   // Snorlax only: Ice Punch
	Fire    bool `json:"fire"`    // Charizard only: Fire Breath
	Toss    bool `json:"toss"`    // Charizard only: Seismic Toss
	Razor   bool `json:"razor"`   // Chikorita only: Razor Leaf
	Vine    bool `json:"vine"`    // Chikorita only: Vine Whip
	// The stick's exact direction, a vector of any length (0,0 = none). Walking stays 8-way on DX/DY; this
	// only aims the Electro Ball and the Ice Punch. A keyboard sends none and aims along DX/DY.
	AimX float64 `json:"ax"`
	AimY float64 `json:"ay"`
}

type Player struct {
	Kind int     `json:"k"` // which character this slot plays: pikachu, snorlax, charizard or chikorita
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"` // height above ground; the screen position is (X, Y-Z)
	VZ   float64 `json:"-"`
	HP   int     `json:"hp"`
	In   Input   `json:"-"`

	FX           int     `json:"fx"` // last non-zero direction held; a dash with no keys goes this way, and so does the ball
	FY           int     `json:"fy"`
	AimX         float64 `json:"ax"` // unit vector the Electro Ball and the Ice Punch go along: the stick's, else the last direction held
	AimY         float64 `json:"ay"`
	DashT        float64 `json:"vd"`  // Volt Tackle: seconds of the dash left
	DashX, DashY float64 `json:"-"`   // unit dash direction
	DashCD       float64 `json:"dcd"` // cooldowns are sent so the client can draw their countdown
	VoltLeft     int     `json:"vn"`  // uses left in the running chain (0 = no chain: the next press starts one)
	VoltT        float64 `json:"vw"`  // seconds left to spend them before the cooldown starts anyway
	VoltHit      bool    `json:"-"`   // this dash has already struck
	ParalyzeT    float64 `json:"pz"`  // seconds left paralyzed by a Volt Tackle: cannot walk or use a movement skill

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

	// Ice Punch (Snorlax): charge time left, dash distance left (> 0 while dashing), recovery time left.
	PunchT       float64 `json:"ut"`
	PunchLeft    float64 `json:"ud"`
	PunchRecover float64 `json:"ur"`
	PunchDX      float64 `json:"udx"` // unit direction of the dash
	PunchDY      float64 `json:"udy"`
	PunchCD      float64 `json:"pcd"` // counted from the moment the charge starts
	PunchID      int     `json:"-"`   // counts the punches, so one punch hits a ball once
	PunchHit     bool    `json:"-"`
	FrozenT      float64 `json:"fz"` // seconds left frozen: no moving, no skills

	// Electro Ball: BallT is the charge time left (0 = not charging).
	BallT  float64 `json:"kt"`
	BallCD float64 `json:"ecd"` // counted from the moment the charge starts

	// Charizard. Fly: seconds of flight left (the height follows in Z) and the cooldown, which starts on landing.
	FlyT  float64 `json:"ft"`
	FlyCD float64 `json:"fcd"`
	// Fire Breath: seconds of breathing left, and how many of the config's quantity have left the mouth.
	BreathT    float64 `json:"ht"`
	BreathN    int     `json:"-"`
	BreathCD   float64 `json:"hcd"` // counted from the moment the breath starts
	BreathLeft float64 `json:"-"`   // the full duration, to tell how far along the breath is
	// Seismic Toss: TossPhase is tossNone, tossDash (running at the target), tossRise (carrying it up) or tossFall.
	TossAir   bool    `json:"-"` // the toss began in flight
	TossPhase int     `json:"gp"`
	TossLeft  float64 `json:"-"` // dash distance left
	TossDX    float64 `json:"gdx"`
	TossDY    float64 `json:"gdy"`
	TossCD    float64 `json:"gcd"` // counted from the moment the dash starts
	Held      bool    `json:"hd"`  // caught in a Seismic Toss: cannot move or act until it lands

	// Chikorita's Leaf Slash: LeafPhase is leafNone, leafHop or leafSlash; LeafT the seconds left in the hop or the
	// current swing, LeafN the swings done so far, LeafK how far along the hop or the swing is (0..1), which the
	// client draws the blade from.
	LeafPhase int     `json:"lp"`
	LeafT     float64 `json:"-"`
	LeafN     int     `json:"ln"`
	LeafK     float64 `json:"lk"`
	LeafHit   bool    `json:"-"` // this swing has already struck
	LeafDX    float64 `json:"-"` // the hop's unit direction
	LeafDY    float64 `json:"-"`
	LeafCD    float64 `json:"lcd"` // counted from the start of the hop

	// Razor Leaf cooldown; Vine Whip: seconds of swing left, the angle each vine is at now (degrees off the aim,
	// they swing in from sweep_angle to 0), the locked aim, whether it has struck, and the cooldown.
	RazorCD     float64 `json:"rcd"`
	VineChargeT float64 `json:"vc"` // seconds of unfurling left before the swing; the aim still follows the stick
	VineRecT    float64 `json:"vr"` // seconds of recovery left after the swing
	VineT       float64 `json:"vt"`
	VineA       float64 `json:"va"`
	VineX       float64 `json:"vx"`
	VineY       float64 `json:"vy"`
	VineHit     bool    `json:"-"`
	VineCD      float64 `json:"vcd"`
	BoundT      float64 `json:"bd"` // seconds left bound by a vine: cannot walk or use a movement skill
}

// Leaf Slash phases.
const (
	leafNone = iota
	leafHop
	leafSlash
)

// Seismic Toss phases.
const (
	tossNone = iota
	tossDash
	tossRise
	tossFall
)

// The two kinds of projectile. Both live in Game.Balls and are hurt by the same skills; a fire ball is
// Charizard's and a plain Ball is Pikachu's Electro Ball.
const (
	ballElectro = 0
	ballFire    = 1
	ballLeaf    = 2 // Chikorita's Razor Leaf
)

// Ball is a flying projectile: an Electro Ball flies straight until it explodes, a fire ball until it
// touches the opponent, runs out of range or HP, or reaches a wall.
type Ball struct {
	K         int     `json:"k,omitempty"` // ballElectro | ballFire | ballLeaf
	Owner     int     `json:"-"`           // the slot that threw it; the other slot is its target
	Left      float64 `json:"-"`           // fire ball: range left
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	DX        float64 `json:"dx,omitempty"` // unit direction
	DY        float64 `json:"dy,omitempty"`
	Delay     float64 `json:"-"`           // leaf: seconds before it shoots
	Wait      bool    `json:"w,omitempty"` // leaf: still hanging around Chikorita
	HP        int     `json:"hp"`
	LastSweep int     `json:"-"` // the Hyper Beam sweep that last hit it
	HitCD     float64 `json:"-"` // until the beam may hit it again (hyper_beam.hit_interval)
	LastPunch int     `json:"-"` // the Ice Punch that last hit it
}

// Event is a one-shot effect the client plays (ring, screen shake).
type Event struct {
	T string  `json:"t"` // "slam" | "leaf" | "thunder" | "skybolt" | "explode" | "freeze" | "paralyze"
	X float64 `json:"x"`
	Y float64 `json:"y"`
	R float64 `json:"r,omitempty"` // slam reach, so the effect can match it
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
	rng   *rand.Rand // the fire's random spread; tests replace it
}

func NewGame(cfg Config) *Game { return NewGameFor(cfg, modeWeb) }

func NewGameFor(cfg Config, mode string) *Game {
	g := &Game{cfg: cfg, court: cfg.courtFor(mode), rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))}
	g.P[0].Kind, g.P[1].Kind = pikachu, snorlax
	g.Reset()
	return g
}

// SetKinds picks the characters of slots 0 and 1 and starts a fresh round.
func (g *Game) SetKinds(a, b int) {
	g.P[0].Kind, g.P[1].Kind = a, b
	g.Reset()
}

// Reset starts a round over with the same characters: 400 apart, centred in the arena.
func (g *Game) Reset() {
	c := g.court
	g.P[0] = Player{Kind: g.P[0].Kind, X: c.Length/2 - spawnHalfGap, Y: c.Width / 2, HP: g.cfg.maxHPOf(g.P[0].Kind), FX: 1, AimX: 1}
	g.P[1] = Player{Kind: g.P[1].Kind, X: c.Length/2 + spawnHalfGap, Y: c.Width / 2, HP: g.cfg.maxHPOf(g.P[1].Kind), FX: -1, AimX: -1}
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
func (g *Game) radius(i int) float64 { return g.cfg.radiusOf(g.P[i].Kind) }

// ballRadius and ballMass are the size and weight of a projectile, which depend on whose it is.
func (g *Game) ballRadius(b *Ball) float64 {
	switch b.K {
	case ballFire:
		return g.cfg.Charizard.FireBreath.Radius
	case ballLeaf:
		return g.cfg.Chikorita.RazorLeaf.Radius
	}
	return g.cfg.Pikachu.ElectroBall.Radius
}

func (g *Game) ballMass(b *Ball) float64 {
	switch b.K {
	case ballFire:
		return g.cfg.Charizard.FireBreath.Mass
	case ballLeaf:
		return g.cfg.Chikorita.RazorLeaf.Mass
	}
	return g.cfg.Pikachu.ElectroBall.Mass
}

// knockBack pushes target d units straight away from (sx, sy), stopping at the walls. Standing exactly
// on it there is no "away", so it goes along (fx, fy).
func (g *Game) knockBack(sx, sy float64, target *Player, d, fx, fy float64) {
	target.X, target.Y = g.pushedAway(target.X, target.Y, sx, sy, d, fx, fy)
}

// The beam's hit cooldown counts down in whole ticks, so an interval would round up to the next tick
// (0.05s would become 0.067s, 0.08s would become 0.1s). To keep the average rate right, the time by
// which a countdown overshoots zero is credited to the next one: hitCDTick lets it go negative for the
// tick it crosses zero, and hitCDNext starts the next interval from that credit.
func hitCDTick(cd, dt float64) float64 {
	if cd > 0 {
		return cd - dt
	}
	return 0 // no cooldown running: any credit is spent
}

// hitCDNext is the cooldown after a hit made with cd on the clock. It cannot be shorter than 0, so an
// interval below one tick still means at most one hit per tick.
func hitCDNext(cd, interval float64) float64 { return math.Max(cd+interval, 0) }

// pushedAway is where a point at (x, y) ends up pushed d units away from (sx, sy), inside the court.
func (g *Game) pushedAway(x, y, sx, sy, d, fx, fy float64) (float64, float64) {
	dx, dy := x-sx, y-sy
	n := math.Hypot(dx, dy)
	if n < 1e-9 {
		dx, dy, n = fx, fy, math.Hypot(fx, fy)
	}
	return math.Min(math.Max(x+dx/n*d, 0), g.court.Length), math.Min(math.Max(y+dy/n*d, 0), g.court.Width)
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

// groundReach is the default band of a ground-level skill (0 up to groundZ); anyHeight is for skills that
// come from the sky or have no height limit.
var (
	groundReach = Reach{MinHeight: 0, MaxHeight: groundZ}
	anyHeight   = Reach{MinHeight: 0, MaxHeight: 1e6}
)

// Step advances the simulation by dt seconds and returns the effects to show.
func (g *Game) Step(dt float64) []Event {
	var events []Event
	for i := range g.P {
		p, other := &g.P[i], &g.P[1-i]

		// A frozen or held player is deaf to its keys until it thaws or lands: the held keys take effect
		// again afterwards.
		in := p.In
		if p.FrozenT > 0 || p.Held {
			if p.FrozenT > 0 {
				if p.FrozenT -= dt; p.FrozenT < 1e-9 {
					p.FrozenT = 0
				}
			}
			in = Input{}
			p.DashT, p.BallT, p.BreathT = 0, 0, 0 // a dash, a charge or a breath in progress is cut short
			g.dropToss(i)                         // a Charizard caught mid-toss lets go
			g.dropLeaf(i)                         // a Chikorita mid-hop or mid-slash is stopped
			p.VineChargeT, p.VineT, p.VineRecT = 0, 0, 0
		}
		// Bound by a vine or paralyzed by a Volt Tackle: no walking and no movement skill (the others still work).
		if p.BoundT > 0 || p.ParalyzeT > 0 {
			in.DX, in.DY, in.Skill, in.Punch, in.Toss = 0, 0, false, false, false
		}
		for _, t := range []*float64{&p.BoundT, &p.ParalyzeT} {
			if *t > 0 {
				if *t -= dt; *t < 1e-9 {
					*t = 0
				}
			}
		}
		if in.DX != 0 || in.DY != 0 {
			p.FX, p.FY = in.DX, in.DY
		}
		// The aim keeps its last value when nothing is held, like the facing.
		if n := math.Hypot(in.AimX, in.AimY); n > 1e-9 && !math.IsInf(n, 0) {
			p.AimX, p.AimY = in.AimX/n, in.AimY/n
		} else if in.DX != 0 || in.DY != 0 {
			n := math.Hypot(float64(in.DX), float64(in.DY))
			p.AimX, p.AimY = float64(in.DX)/n, float64(in.DY)/n
		}
		if p.DashCD > 0 {
			p.DashCD = math.Max(0, p.DashCD-dt)
		}

		// Pikachu's skill, Volt Tackle: dash toward the held direction (or where it faces). A chain of max_uses
		// starts with the first press; the cooldown starts when the last one is spent or use_window runs out.
		vt := g.cfg.Pikachu.VoltTackle
		if p.VoltLeft > 0 {
			if p.VoltT -= dt; p.VoltT < 1e-9 {
				p.VoltLeft, p.VoltT, p.DashCD = 0, 0, vt.Cooldown
			}
		}
		if p.Kind == pikachu && in.Skill && p.DashT == 0 && (p.VoltLeft > 0 || p.DashCD == 0) && p.ThunderLeft == 0 && p.BallT == 0 {
			dx, dy := float64(in.DX), float64(in.DY)
			if dx == 0 && dy == 0 {
				dx, dy = float64(p.FX), float64(p.FY)
			}
			n := math.Hypot(dx, dy)
			p.DashX, p.DashY = dx/n, dy/n
			p.DashT, p.VoltHit = vt.Duration, false
			if p.VoltLeft == 0 {
				p.VoltLeft, p.VoltT = vt.MaxUses, vt.UseWindow
			}
			if p.VoltLeft--; p.VoltLeft == 0 {
				p.VoltT, p.DashCD = 0, vt.Cooldown
			}
		}

		// Snorlax's Hyper Beam: charge -> sweep -> recharge. It is rooted through all
		// three. The aim is locked on the opponent's position when the beam is used.
		busy := false
		casting := false // Pikachu is rooted while Thunder runs
		if p.Kind == snorlax {
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
			punchBusy := p.PunchT > 0 || p.PunchLeft > 0 || p.PunchRecover > 0
			if p.BeamT == 0 && p.SweepT == 0 && p.RechargeT == 0 && p.BeamCD == 0 && in.Beam && p.Z == 0 && !punchBusy {
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
					for j := range g.Balls {
						g.Balls[j].HitCD = 0 // a new sweep hits at once
					}
				}
			} else if p.SweepT > 0 {
				prev := p.BeamA
				if p.SweepT -= dt; p.SweepT < 1e-9 {
					p.SweepT = 0
				}
				p.BeamA = hb.SweepAngle * (2*p.SweepT/hb.SweepDuration - 1) // +angle -> -angle
				p.BeamHitCD = hitCDTick(p.BeamHitCD, dt)
				// Like the landing shock, the beam travels along the ground and misses airborne targets.
				// It hits again every hit_interval for as long as the target stays in the beam (0 = once per sweep).
				ready := p.BeamHitCD <= 1e-9 && (hb.HitInterval > 0 || !p.SweptHit)
				if ready && hb.Reach.hits(other) && sweepHits(p, other, p.BeamX, p.BeamY, prev, p.BeamA, hb.Length+g.radius(1-i), hb.Width+2*g.radius(1-i)) {
					g.hit(other, hb.Damage)
					p.SweptHit, p.BeamHitCD = true, hitCDNext(p.BeamHitCD, hb.HitInterval)
					g.knockBack(p.X, p.Y, other, hb.Knockback, p.BeamX, p.BeamY)
				}
				// A ball is hit like a player: at once, then every hit_interval while it stays in the beam
				// (0 = once per sweep). Each hit also pushes it away from Snorlax by the beam's mass minus
				// the ball's, so a heavy beam can hold a light ball back while the ball keeps flying forward.
				for j := range g.Balls {
					b := &g.Balls[j]
					br, push := g.ballRadius(b), math.Max(0, hb.Mass-g.ballMass(b))
					b.HitCD = hitCDTick(b.HitCD, dt)
					ready := b.HitCD <= 1e-9 && (hb.HitInterval > 0 || b.LastSweep != p.SweepID)
					if ready && sweepHits(p, &Player{X: b.X, Y: b.Y}, p.BeamX, p.BeamY, prev, p.BeamA, hb.Length+br, hb.Width+2*br) {
						b.HP -= hb.Damage
						b.LastSweep, b.HitCD = p.SweepID, hitCDNext(b.HitCD, hb.HitInterval)
						b.X, b.Y = g.pushedAway(b.X, b.Y, p.X, p.Y, push, p.BeamX, p.BeamY)
					}
				}
				if p.SweepT == 0 {
					p.RechargeT = hb.RechargeDuration
				}
			}
			events = append(events, g.stepIcePunch(i, in, dt)...)
			busy = p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0 || p.PunchT > 0 || p.PunchLeft > 0 || p.PunchRecover > 0
		}

		if p.Kind == charizard {
			events = append(events, g.stepCharizard(i, in, dt)...)
			casting = p.TossPhase != tossNone // rooted but for the dash itself, which moves it
		}

		if p.Kind == chikorita {
			events = append(events, g.stepChikorita(i, in, dt)...)
			casting = p.LeafPhase != leafNone || p.vineBusy() // the hop and the advance move it themselves; the vines root it
		}

		// Pikachu's Thunder. Pikachu cannot move until the last bolt has struck, and a
		// new cast waits for the current one to finish. Bolts come from the sky, so
		// height gives no cover. Each strike is also fired up into the sky from Pikachu.
		if p.Kind == pikachu {
			th := g.cfg.Pikachu.Thunder
			if p.ThunderCD > 0 {
				if p.ThunderCD -= dt; p.ThunderCD < 1e-9 {
					p.ThunderCD = 0
				}
			}
			if in.Thunder && p.ThunderLeft == 0 && p.ThunderCD == 0 && p.BallT == 0 {
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
					if th.Reach.hits(other) && math.Hypot(other.X-p.ShadowX, other.Y-p.ShadowY) <= th.Radius+g.radius(1-i) {
						g.hit(other, th.Damage)
					}
					p.ThunderLeft--
					p.ThunderIn += th.Interval
				}
			}
		}

		// Pikachu's Electro Ball: charge rooted, then throw it at the opponent. The aim follows the opponent
		// while charging, so the ball goes where it stands when the charge ends. Only one charge or Thunder
		// cast at a time.
		if p.Kind == pikachu {
			bc := g.cfg.Pikachu.ElectroBall
			if p.BallCD > 0 {
				if p.BallCD -= dt; p.BallCD < 1e-9 {
					p.BallCD = 0
				}
			}
			if in.Ball && p.BallT == 0 && p.ThunderLeft == 0 && p.BallCD == 0 {
				p.BallT, p.BallCD = bc.ChargeTime, bc.Cooldown
				p.DashT = 0 // charging roots Pikachu at once
			}
			if p.BallT > 0 {
				casting = true                                                     // rooted through the tick of the throw, free from the next
				if dx, dy := other.X-p.X, other.Y-p.Y; math.Hypot(dx, dy) > 1e-9 { // on top of it there is no direction: the aim stays
					n := math.Hypot(dx, dy)
					p.AimX, p.AimY = dx/n, dy/n
				}
				if p.BallT -= dt; p.BallT < 1e-9 {
					p.BallT = 0
					off := g.radius(i) + bc.Radius + 1 // start just clear of Pikachu's body
					g.Balls = append(g.Balls, Ball{Owner: i, X: p.X + p.AimX*off, Y: p.Y + p.AimY*off, DX: p.AimX, DY: p.AimY, HP: bc.HP})
				}
			}
		}

		if p.DashT > 0 {
			// Steering is locked while dashing. The last step is clipped so the
			// total distance is exact whatever the tick length.
			step := math.Min(dt, p.DashT)
			v := vt.Distance / vt.Duration
			x0, y0 := p.X, p.Y
			p.X = math.Min(math.Max(p.X+p.DashX*v*step, 0), g.court.Length)
			p.Y = math.Min(math.Max(p.Y+p.DashY*v*step, 0), g.court.Width)
			if p.DashT -= step; p.DashT < 1e-9 {
				p.DashT = 0
			}
			if !p.VoltHit && vt.Reach.hits(other) && segDist(other.X, other.Y, x0, y0, p.X, p.Y) <= g.radius(i)+g.radius(1-i) {
				events = append(events, g.contact(i, vt.Mass)...)
			}
		} else if n := math.Hypot(float64(in.DX), float64(in.DY)); !busy && !casting && n > 0 {
			// Normalise diagonals so they are not faster. Snorlax steers at its own speeds in
			// the air: rising is the first half of the jump, falling the second.
			v := g.cfg.walkSpeedOf(p.Kind)
			if p.Kind == charizard && p.FlyT > 0 {
				v = g.cfg.Charizard.Fly.Speed
			}
			if p.Kind == snorlax && p.Z > 0 {
				v = g.cfg.Snorlax.Jump.AirSpeedSecondHalf
				if p.VZ > 0 {
					v = g.cfg.Snorlax.Jump.AirSpeedFirstHalf
				}
			}
			p.X += float64(in.DX) / n * v * dt
			p.Y += float64(in.DY) / n * v * dt
		}
		p.X = math.Min(math.Max(p.X, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y, 0), g.court.Width)

		// Snorlax's skill: jump. Exact parabola so height and air time match the config.
		if p.Kind == snorlax && in.Skill && p.Z == 0 && !busy && p.JumpCD == 0 {
			p.VZ = g.cfg.jumpV()
			p.JumpCD = g.cfg.Snorlax.Jump.Cooldown // counted from the moment of take-off
		}
		landed := false
		if p.Kind == snorlax && !p.Held && (p.Z > 0 || p.VZ > 0) {
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
			if dist(p, other) < g.cfg.Snorlax.Jump.SlamRadius+g.radius(1-i) && g.cfg.Snorlax.Jump.Reach.hits(other) {
				g.hit(other, slamDamage)
			}
			for j := range g.Balls { // the shock wave is a ground attack: it hits balls too
				b := &g.Balls[j]
				if math.Hypot(b.X-p.X, b.Y-p.Y) < g.cfg.Snorlax.Jump.SlamRadius+g.ballRadius(b) {
					b.HP -= slamDamage
				}
			}
		}
	}
	return append(events, g.stepBalls(dt)...)
}

// stepIcePunch runs Snorlax's Ice Punch: charge -> dash -> recharge, rooted through all three. The aim is
// the direction held when the charge ends. The fist hits whatever it touches along the dash, once.
func (g *Game) stepIcePunch(i int, in Input, dt float64) []Event {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	ip := g.cfg.Snorlax.IcePunch
	if p.PunchCD > 0 {
		if p.PunchCD -= dt; p.PunchCD < 1e-9 {
			p.PunchCD = 0
		}
	}
	idle := p.PunchT == 0 && p.PunchLeft == 0 && p.PunchRecover == 0
	if in.Punch && idle && p.BeamT == 0 && p.SweepT == 0 && p.RechargeT == 0 && p.Z == 0 && p.PunchCD == 0 {
		p.PunchT, p.PunchCD = ip.ChargeDuration, ip.Cooldown
	}
	switch {
	case p.PunchT > 0:
		if p.PunchT -= dt; p.PunchT < 1e-9 { // charged: go
			p.PunchT = 0
			p.PunchDX, p.PunchDY = p.AimX, p.AimY
			p.PunchLeft, p.PunchHit = ip.Distance, false
			p.PunchID++
		}
	case p.PunchLeft > 0:
		step := math.Min(ip.Speed*dt, p.PunchLeft)
		x0, y0 := p.X, p.Y
		p.X = math.Min(math.Max(p.X+p.PunchDX*step, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+p.PunchDY*step, 0), g.court.Width)
		if p.PunchLeft -= step; p.PunchLeft < 1e-9 {
			p.PunchLeft, p.PunchRecover = 0, ip.RechargeDuration
		}
		// Like the other ground attacks it misses an airborne opponent. Tested along the whole step so a
		// fast fist cannot jump over a body between two ticks.
		if !p.PunchHit && ip.Reach.hits(other) && segDist(other.X, other.Y, x0, y0, p.X, p.Y) <= ip.Radius+g.radius(1-i) {
			events = append(events, g.contact(i, ip.Mass)...)
		}
		for j := range g.Balls {
			b := &g.Balls[j]
			if b.LastPunch != p.PunchID && segDist(b.X, b.Y, x0, y0, p.X, p.Y) <= ip.Radius+g.ballRadius(b) {
				b.LastPunch = p.PunchID
				b.HP -= ip.Damage
				// carried along the dash by the mass the punch has over the ball
				push := math.Max(0, ip.Mass-g.ballMass(b))
				b.X, b.Y = g.pushedAway(b.X, b.Y, b.X-p.PunchDX, b.Y-p.PunchDY, push, p.PunchDX, p.PunchDY)
			}
		}
	case p.PunchRecover > 0:
		if p.PunchRecover -= dt; p.PunchRecover < 1e-9 {
			p.PunchRecover = 0
		}
	}
	return events
}

// stepBalls flies the projectiles. An Electro Ball blows up when it touches the opponent, reaches a wall or
// runs out of HP, and the blast also burns the fire balls and leaves caught in it. A Razor Leaf hangs where it was
// scattered until its delay is up, then shoots at the opponent and is spent when it touches it, reaches a wall or runs out of HP. A fire ball is spent when it touches the
// opponent (hurting it), reaches a wall, runs out of range or runs out of HP.
func (g *Game) stepBalls(dt float64) []Event {
	var events []Event
	bc, fb, rl := g.cfg.Pikachu.ElectroBall, g.cfg.Charizard.FireBreath, g.cfg.Chikorita.RazorLeaf
	type blast struct{ x, y float64 }
	var blasts []blast
	kept := g.Balls[:0]
	for _, b := range g.Balls {
		foe := &g.P[1-b.Owner]
		if b.K == ballFire {
			if b.HP > 0 {
				x0, y0 := b.X, b.Y
				step := math.Min(fb.Speed*dt, b.Left)
				b.X += b.DX * step
				b.Y += b.DY * step
				b.Left -= step
				// Along the whole step, so a fast fire ball cannot jump over the opponent.
				if fb.Reach.hits(foe) && segDist(foe.X, foe.Y, x0, y0, b.X, b.Y) <= fb.Radius+g.radius(1-b.Owner) {
					g.hit(foe, fb.Damage)
					continue
				}
				inside := b.X > 0 && b.X < g.court.Length && b.Y > 0 && b.Y < g.court.Width
				if inside && b.Left > 1e-9 {
					kept = append(kept, b)
				}
			}
			continue
		}
		if b.K == ballLeaf {
			if b.HP > 0 {
				if b.Wait { // hanging around Chikorita until its delay is up, then it picks its target
					if b.Delay -= dt; b.Delay <= 1e-9 {
						dx, dy := foe.X-b.X, foe.Y-b.Y
						if n := math.Hypot(dx, dy); n > 1e-9 {
							b.DX, b.DY, b.Wait = dx/n, dy/n, false
						} else {
							b.DX, b.DY, b.Wait = float64(g.P[b.Owner].FX), float64(g.P[b.Owner].FY), false
							b.DX, b.DY = b.DX/math.Hypot(b.DX, b.DY), b.DY/math.Hypot(b.DX, b.DY)
						}
					}
					kept = append(kept, b)
					continue
				}
				x0, y0 := b.X, b.Y
				b.X += b.DX * rl.Speed * dt
				b.Y += b.DY * rl.Speed * dt
				if rl.Reach.hits(foe) && segDist(foe.X, foe.Y, x0, y0, b.X, b.Y) <= rl.Radius+g.radius(1-b.Owner) {
					g.hit(foe, rl.Damage)
					events = append(events, Event{T: "leaf", X: foe.X, Y: foe.Y})
					continue
				}
				if b.X > 0 && b.X < g.court.Length && b.Y > 0 && b.Y < g.court.Width {
					kept = append(kept, b)
				}
			}
			continue
		}
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
			// Outside the ball's heights it flies past the opponent.
			if bc.Reach.hits(foe) && segDist(foe.X, foe.Y, x0, y0, b.X, b.Y) <= bc.Radius+g.radius(1-b.Owner) {
				b.X, b.Y = closestOnSegment(foe.X, foe.Y, x0, y0, b.X, b.Y) // it blows up where it met the opponent
				g.hit(foe, bc.ContactDamage)                                // the ball itself hurts, then the blast does
				boom = true
			}
		}
		if !boom {
			kept = append(kept, b)
			continue
		}
		events = append(events, Event{T: "explode", X: b.X, Y: b.Y, R: bc.ExplosionRadius})
		blasts = append(blasts, blast{b.X, b.Y})
		if bc.Reach.hits(foe) && math.Hypot(foe.X-b.X, foe.Y-b.Y) <= bc.ExplosionRadius+g.radius(1-b.Owner) {
			g.hit(foe, bc.ExplosionDamage)
			g.knockBack(b.X, b.Y, foe, bc.Knockback, b.DX, b.DY)
		}
	}
	g.Balls = kept
	// The blast burns the fire balls near it; they are gone on the next step.
	for _, e := range blasts {
		for j := range g.Balls {
			if b := &g.Balls[j]; b.K != ballElectro && math.Hypot(b.X-e.x, b.Y-e.y) <= bc.ExplosionRadius+g.ballRadius(b) {
				b.HP -= bc.ExplosionDamage
			}
		}
	}
	return events
}

// stepCharizard runs Charizard's three skills.
//
// Fly: the space bar lifts it to fly.height for fly.duration seconds; it only moves faster (fly.speed), and
// each skill's min_height / max_height decide whether it still reaches it. Its cooldown starts when it has landed.
//
// Fire Breath: for `duration` seconds it breathes `quantity` fire balls, evenly spaced, each aimed at where
// the opponent stands when it leaves the mouth, then turned by a random angle within +-random_offset_angle.
// Charizard can walk or fly meanwhile.
//
// Seismic Toss: it runs `length` along the aim at `speed`. A body the run touches (radius) is caught: Charizard
// carries it up `high` and slams it down, where it takes `damage`.
func (g *Game) stepCharizard(i int, in Input, dt float64) []Event {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	cz := g.cfg.Charizard
	stunned := p.FrozenT > 0 || p.Held // in is already empty then; this only stops new skills
	tick := func(v *float64) {
		if *v > 0 {
			if *v -= dt; *v < 1e-9 {
				*v = 0
			}
		}
	}
	tick(&p.FlyCD)
	tick(&p.BreathCD)
	tick(&p.TossCD)

	// ---- Fly ----
	rise := cz.Fly.Height / flyRiseTime
	if in.Skill && p.FlyT == 0 && p.FlyCD == 0 && p.Z == 0 && p.TossPhase == tossNone && !stunned {
		p.FlyT = cz.Fly.Duration
	}
	if p.TossPhase == tossNone {
		if p.FlyT > 0 {
			if p.FlyT -= dt; p.FlyT < 1e-9 {
				p.FlyT = 0
			}
			p.Z = math.Min(p.Z+rise*dt, cz.Fly.Height)
		} else if p.Z > 0 {
			if p.Z -= rise * dt; p.Z <= 1e-9 {
				p.Z, p.FlyCD = 0, cz.Fly.Cooldown
			}
		}
	}

	// ---- Fire Breath ----
	fb := cz.FireBreath
	if in.Fire && p.BreathT == 0 && p.BreathCD == 0 && p.TossPhase == tossNone && !stunned {
		p.BreathT, p.BreathLeft, p.BreathN, p.BreathCD = fb.Duration, fb.Duration, 0, fb.Cooldown
	}
	if p.BreathT > 0 {
		if p.BreathT -= dt; p.BreathT < 1e-9 {
			p.BreathT = 0
		}
		due := fb.Quantity // all of them once the time is up
		if p.BreathT > 0 {
			due = int(float64(fb.Quantity)*(1-p.BreathT/p.BreathLeft) + 1e-9)
		}
		for ; p.BreathN < due; p.BreathN++ {
			dx, dy := other.X-p.X, other.Y-p.Y
			n := math.Hypot(dx, dy)
			if n < 1e-9 { // standing on the opponent: no direction, so use the facing
				dx, dy = float64(p.FX), float64(p.FY)
				n = math.Hypot(dx, dy)
			}
			a := (g.rng.Float64()*2 - 1) * fb.RandomOffsetAngle * math.Pi / 180
			ux, uy := dx/n, dy/n
			ux, uy = ux*math.Cos(a)-uy*math.Sin(a), ux*math.Sin(a)+uy*math.Cos(a)
			off := g.radius(i) + fb.Radius + 1 // starts just clear of Charizard's body
			g.Balls = append(g.Balls, Ball{K: ballFire, Owner: i, Left: fb.Length, X: p.X + ux*off, Y: p.Y + uy*off, DX: ux, DY: uy, HP: fb.HP})
		}
	}

	// ---- Seismic Toss ----
	ts := cz.SeismicToss
	if in.Toss && p.TossPhase == tossNone && p.TossCD == 0 && !stunned {
		// It may start in the air: the flight ends there and the run goes on at that height; the flight cooldown
		// starts when Charizard is back on the ground.
		p.TossAir, p.FlyT = p.Z > 0, 0
		p.TossPhase, p.TossLeft, p.TossCD = tossDash, ts.Length, ts.Cooldown
		p.TossDX, p.TossDY = p.AimX, p.AimY
		p.BreathT = 0 // it is all fire on the run: the breath stops
	}
	switch p.TossPhase {
	case tossDash:
		step := math.Min(ts.Speed*dt, p.TossLeft)
		x0, y0 := p.X, p.Y
		p.X = math.Min(math.Max(p.X+p.TossDX*step, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+p.TossDY*step, 0), g.court.Width)
		p.TossLeft -= step
		// A ground attack like the punch: it misses an opponent who is in the air. Tested along the whole step.
		if ts.Reach.hits(other) && segDist(other.X, other.Y, x0, y0, p.X, p.Y) <= ts.Radius+g.radius(1-i) {
			events = append(events, g.contact(i, ts.Mass)...)
		}
		if p.TossPhase == tossDash && p.TossLeft < 1e-9 {
			p.TossPhase = tossNone // missed
		}
	case tossRise:
		p.Z = math.Min(p.Z+ts.Speed*dt, ts.High)
		if p.Z >= ts.High {
			p.TossPhase = tossFall
		}
	case tossFall:
		if p.Z -= ts.Speed * dt; p.Z <= 1e-9 {
			p.Z, p.TossPhase = 0, tossNone
			if p.TossAir {
				p.FlyCD = cz.Fly.Cooldown
			}
			g.hit(other, ts.Damage)
			other.Held, other.Z = false, 0
			events = append(events, Event{T: "slam", X: other.X, Y: other.Y, R: ts.Radius}) // the same shock as Snorlax's landing
		}
	}
	if other.Held && p.TossPhase != tossNone { // the caught body hangs in front of Charizard
		off := g.radius(i) + g.radius(1-i)
		other.X = math.Min(math.Max(p.X+p.TossDX*off, 0), g.court.Length)
		other.Y = math.Min(math.Max(p.Y+p.TossDY*off, 0), g.court.Width)
		other.Z = p.Z
	}
	return events
}

// stepChikorita runs Chikorita's Leaf Slash: a hop the way it walks (nothing happens when it lands), then
// `slashes` swings of the leaf, alternating upper-left to lower-right and upper-right to lower-left. During a
// swing it moves slash_step towards the opponent (stopping where the bodies touch), and at the middle of the swing
// the blade hits anyone in reach within slash_radius.
func (g *Game) stepChikorita(i int, in Input, dt float64) []Event {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	ls := g.cfg.Chikorita.LeafSlash
	if p.LeafCD > 0 {
		if p.LeafCD -= dt; p.LeafCD < 1e-9 {
			p.LeafCD = 0
		}
	}
	if in.Skill && p.LeafPhase == leafNone && !p.vineBusy() && p.LeafCD == 0 && p.FrozenT == 0 && !p.Held {
		dx, dy := float64(in.DX), float64(in.DY)
		if dx == 0 && dy == 0 {
			dx, dy = float64(p.FX), float64(p.FY)
		}
		n := math.Hypot(dx, dy)
		p.LeafDX, p.LeafDY = dx/n, dy/n
		p.LeafPhase, p.LeafT, p.LeafK, p.LeafN, p.LeafCD = leafHop, ls.JumpDuration, 0, 0, ls.Cooldown
	}
	switch p.LeafPhase {
	case leafHop:
		step := math.Min(dt, p.LeafT) // the last step is clipped so the distance is exact
		v := ls.JumpDistance / ls.JumpDuration
		p.X = math.Min(math.Max(p.X+p.LeafDX*v*step, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+p.LeafDY*v*step, 0), g.court.Width)
		if p.LeafT -= step; p.LeafT < 1e-9 {
			p.Z, p.LeafPhase, p.LeafT, p.LeafK, p.LeafHit = 0, leafSlash, ls.SlashInterval, 0, false
			break
		}
		u := 1 - p.LeafT/ls.JumpDuration
		p.LeafK, p.Z = u, 4*ls.JumpHeight*u*(1-u) // a parabola peaking at jump_height
	case leafSlash:
		if d := dist(p, other); d > 1e-9 {
			// A swing's time carries over into the next tick, so only the very last tick is clipped: the swings add
			// up to exactly slashes x slash_step.
			used := dt
			if p.LeafN == ls.Slashes-1 {
				used = math.Min(dt, p.LeafT)
			}
			adv := math.Min(ls.SlashStep*used/ls.SlashInterval, math.Max(0, d-g.radius(i)-g.radius(1-i)))
			p.X = math.Min(math.Max(p.X+(other.X-p.X)/d*adv, 0), g.court.Length)
			p.Y = math.Min(math.Max(p.Y+(other.Y-p.Y)/d*adv, 0), g.court.Width)
		}
		p.LeafT -= dt
		p.LeafK = math.Min(math.Max(1-p.LeafT/ls.SlashInterval, 0), 1)
		if !p.LeafHit && p.LeafK >= 0.5 {
			p.LeafHit = true
			if ls.Reach.hits(other) && dist(p, other) <= ls.SlashRadius+g.radius(1-i) {
				g.hit(other, ls.Damage)
				events = append(events, Event{T: "leaf", X: other.X, Y: other.Y})
			}
		}
		if p.LeafT < 1e-9 {
			if p.LeafN++; p.LeafN >= ls.Slashes {
				p.LeafPhase, p.LeafK, p.LeafN = leafNone, 0, 0
			} else {
				p.LeafT += ls.SlashInterval
				p.LeafHit = false
			}
		}
	}
	events = append(events, g.stepChikoritaVines(i, in, dt)...)
	return events
}

// stepChikoritaVines runs Chikorita's two small skills: the Razor Leaf cast and the Vine Whip swing.
func (g *Game) stepChikoritaVines(i int, in Input, dt float64) []Event {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	rl, vw := g.cfg.Chikorita.RazorLeaf, g.cfg.Chikorita.VineWhip
	ready := p.LeafPhase == leafNone && !p.vineBusy() && p.Z == 0 && p.FrozenT == 0 && !p.Held
	for _, cd := range []*float64{&p.RazorCD, &p.VineCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}

	// ---- Razor Leaf: the leaves are scattered at once and wait for their own delay ----
	if in.Razor && ready && p.RazorCD == 0 {
		p.RazorCD = rl.Cooldown
		near := g.radius(i) + rl.Radius // not inside the body
		for n := 0; n < rl.Count; n++ {
			a := g.rng.Float64() * 2 * math.Pi
			d := near + g.rng.Float64()*math.Max(rl.ScatterRadius-near, 0)
			g.Balls = append(g.Balls, Ball{
				K: ballLeaf, Owner: i, Wait: true, HP: rl.LeafHP,
				X:     math.Min(math.Max(p.X+math.Cos(a)*d, 0), g.court.Length),
				Y:     math.Min(math.Max(p.Y+math.Sin(a)*d, 0), g.court.Width),
				Delay: rl.MinDelay + g.rng.Float64()*(rl.MaxDelay-rl.MinDelay),
			})
		}
	}

	// ---- Vine Whip: two vines swing in from either side of the aim and meet on it ----
	if in.Vine && ready && p.VineCD == 0 {
		p.VineChargeT, p.VineHit, p.VineCD = vw.ChargeDuration, false, vw.Cooldown
		p.VineX, p.VineY = p.AimX, p.AimY
		if p.VineChargeT == 0 {
			p.VineT, p.VineA = vw.SweepDuration, vw.SweepAngle
		}
	}
	if p.VineChargeT > 0 { // unfurling: rooted, and the vines keep pointing where the stick does
		p.VineX, p.VineY = p.AimX, p.AimY
		if p.VineChargeT -= dt; p.VineChargeT < 1e-9 {
			p.VineChargeT = 0
			p.VineT, p.VineA = vw.SweepDuration, vw.SweepAngle // the aim is locked from here
		}
	}
	if p.VineT > 0 {
		prev := p.VineA
		if p.VineT -= dt; p.VineT < 1e-9 {
			p.VineT, p.VineRecT = 0, vw.RechargeDuration
		}
		p.VineA = vw.SweepAngle * p.VineT / vw.SweepDuration
		// The vines are mirror images, so one test along the swept angles of each covers both. Whichever touches
		// first hurts and binds; a second touch in the same swing does not.
		reach, w := vw.Length+g.radius(1-i), vw.Width+2*g.radius(1-i)
		if !p.VineHit && vw.Reach.hits(other) &&
			(sweepHits(p, other, p.VineX, p.VineY, prev, p.VineA, reach, w) || sweepHits(p, other, p.VineX, p.VineY, -p.VineA, -prev, reach, w)) {
			p.VineHit = true
			g.hit(other, vw.Damage)
			other.BoundT = math.Max(other.BoundT, vw.BindDuration)
			events = append(events, Event{T: "leaf", X: other.X, Y: other.Y})
		}
	}
	if p.VineRecT > 0 && p.VineT == 0 {
		if p.VineRecT -= dt; p.VineRecT < 1e-9 {
			p.VineRecT = 0
		}
	}
	return events
}

// vineBusy is whether Chikorita is anywhere in a Vine Whip: unfurling, swinging or pulling the vines back.
func (p *Player) vineBusy() bool { return p.VineChargeT > 0 || p.VineT > 0 || p.VineRecT > 0 }

// dropLeaf stops a Leaf Slash in progress (Chikorita was frozen or caught): it comes straight down.
func (g *Game) dropLeaf(i int) {
	p := &g.P[i]
	if p.LeafPhase == leafNone {
		return
	}
	if p.LeafPhase == leafHop {
		p.Z = 0
	}
	p.LeafPhase, p.LeafK, p.LeafN = leafNone, 0, 0
}

// dropToss lets go of a caught opponent: Charizard was frozen (or caught itself) mid-toss. The opponent falls
// where it is, unhurt.
func (g *Game) dropToss(i int) {
	p, other := &g.P[i], &g.P[1-i]
	if p.TossPhase == tossNone {
		return
	}
	if other.Held {
		other.Held, other.Z = false, 0
	}
	if p.TossPhase != tossDash {
		p.Z = 0
		if p.TossAir {
			p.FlyCD = g.cfg.Charizard.Fly.Cooldown
		}
	}
	p.TossPhase = tossNone
}

// runningMass is the mass of the attack player j is in the middle of running into things (Volt Tackle, Ice Punch
// or Seismic Toss dash), if any.
func (g *Game) runningMass(j int) (float64, bool) {
	p := &g.P[j]
	switch {
	case p.Kind == pikachu && p.DashT > 0:
		return g.cfg.Pikachu.VoltTackle.Mass, true
	case p.Kind == snorlax && p.PunchLeft > 0:
		return g.cfg.Snorlax.IcePunch.Mass, true
	case p.Kind == charizard && p.TossPhase == tossDash:
		return g.cfg.Charizard.SeismicToss.Mass, true
	}
	return 0, false
}

// contact resolves player i's running attack (of this mass) touching the opponent. If the opponent is running
// an attack of its own, the heavier mass wins: it lands, and the lighter attack is cut short and does nothing.
// Equal masses cancel each other.
func (g *Game) contact(i int, mass float64) []Event {
	j := 1 - i
	mj, clash := g.runningMass(j)
	switch {
	case !clash:
		return g.land(i, j)
	case mj > mass:
		g.cut(i)
		return g.land(j, i)
	case mj < mass:
		g.cut(j)
		return g.land(i, j)
	}
	g.cut(i)
	g.cut(j)
	return nil
}

// cut stops the running attack of player i.
func (g *Game) cut(i int) {
	p := &g.P[i]
	switch p.Kind {
	case pikachu:
		p.DashT = 0
	case snorlax:
		p.PunchLeft, p.PunchRecover = 0, g.cfg.Snorlax.IcePunch.RechargeDuration
	case charizard:
		p.TossPhase, p.TossLeft = tossNone, 0 // in the air it comes down through the fly code
	}
}

// land applies the effect of player w's running attack on victim v: a paralyzing hit, a freezing hit or a catch.
func (g *Game) land(w, v int) []Event {
	p, victim := &g.P[w], &g.P[v]
	switch p.Kind {
	case pikachu:
		vt := g.cfg.Pikachu.VoltTackle
		p.VoltHit = true
		g.hit(victim, vt.Damage)
		victim.ParalyzeT = math.Max(victim.ParalyzeT, vt.ParalyzeDuration)
		return []Event{{T: "paralyze", X: victim.X, Y: victim.Y}}
	case snorlax:
		ip := g.cfg.Snorlax.IcePunch
		p.PunchHit = true
		g.hit(victim, ip.Damage)
		victim.FrozenT = math.Max(victim.FrozenT, ip.FreezeDuration)
		return []Event{{T: "freeze", X: victim.X, Y: victim.Y}}
	case charizard:
		victim.Held, victim.VZ = true, 0
		p.TossPhase = tossRise
	}
	return nil
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
