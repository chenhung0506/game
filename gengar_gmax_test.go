package main

import (
	"math"
	"testing"
)

// gmGen is a Gigantamax Gengar (slot 0, at 300,250) with round numbers for its skills, against a Snorlax (slot 1) `d` ahead of it.
func gmGen(cfg Config, d float64) *Game {
	gg := &cfg.Gengar.Gigantamax
	gg.MaxHP, gg.Radius, gg.WalkSpeed, gg.Duration = 400, 40, 100, 100
	gg.Lick = LickCfg{Reach: Reach{MinHeight: 0, MaxHeight: 80}, ChargeDuration: 0.1, RechargeDuration: 0.2, Speed: 600, Length: 500, Width: 40, Damage: 7, PullSpeed: 600,
		BiteDuration: 0.1, BiteDamage: 11, BiteStunDuration: 1, Cooldown: 5, TongueScale: 1, BiteScale: 1}
	gg.Devour = DevourCfg{Reach: Reach{MinHeight: 0, MaxHeight: 80}, GravityRadius: 400, SilenceRadius: 150, Gravity: 300, GravityDuration: 0.5, SilenceDuration: 1.5, Cooldown: 6, Scale: 1}
	g := gen(cfg, snorlax)
	g.P[1].X = g.P[0].X + d
	g.P[0].GmaxT = gg.Duration
	g.P[0].HP, g.P[1].HP = g.maxHP(0), g.maxHP(1)
	return g
}

func TestGengarsUltimateNeedsAFullChargeAndMakesItGigantamaxForItsDuration(t *testing.T) {
	cfg := DefaultConfig()
	gg := &cfg.Gengar.Gigantamax
	gg.Duration, gg.MaxHP, gg.Radius, gg.WalkSpeed, gg.ChargeDuration = 1, 500, 55, 99, 0
	g := gen(cfg, snorlax)
	g.P[0].Charge = cfg.Gengar.Ultimate.Max - 1
	gpress(g, Input{Ult: true})
	if g.P[0].GmaxT != 0 {
		t.Fatal("it changed with the charge not full")
	}
	g.P[0].Charge = cfg.Gengar.Ultimate.Max
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
	if !ended || g.P[0].GmaxT != 0 || g.maxHP(0) != cfg.Gengar.MaxHP || g.radius(0) != cfg.Gengar.Radius {
		t.Fatalf("ended %v: it should be Gengar again (max HP %d, radius %v)", ended, g.maxHP(0), g.radius(0))
	}
}

func TestTheGigantamaxGengarsMainButtonLicksInsteadOfSneakingAndNormalGengarStillSneaks(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 300)
	gpress(g, Input{Skill: true})
	if g.P[0].SneakT != 0 || g.P[0].LickPhase == lickNone {
		t.Fatalf("SneakT %v, lick phase %d: the main button should lick", g.P[0].SneakT, g.P[0].LickPhase)
	}
	n := gen(cfg, snorlax)
	gpress(n, Input{Skill: true})
	if n.P[0].SneakT <= 0 || n.P[0].LickPhase != lickNone {
		t.Fatalf("an ordinary Gengar should sneak: SneakT %v, lick phase %d", n.P[0].SneakT, n.P[0].LickPhase)
	}
}

