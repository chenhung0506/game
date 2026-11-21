package main

import "testing"

// czGm is a Charizard (slot 0) with its charge full against a Snorlax `d` ahead, in a config with round numbers for the Gigantamax form.
func czGm(cfg Config, d float64) *Game {
	g := duel(cfg, charizard, snorlax)
	g.P[1].X = g.P[0].X + d
	g.P[0].Charge = cfg.Charizard.Ultimate.Max
	g.P[0].AimX, g.P[0].AimY = 1, 0
	return g
}

func TestCharizardsUltimateNeedsAFullChargeAndMakesItGigantamaxForItsDuration(t *testing.T) {
	cfg := DefaultConfig()
	gm := &cfg.Charizard.Gigantamax
	gm.Duration, gm.MaxHP, gm.Radius, gm.WalkSpeed = 1, 500, 55, 99
	g := czGm(cfg, 800)
	g.P[0].Charge = cfg.Charizard.Ultimate.Max - 1
	gpress(g, Input{Ult: true})
	if g.P[0].GmaxT != 0 {
		t.Fatal("it changed with the charge not full")
	}
	g.P[0].Charge = cfg.Charizard.Ultimate.Max
	var changed bool
	g.P[0].In = Input{Ult: true}
	for _, e := range g.Step(dt) {
		changed = changed || e.T == "gmax"
	}
	g.P[0].In = Input{}
	if !changed || g.P[0].GmaxT <= 0 || g.P[0].Charge >= 1 {
		t.Fatalf("changed %v, GmaxT %v, charge %v: the press should start it and empty the charge", changed, g.P[0].GmaxT, g.P[0].Charge)
	}
	if g.maxHP(0) != 500 || g.radius(0) != 55 || g.walkSpeedOf(&g.P[0]) != 99 {
		t.Fatalf("max HP %d radius %v walk %v: the Gigantamax numbers should be in force", g.maxHP(0), g.radius(0), g.walkSpeedOf(&g.P[0]))
	}
	var ended bool
	for n := 0; n < 60 && g.P[0].GmaxT > 0; n++ {
		for _, e := range g.Step(dt) {
			ended = ended || e.T == "gmaxend"
		}
	}
	if !ended || g.P[0].GmaxT != 0 || g.maxHP(0) != cfg.Charizard.MaxHP || g.radius(0) != cfg.Charizard.Radius {
		t.Fatalf("ended %v: it should be Charizard again (max HP %d, radius %v)", ended, g.maxHP(0), g.radius(0))
	}
}

func TestCharizardsHPIsScaledToTheGigantamaxMaximumAndBackWithoutChargingAnyone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.MaxHP = 100
	cfg.Charizard.Gigantamax.MaxHP, cfg.Charizard.Gigantamax.Duration = 300, 0.5
	g := czGm(cfg, 800)
	g.P[0].HP = 50
	gpress(g, Input{Ult: true})
	if g.P[0].HP != 150 {
		t.Fatalf("HP %d, want half of 300", g.P[0].HP)
	}
	g.P[0].HP = 90
	c0 := g.P[0].Charge
	steps(g, 20)
	if g.P[0].GmaxT != 0 || g.P[0].HP != 30 {
		t.Fatalf("GmaxT %v HP %d, want 90/300 of 100 = 30", g.P[0].GmaxT, g.P[0].HP)
	}
	if g.P[0].Charge > c0+cfg.Charizard.Ultimate.GainPerSecond {
		t.Fatalf("the charge went %v -> %v: changing back is not damage", c0, g.P[0].Charge)
	}
}

func TestTheGigantamaxCharizardPlaysByItsOwnSkillNumbers(t *testing.T) {
	cfg := DefaultConfig()
	gm := &cfg.Charizard.Gigantamax
	gm.Fly.Height, gm.Fly.Speed, gm.FireBreath.Radius, gm.FireBreath.Quantity, gm.FireBreath.Damage = 150, 400, 33, 3, 9
	cfg.Charizard.Fly.Height, cfg.Charizard.FireBreath.Radius, cfg.Charizard.FireBreath.Quantity = 60, 10, 20
	g := czGm(cfg, 800)
	if g.charizardP(&g.P[0]).FireBreath.Quantity != 20 {
		t.Fatal("Charizard's own breath is not in force before it changes")
	}
	gpress(g, Input{Ult: true})
	if cz := g.charizardP(&g.P[0]); cz.Fly.Height != 150 || cz.FireBreath.Quantity != 3 {
		t.Fatalf("height %v quantity %d", cz.Fly.Height, cz.FireBreath.Quantity)
	}
	g.P[0].BreathCD = 0
	g.P[0].In = Input{Fire: true}
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 3*tickRate)
	var big int
	for _, b := range g.Balls {
		if b.K == ballFire && b.Ult && g.ballRadius(&b) == 33 {
			big++
		}
	}
	if big == 0 && g.P[1].HP == g.maxHP(1) {
		t.Fatal("no Gigantamax fire ball flew or hit")
	}
	g.P[0].In = Input{Skill: true}
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 30)
	if g.P[0].Z != 150 {
		t.Fatalf("flying at %v, want its own height 150", g.P[0].Z)
	}
}

