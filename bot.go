package main

import (
	"math"
	"math/rand/v2"
)

// A Bot plays one slot the way a player would: every tick it is asked for the keys it holds, and the room puts
// them in that slot's Input, so the game cannot tell it from a person and it can do nothing a person cannot.
//
// It is meant to be beatable, not clever, so it plays with a human's handicaps:
//   - it looks at the opponent and its projectiles as they were botReact ticks ago;
//   - it decides only every botEvery ticks and holds the stick in between (a skill key is pressed for one tick);
//   - it takes a chance on some moves instead of always making the best one.
//
// Everything about how a character plays is in its own function (pikachu, snorlax); the rest here is shared: what
// threatens the bot, where to stand, and the 8-way stick.
const (
	botEvery = 4 // ticks between decisions
	botReact = 5 // ticks the bot's view of the opponent lags behind
)

// botKinds are the characters a Bot can play so far.
var botKinds = []int{pikachu, snorlax}

func botCanPlay(kind int) bool {
	for _, k := range botKinds {
		if k == kind {
			return true
		}
	}
	return false
}

type botSnap struct {
	foe   Player
	balls []Ball
}

type Bot struct {
	kind    int
	rng     *rand.Rand
	hist    []botSnap // the last botReact+1 ticks, newest last
	ticks   int
	held    Input   // the stick and aim, kept between decisions
	strafe  float64 // +1 / -1: which way it circles
	strafeT float64 // seconds before it may change its mind
	comboT  float64 // Snorlax: seconds left to follow a landing with the punch
	wasAir  bool
}

func newBot(kind int, seed uint64) *Bot {
	return &Bot{kind: kind, rng: rand.New(rand.NewPCG(seed, 7)), strafe: 1}
}

// reset forgets the round that just ended.
func (b *Bot) reset() {
	b.hist, b.ticks, b.held, b.comboT, b.wasAir = b.hist[:0], 0, Input{}, 0, false
}

// Decide is the bot's keys for the coming tick in slot me. It must be called once every tick.
func (b *Bot) Decide(g *Game, me int) Input {
	b.observe(g, me)
	b.ticks++
	if b.ticks%botEvery != 0 {
		return b.held // the skill keys are not held: they were pressed once, on the tick they were decided
	}
	if b.strafeT -= botEvery * dtBot; b.strafeT <= 0 {
		b.strafe, b.strafeT = []float64{-1, 1}[b.rng.IntN(2)], 0.8+1.2*b.rng.Float64()
	}
	c := b.look(g, me)
	var in Input
	switch b.kind {
	case pikachu:
		in = b.pikachu(c)
	case snorlax:
		in = b.snorlax(c)
	}
	b.held = Input{DX: in.DX, DY: in.DY, AimX: in.AimX, AimY: in.AimY}
	return in
}

const dtBot = 1.0 / tickRate

func (b *Bot) observe(g *Game, me int) {
	if len(b.hist) == botReact+1 {
		s := b.hist[0] // its balls buffer is reused for the newest tick
		copy(b.hist, b.hist[1:])
		b.hist[botReact] = s
	} else {
		b.hist = append(b.hist, botSnap{})
	}
	s := &b.hist[len(b.hist)-1]
	s.foe = g.P[1-me]
	s.balls = append(s.balls[:0], g.Balls...)
}

// bctx is what one decision looks at: itself as it is, the opponent and the projectiles as they were botReact
// ticks ago.
type bctx struct {
	g      *Game
	b      *Bot
	me     int
	p, f   *Player
	balls  []Ball
	pr, fr float64 // body radii
	dist   float64
	ux, uy float64 // unit vector from the bot to the opponent
	vx, vy float64 // how fast the opponent was moving
	th     threat
}

