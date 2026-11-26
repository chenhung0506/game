package main

import (
	"math"
	"testing"
)

// karpBotScene is a Magikarp bot (slot 0) with a Snorlax (slot 1) `gap` to its right, on a scene that does not change.
// With gya true the Magikarp has already evolved.
func karpBotScene(gap float64, gya bool) (*Bot, *Game) {
	g := duel(DefaultConfig(), magikarp, snorlax)
	g.P[0].X, g.P[0].Y = 600, 500
	g.P[1].X, g.P[1].Y = 600+gap, 500
	if gya {
		g.evolve(0)
		g.P[0].HP = g.maxHP(0)
	}
	return newBot(magikarp, 5), g
}

func TestTheMagikarpBotHopsTowardsItsEvolutionWhenNothingIsComing(t *testing.T) {
	b, g := karpBotScene(550, false)
	in, ok := firstAct(b, g, 0, 30)
	if !ok || !in.Skill {
		t.Fatalf("it should hop when the foe is far and nothing is coming: %+v", in)
	}
}

func TestTheMagikarpBotBacksOffAFoeThatIsClose(t *testing.T) {
	b, g := karpBotScene(200, false)
	g.P[0].HopCD = 3 // no hop to spend: only the stick
	in, _ := firstAct(b, g, 0, 8)
	if in.DX >= 0 {
		t.Fatalf("stick (%d,%d): it has nothing that hurts, so it should back away from a foe on its right", in.DX, in.DY)
	}
}

func TestTheMagikarpBotHopsOverASurfWaveAboutToReachIt(t *testing.T) {
	cfg := DefaultConfig()
	g := duel(cfg, magikarp, magikarp)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 600, 500, 470, 500
	g.evolve(1)
	g.P[1].SurfLeft, g.P[1].SurfX, g.P[1].SurfY = cfg.Gyarados.Wave.Distance, 1, 0 // riding straight at it
	b := newBot(magikarp, 5)
	in, ok := firstAct(b, g, 0, 8)
	if !ok || !in.Skill {
		t.Fatalf("it should hop over a wave that is a moment away: %+v", in)
	}
}

func TestABotWalksSidewaysOutOfASurfLane(t *testing.T) {
	cfg := DefaultConfig()
	g := duel(cfg, pikachu, magikarp)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 600, 500, 300, 500 // in the middle of the lane: the side is the near way out, not its far end
	g.evolve(1)
	g.P[1].SurfT, g.P[1].SurfLeft, g.P[1].SurfX, g.P[1].SurfY = cfg.Gyarados.Wave.SummonDuration, cfg.Gyarados.Wave.Distance, 1, 0
	c := newBot(pikachu, 5)
	c.observe(g, 0)
	th := c.look(g, 0).th
	if th.kind == thNone || math.Abs(th.ay) < 0.9 {
		t.Fatalf("threat %+v: the lane runs along x, so the way out is along y", th)
	}
}

func TestTheGyaradosBotSurfsAtAFoeThatCannotGetOutOfTheLane(t *testing.T) {
	b, g := karpBotScene(400, true)
	g.P[1].StunT = 3
	in, ok := firstAct(b, g, 0, 8)
	if !ok || !in.Skill || in.DX != 0 || in.DY != 0 {
		t.Fatalf("it should surf straight at a stunned foe in reach, with no stick: %+v", in)
	}
}

func TestTheGyaradosBotDoesNotSurfAtAFoeOutOfReach(t *testing.T) {
	b, g := karpBotScene(1300, true)
	for i := 0; i < 60; i++ {
		if in := b.Decide(g, 0); in.Skill {
			t.Fatal("it surfed at a foe further than the wave runs")
		}
	}
}

func TestTheGyaradosBotWhipsANearFoeAtWhereItWillBe(t *testing.T) {
	b, g := karpBotScene(150, true)
	g.P[0].WaveCD = 5
	in, ok := firstAct(b, g, 0, 8)
	if !ok || !in.Tail || in.CastX <= 0 {
		t.Fatalf("it should whip a foe in reach on its right: %+v", in)
	}
}

func TestTheGyaradosBotBeamsAFoeThatCannotMove(t *testing.T) {
	b, g := karpBotScene(250, true)
	g.P[0].WaveCD, g.P[0].TailCD = 5, 5
	g.P[1].StunT = 3
	in, ok := firstAct(b, g, 0, 8)
	if !ok || !in.Beam {
		t.Fatalf("it should beam a stunned foe in reach: %+v", in)
	}
}

func TestTheGyaradosBotPressesNothingWhileItRides(t *testing.T) {
	b, g := karpBotScene(400, true)
	g.P[0].SurfLeft, g.P[0].SurfX = 300, 1
	g.P[1].StunT = 3
	if _, ok := firstAct(b, g, 0, 30); ok {
		t.Fatal("it pressed a key in the middle of a ride")
	}
}

func TestAMagikarpBotEvolvesAndFightsAsAGyaradosAgainstTheLiveNumbers(t *testing.T) {
	cfg := liveConfig(t)
	wins := 0
	for seed := uint64(1); seed <= 4; seed++ {
		g := duel(cfg, magikarp, snorlax)
		g.P[0].X, g.P[1].X = 700, 1300
		bots := [2]*Bot{newBot(magikarp, seed), newBot(snorlax, seed+10)}
		keys := map[string]int{}
		evolved, done := false, false
		for range 120 * 30 {
			for me := range bots {
				in := bots[me].Decide(g, me)
				if me == 0 && g.P[0].Kind == gyarados {
					for name, on := range map[string]bool{"surf": in.Skill, "tail": in.Tail, "beam": in.Beam} {
						if on {
							keys[name]++
						}
					}
				}
				g.P[me].In = in
			}
			g.Step(dt)
			evolved = evolved || g.P[0].Kind == gyarados
			if g.P[0].HP <= 0 || g.P[1].HP <= 0 {
				done = true
				break
			}
		}
		if !evolved {
			t.Fatalf("seed %d: the Magikarp never evolved", seed)
		}
		if !done {
			t.Errorf("seed %d: no winner in two minutes (HP %d vs %d)", seed, g.P[0].HP, g.P[1].HP)
		}
		if keys["surf"] == 0 || keys["tail"] == 0 {
			t.Errorf("seed %d: as a Gyarados it pressed %v: it should surf and whip", seed, keys)
		}
		if g.P[1].HP <= 0 {
			wins++
		}
		t.Logf("seed %d: keys %v, HP %d vs %d", seed, keys, g.P[0].HP, g.P[1].HP)
	}
	t.Logf("the Magikarp bot won %d of 4 against Snorlax", wins)
}
