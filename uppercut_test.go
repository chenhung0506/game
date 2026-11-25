package main

import (
	"math"
	"testing"
)

// The Uppercut is Machamp's main skill: a slide along the stick, then a rush at the opponent that throws it up (it is not pushed away).

func TestTheUppercutSlidesThenRushesAndThrowsTheFoeUpWithoutPushingIt(t *testing.T) {
	cfg := DefaultConfig()
	uc := &cfg.Machamp.Uppercut
	uc.SlideDistance, uc.SlideDuration, uc.LaunchHeight, uc.LaunchDuration, uc.Damage = 100, 0.2, 150, 1, 25
	g := kickDuel(cfg, 500)
	g.P[1].Y = 250 // on the line the rush will take (the slide is along it too)
	hp0, x0, y0 := g.P[1].HP, g.P[1].X, g.P[1].Y
	press(g, 0, Input{Skill: true, DX: 1})
	if g.P[0].UpPhase != upSlide {
		t.Fatalf("phase %d after the press, want the slide", g.P[0].UpPhase)
	}
	var hit bool
	for n := 0; n < 90 && g.P[0].UpPhase != upNone; n++ {
		for _, e := range g.Step(dt) {
			hit = hit || e.T == "uphit"
		}
	}
	if !hit || g.P[0].UpPhase != upNone {
		t.Fatalf("hit %v, phase %d: the rush never landed", hit, g.P[0].UpPhase)
	}
	if g.P[1].HP != hp0-uc.Damage {
		t.Fatalf("foe HP %d, want %d", g.P[1].HP, hp0-uc.Damage)
	}
	if g.P[1].X != x0 || g.P[1].Y != y0 {
		t.Fatalf("the foe was pushed to %v,%v", g.P[1].X, g.P[1].Y)
	}
	steps(g, 20)
	if g.P[1].Z < uc.LaunchHeight-1e-6 || !g.P[1].Held {
		t.Fatalf("foe Z %v held %v: it should be thrown up to %v", g.P[1].Z, g.P[1].Held, uc.LaunchHeight)
	}
	steps(g, int(uc.LaunchDuration/dt)+20)
	if g.P[1].Z != 0 || g.P[1].Held || g.P[1].Juggle {
		t.Fatalf("foe Z %v held %v: it should be down and free", g.P[1].Z, g.P[1].Held)
	}
}

func TestTheUppercutSlidesTheConfiguredDistanceAlongTheStickFirst(t *testing.T) {
	cfg := DefaultConfig()
	uc := &cfg.Machamp.Uppercut
	uc.SlideDistance, uc.SlideDuration = 120, 0.3
	g := kickDuel(cfg, 1500)
	g.P[1].Y = 900 // far off the line: this is about the slide
	press(g, 0, Input{Skill: true, DY: 1})
	for g.P[0].UpPhase == upSlide {
		g.P[0].In = Input{DX: -1} // the stick moves on: the slide keeps its way
		g.Step(dt)
	}
	if got := g.P[0].Y - 250; math.Abs(got-uc.SlideDistance) > 1e-6 || g.P[0].X != 300 {
		t.Fatalf("slid %v along y and to x=%v, want %v and 300", got, g.P[0].X, uc.SlideDistance)
	}
	if g.P[0].UpPhase != upRush {
		t.Fatalf("phase %d, want the rush", g.P[0].UpPhase)
	}
}

func TestAnUppercutWithNoSlideRushesAtOnceAtTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	uc := &cfg.Machamp.Uppercut
	uc.SlideDistance, uc.SlideDuration, uc.RushDelay = 0, 0, 0
	g := kickDuel(cfg, 700)
	g.P[1].Y = 450
	press(g, 0, Input{Skill: true, DY: 1}) // the stick is held, but there is nothing to slide with
	if g.P[0].X != 300 || g.P[0].Y != 250 || g.P[0].UpPhase != upRush {
		t.Fatalf("at %v,%v phase %d, want it on the spot and rushing", g.P[0].X, g.P[0].Y, g.P[0].UpPhase)
	}
	wx, wy := unit(g.P[1].X-300, g.P[1].Y-250)
	if math.Abs(g.P[0].UpX-wx) > 1e-9 || math.Abs(g.P[0].UpY-wy) > 1e-9 {
		t.Fatalf("rush aimed %v,%v, want it at the opponent %v,%v", g.P[0].UpX, g.P[0].UpY, wx, wy)
	}
}