func TestAGigantamaxFireBallKeepsItsNumbersAndItsDamageIsTheUltimatesAfterChangingBack(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.Gigantamax.Duration = 0.1
	cfg.Charizard.Gigantamax.FireBreath.Radius, cfg.Charizard.Gigantamax.FireBreath.Damage, cfg.Charizard.Gigantamax.FireBreath.Speed = 40, 20, 200
	cfg.Charizard.FireBreath.Radius, cfg.Charizard.FireBreath.Damage, cfg.Charizard.FireBreath.Speed = 10, 2, 200
	cfg.Charizard.Ultimate = Ultimate{Max: 100, GainDealt: 1, GainUltDealt: 0}
	g := czGm(cfg, 600)
	g.P[1].HP = g.maxHP(1)
	g.Balls = append(g.Balls,
		Ball{K: ballFire, Owner: 0, Left: 500, X: g.P[0].X + 60, Y: g.P[0].Y, DX: 1, HP: 5, Ult: true},
	)
	g.P[0].Charge = 0
	if r := g.ballRadius(&g.Balls[0]); r != 40 {
		t.Fatalf("radius %v, want the Gigantamax 40 though Charizard is not Gigantamax", r)
	}
	hp := g.P[1].HP
	steps(g, 90)
	if g.P[1].HP != hp-20 {
		t.Fatalf("the foe lost %d, want the Gigantamax ball's 20", hp-g.P[1].HP)
	}
	if g.P[0].Charge != 0 {
		t.Fatalf("charge %v: an ultimate's damage pays gain_ultimate_dealt (0 here), not gain_dealt", g.P[0].Charge)
	}
	// the same ball, but an ordinary one, pays gain_dealt
	g.P[1].X, g.P[1].Y = g.P[0].X+300, g.P[0].Y
	g.Balls = append(g.Balls[:0], Ball{K: ballFire, Owner: 0, Left: 500, X: g.P[0].X + 60, Y: g.P[0].Y, DX: 1, HP: 5})
	steps(g, 90)
	if g.P[0].Charge <= 0 {
		t.Fatal("an ordinary fire ball paid nothing")
	}
}

func TestTheBurningGroundOfAGigantamaxTossIsBiggerAndTheUltimatesToo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.Gigantamax.SeismicToss.FireRadius = 333
	cfg.Charizard.Gigantamax.SeismicToss.FireDamage = 7
	cfg.Charizard.Ultimate = Ultimate{Max: 100, GainDealt: 1, GainUltDealt: 0}
	g := czGm(cfg, 150)
	gpress(g, Input{Ult: true})
	g.P[0].In = Input{Toss: true}
	g.Step(dt)
	g.P[0].In = Input{}
	for n := 0; n < 10*tickRate && len(g.Fires) == 0; n++ {
		g.Step(dt)
	}
	if len(g.Fires) == 0 || !g.Fires[0].Ult || g.Fires[0].R != 333 {
		t.Fatalf("fires %+v: want one lit by the Gigantamax Charizard, 333 wide", g.Fires)
	}
	g.P[0].GmaxT = 0 // changed back: the ground still burns by the Gigantamax numbers
	g.P[0].Charge = 0
	hp := g.P[1].HP
	g.Fires[0].HitCD = 0
	g.P[1].X, g.P[1].Y, g.P[1].Z = g.Fires[0].X, g.Fires[0].Y, 0
	g.tickFires(dt)
	if hp-g.P[1].HP != 7 {
		t.Fatalf("burnt %d, want the Gigantamax 7", hp-g.P[1].HP)
	}
}

func TestTheGigantamaxCharizardNumbersAreChecked(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"max_hp":     func(c *Config) { c.Charizard.Gigantamax.MaxHP = 0 },
		"radius":     func(c *Config) { c.Charizard.Gigantamax.Radius = 0 },
		"duration":   func(c *Config) { c.Charizard.Gigantamax.Duration = 0 },
		"its fly":    func(c *Config) { c.Charizard.Gigantamax.Fly.Height = 0 },
		"its breath": func(c *Config) { c.Charizard.Gigantamax.FireBreath.Radius = 0 },
		"its toss":   func(c *Config) { c.Charizard.Gigantamax.SeismicToss.Length = 0 },
		"the charge": func(c *Config) { c.Charizard.Ultimate.Max = 0 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Fatalf("%s: a nonsense number was accepted", name)
		}
	}
}

func TestTheCharizardBotChangesWhenChargedAndTheFoeIsInReach(t *testing.T) {
	cfg := liveConfig(t)
	for _, full := range []bool{false, true} {
		g := czGm(cfg, 400)
		if !full {
			g.P[0].Charge = 0
		}
		bot := newBot(charizard, 2)
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

func TestTheGigantamaxCharizardIsSentToThePage(t *testing.T) {
	cfg := DefaultConfig()
	hi := string(hiMessage(cfg, "web", cfg.courtFor("web"), 0))
	for _, want := range []string{`"cgmax"`, `"firehp"`} {
		if !contains(hi, want) {
			t.Fatalf("hi lacks %s", want)
		}
	}
	if u := ultMax(cfg); u[charizard] != cfg.Charizard.Ultimate.Max {
		t.Fatalf("ult[charizard] = %v", u[charizard])
	}
}
