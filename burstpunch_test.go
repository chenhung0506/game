package main

import (
	"math"
	"strings"
	"testing"
)

// bpGame is a Gigantamax Machamp (slot 0) with `bp` for its Burst Punch against a Snorlax `d` ahead with plenty of HP.
func bpGame(bp BurstPunchCfg, d float64) *Game {
	cfg := DefaultConfig()
	cfg.Machamp.Gigantamax.BurstPunch = bp
	g := kickDuel(cfg, d)
	g.P[0].GmaxT = 100
	g.P[1].HP = 100000
	return g
}

// bpEverywhere is a Burst Punch whose explosions all reach the foe: a tiny cylinder and huge explosions.
func bpEverywhere() BurstPunchCfg {
	return BurstPunchCfg{Distance: 300, Radius: 10, Height: 10, Count: 6, ExplosionRadius: 900, ExplosionDamage: 3, ExplosionHeight: 100, HeightGain: 50, LaunchCap: 220, LaunchDuration: 1, Cooldown: 5, FistScale: 1, ExplosionScale: 1}
}

func TestTheGigantamaxMachampsMainButtonIsTheBurstPunchNotTheUppercut(t *testing.T) {
	bp := bpEverywhere()
	bp.ChargeDuration = 0.5
	g := bpGame(bp, 300)
	press(g, 0, Input{Skill: true})
	if g.P[0].UpPhase != upNone || g.P[0].BpT <= 0 || g.P[0].BpCD != bp.Cooldown {
		t.Fatalf("uppercut phase %d, gathering %v, cooldown %v: it should gather the Burst Punch", g.P[0].UpPhase, g.P[0].BpT, g.P[0].BpCD)
	}
	x := g.P[0].X
	var out *Event
	for n := 0; n < 40 && out == nil; n++ {
		if g.P[0].X != x {
			t.Fatal("it moved while it gathered")
		}
		for _, e := range g.Step(dt) {
			if e.T == "bpunch" {
				e := e
				out = &e
			}
		}
	}
	if out == nil {
		t.Fatal("the fist never came out")
	}
	if math.Abs(out.X-(x+bp.Distance)) > 1e-6 || out.Y != g.P[0].Y || out.S != 0 {
		t.Fatalf("the fist is at %v,%v, want %v ahead of the player", out.X, out.Y, bp.Distance)
	}
	g2 := bpGame(bp, 300)
	g2.P[0].GmaxT = 0 // the ordinary Machamp still has its Uppercut
	press(g2, 0, Input{Skill: true})
	if g2.P[0].UpPhase == upNone || g2.P[0].BpT != 0 {
		t.Fatal("an ordinary Machamp lost its Uppercut")
	}
}

func TestTheBurstPunchGathersAimedAtTheFoeAndAFreezeCancelsIt(t *testing.T) {
	bp := bpEverywhere()
	bp.ChargeDuration = 0.6
	g := bpGame(bp, 300)
	press(g, 0, Input{Skill: true})
	g.P[1].X, g.P[1].Y = g.P[0].X, g.P[0].Y+400 // the foe moves round to the side
	g.Step(dt)
	if g.P[0].BpX != 0 || g.P[0].BpY != 1 {
		t.Fatalf("aim %v,%v: the fist should follow the foe while it gathers", g.P[0].BpX, g.P[0].BpY)
	}
	g.freeze(&g.P[0], 1)
	if steps(g, 40); len(g.Bursts) != 0 || g.P[0].BpT != 0 {
		t.Fatalf("%d casts, gathering %v: a frozen Machamp throws no fist", len(g.Bursts), g.P[0].BpT)
	}
}

func TestEveryExplosionOfTheBurstPunchHurtsAndEachOneThrowsTheFoeHigher(t *testing.T) {
	bp := bpEverywhere()
	g := bpGame(bp, 300)
	hp := g.P[1].HP
	var booms []Event
	for _, e := range press(g, 0, Input{Skill: true}) {
		if e.T == "bpboom" {
			booms = append(booms, e)
		}
	}
	if len(booms) != bp.Count || len(g.Bursts) != 0 {
		t.Fatalf("%d explosions, %d casts left: with no duration all go off at once", len(booms), len(g.Bursts))
	}
	if got := hp - g.P[1].HP; got != bp.Count*bp.ExplosionDamage {
		t.Fatalf("took %d, want %d", got, bp.Count*bp.ExplosionDamage)
	}
	if g.P[1].JuggleH != bp.LaunchCap || !g.P[1].Juggle {
		t.Fatalf("thrown to %v (juggled %v), want the cap %v after %d hits", g.P[1].JuggleH, g.P[1].Juggle, bp.LaunchCap, bp.Count)
	}
}

