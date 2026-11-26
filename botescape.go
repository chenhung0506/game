package main

// escapeBySkill is a bot's way out of a threat that walking cannot get out of in time (bctx.cant: the icicles of Glaciate, a bird of fire or Brave
// Bird coming down on the spot, a storm or a tornado that is faster than the bot and already at its heels): the skill each character has that
// moves the body, pushed the way out. It says whether it took one. Every character that has a bot has one:
//
//	Pikachu    Volt Tackle (a dash)         Snorlax   the Jump (and over what only reaches the ground)
//	Chikorita  the Leaf Slash's hop         Ash       the Tree Slam's jump          Machamp   the Uppercut's slide
//	Charizard  Fly (the Seismic Toss as the Gigantamax one, which cannot fly)
//	Vulpix     the Frost Field's slide      Gengar    Shadow Sneak (under the ground)     Rowlet   the Tailwind's drift
//	Jigglypuff the Roll Kick's roll         Magikarp  the jump (over it)    Gyarados  Surf (once the wave is up)
func (b *Bot) escapeBySkill(c *bctx) (Input, bool) {
	th, p, g := c.th, c.p, c.g
	if th.kind == thNone || !c.cant || th.eta > botHorizon {
		return Input{}, false
	}
	if p.FrozenT > 0 || p.Held || p.StunT > 0 || p.ParalyzeT > 0 || p.BoundT > 0 || p.SilentT > 0 || p.UltT > 0 {
		return Input{}, false // no skill starts, or the body is not its own
	}
	in := Input{AimX: th.ax, AimY: th.ay}
	in.DX, in.DY = dir8(th.ax, th.ay)
	if in.DX == 0 && in.DY == 0 {
		return Input{}, false
	}
	if th.kind == thChase && p.Kind == snorlax {
		return Input{}, false // its jump takes it up, not away: a tornado reaches the air and a storm follows below, so only walking buys time
	}
	bc := g.brawler(c.me)
	switch p.Kind {
	case pikachu:
		if p.DashT == 0 && (p.VoltLeft > 0 || p.DashCD == 0) && p.BallT == 0 && p.ThunderLeft == 0 {
			in.Skill = true
			return in, true
		}
	case snorlax:
		busy := p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0 || p.PunchT > 0 || p.PunchLeft > 0 || p.PunchRecover > 0
		if p.JumpCD == 0 && p.Z == 0 && p.VZ == 0 && !busy {
			in.Skill = true
			return in, true
		}
	case chikorita:
		if p.LeafCD == 0 && p.LeafPhase == leafNone && !p.vineBusy() && !g.giantOut(c.me) && p.Z == 0 {
			in.Skill = true
			return in, true
		}
	case ash, machamp:
		idle := p.Z == 0 && p.TreePhase == treeNone && p.CometT == 0 && p.MegaT == 0 && p.MegaJT == 0 && p.KickPhase == kickNone && p.UpPhase == upNone && p.LpT == 0 && p.BpT == 0 && p.BpS == 0 && p.RockT == 0
		switch {
		case p.Kind == ash && idle && p.TreeCD == 0 && bc.TreeSlam.JumpDistance > 0: // the jump the Tree Slam starts with goes the way the stick says
			in.Skill = true
			return in, true
		case p.Kind == machamp && idle && p.UpCD == 0 && bc.Uppercut.SlideDistance > 0: // the Uppercut slides the way the stick says first
			in.Skill = true
			return in, true
		}
	case charizard:
		if p.TossPhase != tossNone || p.InfT > 0 || p.BreathT > 0 || p.Z > 0 {
			break
		}
		switch {
		case !p.gigantamax() && p.FlyCD == 0 && p.FlyT == 0:
			in.Skill = true
			return in, true
		case p.TossCD == 0: // the dash of the Seismic Toss, along the aim
			in.Toss = true
			return in, true
		}
	case vulpix:
		if p.FrostCD == 0 && p.FrostT == 0 && p.FBeamT == 0 {
			in.Skill = true
			return in, true
		}
	case gengar:
		if p.SneakCD == 0 && p.SneakT == 0 && !p.gigantamax() { // the Gigantamax Gengar's main button is the Lick, not a way out
			in.Skill = true
			return in, true
		}
	case jigglypuff:
		if p.RollCD == 0 && p.Z == 0 && !p.JpFly && p.SlapT == 0 && p.RollT == 0 {
			in.Roll = true // along the aim (the way out), the exact angle
			return in, true
		}
	case magikarp:
		if th.kind != thChase && p.HopLeft == 0 && p.HopCD == 0 && th.eta <= 0.6 { // up and over (a chaser waits below); the stick steers it in the air
			in.Skill = true
			return in, true
		}
	case gyarados:
		busy := p.TailT > 0 || p.BeamT > 0 || p.SweepT > 0 || p.RechargeT > 0 || p.surfing() || p.Z > 0
		if !busy && p.WaveCD == 0 && th.eta > g.cfg.Gyarados.Wave.SummonDuration+0.15 { // the wave rides it out along the stick, once it is up
			in.Skill = true
			return in, true
		}
	case rowlet:
		if p.DriftCD == 0 && p.DriftT == 0 && p.TwistT == 0 && p.TwistRecT == 0 && p.ArrowT == 0 && !p.BirdDive {
			in.Skill = true
			return in, true
		}
	}
	return Input{}, false
}
