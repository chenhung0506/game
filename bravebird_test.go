package main

import (
	"math"
	"testing"
)

// bbGame is Rowlet (slot 0, at 300,250) against a Snorlax `d` ahead of it, Rowlet with a full charge.
func bbGame(cfg Config, d float64) *Game {
	g := duel(cfg, rowlet, snorlax)
	g.P[1].X, g.P[1].Y = 300+d, 250
	g.P[1].HP = 100000
	g.P[0].Charge = cfg.Rowlet.Ultimate.Max
	return g
}

func TestBraveBirdNeedsAFullChargeAndEmptiesIt(t *testing.T) {
	cfg := DefaultConfig()
	g := bbGame(cfg, 400)
	g.P[0].Charge = cfg.Rowlet.Ultimate.Max - 1
	gpress(g, Input{Ult: true})
	if g.P[0].UltT != 0 {
		t.Fatal("it started with the charge not full")
	}
	g.P[0].Charge = cfg.Rowlet.Ultimate.Max
	gpress(g, Input{Ult: true})
	if g.P[0].UltT <= 0 || g.P[0].Charge >= 1 {
		t.Fatalf("UltT %v charge %v: the press did not start it and empty the charge", g.P[0].UltT, g.P[0].Charge)
	}
}

func TestBraveBirdClimbsDuringTheChargeThenDivesAtTheFoeAndBurstsThrowingItUpWithoutAPush(t *testing.T) {
	cfg := DefaultConfig()
	bb := &cfg.Rowlet.BraveBird
	bb.RingDuration, bb.ChargeDuration, bb.SkyHeight, bb.DiveSpeed = 0.3, 0.6, 300, 1000
	bb.ExplosionRadius, bb.Damage, bb.LaunchHeight, bb.LaunchDuration = 120, 25, 150, 1
	bb.Tornado.Duration = 0 // no tornado to catch the foe as it comes down (the next test)
	g := bbGame(cfg, 400)
	gpress(g, Input{Ult: true})
	x0 := g.P[0].X
	steps(g, int((bb.RingDuration+bb.ChargeDuration)/dt)-3)
	if z := g.P[0].Z; z <= cfg.Rowlet.FlyHigh || z > bb.SkyHeight || g.P[0].X != x0 || g.P[0].BirdDive {
		t.Fatalf("Z %v x %v dive %v: it should be climbing in place", z, g.P[0].X, g.P[0].BirdDive)
	}
	steps(g, 6)
	if !g.P[0].BirdDive {
		t.Fatal("the charge is over and it is not diving")
	}
	hp0, fx, fy := g.P[1].HP, g.P[1].X, g.P[1].Y
	var boom bool
	for n := 0; n < 90 && g.P[0].BirdDive; n++ {
		for _, e := range g.Step(dt) {
			boom = boom || e.T == "bbboom"
		}
	}
	if !boom || g.P[0].BirdDive {
		t.Fatalf("boom %v, dive %v: the dive never landed", boom, g.P[0].BirdDive)
	}
	if g.P[1].HP != hp0-bb.Damage {
		t.Fatalf("foe HP %d, want %d", g.P[1].HP, hp0-bb.Damage)
	}
	if g.P[1].X != fx || g.P[1].Y != fy {
		t.Fatal("the burst pushed the foe")
	}
	steps(g, 20)
	if g.P[1].Z < bb.LaunchHeight-1e-6 || !g.P[1].Held {
		t.Fatalf("foe Z %v held %v: it should be up in the air", g.P[1].Z, g.P[1].Held)
	}
	steps(g, int(bb.LaunchDuration/dt)+20)
	if g.P[1].Z != 0 || g.P[1].Held || g.P[1].Juggle {
		t.Fatalf("foe Z %v held %v: it should be back down and free", g.P[1].Z, g.P[1].Held)
	}
}

func TestBraveBirdKeepsTheFoeUpForLaunchDurationAndRowletBackInTheAir(t *testing.T) {
	cfg := DefaultConfig()
	bb := &cfg.Rowlet.BraveBird
	bb.RingDuration, bb.ChargeDuration, bb.LaunchDuration = 0.1, 0.1, 2
	g := bbGame(cfg, 300)
	gpress(g, Input{Ult: true})
	for n := 0; n < 300 && g.P[1].Z == 0; n++ {
		g.Step(dt)
	}
	steps(g, int(1.5/dt)) // 1.5 s later it is still held up there (the rise takes 0.4 s, then 2 s of hanging)
	if g.P[1].Z < bb.LaunchHeight-1e-6 {
		t.Fatalf("foe Z %v: it came down early", g.P[1].Z)
	}
	if g.P[0].Z < 0 || g.P[0].BirdDive {
		t.Fatalf("Rowlet Z %v dive %v", g.P[0].Z, g.P[0].BirdDive)
	}
	steps(g, 30)
	if g.P[0].Z != cfg.Rowlet.FlyHigh {
		t.Fatalf("Rowlet Z %v, want it back at fly_high %v", g.P[0].Z, cfg.Rowlet.FlyHigh)
	}
}

