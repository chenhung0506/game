package main

import "math"

// A bot walled in by solids (the pillars of the Freeze Beam, Glaciate's icicles, the stones of Rock Tomb) cannot walk or dodge out, but every one of them has HP and
// breaks under any skill that hurts projectiles. So when none of the eight ways of the stick gets anywhere, it stops playing the usual game and spends its skills on the
// solid that is in the way: the one nearest to it, leaning to the one on the line to the foe, where the projectiles it throws go anyway.

// botBoxRay is how far (pixels) a way out has to be free for the bot to call itself not walled in.
const botBoxRay = 350.0

// breakKeys are the keys of each character that can hurt what is around it. The server only reads the keys of the character in the slot, so pressing one of another is harmless,
// but the list is by character so that no decision is spent on a key that does nothing. The ultimates are left out: they are not to be wasted on a stone.
var breakKeys = map[int][]func(*Input){
	pikachu:   {func(in *Input) { in.Skill = true }, func(in *Input) { in.Ball = true }, func(in *Input) { in.Thunder = true }},
	snorlax:   {func(in *Input) { in.Skill = true }, func(in *Input) { in.Beam = true }, func(in *Input) { in.Punch = true }},
	charizard: {func(in *Input) { in.Fire = true }, func(in *Input) { in.Toss = true }, func(in *Input) { in.Skill = true }},
	chikorita: {func(in *Input) { in.Razor = true }, func(in *Input) { in.Vine = true }, func(in *Input) { in.Skill = true }},
	vulpix:    {func(in *Input) { in.Storm = true }, func(in *Input) { in.Freeze = true }, func(in *Input) { in.Skill = true }},
	rowlet:    {func(in *Input) { in.Arrow = true }, func(in *Input) { in.Twister = true }, func(in *Input) { in.Skill = true }},
	ash:       {func(in *Input) { in.Comet = true }, func(in *Input) { in.Skill = true }, func(in *Input) { in.Kick = true }},
	machamp:   {func(in *Input) { in.Comet = true }, func(in *Input) { in.Skill = true }, func(in *Input) { in.Kick = true }},
	gengar:    {func(in *Input) { in.GFire = true }, func(in *Input) { in.SBall = true }, func(in *Input) { in.Skill = true }},
}

// walledIn is the solid to break when every way out is shut, and whether they are.
func (c *bctx) walledIn() (*Ball, bool) {
	g, p := c.g, c.p
	var solids []*Ball
	var radii []float64
	for i := range c.balls {
		b := &c.balls[i]
		if !solid(b.K) || g.side(b.Owner) == g.side(c.me) || b.HP <= 0 || b.Z > 0 {
			continue
		}
		if reach, r := g.solidBody(b); reach.hits(p) {
			solids, radii = append(solids, b), append(radii, r)
		}
	}
	if len(solids) == 0 {
		return nil, false
	}
	stops := map[int]bool{} // the solids that stand in the way of some way out
	for k := 0; k < 8; k++ {
		ux, uy := unit(float64(stickX[k]), float64(stickY[k]))
		shut := false
	ray:
		for d := 10.0; d <= botBoxRay; d += 10 {
			x, y := p.X+ux*d, p.Y+uy*d
			if x < 0 || x > g.court.Length || y < 0 || y > g.court.Width {
				shut = true
				break
			}
			for i, b := range solids {
				if math.Hypot(x-b.X, y-b.Y) < c.pr+radii[i] {
					shut = true
					stops[i] = true
					break ray
				}
			}
		}
		if !shut {
			return nil, false
		}
	}
	if len(stops) == 0 {
		return nil, false // only the edges of the court: nothing to break
	}
	best, bestScore := -1, math.Inf(1)
	for i := range stops {
		b := solids[i]
		if s := math.Hypot(p.X-b.X, p.Y-b.Y) - radii[i] + 0.5*segDist(b.X, b.Y, p.X, p.Y, c.f.X, c.f.Y); s < bestScore {
			best, bestScore = i, s
		}
	}
	return solids[best], true
}

// breakOut is what the bot holds while it is walled in: a skill of its own at random (the ones that can be used again are the ones that go off), the stick and the aim on blk.
func (b *Bot) breakOut(c *bctx, blk *Ball) Input {
	ux, uy := unit(blk.X-c.p.X, blk.Y-c.p.Y)
	in := Input{AimX: ux, AimY: uy, CastX: ux, CastY: uy}
	in.DX, in.DY = dir8(ux, uy)
	if keys := breakKeys[b.kind]; len(keys) > 0 {
		keys[b.rng.IntN(len(keys))](&in)
	}
	return in
}
