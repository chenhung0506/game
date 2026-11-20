package main

import "math"

// Rowlet's ultimate, Brave Bird (Z): rooted at first for `ring_duration` (rings close in on it) and then for `charge_duration` while it
// climbs to `sky_height` and a bird of light grows out of its middle (both are counted down in Player.UltT, like Thousand Volt's
// gathering); then it dives, `dive_speed` along the straight way, at where the nearest foe stood as the dive began, and bursts where it
// lands. Foes in the burst take `damage` and are not pushed: they are thrown up (stepJuggle) and held helpless in the air. A still tornado
// (`tornado_*`) is left where it landed.

// stepBraveBird runs Z for Rowlet. It says whether Rowlet is rooted by the skill this tick.
func (g *Game) stepBraveBird(i int, in Input, u Ultimate, dt float64) ([]Event, bool) {
	p, bb := &g.P[i], g.cfg.Rowlet.BraveBird
	if p.BirdDive {
		return g.diveBird(i, dt)
	}
	if p.UltT == 0 {
		if !in.Ult || p.Charge < u.Max || p.Held || p.FrozenT > 0 || p.StunT > 0 || p.DriftT > 0 || p.TwistT > 0 || p.TwistRecT > 0 {
			return nil, false
		}
		p.Charge, p.ArrowT = 0, 0
		if p.UltT = bb.RingDuration + bb.ChargeDuration; p.UltT == 0 {
			return nil, g.startDive(i)
		}
	}
	if p.UltT -= dt; p.UltT < 1e-9 {
		p.UltT = 0
		return nil, g.startDive(i)
	}
	if bb.ChargeDuration > 0 && p.UltT < bb.ChargeDuration { // the climb takes the whole charge
		p.Z = g.cfg.Rowlet.FlyHigh + (bb.SkyHeight-g.cfg.Rowlet.FlyHigh)*(1-p.UltT/bb.ChargeDuration)
	}
	return nil, true
}

// startDive points Rowlet at where the nearest foe stands now and starts the fall. It says it is rooted (it is still the tick of the cast).
func (g *Game) startDive(i int) bool {
	p, bb := &g.P[i], g.cfg.Rowlet.BraveBird
	foe := &g.P[g.nearestFoe(i)]
	p.Z = math.Max(p.Z, bb.SkyHeight)
	p.BirdDive, p.BirdTX, p.BirdTY, p.BirdZ0 = true, foe.X, foe.Y, p.Z
	p.BirdT0 = math.Max(math.Hypot(math.Hypot(foe.X-p.X, foe.Y-p.Y), p.Z)/bb.DiveSpeed, 1e-3)
	p.BirdLeft = p.BirdT0
	return true
}

// diveBird moves Rowlet one step along its dive and bursts when it is down.
func (g *Game) diveBird(i int, dt float64) ([]Event, bool) {
	p, bb := &g.P[i], g.cfg.Rowlet.BraveBird
	step := math.Min(dt, p.BirdLeft)
	p.X += (p.BirdTX - p.X) * step / p.BirdLeft
	p.Y += (p.BirdTY - p.Y) * step / p.BirdLeft
	p.BirdLeft -= step
	p.Z = p.BirdZ0 * p.BirdLeft / p.BirdT0
	if p.BirdLeft > 1e-9 {
		return nil, true
	}
	p.BirdDive, p.Z = false, 0
	for _, j := range g.foes(i) {
		f := &g.P[j]
		if !bb.Reach.hits(f) || math.Hypot(f.X-p.X, f.Y-p.Y) > bb.ExplosionRadius+g.radius(j) {
			continue
		}
		g.hit(f, bb.Damage)
		if !f.Held && bb.LaunchHeight > 0 && bb.LaunchDuration > 0 {
			f.Held, f.Juggle, f.JuggleT, f.VZ = true, true, bb.LaunchDuration, 0
		}
	}
	if tn := bb.Tornado; tn.Duration > 0 { // a tornado is left where it landed: it does not chase, but takes whoever comes near
		sp := tn.spin()
		g.Balls = append(g.Balls, Ball{K: ballTornado, Owner: i, HP: tn.HP, Life: tn.Duration, Still: true, Spin: &sp, Rad: tn.Radius, Scale: tn.Scale, X: p.X, Y: p.Y})
	}
	return []Event{{T: "bbboom", X: p.X, Y: p.Y, R: bb.ExplosionRadius, K: p.Kind}}, true
}

// stepJuggle keeps the bodies Brave Bird threw up in the air: up to launch_height as fast as a tornado's fall is, there for
// `launch_duration` seconds, then down. They are Held all the while, so they cannot move or act.
func (g *Game) stepJuggle(dt float64) {
	bb := g.cfg.Rowlet.BraveBird
	rate := bb.LaunchHeight / twirlFallTime
	for i := range g.P {
		p := &g.P[i]
		if !p.Juggle {
			continue
		}
		p.Held = true
		if p.JuggleT > 0 {
			p.Z = math.Min(p.Z+rate*dt, bb.LaunchHeight)
			if p.Z >= bb.LaunchHeight {
				p.JuggleT = math.Max(p.JuggleT-dt, 0) // the time counts once it is up there
			}
			if p.JuggleT == 0 {
				p.JuggleT = -1
			}
			continue
		}
		if p.Z -= rate * dt; p.Z <= 0 {
			p.Z, p.Juggle, p.JuggleT, p.Held = 0, false, 0, false
		}
	}
}
