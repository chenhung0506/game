package main

import "math"

// What a tornado is doing to a player (Player.Twirl).
const (
	twirlNone = iota
	twirlSpin // carried up and spun inside the tornado
	twirlFall // let go: coming down
)

// twirlFallTime is the seconds a body let go by a tornado takes to come down from the full lift height.
const twirlFallTime = 0.4

// rotate turns the vector (x, y) by a radians.
func rotate(x, y, a float64) (float64, float64) {
	return x*math.Cos(a) - y*math.Sin(a), x*math.Sin(a) + y*math.Cos(a)
}

// stepRowlet runs Rowlet's three skills and reports whether it is rooted this tick (the drift moves it itself, so
// the walking code stays out of its way). Rowlet is always in the air: it sits at fly_high, so only the skills
// whose min_height..max_height take in that height can hurt it; if a tornado or a toss brings it down it climbs
// back at the pace Charizard takes off.
//
// Crescent Drift: it runs `speed` for `duration` seconds along a circle of `radius`, starting along the direction
// held (else the way it faces) and curving to the side the opponent is on, a half-moon that ends in a skid. How
// much of the circle it covers is speed x duration / radius.
//
// Arrows: after `charge_duration` of aiming (it can walk and drift meanwhile) `count` arrows fan out, `angle`
// degrees apart and centred on where the opponent stands then. An arrow that touches the opponent hurts and slows it.
//
// Tornado: it charges, rooted; the tornado appears in front of it, then it recovers, rooted. The tornado is a
// projectile that chases the opponent (see stepBalls) and carries it up (see stepTwirl).
func (g *Game) stepRowlet(i int, in Input, dt float64) ([]Event, bool) {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	rw := g.cfg.Rowlet
	dr, ar, tn := rw.CrescentDrift, rw.Arrows, rw.Tornado
	for _, cd := range []*float64{&p.DriftCD, &p.ArrowCD, &p.TwistCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}
	if !p.Held { // a tornado or a toss has it while Held
		p.Z = math.Min(p.Z+rw.FlyHigh/flyRiseTime*dt, rw.FlyHigh)
	}
	twisting := p.TwistT > 0 || p.TwistRecT > 0
	release := func() { // the tornado appears in front of Rowlet, along its aim
		off := g.radius(i) + tn.Radius
		g.Balls = append(g.Balls, Ball{
			K: ballTornado, Owner: i, HP: tn.HP, Left: tn.Distance, Life: tn.Duration,
			X: math.Min(math.Max(p.X+p.AimX*off, 0), g.court.Length),
			Y: math.Min(math.Max(p.Y+p.AimY*off, 0), g.court.Width),
		})
		p.TwistRecT = tn.RechargeDuration
	}

	// ---- Crescent Drift ----
	if in.Skill && p.DriftT == 0 && p.DriftCD == 0 && !twisting {
		dx, dy := float64(in.DX), float64(in.DY)
		if dx == 0 && dy == 0 {
			dx, dy = float64(p.FX), float64(p.FY)
		}
		n := math.Hypot(dx, dy)
		p.DriftX, p.DriftY, p.DriftS = dx/n, dy/n, 1
		if p.DriftX*(other.Y-p.Y)-p.DriftY*(other.X-p.X) < 0 {
			p.DriftS = -1 // the opponent is on the other side: curve that way
		}
		p.DriftT, p.DriftCD = dr.Duration, dr.Cooldown
	}

	// ---- Tornado: charge, then let it go ----
	if in.Twister && !twisting && p.DriftT == 0 && p.TwistCD == 0 {
		p.TwistT, p.TwistCD = tn.ChargeDuration, tn.Cooldown
		if p.TwistT == 0 {
			release()
		}
	}

	// ---- Arrows: aim for charge_duration (free to walk and drift), then the volley leaves ----
	fire := func() {
		dx, dy := other.X-p.X, other.Y-p.Y
		if math.Hypot(dx, dy) < 1e-9 { // standing on the opponent: no direction, so use the facing
			dx, dy = float64(p.FX), float64(p.FY)
		}
		n := math.Hypot(dx, dy)
		off := g.radius(i) + ar.Radius + 1 // each starts just clear of the body
		for k := 0; k < ar.Count; k++ {
			a := (float64(k) - float64(ar.Count-1)/2) * ar.Angle * math.Pi / 180
			ux, uy := rotate(dx/n, dy/n, a)
			g.Balls = append(g.Balls, Ball{
				K: ballArrow, Owner: i, HP: ar.HP, DX: ux, DY: uy,
				X: math.Min(math.Max(p.X+ux*off, 0), g.court.Length),
				Y: math.Min(math.Max(p.Y+uy*off, 0), g.court.Width),
			})
		}
	}
	if in.Arrow && p.ArrowCD == 0 && p.ArrowT == 0 && !twisting {
		p.ArrowCD, p.ArrowT = ar.Cooldown, ar.ChargeDuration
		if p.ArrowT == 0 {
			fire()
		}
	}

	rooted := p.DriftT > 0 || p.TwistT > 0 || p.TwistRecT > 0 // through the tick a skill ends in, free from the next

	if p.DriftT > 0 {
		step := math.Min(dt, p.DriftT) // the last step is clipped so the arc is exact
		turn := dr.Speed / dr.Radius * step * p.DriftS
		mx, my := rotate(p.DriftX, p.DriftY, turn/2) // the chord of an arc runs along the heading halfway through it
		chord := 2 * dr.Radius * math.Sin(math.Abs(turn)/2)
		p.X = math.Min(math.Max(p.X+mx*chord, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+my*chord, 0), g.court.Width)
		p.DriftX, p.DriftY = rotate(p.DriftX, p.DriftY, turn)
		if p.DriftT -= step; p.DriftT < 1e-9 {
			p.DriftT = 0
		}
	}
	if p.ArrowT > 0 {
		if p.ArrowT -= dt; p.ArrowT < 1e-9 {
			p.ArrowT = 0
			fire()
		}
	}
	if p.TwistT > 0 {
		if p.TwistT -= dt; p.TwistT < 1e-9 {
			p.TwistT = 0
			release()
		}
	} else if p.TwistRecT > 0 {
		if p.TwistRecT -= dt; p.TwistRecT < 1e-9 {
			p.TwistRecT = 0
		}
	}
	return events, rooted
}

