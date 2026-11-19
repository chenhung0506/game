package main

import "testing"

// rowletFoe runs a Rowlet computer against a Snorlax that stands where it is (nothing pressed), from the middle of the court, and returns
// the game after secs seconds together with what happened: the HP of the foe at each tick it dropped, when it was first slowed and when the
// tornado first held it.
type rowletLog struct {
	drops    []int // HP lost by the foe in the tick (> 0 only)
	slowedAt float64
	heldAt   float64
	arrows   int // how many arrows were in the air at once, at most
}

func rowletFoe(cfg Config, seed uint64, secs float64) (*Game, rowletLog) {
	g := duel(cfg, rowlet, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 500, 450, 1200, 450
	g.P[1].HP = 100000 // it is only there to be hit
	g.P[1].Z = 0
	b := newBot(rowlet, seed)
	lg := rowletLog{slowedAt: -1, heldAt: -1}
	for t := 0.0; t < secs; t += dt {
		g.P[0].In = b.Decide(g, 0)
		hp := g.P[1].HP
		g.Step(dt)
		if d := hp - g.P[1].HP; d > 0 {
			lg.drops = append(lg.drops, d)
		}
		if lg.slowedAt < 0 && g.P[1].SlowT > 0 {
			lg.slowedAt = t
		}
		if lg.heldAt < 0 && g.P[1].Twirl == twirlSpin {
			lg.heldAt = t
		}
		n := 0
		for _, bl := range g.Balls {
			if bl.K == ballArrow {
				n++
			}
		}
		lg.arrows = max(lg.arrows, n)
	}
	return g, lg
}

func TestTheRowletBotLandsTheWholeVolleyThenSpinsAnOpponentItHasSlowedAndLandsAnotherAsItComesDown(t *testing.T) {
	cfg := liveConfig(t)
	ar := cfg.Rowlet.Arrows
	whole := ar.Damage * ar.Count
	const n = 10
	first, second, held := 0, 0, 0
	for seed := uint64(1); seed <= n; seed++ {
		_, lg := rowletFoe(cfg, seed, 14)
		t.Logf("seed %d: drops %v slowed at %.1f held at %.1f", seed, lg.drops, lg.slowedAt, lg.heldAt)
		var volleys []int // the ticks in which arrows hurt it (the air slashes of the run are smaller, and the tornado's spin is not an arrow's)
		for _, d := range lg.drops {
			if d >= 2*ar.Damage { // more than the tornado's landing (which can be as much as one arrow) or a slash or a spin
				volleys = append(volleys, d)
			}
		}
		if len(volleys) > 0 && volleys[0] == whole {
			first++
		}
		if len(volleys) > 1 && volleys[1] == whole {
			second++
		}
		if lg.heldAt > lg.slowedAt && lg.slowedAt >= 0 {
			held++
		}
	}
	if first < n*8/10 {
		t.Errorf("the whole volley of %d landed at once in only %d of %d first volleys", ar.Count, first, n)
	}
	if held < n*8/10 {
		t.Errorf("the tornado held a slowed opponent in only %d of %d fights", held, n)
	}
	if second < n*8/10 {
		t.Errorf("the whole volley landed as the opponent came down in only %d of %d fights", second, n)
	}
}