func TestTheHeightGrowsWithEachHitUntilTheCap(t *testing.T) {
	bp := bpEverywhere()
	bp.Count, bp.LaunchCap = 3, 1000
	g := bpGame(bp, 300)
	press(g, 0, Input{Skill: true})
	if want := bp.ExplosionHeight + 2*bp.HeightGain; g.P[1].JuggleH != want {
		t.Fatalf("thrown to %v, want %v after three hits", g.P[1].JuggleH, want)
	}
	bp.Count = 1
	g = bpGame(bp, 300)
	press(g, 0, Input{Skill: true})
	if g.P[1].JuggleH != bp.ExplosionHeight {
		t.Fatalf("thrown to %v, want %v after one hit", g.P[1].JuggleH, bp.ExplosionHeight)
	}
}

func TestAFoeInTheAirIsHitAgainAndThrownOnFromWhereItIs(t *testing.T) {
	bp := bpEverywhere()
	bp.Count, bp.Duration, bp.LaunchDuration, bp.LaunchCap = 4, 2, 5, 1000
	g := bpGame(bp, 300)
	press(g, 0, Input{Skill: true})
	steps(g, 30) // the first explosion has thrown it up and it is up there
	if g.P[1].Z < bp.ExplosionHeight-1 || !g.P[1].Held {
		t.Fatalf("z %v held %v: it should be up in the air", g.P[1].Z, g.P[1].Held)
	}
	steps(g, 70)
	if want := bp.ExplosionHeight + 3*bp.HeightGain; g.P[1].JuggleH != want || g.P[1].Z < want-1 {
		t.Fatalf("height %v, z %v, want it thrown on to %v by the later explosions", g.P[1].JuggleH, g.P[1].Z, want)
	}
}

