package main

import "math"

// Magikarp and the Gyarados it evolves into.
//
// Magikarp has one skill, the jump: up `height`, a stay at the top, and down, all in `duration` seconds, harmless to
// everyone, a way to be out of reach of whatever cannot hit that high. Every landing counts, and the evolve_hops-th
// one turns it into Gyarados, which keeps the share of its HP it has left.
//
// Gyarados rides a wave straight on and sweeps up whoever it runs into (Skill, Surf), hits a fan in front of it with
// its tail and knocks back and stuns whoever it catches (Tail), and fires Snorlax's Hyper Beam (Beam; see the
// Snorlax block in Step).

// evolve turns slot i's Magikarp into a Gyarados.
func (g *Game) evolve(i int) Event {
	p := &g.P[i]
	before := g.maxHP(i)
	p.Kind = gyarados
	p.HP = max(1, int(math.Round(float64(p.HP)*float64(g.maxHP(i))/float64(before))))
	return Event{T: "evolve", X: p.X, Y: p.Y, R: g.radius(i)}
}

// stepMagikarp runs the jump. The height follows the time since take-off, not the usual gravity: rising slows to a
// stop at the top, the stay is level, and the fall speeds up to the ground, so a jump is exactly `height` high and
// `duration` long whatever the tick length.
func (g *Game) stepMagikarp(i int, in Input, dt float64) []Event {
	var events []Event
	p := &g.P[i]
	mj := g.cfg.Magikarp.Jump
	if p.HopCD > 0 {
		if p.HopCD -= dt; p.HopCD < 1e-9 {
			p.HopCD = 0
		}
	}
	if p.Held { // caught: whoever holds it decides its height, and the jump does not count
		p.HopLeft = 0
		return nil
	}
	if in.Skill && p.HopLeft == 0 && p.HopCD == 0 {
		p.HopLeft, p.HopCD = mj.Duration, mj.Cooldown
	}
	if p.HopLeft == 0 {
		return nil
	}
	if p.HopLeft -= dt; p.HopLeft < 1e-9 { // down
		p.HopLeft, p.Z = 0, 0
		if p.Hops++; p.Hops >= g.cfg.Magikarp.EvolveHops {
			events = append(events, g.evolve(i))
		}
		return events
	}
	t := mj.Duration - p.HopLeft // seconds since take-off
	rise := (mj.Duration - mj.HighestPointDuration) / 2
	switch {
	case t < rise:
		s := t / rise
		p.Z = mj.Height * (1 - (1-s)*(1-s))
	case t < rise+mj.HighestPointDuration:
		p.Z = mj.Height
	default:
		s := (t - rise - mj.HighestPointDuration) / rise
		p.Z = mj.Height * (1 - s*s)
	}
	return events
}

// magikarpAirSpeed is how fast Magikarp is steered in the air: the first speed until the middle of the jump, the second after.
func (g *Game) magikarpAirSpeed(p *Player) float64 {
	mj := g.cfg.Magikarp.Jump
	if mj.Duration-p.HopLeft < mj.Duration/2 {
		return mj.AirSpeedFirstHalf
	}
	return mj.AirSpeedSecondHalf
}

// surfing reports whether Gyarados is calling up or riding its Surf wave.
func (p *Player) surfing() bool { return p.SurfT > 0 || p.SurfLeft > 0 }