func TestTheLickGathersRootedThenHurtsStunsDragsInAndBitesTheFoe(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 300)
	lk := g.cfg.Gengar.Gigantamax.Lick
	hp0 := g.P[1].HP
	gpress(g, Input{Skill: true})
	x0 := g.P[0].X
	g.P[0].In = Input{DX: 1}
	g.Step(dt)
	if g.P[0].X != x0 || g.P[0].LickPhase != lickCharge {
		t.Fatalf("x %v -> %v, phase %d: it should stand rooted while it gathers", x0, g.P[0].X, g.P[0].LickPhase)
	}
	var sawHit, sawBite bool
	var tongueMax float64
	for n := 0; n < 200 && g.P[0].LickPhase != lickNone; n++ {
		g.P[0].In = Input{DX: 1}
		for _, e := range g.Step(dt) {
			sawHit = sawHit || e.T == "lickhit"
			sawBite = sawBite || e.T == "bite"
		}
		tongueMax = math.Max(tongueMax, g.P[0].LickD)
		if g.P[0].X != x0 {
			t.Fatalf("it walked from %v to %v mid-lick: the whole lick roots it", x0, g.P[0].X)
		}
		if g.P[0].LickPhase == lickPull && g.P[1].StunT <= 0 {
			t.Fatal("the foe is not stunned while it is dragged in")
		}
	}
	g.P[0].In = Input{}
	if !sawHit || !sawBite {
		t.Fatalf("hit %v, bite %v: the tongue should hit and then the bite should follow", sawHit, sawBite)
	}
	if got := hp0 - g.P[1].HP; got != lk.Damage+lk.BiteDamage {
		t.Fatalf("the foe lost %d HP, want %d (the tongue) + %d (the bite)", got, lk.Damage, lk.BiteDamage)
	}
	if d := math.Hypot(g.P[0].X-g.P[1].X, g.P[0].Y-g.P[1].Y); math.Abs(d-(g.radius(0)+g.radius(1))) > 1 {
		t.Fatalf("the foe ended %v away, want touching (%v)", d, g.radius(0)+g.radius(1))
	}
	if g.P[1].StunT < lk.BiteStunDuration-0.5 {
		t.Fatalf("StunT %v: the bite should stun it for about %v", g.P[1].StunT, lk.BiteStunDuration)
	}
	if tongueMax <= 0 || tongueMax > lk.Length+1e-6 {
		t.Fatalf("the tongue got %v long, at most %v", tongueMax, lk.Length)
	}
	if g.P[0].LickPhase != lickNone || g.P[0].LickCD <= 0 {
		t.Fatalf("phase %d, cooldown %v: it should be over with the cooldown running", g.P[0].LickPhase, g.P[0].LickCD)
	}
}

func TestTheLickTongueGoesOutAtItsSpeedAndNoFurtherThanItsLength(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 900)
	lk := g.cfg.Gengar.Gigantamax.Lick
	hp0 := g.P[1].HP
	gpress(g, Input{Skill: true})
	var out int
	for n := 0; n < 200 && g.P[0].LickPhase != lickNone; n++ {
		d0 := g.P[0].LickD
		g.Step(dt)
		if g.P[0].LickPhase == lickOut && g.P[0].LickD > d0 {
			if step := g.P[0].LickD - d0; step > lk.Speed*dt+1e-6 {
				t.Fatalf("the tongue went %v in a tick, at most %v", step, lk.Speed*dt)
			}
			out++
		}
		if g.P[0].LickD > lk.Length+1e-6 {
			t.Fatalf("the tongue is %v long, more than %v", g.P[0].LickD, lk.Length)
		}
	}
	if out == 0 || g.P[1].HP != hp0 {
		t.Fatalf("ticks out %d, foe HP %d -> %d: a foe past the length is not touched", out, hp0, g.P[1].HP)
	}
	if g.P[0].LickPhase != lickNone || g.P[0].LickD != 0 {
		t.Fatalf("phase %d, length %v: the tongue should be back and the lick over", g.P[0].LickPhase, g.P[0].LickD)
	}
}

func TestTheLickMissesAFoeOutsideItsWidthOrHeight(t *testing.T) {
	cfg := DefaultConfig()
	for _, c := range []struct {
		name string
		set  func(g *Game)
	}{
		{"off to the side", func(g *Game) { g.P[1].Y += 200 }},
		{"below its height band", func(g *Game) { g.cfg.Gengar.Gigantamax.Lick.Reach = Reach{MinHeight: 40, MaxHeight: 80} }},
	} {
		g := gmGen(cfg, 300)
		hp0 := g.P[1].HP
		gpress(g, Input{Skill: true})
		for n := 0; n < 100 && g.P[0].LickPhase != lickOut; n++ { // the tongue is aimed at the foe as it gathers and goes where it was aimed: the foe steps aside as it goes out
			g.Step(dt)
		}
		c.set(g)
		steps(g, 150)
		if g.P[1].HP != hp0 || g.P[0].LickPhase != lickNone {
			t.Fatalf("%s: foe HP %d -> %d, phase %d: it should be missed", c.name, hp0, g.P[1].HP, g.P[0].LickPhase)
		}
	}
}