func TestBraveBirdCannotBeStunnedOnceTheChargeBeginsAndIsBrokenDuringTheRings(t *testing.T) {
	cfg := DefaultConfig()
	g := bbGame(cfg, 400)
	gpress(g, Input{Ult: true})
	g.P[0].UltT = cfg.Rowlet.BraveBird.RingDuration + cfg.Rowlet.BraveBird.ChargeDuration // the rings: not yet safe
	if !g.stun(&g.P[0], 1) {
		t.Fatal("the rings are the part that can be broken")
	}
	g.P[0].StunT = 0
	g.P[0].UltT = cfg.Rowlet.BraveBird.ChargeDuration / 2
	if g.stun(&g.P[0], 1) {
		t.Fatal("a charging Rowlet was stunned")
	}
	g.P[0].UltT, g.P[0].BirdDive = 0, true
	if g.freeze(&g.P[0], 1) {
		t.Fatal("a diving Rowlet was frozen")
	}
}

func TestTheRowletBotCastsBraveBirdOnlyOnceItIsCharged(t *testing.T) {
	cfg := liveConfig(t)
	for _, full := range []bool{false, true} {
		g := bbGame(cfg, 500)
		if !full {
			g.P[0].Charge = 0
		}
		bot := newBot(rowlet, 1)
		cast := false
		for n := 0; n < 3*tickRate && !cast; n++ {
			in := bot.Decide(g, 0)
			cast = in.Ult
			g.P[0].In = in
			g.Step(dt)
		}
		if cast != full {
			t.Fatalf("charged %v: the bot cast = %v", full, cast)
		}
	}
}

func TestBraveBirdLeavesAStillTornadoOfItsOwnSizeThatCatchesWhoeverComesNear(t *testing.T) {
	cfg := DefaultConfig()
	bb := &cfg.Rowlet.BraveBird
	bb.RingDuration, bb.ChargeDuration = 0.1, 0.1
	bb.Tornado.Radius, bb.Tornado.Scale, bb.Tornado.Duration = 70, 2, 4
	bb.LockDistance = 1e6 // the way is fixed from the start: the foe steps aside once it is aimed
	bb.LaunchHeight = 0   // the foe is not thrown, so only the tornado can hold it
	g := bbGame(cfg, 400)
	gpress(g, Input{Ult: true})
	for n := 0; n < 300 && !g.P[0].BirdDive; n++ {
		g.Step(dt)
	}
	g.P[1].X, g.P[1].Y = 1500, 700 // it stepped aside once the dive was aimed
	for n := 0; n < 90 && g.P[0].BirdDive; n++ {
		g.Step(dt)
	}
	var tn *Ball
	for i := range g.Balls {
		if g.Balls[i].K == ballTornado {
			tn = &g.Balls[i]
		}
	}
	if tn == nil || tn.Rad != 70 || tn.Scale != 2 || !tn.Still {
		t.Fatalf("tornado %+v: want a still one of reach 70 and scale 2", tn)
	}
	x, y := tn.X, tn.Y
	steps(g, 60)
	for _, b := range g.Balls {
		if b.K == ballTornado && (b.X != x || b.Y != y || b.Caught) {
			t.Fatalf("the tornado moved to %v,%v or caught the foe (%v) while it was far", b.X, b.Y, b.Caught)
		}
	}
	g.P[1].X, g.P[1].Y = x+50, y // inside 70 + its body
	steps(g, 3)
	if g.P[1].Twirl != twirlSpin || !g.P[1].Held {
		t.Fatalf("Twirl %d held %v: the tornado did not take the foe that came near", g.P[1].Twirl, g.P[1].Held)
	}
}

