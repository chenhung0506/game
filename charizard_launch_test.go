package main

import "testing"

// A Charizard thrown up by a foe (the Uppercut) comes down by the foe's skill. It used to be lowered by its own flight code too, which counted every time it touched the ground on the
// way as the end of a flight and breathed a ring of fire balls out each time: hundreds of them, over whoever stood next to it.
func TestACharizardThrownUpByTheUppercutBreathesNoRingOfFire(t *testing.T) {
	cfg := liveConfig(t)
	g := duel(cfg, machamp, charizard)
	g.P[0].HP = g.maxHP(0)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 300, 250, 500, 250
	g.P[1].HP = 100000
	hp := g.P[0].HP
	g.P[0].In = Input{Skill: true}
	var launched bool
	for n := 0; n < 200; n++ {
		g.Step(dt)
		g.P[0].In = Input{}
		launched = launched || g.P[1].Juggle
		for _, b := range g.Balls {
			if b.K == ballFire && b.Owner == 1 {
				t.Fatalf("tick %d: a fire ball from a Charizard that was only thrown up", n)
			}
		}
	}
	if !launched {
		t.Fatal("the uppercut never threw it up: this test is about nothing")
	}
	if g.P[0].HP != hp || g.P[1].Z != 0 || g.P[1].Held {
		t.Fatalf("attacker HP %d (was %d), Charizard z=%v held=%v: it should be unhurt and the other down on the ground", g.P[0].HP, hp, g.P[1].Z, g.P[1].Held)
	}
}

func TestACharizardThrownUpInTheMiddleOfAFlightKeepsTheHeightItWasThrownTo(t *testing.T) {
	cfg := liveConfig(t)
	g := duel(cfg, machamp, charizard)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 300, 250, 500, 250
	g.P[1].HP = 100000
	g.P[1].FlyT, g.P[1].Z = 5, cfg.Charizard.Fly.Height
	g.P[0].In = Input{Skill: true}
	var top float64
	for n := 0; n < 120; n++ {
		g.Step(dt)
		g.P[0].In = Input{}
		if g.P[1].Juggle && g.P[1].Z > top {
			top = g.P[1].Z
		}
	}
	if up := cfg.Machamp.Uppercut.LaunchHeight; top < up-1 {
		t.Fatalf("it only got up to %v, want the uppercut's %v: its own flight pulled it back down", top, up)
	}
}

func TestACharizardStillBreathesItsRingWhenItLandsFromAFlight(t *testing.T) {
	cfg := liveConfig(t)
	g := duel(cfg, pikachu, charizard)
	g.P[1].FlyT, g.P[1].Z = 0.1, cfg.Charizard.Fly.Height
	g.P[1].X, g.P[1].Y, g.P[0].X, g.P[0].Y = 1500, 250, 100, 900
	for n := 0; n < 3*tickRate && len(g.Balls) == 0; n++ {
		g.Step(dt)
	}
	if len(g.Balls) != cfg.Charizard.Fly.RingCount {
		t.Fatalf("%d fire balls, want the ring of %d", len(g.Balls), cfg.Charizard.Fly.RingCount)
	}
}
