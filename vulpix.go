package main

import "math"

const (
	// pillarMass keeps a pillar where it is when a beam or a punch pushes it by "its mass minus the ball's".
	pillarMass = 1e9
	// pillarBlock is what a pillar takes off a projectile that runs into it: more than any projectile has, so it is spent.
	pillarBlock = 1 << 20
	// maxPillars bounds the pillars alive at once, whatever the config says.
	maxPillars = 200
)

// stepVulpix runs Vulpix's three skills. They do not lock each other: the frost can be laid while a storm runs or
// the beam is held; only the beam roots it.
//
// Frost Field: Vulpix rides an iceberg for spray_duration seconds, sliding fast and leaving a trail of frost; the iceberg
// bursts when it touches the opponent or when the ride is over.
//
// Ice Storm: the storm starts on Vulpix and chases the opponent, striking every `interval` seconds like Thunder. Each
// strike also takes `damage` off every projectile of the other player inside its area.
//
// Freeze Beam: the end of the beam starts `start_distance` from the body along the aim; the stick then moves it over the ground at `speed`, at most `length` from the body; the
// beam hurts nobody, and an ice pillar rises wherever its end passes (not on top of another of Vulpix's).
func (g *Game) stepVulpix(i int, in Input, dt float64) []Event {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	fs, is, fb := g.cfg.Vulpix.FrostField, g.cfg.Vulpix.IceStorm, g.cfg.Vulpix.FreezeBeam
	for _, cd := range []*float64{&p.FrostCD, &p.StormCD, &p.FBeamCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}

	// ---- Frost Field: ride the iceberg ----
	p.Rode = false
	if in.Skill && p.FrostT == 0 && p.FrostCD == 0 && p.FBeamT == 0 {
		p.FrostT, p.FrostCD = fs.SprayDuration, fs.Cooldown
		p.FrostX, p.FrostY = p.X, p.Y
		g.layFrost(i, p.X, p.Y)
	}
	if p.FrostT > 0 {
		if p.BoundT > 0 || p.ParalyzeT > 0 { // no movement skill while bound or paralyzed: the iceberg is gone, without a burst
			p.FrostT = 0
		} else {
			dx, dy := float64(in.DX), float64(in.DY)
			if dx == 0 && dy == 0 {
				dx, dy = float64(p.FX), float64(p.FY) // it cannot stop: with no stick it keeps the way it faces
			}
			ux, uy := unit(dx, dy)
			p.Rode = true
			step := math.Min(dt, p.FrostT) // the last step is clipped so the ride covers exactly speed x duration
			x0, y0 := p.X, p.Y
			p.X = math.Min(math.Max(x0+ux*fs.SlideSpeed*step, 0), g.court.Length)
			p.Y = math.Min(math.Max(y0+uy*fs.SlideSpeed*step, 0), g.court.Width)
			for d := math.Hypot(p.X-p.FrostX, p.Y-p.FrostY); d >= fs.Radius; d = math.Hypot(p.X-p.FrostX, p.Y-p.FrostY) {
				p.FrostX += (p.X - p.FrostX) / d * fs.Radius // a bit of frost every `radius` along the way, so the trail has no gap
				p.FrostY += (p.Y - p.FrostY) / d * fs.Radius
				g.layFrost(i, p.FrostX, p.FrostY)
			}
			touched := fs.Reach.hits(other) && segDist(other.X, other.Y, x0, y0, p.X, p.Y) <= fs.Radius+g.radius(1-i)
			if touched {
				g.hit(other, fs.ContactDamage)
			}
			if p.FrostT -= step; touched || p.FrostT < 1e-9 {
				p.FrostT = 0
				events = append(events, Event{T: "iceburst", X: p.X, Y: p.Y, R: fs.ExplosionRadius})
				if fs.ExplosionRadius > 0 { // the blast leaves a patch of frost as wide as itself
					g.layFrostOf(i, p.X, p.Y, fs.ExplosionRadius)
				}
				if fs.Reach.hits(other) && math.Hypot(other.X-p.X, other.Y-p.Y) <= fs.ExplosionRadius+g.radius(1-i) {
					g.hit(other, fs.ExplosionDamage)
				}
			}
		}
	}

	// ---- Ice Storm ----
	if in.Storm && p.StormLeft == 0 && p.StormCD == 0 {
		p.StormLeft, p.StormIn, p.StormHits, p.StormCD = is.Strikes, is.Interval, 0, is.Cooldown
		p.StormX, p.StormY = p.X, p.Y
	}
	if p.StormLeft > 0 {
		dx, dy := other.X-p.StormX, other.Y-p.StormY
		if d := math.Hypot(dx, dy); d > 0 { // chase, without overshooting
			step := math.Min(d, is.FollowSpeed*dt)
			p.StormX += dx / d * step
			p.StormY += dy / d * step
		}
		p.StormIn -= dt
		for p.StormLeft > 0 && p.StormIn < 1e-9 { // a loop so a tiny interval cannot lose strikes
			events = append(events, Event{T: "storm", X: p.StormX, Y: p.StormY, R: is.Radius})
			for j := range g.Balls { // the storm wears down the other player's projectiles (and pillars) caught in it, whether or not it hits a body
				if b := &g.Balls[j]; b.Owner != i && math.Hypot(b.X-p.StormX, b.Y-p.StormY) <= is.Radius+g.ballRadius(b) {
					b.HP -= is.Damage
				}
			}
			if is.Reach.hits(other) && math.Hypot(other.X-p.StormX, other.Y-p.StormY) <= is.Radius+g.radius(1-i) {
				g.hit(other, is.Damage)
				if p.StormHits++; p.StormHits%is.FreezeHits == 0 {
					other.FrozenT = math.Max(other.FrozenT, is.FreezeDuration)
					events = append(events, Event{T: "freeze", X: other.X, Y: other.Y})
				}
			}
			p.StormLeft--
			p.StormIn += is.Interval
		}
	}

	// ---- Freeze Beam ----
	if in.Freeze && p.FBeamT == 0 && p.FBeamCD == 0 && p.FrostT == 0 { // not while riding: the beam roots it
		p.FBeamT, p.FBeamCD = fb.ControlDuration, fb.Cooldown
		p.FBeamX, p.FBeamY = g.clampBeamEnd(p, p.X+p.AimX*fb.StartDistance, p.Y+p.AimY*fb.StartDistance)
		g.raisePillars(i, p.FBeamX, p.FBeamY, p.FBeamX, p.FBeamY)
	} else if p.FBeamT > 0 {
		x0, y0 := p.FBeamX, p.FBeamY
		if in.DX != 0 || in.DY != 0 { // the stick's exact direction, else the 8-way keys: the same as the aim
			p.FBeamX, p.FBeamY = g.clampBeamEnd(p, x0+p.AimX*fb.Speed*dt, y0+p.AimY*fb.Speed*dt)
		}
		g.raisePillars(i, x0, y0, p.FBeamX, p.FBeamY)
		if p.FBeamT -= dt; p.FBeamT < 1e-9 {
			p.FBeamT = 0
		}
	}
	return events
}