func (b *Bot) look(g *Game, me int) *bctx {
	old := &b.hist[0]
	c := &bctx{g: g, b: b, me: me, p: &g.P[me], f: &old.foe, balls: old.balls}
	c.pr, c.fr = g.cfg.radiusOf(c.p.Kind), g.cfg.radiusOf(c.f.Kind)
	dx, dy := c.f.X-c.p.X, c.f.Y-c.p.Y
	c.dist = math.Hypot(dx, dy)
	if c.dist > 1e-9 {
		c.ux, c.uy = dx/c.dist, dy/c.dist
	} else {
		c.ux = float64(c.p.FX)
	}
	if len(b.hist) > 1 {
		n := &b.hist[1].foe // the tick after the one it is looking at
		c.vx, c.vy = (n.X-c.f.X)/dtBot, (n.Y-c.f.Y)/dtBot
	}
	c.th = c.findThreat()
	return c
}

// ---- what threatens the bot ----

// The kinds of threat. The direction in threat.a* is a unit step that gets out of the way.
const (
	thNone   = iota
	thBall   // a projectile on a course that touches the bot (Snorlax can jump over it)
	thChase  // something that follows the bot: a tornado, an ice storm
	thBeam   // inside the fan of a Hyper Beam that is charging or sweeping
	thPunch  // on the line of an Ice Punch
	thLand   // near where a jumping Snorlax will come down
	thShadow // under a Thunder shadow
	thRush   // Pikachu is in the middle of a Volt Tackle chain (only Snorlax minds it)
)

type threat struct {
	kind   int
	eta    float64 // seconds until it arrives (can be 0: already here)
	ax, ay float64
}

func unit(x, y float64) (float64, float64) {
	n := math.Hypot(x, y)
	if n < 1e-9 {
		return 0, 0
	}
	return x / n, y / n
}

// sideOf is the unit vector at right angles to the line from (ox, oy) along (ux, uy) that points to where
// (px, py) is, so stepping along it leaves the line. On the line it takes the bot's current circling side.
func (c *bctx) sideOf(ox, oy, ux, uy, px, py float64) (float64, float64) {
	rx, ry := px-ox, py-oy
	nx, ny := -uy, ux
	switch s := rx*nx + ry*ny; {
	case s < 0:
		return -nx, -ny
	case s == 0:
		return nx * c.b.strafe, ny * c.b.strafe
	}
	return nx, ny
}

func (c *bctx) findThreat() threat {
	g, p, f := c.g, c.p, c.f
	cfg := &g.cfg
	best := threat{kind: thNone, eta: math.Inf(1)}
	consider := func(t threat) {
		if t.eta < best.eta {
			best = t
		}
	}
	lag := botReact * dtBot

	hz := c.hazards()
	var safe *[2]float64 // the way to go, worked out once for whatever of them is coming
	for _, h := range hz {
		hx, hy := h.x+h.vx*lag, h.y+h.vy*lag // where it is now, as far as the bot can tell
		rx, ry := p.X-hx, p.Y-hy
		v2 := h.vx*h.vx + h.vy*h.vy
		if v2 < 1e-9 {
			continue
		}
		if h.kind == thChase { // it never gives up, so stay out of its way for as long as it is near, not only when it is about to arrive
			if math.Hypot(rx, ry)-h.r-c.pr <= botChaseZone {
				if safe == nil {
					ax, ay := c.safeDir(hz)
					safe = &[2]float64{ax, ay}
				}
				consider(threat{thChase, 0.3, safe[0], safe[1]})
			}
			continue
		}
		t := (rx*h.vx + ry*h.vy) / v2
		if t < 0 || t > botHorizon {
			continue // it is going away, or it is still far
		}
		if math.Hypot(rx-h.vx*t, ry-h.vy*t) > h.r+c.pr+25 {
			continue
		}
		if safe == nil {
			ax, ay := c.safeDir(hz)
			safe = &[2]float64{ax, ay}
		}
		consider(threat{h.kind, t, safe[0], safe[1]})
	}

	if (f.Kind == snorlax || f.Kind == gyarados) && (f.BeamT > 0 || f.SweepT > 0) {
		hb := cfg.beamOf(f.Kind)
		rx, ry := p.X-f.X, p.Y-f.Y
		d := math.Hypot(rx, ry)
		if d < hb.Length+c.pr {
			cos := (rx*f.BeamX + ry*f.BeamY) / math.Max(d, 1e-9)
			if cos > 0 && math.Acos(math.Min(cos, 1))*180/math.Pi < hb.SweepAngle+8 {
				ax, ay := c.sideOf(f.X, f.Y, f.BeamX, f.BeamY, p.X, p.Y)
				consider(threat{thBeam, f.BeamT, ax, ay})
			}
		}
	}

	if f.Kind == snorlax && (f.PunchT > 0 || f.PunchLeft > 0) {
		ip := cfg.Snorlax.IcePunch
		dx, dy := f.PunchDX, f.PunchDY
		if f.PunchT > 0 { // the fist goes where the aim is when the charge ends
			dx, dy = f.AimX, f.AimY
		}
		if segDist(p.X, p.Y, f.X, f.Y, f.X+dx*ip.Distance, f.Y+dy*ip.Distance) < ip.Radius+c.pr+15 {
			ax, ay := c.sideOf(f.X, f.Y, dx, dy, p.X, p.Y)
			consider(threat{thPunch, f.PunchT, ax, ay})
		}
	}

	if f.Kind == snorlax && f.Z > 0 {
		jp := cfg.Snorlax.Jump
		if d := math.Hypot(p.X-f.X, p.Y-f.Y); d < jp.SlamRadius+c.pr+60 { // it steers in the air, so leave a margin
			gr := cfg.gravity()
			eta := (f.VZ + math.Sqrt(f.VZ*f.VZ+2*gr*f.Z)) / gr
			ax, ay := unit(p.X-f.X, p.Y-f.Y)
			consider(threat{thLand, math.Max(0, eta-lag), ax, ay})
		}
	}

	if f.Kind == pikachu && f.ThunderLeft > 0 {
		if math.Hypot(p.X-f.ShadowX, p.Y-f.ShadowY) < cfg.Pikachu.Thunder.Radius+c.pr+50 {
			ax, ay := unit(p.X-f.ShadowX, p.Y-f.ShadowY)
			consider(threat{thShadow, f.ThunderIn, ax, ay})
		}
	}

	if f.Kind == pikachu && p.Kind == snorlax && (f.DashT > 0 || f.VoltLeft > 0) && c.rootedFor() == 0 {
		ax, ay := unit(p.X-f.X, p.Y-f.Y) // a chain is on: the next dash is coming
		consider(threat{thRush, 0.3, ax, ay})
	}
	return best
}