func TestTheUppercutRushMissesAFoeThatStepsOutOfTheLine(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.Uppercut.SlideDistance, cfg.Machamp.Uppercut.SlideDuration = 0, 0
	g := kickDuel(cfg, 400)
	press(g, 0, Input{Skill: true})
	g.P[1].Y = 700 // it has stepped aside once the rush was aimed
	steps(g, 60)
	if g.P[1].HP != g.maxHP(1) || g.P[0].UpPhase != upNone || g.P[1].Held {
		t.Fatalf("foe HP %d held %v, phase %d: the rush should have gone by", g.P[1].HP, g.P[1].Held, g.P[0].UpPhase)
	}
	if want := 300 + cfg.Machamp.Uppercut.RushDistance; math.Abs(g.P[0].X-want) > 1e-6 {
		t.Fatalf("rushed to %v, want its whole distance, to %v", g.P[0].X, want)
	}
}

func TestTheUppercutHasItsCooldownAndAHeightBandAndNeedsNoTarget(t *testing.T) {
	cfg := DefaultConfig()
	g := kickDuel(cfg, 1500)
	g.P[1].Y = 900
	press(g, 0, Input{Skill: true})
	if g.P[0].UpPhase == upNone || g.P[0].UpCD <= 0 {
		t.Fatal("it did not start")
	}
	for g.P[0].UpPhase != upNone {
		g.Step(dt)
	}
	press(g, 0, Input{Skill: true})
	if g.P[0].UpPhase != upNone {
		t.Fatal("a second one started during the cooldown")
	}
	g.P[0].UpCD = 0
	press(g, 0, Input{Skill: true})
	if g.P[0].UpPhase == upNone {
		t.Fatal("it would not start once the cooldown was over")
	}
	cfg2 := cfg // a foe above its band is passed under: Rowlet flies at fly_high
	cfg2.Machamp.Uppercut.MaxHeight = 10
	h := duel(cfg2, machamp, rowlet)
	h.P[1].X, h.P[1].Y = 600, 250
	press(h, 0, Input{Skill: true})
	steps(h, 60)
	if h.P[1].HP != h.maxHP(1) {
		t.Fatal("hit something above its band")
	}
}

func TestAMachampPersonsKeysAreTheUppercutOnTheMainButtonAndTheKickOnK(t *testing.T) {
	in := personsInput(machamp, Input{Skill: true, Rock: true})
	if !in.Skill || !in.Kick || in.Rock {
		t.Fatalf("Skill %v Kick %v Rock %v: the main button is the uppercut and K the kick, and the Rock Tomb has no key", in.Skill, in.Kick, in.Rock)
	}
	if in := personsInput(machamp, Input{Kick: true}); in.Kick {
		t.Fatal("a person's own Kick key got through")
	}
}

func TestAMachampThatKicksDoesNotStartTheUppercutAndTheRockTombIsNeverStarted(t *testing.T) {
	cfg := DefaultConfig()
	g := kickDuel(cfg, 1500)
	g.P[1].Y = 900
	press(g, 0, Input{Kick: true})
	if g.P[0].KickPhase == kickNone || g.P[0].UpPhase != upNone {
		t.Fatalf("kick phase %d, uppercut phase %d", g.P[0].KickPhase, g.P[0].UpPhase)
	}
	cfg.Machamp.RockTomb.Enabled = false
	r := kickDuel(cfg, 1500)
	press(r, 0, Input{Rock: true})
	if r.P[0].RockT != 0 || r.P[0].RockCD != 0 {
		t.Fatal("the Rock Tomb started although it is off")
	}
	cfg.Machamp.RockTomb.Enabled = true
	r = kickDuel(cfg, 1500)
	press(r, 0, Input{Rock: true})
	if r.P[0].RockT != 0 {
		t.Fatal("Machamp started a Rock Tomb: it has none, whatever the config says")
	}
}

func TestTheMachampBotUsesTheUppercutAtAMiddlingDistanceAndNeverTheRockTombWhenItIsOff(t *testing.T) {
	cfg := liveConfig(t)
	cfg.Machamp.RockTomb.Enabled = false
	g := duel(cfg, machamp, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 600, 400, 600+300, 400
	b := newBot(machamp, 3)
	var upper, rock bool
	for n := 0; n < 400; n++ {
		in := b.Decide(g, 0)
		upper, rock = upper || (in.Skill && !in.Kick), rock || in.Rock
		g.P[0].In = in
		g.Step(dt)
		g.P[0].UpCD, g.P[0].KickCD, g.P[0].CometCD = 0, 99, 99 // the comet punch and the kick are not what is looked for
		g.P[1].X, g.P[1].Y, g.P[1].HP = 600+300, 400, 100000
	}
	if !upper || rock {
		t.Fatalf("uppercut %v rock %v", upper, rock)
	}
}
