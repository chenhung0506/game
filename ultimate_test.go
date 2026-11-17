package main

import (
	"math"
	"testing"
)

// pika is Pikachu (slot 0, at 300,250) against foe (slot 1, at 900,250), the charge of its ultimate full when full is set.
func pika(cfg Config, foe int, full bool) *Game {
	g := duel(cfg, pikachu, foe)
	g.P[0].HP, g.P[1].HP = g.maxHP(0), g.maxHP(1)
	if full {
		g.P[0].Charge = cfg.Pikachu.Ultimate.Max
	}
	return g
}

// ultimate presses Z and plays the gathering through: it returns on the tick the bolts leave.
func ultimate(g *Game) {
	gpress(g, Input{Ult: true})
	for n := 0; g.P[0].UltT > 0 && n < 10*tickRate; n++ {
		g.Step(dt)
	}
}

func volts(g *Game) []Ball {
	var out []Ball
	for _, b := range g.Balls {
		if b.K == ballVolt {
			out = append(out, b)
		}
	}
	return out
}

func TestUltimateStaysLockedUntilTheChargeIsFull(t *testing.T) {
	cfg := DefaultConfig()
	g := pika(cfg, snorlax, false)
	g.P[0].Charge = cfg.Pikachu.Ultimate.Max - 1
	ultimate(g)
	if len(volts(g)) != 0 || g.P[0].Charge < cfg.Pikachu.Ultimate.Max-1 {
		t.Fatalf("it went off at %v of %v: %d bolts", g.P[0].Charge, cfg.Pikachu.Ultimate.Max, len(volts(g)))
	}
}

func TestUltimateChargeFillsFromTimeFromDamageTakenAndFromDamageDealtEachOnItsOwnRate(t *testing.T) {
	for _, tc := range []struct {
		name          string
		taken, dealt  float64
		perSecond     float64
		hurtMe, hurtF int // HP the pikachu loses, HP its foe loses, in one tick
		want          float64
	}{
		{"time only", 0, 0, 3, 0, 0, 3 * dt},
		{"taken only", 2, 0, 0, 10, 0, 20},
		{"dealt only", 0, 5, 0, 0, 4, 20},
		{"all three", 2, 5, 3, 10, 4, 20 + 20 + 3*dt},
		{"all zero", 0, 0, 0, 10, 4, 0},
	} {
		cfg := DefaultConfig()
		cfg.Pikachu.Ultimate = Ultimate{Max: 1000, GainTaken: tc.taken, GainDealt: tc.dealt, GainPerSecond: tc.perSecond}
		g := pika(cfg, snorlax, false)
		hp0 := [maxSlots]int{g.P[0].HP, g.P[1].HP}
		g.P[0].HP -= tc.hurtMe // within the tick
		g.P[1].HP -= tc.hurtF
		g.gainCharge(hp0, dt)
		if math.Abs(g.P[0].Charge-tc.want) > 1e-9 {
			t.Errorf("%s: charge %v, want %v", tc.name, g.P[0].Charge, tc.want)
		}
	}
}

func TestUltimateChargeCountsDamageFromAnySkillAndStopsAtMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Ultimate = Ultimate{Max: 30, GainTaken: 1, GainDealt: 1}
	g := pika(cfg, snorlax, false)
	g.P[1].HP -= 0
	hp0 := [maxSlots]int{g.P[0].HP, g.P[1].HP}
	g.hit(&g.P[1], 12) // a hit goes through hit(), a tornado's spin does not: both must count
	g.P[1].HP -= 8
	g.gainCharge(hp0, dt)
	if g.P[0].Charge != 20 {
		t.Fatalf("charge %v after 20 HP dealt, want 20", g.P[0].Charge)
	}
	hp0 = [maxSlots]int{g.P[0].HP, g.P[1].HP}
	g.hit(&g.P[1], 50)
	g.gainCharge(hp0, dt)
	if g.P[0].Charge != 30 {
		t.Fatalf("charge %v, want it held at max 30", g.P[0].Charge)
	}
}