func TestBraveBirdsTornadoTakesAFoeHangingInTheAirAndLiftsItHigherButOnlyWithinItsOwnHeightBand(t *testing.T) {
	for _, minH := range []float64{0, 450} { // a band that starts above where the foe is thrown (400) cannot take it
		cfg := DefaultConfig()
		bb := &cfg.Rowlet.BraveBird
		bb.RingDuration, bb.ChargeDuration, bb.LaunchHeight, bb.LaunchDuration = 0.1, 0.1, 400, 3
		bb.Tornado.Radius, bb.Tornado.Duration, bb.Tornado.LiftHeight, bb.Tornado.MinHeight, bb.Tornado.MaxHeight = 120, 6, 600, minH, 1000
		g := bbGame(cfg, 300)
		gpress(g, Input{Ult: true})
		var spun bool
		var top, zAtCatch float64
		for n := 0; n < 240; n++ {
			g.Step(dt)
			if !spun && g.P[1].Twirl == twirlSpin {
				spun, zAtCatch = true, g.P[1].Z
			}
			top = max(top, g.P[1].Z)
		}
		if minH == 0 && (!spun || top < 599 || zAtCatch < 399) {
			t.Fatalf("band from %v: spun %v at %v, highest %v: the tornado should have taken the foe up at its launch height of 400 and lifted it to 600", minH, spun, zAtCatch, top)
		}
		if minH == 450 && (spun || top > 401) {
			t.Fatalf("band from %v: spun %v, highest %v: the foe at 400 is out of the tornado's band", minH, spun, top)
		}
	}
}

func TestBraveBirdPutsTheTornadoAndArrowsSkillsIntoTheirCooldownWhenItLands(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rowlet.BraveBird.RingDuration, cfg.Rowlet.BraveBird.ChargeDuration = 0.1, 0.1
	g := bbGame(cfg, 300)
	gpress(g, Input{Ult: true})
	for n := 0; n < 300 && !g.P[0].BirdDive; n++ {
		g.Step(dt)
		if g.P[0].TwistCD != 0 || g.P[0].ArrowCD != 0 {
			t.Fatalf("the tornado (%v) or the arrows (%v) went into their cooldown before the bird landed", g.P[0].TwistCD, g.P[0].ArrowCD)
		}
	}
	for n := 0; n < 90 && g.P[0].BirdDive; n++ {
		g.Step(dt)
	}
	if want := cfg.Rowlet.Tornado.Cooldown; g.P[0].TwistCD < want-2*dt {
		t.Fatalf("tornado cooldown %v after the landing, want about %v", g.P[0].TwistCD, want)
	}
	if want := cfg.Rowlet.Arrows.Cooldown; g.P[0].ArrowCD < want-2*dt {
		t.Fatalf("arrows cooldown %v after the landing, want about %v", g.P[0].ArrowCD, want)
	}
}

func TestTheDiveFollowsTheFoeUntilItIsLockDistanceAwayAndNotAfter(t *testing.T) {
	for name, tc := range map[string]struct {
		lock    float64
		moveAt  float64 // the foe steps aside when the bird is this far (along the way) from it
		wantHit bool
	}{
		"steps aside early, the dive follows":         {lock: 400, moveAt: 1200, wantHit: true},
		"steps aside late, the way is fixed":          {lock: 400, moveAt: 300, wantHit: false},
		"fixed from the start, however early it goes": {lock: 5000, moveAt: 1200, wantHit: false},
	} {
		cfg := DefaultConfig()
		bb := &cfg.Rowlet.BraveBird
		bb.RingDuration, bb.ChargeDuration, bb.SkyHeight, bb.DiveSpeed, bb.LockDistance = 0.1, 0.1, 300, 800, tc.lock
		bb.ExplosionRadius, bb.LaunchHeight = 120, 0
		bb.Tornado.Duration = 0
		g := bbGame(cfg, 1400)
		gpress(g, Input{Ult: true})
		for n := 0; n < 300 && !g.P[0].BirdDive; n++ {
			g.Step(dt)
		}
		hp0, moved := g.P[1].HP, false
		for n := 0; n < 200 && g.P[0].BirdDive; n++ {
			if left := math.Hypot(math.Hypot(g.P[1].X-g.P[0].X, g.P[1].Y-g.P[0].Y), g.P[0].Z); !moved && left <= tc.moveAt {
				g.P[1].Y += 500 // out of the explosion's reach from where it was
				moved = true
			}
			g.Step(dt)
		}
		if !moved {
			t.Fatalf("%s: the foe never stepped aside", name)
		}
		if hit := g.P[1].HP < hp0; hit != tc.wantHit {
			t.Fatalf("%s: hit %v, want %v", name, hit, tc.wantHit)
		}
	}
}
