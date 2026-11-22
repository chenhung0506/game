package main

import (
	"math"
	"testing"
)

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
	gm.FireBreath.Radius, gm.FireBreath.Quantity, gm.FireBreath.Damage = 33, 3, 9
	cfg.Charizard.Fly.Height, cfg.Charizard.FireBreath.Radius, cfg.Charizard.FireBreath.Quantity = 60, 10, 20
	g := czGm(cfg, 800)
	if g.charizardP(&g.P[0]).FireBreath.Quantity != 20 {
		t.Fatal("Charizard's own breath is not in force before it changes")
	}
	gpress(g, Input{Ult: true})
	if cz := g.charizardP(&g.P[0]); cz.Inferno.DiveSpeed == 0 || cz.FireBreath.Quantity != 3 {
		t.Fatalf("inferno %+v quantity %d", cz.Inferno, cz.FireBreath.Quantity)
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
	g.P[0].BreathCD, g.P[0].FlyCD = 0, 0
	gpress(g, Input{Skill: true})
	steps(g, 30)
	if g.P[0].FlyT != 0 || g.P[0].Z != 0 {
		t.Fatalf("it flew (FlyT %v, Z %v): the main button is the Inferno, not Fly", g.P[0].FlyT, g.P[0].Z)
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
		"max_hp":      func(c *Config) { c.Charizard.Gigantamax.MaxHP = 0 },
		"radius":      func(c *Config) { c.Charizard.Gigantamax.Radius = 0 },
		"duration":    func(c *Config) { c.Charizard.Gigantamax.Duration = 0 },
		"its inferno": func(c *Config) { c.Charizard.Gigantamax.Inferno.DiveSpeed = 0 },
		"its breath":  func(c *Config) { c.Charizard.Gigantamax.FireBreath.Radius = 0 },
		"its toss":    func(c *Config) { c.Charizard.Gigantamax.SeismicToss.Length = 0 },
		"the charge":  func(c *Config) { c.Charizard.Ultimate.Max = 0 },
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

// ---- the Inferno ----

func infGame(cfg Config, d float64) *Game {
	g := czGm(cfg, d)
	g.P[0].GmaxT = 100
	g.P[0].HP = g.maxHP(0)
	g.P[1].HP = g.maxHP(1)
	return g
}

func birds(g *Game) (out []Ball) {
	for _, b := range g.Balls {
		if b.K == ballFirebird {
			out = append(out, b)
		}
	}
	return
}

func TestTheInfernoCallsABirdWhileRootedThenItDivesAndBurstsWhereTheFoeWas(t *testing.T) {
	cfg := DefaultConfig()
	in := &cfg.Charizard.Gigantamax.Inferno
	in.SummonDuration, in.SkyHeight, in.DiveSpeed, in.LockDistance, in.ExplosionRadius, in.Damage = 0.5, 300, 900, 100, 150, 40
	in.LaunchHeight, in.LaunchDuration, in.Cooldown = 120, 0.6, 9
	cfg.Snorlax.MaxHP = 400
	g := infGame(cfg, 500)
	foe0 := g.P[1].HP
	x0 := g.P[0].X
	gpress(g, Input{Skill: true})
	if len(birds(g)) != 1 || g.P[0].InfT <= 0 || g.P[0].FlyCD != 9 {
		t.Fatalf("birds %d, InfT %v, cooldown %v: the press should call a bird, root Charizard and start the cooldown", len(birds(g)), g.P[0].InfT, g.P[0].FlyCD)
	}
	g.P[0].In = Input{DX: -1}
	steps(g, 10) // rooted: it does not walk
	g.P[0].In = Input{}
	if g.P[0].X != x0 || g.P[0].Z != 0 {
		t.Fatalf("X %v Z %v: it should stand rooted on the ground while it calls", g.P[0].X, g.P[0].Z)
	}
	steps(g, 10)
	if b := birds(g)[0]; b.Phase != birdDiving || b.Z <= 0 {
		t.Fatalf("bird phase %d Z %v after the calling", b.Phase, b.Z)
	}
	if g.P[0].InfT != 0 {
		t.Fatal("Charizard is still rooted after the calling")
	}
	var boom bool
	for n := 0; n < 90 && len(birds(g)) > 0; n++ {
		for _, e := range g.Step(dt) {
			boom = boom || e.T == "infboom"
		}
	}
	if !boom || len(birds(g)) != 0 {
		t.Fatalf("boom %v birds %d: it should have burst", boom, len(birds(g)))
	}
	if g.P[1].HP != foe0-40 || !g.P[1].Juggle || len(g.Fires) != 1 || !g.Fires[0].Inf || !g.Fires[0].Ult {
		t.Fatalf("HP %d (want %d) juggle %v fires %+v: the foe should be hurt and thrown up and the ground burn", g.P[1].HP, foe0-40, g.P[1].Juggle, g.Fires)
	}
}

func TestTheBirdOfFireMissesAFoeThatStepsAsideInTheLastStretch(t *testing.T) {
	cfg := DefaultConfig()
	in := &cfg.Charizard.Gigantamax.Inferno
	in.SummonDuration, in.DiveSpeed, in.LockDistance, in.ExplosionRadius = 0, 600, 200, 60
	g := infGame(cfg, 600)
	gpress(g, Input{Skill: true})
	for n := 0; n < 200 && len(birds(g)) > 0; n++ {
		b := birds(g)[0]
		if left := math.Hypot(math.Hypot(b.AnchorX-b.X, b.AnchorY-b.Y), b.Z); b.Phase == birdDiving && left < 190 && g.P[1].Y < 600 { // inside the lock distance (200): the foe steps away
			g.P[1].Y += 300
		}
		g.Step(dt)
	}
	if g.P[1].HP != g.maxHP(1) {
		t.Fatalf("HP %d: a foe that stepped aside in the last stretch should be missed", g.P[1].HP)
	}
	if len(g.Fires) != 1 {
		t.Fatal("the ground still burns where the bird came down")
	}
}

func TestTheInfernoBurningGroundBurnsByItsOwnNumbersAndCountsAsTheUltimates(t *testing.T) {
	cfg := DefaultConfig()
	in := &cfg.Charizard.Gigantamax.Inferno
	in.FireDamage, in.FireInterval, in.FireMaxHeight = 9, 0.5, 30
	cfg.Charizard.Gigantamax.SeismicToss.FireDamage = 1
	cfg.Charizard.Ultimate = Ultimate{Max: 100, GainDealt: 1, GainUltDealt: 0}
	g := infGame(cfg, 600)
	g.P[0].Charge = 0
	g.Fires = append(g.Fires, FireZone{Owner: 0, X: g.P[1].X, Y: g.P[1].Y, R: 100, Left: 5, Ult: true, Inf: true})
	hp := g.P[1].HP
	g.tickFires(dt)
	if hp-g.P[1].HP != 9 {
		t.Fatalf("burnt %d, want the Inferno's 9", hp-g.P[1].HP)
	}
	steps(g, 1)
	if g.P[0].Charge != 0 {
		t.Fatalf("charge %v: the damage of the ultimate pays gain_ultimate_dealt (0 here)", g.P[0].Charge)
	}
}

func TestTheInfernoIsCutShortByAFreezeAndWaitsForItsCooldown(t *testing.T) {
	cfg := DefaultConfig()
	g := infGame(cfg, 800)
	gpress(g, Input{Skill: true})
	g.freeze(&g.P[0], 1)
	steps(g, 2)
	if g.P[0].InfT != 0 || len(birds(g)) != 0 {
		t.Fatalf("InfT %v birds %d: a freeze should end the calling and the bird with it", g.P[0].InfT, len(birds(g)))
	}
	g.P[0].FrozenT = 0
	gpress(g, Input{Skill: true})
	if len(birds(g)) != 0 {
		t.Fatal("it called again during the cooldown")
	}
}

func TestChangingBackDoesNotStopABirdAlreadyInTheSky(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.Gigantamax.Inferno.SummonDuration = 0.1
	g := infGame(cfg, 800)
	gpress(g, Input{Skill: true})
	steps(g, 6)
	g.P[0].GmaxT = 0
	if len(birds(g)) != 1 {
		t.Fatalf("birds %d: the bird goes on", len(birds(g)))
	}
	if g.fireBreathOf(true).Radius != cfg.Charizard.Gigantamax.FireBreath.Radius {
		t.Fatal("the fire numbers are not the Gigantamax ones")
	}
}

func TestOrdinaryCharizardStillFliesAndHasNoInferno(t *testing.T) {
	cfg := DefaultConfig()
	g := czGm(cfg, 800)
	gpress(g, Input{Skill: true})
	if g.P[0].FlyT <= 0 || g.P[0].InfT != 0 || len(birds(g)) != 0 {
		t.Fatalf("FlyT %v InfT %v birds %d: the main button is Fly when it is not Gigantamax", g.P[0].FlyT, g.P[0].InfT, len(birds(g)))
	}
}