// stepGyarados runs Surf and the tail whip, and reports whether Gyarados is rooted this tick (calling the wave up or
// riding it: the wave moves it, so the walking code stays out of it).
func (g *Game) stepGyarados(i int, in Input, dt float64) ([]Event, bool) {
	var events []Event
	p := &g.P[i]
	wv, tw := g.cfg.Gyarados.Wave, g.cfg.Gyarados.TailWhip
	for _, cd := range []*float64{&p.WaveCD, &p.TailCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}
	beamBusy := p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0

	// ---- Surf: call up a wave, then ride it straight on ----
	if in.Skill && !p.surfing() && p.WaveCD == 0 && !beamBusy && p.TailT == 0 && p.Z == 0 {
		p.WaveCD, p.SurfT, p.SurfLeft = wv.Cooldown, wv.SummonDuration, wv.Distance
		p.SurfX, p.SurfY = g.surfDir(i, in)
		events = append(events, Event{T: "surf", X: p.X, Y: p.Y, R: wv.Width, DX: p.SurfX, DY: p.SurfY})
	}
	rooted := p.surfing() // through the tick the ride ends in, free from the next
	if p.SurfT > 0 {
		if p.SurfT -= dt; p.SurfT < 1e-9 {
			p.SurfT = 0
		}
	} else if p.SurfLeft > 0 {
		events = append(events, g.surf(i, math.Min(wv.Speed*dt, p.SurfLeft))...)
	}

	// ---- Tail whip: wind up with the fan shown, then hit what is in it ----
	// The fan goes the way the player chose (the page's tail stick, see Input.CastX) as it was when the cast came in, or at the
	// opponent when none was chosen, so the warning is exactly what gets hit. Gyarados is rooted while it winds up, and through the tick it hits in.
	if in.Tail && p.TailCD == 0 && p.TailT == 0 && !p.surfing() && !beamBusy {
		p.TailCD = tw.Cooldown // counted from the press
		p.TailX, p.TailY = g.castDir(i, in)
		p.TailT = tw.ChargeDuration
		if p.TailT == 0 {
			events = append(events, g.whip(i)...)
		}
	}
	if p.TailT > 0 {
		rooted = true
		if p.TailT -= dt; p.TailT < 1e-9 {
			p.TailT = 0
			events = append(events, g.whip(i)...)
		}
	}
	return events, rooted
}

// whip is the tail whip landing: the fan fixed when it was started, knocking back and stunning the opponent in it.
func (g *Game) whip(i int) []Event {
	p := &g.P[i]
	tw := g.cfg.Gyarados.TailWhip
	ux, uy := p.TailX, p.TailY
	for _, j := range g.foes(i) {
		other := &g.P[j]
		dx, dy := other.X-p.X, other.Y-p.Y
		d := math.Hypot(dx, dy)
		if tw.Reach.hits(other) && d <= tw.Length+g.radius(j) && (d < 1e-9 || (dx*ux+dy*uy)/d >= math.Cos(tw.Angle/2*math.Pi/180)) {
			g.knockBack(p.X, p.Y, other, tw.Knockback, ux, uy)
			g.stun(other, tw.StunDuration)
		}
	}
	return []Event{{T: "tail", X: p.X, Y: p.Y, R: tw.Length, DX: ux, DY: uy}}
}

// surfDir is the way slot i's Surf wave runs: the way held, else at the nearest foe (else the way it faces).
func (g *Game) surfDir(i int, in Input) (float64, float64) {
	if in.DX != 0 || in.DY != 0 {
		return unit(float64(in.DX), float64(in.DY))
	}
	return g.castDir(i, Input{})
}

// surf moves slot i's Gyarados `step` along its wave and sweeps up the foes the wave's front runs into on the way.
// The wave is `width` across and as deep as Gyarados's body, and it is checked along the whole step, so a fast wave
// cannot jump over anyone. The ride ends when the distance is run or the wave hits a wall (stepSurf then lets go).
func (g *Game) surf(i int, step float64) []Event {
	var events []Event
	p := &g.P[i]
	wv := g.cfg.Gyarados.Wave
	ux, uy := p.SurfX, p.SurfY
	vx, vy := -uy, ux // across the wave
	x0, y0 := p.X, p.Y
	nx, ny := x0+ux*step, y0+uy*step
	p.X, p.Y = math.Min(math.Max(nx, 0), g.court.Length), math.Min(math.Max(ny, 0), g.court.Width)
	wall := p.X != nx || p.Y != ny
	if p.SurfLeft -= step; p.SurfLeft < 1e-9 {
		p.SurfLeft = 0
	}
	rg := g.radius(i)
	for _, j := range g.foes(i) {
		f := &g.P[j]
		if f.Held || f.Twirl != twirlNone || !wv.Reach.hits(f) {
			continue
		}
		rf := g.radius(j)
		dx, dy := f.X-x0, f.Y-y0
		along, across := dx*ux+dy*uy, dx*vx+dy*vy
		if along < -rg-rf || along > step+rg+rf || math.Abs(across) > wv.Width/2+rf {
			continue
		}
		f.Held, f.VZ, f.SurfBy, f.SurfIn = true, 0, i+1, wv.CarryInterval
		f.SurfOff = math.Min(math.Max(across, -wv.Width/2), wv.Width/2)
		g.hit(f, wv.Damage)
		events = append(events, Event{T: "surfhit", X: f.X, Y: f.Y})
	}
	if wall {
		p.SurfLeft = 0 // the wave breaks on the wall; stepSurf lets go of what it carries, where it is now
	}
	return events
}