func TestTheExplosionsAreSpreadOverTheDurationAndFillTheWholeCylinder(t *testing.T) {
	bp := bpEverywhere()
	bp.Count, bp.Duration, bp.Radius, bp.Height, bp.ExplosionRadius = 40, 2, 200, 300, 5
	g := bpGame(bp, 300)
	g.P[1].X, g.P[1].Y = 1900, 250 // far down the line, out of reach of everything
	var at []int
	var zs, rs []float64
	press(g, 0, Input{Skill: true})
	for n := 0; n < 90; n++ {
		for _, e := range g.Step(dt) {
			if e.T == "bpboom" {
				at = append(at, n)
				zs = append(zs, e.Z)
				rs = append(rs, math.Hypot(e.X-(300+bp.Distance), e.Y-250))
			}
		}
	}
	// the one let off in the pressing tick was not counted above
	if len(at)+1 != bp.Count {
		t.Fatalf("%d explosions, want %d", len(at)+1, bp.Count)
	}
	if mid := at[len(at)/2]; mid < 25 || mid > 40 {
		t.Fatalf("half of them were over by tick %d, want about the middle of the %v seconds", mid, bp.Duration)
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	var far float64
	for k, z := range zs {
		lo, hi = math.Min(lo, z), math.Max(hi, z)
		far = math.Max(far, rs[k])
		if z < 0 || z > bp.Height || rs[k] > bp.Radius+1e-9 {
			t.Fatalf("an explosion at height %v, %v from the middle: outside the cylinder", z, rs[k])
		}
	}
	if lo > bp.Height*0.2 || hi < bp.Height*0.8 || far < bp.Radius*0.7 {
		t.Fatalf("heights %v..%v and reach %v: the explosions should fill the cylinder", lo, hi, far)
	}
}

func TestAnExplosionOnlyHurtsWhatItsSphereTouches(t *testing.T) {
	bp := bpEverywhere()
	bp.ExplosionRadius = 30
	g := bpGame(bp, 300)
	g.P[1].X, g.P[1].Y = 300+bp.Distance+500, 250 // far from the cylinder
	hp := g.P[1].HP
	press(g, 0, Input{Skill: true})
	if g.P[1].HP != hp || g.P[1].Juggle {
		t.Fatal("a foe far from every explosion was hurt")
	}
	g = bpGame(bp, 300)
	g.P[1].X, g.P[1].Y, g.P[1].Z = 300+bp.Distance, 250, 400 // right above it, but the cylinder is only 10 tall
	hp = g.P[1].HP
	press(g, 0, Input{Skill: true})
	if g.P[1].HP != hp {
		t.Fatal("a foe high above the cylinder was hurt")
	}
	g = bpGame(bp, 300)
	g.P[1].X, g.P[1].Y, g.P[1].Z = 300+bp.Distance, 250, -10 // under the ground
	hp = g.P[1].HP
	press(g, 0, Input{Skill: true})
	if g.P[1].HP != hp {
		t.Fatal("a foe under the ground was hurt")
	}
}

func TestACastGoesOnWhateverBecomesOfTheOneWhoThrewItAndEndsWithTheRound(t *testing.T) {
	bp := bpEverywhere()
	bp.Duration = 2
	g := bpGame(bp, 300)
	press(g, 0, Input{Skill: true})
	g.P[0].GmaxT = 0 // it is no longer Gigantamax
	g.freeze(&g.P[0], 1)
	hp := g.P[1].HP
	steps(g, 30)
	if g.P[1].HP >= hp {
		t.Fatal("the explosions stopped with the thrower")
	}
	g.Reset()
	if len(g.Bursts) != 0 {
		t.Fatal("a cast survived the round")
	}
}

func TestTheBurstPunchesNumbersAreChecked(t *testing.T) {
	for name, edit := range map[string]func(*BurstPunchCfg){
		"count 0":             func(b *BurstPunchCfg) { b.Count = 0 },
		"count too many":      func(b *BurstPunchCfg) { b.Count = maxBursts + 1 },
		"radius":              func(b *BurstPunchCfg) { b.Radius = 0 },
		"explosion radius":    func(b *BurstPunchCfg) { b.ExplosionRadius = 0 },
		"explosion height":    func(b *BurstPunchCfg) { b.ExplosionHeight = 0 },
		"negative gain":       func(b *BurstPunchCfg) { b.HeightGain = -1 },
		"cap below the first": func(b *BurstPunchCfg) { b.LaunchCap = b.ExplosionHeight - 1 },
		"negative duration":   func(b *BurstPunchCfg) { b.Duration = -1 },
	} {
		c := DefaultConfig()
		edit(&c.Machamp.Gigantamax.BurstPunch)
		if c.validate() == nil {
			t.Fatalf("%s: a nonsense number was accepted", name)
		}
	}
}

func TestTheGigantamaxMachampBotBurstsWhenTheFoeIsInTheCylinderAndNeverUsesTheUppercut(t *testing.T) {
	bp := bpEverywhere()
	bp.Radius, bp.Distance = 200, 450
	g := bpGame(bp, 450)
	bot := newBot(machamp, 2)
	var burst, upper bool
	for n := 0; n < 4*tickRate && !burst; n++ {
		in := bot.Decide(g, 0)
		burst, upper = burst || in.Skill, upper || (in.Skill && g.P[0].UpCD != 0)
		g.P[0].In = in
		g.Step(dt)
	}
	if !burst || g.P[0].UpPhase != upNone || g.P[0].BpCD == 0 {
		t.Fatalf("burst %v, uppercut phase %d, cooldown %v: it should have started a Burst Punch", burst, g.P[0].UpPhase, g.P[0].BpCD)
	}
}

func TestABotKeepsOutOfABurstPunchesCylinder(t *testing.T) {
	bp := bpEverywhere()
	bp.Radius, bp.ExplosionRadius, bp.Distance, bp.ChargeDuration = 150, 60, 300, 1
	g := bpGame(bp, 300)
	g.P[0].Kind, g.P[1].Kind = machamp, machamp
	g.P[1].GmaxT = 0
	press(g, 0, Input{Skill: true}) // the Burst Punch is gathering, aimed at slot 1, which stands in its cylinder
	bot := newBot(machamp, 2)
	var moved bool
	for n := 0; n < 12; n++ { // it sees the gathering a few ticks late
		in := bot.Decide(g, 1)
		moved = moved || in.DX != 0 || in.DY != 0
		g.P[1].In = in
		g.Step(dt)
	}
	if !moved {
		t.Fatal("the bot stood in the cylinder of a Burst Punch that was gathering")
	}
}

func TestTheBurstPunchSlidesAlongTheStickBeforeItGathers(t *testing.T) {
	bp := bpEverywhere()
	bp.SlideDistance, bp.SlideDuration, bp.ChargeDuration = 150, 0.3, 0.5
	g := bpGame(bp, 700)
	g.P[1].Y = 900 // far off to the side: the slide is not at the foe
	x0, y0 := g.P[0].X, g.P[0].Y
	press(g, 0, Input{Skill: true, DY: -1})
	n := 1
	for g.P[0].BpS > 0 {
		if g.P[0].BpT != 0 || len(g.Bursts) != 0 {
			t.Fatal("it gathered or punched while it slid")
		}
		g.P[0].In = Input{DX: 1} // the stick moves on: the slide keeps its way
		g.Step(dt)
		n++
	}
	if g.P[0].X != x0 || math.Abs((y0-g.P[0].Y)-bp.SlideDistance) > 1e-6 {
		t.Fatalf("slid to %v,%v from %v,%v, want %v along the stick", g.P[0].X, g.P[0].Y, x0, y0, bp.SlideDistance)
	}
	if want := int(math.Ceil(bp.SlideDuration * tickRate)); n < want-1 || n > want+1 {
		t.Fatalf("the slide took %d ticks, want about %d", n, want)
	}
	if g.P[0].BpT <= 0 {
		t.Fatal("it did not go on to gather")
	}
	g.P[0].In = Input{}
	px, py := g.P[0].X, g.P[0].Y
	steps(g, 40)
	if g.P[0].X != px || g.P[0].Y != py {
		t.Fatal("it moved while it gathered")
	}
}

func TestTheBurstPunchSlidesTheWayItFacesWithNoStickAndNotWhenHeld(t *testing.T) {
	bp := bpEverywhere()
	bp.SlideDistance, bp.SlideDuration = 150, 0.3
	g := bpGame(bp, 700)
	g.P[1].Y = 900
	g.P[0].FX, g.P[0].FY = -1, 0
	x0 := g.P[0].X
	press(g, 0, Input{Skill: true})
	steps(g, 15)
	if math.Abs((x0-g.P[0].X)-bp.SlideDistance) > 1e-6 {
		t.Fatalf("slid %v the way it faces, want %v", x0-g.P[0].X, bp.SlideDistance)
	}
	g = bpGame(bp, 700)
	g.P[1].Y = 900
	g.P[0].BoundT = 5 // a vine holds it
	x0 = g.P[0].X
	press(g, 0, Input{Skill: true, DX: 1})
	steps(g, 15)
	if g.P[0].X != x0 {
		t.Fatal("a held Machamp slid")
	}
}

func TestTheExplosionsClimbFromTheGroundUp(t *testing.T) {
	bp := bpEverywhere()
	bp.Count, bp.Duration, bp.Height, bp.Radius, bp.ExplosionRadius = 20, 1, 400, 100, 5
	g := bpGame(bp, 300)
	g.P[1].X, g.P[1].Y = 1900, 250
	var zs []float64
	for _, e := range press(g, 0, Input{Skill: true}) {
		if e.T == "bpboom" {
			zs = append(zs, e.Z)
		}
	}
	for n := 0; n < 40; n++ {
		for _, e := range g.Step(dt) {
			if e.T == "bpboom" {
				zs = append(zs, e.Z)
			}
		}
	}
	if len(zs) != bp.Count {
		t.Fatalf("%d explosions, want %d", len(zs), bp.Count)
	}
	for n, z := range zs {
		if lo, hi := float64(n)/float64(bp.Count)*bp.Height, float64(n+1)/float64(bp.Count)*bp.Height; z < lo-1e-9 || z > hi+1e-9 {
			t.Fatalf("explosion %d at height %v, want it in the slice %v..%v: they should climb", n, z, lo, hi)
		}
	}
}

func TestTheBurstPunchSlideIsChecked(t *testing.T) {
	c := DefaultConfig()
	c.Machamp.Gigantamax.BurstPunch.SlideDistance = -1
	if c.validate() == nil {
		t.Fatal("a negative slide was accepted")
	}
}

func TestTheExplosionColoursAreChecked(t *testing.T) {
	for name, edit := range map[string]func(*BurstPunchCfg){
		"empty":     func(b *BurstPunchCfg) { b.ExplosionColor1 = "" },
		"not color": func(b *BurstPunchCfg) { b.ExplosionColor2 = "red;</script>" },
		"too long":  func(b *BurstPunchCfg) { b.ExplosionColor1 = "#" + strings.Repeat("a", 80) },
	} {
		c := DefaultConfig()
		edit(&c.Machamp.Gigantamax.BurstPunch)
		if c.validate() == nil {
			t.Fatalf("%s: a bad colour was accepted", name)
		}
	}
	c := DefaultConfig()
	c.Machamp.Gigantamax.BurstPunch.ExplosionColor1, c.Machamp.Gigantamax.BurstPunch.ExplosionColor2 = "rgba(255, 0, 0, 0.5)", "hsl(120, 50%, 50%)"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}
