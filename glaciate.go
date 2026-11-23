package main

import "math"

// Glaciate (Z, Vulpix): after a short gathering (ring_duration, rooted) Vulpix calls icicles down, count of them spread evenly over duration
// seconds, at random places in a circle of `radius` whose middle is on the nearest foe, as it was when the gathering ended, or, if that is
// further than `distance` from Vulpix, `distance` from Vulpix toward it. An icicle shows where it will land for fall_duration seconds; then foes within damage_radius take damage and are slowed, and it stays
// standing as a pillar (a ballPillar with its own radius, hp and scale) that the other player cannot walk through, until it is broken or melts.

// maxIcicles bounds the icicles of one cast, whatever the config says.
const maxIcicles = 100

// icicle is one on its way down; the page draws it falling over its place on the ground.
type icicle struct {
	Owner int     `json:"o"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	T     float64 `json:"t"` // seconds before it lands
}

// glaciateCast is a Glaciate that is still calling its icicles.
type glaciateCast struct {
	Owner int
	X, Y  float64 // the middle of the circle they fall in
	T     float64 // seconds since it began
	N     int     // icicles called so far
}

// stepGlaciateCast runs Z for Vulpix: with a full charge the press empties the charge and starts the gathering, and when that is over the
// icicles begin to come. It says whether Vulpix is rooted by the gathering.
func (g *Game) stepGlaciateCast(i int, in Input, u Ultimate, dt float64) ([]Event, bool) {
	p, gl := &g.P[i], g.cfg.Vulpix.Glaciate
	if p.UltT == 0 {
		if !in.Ult || p.Charge < u.Max || p.Held || p.FrozenT > 0 || p.StunT > 0 || p.FrostT > 0 || p.FBeamT > 0 { // not while sliding or casting the beam
			return nil, false
		}
		p.Charge = 0
		if p.UltT = gl.RingDuration; p.UltT == 0 {
			return g.startGlaciate(i), false
		}
	}
	if p.UltT -= dt; p.UltT < 1e-9 {
		p.UltT = 0
		return g.startGlaciate(i), true // rooted through the tick it begins, free from the next
	}
	return nil, true
}

// startGlaciate fixes the circle on the nearest foe as it is now (no further than `distance` from Vulpix) and begins calling icicles into it.
func (g *Game) startGlaciate(i int) []Event {
	p, gl := &g.P[i], g.cfg.Vulpix.Glaciate
	ux, uy, d := p.AimX, p.AimY, gl.Distance
	if foes := g.foes(i); len(foes) > 0 {
		foe := &g.P[g.nearestFoe(i)]
		if dx, dy := foe.X-p.X, foe.Y-p.Y; math.Hypot(dx, dy) > 1e-6 {
			ux, uy = dx/math.Hypot(dx, dy), dy/math.Hypot(dx, dy)
			d = math.Min(math.Hypot(dx, dy), gl.Distance) // on the foe if it is near enough, else as far toward it as `distance` allows
		}
	}
	x := math.Min(math.Max(p.X+ux*d, 0), g.court.Length)
	y := math.Min(math.Max(p.Y+uy*d, 0), g.court.Width)
	g.Casts = append(g.Casts, glaciateCast{Owner: i, X: x, Y: y})
	return []Event{{T: "glacast", X: x, Y: y, R: gl.Radius, S: i}}
}

// stepGlaciates calls the icicles of every cast that is running, and lands the ones that have fallen.
func (g *Game) stepGlaciates(dt float64) []Event {
	gl := g.cfg.Vulpix.Glaciate
	kept := g.Casts[:0]
	for _, c := range g.Casts {
		c.T += dt
		gap := gl.Duration / float64(gl.Count)
		for c.N < gl.Count && float64(c.N)*gap <= c.T+1e-9 {
			a, r := 2*math.Pi*g.rng.Float64(), gl.Radius*math.Sqrt(g.rng.Float64())
			x := math.Min(math.Max(c.X+math.Cos(a)*r, 0), g.court.Length)
			y := math.Min(math.Max(c.Y+math.Sin(a)*r, 0), g.court.Width)
			g.Icicles = append(g.Icicles, icicle{Owner: c.Owner, X: x, Y: y, T: gl.Icicle.FallDuration})
			c.N++
		}
		if c.N < gl.Count {
			kept = append(kept, c)
		}
	}
	g.Casts = kept

	var events []Event
	left := g.Icicles[:0]
	for _, ic := range g.Icicles {
		if ic.T -= dt; ic.T > 1e-9 {
			left = append(left, ic)
			continue
		}
		events = append(events, g.landIcicle(ic)...)
	}
	g.Icicles = left
	return events
}

// landIcicle is an icicle coming down: the foes within damage_radius take the damage and are slowed, and it stays where it fell.
func (g *Game) landIcicle(ic icicle) []Event {
	defer g.ultimateDamage()()
	gl := g.cfg.Vulpix.Glaciate
	it := gl.Icicle
	for _, j := range g.foes(ic.Owner) {
		if f := &g.P[j]; gl.Reach.hits(f) && math.Hypot(f.X-ic.X, f.Y-ic.Y) <= it.DamageRadius+g.radius(j) {
			g.hit(f, it.Damage)
			g.slow(f, it.SlowDuration, it.Slow)
		}
	}
	pillars := 0
	for _, b := range g.Balls {
		if b.K == ballPillar {
			pillars++
		}
	}
	if pillars < maxPillars {
		g.Balls = append(g.Balls, Ball{K: ballPillar, Owner: ic.Owner, X: ic.X, Y: ic.Y, HP: it.HP, Life: it.Duration, Rad: it.Radius, Scale: it.Scale})
	}
	return []Event{{T: "glaslam", X: ic.X, Y: ic.Y, R: it.DamageRadius, S: ic.Owner}}
}