func TestAStunEndsTheLickAndLetsTheFoeGo(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 300)
	gpress(g, Input{Skill: true})
	for n := 0; n < 100 && g.P[0].LickPhase != lickPull; n++ {
		g.Step(dt)
	}
	if g.P[0].LickPhase != lickPull {
		t.Fatal("the tongue never took hold")
	}
	g.P[0].StunT = 1
	g.Step(dt)
	if g.P[0].LickPhase != lickNone || g.P[0].LickPrey != 0 {
		t.Fatalf("phase %d prey %d: a stunned Gengar loses the tongue and its hold", g.P[0].LickPhase, g.P[0].LickPrey)
	}
	x := g.P[1].X
	steps(g, 5)
	if g.P[1].X != x {
		t.Fatalf("the foe went on moving (%v -> %v) after it was let go", x, g.P[1].X)
	}
}

func TestTheLickPullsAndBitesAPracticeDummyLikeAnyoneElse(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 300)
	g.P[1].Kind = dummy
	g.P[1].HP = 1000
	lk := g.cfg.Gengar.Gigantamax.Lick
	gpress(g, Input{Skill: true})
	steps(g, 100)
	if d := math.Hypot(g.P[0].X-g.P[1].X, g.P[0].Y-g.P[1].Y); math.Abs(d-(g.radius(0)+g.radius(1))) > 1 {
		t.Fatalf("the dummy is %v away, want touching (%v): the tongue should drag it in", d, g.radius(0)+g.radius(1))
	}
	if got := 1000 - g.P[1].HP; got != lk.Damage+lk.BiteDamage {
		t.Fatalf("the dummy lost %d HP, want %d + %d", got, lk.Damage, lk.BiteDamage)
	}
}

func TestTheDevourDragsFoesWithinTheGravityRadiusAndSilencesOnlyThoseWithinTheSilenceRadius(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 300) // inside the pull (400), outside the silence (150)
	gpress(g, Input{SBall: true})
	if g.P[0].DevourT <= 0 || g.P[0].DevourCD <= 0 {
		t.Fatalf("pull %v cooldown %v: the press should start the devour", g.P[0].DevourT, g.P[0].DevourCD)
	}
	x0 := g.P[1].X
	steps(g, 5)
	if g.P[1].X >= x0 {
		t.Fatalf("x %v -> %v: a foe inside the gravity radius should be dragged toward Gengar", x0, g.P[1].X)
	}
	if g.P[1].SilentT > 0 {
		t.Fatalf("SilentT %v: it was outside the silence radius", g.P[1].SilentT)
	}
	far := gmGen(cfg, 700) // outside both
	gpress(far, Input{SBall: true})
	steps(far, 5)
	if far.P[1].X != 1000 || far.P[1].SilentT > 0 {
		t.Fatalf("x %v silent %v: a foe outside the radii is left alone", far.P[1].X, far.P[1].SilentT)
	}
	near := gmGen(cfg, 100) // inside the silence
	gpress(near, Input{SBall: true})
	near.Step(dt)
	if near.P[1].SilentT <= 0 {
		t.Fatalf("SilentT %v: a foe inside the silence radius cannot start a skill", near.P[1].SilentT)
	}
}

