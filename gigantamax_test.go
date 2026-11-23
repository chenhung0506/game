package main

import (
	"net/http/httptest"
	"testing"
)

// gmGame is a Machamp (slot 0) with its charge full against a Snorlax `d` ahead.
func gmGame(cfg Config, d float64) *Game {
	g := kickDuel(cfg, d)
	g.P[0].Charge = cfg.Machamp.Ultimate.Max
	return g
}

func TestMachampsUltimateNeedsAFullChargeAndMakesItGigantamaxForItsDuration(t *testing.T) {
	cfg := DefaultConfig()
	gm := &cfg.Machamp.Gigantamax
	gm.Duration, gm.MaxHP, gm.Radius, gm.WalkSpeed = 1, 400, 44, 99
	g := gmGame(cfg, 800)
	g.P[0].Charge = cfg.Machamp.Ultimate.Max - 1
	gpress(g, Input{Ult: true})
	if g.P[0].GmaxT != 0 {
		t.Fatal("it changed with the charge not full")
	}
	g.P[0].Charge = cfg.Machamp.Ultimate.Max
	var changed bool
	g.P[0].In = Input{Ult: true}
	for _, e := range g.Step(dt) {
		changed = changed || e.T == "gmax"
	}
	g.P[0].In = Input{}
	if !changed || g.P[0].GmaxT <= 0 || g.P[0].Charge >= 1 {
		t.Fatalf("changed %v, GmaxT %v, charge %v: the press should start it and empty the charge", changed, g.P[0].GmaxT, g.P[0].Charge)
	}
	if g.maxHP(0) != 400 || g.radius(0) != 44 || g.walkSpeedOf(&g.P[0]) != 99 {
		t.Fatalf("max HP %d radius %v walk %v: the Gigantamax numbers should be in force", g.maxHP(0), g.radius(0), g.walkSpeedOf(&g.P[0]))
	}
	var ended bool
	for n := 0; n < 60 && g.P[0].GmaxT > 0; n++ {
		for _, e := range g.Step(dt) {
			ended = ended || e.T == "gmaxend"
		}
	}
	if !ended || g.P[0].GmaxT != 0 || g.maxHP(0) != cfg.Machamp.MaxHP || g.radius(0) != cfg.Machamp.Radius {
		t.Fatalf("ended %v: it should be Machamp again (max HP %d, radius %v)", ended, g.maxHP(0), g.radius(0))
	}
}

func TestTheHPIsScaledToTheGigantamaxMaximumAndBack(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.MaxHP = 100
	cfg.Machamp.Gigantamax.MaxHP, cfg.Machamp.Gigantamax.Duration = 300, 0.5
	g := gmGame(cfg, 800)
	g.P[0].HP = 50 // half of 100
	gpress(g, Input{Ult: true})
	if g.P[0].HP != 150 {
		t.Fatalf("HP %d, want half of 300", g.P[0].HP)
	}
	g.P[0].HP = 90 // hurt meanwhile
	steps(g, 20)
	if g.P[0].GmaxT != 0 || g.P[0].HP != 30 {
		t.Fatalf("GmaxT %v HP %d, want 90/300 of 100 = 30", g.P[0].GmaxT, g.P[0].HP)
	}
}

func TestTheGigantamaxMachampUsesItsOwnNumbersForItsSkills(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.Gigantamax.CometPunch.Quantity = 5
	cfg.Machamp.CometPunch.Quantity = 11
	g := gmGame(cfg, 800)
	if g.brawler(0).CometPunch.Quantity != 11 {
		t.Fatal("Machamp's own comet punch is not in force before it changes")
	}
	gpress(g, Input{Ult: true})
	if b := g.brawler(0); b.CometPunch.Quantity != 5 {
		t.Fatalf("fists %d", b.CometPunch.Quantity)
	}
	if b := g.brawler(1); b == &cfg.Machamp {
		t.Fatal("the foe's numbers are Machamp's")
	}
}

func TestTheGigantamaxChangeNeedsNothingElseGoingOnAndEndsWithARound(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGame(cfg, 1500)
	g.P[1].Y = 900
	press(g, 0, Input{Kick: true}) // mid-kick
	g.P[0].In = Input{Ult: true}
	g.Step(dt)
	if g.P[0].GmaxT != 0 {
		t.Fatal("it changed in the middle of a kick")
	}
	g.P[0].GmaxT = 5
	g.Reset()
	if g.P[0].GmaxT != 0 || g.maxHP(0) != cfg.Machamp.MaxHP {
		t.Fatal("it is still Gigantamax in the next round")
	}
}

