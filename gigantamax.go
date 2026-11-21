package main

import "math"

// Machamp's ultimate (Z): once the charge is full, the press empties it and Machamp becomes the Gigantamax Machamp for `machamp.gigantamax.duration` seconds. It is
// still Machamp (Kind machamp, the same skills and keys), with its own max_hp, walk_speed and radius and its own numbers for the Uppercut, the High Jump Kick and the
// Comet Punch (see GigantamaxCfg, brawlerP). Its HP is scaled to the new maximum, and back when it is over.

// rescaleHP changes slot i's HP in proportion to a change of its maximum (from `before` to what it is now), keeping at least 1.
func (g *Game) rescaleHP(i, before int) {
	p := &g.P[i]
	was := p.HP
	p.HP = min(g.maxHP(i), max(1, int(math.Round(float64(p.HP)*float64(g.maxHP(i))/float64(before)))))
	g.hpAdj[i] += p.HP - was // not damage and not healing: the charge does not count it (gainCharge)
}

// stepGigantamax runs Z for Machamp: it starts the transformation when the charge is full and nothing else is going on, and ends it when its time is up.
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
		p.Z != 0 || p.Held || p.FrozenT > 0 || p.StunT > 0
	if !in.Ult || p.Charge < u.Max || busy {
		return nil, false
	}
	p.Charge = 0
	before := g.maxHP(i)
	p.GmaxT = g.cfg.Machamp.Gigantamax.Duration
	g.rescaleHP(i, before)
	return []Event{{T: "gmax", X: p.X, Y: p.Y, R: g.radius(i), K: p.Kind}}, false
}