// botHorizon is how far ahead (seconds) the bot looks for things coming at it.
const botHorizon = 1.2

// botChaseZone is how close (between the bodies' edges) a chaser may get before the bot walks away from it.
const botChaseZone = 220.0

// A hazard is something that hurts what it touches and moves: a projectile flies straight, a chaser (thChase)
// heads for the bot.
type hazard struct {
	kind         int
	x, y, vx, vy float64 // where the bot last saw it, and its velocity
	r            float64
}

func (c *bctx) hazards() []hazard {
	cfg, p, f := &c.g.cfg, c.p, c.f
	var hz []hazard
	chase := func(x, y, speed, r float64) {
		ux, uy := unit(p.X-x, p.Y-y)
		hz = append(hz, hazard{thChase, x, y, ux * speed, uy * speed, r})
	}
	for i := range c.balls {
		bl := &c.balls[i]
		if bl.Owner == c.me || bl.Wait || bl.HP <= 0 {
			continue
		}
		switch bl.K {
		case ballPillar:
		case ballTornado:
			if !bl.Caught {
				chase(bl.X, bl.Y, cfg.Rowlet.Tornado.Speed, cfg.Rowlet.Tornado.Radius)
			}
		default:
			sp := cfg.ballSpeed(bl.K)
			hz = append(hz, hazard{thBall, bl.X, bl.Y, bl.DX * sp, bl.DY * sp, c.g.ballRadius(bl)})
		}
	}
	if f.Kind == vulpix && f.StormLeft > 0 {
		chase(f.StormX, f.StormY, cfg.Vulpix.IceStorm.FollowSpeed, cfg.Vulpix.IceStorm.Radius)
	}
	return hz
}

