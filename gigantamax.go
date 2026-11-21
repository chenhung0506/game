package main

import "math"

// Machamp's and Charizard's ultimate (Z): once the charge is full, the press empties it and the character becomes its Gigantamax self for `<character>.gigantamax.duration`
// seconds. It is still the same character (the same Kind, skills and keys), with its own max_hp, walk_speed and radius. Machamp has its own numbers for the Uppercut, the
// Comet Punch and a Lightning Punch (see GigantamaxCfg, brawlerP); Charizard for its three skills, Fly, Fire Breath and Seismic Toss (see CharizardGmaxCfg, charizardP).
// The HP is scaled to the new maximum, and back when it is over.

// gmaxStats is what a Gigantamax form changes about its body.
type gmaxStats struct {
	MaxHP     int
	WalkSpeed float64
	Radius    float64
	Duration  float64
}

// gigantamaxOf is the body of a character's Gigantamax form, and whether the character has one.
func (c Config) gigantamaxOf(kind int) (gmaxStats, bool) {
	switch kind {
	case machamp:
		gm := c.Machamp.Gigantamax
		return gmaxStats{gm.MaxHP, gm.WalkSpeed, gm.Radius, gm.Duration}, true
	case charizard:
		gm := c.Charizard.Gigantamax
		return gmaxStats{gm.MaxHP, gm.WalkSpeed, gm.Radius, gm.Duration}, true
	}
	return gmaxStats{}, false
}

// CharizardSkills are the numbers Charizard's three skills run on: its own, or the Gigantamax Charizard's.
type CharizardSkills struct {
	Fly         FlyCfg
	FireBreath  FireBreathCfg
	SeismicToss SeismicTossCfg
}

// charizardP is the numbers p (a Charizard) plays by right now.
func (g *Game) charizardP(p *Player) CharizardSkills {
	if p.gigantamax() {
		gm := g.cfg.Charizard.Gigantamax
		return CharizardSkills{gm.Fly, gm.FireBreath, gm.SeismicToss}
	}
	cz := g.cfg.Charizard
	return CharizardSkills{cz.Fly, cz.FireBreath, cz.SeismicToss}
}

// fireBreathOf is the fire ball numbers of a fire ball: a ball breathed out by a Gigantamax Charizard keeps the Gigantamax numbers after it has changed back.
func (g *Game) fireBreathOf(ult bool) FireBreathCfg {
	if ult {
		return g.cfg.Charizard.Gigantamax.FireBreath
	}
	return g.cfg.Charizard.FireBreath
}

// rescaleHP changes slot i's HP in proportion to a change of its maximum (from `before` to what it is now), keeping at least 1.
func (g *Game) rescaleHP(i, before int) {
	p := &g.P[i]
	was := p.HP
	p.HP = min(g.maxHP(i), max(1, int(math.Round(float64(p.HP)*float64(g.maxHP(i))/float64(before)))))
	g.hpAdj[i] += p.HP - was // not damage and not healing: the charge does not count it (gainCharge)
}

// stepGigantamax runs Z for Machamp and Charizard: it starts the transformation when the charge is full and nothing else is going on, and ends it when its time is up.
func (g *Game) stepGigantamax(i int, in Input, u Ultimate, dt float64) ([]Event, bool) {
	p := &g.P[i]
	if p.GmaxT > 0 {
		before := g.maxHP(i) // taken while it is still Gigantamax: the tick that takes off the last of the time can leave GmaxT at (or a hair under) zero
		if p.GmaxT -= dt; p.GmaxT < 1e-9 {
			p.GmaxT = 0
			g.rescaleHP(i, before)
			return []Event{{T: "gmaxend", X: p.X, Y: p.Y, R: g.radius(i), K: p.Kind}}, false
		}
		return nil, false
	}
	busy := p.TreePhase != treeNone || p.CometT > 0 || p.RockT > 0 || p.MegaT > 0 || p.MegaJT > 0 || p.KickPhase != kickNone || p.UpPhase != upNone || p.LpT > 0 ||
		p.Z != 0 || p.Held || p.FrozenT > 0 || p.StunT > 0 ||
		p.TossPhase != tossNone || p.FlyT > 0 || p.BreathT > 0 // Charizard: in a toss, a flight or a breath
	if !in.Ult || p.Charge < u.Max || busy {
		return nil, false
	}
	p.Charge = 0
	before := g.maxHP(i)
	gm, _ := g.cfg.gigantamaxOf(p.Kind)
	p.GmaxT = gm.Duration
	g.rescaleHP(i, before)
	return []Event{{T: "gmax", X: p.X, Y: p.Y, R: g.radius(i), K: p.Kind}}, false
}