func TestUltimateChargeIgnoresACharacterWithoutOneAndEmptiesWithTheRound(t *testing.T) {
	cfg := DefaultConfig()
	g := duel(cfg, snorlax, pikachu)
	g.hit(&g.P[0], 10)
	g.Step(dt)
	if g.P[0].Charge != 0 {
		t.Fatalf("snorlax has no ultimate but holds %v", g.P[0].Charge)
	}
	g = pika(cfg, snorlax, true)
	g.Reset()
	if g.P[0].Charge != 0 {
		t.Fatalf("a new round starts with %v", g.P[0].Charge)
	}
}

func TestThousandVoltSendsTheRingOutThenEmptiesTheCharge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ThousandVolt.FollowSpeed = 0 // they stay where the ring ends
	tv := cfg.Pikachu.ThousandVolt
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 2000, 900 // far: the bolts have nothing to touch yet
	ultimate(g)
	if got := len(volts(g)); got != tv.Count {
		t.Fatalf("%d bolts, want %d", got, tv.Count)
	}
	if gathered := tv.RingDuration + tv.BallDuration + dt; g.P[0].Charge > cfg.Pikachu.Ultimate.GainPerSecond*gathered+1e-9 {
		t.Fatalf("charge %v after the cast, want it emptied (and only the time of the gathering back)", g.P[0].Charge)
	}
	for _, b := range volts(g) {
		if b.HP != tv.HP || b.Owner != 0 {
			t.Fatalf("bolt %+v", b)
		}
	}
	ultimate(g) // the charge is gone: a second press does nothing
	if got := len(volts(g)); got != tv.Count {
		t.Fatalf("%d bolts after a press with no charge, want still %d", got, tv.Count)
	}
	steps(g, int(math.Ceil((tv.RingRadius/tv.SpreadSpeed)*tickRate))+1)
	for _, b := range volts(g) {
		if d := math.Hypot(b.X-g.P[0].X, b.Y-g.P[0].Y); math.Abs(d-tv.RingRadius) > 1e-6 {
			t.Fatalf("a bolt is %v from Pikachu, want about the ring radius %v", d, tv.RingRadius)
		}
	}
}

func TestThousandVoltBoltsChaseTheFoeWhereverItGoes(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.TerminalTime = 60
	tv.Knockback, tv.ContactDamage = 0, 0
	g := pika(cfg, snorlax, true)
	ultimate(g)
	steps(g, 15)
	before := func() float64 {
		var s float64
		for _, b := range volts(g) {
			s += math.Hypot(b.X-g.P[1].X, b.Y-g.P[1].Y)
		}
		return s
	}
	d0 := before()
	steps(g, 15)
	if d1 := before(); d1 >= d0-1 {
		t.Fatalf("the bolts did not close in: %v then %v", d0, d1)
	}
	g.P[1].X, g.P[1].Y = 300, 700 // the foe moves away: they follow its new place
	steps(g, 90)
	for _, b := range volts(g) {
		if d := math.Hypot(b.X-g.P[1].X, b.Y-g.P[1].Y); d > tv.Radius+g.radius(1)+1 {
			t.Fatalf("a bolt is still %v from the foe", d)
		}
	}
}

func TestThousandVoltHurtsAndPushesTheFoeItTouchesAndAgainAfterTheRehitInterval(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.Count, tv.ContactDamage, tv.Knockback, tv.RehitInterval, tv.TerminalTime = 1, 7, 50, 1, 60
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 300+tv.RingRadius+80, 250
	ultimate(g)
	var x0 float64
	n := 0
	for ; n < 3*tickRate && g.P[1].HP == g.maxHP(1); n++ {
		x0 = g.P[1].X
		g.Step(dt)
	}
	if g.P[1].HP != g.maxHP(1)-7 {
		t.Fatalf("foe HP %d, want the contact damage 7 off %d", g.P[1].HP, g.maxHP(1))
	}
	if pushed := g.P[1].X - x0; pushed < 49 {
		t.Fatalf("the foe was pushed %v, want about 50 away from the bolt", pushed)
	}
	hp := g.P[1].HP
	steps(g, tickRate/2)
	if g.P[1].HP != hp {
		t.Fatal("it hurt again before the rehit interval")
	}
	steps(g, tickRate*2)
	if g.P[1].HP >= hp {
		t.Fatal("it never hurt again after the rehit interval")
	}
}