// safeDir is the unit step (or none) that keeps the bot farthest from every hazard over the next botHorizon
// seconds, walking at its own speed: each of the eight stick positions is played forward, with projectiles
// flying straight and chasers heading for wherever the bot is then. That finds the gap in a volley, which one
// "step aside" from the nearest arrow does not.
func (c *bctx) safeDir(hz []hazard) (float64, float64) {
	const step = 0.1
	p, cfg := c.p, &c.g.cfg
	speed := cfg.walkSpeedOf(p.Kind)
	lag := botReact * dtBot
	bestScore, bx, by := math.Inf(-1), 0.0, 0.0
	pos := make([][2]float64, len(hz))
	for k := -1; k < 8; k++ {
		var ux, uy float64
		if k >= 0 {
			ux, uy = unit(float64(stickX[k]), float64(stickY[k]))
		}
		for i, h := range hz {
			pos[i] = [2]float64{h.x + h.vx*lag, h.y + h.vy*lag}
		}
		x, y, score := p.X, p.Y, math.Inf(1)
		for t := step; t <= botHorizon+1e-9; t += step {
			x = math.Min(math.Max(x+ux*speed*step, 0), c.g.court.Length)
			y = math.Min(math.Max(y+uy*speed*step, 0), c.g.court.Width)
			for i, h := range hz {
				nx, ny := pos[i][0]+h.vx*step, pos[i][1]+h.vy*step
				if h.kind == thChase {
					dx, dy := unit(x-pos[i][0], y-pos[i][1])
					nx, ny = pos[i][0]+dx*math.Hypot(h.vx, h.vy)*step, pos[i][1]+dy*math.Hypot(h.vx, h.vy)*step
				}
				// the nearest they get during the step, so a fast projectile cannot jump over the bot between two samples
				cx, cy := closestOnSegment(x, y, pos[i][0], pos[i][1], nx, ny)
				score = math.Min(score, math.Hypot(x-cx, y-cy)-h.r-c.pr)
				pos[i] = [2]float64{nx, ny}
			}
		}
		if k < 0 {
			score -= 1 // standing still only if nothing else is any better
		}
		if score > bestScore {
			bestScore, bx, by = score, ux, uy
		}
	}
	return bx, by
}

// ballSpeed is how fast a projectile of this kind flies.
func (c Config) ballSpeed(kind int) float64 {
	switch kind {
	case ballFire:
		return c.Charizard.FireBreath.Speed
	case ballLeaf:
		return c.Chikorita.RazorLeaf.Speed
	case ballArrow:
		return c.Rowlet.Arrows.Speed
	}
	return c.Pikachu.ElectroBall.Speed
}

// lead is the unit direction from the bot to where the opponent will be in t seconds if it keeps going as it was.
func (c *bctx) lead(t float64) (float64, float64) {
	x := math.Min(math.Max(c.f.X+c.vx*t, 0), c.g.court.Length)
	y := math.Min(math.Max(c.f.Y+c.vy*t, 0), c.g.court.Width)
	if ux, uy := unit(x-c.p.X, y-c.p.Y); ux != 0 || uy != 0 {
		return ux, uy
	}
	return c.ux, c.uy
}

// rootedFor is how many more seconds the opponent is known to be unable to walk away.
func (c *bctx) rootedFor() float64 {
	f := c.f
	t := math.Max(math.Max(f.ParalyzeT, f.FrozenT), f.StunT)
	if f.Kind == snorlax || f.Kind == gyarados {
		t = math.Max(t, f.BeamT+f.SweepT+f.RechargeT)
	}
	if f.Kind == pikachu { // charging a ball or casting Thunder roots it too
		t = math.Max(t, f.BallT)
		if f.ThunderLeft > 0 {
			t = math.Max(t, f.ThunderIn+float64(f.ThunderLeft-1)*c.g.cfg.Pikachu.Thunder.Interval)
		}
	}
	if f.Held {
		t = math.Max(t, 1)
	}
	return t
}

// ---- the stick ----

var (
	stickX = [8]int{1, 1, 0, -1, -1, -1, 0, 1}
	stickY = [8]int{0, 1, 1, 1, 0, -1, -1, -1}
)

// dir8 is the 8-way stick position closest to the direction (vx, vy); (0, 0) if there is none.
func dir8(vx, vy float64) (int, int) {
	if math.Hypot(vx, vy) < 1e-9 {
		return 0, 0
	}
	k := (int(math.Round(math.Atan2(vy, vx)/(math.Pi/4))) + 8) % 8
	return stickX[k], stickY[k]
}

