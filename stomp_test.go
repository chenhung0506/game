package main

import (
	"math"
	"testing"
)

// hopTo is a Jigglypuff (slot 0, at 300,250) with a Pikachu foe `gap` in front of it.
func hopTo(gap float64) *Game {
	g := duel(DefaultConfig(), jigglypuff, pikachu)
	g.P[1].X, g.P[1].Y = g.P[0].X+gap, g.P[0].Y
	return g
}

func TestStompOnTheGroundJumpsTheConfiguredHeightAndLandsWithAShock(t *testing.T) {
	g := hopTo(60)
	hs := g.cfg.Jigglypuff.Stomp
	hp := g.P[1].HP
	press(g, 0, Input{Stomp: true})
	top, ticks := 0.0, 0
	for ; ticks < int(3*tickRate) && (g.P[0].JpFly || ticks < 2); ticks++ {
		g.Step(dt)
		top = math.Max(top, g.P[0].Z)
	}
	if math.Abs(top-hs.Height) > hs.Height*0.1 {
		t.Fatalf("it jumped %v, want about %v", top, hs.Height)
	}
	if got := float64(ticks) / tickRate; math.Abs(got-hs.Duration) > 0.15 {
		t.Fatalf("it was up for %v s, want about %v", got, hs.Duration)
	}
	if g.P[1].HP != hp-hs.Damage {
		t.Fatalf("the landing should hurt the foe beside it: HP %d, want %d", g.P[1].HP, hp-hs.Damage)
	}
	if g.P[0].StompCD <= 0 {
		t.Fatal("the cooldown should be running")
	}
}

func TestStompShockMissesAFoeOutOfReach(t *testing.T) {
	g := hopTo(600)
	hp := g.P[1].HP
	press(g, 0, Input{Stomp: true})
	steps(g, int(2*tickRate))
	if g.P[1].HP != hp {
		t.Fatalf("the shock reached a foe 600 away: HP %d", g.P[1].HP)
	}
}

func TestStompInTheAirDrivesAFoeInTheAirDownAndItBouncesUp(t *testing.T) {
	g := hopTo(30)
	hs := g.cfg.Jigglypuff.Stomp
	g.P[1].Z = 70 // in the air, held there (a Pikachu does not fall by itself in this test)
	hp := g.P[1].HP
	press(g, 0, Input{Stomp: true})
	stomped := false
	for i := 0; i < int(2*tickRate) && !stomped; i++ {
		g.Step(dt)
		stomped = g.P[1].ArcOn
	}
	if !stomped {
		t.Fatal("coming down on a foe in the air should stomp it")
	}
	if g.P[1].HP != hp-hs.FoeDamage {
		t.Fatalf("foe HP %d, want %d", g.P[1].HP, hp-hs.FoeDamage)
	}
	landed, top := false, 0.0
	for i := 0; i < int(4*tickRate); i++ {
		g.Step(dt)
		if g.P[1].Z == 0 {
			landed = true
		}
		if landed && g.P[1].Z > top {
			top = g.P[1].Z
		}
	}
	if math.Abs(top-hs.FoeBounceHeight) > hs.FoeBounceHeight*0.12 {
		t.Fatalf("after hitting the floor the foe bounced to %v, want about %v", top, hs.FoeBounceHeight)
	}
}

func TestStompIsOnePerJumpAndNeedsTheFoeToBeInTheAir(t *testing.T) {
	g := hopTo(30) // the foe stands on the ground: nothing to stomp down
	press(g, 0, Input{Stomp: true})
	steps(g, int(2*tickRate))
	if g.P[1].ArcOn {
		t.Fatal("a foe on the ground is not stomped down")
	}
}

func TestStompCanBeUsedInTheAirAsAFreshJump(t *testing.T) {
	g := hopTo(900)
	g.P[0].Z, g.P[0].JpFly, g.P[0].VZ = 100, true, -50
	press(g, 0, Input{Stomp: true})
	if g.P[0].VZ <= 0 {
		t.Fatalf("the jump in the air should push it up: VZ %v", g.P[0].VZ)
	}
	top := 0.0
	for i := 0; i < int(2*tickRate) && g.P[0].JpFly; i++ {
		g.Step(dt)
		top = math.Max(top, g.P[0].Z)
	}
	if top < 100+g.cfg.Jigglypuff.Stomp.Height*0.8 {
		t.Fatalf("it should rise about %v above where it was, topped at %v", g.cfg.Jigglypuff.Stomp.Height, top)
	}
}

func TestStompEndsAChainAndCannotStartMidSwing(t *testing.T) {
	g := jig(300)
	swing(g, 0)
	press(g, 0, Input{Stomp: true})
	if g.P[0].SlapChain || !g.P[0].JpFly {
		t.Fatalf("jumping out of a chain should end it: chain %v, flying %v", g.P[0].SlapChain, g.P[0].JpFly)
	}
	g2 := jig(300)
	press(g2, 0, Input{Skill: true})
	g2.Step(dt)
	press(g2, 0, Input{Stomp: true})
	if g2.P[0].JpFly {
		t.Fatal("a swing in progress is rooted: no jump")
	}
}

func TestStompConfigIsChecked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Jigglypuff.Stomp.Duration = 0
	if cfg.validate() == nil {
		t.Fatal("a jump of no duration should be refused")
	}
	cfg = DefaultConfig()
	cfg.Jigglypuff.Stomp.FoeBounceHeight = -1
	if cfg.validate() == nil {
		t.Fatal("a negative bounce height should be refused")
	}
}
