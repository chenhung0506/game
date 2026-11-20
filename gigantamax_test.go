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

func TestTheGigantamaxMachampUsesItsOwnNumbersForTheThreeSkills(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.Gigantamax.Uppercut.Damage = 77
	cfg.Machamp.Gigantamax.HighJumpKick.Damage = 66
	cfg.Machamp.Gigantamax.CometPunch.Quantity = 5
	cfg.Machamp.Uppercut.Damage, cfg.Machamp.HighJumpKick.Damage = 11, 12
	g := gmGame(cfg, 800)
	if g.brawler(0).Uppercut.Damage != 11 {
		t.Fatal("Machamp's own uppercut is not in force before it changes")
	}
	gpress(g, Input{Ult: true})
	if b := g.brawler(0); b.Uppercut.Damage != 77 || b.HighJumpKick.Damage != 66 || b.CometPunch.Quantity != 5 {
		t.Fatalf("uppercut %d kick %d fists %d", b.Uppercut.Damage, b.HighJumpKick.Damage, b.CometPunch.Quantity)
	}
	if b := g.brawler(1); b == &cfg.Machamp {
		t.Fatal("the foe's numbers are Machamp's")
	}
	// the uppercut that is thrown does the Gigantamax damage
	g.P[1].X, g.P[1].Y, g.P[1].HP = g.P[0].X+300, g.P[0].Y, 100000
	hp := g.P[1].HP
	g.P[0].UpCD = 0
	press(g, 0, Input{Skill: true})
	steps(g, 90)
	if got := hp - g.P[1].HP; got != 77 {
		t.Fatalf("the uppercut did %d, want the Gigantamax 77", got)
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
		"its skill": func(c *Config) { c.Machamp.Gigantamax.Uppercut.RushSpeed = 0 },
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
