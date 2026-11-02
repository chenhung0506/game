package main

import (
	"math"
	"testing"
)

// megaDuel is Ash (slot 0, at 300,250) against a Snorlax (slot 1) `d` ahead of it on the same line.
func megaDuel(cfg Config, d float64) *Game {
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+d, 250
	return g
}

func TestMegatonPunchGathersRootedThenHurtsStunsAndKnocksBackWhatItReaches(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := megaDuel(cfg, 300)
	press(g, 0, Input{Mega: true, DX: 1}) // the stick does not move it: it is rooted
	n := 1
	for g.P[0].MegaT > 0 {
		if g.P[1].HP != g.maxHP(1) || g.P[1].StunT > 0 {
			t.Fatal("it hit before the charge was over")
		}
		if g.P[0].X != 300 {
			t.Fatalf("Ash walked while it gathered: x = %v", g.P[0].X)
		}
		g.P[0].In = Input{DX: 1}
		g.Step(dt)
		n++
	}
	if want := int(math.Ceil(mp.ChargeDuration / dt)); n != want {
		t.Fatalf("charge took %d ticks, want %d", n, want)
	}
	if g.P[1].HP != g.maxHP(1)-mp.Damage {
		t.Fatalf("HP %d, want %d", g.P[1].HP, g.maxHP(1)-mp.Damage)
	}
	if math.Abs(g.P[1].StunT-mp.StunDuration) > 2*dt {
		t.Fatalf("stun %v, want %v", g.P[1].StunT, mp.StunDuration)
	}
	if got := g.P[1].X - 600; math.Abs(got-mp.Knockback) > 1e-6 {
		t.Fatalf("knocked back %v, want %v", got, mp.Knockback)
	}
	x := g.P[1].X
	for g.P[1].StunT > 0 { // stunned: deaf to its keys
		g.P[1].In = Input{DX: 1}
		g.Step(dt)
		if g.P[1].X != x {
			t.Fatal("a stunned Snorlax walked")
		}
	}
}

func TestMegatonPunchFliesTheLineLockedAtTheStartSoSteppingAsideDodgesIt(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := megaDuel(cfg, 300)
	press(g, 0, Input{Mega: true})
	g.P[1].Y = 250 + mp.Radius + g.radius(1) + 10 // out of the line
	steps(g, int(mp.ChargeDuration/dt)+2)
	if g.P[1].HP != g.maxHP(1) || g.P[1].StunT > 0 {
		t.Fatal("hit a body that was beside the line")
	}
	g = megaDuel(cfg, mp.Distance+g.radius(1)+mp.Radius+20)
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("hit a body beyond its reach")
	}
}

func TestMegatonPunchHasItsHeightBandAndCooldownAndBelongsToAshAlone(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := flyer(cfg, mp.MaxHeight+30)
	g.P[1].X, g.P[1].Y = 600, 250
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("hit something above its band")
	}
	g.P[0].MegaT = 0
	press(g, 0, Input{Mega: true})
	if g.P[0].MegaT != 0 {
		t.Fatal("a second punch started during the cooldown")
	}
	g.P[0].MegaCD = 0
	press(g, 0, Input{Mega: true})
	if g.P[0].MegaT == 0 {
		t.Fatal("it would not start once the cooldown was over")
	}

	m := megaDuel(cfg, 300)
	m.SetKinds(machamp, snorlax)
	press(m, 0, Input{Mega: true})
	if m.P[0].MegaT != 0 || m.P[0].MegaCD != 0 {
		t.Fatal("Machamp threw the Megaton Punch")
	}
}

func TestBeingStunnedCutsTheMegatonChargeShort(t *testing.T) {
	g := megaDuel(DefaultConfig(), 300)
	press(g, 0, Input{Mega: true})
	g.P[0].StunT = 0.5
	steps(g, 3)
	if g.P[0].MegaT != 0 || g.P[1].HP != g.maxHP(1) {
		t.Fatal("the punch went on through a stun")
	}
}

func TestAnyOtherSkillWaitsWhileTheMegatonPunchGathers(t *testing.T) {
	g := megaDuel(DefaultConfig(), 300)
	press(g, 0, Input{Mega: true})
	press(g, 0, Input{Skill: true, Comet: true, Rock: true})
	if g.P[0].TreePhase != treeNone || g.P[0].CometT != 0 || g.P[0].RockT != 0 {
		t.Fatal("another skill started in the middle of the charge")
	}
}

func TestMegatonPunchWithNoChargeLandsOnTheTickItIsPressed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.ChargeDuration = 0
	g := megaDuel(cfg, 300)
	press(g, 0, Input{Mega: true})
	if g.P[1].HP != g.maxHP(1)-cfg.Ash.MegatonPunch.Damage {
		t.Fatal("no immediate hit")
	}
}

