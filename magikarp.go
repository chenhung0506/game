package main

import "math"

// Magikarp and the Gyarados it evolves into.
//
// Magikarp has one skill, the jump: up `height`, a stay at the top, and down, all in `duration` seconds, harmless to
// everyone, a way to be out of reach of whatever cannot hit that high. Every landing counts, and the evolve_hops-th
// one turns it into Gyarados, which keeps the share of its HP it has left.
//
// Gyarados hops and sends a wave out over the ground (Skill), hits a fan in front of it with its tail and knocks
// back and stuns whoever it catches (Tail), and fires Snorlax's Hyper Beam (Beam; see the Snorlax block in Step).

// wave is one of Gyarados's waves: a disc growing from where it landed, which hurts the opponent once, when its edge
// first reaches it.
type wave struct {
	Owner int
	X, Y  float64
	R     float64
	Hit   bool
}

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

// stepGyarados runs the wave and the tail whip, and reports whether Gyarados is rooted this tick (in its hop: the
// walking code stays out of it).
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

	// ---- Wave: a hop, and the landing shakes the ground ----
	if in.Skill && p.WaveT == 0 && p.WaveCD == 0 && !beamBusy && p.TailT == 0 && p.Z == 0 {
		p.WaveT, p.WaveCD = wv.JumpDuration, wv.Cooldown
	}
	rooted := p.WaveT > 0 // through the tick the hop ends in, free from the next
	if p.WaveT > 0 {
		if p.WaveT -= dt; p.WaveT < 1e-9 {
			p.WaveT, p.Z = 0, 0
			g.Waves = append(g.Waves, wave{Owner: i, X: p.X, Y: p.Y, R: g.radius(i)})
			events = append(events, Event{T: "wave", X: p.X, Y: p.Y, R: wv.Radius})
		} else {
			s := 1 - p.WaveT/wv.JumpDuration
			p.Z = 4 * wv.JumpHeight * s * (1 - s)
		}
	} else if p.Z > 0 && !p.Held && p.Twirl == twirlNone { // a hop that was cut short (frozen, stunned) leaves it in the air
		p.Z = 0
	}

	// ---- Tail whip: wind up with the fan shown, then hit what is in it ----
	// The fan goes along the aim (the stick, else the last way it walked, like the Ice Punch) as it was when K was
	// pressed, so the warning is exactly what gets hit. Gyarados is rooted while it winds up, and through the tick it hits in.
	if in.Tail && p.TailCD == 0 && p.TailT == 0 && p.WaveT == 0 && !beamBusy {
		p.TailCD = tw.Cooldown // counted from the press
		p.TailX, p.TailY = p.AimX, p.AimY
		if math.Hypot(p.TailX, p.TailY) < 1e-9 {
			p.TailX, p.TailY = unit(float64(p.FX), float64(p.FY))
		}
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
	p, other := &g.P[i], &g.P[1-i]
	tw := g.cfg.Gyarados.TailWhip
	ux, uy := p.TailX, p.TailY
	dx, dy := other.X-p.X, other.Y-p.Y
	d := math.Hypot(dx, dy)
	if tw.Reach.hits(other) && d <= tw.Length+g.radius(1-i) && (d < 1e-9 || (dx*ux+dy*uy)/d >= math.Cos(tw.Angle/2*math.Pi/180)) {
		g.knockBack(p.X, p.Y, other, tw.Knockback, ux, uy)
		other.StunT = math.Max(other.StunT, tw.StunDuration)
	}
	return []Event{{T: "tail", X: p.X, Y: p.Y, R: tw.Length, DX: ux, DY: uy}}
}

// stepWaves spreads Gyarados's waves. The edge of a wave hurts the opponent once, the first time it reaches the
// opponent's body, if the opponent is then at a height the wave can hit; a wave that reaches it in the air passes under.
func (g *Game) stepWaves(dt float64) {
	wv := g.cfg.Gyarados.Wave
	kept := g.Waves[:0]
	for _, w := range g.Waves {
		foe := &g.P[1-w.Owner]
		w.R = math.Min(w.R+wv.Speed*dt, wv.Radius)
		if !w.Hit && math.Hypot(foe.X-w.X, foe.Y-w.Y)-g.radius(1-w.Owner) <= w.R {
			w.Hit = true
			if wv.Reach.hits(foe) {
				g.hit(foe, wv.Damage)
			}
		}
		if w.R < wv.Radius {
			kept = append(kept, w)
		}
	}
	g.Waves = kept
}