// inward pushes away from the walls, harder the closer it is to one.
func (c *bctx) inward() (float64, float64) {
	const m = 100.0
	push := func(v, hi float64) float64 {
		switch {
		case v < m:
			return 1 - v/m
		case v > hi-m:
			return -(1 - (hi-v)/m)
		}
		return 0
	}
	return push(c.p.X, c.g.court.Length), push(c.p.Y, c.g.court.Width)
}

// move is an Input that walks along (vx, vy), bent away from the walls, and aims at the opponent.
func (c *bctx) move(vx, vy float64) Input {
	ix, iy := c.inward()
	in := Input{AimX: c.ux, AimY: c.uy}
	in.DX, in.DY = dir8(vx+ix*1.5, vy+iy*1.5)
	return in
}

func (c *bctx) stay() Input { return Input{AimX: c.ux, AimY: c.uy} }

// ---- Pikachu ----

// Pikachu keeps out of the opponent's reach and throws Electro Balls. A Volt Tackle is its way out of danger,
// and the two uses it has left then go at the opponent; a paralyzed (or otherwise rooted) opponent gets Thunder.
func (b *Bot) pikachu(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	vt, bc := g.cfg.Pikachu.VoltTackle, g.cfg.Pikachu.ElectroBall
	if p.ThunderLeft > 0 || p.BallT > 0 || p.FrozenT > 0 || p.Held {
		return c.stay() // rooted: nothing to decide
	}
	th := c.th
	volley := f.Kind == rowlet && f.ArrowT > 0 // the arrows go where it stands when they leave: do not be rooted
	canDash := p.DashT == 0 && (p.VoltLeft > 0 || p.DashCD == 0) && p.ParalyzeT == 0
	// aligned reports whether a dash along the best 8-way direction would touch the opponent.
	aligned := func() (int, int, bool) {
		dx, dy := dir8(c.ux, c.uy)
		if dx == 0 && dy == 0 || !vt.Reach.hits(f) {
			return 0, 0, false
		}
		ux, uy := unit(float64(dx), float64(dy))
		hit := segDist(f.X, f.Y, p.X, p.Y, p.X+ux*vt.Distance, p.Y+uy*vt.Distance) <= vt.Radius+c.fr-3
		return dx, dy, hit
	}

	// An opponent that cannot walk away is the time for Thunder, unless something is coming at the bot: the cast
	// roots it for seconds.
	if c.rootedFor() >= 0.6 && th.kind == thNone && !volley && p.ThunderCD == 0 && g.cfg.Pikachu.Thunder.Reach.hits(f) {
		in := c.stay()
		in.Thunder = true
		return in
	}

	if th.kind != thNone && th.eta <= botHorizon {
		if canDash && th.eta <= 0.5 && th.kind != thShadow && th.kind != thChase {
			in := c.stay()
			in.Skill = true
			in.DX, in.DY = dir8(th.ax, th.ay)
			return in
		}
		return c.move(th.ax, th.ay)
	}

	// The rest of a chain after a dodge, or a chain begun on an opponent that came too close.
	if p.VoltLeft > 0 && p.DashT == 0 && p.ParalyzeT == 0 {
		if dx, dy, ok := aligned(); ok {
			in := c.stay()
			in.Skill, in.DX, in.DY = true, dx, dy
			return in
		}
		if c.dist < vt.Distance+vt.Radius+c.fr+150 {
			return c.move(c.ux, c.uy) // close in until a dash can reach
		}
	} else if canDash && p.VoltLeft == 0 && b.rng.Float64() < 0.5 {
		if dx, dy, ok := aligned(); ok {
			in := c.stay()
			in.Skill, in.DX, in.DY = true, dx, dy
			return in
		}
	}

	// Stand where the opponent cannot reach and circle: outside its longest attack, if it has a long one. The
	// ball roots Pikachu while it charges, so it is only thrown from out there (or at someone who cannot move).
	lo := 260.0
	if f.Kind == snorlax {
		lo = g.cfg.Snorlax.IcePunch.Distance + g.cfg.Snorlax.IcePunch.Radius + c.fr + 30
	}
	if p.BallCD == 0 && !volley && bc.Reach.hits(f) && (c.dist >= lo || c.rootedFor() > 0.6 && c.dist >= 100) {
		in := c.stay()
		in.Ball = true
		return in
	}
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	switch {
	case c.dist < lo:
		return c.move(-c.ux+tx*0.6, -c.uy+ty*0.6)
	case c.dist > lo+150:
		return c.move(c.ux+tx*0.4, c.uy+ty*0.4)
	}
	return c.move(tx, ty)
}