// clampBeamEnd keeps the end of p's beam inside the court and within the beam's length of the body.
func (g *Game) clampBeamEnd(p *Player, x, y float64) (float64, float64) {
	x, y = math.Min(math.Max(x, 0), g.court.Length), math.Min(math.Max(y, 0), g.court.Width)
	if d, l := math.Hypot(x-p.X, y-p.Y), g.cfg.Vulpix.FreezeBeam.Length; d > l {
		x, y = p.X+(x-p.X)/d*l, p.Y+(y-p.Y)/d*l
	}
	return x, y
}

// raisePillars raises player i's pillars along the way the beam's end went from (x0, y0) to (x1, y1) in one step.
// The way is sampled finer than a pillar is wide, so a fast end leaves no gap (neighbours end up 1.5 to 1.9 radii apart, less than a diameter); a spot within 1.5 radii of one of
// i's living pillars gets none, so sweeping back and forth does not pile them up.
func (g *Game) raisePillars(i int, x0, y0, x1, y1 float64) {
	fb := g.cfg.Vulpix.FreezeBeam
	gap := fb.PillarRadius * 1.5
	n := int(math.Ceil(math.Hypot(x1-x0, y1-y0) / (gap / 4)))
	for k := 0; k <= n; k++ {
		x, y := x1, y1
		if n > 0 {
			x, y = x0+(x1-x0)*float64(k)/float64(n), y0+(y1-y0)*float64(k)/float64(n)
		}
		free, total := true, 0
		for j := range g.Balls {
			if b := &g.Balls[j]; b.K == ballPillar {
				total++
				free = free && !(b.Owner == i && b.HP > 0 && math.Hypot(b.X-x, b.Y-y) < gap)
			}
		}
		if free && total < maxPillars {
			g.Balls = append(g.Balls, Ball{K: ballPillar, Owner: i, X: x, Y: y, HP: fb.PillarHP, Life: fb.PillarDuration})
		}
	}
}