func TestTheDevoursPullAndSilenceHaveDurationsOfTheirOwn(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 120) // inside the silence radius, and far enough from the body to be pulled
	gpress(g, Input{SBall: true})
	steps(g, 18) // 0.6 s: the pull (0.5 s) is over, the silence (1.5 s) is not
	if g.P[0].DevourT != 0 || g.P[0].DevourSilT <= 0 {
		t.Fatalf("pull left %v, silence left %v after 0.6 s", g.P[0].DevourT, g.P[0].DevourSilT)
	}
	x := g.P[1].X
	steps(g, 3)
	if g.P[1].X != x {
		t.Fatalf("the foe was still pulled after the pull's time was up (%v -> %v)", x, g.P[1].X)
	}
	if g.P[1].SilentT <= 0 {
		t.Fatal("the silence should still hold the foe")
	}
	steps(g, 40)
	if g.P[0].DevourSilT != 0 || g.P[1].SilentT != 0 {
		t.Fatalf("silence left %v, foe silent %v: both should be over", g.P[0].DevourSilT, g.P[1].SilentT)
	}
}

func TestTheDevourNeverDragsAFoeIntoGengarsBody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.Gigantamax.Devour.GravityDuration = 3
	g := gmGen(cfg, 200)
	gpress(g, Input{SBall: true})
	steps(g, 60)
	if d := math.Hypot(g.P[0].X-g.P[1].X, g.P[0].Y-g.P[1].Y); d < g.radius(0)+g.radius(1)-1e-6 {
		t.Fatalf("the foe is %v from Gengar, inside its body (%v)", d, g.radius(0)+g.radius(1))
	}
}

func TestTheGigantamaxGhostFireIsBreathedOnlyForwardInTheFanItsAngleSets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.Gigantamax.GhostFire.Angle = 60
	cfg.Gengar.Gigantamax.GhostFire.Count = 9
	g := gmGen(cfg, 400)
	gpress(g, Input{GFire: true})
	var n int
	for _, b := range g.Balls {
		if b.K != ballGhost {
			continue
		}
		n++
		if ang := math.Abs(math.Atan2(b.DY, b.DX)) * 180 / math.Pi; ang > 30+1e-6 {
			t.Fatalf("a flame went %v degrees off the foe, the fan is 60 wide", ang)
		}
		if !b.Ult {
			t.Fatal("a Gigantamax Gengar's flame is its ultimate's")
		}
	}
	if n != 9 {
		t.Fatalf("%d flames, want 9", n)
	}
	o := gen(cfg, snorlax)
	gpress(o, Input{GFire: true})
	var back bool
	for _, b := range o.Balls {
		back = back || (b.K == ballGhost && b.DX < -0.5)
	}
	if !back {
		t.Fatal("an ordinary Gengar's flames still go out all round")
	}
}

func TestWhatTheGigantamaxGengarDealsIsAnUltimatesDamageForTheCharge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.Ultimate = Ultimate{Max: 10000, GainDealt: 1, GainUltDealt: 10}
	g := gmGen(cfg, 300)
	g.P[0].Charge = 0
	gpress(g, Input{Skill: true})
	steps(g, 100)
	lk := g.cfg.Gengar.Gigantamax.Lick
	if want := float64(10 * (lk.Damage + lk.BiteDamage)); math.Abs(g.P[0].Charge-want) > 1e-6 {
		t.Fatalf("charge %v, want %v: the tongue and the bite are ultimate damage (10 a point)", g.P[0].Charge, want)
	}
	if g.ultNow {
		t.Fatal("the ultimate-damage mark must not stay set after the Gigantamax Gengar's turn")
	}
}

