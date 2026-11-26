package main

// The Jigglypuff bot: it closes in and slaps in a chain, rolls into the foe from a little way off (the kick launches both of them, and it carries on in the air with the stomps and the
// kick), stomps a foe that is in the air by jumping on it from above, and lets the song out (its ultimate) as soon as it is charged and the foe is in reach. It rolls out of the way
// of what it cannot walk away from. It does not mind a Jigglypuff's own attacks (the notes hurt nothing, the slaps are met in melee) as a foe, only the usual threats.
func (b *Bot) jigglypuff(c *bctx) Input {
	g, p, f := c.g, c.p, c.f
	jp := g.cfg.Jigglypuff
	gr, ar, rk, jm := jp.Combo.Ground, jp.Combo.Air, jp.RollKick, jp.Jump
	if p.FrozenT > 0 || p.StunT > 0 || p.Held || p.ParalyzeT > 0 {
		return c.stay()
	}
	danger := f.Kind == rowlet && f.ArrowT > 0
	if p.SlapT > 0 || p.RollT > 0 { // in the middle of a blow or a roll: a blow already queued is all it can do (and a press now is the next blow)
		in := c.stay()
		in.Skill = p.SlapT > 0 && p.SlapChain && !p.SlapQ
		return in
	}
	if p.Charge >= jp.Ultimate.Max && !danger && c.dist <= jp.Sing.RingRadius*1.8+300 && f.DozeT == 0 { // the song, once it is charged and the foe is within reach of the notes
		in := c.stay()
		in.Ult = true
		return in
	}
	if th := c.th; th.kind != thNone && th.eta <= botHorizon {
		in := c.move(th.ax, th.ay)
		if th.eta <= 0.35 && p.RollCD == 0 && p.Z == 0 && !p.JpFly { // too close to walk clear: roll out of it
			in.AimX, in.AimY = th.ax, th.ay
			in.Roll = true
		}
		return in
	}
	foeAir := f.Z > 20
	if p.Z > 0 || p.JpFly { // in the air: stomp what is under it, kick what has been stomped, jump on a foe that is up in the air
		near := c.dist <= ar.Radius+c.fr+c.pr
		in := c.move(c.ux, c.uy)
		switch {
		case p.SlapChain && p.SlapAir: // a chain going: the next blow (the third kicks a foe above as well as below)
			in.Skill = near
		case foeAir && p.JumpSkillCD == 0 && c.dist <= jm.CatchRadius*2+c.fr && p.Z >= f.Z-30 && b.rng.Float64() < 0.5:
			in.Jump = true // a fresh jump in the air: it comes down on the foe and stomps it to the floor
		case near && f.Z <= p.Z+10 && p.SlapCD == 0:
			in.Skill = true
		}
		return in
	}
	// on the ground
	meleeR := gr.Length*0.8 + c.fr + gr.LungeDistance*0.5
	if p.RollCD == 0 && c.dist >= 120 && c.dist <= rk.RollDistance+rk.KickRadius*0.6 && c.dist > meleeR && b.rng.Float64() < 0.65 && !danger {
		in := c.stay() // aimed at the foe: the roll goes along the exact angle
		in.Roll = true
		return in
	}
	if c.dist <= meleeR+20 && (p.SlapChain || p.SlapCD == 0) && !danger {
		in := c.stay()
		in.Skill = true
		return in
	}
	tx, ty := -c.uy*b.strafe, c.ux*b.strafe
	switch {
	case p.SlapCD > 0 && !p.SlapChain && c.dist < meleeR+120: // its chain is cooling down: not in the foe's face meanwhile
		return c.move(-c.ux+tx*0.5, -c.uy+ty*0.5)
	case c.dist > meleeR:
		return c.move(c.ux+tx*0.15, c.uy+ty*0.15)
	}
	return c.move(tx, ty)
}