// frostFactor is how much faster or slower than usual slot i walks where it stands: on its own frost by `boost`,
// on the other's by `slow`, on both (patches overlap) both at once. It only counts at the heights of the frost
// field's band.
func (g *Game) frostFactor(i int) float64 {
	fs := g.cfg.Vulpix.FrostField
	p := &g.P[i]
	if len(g.Frost) == 0 || !fs.Reach.hits(p) {
		return 1
	}
	own, foe := false, false
	for _, f := range g.Frost {
		w := fs.Radius
		if f.R > 0 {
			w = f.R
		}
		if math.Hypot(p.X-f.X, p.Y-f.Y) <= w {
			own, foe = own || f.Owner == i, foe || f.Owner != i
		}
	}
	v := 1.0
	if own {
		v *= 1 + fs.Boost
	}
	if foe {
		v *= 1 - fs.Slow
	}
	return v
}

// maxFrost bounds the bits of frost on the court, whatever the config says (the oldest are dropped first).
const maxFrost = 1500

// layFrost leaves a bit of slot i's trail at (x, y).
func (g *Game) layFrost(i int, x, y float64) { g.layFrostOf(i, x, y, 0) }

// layFrostOf leaves slot i's frost, r wide (0 = as wide as the trail), at (x, y).
func (g *Game) layFrostOf(i int, x, y, r float64) {
	g.Frost = append(g.Frost, Frost{Owner: i, X: x, Y: y, R: r, Left: g.cfg.Vulpix.FrostField.Duration})
	if over := len(g.Frost) - maxFrost; over > 0 {
		g.Frost = g.Frost[over:]
	}
}

// tickFrost melts the frost.
func (g *Game) tickFrost(dt float64) {
	kept := g.Frost[:0]
	for _, f := range g.Frost {
		if f.Left -= dt; f.Left > 1e-9 {
			kept = append(kept, f)
		}
	}
	g.Frost = kept
}

// blockByPillars pushes slot i out of the other player's pillars it overlaps: a pillar is a solid disc for anyone
// but its owner, at the heights of the beam's band. Two passes, so being pushed out of one into the next is
// settled; a body walled in by a ring of pillars is simply held until one melts or breaks.
func (g *Game) blockByPillars(i int) {
	p, fb := &g.P[i], g.cfg.Vulpix.FreezeBeam
	if !fb.Reach.hits(p) {
		return
	}
	for pass := 0; pass < 2; pass++ {
		for j := range g.Balls {
			b := &g.Balls[j]
			if b.K != ballPillar || b.Owner == i || b.HP <= 0 || b.Z > 0 { // one in the air (a tornado has it) blocks nothing
				continue
			}
			if gap := g.radius(i) + fb.PillarRadius; math.Hypot(p.X-b.X, p.Y-b.Y) < gap {
				p.X, p.Y = g.pushedAway(p.X, p.Y, b.X, b.Y, gap-math.Hypot(p.X-b.X, p.Y-b.Y), 1, 0)
			}
		}
	}
}