func TestThousandVoltRehitIntervalZeroHurtsOnce(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.Count, tv.ContactDamage, tv.Knockback, tv.RehitInterval, tv.TerminalTime = 1, 7, 0, 0, 60
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 300+tv.RingRadius+40, 250
	ultimate(g)
	steps(g, 5*tickRate)
	if g.P[1].HP != g.maxHP(1)-7 {
		t.Fatalf("foe HP %d, want exactly one hit of 7 off %d", g.P[1].HP, g.maxHP(1))
	}
}

func TestThousandVoltAllExplodeAtTerminalTimeAndAFoeInRangeIsHurtOnceForTheWholeCast(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.ContactDamage, tv.Knockback, tv.TerminalTime = 0, 0, 1
	tv.ExplosionDamage, tv.ExplosionRadius = 9, 400
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 600, 250
	ultimate(g)
	n := 1
	var booms int
	for ; len(volts(g)) > 0 && n < 5*tickRate; n++ {
		for _, e := range g.Step(dt) {
			if e.T == "voltburst" {
				booms++
			}
		}
	}
	if want := int(math.Round(tv.TerminalTime * tickRate)); n != want {
		t.Fatalf("they blew up after %d ticks, want %d", n, want)
	}
	if booms != tv.Count {
		t.Fatalf("%d explosions, want one per bolt (%d)", booms, tv.Count)
	}
	if g.P[1].HP != g.maxHP(1)-9 {
		t.Fatalf("foe HP %d, want one explosion of 9 off %d however many bolts reached it", g.P[1].HP, g.maxHP(1))
	}
}

func TestThousandVoltExplosionMissesAFoeOutOfRange(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.ContactDamage, tv.Knockback, tv.TerminalTime, tv.FollowSpeed = 0, 0, 0.5, 0
	tv.ExplosionDamage, tv.ExplosionRadius = 9, 50
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 1500, 250
	ultimate(g)
	steps(g, tickRate)
	if len(volts(g)) != 0 || g.P[1].HP != g.maxHP(1) {
		t.Fatalf("%d bolts left, foe HP %d of %d", len(volts(g)), g.P[1].HP, g.maxHP(1))
	}
}

func TestThousandVoltBoltsAreShotDownByTheFoesSkillsAndDoNotExplode(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.TerminalTime, tv.ExplosionDamage, tv.ExplosionRadius = 1, 40, 2000
	tv.ContactDamage, tv.Knockback = 0, 0
	g := pika(cfg, snorlax, true)
	ultimate(g)
	for j := range g.Balls {
		g.Balls[j].HP = 0 // worn out by the foe's skills, which take HP off every ball of the other side
	}
	steps(g, 2*tickRate)
	if len(volts(g)) != 0 || g.P[1].HP != g.maxHP(1) {
		t.Fatalf("%d bolts, foe HP %d of %d: a bolt with no HP is gone and does not blow up", len(volts(g)), g.P[1].HP, g.maxHP(1))
	}
}

func TestThousandVoltBoltsClashWithTheFoesProjectilesLikeAnyOther(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.Count, tv.HP, tv.TerminalTime, tv.ContactDamage, tv.Knockback = 1, cfg.Charizard.FireBreath.Damage, 60, 0, 0
	g := pika(cfg, charizard, true)
	ultimate(g)
	b := volts(g)[0]
	g.Balls = append(g.Balls, Ball{K: ballFire, Owner: 1, X: b.X, Y: b.Y, HP: 99, DX: 1, Left: 100})
	g.Step(dt)
	if len(volts(g)) != 0 {
		t.Fatal("a fire ball on top of the bolt did not wear it down")
	}
}