// caughtBy is the tornado of slot owner that holds the other player, if any.
func (g *Game) caughtBy(owner int) *Ball {
	for j := range g.Balls {
		if b := &g.Balls[j]; b.K == ballTornado && b.Caught && b.Owner == owner && b.HP > 0 {
			return b
		}
	}
	return nil
}

// stepTwirl moves the players a tornado holds. While the tornado stands, the body is pinned in it, climbs to
// lift_height by the middle of the spin and takes spin_damage every spin_interval. When the tornado is gone (the
// spin is over, or it was broken) the body comes down in twirlFallTime, and on landing takes landing_damage and
// shakes the ground like Snorlax's landing. It runs after the projectiles, so the heights it sets are the last word.
func (g *Game) stepTwirl(dt float64) []Event {
	var events []Event
	tn := g.cfg.Rowlet.Tornado
	for v := range g.P {
		p := &g.P[v]
		if p.Twirl == twirlSpin {
			if b := g.caughtBy(1 - v); b != nil {
				p.X, p.Y = b.X, b.Y
				p.Z = tn.LiftHeight * math.Min(1, (tn.SpinDuration-b.Life)/(tn.SpinDuration/2))
				for p.TwirlIn -= dt; p.TwirlIn < 1e-9; p.TwirlIn += tn.SpinInterval { // a loop so a tiny interval cannot lose ticks
					g.hit(p, tn.SpinDamage)
				}
				continue
			}
			p.Twirl = twirlFall
		}
		if p.Twirl == twirlFall {
			if p.Z -= tn.LiftHeight / twirlFallTime * dt; p.Z <= 1e-9 {
				p.Z, p.Twirl, p.Held = 0, twirlNone, false
				g.hit(p, tn.LandingDamage)
				events = append(events, Event{T: "slam", X: p.X, Y: p.Y, R: tn.Radius})
			}
		}
	}
	return events
}

// grindPillars lets each tornado wear down the other player's ice pillars it touches: spin_damage the moment it
// touches one, then again every spin_interval while it stays. The tornado itself is not hurt by a pillar.
func (g *Game) grindPillars() {
	tn := g.cfg.Rowlet.Tornado
	for i := range g.Balls {
		t := &g.Balls[i]
		if t.K != ballTornado || t.HP <= 0 {
			continue
		}
		for j := range g.Balls {
			if p := &g.Balls[j]; p.K == ballPillar && p.Owner != t.Owner && p.HP > 0 && p.GrindCD == 0 &&
				math.Hypot(p.X-t.X, p.Y-t.Y) <= tn.Radius+g.ballRadius(p) {
				p.HP -= tn.SpinDamage
				p.GrindCD = tn.SpinInterval
			}
		}
	}
}