func TestTheGigantamaxNumbersAreChecked(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"max_hp":    func(c *Config) { c.Machamp.Gigantamax.MaxHP = 0 },
		"radius":    func(c *Config) { c.Machamp.Gigantamax.Radius = 0 },
		"duration":  func(c *Config) { c.Machamp.Gigantamax.Duration = 0 },
		"its skill": func(c *Config) { c.Machamp.Gigantamax.BurstPunch.Count = 0 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Fatalf("%s: a nonsense number was accepted", name)
		}
	}
	if err := DefaultConfig().validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTheMachampBotChangesWhenChargedAndTheFoeIsInReach(t *testing.T) {
	cfg := liveConfig(t)
	for _, full := range []bool{false, true} {
		g := gmGame(cfg, 400)
		if !full {
			g.P[0].Charge = 0
		}
		bot := newBot(machamp, 2)
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

func TestTheCameraSettingForGigantamaxIsSentToThePage(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Court.Mobile.GigantamaxZoomOut || cfg.Court.Web.GigantamaxZoomOut {
		t.Fatal("it is off unless the config turns it on")
	}
	cfg.Court.Web.GigantamaxZoomOut = true
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	hi := next(t, dialPlay(t, srv, "dummy"), "hi")
	if cam := hi["cam"].(map[string]any); cam["gz"] != float64(1) {
		t.Fatalf("cam = %v", cam)
	}
}

// lpGame is a Gigantamax Machamp (slot 0, at 300,250) against a Snorlax `d` ahead on the same line.
func lpGame(cfg Config, d float64) *Game {
	g := gmGame(cfg, d)
	g.P[0].GmaxT = 100
	g.P[1].HP = 100000
	return g
}

func TestTheGigantamaxKickIsTheLightningPunchAndTheNormalKickIsNotAvailable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.Gigantamax.LightningPunch.ChargeDuration = 0.5
	g := lpGame(cfg, 600)
	press(g, 0, Input{Kick: true})
	if g.P[0].LpT <= 0 || g.P[0].KickPhase != kickNone || g.P[0].LpCD <= 0 {
		t.Fatalf("LpT %v kick phase %d cooldown %v: the Kick key should gather the Lightning Punch", g.P[0].LpT, g.P[0].KickPhase, g.P[0].LpCD)
	}
	n := DefaultConfig()
	k := kickDuel(n, 600)
	press(k, 0, Input{Kick: true})
	if k.P[0].KickPhase == kickNone || k.P[0].LpT != 0 {
		t.Fatal("a normal Machamp's Kick is the High Jump Kick")
	}
}

func TestTheLightningPunchGathersRootedAimedAtTheFoeThenHitsAndABoltComesDownOnIt(t *testing.T) {
	cfg := DefaultConfig()
	lp := &cfg.Machamp.Gigantamax.LightningPunch
	lp.ChargeDuration, lp.Distance, lp.Radius, lp.Damage, lp.Knockback, lp.StunDuration = 0.4, 600, 60, 40, 100, 1
	lp.LightningRadius, lp.LightningDamage = 150, 30
	g := lpGame(cfg, 400)
	hp0 := g.P[1].HP
	press(g, 0, Input{Kick: true, DY: 1}) // the stick is held, but it gathers on the spot
	x0, y0 := g.P[0].X, g.P[0].Y
	g.P[1].Y = 450 // the foe moves while it gathers: the fist follows
	var punch, bolt, stunned bool
	for n := 0; n < 60; n++ {
		g.P[0].In = Input{DX: 1}
		for _, e := range g.Step(dt) {
			punch, bolt = punch || e.T == "lpunch", bolt || e.T == "lpbolt"
		}
		stunned = stunned || g.P[1].StunT > 0
		if g.P[0].LpT > 0 && (g.P[0].X != x0 || g.P[0].Y != y0) {
			t.Fatal("it walked while it gathered")
		}
	}
	if !punch {
		t.Fatal("the fist was never thrown")
	}
	if !bolt {
		// the foe at (700,450) is 200 off the line of the first aim, but the fist followed it: it must have been hit
		t.Fatalf("no bolt: foe HP %d of %d, LpX %v LpY %v", g.P[1].HP, hp0, g.P[0].LpX, g.P[0].LpY)
	}
	if got := hp0 - g.P[1].HP; got != lp.Damage+lp.LightningDamage {
		t.Fatalf("the foe lost %d, want the punch %d plus the bolt %d", got, lp.Damage, lp.LightningDamage)
	}
	if !stunned {
		t.Fatal("the foe was not stunned")
	}
}

func TestTheLightningPunchMissesAFoeOutOfReach(t *testing.T) {
	cfg := DefaultConfig()
	lp := &cfg.Machamp.Gigantamax.LightningPunch
	lp.ChargeDuration, lp.Distance = 0, 300
	g := lpGame(cfg, 900) // beyond the reach of the fist
	press(g, 0, Input{Kick: true})
	steps(g, 5)
	if g.P[1].HP != 100000 {
		t.Fatal("hit a foe out of reach")
	}
	if g.P[0].LpCD <= 0 {
		t.Fatal("a miss has its cooldown as well")
	}
}

func TestTheLightningPunchIsCancelledWhenStunnedAndTheNumbersAreChecked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.Gigantamax.LightningPunch.ChargeDuration = 1
	g := lpGame(cfg, 400)
	press(g, 0, Input{Kick: true})
	g.P[0].StunT = 0.5
	steps(g, 3)
	if g.P[0].LpT != 0 {
		t.Fatalf("LpT %v: it should have been broken off", g.P[0].LpT)
	}
	for name, edit := range map[string]func(*Config){
		"distance":  func(c *Config) { c.Machamp.Gigantamax.LightningPunch.Distance = 0 },
		"radius":    func(c *Config) { c.Machamp.Gigantamax.LightningPunch.Radius = 0 },
		"damage":    func(c *Config) { c.Machamp.Gigantamax.LightningPunch.Damage = -1 },
		"the reach": func(c *Config) { c.Machamp.Gigantamax.LightningPunch.MinHeight = 100 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Fatalf("%s: a nonsense number was accepted", name)
		}
	}
}