func TestTheGigantamaxGengarConfigIsChecked(t *testing.T) {
	for _, c := range []struct {
		name string
		mut  func(c *Config)
	}{
		{"no body", func(c *Config) { c.Gengar.Gigantamax.Radius = 0 }},
		{"no tongue speed", func(c *Config) { c.Gengar.Gigantamax.Lick.Speed = 0 }},
		{"no pull speed", func(c *Config) { c.Gengar.Gigantamax.Lick.PullSpeed = 0 }},
		{"negative bite", func(c *Config) { c.Gengar.Gigantamax.Lick.BiteDamage = -1 }},
		{"no gravity radius", func(c *Config) { c.Gengar.Gigantamax.Devour.GravityRadius = 0 }},
		{"negative silence", func(c *Config) { c.Gengar.Gigantamax.Devour.SilenceDuration = -1 }},
		{"no fan", func(c *Config) { c.Gengar.Gigantamax.GhostFire.Angle = 0 }},
		{"a fan over a full circle", func(c *Config) { c.Gengar.Gigantamax.GhostFire.Angle = 361 }},
		{"flames with no radius", func(c *Config) { c.Gengar.Gigantamax.GhostFire.Radius = 0 }},
		{"a backwards flame band", func(c *Config) { c.Gengar.Gigantamax.GhostFire.Reach = Reach{MinHeight: 50, MaxHeight: 10} }},
		{"a backwards band", func(c *Config) { c.Gengar.Gigantamax.Lick.Reach = Reach{MinHeight: 50, MaxHeight: 10} }},
	} {
		cfg := DefaultConfig()
		c.mut(&cfg)
		if err := cfg.validate(); err == nil {
			t.Errorf("%s: the config was accepted", c.name)
		}
	}
	if err := DefaultConfig().validate(); err != nil {
		t.Fatalf("the default config is rejected: %v", err)
	}
}

func TestTheGigantamaxGhostFireRunsOnItsOwnNumbersAndGengarsOwnAreLeftAlone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.GhostFire.Count, cfg.Gengar.GhostFire.Radius, cfg.Gengar.GhostFire.HP, cfg.Gengar.GhostFire.Mass, cfg.Gengar.GhostFire.Damage = 12, 22, 10, 20, 4
	cfg.Gengar.Gigantamax.GhostFire = GmaxGhostFireCfg{GhostFireCfg: GhostFireCfg{Reach: Reach{MinHeight: 0, MaxHeight: 80}, Cooldown: 3, RingRadius: 123, SpreadSpeed: 500, ConvergenceSpeed: 300,
		Damage: 33, Count: 5, Radius: 77, HP: 9, Mass: 3}, Angle: 120}
	g := gmGen(cfg, 400)
	gpress(g, Input{GFire: true})
	var gm []Ball
	for _, b := range g.Balls {
		if b.K == ballGhost {
			gm = append(gm, b)
		}
	}
	if len(gm) != 5 || gm[0].HP != 9 || !gm[0].Ult {
		t.Fatalf("%d flames, HP %d: want 5 flames of HP 9 from the Gigantamax numbers", len(gm), gm[0].HP)
	}
	if r, m, d := g.ballRadius(&gm[0]), g.ballMass(&gm[0]), g.ballDamage(&gm[0]); r != 77 || m != 3 || d != 33 {
		t.Fatalf("radius %v mass %v damage %d, want 77, 3 and 33", r, m, d)
	}
	if g.P[0].GFireCD != 3 {
		t.Fatalf("cooldown %v, want the Gigantamax Ghost Fire's own 3", g.P[0].GFireCD)
	}
	g.P[0].GmaxT = 0 // changed back: the flames already out keep their numbers, a new one is Gengar's own
	if r := g.ballRadius(&gm[0]); r != 77 {
		t.Fatalf("a flame breathed while Gigantamax changed to radius %v when it ended", r)
	}
	o := gen(cfg, snorlax)
	gpress(o, Input{GFire: true})
	var own int
	for _, b := range o.Balls {
		if b.K == ballGhost {
			own++
			if b.Ult || o.ballRadius(&b) != 22 || b.HP != 10 {
				t.Fatalf("an ordinary flame is Ult %v, radius %v, HP %d: it should keep Gengar's own numbers", b.Ult, o.ballRadius(&b), b.HP)
			}
		}
	}
	if own != 12 {
		t.Fatalf("%d flames, want Gengar's own 12", own)
	}
}

func TestTheDevourDragsAPracticeDummyToo(t *testing.T) {
	cfg := DefaultConfig()
	g := gmGen(cfg, 300)
	g.P[1].Kind = dummy
	g.P[1].HP = 1000
	gpress(g, Input{SBall: true})
	x0 := g.P[1].X
	steps(g, 5)
	if g.P[1].X >= x0 {
		t.Fatalf("x %v -> %v: the dummy should be dragged toward Gengar", x0, g.P[1].X)
	}
}
