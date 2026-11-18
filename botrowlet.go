package main

import "math"

// Rowlet's computer. It plays one combo, over and over, and walks about out of reach between the rounds of it:
//
//  1. Arrows, with a Tailwind run into the opponent's face. The volley is aimed at where the opponent stands as it leaves and fans out by
//     `angle` degrees, so it only lands all at once from right next to it. The arrows take `charge_duration` to be ready and Rowlet can run
//     meanwhile, so the run is started so that it ends just before the volley leaves, with a little time left to step up to the face.
//     The run is one of the eight stick directions, curved by the config, so the bot first walks to where one of them ends on the opponent.
//  2. A slowed opponent (an arrow hit it) cannot walk out of a tornado's way: it casts the Tornado at once, in its face.
//  3. While the opponent spins in the tornado it draws the next volley so that the arrows leave just as the opponent comes down into their
//     height band, and stands in its face for that.
const (
	rowletStartTol = 170  // pixels the run may be off the opponent when the arrows are drawn: the rest is walked off
	rowletSkipTol  = 280  // the run is not made if it would end this far from the opponent
	rowletWalkBack = 0.55 // seconds left of the arrows when the run ends: the walk into the face
)

// driftEnd is where a Tailwind run along the stick direction k ends when the opponent stands at (tx, ty): the heading curves towards it
// as the game does (see stepRowlet), so the end of the run is the chord of the arc.
func (c *bctx) driftEnd(k int, tx, ty float64) (ex, ey float64) {
	g, p := c.g, c.p
	dr := g.cfg.Rowlet.Tailwind
	hx, hy := unit(float64(stickX[k]), float64(stickY[k]))
	side := 1.0
	if hx*(ty-p.Y)-hy*(tx-p.X) < 0 {
		side = -1
	}
	turn := dr.Speed / dr.Radius * dr.Duration
	chord := 2 * dr.Radius * math.Sin(math.Abs(turn)/2)
	mx, my := rotate(hx, hy, side*turn/2)
	return math.Min(math.Max(p.X+mx*chord, 0), g.court.Length), math.Min(math.Max(p.Y+my*chord, 0), g.court.Width)
}

// bestDrift is the stick direction whose run ends nearest to (tx, ty), and how far from it.
func (c *bctx) bestDrift(tx, ty float64) (k int, ex, ey, err float64) {
	err = math.Inf(1)
	for n := range stickX {
		x, y := c.driftEnd(n, tx, ty)
		if d := math.Hypot(x-tx, y-ty); d < err {
			k, ex, ey, err = n, x, y, d
		}
	}
	return
}

// foeAt is where the opponent will be in t seconds if it goes on as it was, kept in the court.
func (c *bctx) foeAt(t float64) (float64, float64) {
	return math.Min(math.Max(c.f.X+c.vx*t, 0), c.g.court.Length), math.Min(math.Max(c.f.Y+c.vy*t, 0), c.g.court.Width)
}

// spinLeft is how many more seconds the opponent is held by one of its own tornadoes.
func (c *bctx) spinLeft() (float64, bool) {
	if c.f.Twirl != twirlSpin {
		return 0, false
	}
	for i := range c.balls {
		if bl := &c.balls[i]; bl.K == ballTornado && bl.Owner == c.me && bl.Caught {
			return math.Max(bl.Life-botReact*dtBot, 0), true // what it was botReact ticks ago, less what has passed since
		}
	}
	return 0, false
}

func (b *Bot) rowlet(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	rw := g.cfg.Rowlet
	dr, ar, tn := rw.Tailwind, rw.Arrows, rw.Tornado
	if p.FrozenT > 0 || p.Held || p.StunT > 0 || p.DriftT > 0 || p.TwistT > 0 || p.TwistRecT > 0 {
		return c.stay() // rooted, or the run moves it itself
	}
	face := c.pr + c.fr + 12 // close enough for the whole volley to land
	toFace := func() Input {
		if c.dist > face {
			return c.move(c.ux, c.uy)
		}
		return c.stay()
	}

	// What threatens it is walked out of, except while the arrows are being drawn: they leave all the same, and the plan needs the walk.
	if th := c.th; th.kind != thNone && th.eta <= botHorizon && (p.ArrowT == 0 || th.eta <= 0.25) {
		return c.move(th.ax, th.ay)
	}

	// 3. The opponent is in the tornado (or still coming down out of it).
	if f.Twirl != twirlNone {
		if left, ok := c.spinLeft(); ok && p.ArrowT == 0 && p.ArrowCD == 0 {
			fall := twirlFallTime*(1-math.Min(1, ar.MaxHeight/math.Max(tn.LiftHeight, 1e-9))) + 0.02 // until it is low enough for the arrows
			if left+fall <= ar.ChargeDuration {
				in := toFace()
				in.Arrow = true
				return in
			}
		} else if f.Twirl == twirlFall && ar.ChargeDuration <= 1e-9 && p.ArrowT == 0 && p.ArrowCD == 0 && f.Z <= ar.MaxHeight {
			in := toFace()
			in.Arrow = true
			return in
		}
		return toFace()
	}

	// 2. A slowed opponent cannot get away from the tornado.
	if p.ArrowT == 0 && p.TwistCD == 0 && f.SlowT >= tn.ChargeDuration+0.3 && c.dist <= tn.Distance*0.6 && tn.Reach.hits(f) {
		in := c.stay()
		in.Twister = true
		return in
	}

	// 1. The volley. It is drawn when a run can end on the opponent (or nearly), and the run starts as the end of the draw comes near.
	if p.ArrowT > 0 {
		tx, ty := c.foeAt(p.ArrowT + botReact*dtBot)
		if p.DriftCD == 0 {
			k, ex, ey, err := c.bestDrift(tx, ty)
			if p.ArrowT <= dr.Duration+rowletWalkBack && err <= rowletSkipTol {
				in := c.stay()
				in.Skill, in.DX, in.DY = true, stickX[k], stickY[k]
				return in
			}
			if dx, dy := tx-ex, ty-ey; p.ArrowT > dr.Duration+rowletWalkBack && math.Hypot(dx, dy) > 25 {
				return c.move(dx, dy) // moving the start moves the end of the run the same way
			}
			if p.ArrowT > dr.Duration+rowletWalkBack {
				return c.stay()
			}
		}
		return toFace()
	}
	if p.ArrowCD == 0 && p.DriftCD == 0 && f.SlowT == 0 {
		tx, ty := c.foeAt(ar.ChargeDuration * 0.5)
		_, ex, ey, err := c.bestDrift(tx, ty)
		if err <= rowletStartTol {
			in := c.stay()
			in.Arrow = true
			return in
		}
		return c.move(tx-ex, ty-ey)
	}

	// Between the rounds: out of the opponent's reach, circling.
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	switch {
	case c.dist < 280:
		return c.move(-c.ux+tx*0.6, -c.uy+ty*0.6)
	case c.dist > 520:
		return c.move(c.ux+tx*0.3, c.uy+ty*0.3)
	}
	return c.move(tx, ty)
}