// dropSurf ends slot i's Surf, if it is calling up or riding a wave, and lets go of everything the wave carries: each
// body falls the way a tornado's does (stepTwirl), and takes landing_damage when it lands.
func (g *Game) dropSurf(i int) {
	p := &g.P[i]
	p.SurfT, p.SurfLeft = 0, 0
	wv := g.cfg.Gyarados.Wave
	for v := range g.P {
		if f := &g.P[v]; f.SurfBy == i+1 {
			f.SurfBy, f.SurfOff, f.SurfIn = 0, 0, 0
			// SpinDuration only marks the numbers as set (stepTwirl falls back to the tornado's on a zero one); the fall does not use it
			f.Twirl, f.TwZ0, f.TwUlt = twirlFall, 0, false
			f.Tw = spinParams{Reach: wv.Reach, LiftHeight: wv.LiftHeight, SpinDuration: 1, LandingDamage: wv.LandingDamage}
		}
	}
}

// stepSurf carries the bodies Surf waves have swept up: each rides just in front of its Gyarados, where it was hit
// across the wave, rises to lift_height as fast as a tornado's catch falls, and takes carry_damage every
// carry_interval. A wave whose ride is over, or whose Gyarados is gone (fallen in a 2v2), lets go after placing them
// one last time. It runs after the players have moved.
func (g *Game) stepSurf(dt float64) {
	wv := g.cfg.Gyarados.Wave
	for v := range g.P {
		f := &g.P[v]
		if f.SurfBy == 0 {
			continue
		}
		o := f.SurfBy - 1
		p := &g.P[o]
		d := g.radius(o) + g.radius(v)
		f.X = math.Min(math.Max(p.X+p.SurfX*d-p.SurfY*f.SurfOff, 0), g.court.Length)
		f.Y = math.Min(math.Max(p.Y+p.SurfY*d+p.SurfX*f.SurfOff, 0), g.court.Width)
		f.Z = math.Min(wv.LiftHeight, f.Z+wv.LiftHeight/twirlFallTime*dt)
		for f.SurfIn -= dt; f.SurfIn < 1e-9; f.SurfIn += wv.CarryInterval { // a loop so a tiny interval cannot lose ticks
			g.hit(f, wv.CarryDamage)
		}
		if p.SurfLeft == 0 || p.HP <= 0 { // the ride is over (this tick, where it ended), or its Gyarados has fallen
			g.dropSurf(o)
		}
	}
}

// castDir is the unit direction slot i's aimed skill goes: the one the player chose, else straight at the opponent
// (else the way it faces, when they are on the same spot).
func (g *Game) castDir(i int, in Input) (float64, float64) {
	if n := math.Hypot(in.CastX, in.CastY); n > 1e-9 && !math.IsInf(n, 0) {
		return in.CastX / n, in.CastY / n
	}
	p, o := &g.P[i], &g.P[g.nearestFoe(i)]
	if n := math.Hypot(o.X-p.X, o.Y-p.Y); n > 1e-9 {
		return (o.X - p.X) / n, (o.Y - p.Y) / n
	}
	return unit(float64(p.FX), float64(p.FY))
}