func TestTheTreeStunsWhatItLandsOnForTheConfiguredTime(t *testing.T) {
	for _, kind := range []int{ash, machamp} {
		cfg := DefaultConfig()
		if kind == ash {
			cfg.Ash.TreeSlam.StunDuration = 1.7
		} else {
			cfg.Machamp.TreeSlam.StunDuration = 1.7
		}
		ts := cfg.brawlerOf(kind).TreeSlam
		g := duel(cfg, kind, snorlax)
		g.P[1].X, g.P[1].Y = 300+ts.JumpDistance+150, 250 // on the trunk once it has landed
		press(g, 0, Input{Skill: true, DX: 1})
		for g.P[0].TreePhase != treeNone {
			if g.P[1].StunT > 0 {
				t.Fatal("stunned before the tree came down")
			}
			g.Step(dt)
		}
		if g.P[1].HP != g.maxHP(1)-ts.Damage || math.Abs(g.P[1].StunT-1.7) > 2*dt {
			t.Fatalf("kind %d: HP %d stun %v", kind, g.P[1].HP, g.P[1].StunT)
		}
	}
}

func TestATreeThatMissesDoesNotStun(t *testing.T) {
	g := duel(DefaultConfig(), ash, snorlax)
	g.P[1].X, g.P[1].Y = 1800, 900
	press(g, 0, Input{Skill: true, DX: 1})
	steps(g, 90)
	if g.P[1].StunT != 0 {
		t.Fatal("stunned by a tree that missed")
	}
}

func TestTheConfigRefusesNonsenseMegatonAndStunNumbers(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"distance":    func(c *Config) { c.Ash.MegatonPunch.Distance = 0 },
		"radius":      func(c *Config) { c.Ash.MegatonPunch.Radius = 0 },
		"stun":        func(c *Config) { c.Ash.MegatonPunch.StunDuration = -1 },
		"charge":      func(c *Config) { c.Ash.MegatonPunch.ChargeDuration = -1 },
		"knockback":   func(c *Config) { c.Ash.MegatonPunch.Knockback = -1 },
		"fist_scale":  func(c *Config) { c.Ash.MegatonPunch.FistScale = 0 },
		"tree stun":   func(c *Config) { c.Ash.TreeSlam.StunDuration = -1 },
		"mach stun":   func(c *Config) { c.Machamp.TreeSlam.StunDuration = -1 },
		"height band": func(c *Config) { c.Ash.MegatonPunch.MaxHeight = -5 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// ---- the computer ----

func TestAshBotThrowsTheMegatonPunchAtAnOpponentThatCannotMove(t *testing.T) {
	g := ashDuel()
	g.P[0].CometCD, g.P[0].TreeCD, g.P[0].RockCD = 99, 99, 99
	g.P[1].StunT = 5
	b := newBot(ash, 1)
	in, ok := firstAct(b, g, 0, 30)
	if !ok || !in.Mega {
		t.Fatalf("decision %+v: the Megaton Punch was not thrown at a stunned opponent", in)
	}
}

func TestTheBotStepsOutOfTheLineOfAMegatonPunch(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	for _, kind := range []int{pikachu, snorlax, chikorita, ash, gengar} {
		for seed := uint64(1); seed <= 5; seed++ {
			g := duel(cfg, kind, ash)
			g.P[1].X, g.P[1].Y = 1300, 450
			g.P[0].X, g.P[0].Y = 1300-300, 450 // in the line
			g.P[1].CometCD, g.P[1].TreeCD, g.P[1].RockCD = 99, 99, 99
			press(g, 1, Input{Mega: true})
			b := newBot(kind, seed)
			for i := 0; i < 8; i++ {
				b.observe(g, 0)
			}
			if th := b.look(g, 0).th; th.kind != thPunch {
				t.Fatalf("%s: threat %+v, want the punch", NAMEOF[kind], th)
			}
			if lost := survive(g, b, 0, mp.ChargeDuration+0.5); lost > 0 {
				t.Fatalf("%s seed %d: lost %d HP standing in the line of the punch", NAMEOF[kind], seed, lost)
			}
		}
	}
}

// ---- Ghost Fire ----

func TestTheBotKeepsOutOfGhostFireFarBetterThanOneThatWalksStraightAtGengar(t *testing.T) {
	cfg := DefaultConfig()
	for _, kind := range []int{pikachu, snorlax, chikorita, ash} {
		walked, dodged := 0, 0
		for _, d := range []float64{120, 180, 240, 320} {
			for seed := uint64(1); seed <= 4; seed++ {
				for _, bot := range []bool{false, true} {
					g := duel(cfg, kind, gengar)
					g.P[1].X, g.P[1].Y = 1300, 450
					g.P[0].X, g.P[0].Y = 1300-d, 450
					g.P[1].SneakCD, g.P[1].SBallCD = 99, 99
					press(g, 1, Input{GFire: true})
					b := newBot(kind, seed)
					hp := g.P[0].HP
					for range 8 {
						b.observe(g, 0)
					}
					for range 90 {
						if bot {
							g.P[0].In = b.Decide(g, 0)
						} else {
							g.P[0].In = Input{DX: 1}
						}
						g.Step(dt)
					}
					if bot {
						dodged += hp - g.P[0].HP
					} else {
						walked += hp - g.P[0].HP
					}
				}
			}
		}
		if walked == 0 {
			t.Fatalf("%s: walking at Gengar lost nothing, so the scene proves nothing", NAMEOF[kind])
		}
		if dodged*2 > walked {
			t.Errorf("%s: lost %d HP, against %d for walking straight at it: it hardly dodges", NAMEOF[kind], dodged, walked)
		}
	}
}