func TestThousandVoltBoltsDoNotReachAFoeAboveTheirBand(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.Count, tv.TerminalTime, tv.Knockback, tv.ExplosionDamage = 1, 60, 0, 0
	tv.MaxHeight = cfg.Rowlet.FlyHigh - 10
	g := pika(cfg, rowlet, true) // a Rowlet is always in the air, above the bolts' band
	g.P[1].X, g.P[1].Y, g.P[1].Z = 300+tv.RingRadius+30, 250, cfg.Rowlet.FlyHigh
	ultimate(g)
	steps(g, 3*tickRate)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatalf("a foe flying above the band lost %d HP", g.maxHP(1)-g.P[1].HP)
	}
}

func TestThousandVoltBoltsChaseOnlyAFoeStillStandingInA2v2(t *testing.T) {
	cfg := DefaultConfig()
	tv := &cfg.Pikachu.ThousandVolt
	tv.TerminalTime, tv.ContactDamage, tv.Knockback = 60, 0, 0
	g := NewTeamGame(cfg, modeWeb, 4)
	g.SetKinds(pikachu, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 300, 250, 900, 250
	g.P[3].X, g.P[3].Y = 1000, 700
	g.P[1].HP = 0 // the nearer foe has fallen
	g.P[0].Charge = cfg.Pikachu.Ultimate.Max
	ultimate(g)
	steps(g, 6*tickRate)
	for _, b := range volts(g) {
		if d := math.Hypot(b.X-g.P[3].X, b.Y-g.P[3].Y); d > tv.Radius+g.radius(3)+2 {
			t.Fatalf("a bolt is %v from the foe left standing, it should have gone for it", d)
		}
	}
}

func TestUltimateConfigIsValidated(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ThousandVolt.Count = maxVoltBolts + 1
	if cfg.validate() == nil {
		t.Fatal("a ring of too many bolts was accepted")
	}
	cfg = DefaultConfig()
	cfg.Pikachu.ThousandVolt.BoltCount = 21
	if cfg.validate() == nil {
		t.Fatal("21 bolts from the sky were accepted")
	}
	cfg = DefaultConfig()
	cfg.Pikachu.Ultimate.Max = 0
	if cfg.validate() == nil {
		t.Fatal("an ultimate that needs no charge was accepted")
	}
	cfg = DefaultConfig()
	cfg.Pikachu.Ultimate.GainDealt = -1
	if cfg.validate() == nil {
		t.Fatal("a negative charge rate was accepted")
	}
}

func TestThePikachuBotCastsTheUltimateOnlyOnceItIsCharged(t *testing.T) {
	cfg := liveConfig(t)
	for _, full := range []bool{false, true} {
		g := pika(cfg, snorlax, full)
		bot := newBot(pikachu, 1)
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

func TestThousandVoltGathersRootedAndOnlyThenReleasesTheBolts(t *testing.T) {
	cfg := DefaultConfig()
	tv := cfg.Pikachu.ThousandVolt
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 2000, 900
	x0 := g.P[0].X
	gpress(g, Input{Ult: true, DX: 1})
	if g.P[0].UltT <= 0 || len(volts(g)) != 0 {
		t.Fatalf("UltT %v with %d bolts: the press should only start the gathering", g.P[0].UltT, len(volts(g)))
	}
	n := 1
	for ; g.P[0].UltT > 0 && n < 10*tickRate; n++ {
		g.P[0].In = Input{DX: 1, Thunder: true, Ball: true, Skill: true} // walking and every other skill are shut out meanwhile
		g.Step(dt)
		if g.P[0].UltT > 0 && len(volts(g)) != 0 {
			t.Fatal("bolts left before the gathering was over")
		}
	}
	if want := int(math.Round((tv.RingDuration + tv.BallDuration) * tickRate)); n != want {
		t.Fatalf("it gathered for %d ticks, want %d", n, want)
	}
	if g.P[0].X != x0 || g.P[0].ThunderLeft != 0 || g.P[0].BallT != 0 || g.P[0].DashT != 0 {
		t.Fatalf("moved to %v or started another skill (thunder %d, ball %v, dash %v)", g.P[0].X, g.P[0].ThunderLeft, g.P[0].BallT, g.P[0].DashT)
	}
	if len(volts(g)) != tv.Count {
		t.Fatalf("%d bolts on the tick it went off, want %d", len(volts(g)), tv.Count)
	}
	g.P[0].In = Input{DX: 1}
	g.Step(dt)
	if g.P[0].X <= x0 {
		t.Fatal("still rooted the tick after it went off")
	}
}

func TestThousandVoltGatheringIsBrokenByAFreezeAndThenNothingGoesOff(t *testing.T) {
	cfg := DefaultConfig()
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{Ult: true})
	steps(g, 10)
	g.P[0].FrozenT = 1
	steps(g, 5*tickRate)
	if g.P[0].UltT != 0 || len(volts(g)) != 0 {
		t.Fatalf("UltT %v and %d bolts after being frozen mid-gathering", g.P[0].UltT, len(volts(g)))
	}
}

func TestThousandVoltWithNoGatheringGoesOffAtOnce(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ThousandVolt.RingDuration, cfg.Pikachu.ThousandVolt.BallDuration = 0, 0
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{Ult: true})
	if len(volts(g)) != cfg.Pikachu.ThousandVolt.Count {
		t.Fatalf("%d bolts on the press", len(volts(g)))
	}
}

func TestThousandVoltBoltBlowsUpAtItsNthTouchOfTheFoe(t *testing.T) {
	for _, hits := range []int{1, 3} {
		cfg := DefaultConfig()
		tv := &cfg.Pikachu.ThousandVolt
		tv.Count, tv.ContactDamage, tv.Knockback, tv.RehitInterval, tv.TerminalTime = 1, 2, 0, 0.2, 60
		tv.ExplosionDamage, tv.ExplosionRadius, tv.HitsToExplode = 9, 100, hits
		g := pika(cfg, snorlax, true)
		g.P[1].X, g.P[1].Y = 300+tv.RingRadius+40, 250
		ultimate(g)
		booms := 0
		for n := 0; n < 5*tickRate && booms == 0; n++ {
			for _, e := range g.Step(dt) {
				if e.T == "voltburst" {
					booms++
				}
			}
		}
		if booms != 1 || len(volts(g)) != 0 {
			t.Fatalf("hits_to_explode %d: %d explosions, %d bolts left", hits, booms, len(volts(g)))
		}
		if want := g.maxHP(1) - hits*tv.ContactDamage - tv.ExplosionDamage; g.P[1].HP != want {
			t.Fatalf("hits_to_explode %d: foe HP %d, want %d (that many touches, then one explosion)", hits, g.P[1].HP, want)
		}
	}
}

func TestThousandVoltCanKeepPikachuRootedUntilNoneOfItsBoltsIsLeft(t *testing.T) {
	for _, rooted := range []bool{true, false} {
		cfg := DefaultConfig()
		tv := &cfg.Pikachu.ThousandVolt
		tv.RootedWhileBolts, tv.TerminalTime, tv.HitsToExplode = rooted, 1, 0
		tv.ContactDamage, tv.Knockback, tv.ExplosionDamage = 0, 0, 0
		g := pika(cfg, snorlax, true)
		g.P[1].X, g.P[1].Y = 2000, 900
		ultimate(g)
		x0 := g.P[0].X
		steps(g, 5)
		g.P[0].In = Input{DX: 1, Skill: true}
		steps(g, 5)
		moved := g.P[0].X != x0
		if moved == rooted || (rooted && g.P[0].DashT != 0) {
			t.Fatalf("rooted_while_bolts %v: moved %v (dash %v) with the bolts out", rooted, moved, g.P[0].DashT)
		}
		g.P[0].In = Input{}
		for n := 0; len(volts(g)) > 0 && n < 5*tickRate; n++ {
			g.Step(dt)
		}
		x1 := g.P[0].X
		g.P[0].In = Input{DX: 1}
		steps(g, 3)
		if g.P[0].X <= x1 {
			t.Fatalf("rooted_while_bolts %v: still stuck after every bolt exploded", rooted)
		}
	}
}

func TestThousandVoltRootEndsWhenTheBoltsAreShotDownToo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ThousandVolt.RootedWhileBolts, cfg.Pikachu.ThousandVolt.TerminalTime = true, 60
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 2000, 900
	ultimate(g)
	for j := range g.Balls {
		g.Balls[j].HP = 0
	}
	g.Step(dt)
	x0 := g.P[0].X
	g.P[0].In = Input{DX: 1}
	steps(g, 3)
	if g.P[0].X <= x0 {
		t.Fatal("still rooted with every bolt shot down")
	}
}

