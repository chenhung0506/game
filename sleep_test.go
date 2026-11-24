package main

import (
	"math"
	"testing"
)

// slGame is Snorlax (slot 0, at 300,250) with a full charge and `hp` HP, with round numbers for Sleep, against a Pikachu (slot 1).
func slGame(hp int, dur float64, heal int) *Game {
	cfg := DefaultConfig()
	cfg.Snorlax.MaxHP = 400 // the default is only 100, which would cap the healing
	cfg.Snorlax.Sleep = SleepCfg{SleepDuration: dur, HealthHP: heal}
	g := duel(cfg, snorlax, pikachu)
	g.P[0].HP = hp
	g.P[0].Charge = cfg.Snorlax.Ultimate.Max
	return g
}

func TestSnorlaxsUltimateNeedsAFullChargeAndPutsItToSleepForSleepDuration(t *testing.T) {
	g := slGame(100, 2, 60)
	g.P[0].Charge = g.cfg.Snorlax.Ultimate.Max - 1
	gpress(g, Input{Ult: true})
	if g.P[0].SleepT != 0 {
		t.Fatal("it fell asleep with the charge not full")
	}
	g.P[0].Charge = g.cfg.Snorlax.Ultimate.Max
	var slept bool
	g.P[0].In = Input{Ult: true}
	for _, e := range g.Step(dt) {
		slept = slept || e.T == "sleep"
	}
	g.P[0].In = Input{}
	if !slept || g.P[0].SleepT <= 0 || g.P[0].Charge >= 1 {
		t.Fatalf("slept %v, SleepT %v, charge %v: the press should start it and empty the charge", slept, g.P[0].SleepT, g.P[0].Charge)
	}
	var woke bool
	for n := 0; n < 200 && g.P[0].SleepT > 0; n++ {
		for _, e := range g.Step(dt) {
			woke = woke || e.T == "wake"
		}
	}
	if !woke || g.P[0].SleepT != 0 {
		t.Fatalf("woke %v, SleepT %v: the sleep should end after its duration", woke, g.P[0].SleepT)
	}
}

func TestTheSleepGivesBackExactlyHealthHPAcrossItsDurationAndNotPastTheMaximum(t *testing.T) {
	g := slGame(100, 2, 60)
	gpress(g, Input{Ult: true})
	prev := g.P[0].HP
	for n := 0; n < 200 && g.P[0].SleepT > 0; n++ {
		g.Step(dt)
		if g.P[0].HP < prev {
			t.Fatalf("HP went down %d -> %d while asleep", prev, g.P[0].HP)
		}
		prev = g.P[0].HP
	}
	if g.P[0].HP != 160 { // 100 + 60 (the tick of the press already gave its share)
		t.Fatalf("HP %d, want 100 + 60", g.P[0].HP)
	}
	cap := slGame(g.cfg.Snorlax.MaxHP-10, 1, 500)
	gpress(cap, Input{Ult: true})
	steps(cap, 60)
	if cap.P[0].HP != cap.maxHP(0) {
		t.Fatalf("HP %d, want the maximum %d", cap.P[0].HP, cap.maxHP(0))
	}
}

func TestAsleepSnorlaxCannotWalkOrUseASkill(t *testing.T) {
	g := slGame(100, 2, 60)
	gpress(g, Input{Ult: true})
	x0 := g.P[0].X
	g.P[0].In = Input{DX: 1, Skill: true, Beam: true, Punch: true}
	steps(g, 20)
	if g.P[0].X != x0 || g.P[0].Z != 0 || g.P[0].JumpCD != 0 || g.P[0].BeamT != 0 || g.P[0].BeamCD != 0 || g.P[0].PunchCD != 0 {
		t.Fatalf("x %v->%v z %v jump cd %v beam %v/%v punch cd %v: asleep it should do nothing", x0, g.P[0].X, g.P[0].Z, g.P[0].JumpCD, g.P[0].BeamT, g.P[0].BeamCD, g.P[0].PunchCD)
	}
	g.P[0].In = Input{}
	steps(g, 80)
	g.P[0].In = Input{DX: 1}
	steps(g, 10)
	if g.P[0].X <= x0 {
		t.Fatal("it never got up and walked after the sleep")
	}
}

func TestNothingDoneToASleepingSnorlaxEndsTheSleepOrTheHealing(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(g *Game)
	}{
		{"stun", func(g *Game) { g.stun(&g.P[0], 5) }},
		{"freeze", func(g *Game) { g.freeze(&g.P[0], 5) }},
		{"held", func(g *Game) { g.P[0].Held = true }},
		{"bound", func(g *Game) { g.P[0].BoundT = 5 }},
		{"paralysis", func(g *Game) { g.P[0].ParalyzeT = 5 }},
		{"silence", func(g *Game) { g.P[0].SilentT = 5 }},
		{"thrown up", func(g *Game) { g.launch(&g.P[0], 200, 3) }},
		{"knocked back", func(g *Game) { g.knockBack(g.P[0].X-50, g.P[0].Y, &g.P[0], 150, 1, 0) }},
	} {
		g := slGame(100, 2, 60)
		gpress(g, Input{Ult: true})
		t0, hp0 := g.P[0].SleepT, g.P[0].HP
		c.do(g)
		steps(g, 12)
		if g.P[0].SleepT <= 0 || g.P[0].SleepT >= t0 || g.P[0].HP <= hp0 {
			t.Fatalf("%s: SleepT %v -> %v, HP %d -> %d: the sleep and the healing should go on", c.name, t0, g.P[0].SleepT, hp0, g.P[0].HP)
		}
		g.P[0].Held, g.P[0].Juggle = false, false
		steps(g, 120)
		if g.P[0].SleepT != 0 || g.P[0].HP != 160 {
			t.Fatalf("%s: SleepT %v HP %d at the end, want it over and HP 160", c.name, g.P[0].SleepT, g.P[0].HP)
		}
	}
}