// ---- Snorlax ----

// Snorlax walks slowly, so its Jump is what it answers danger with and what it attacks with: it goes over what
// runs along the ground and comes down on the opponent, then follows the landing with an Ice Punch. A frozen
// opponent gets the Hyper Beam at point-blank range, where it is hit the most times.
func (b *Bot) snorlax(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	jp, ip, hb := g.cfg.Snorlax.Jump, g.cfg.Snorlax.IcePunch, g.cfg.Snorlax.HyperBeam
	busy := p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0 || p.PunchT > 0 || p.PunchLeft > 0 || p.PunchRecover > 0
	air := p.Z > 0 || p.VZ > 0
	if b.wasAir && !air {
		b.comboT = 1.0
	}
	b.wasAir = air
	b.comboT = math.Max(0, b.comboT-botEvery*dtBot)

	if p.FrozenT > 0 || p.Held || p.ParalyzeT > 0 || busy { // paralyzed it can neither walk, jump nor punch, and a beam would only root it
		return c.stay()
	}
	if air {
		return c.move(c.ux, c.uy) // steer the landing onto the opponent
	}
	canJump := p.JumpCD == 0
	th := c.th
	toward := func() Input { return c.move(c.ux, c.uy) }

	if th.kind != thNone && th.eta <= botHorizon {
		jump := false
		switch th.kind {
		case thBall: // it has to still be coming when Snorlax is at the top, and not be there yet
			jump = canJump && th.eta >= 0.25 && th.eta <= 0.9
		case thPunch:
			jump = canJump && th.eta <= 0.5
		case thRush:
			jump = canJump
		}
		switch {
		case jump:
			in := toward()
			in.Skill = true
			return in
		case th.kind == thBall && canJump && th.eta > 0.9: // the moment to jump is coming: do not start anything slow
		case th.kind != thRush:
			return c.move(th.ax, th.ay)
		}
	}
	// Anything slow (a punch, a beam) is held back while projectiles are on their way or a volley is being aimed.
	danger := th.kind == thBall || f.Kind == rowlet && f.ArrowT > 0

	punchReady := p.PunchCD == 0 && ip.Reach.hits(f)
	punchRange := ip.Distance + ip.Radius + c.fr - 40

	// An opponent the punch froze (or that cannot walk for a good while) gets the beam whatever the distance: the
	// nearer it is the longer it stays in the beam, so Snorlax walks up to it if it can be there before the
	// opponent is free, and beams from where it stands if it cannot.
	rooted := c.rootedFor()
	if (f.FrozenT > 0 || rooted >= 0.8) && p.BeamCD == 0 && hb.Reach.hits(f) && !danger {
		walk := (c.dist - 150) / g.cfg.walkSpeedOf(snorlax)
		switch {
		case c.dist <= 150, c.dist <= hb.Length-50 && walk > rooted-0.3:
			in := c.stay()
			in.Beam = true
			return in
		}
		return toward()
	}

	if punchReady && !danger && c.dist <= punchRange && (b.comboT > 0 || c.rootedFor() > 0 || b.rng.Float64() < 0.5) {
		in := c.stay()
		in.AimX, in.AimY = c.lead(ip.ChargeDuration + c.dist/ip.Speed)
		in.Punch = true
		return in
	}

	if canJump && !danger && c.dist >= 60 && c.dist <= 330 && f.Z <= jp.Reach.MaxHeight && b.rng.Float64() < 0.25 {
		in := toward()
		in.Skill = true
		return in
	}

	if c.dist > 120 {
		return toward()
	}
	return c.stay()
}
