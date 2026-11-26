package main

import "math"

// The Magikarp bot, and the Gyarados it turns into (one Bot plays both: the kind it was made for stays magikarp, the
// body it steers is what decides, see Decide).
//
// Magikarp has nothing that hurts, so it keeps its distance, walking round the foe, and uses its jump two ways: to go
// over what is about to hit it, and, when nothing is coming, to count landings towards the evolution as soon as it can.
//
// Gyarados surfs at the foe (straight at it: the wave runs where the foe stood at the press), always when the foe
// cannot walk out of the lane in time and otherwise by chance when it is in reach, whips the tail at where the foe
// will be when the wind-up ends, and beams a foe that cannot move. While it calls up a wave or rides it, there is
// nothing to decide.
func (b *Bot) magikarp(c *bctx) Input {
	if c.p.Kind == gyarados {
		return b.gyarados(c)
	}
	g, p, f := c.g, c.p, c.f
	mj := g.cfg.Magikarp.Jump
	if p.FrozenT > 0 || p.StunT > 0 || p.Held {
		return c.stay()
	}
	canHop := p.HopLeft == 0 && p.HopCD == 0
	if th := c.th; th.kind != thNone && th.eta <= botHorizon {
		in := c.move(th.ax, th.ay)
		// over it: a jump is up high a third of its time in, so start it a little before the threat arrives (a
		// chaser follows below and waits, so walking is the only thing that helps there)
		in.Skill = canHop && th.kind != thChase && th.eta <= 0.5
		return in
	}
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	var in Input
	switch {
	case c.dist < 420: // it cannot hurt the foe: back off
		in = c.move(-c.ux+tx*0.4, -c.uy+ty*0.4)
	case c.dist > 620:
		in = c.move(c.ux+tx*0.3, c.uy+ty*0.3)
	default:
		in = c.move(tx, ty)
	}
	// nothing is coming: a landing more towards the evolution, but not with the foe close enough to be waiting under it
	in.Skill = canHop && c.dist-c.fr-c.pr > mj.AirSpeedFirstHalf*mj.Duration+80 && !f.surfing()
	return in
}

func (b *Bot) gyarados(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	gy := g.cfg.Gyarados
	wv, tw, hb := gy.Wave, gy.TailWhip, g.cfg.beamOf(gyarados)
	if p.FrozenT > 0 || p.StunT > 0 || p.Held || p.surfing() || p.TailT > 0 || p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0 {
		return c.stay() // busy, or not its own: the wave and the wind-up go on without keys
	}
	th := c.th
	if th.kind != thNone && th.eta <= botHorizon {
		return c.move(th.ax, th.ay)
	}
	danger := f.Kind == rowlet && f.ArrowT > 0 // everything it has roots it: not while a volley is being aimed at it
	rooted := c.rootedFor()

	// Surf: the foe in reach of the run, and either unable to walk out of the lane before the wave is there or by chance.
	if p.WaveCD == 0 && !danger && wv.Reach.hits(f) && c.dist <= wv.Distance+g.radius(c.me)+c.fr-20 {
		arrive := wv.SummonDuration + math.Max(0, c.dist-c.pr-c.fr)/wv.Speed
		out := (wv.Width/2 + c.fr) / g.walkSpeedOf(f) // sideways out of the lane, from its middle
		if rooted+out >= arrive || b.rng.Float64() < 0.35 {
			in := c.stay() // no stick: the wave runs straight at the foe
			in.Skill = true
			return in
		}
	}
	// The tail whip: at the foe's place when the wind-up ends; a stunned foe is a beam's.
	if p.TailCD == 0 && !danger && tw.Reach.hits(f) && c.dist <= tw.Length+c.fr-15 {
		in := c.stay()
		ax, ay := c.lead(tw.ChargeDuration)
		in.Tail, in.CastX, in.CastY = true, ax, ay
		return in
	}
	// The beam: a foe that cannot move for a while, within the beam's length.
	if p.BeamCD == 0 && !danger && hb.Reach.hits(f) && rooted >= 0.8 && c.dist <= hb.Length-30 {
		in := c.stay()
		in.Beam = true
		return in
	}
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	switch {
	case p.WaveCD == 0 && c.dist > wv.Distance*0.8: // the wave is ready: into its reach
		return c.move(c.ux+tx*0.2, c.uy+ty*0.2)
	case p.WaveCD > 0 && p.TailCD > 0 && c.dist < 260: // nothing ready: not in the foe's face
		return c.move(-c.ux+tx*0.5, -c.uy+ty*0.5)
	case c.dist > tw.Length+c.fr:
		return c.move(c.ux+tx*0.3, c.uy+ty*0.3)
	}
	return c.move(tx, ty)
}

// surfLane is whether (x, y) is where the Surf wave of f (calling up or riding) still runs, with `pad` more on every side.
func (g *Game) surfLane(f *Player, x, y, pad float64) bool {
	if !f.surfing() {
		return false
	}
	w := g.cfg.Gyarados.Wave
	dx, dy := x-f.X, y-f.Y
	along, across := dx*f.SurfX+dy*f.SurfY, -dx*f.SurfY+dy*f.SurfX
	fr := g.bodyRadius(f)
	return along >= -fr-pad && along <= f.SurfLeft+fr+pad && math.Abs(across) <= w.Width/2+pad
}