func TestSnorlaxDoesNotFallAsleepInTheMiddleOfAnotherSkillOrWhileStunned(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(p *Player)
	}{
		{"in the air", func(p *Player) { p.Z = 30 }},
		{"charging the beam", func(p *Player) { p.BeamT = 1 }},
		{"recovering from the beam", func(p *Player) { p.RechargeT = 1 }},
		{"stunned", func(p *Player) { p.StunT = 1 }},
		{"frozen", func(p *Player) { p.FrozenT = 1 }},
	} {
		g := slGame(100, 2, 60)
		c.set(&g.P[0])
		gpress(g, Input{Ult: true})
		if g.P[0].SleepT != 0 || g.P[0].Charge == 0 {
			t.Fatalf("%s: it went to sleep (SleepT %v, charge %v)", c.name, g.P[0].SleepT, g.P[0].Charge)
		}
	}
}

func TestTheHealingOfTheSleepIsNotDamageAndFillsNoUltimate(t *testing.T) {
	g := slGame(100, 2, 60)
	g.P[1].Charge = 0
	gpress(g, Input{Ult: true})
	steps(g, 90)
	if math.Abs(g.P[1].Charge-g.cfg.Pikachu.Ultimate.GainPerSecond*91*dt) > 1e-6 {
		t.Fatalf("the foe's charge is %v: only time should have filled it, not Snorlax getting better", g.P[1].Charge)
	}
}

func TestTheSleepConfigIsChecked(t *testing.T) {
	for _, c := range []struct {
		name string
		mut  func(c *Config)
	}{
		{"no sleep time", func(c *Config) { c.Snorlax.Sleep.SleepDuration = 0 }},
		{"a delay as long as the sleep", func(c *Config) { c.Snorlax.Sleep.Delay = c.Snorlax.Sleep.SleepDuration }},
		{"a negative delay", func(c *Config) { c.Snorlax.Sleep.Delay = -1 }},
		{"a negative gathering", func(c *Config) { c.Snorlax.Sleep.RingDuration = -1 }},
		{"negative healing", func(c *Config) { c.Snorlax.Sleep.HealthHP = -1 }},
		{"no charge to fill", func(c *Config) { c.Snorlax.Ultimate.Max = 0 }},
	} {
		cfg := DefaultConfig()
		c.mut(&cfg)
		if err := cfg.validate(); err == nil {
			t.Errorf("%s: the config was accepted", c.name)
		}
	}
}

func TestTheSleepGathersForRingDurationRootedBeforeItBeginsAndAStunEndsTheGatheringOnly(t *testing.T) {
	g := slGame(100, 2, 60)
	g.cfg.Snorlax.Sleep.RingDuration = 0.5
	gpress(g, Input{Ult: true})
	if g.P[0].UltT <= 0 || g.P[0].SleepT != 0 || g.P[0].Charge != 0 {
		t.Fatalf("gathering %v, sleep %v, charge %v: the press should start the rings, not the sleep", g.P[0].UltT, g.P[0].SleepT, g.P[0].Charge)
	}
	x0 := g.P[0].X
	g.P[0].In = Input{DX: 1}
	steps(g, 5)
	if g.P[0].X != x0 || g.P[0].HP != 100 {
		t.Fatalf("x %v->%v HP %d: it is rooted and not healing while it gathers", x0, g.P[0].X, g.P[0].HP)
	}
	g.P[0].In = Input{}
	var slept bool
	for n := 0; n < 40 && g.P[0].SleepT == 0; n++ {
		for _, e := range g.Step(dt) {
			slept = slept || e.T == "sleep"
		}
	}
	if !slept || g.P[0].UltT != 0 || g.P[0].SleepT <= 0 {
		t.Fatalf("slept %v, gathering %v, sleep %v: the sleep should begin when the rings are done", slept, g.P[0].UltT, g.P[0].SleepT)
	}
	s := slGame(100, 2, 60)
	s.cfg.Snorlax.Sleep.RingDuration = 0.5
	gpress(s, Input{Ult: true})
	s.stun(&s.P[0], 1)
	steps(s, 30)
	if s.P[0].SleepT != 0 || s.P[0].UltT != 0 || s.P[0].Charge >= 1 {
		t.Fatalf("a stunned gathering: sleep %v, gathering %v, charge %v: it should end with no sleep and no charge back", s.P[0].SleepT, s.P[0].UltT, s.P[0].Charge)
	}
}

func TestTheHealingWaitsForDelayThenGivesBackExactlyHealthHPOverTheRestOfTheSleep(t *testing.T) {
	g := slGame(100, 2, 60)
	g.cfg.Snorlax.Sleep.Delay = 0.5
	gpress(g, Input{Ult: true})
	for n := 0; n < 200 && g.P[0].SleepT > 2-0.5+dt+1e-9; n++ { // every step that ends inside the delay
		g.Step(dt)
		if g.P[0].HP != 100 {
			t.Fatalf("HP %d during the delay (%v s of sleep gone): it should not have begun to heal", g.P[0].HP, 2-g.P[0].SleepT)
		}
	}
	steps(g, 5)
	if g.P[0].HP <= 100 {
		t.Fatalf("HP %d: the healing should have begun by now", g.P[0].HP)
	}
	steps(g, 100)
	if g.P[0].SleepT != 0 || g.P[0].HP != 160 {
		t.Fatalf("SleepT %v HP %d, want it over and exactly 60 back", g.P[0].SleepT, g.P[0].HP)
	}
}
