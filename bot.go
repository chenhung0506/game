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
// Everything about how a character plays is in its own function (pikachu, snorlax, chikorita, ash, charizard, gengar); the rest here is shared: what
// threatens the bot, where to stand, and the 8-way stick.
const (
	botEvery = 4 // ticks between decisions
	botReact = 5 // ticks the bot's view of the opponent lags behind
)

// botKinds are the characters a Bot can play so far.
var botKinds = []int{pikachu, snorlax, chikorita, ash, charizard, gengar}

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
	case chikorita:
		in = b.chikorita(c)
	case ash:
		in = b.ash(c)
	case charizard:
		in = b.charizard(c)
	case gengar:
		in = b.gengar(c)
	}
	if (in.DX != 0 || in.DY != 0) && !in.Skill && c.th.kind == thNone && c.walksIntoFlames(in) {
		in.DX, in.DY = 0, 0 // no threat where it stands, but where it is going there is: let the flames pass
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
	thFire   // standing on ground a Seismic Toss set alight
	thTree   // inside the shape (trunk and canopy) of a tree that is held up or coming down
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

	if f.Kind == gyarados && f.TailT > 0 { // a tail whip winding up: its fan is where it was warned to be
		tw := cfg.Gyarados.TailWhip
		dx, dy := p.X-f.X, p.Y-f.Y
		if d := math.Hypot(dx, dy); d <= tw.Length+c.pr && (d < 1e-9 || (dx*f.TailX+dy*f.TailY)/d >= math.Cos((tw.Angle/2+8)*math.Pi/180)) {
			ax, ay := unit(dx, dy) // out of reach is the way out; a jump goes over it (it hits at the ground)
			consider(threat{thPunch, f.TailT, ax, ay})
		}
	}

	if (f.Kind == ash || f.Kind == machamp) && (f.TreePhase == treeSmash || f.TreePhase == treeSwing) {
		// The whole shape is on the ground for everyone to see: the trunk and the canopy. Leave all of it, by the way out that is soonest.
		ts := cfg.brawlerOf(f.Kind).TreeSlam
		eta := f.TreeT
		if f.TreePhase == treeSmash {
			eta += ts.SwingDuration
		}
		hits := func(x, y float64) bool {
			return treeHits(f.X, f.Y, f.TreeX, f.TreeY, x, y, c.pr+15, ts.TrunkLength, ts.TrunkWidth, ts.LeafRadius, ts.LeafAngle)
		}
		if hits(p.X, p.Y) {
			ax, ay := c.wayOutOf(hits)
			consider(threat{thTree, math.Max(0, eta-lag), ax, ay})
		}
	}

	if f.Kind == machamp && f.KickPhase != kickNone { // a High Jump Kick: the line it flies along is fixed once the slide ends, at where the bot stands now
		hk := cfg.Machamp.HighJumpKick
		sx, sy, kx, ky, left, eta := f.X, f.Y, f.KickX, f.KickY, hk.KickDistance-f.KickD, 0.0
		if f.KickPhase == kickSlide { // not yet aimed: it will kick from where the slide ends, straight at the bot
			if hk.SlideDuration > 0 {
				sx, sy = sx+f.KickX*hk.SlideDistance*f.KickT/hk.SlideDuration, sy+f.KickY*hk.SlideDistance*f.KickT/hk.SlideDuration
			}
			kx, ky = unit(p.X-sx, p.Y-sy)
			left, eta = hk.KickDistance, f.KickT
		}
		if d := math.Hypot(p.X-sx, p.Y-sy); (kx != 0 || ky != 0) && d > 0 && eta+d/hk.KickSpeed >= 0 &&
			segDist(p.X, p.Y, sx, sy, sx+kx*left, sy+ky*left) < hk.Radius+c.pr+c.fr+15 {
			ax, ay := c.sideOf(sx, sy, kx, ky, p.X, p.Y)
			consider(threat{thPunch, eta + d/hk.KickSpeed, ax, ay})
		}
	}

	if f.Kind == ash && f.MegaT > 0 { // a Megaton Punch gathering: the line it will fly along is on the ground and follows the target (or the stick of a person), so it is read afresh every tick
		mp := cfg.Ash.MegatonPunch
		if segDist(p.X, p.Y, f.X, f.Y, f.X+f.MegaX*mp.Distance, f.Y+f.MegaY*mp.Distance) < mp.Radius+c.pr+15 {
			ax, ay := unit(p.X-f.X, p.Y-f.Y) // the fist follows its target unless a person steers it away: stepping aside is no use, only getting out of its reach is
			consider(threat{thPunch, f.MegaT, ax, ay})
		}
	}

	if fl := c.flames(); len(fl) > 0 { // Ghost Fire: the first moment one of them reaches where the bot stands
		gf := cfg.Gengar.GhostFire
	soon:
		for t := 0.0; t <= botHorizon+1e-9; t += 0.05 {
			for _, bl := range fl {
				if x, y := c.flameAt(bl, t+lag); math.Hypot(p.X-x, p.Y-y) <= gf.Radius+c.pr+25 {
					ax, ay := c.safeDir(hz)
					consider(threat{thBall, t, ax, ay})
					break soon
				}
			}
		}
	}

	for _, z := range g.Fires { // burning ground: it burns whoever stands in it every half second, so get out of it
		if z.Owner != c.me && p.Z <= cfg.Charizard.SeismicToss.FireMaxHeight && math.Hypot(p.X-z.X, p.Y-z.Y) <= z.R+c.pr+10 {
			zz := z
			ax, ay := c.wayOutOf(func(x, y float64) bool { return math.Hypot(x-zz.X, y-zz.Y) <= zz.R+c.pr+10 })
			consider(threat{thFire, 0.2, ax, ay})
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

// flames is the opponent's Ghost Fire flames that can still hurt the bot.
func (c *bctx) flames() []*Ball {
	var fl []*Ball
	for i := range c.balls {
		if bl := &c.balls[i]; bl.K == ballGhost && bl.Owner != c.me && bl.HP > 0 && !bl.Hit {
			fl = append(fl, bl)
		}
	}
	return fl
}

// flameAt is where a Ghost Fire flame will be t seconds on: straight out until it has flown its distance, then straight
// at its owner, taken to stay where it is.
func (c *bctx) flameAt(bl *Ball, t float64) (float64, float64) {
	gf := c.g.cfg.Gengar.GhostFire
	x, y := bl.X, bl.Y
	if !bl.Back {
		out := math.Min(gf.SpreadSpeed*t, bl.Left)
		x, y = x+bl.DX*out, y+bl.DY*out
		t -= out / gf.SpreadSpeed
		if t <= 0 {
			return x, y
		}
	}
	ux, uy := unit(c.f.X-x, c.f.Y-y)
	back := math.Min(gf.ConvergenceSpeed*t, math.Hypot(c.f.X-x, c.f.Y-y))
	return x + ux*back, y + uy*back
}

// walksIntoFlames is whether walking the way the stick says puts the bot in the way of a Ghost Fire flame within the next
// half second.
func (c *bctx) walksIntoFlames(in Input) bool {
	fl := c.flames()
	if len(fl) == 0 {
		return false
	}
	gf := c.g.cfg.Gengar.GhostFire
	ux, uy := unit(float64(in.DX), float64(in.DY))
	speed := c.g.cfg.walkSpeedOf(c.p.Kind)
	lag := botReact * dtBot
	for t := 0.0; t <= 0.5+1e-9; t += 0.05 {
		x := math.Min(math.Max(c.p.X+ux*speed*t, 0), c.g.court.Length)
		y := math.Min(math.Max(c.p.Y+uy*speed*t, 0), c.g.court.Width)
		for _, bl := range fl {
			if fx, fy := c.flameAt(bl, t+lag); math.Hypot(x-fx, y-fy) <= gf.Radius+c.pr+25 {
				return true
			}
		}
	}
	return false
}

// wayOutOf is the 8-way stick step that gets the bot out of an area soonest (inside(x, y) says where it is), walking at
// its own speed; none of them does it in time then it is the one that gets nearest to being out (the last it tries).
func (c *bctx) wayOutOf(inside func(x, y float64) bool) (float64, float64) {
	const step = 0.05
	speed := c.g.cfg.walkSpeedOf(c.p.Kind)
	bestT, bx, by := math.Inf(1), 0.0, 0.0
	for k := 0; k < 8; k++ {
		ux, uy := unit(float64(stickX[k]), float64(stickY[k]))
		for t := step; t <= 2+1e-9 && t < bestT; t += step {
			x := math.Min(math.Max(c.p.X+ux*speed*t, 0), c.g.court.Length)
			y := math.Min(math.Max(c.p.Y+uy*speed*t, 0), c.g.court.Width)
			if !inside(x, y) {
				bestT, bx, by = t, ux, uy
				break
			}
		}
	}
	if math.IsInf(bestT, 1) {
		return unit(-c.ux, -c.uy) // walled in: away from the opponent
	}
	return bx, by
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
		case ballPillar, ballGhost: // a ghost flame turns round, so a straight line is wrong for it: see flames
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
	fl := c.flames()
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
			for _, bl := range fl {
				x0, y0 := c.flameAt(bl, t-step+lag)
				x1, y1 := c.flameAt(bl, t+lag)
				cx, cy := closestOnSegment(x, y, x0, y0, x1, y1)
				score = math.Min(score, math.Hypot(x-cx, y-cy)-c.g.cfg.Gengar.GhostFire.Radius-c.pr)
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

// ---- Chikorita ----

// Chikorita keeps a middle distance and scatters Razor Leaves, which fly at the opponent on their own; close in, it
// hops at it with the Leaf Slash or catches it in the Vine Whip, which roots it while the vines unfurl, so that is
// kept for when nothing is coming.
func (b *Bot) chikorita(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	ls, rl, vw := g.cfg.Chikorita.LeafSlash, g.cfg.Chikorita.RazorLeaf, g.cfg.Chikorita.VineWhip
	if p.FrozenT > 0 || p.Held || p.vineBusy() || p.LeafPhase != leafNone {
		return c.stay() // rooted, or the leaf slash is moving it
	}
	th := c.th
	if th.kind != thNone && th.eta <= botHorizon {
		return c.move(th.ax, th.ay)
	}
	danger := f.Kind == rowlet && f.ArrowT > 0
	vineRange := vw.Length + c.fr - 20
	if p.VineCD == 0 && vw.Reach.hits(f) && c.dist <= vineRange && !danger && (c.rootedFor() > 0.4 || b.rng.Float64() < 0.5) {
		in := c.stay()
		in.Vine = true // a tap: straight at the opponent
		return in
	}
	if p.LeafCD == 0 && p.BoundT == 0 && ls.Reach.hits(f) && c.dist >= 60 && c.dist <= ls.JumpDistance+ls.SlashStep*2 && !danger && b.rng.Float64() < 0.3 {
		in := c.move(c.ux, c.uy)
		in.Skill = true
		return in
	}
	if p.RazorCD == 0 && rl.Reach.hits(f) && c.dist >= 180 && !danger && b.rng.Float64() < 0.8 {
		in := c.stay()
		in.Razor = true
		return in
	}
	const lo = 230.0
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	switch {
	case c.dist < 130:
		return c.move(-c.ux+tx*0.6, -c.uy+ty*0.6)
	case c.dist > lo+120:
		return c.move(c.ux+tx*0.3, c.uy+ty*0.3)
	}
	return c.move(tx, ty)
}

// ---- Ash ----

// Ash (and Machamp, who has the same three skills) is a brawler with a trap. The Rock Tomb raises a ring of stones
// round where it lands that the opponent cannot walk through and Ash can, so it jumps at the opponent to land with
// it inside the ring, and then, with the opponent shut in, it runs at it with the Comet Punch or smashes the tree
// down on it. Outside a ring it still punches and smashes, but only when it has the chance.
func (b *Bot) ash(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	bc := g.brawler(c.me)
	ts, cp, rt := bc.TreeSlam, bc.CometPunch, bc.RockTomb
	if p.FrozenT > 0 || p.Held || p.StunT > 0 || p.TreePhase != treeNone || p.CometT > 0 || p.MegaT > 0 || p.MegaJT > 0 {
		return c.stay() // rooted, or it moves itself
	}
	if p.RockT > 0 {
		return c.move(c.ux, c.uy) // steer the jump so it comes down on the opponent
	}
	th := c.th
	if th.kind != thNone && th.eta <= botHorizon {
		return c.move(th.ax, th.ay)
	}
	danger := f.Kind == rowlet && f.ArrowT > 0

	// Is the opponent shut in? The ring is the stones that are still standing: it is inside when it is nearer their middle than they are.
	var n int
	var mx, my float64
	for i := range c.balls {
		if bl := &c.balls[i]; bl.K == ballStone && bl.Owner == c.me && bl.HP > 0 {
			n++
			mx, my = mx+bl.X, my+bl.Y
		}
	}
	trapped := false
	if n >= rt.Count/2 {
		mx, my = mx/float64(n), my/float64(n)
		trapped = math.Hypot(f.X-mx, f.Y-my)+c.fr < rt.RingRadius-rt.StoneRadius*0.5
	}

	// The Megaton Punch goes where the opponent is as it starts: it is for one that cannot move out of the way, and now and then for one that may not.
	mp := bc.MegatonPunch
	mega := p.Kind == ash && p.MegaCD == 0 && mp.Reach.hits(f) && c.dist <= mp.Distance*0.8 && !danger &&
		(c.rootedFor() >= mp.ChargeDuration*0.8 || b.rng.Float64() < 0.15)
	if mega {
		return Input{Mega: true}
	}
	punch := p.CometCD == 0 && cp.Reach.hits(f) && c.dist <= cp.Distance+cp.ScatterMaxDistance+c.fr
	smash := p.TreeCD == 0 && p.Z == 0 && ts.Reach.hits(f) && c.dist <= ts.JumpDistance+ts.TrunkLength*0.8
	if trapped {
		switch {
		case punch && !danger:
			in := c.stay()
			in.Comet = true
			return in
		case smash && !danger:
			in := c.move(c.ux, c.uy)
			in.Skill = true // the jump takes it onto the opponent, and the tree comes down on it
			return in
		case c.dist > 100:
			return c.move(c.ux, c.uy) // the ring is its to walk in: close in while the skills are charging
		}
		return c.stay()
	}
	if p.RockCD == 0 && !danger && rt.Reach.hits(f) && c.dist <= rt.RingRadius*0.5 && b.rng.Float64() < 0.7 {
		in := c.move(c.ux, c.uy)
		in.Rock = true
		return in
	}
	if punch && !danger && c.dist <= cp.Distance && b.rng.Float64() < 0.35 {
		in := c.stay()
		in.Comet = true
		return in
	}
	if smash && !danger && c.dist <= ts.JumpDistance+ts.TrunkLength*0.5 && b.rng.Float64() < 0.25 {
		in := c.move(c.ux, c.uy)
		in.Skill = true
		return in
	}
	if c.dist > 110 {
		return c.move(c.ux, c.uy)
	}
	return c.stay()
}

// ---- Charizard ----

// Charizard breathes fire while it walks, which costs it nothing, so it does that from a distance; its Seismic Toss
// is the big hit and has to run into the opponent, so it closes in for it when it is ready. A flight is a fast way
// to cross the court, and ends in a ring of fire balls, so it is taken now and then, not only to run.
func (b *Bot) charizard(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	ts, fb, fly := g.cfg.Charizard.SeismicToss, g.cfg.Charizard.FireBreath, g.cfg.Charizard.Fly
	if p.FrozenT > 0 || p.Held || p.TossPhase != tossNone {
		return c.stay() // rooted, or the run (and the lift) moves it
	}
	th := c.th
	danger := f.Kind == rowlet && f.ArrowT > 0
	var in Input
	if th.kind != thNone && th.eta <= botHorizon {
		in = c.move(th.ax, th.ay)
	} else {
		tossRange := ts.Length + g.radius(1-c.me) - 30
		tx, ty := -c.uy*b.strafe, c.ux*b.strafe
		switch {
		case p.TossCD == 0 && ts.Reach.hits(f) && c.dist <= tossRange && !danger && (c.rootedFor() > 0.3 || b.rng.Float64() < 0.6):
			in = c.stay()
			in.Toss = true
			in.AimX, in.AimY = c.lead(c.dist / ts.Speed)
			return in
		case p.TossCD == 0 && ts.Reach.hits(f):
			in = c.move(c.ux+tx*0.3, c.uy+ty*0.3) // close in for the toss
		case c.dist < 200:
			in = c.move(-c.ux+tx*0.6, -c.uy+ty*0.6)
		case c.dist > 450:
			in = c.move(c.ux+tx*0.3, c.uy+ty*0.3)
		default:
			in = c.move(tx, ty)
		}
	}
	if p.FlyCD == 0 && p.FlyT == 0 && p.Z == 0 && fly.RingCount > 0 && th.kind == thNone && c.dist < fly.RingLength*0.7 && b.rng.Float64() < 0.15 {
		in.Skill = true // up, and a ring of fire where it comes down
	}
	if p.BreathCD == 0 && p.BreathT == 0 && fb.Reach.hits(f) && c.dist >= 150 && b.rng.Float64() < 0.7 {
		in.Fire = true // it walks while it breathes
	}
	return in
}

// ---- Gengar ----

// Gengar keeps a middle distance and throws Shadow Balls where the opponent is going to be, with Ghost Fire when it
// comes close. Shadow Sneak is both its way out (nothing at height 0 can touch it under the ground) and its way in: it
// slips towards the opponent so that its shadow, which keeps the opponent from starting a skill, falls on it, and comes
// up beside it to cast.
func (b *Bot) gengar(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	ss, gf, sb := g.cfg.Gengar.ShadowSneak, g.cfg.Gengar.GhostFire, g.cfg.Gengar.ShadowBall
	if p.FrozenT > 0 || p.Held || p.StunT > 0 {
		return c.stay()
	}
	if p.SneakT > 0 { // under the ground: it can only walk, and walks at the opponent to put its shadow on it
		if c.dist > ss.ShadowRadius*0.5 {
			return c.move(c.ux, c.uy)
		}
		return c.move(-c.uy*b.strafe, c.ux*b.strafe)
	}
	danger := f.Kind == rowlet && f.ArrowT > 0
	th := c.th
	if th.kind != thNone && th.eta <= botHorizon {
		in := c.move(th.ax, th.ay)
		if th.eta <= 0.5 && p.SneakCD == 0 && p.Z == 0 {
			in.Skill = true // under the ground it is out of reach
		}
		return in
	}
	reach := sb.Speed * sb.Duration
	if p.GFireCD == 0 && gf.Reach.hits(f) && c.dist <= gf.RingRadius*0.8 && !danger && b.rng.Float64() < 0.6 {
		in := c.stay()
		in.GFire = true
		return in
	}
	if p.SBallCD == 0 && sb.Reach.hits(f) && c.dist <= reach && c.dist >= sb.SlamRadius*0.5 && !danger && (c.rootedFor() > 0.3 || b.rng.Float64() < 0.5) {
		// at where the opponent will be when it lands: the cast is as long as the share of its range
		x := math.Min(math.Max(c.f.X+c.vx*sb.Duration, 0), g.court.Length)
		y := math.Min(math.Max(c.f.Y+c.vy*sb.Duration, 0), g.court.Width)
		in := c.stay()
		if dx, dy := x-p.X, y-p.Y; math.Hypot(dx, dy) > 1e-9 {
			share := math.Min(math.Hypot(dx, dy)/reach, 1)
			n := math.Hypot(dx, dy)
			in.SBall, in.CastX, in.CastY = true, dx/n*share, dy/n*share
			return in
		}
	}
	if p.SneakCD == 0 && p.Z == 0 && c.dist > 280 && !danger && b.rng.Float64() < 0.2 {
		in := c.move(c.ux, c.uy)
		in.Skill = true // in under the ground to silence it
		return in
	}
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	switch {
	case c.dist < 160:
		return c.move(-c.ux+tx*0.6, -c.uy+ty*0.6)
	case c.dist > 400:
		return c.move(c.ux+tx*0.3, c.uy+ty*0.3)
	}
	return c.move(tx, ty)
}