// A tick that takes the last of its time off can leave GmaxT a hair below zero; the HP must be scaled back from the Gigantamax maximum all the same, for whatever the duration is.
func TestTheHPComesBackScaledWhateverTheDurationIsAndNeverAboveTheMaximum(t *testing.T) {
	for _, dur := range []float64{0.3, 0.7, 1.1, 2.3, 7.7, 15, 33.3, 150} {
		cfg := DefaultConfig()
		cfg.Machamp.MaxHP = 170
		cfg.Machamp.Gigantamax.MaxHP, cfg.Machamp.Gigantamax.Duration = 510, dur
		g := gmGame(cfg, 800)
		gpress(g, Input{Ult: true})
		g.P[0].HP = 90 // 90 of 510
		for n := 0; n < int(dur/dt)+100 && g.P[0].GmaxT > 0; n++ {
			g.Step(dt)
		}
		if g.P[0].GmaxT != 0 {
			t.Fatalf("duration %v: it never ended", dur)
		}
		if g.P[0].HP != 30 || g.P[0].HP > g.maxHP(0) {
			t.Fatalf("duration %v: HP %d after it ended (max %d), want 30", dur, g.P[0].HP, g.maxHP(0))
		}
	}
}

func TestChangingBackScalesTheHPWithoutHurtingAnyoneForTheOthersCharge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.MaxHP = 170
	cfg.Machamp.Gigantamax.MaxHP, cfg.Machamp.Gigantamax.Duration = 510, 0.3
	cfg.Pikachu.Ultimate.GainPerSecond, cfg.Pikachu.Ultimate.GainDealt, cfg.Pikachu.Ultimate.GainUltDealt = 0, 1, 1
	cfg.Machamp.Ultimate.GainTaken, cfg.Machamp.Ultimate.GainPerSecond = 1, 0
	g := duel(cfg, machamp, pikachu)
	g.P[0].Charge = cfg.Machamp.Ultimate.Max
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{Ult: true})
	g.P[0].HP = 300
	c0, p1 := g.P[0].Charge, g.P[1].Charge
	steps(g, 30)
	if g.P[0].GmaxT != 0 || g.P[0].HP != 100 {
		t.Fatalf("GmaxT %v HP %d, want it back with 300/510 of 170 = 100", g.P[0].GmaxT, g.P[0].HP)
	}
	if g.P[1].Charge != p1 || g.P[0].Charge != c0 {
		t.Fatalf("charges moved (%v -> %v, %v -> %v): nobody was hurt", p1, g.P[1].Charge, c0, g.P[0].Charge)
	}
}