func TestThousandVoltBoltRemembersWhereItsFlightOutEndedForTheLineBackToPikachu(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ThousandVolt.FollowSpeed = 0 // it stays where the flight out ended
	tv := cfg.Pikachu.ThousandVolt
	g := pika(cfg, snorlax, true)
	g.P[1].X, g.P[1].Y = 2000, 900
	ultimate(g)
	steps(g, tickRate)
	for _, b := range volts(g) {
		if math.Hypot(b.X-b.AnchorX, b.Y-b.AnchorY) > 1e-6 {
			t.Fatalf("bolt ended at (%v,%v) but its anchor is (%v,%v)", b.X, b.Y, b.AnchorX, b.AnchorY)
		}
		if d := math.Hypot(b.AnchorX-g.P[0].X, b.AnchorY-g.P[0].Y); math.Abs(d-tv.RingRadius) > 1e-6 {
			t.Fatalf("anchor is %v from Pikachu, want the ring radius %v", d, tv.RingRadius)
		}
	}
}

func TestOnceTheBallOfAGatheringSwellsItsCasterCannotBeStunnedOrFrozen(t *testing.T) {
	cfg := DefaultConfig()
	tv, mp := cfg.Pikachu.ThousandVolt, cfg.Ash.MegatonPunch
	for _, tc := range []struct {
		name     string
		kind     int
		set      func(p *Player, left float64)
		ringLeft float64 // seconds of gathering left while the rings are still closing
		ballLeft float64 // ...and while the ball swells
	}{
		{"pikachu", pikachu, func(p *Player, left float64) { p.UltT = left }, tv.RingDuration + tv.BallDuration - 0.1, tv.BallDuration},
		{"ash", ash, func(p *Player, left float64) { p.MegaT = left }, mp.RingDuration + mp.BallDuration - 0.1, mp.BallDuration},
	} {
		g := duel(cfg, tc.kind, snorlax)
		p := &g.P[0]
		tc.set(p, tc.ringLeft)
		if !g.freeze(p, 1) || !g.stun(p, 1) || p.FrozenT != 1 || p.StunT != 1 {
			t.Errorf("%s: in the rings it should still be freezable and stunnable (frozen %v stunned %v)", tc.name, p.FrozenT, p.StunT)
		}
		p.FrozenT, p.StunT = 0, 0
		tc.set(p, tc.ballLeft)
		if g.freeze(p, 1) || g.stun(p, 1) || p.FrozenT != 0 || p.StunT != 0 {
			t.Errorf("%s: with the ball swelling it was frozen %v or stunned %v", tc.name, p.FrozenT, p.StunT)
		}
		tc.set(p, 0)
		if !g.freeze(p, 1) || !g.stun(p, 1) {
			t.Errorf("%s: once the gathering is over it should be freezable again", tc.name)
		}
	}
}

func TestAnIcePunchDoesNotFreezeAPikachuGatheringItsBallButFreezesOneStillInTheRings(t *testing.T) {
	cfg := DefaultConfig()
	tv := cfg.Pikachu.ThousandVolt
	for _, ball := range []bool{true, false} {
		g := duel(cfg, pikachu, snorlax)
		g.P[1].X, g.P[1].Y = 420, 250
		left := tv.RingDuration + tv.BallDuration - 0.05
		if ball {
			left = tv.BallDuration
		}
		g.P[0].UltT = left
		for n := 0; n < tickRate && !g.P[1].PunchHit; n++ {
			g.P[1].In = Input{Punch: true}
			g.Step(dt)
		}
		if !g.P[1].PunchHit {
			t.Fatal("the punch never landed, the test is not testing anything")
		}
		if frozen := g.P[0].FrozenT > 0; frozen == ball {
			t.Fatalf("ball phase %v: frozen %v", ball, frozen)
		}
		if ball && g.P[0].UltT <= 0 {
			t.Fatal("the gathering was cut short though the freeze was shrugged off")
		}
	}
}
