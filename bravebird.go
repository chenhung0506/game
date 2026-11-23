package main

import "math"

// Rowlet's ultimate, Brave Bird (Z): rooted at first for `ring_duration` (rings close in on it) and then for `charge_duration` while it
// climbs to `sky_height` and a bird of light grows out of its middle (both are counted down in Player.UltT, like Thousand Volt's
// gathering); then it dives, `dive_speed` along the straight way, at where the nearest foe stood as the dive began, and bursts where it
// lands, and Rowlet's Tornado skill (K) is put into its cooldown. Foes in the burst take `damage` and are not pushed: they are thrown up (stepJuggle) and held helpless in the air. A still tornado
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
	p.BirdDive, p.BirdTX, p.BirdTY = true, foe.X, foe.Y
	return true
}

// diveBird moves Rowlet one step along its dive and bursts when it is down. It aims at the nearest foe's place as it is now until it is `lock_distance` (along the way) from it;
// from then on the way is fixed, so a foe that steps aside in the last stretch is missed.
func (g *Game) diveBird(i int, dt float64) ([]Event, bool) {
	defer g.ultimateDamage()()
	p, bb := &g.P[i], g.cfg.Rowlet.BraveBird
	total := math.Hypot(math.Hypot(p.BirdTX-p.X, p.BirdTY-p.Y), p.Z)
	if total > bb.LockDistance {
		foe := &g.P[g.nearestFoe(i)]
		p.BirdTX, p.BirdTY = foe.X, foe.Y
		total = math.Hypot(math.Hypot(p.BirdTX-p.X, p.BirdTY-p.Y), p.Z)
	}
	if step := bb.DiveSpeed * dt; step < total-1e-9 {
		f := step / total
		p.X += (p.BirdTX - p.X) * f
		p.Y += (p.BirdTY - p.Y) * f
		p.Z -= p.Z * f
		return nil, true
	}
	p.X, p.Y = p.BirdTX, p.BirdTY
	p.BirdDive, p.Z = false, 0
	p.TwistCD = math.Max(p.TwistCD, g.cfg.Rowlet.Tornado.Cooldown) // the Tornado skill (K) goes into its cooldown too,
	p.ArrowCD = math.Max(p.ArrowCD, g.cfg.Rowlet.Arrows.Cooldown)  // and so do the Arrows (J)
	for _, j := range g.foes(i) {
		f := &g.P[j]
		if !bb.Reach.hits(f) || math.Hypot(f.X-p.X, f.Y-p.Y) > bb.ExplosionRadius+g.radius(j) {
			continue
		}
		g.hit(f, bb.Damage)
		if !f.Held {
			g.launch(f, bb.LaunchHeight, bb.LaunchDuration)
		}
	}
	if tn := bb.Tornado; tn.Duration > 0 { // a tornado is left where it landed: it does not chase, but takes whoever comes near
		sp := tn.spin()
		g.Balls = append(g.Balls, Ball{K: ballTornado, Owner: i, HP: tn.HP, Life: tn.Duration, Still: true, Spin: &sp, Rad: tn.Radius, Scale: tn.Scale, X: p.X, Y: p.Y})
	}
	events := []Event{{T: "bbboom", X: p.X, Y: p.Y, R: bb.ExplosionRadius, K: p.Kind}}
	if bb.PullDuration > 0 && bb.Gravity > 0 && bb.GravityRadius > 0 { // the landing drags the foes near it toward the spot, like a Shadow Ball's well
		g.Wells = append(g.Wells, pullWell{Owner: i, X: p.X, Y: p.Y, T: bb.PullDuration, R: bb.GravityRadius, Gravity: bb.Gravity, Reach: bb.Reach, Juggled: true})
		events = append(events, Event{T: "bbwell", X: p.X, Y: p.Y, R: bb.GravityRadius})
	}
	return events, true
}

// launch throws f up to `height` and holds it helpless there for `duration` seconds (Brave Bird's, the Uppercut's): nothing happens to a body
// something else holds already, or when either number is 0.
func (g *Game) launch(f *Player, height, duration float64) {
	if f.Held || height <= 0 || duration <= 0 {
		return
	}
	f.Held, f.Juggle, f.JuggleT, f.JuggleH, f.VZ = true, true, duration, height, 0
}

// stepJuggle keeps the bodies that were thrown up in the air: up to the height they were thrown to as fast as a tornado's fall is, there for
// their `launch_duration` seconds, then down. They are Held all the while, so they cannot move or act.
func (g *Game) stepJuggle(dt float64) {
	for i := range g.P {
		p := &g.P[i]
		if !p.Juggle {
			continue
		}
		rate := p.JuggleH / twirlFallTime
		p.Held = true
		if p.JuggleT > 0 {
			p.Z = math.Min(p.Z+rate*dt, p.JuggleH)
			if p.Z >= p.JuggleH {
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
