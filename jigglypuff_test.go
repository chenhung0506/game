package main

import (
	"math"
	"testing"
)

// jig is a Jigglypuff (slot 0, at 300,250) with a Snorlax (slot 1) `gap` in front of it.
func jig(gap float64) *Game {
	g := duel(DefaultConfig(), jigglypuff, snorlax)
	g.P[1].X, g.P[1].Y = g.P[0].X+gap, g.P[0].Y
	return g
}

// swing presses the main button and runs until the swing has landed.
func swing(g *Game, n int) {
	press(g, 0, Input{Skill: true})
	steps(g, int(g.cfg.Jigglypuff.Combo.Ground.Duration[n]*tickRate)+2)
}

func TestGroundChainHitsFiveTimesThenBlowsTheFoeAway(t *testing.T) {
	g := jig(80)
	gr := g.cfg.Jigglypuff.Combo.Ground
	hp, x0 := g.P[1].HP, g.P[1].X
	want := 0
	for n := 0; n < 5; n++ {
		g.P[1].StunT = 0
		swing(g, n)
		want += gr.Damage[n]
		if g.P[1].HP != hp-want {
			t.Fatalf("after blow %d foe HP %d, want %d", n+1, g.P[1].HP, hp-want)
		}
		if n == 3 && g.P[0].SlapCD != 0 {
			t.Fatal("cooldown started before the last blow")
		}
	}
	if g.P[1].X-x0 < gr.Knockback[4] {
		t.Fatalf("the last blow moved the foe %v, want at least %v", g.P[1].X-x0, gr.Knockback[4])
	}
	if g.P[0].SlapCD <= 0 || g.P[0].SlapChain {
		t.Fatalf("after the fifth blow: cooldown %v, chain %v", g.P[0].SlapCD, g.P[0].SlapChain)
	}
}

func TestPressMadeDuringASwingStartsTheNextOneWhenItLands(t *testing.T) {
	g := jig(80)
	press(g, 0, Input{Skill: true})
	g.Step(dt)                      // let go
	press(g, 0, Input{Skill: true}) // a second press while the first swing is on
	steps(g, int(g.cfg.Jigglypuff.Combo.Ground.Duration[0]*tickRate)+2)
	if g.P[0].SlapN != 1 || g.P[0].SlapT <= 0 {
		t.Fatalf("SlapN %d SlapT %v: the second blow should have started by itself", g.P[0].SlapN, g.P[0].SlapT)
	}
}

func TestHoldingTheButtonIsOnePressNotAChain(t *testing.T) {
	g := jig(80)
	g.P[0].In = Input{Skill: true}
	steps(g, int(1*tickRate))
	if g.P[0].SlapN > 1 {
		t.Fatalf("a held button went through %d blows", g.P[0].SlapN)
	}
}

func TestChainEndsWhenItsWindowRunsOut(t *testing.T) {
	g := jig(80)
	swing(g, 0)
	if g.P[0].SlapCD != 0 || !g.P[0].SlapChain {
		t.Fatal("the chain should be open after one blow")
	}
	steps(g, int((g.cfg.Jigglypuff.Combo.UseWindow+0.2)*tickRate))
	if g.P[0].SlapChain || g.P[0].SlapCD <= 0 {
		t.Fatalf("chain %v, cooldown %v: the window should have closed and the cooldown begun", g.P[0].SlapChain, g.P[0].SlapCD)
	}
	press(g, 0, Input{Skill: true})
	if g.P[0].SlapT != 0 {
		t.Fatal("a blow started during the cooldown")
	}
}

func TestSwingIsRootedAndCutShortByAStun(t *testing.T) {
	g := jig(80)
	hp, x0 := g.P[1].HP, g.P[0].X
	press(g, 0, Input{Skill: true})
	g.P[0].In = Input{DX: -1}
	g.Step(dt)
	if g.P[0].X != x0 {
		t.Fatal("walked while swinging")
	}
	g.stun(&g.P[0], 1)
	steps(g, int(0.6*tickRate))
	if g.P[1].HP != hp || g.P[0].SlapT != 0 || g.P[0].SlapChain {
		t.Fatalf("foe HP %d -> %d, SlapT %v, chain %v: a stunned Jigglypuff must not land the blow", hp, g.P[1].HP, g.P[0].SlapT, g.P[0].SlapChain)
	}
}

// airJig is a Jigglypuff in the air (as if after a kick) with the foe on the ground under it.
func airJig() *Game {
	g := jig(40)
	g.P[0].Z, g.P[0].JpFly, g.P[0].VZ = 150, true, 0
	return g
}

func airSwing(g *Game, n int) {
	press(g, 0, Input{Skill: true})
	steps(g, int(g.cfg.Jigglypuff.Combo.Air.Duration[n]*tickRate)+2)
}

func TestAirChainStompsTheFoeIntoABallThatBouncesThenKicksItAway(t *testing.T) {
	g := airJig()
	air := g.cfg.Jigglypuff.Combo.Air
	hp := g.P[1].HP
	airSwing(g, 0)
	f := &g.P[1]
	if f.HP != hp-air.Damage[0] || !f.ArcOn || !f.Held {
		t.Fatalf("after the first stomp: HP %d, ArcOn %v, Held %v", f.HP, f.ArcOn, f.Held)
	}
	if g.P[0].VZ <= 0 {
		t.Fatalf("Jigglypuff should bounce up off the foe, VZ %v", g.P[0].VZ)
	}
	// the foe lands and bounces up to the configured height, drifting away
	x := f.X
	top := 0.0
	for i := 0; i < int(0.6*tickRate); i++ {
		g.Step(dt)
		top = math.Max(top, f.Z)
	}
	if top < air.FoeBounceHeight[0]*0.9 || top > air.FoeBounceHeight[0]*1.1 {
		t.Fatalf("the foe bounced to %v, want about %v", top, air.FoeBounceHeight[0])
	}
	if f.X-x < air.FoeBounceDistance[0]*0.5 {
		t.Fatalf("the foe drifted %v, want about %v", f.X-x, air.FoeBounceDistance[0])
	}
	// second stomp and the kick: bring the bodies together so the test is about the chain, not the chase
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	g.P[0].Z, g.P[0].VZ, f.VZ = f.Z+20, 400, 300 // both still rising, so neither lands before the blow
	airSwing(g, 1)
	if !f.ArcOn || f.HP != hp-air.Damage[0]-air.Damage[1] {
		t.Fatalf("after the second stomp: HP %d, ArcOn %v", f.HP, f.ArcOn)
	}
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	g.P[0].Z, g.P[0].VZ, f.VZ = f.Z+20, 400, 300
	x = f.X
	airSwing(g, 2)
	if f.HP != hp-air.Damage[0]-air.Damage[1]-air.Damage[2] || f.X-x < air.Knockback[2] {
		t.Fatalf("after the kick: HP %d, moved %v, want knock-back %v", f.HP, f.X-x, air.Knockback[2])
	}
	if g.P[0].SlapChain || g.P[0].SlapCD <= 0 {
		t.Fatal("the chain should be over with its cooldown running")
	}
}

func TestAirStompThatMissesEndsTheChain(t *testing.T) {
	g := airJig()
	g.P[1].X += 400
	airSwing(g, 0)
	if g.P[0].SlapChain || g.P[0].SlapCD <= 0 {
		t.Fatal("a stomp that hit nobody must end the chain")
	}
}

func TestLandingEndsTheAirChain(t *testing.T) {
	g := airJig()
	g.P[1].X += 400
	g.P[0].SlapChain, g.P[0].SlapAir = true, true
	steps(g, int(2*tickRate))
	if g.P[0].Z != 0 || g.P[0].JpFly || g.P[0].SlapChain {
		t.Fatalf("Z %v, flying %v, chain %v: it should have landed and ended the chain", g.P[0].Z, g.P[0].JpFly, g.P[0].SlapChain)
	}
}

func TestRollKickRollsTheConfiguredDistanceAtTheConfiguredSpeed(t *testing.T) {
	g := jig(900)
	g.P[1].X, g.P[1].Y = 1500, 900
	rk := g.cfg.Jigglypuff.RollKick
	x0 := g.P[0].X
	press(g, 0, Input{Roll: true, DX: 1})
	n := 0
	for g.P[0].RollT > 0 && n < 500 {
		g.Step(dt)
		n++
	}
	if got := g.P[0].X - x0; math.Abs(got-rk.RollDistance) > 1e-6 {
		t.Fatalf("rolled %v, want %v", got, rk.RollDistance)
	}
	if want := rk.RollDistance / rk.RollSpeed * tickRate; math.Abs(float64(n)-want) > 2 {
		t.Fatalf("rolled for %d ticks, want about %v", n, want)
	}
	if g.P[0].RollCD <= 0 {
		t.Fatal("no cooldown")
	}
}

func TestRollKickSendsTheFoeUpAtAnAngleStunsItAndTakesJigglypuffAlong(t *testing.T) {
	g := jig(120)
	rk := g.cfg.Jigglypuff.RollKick
	hp, x1 := g.P[1].HP, g.P[1].X
	press(g, 0, Input{Roll: true, DX: 1})
	steps(g, int((rk.RollDistance/rk.RollSpeed+0.1)*tickRate))
	f, p := &g.P[1], &g.P[0]
	if f.HP != hp-rk.Damage || !f.ArcOn || f.StunT <= 0 {
		t.Fatalf("foe HP %d (want %d), ArcOn %v, StunT %v", f.HP, hp-rk.Damage, f.ArcOn, f.StunT)
	}
	if !p.JpFly || p.VZ <= 0 {
		t.Fatalf("Jigglypuff should fly up with it: flying %v, VZ %v", p.JpFly, p.VZ)
	}
	topF, topP := 0.0, 0.0
	for i := 0; i < int(2*tickRate); i++ {
		g.Step(dt)
		topF, topP = math.Max(topF, f.Z), math.Max(topP, p.Z)
	}
	if math.Abs(topF-rk.KickHeight) > rk.KickHeight*0.1 || math.Abs(topP-rk.SelfHeight) > rk.SelfHeight*0.1 {
		t.Fatalf("heights: foe %v (want %v), Jigglypuff %v (want %v)", topF, rk.KickHeight, topP, rk.SelfHeight)
	}
	if f.ArcOn || f.Held || p.JpFly {
		t.Fatal("both should have landed")
	}
	if got := f.X - x1; math.Abs(got-rk.KickDistance) > rk.KickDistance*0.1 {
		t.Fatalf("the foe flew %v, want about %v", got, rk.KickDistance)
	}
}

func TestRollKickMissesAFoeOutOfReach(t *testing.T) {
	g := jig(600)
	hp := g.P[1].HP
	press(g, 0, Input{Roll: true, DX: 1})
	steps(g, int(1*tickRate))
	if g.P[1].HP != hp || g.P[0].JpFly {
		t.Fatal("kicked a foe that was out of reach")
	}
}

func TestJigglypuffConfigListsAreChecked(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.validateJigglypuff(); err != nil {
		t.Fatal(err)
	}
	cfg.Jigglypuff.Combo.Ground.Damage = []int{1, 2}
	if cfg.validateJigglypuff() == nil {
		t.Fatal("a short damage list was accepted")
	}
	cfg = DefaultConfig()
	cfg.Jigglypuff.RollKick.RollSpeed = 0
	if cfg.validateJigglypuff() == nil {
		t.Fatal("a roll speed of 0 was accepted")
	}
}

func TestJigglypuffIsPickableAndHasItsOwnBody(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.maxHPOf(jigglypuff) != cfg.Jigglypuff.MaxHP || cfg.radiusOf(jigglypuff) != cfg.Jigglypuff.Radius || cfg.walkSpeedOf(jigglypuff) != cfg.Jigglypuff.WalkSpeed {
		t.Fatal("Jigglypuff's HP, size and speed are not its own")
	}
	if !teamRoom().choose(0, jigglypuff) {
		t.Fatal("Jigglypuff should be pickable")
	}
}

func TestAirStompBouncesJigglypuffTheConfiguredDistanceTheWayItAimed(t *testing.T) {
	g := airJig()
	g.cfg.Jigglypuff.Combo.Air.SelfBounceDistance = []float64{150, 0}
	p := &g.P[0]
	x0 := p.X
	airSwing(g, 0)
	if !p.JpFly || p.JpVX <= 0 || math.Abs(p.JpVY) > 1e-6 {
		t.Fatalf("it should drift along the aim (+x): flying %v, JpVX %v, JpVY %v", p.JpFly, p.JpVX, p.JpVY)
	}
	for i := 0; i < int(5*tickRate) && p.JpFly; i++ {
		g.Step(dt)
	}
	if p.JpFly {
		t.Fatal("it should have landed")
	}
	if got := p.X - x0; math.Abs(got-150) > 20 { // the swing itself took a moment, in which the foe was already under it
		t.Fatalf("it drifted %v, want about 150", got)
	}
}

func TestAirStompWithNoSelfBounceDistanceBouncesStraightUp(t *testing.T) {
	g := airJig()
	g.cfg.Jigglypuff.Combo.Air.SelfBounceDistance = []float64{0, 0}
	airSwing(g, 0)
	if p := &g.P[0]; p.JpVX != 0 || p.JpVY != 0 {
		t.Fatalf("drift %v,%v, want none", p.JpVX, p.JpVY)
	}
}

func TestGroundBlowsTellThePageWhichHandDidIt(t *testing.T) {
	g := jig(80)
	want := []int{0, 3, 3, 3, 0} // right, left, then the right-left pairs end on the left, and the big blow is the right
	for n := 0; n < 5; n++ {
		g.P[1].StunT = 0
		swing(g, n)
		if g.P[0].JpPose != want[n] {
			t.Fatalf("blow %d: JpPose %d, want %d", n+1, g.P[0].JpPose, want[n])
		}
	}
}

func TestGroundBlowStepsForwardTheConfiguredDistanceAndStopsAtTheFoe(t *testing.T) {
	g := jig(300)
	g.cfg.Jigglypuff.Combo.Ground.StepDistance = []float64{80, 0, 0, 0, 0}
	x0 := g.P[0].X
	swing(g, 0)
	steps(g, int(0.3*tickRate))
	if got := g.P[0].X - x0; math.Abs(got-80) > 1 {
		t.Fatalf("it stepped %v, want 80", got)
	}
	// with the foe right in front it cannot step into its body
	g = jig(70)
	g.cfg.Jigglypuff.Combo.Ground.StepDistance = []float64{200, 0, 0, 0, 0}
	swing(g, 0)
	steps(g, int(0.3*tickRate))
	if d := math.Hypot(g.P[1].X-g.P[0].X, g.P[1].Y-g.P[0].Y); d < g.radius(0)+g.radius(1)-1e-6 {
		t.Fatalf("it walked into the foe: %v apart", d)
	}
}

func TestKickReachesAFoeFlyingAboveJigglypuff(t *testing.T) {
	g := airJig()
	g.cfg.Jigglypuff.Combo.Air.Radius = 120
	air := g.cfg.Jigglypuff.Combo.Air
	airSwing(g, 0)
	f, p := &g.P[1], &g.P[0]
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.VZ = f.Z+20, 400, 300 // both still rising, so neither lands before the second blow
	airSwing(g, 1)
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.Z, f.VZ = 20, 100, 90, 0 // the ball is above it now
	hp := f.HP
	airSwing(g, 2)
	if f.HP != hp-air.Damage[2] {
		t.Fatalf("the kick missed a foe 70 above it: HP %d, want %d", f.HP, hp-air.Damage[2])
	}
}

func TestLandingInTheMiddleOfTheKickSwingStillKicks(t *testing.T) {
	g := airJig()
	airSwing(g, 0)
	f, p := &g.P[1], &g.P[0]
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.VZ = f.Z+20, 400, 300
	airSwing(g, 1)
	if !p.SlapChain {
		t.Fatal("the chain should be waiting for the kick")
	}
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.Z, f.VZ = 20, -300, 20, 0
	hp := f.HP
	press(g, 0, Input{Skill: true})
	steps(g, 4) // it lands while the swing is on
	if p.Z != 0 || !p.SlapChain || p.SlapT <= 0 {
		t.Fatalf("it should have landed with the swing still on: Z %v, chain %v, SlapT %v", p.Z, p.SlapChain, p.SlapT)
	}
	steps(g, int(g.cfg.Jigglypuff.Combo.Air.Duration[2]*tickRate)+2)
	if f.HP >= hp {
		t.Fatalf("the kick was lost: HP %d", f.HP)
	}
}

func TestSelfGravityOnlyChangesJigglypuffsOwnFlight(t *testing.T) {
	flight := func(selfG float64) (self, foe int) {
		g := airJig()
		g.cfg.Jigglypuff.SelfGravity = selfG
		airSwing(g, 0)
		p, f := &g.P[0], &g.P[1]
		for i := 0; i < int(10*tickRate) && (p.JpFly || f.ArcOn); i++ {
			g.Step(dt)
			if p.JpFly {
				self++
			}
			if f.ArcOn {
				foe++
			}
		}
		return
	}
	s1, f1 := flight(1500)
	s2, f2 := flight(600)
	if s2 <= s1+5 {
		t.Fatalf("a lower self_gravity should keep Jigglypuff up longer: %d ticks, then %d", s1, s2)
	}
	if f1 != f2 {
		t.Fatalf("the foe's ball flight must not change with self_gravity: %d ticks, then %d", f1, f2)
	}
}

func TestSelfGravityAlsoPullsTheFoeJigglypuffKicked(t *testing.T) {
	fall := func(selfG, g2 float64) int {
		g := jig(120)
		g.cfg.Jigglypuff.SelfGravity, g.cfg.Jigglypuff.Gravity = selfG, g2
		rk := g.cfg.Jigglypuff.RollKick
		press(g, 0, Input{Roll: true, DX: 1})
		steps(g, int((rk.RollDistance/rk.RollSpeed+0.1)*tickRate))
		f := &g.P[1]
		if !f.ArcOn {
			t.Fatal("the foe should have been kicked into the air")
		}
		n := 0
		for ; f.ArcOn && n < int(10*tickRate); n++ {
			g.Step(dt)
		}
		return n
	}
	slow, fast := fall(600, 1500), fall(1500, 1500)
	if slow <= fast+5 {
		t.Fatalf("a kicked foe should stay up longer under a lower self_gravity: %d ticks vs %d", slow, fast)
	}
	if again := fall(600, 3000); again != slow {
		t.Fatalf("the kicked foe must not depend on `gravity` (that is for stomped ones): %d vs %d ticks", again, slow)
	}
}

func TestGroundBlowsStunOnlyWhereTheListSaysSo(t *testing.T) {
	g := jig(80)
	g.cfg.Jigglypuff.Combo.Ground.StunDuration = []float64{0, 0.7, 0, 0, 1.5}
	want := []float64{0, 0.7, 0, 0, 1.5}
	for n := 0; n < 5; n++ {
		g.P[1].StunT = 0
		swing(g, n)
		if got := g.P[1].StunT; math.Abs(got-want[n]) > 0.1 {
			t.Fatalf("blow %d: stunned %v, want %v", n+1, got, want[n])
		}
	}
}

func TestFirstGroundBlowClosesInBeforeItLands(t *testing.T) {
	g := jig(400)
	gr := &g.cfg.Jigglypuff.Combo.Ground
	gr.LungeDistance, gr.StepDistance = 90, []float64{0, 0, 0, 0, 0}
	x0 := g.P[0].X
	press(g, 0, Input{Skill: true})
	steps(g, int(gr.Duration[0]*tickRate)-2) // the blow has not landed yet
	if g.P[0].SlapN != 0 || g.P[0].SlapT <= 0 {
		t.Fatalf("the first blow should still be coming down: SlapN %d SlapT %v", g.P[0].SlapN, g.P[0].SlapT)
	}
	if moved := g.P[0].X - x0; moved < 60 {
		t.Fatalf("it should be closing in during the first swing, moved only %v", moved)
	}
	steps(g, 10)
	if moved := g.P[0].X - x0; math.Abs(moved-90) > 1 {
		t.Fatalf("it moved %v, want 90", moved)
	}
	// only the first blow: the second does not lunge
	x1 := g.P[0].X
	swing(g, 1)
	if g.P[0].X != x1 {
		t.Fatalf("the second blow moved it %v", g.P[0].X-x1)
	}
}

func TestRollIsAllowedBetweenTheBlowsOfAChainAndEndsIt(t *testing.T) {
	g := jig(80)
	swing(g, 0) // one blow done, the window is open
	if !g.P[0].SlapChain || g.P[0].SlapN != 1 {
		t.Fatalf("the chain should be waiting for blow 2: chain %v N %d", g.P[0].SlapChain, g.P[0].SlapN)
	}
	press(g, 0, Input{Roll: true, DX: 1})
	if g.P[0].RollT <= 0 {
		t.Fatal("the roll should start inside use_window")
	}
	if g.P[0].SlapChain || g.P[0].SlapCD != 0 {
		t.Fatalf("rolling should end the chain without a cooldown: chain %v, cooldown %v", g.P[0].SlapChain, g.P[0].SlapCD)
	}
}

func TestRollIsNotAllowedMidSwing(t *testing.T) {
	g := jig(80)
	press(g, 0, Input{Skill: true})
	g.Step(dt)
	press(g, 0, Input{Roll: true, DX: 1})
	if g.P[0].RollT > 0 || g.P[0].SlapT <= 0 {
		t.Fatalf("a swing in progress is rooted: RollT %v SlapT %v", g.P[0].RollT, g.P[0].SlapT)
	}
}

func TestKickedFoeBouncesUpWhenItLandsAsHighAsTheThirdFoeBounceHeight(t *testing.T) {
	g := airJig()
	g.cfg.Jigglypuff.Combo.Air.FoeBounceHeight = []float64{90, 110, 120}
	airSwing(g, 0)
	f, p := &g.P[1], &g.P[0]
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.VZ = f.Z+20, 400, 300
	airSwing(g, 1)
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.VZ = f.Z+20, 400, 300
	airSwing(g, 2) // the kick
	if !f.ArcOn {
		t.Fatal("the kicked foe should be in the air")
	}
	landed, top := false, 0.0
	for i := 0; i < int(6*tickRate); i++ {
		g.Step(dt)
		if f.Z == 0 {
			landed = true
		}
		if landed && f.Z > top {
			top = f.Z
		}
	}
	if math.Abs(top-120) > 12 {
		t.Fatalf("after landing it bounced to %v, want about 120", top)
	}
}

func TestKickedFoeDriftsTheThirdFoeBounceDistanceAsItBounces(t *testing.T) {
	g := airJig()
	g.cfg.Jigglypuff.Combo.Air.FoeBounceHeight = []float64{90, 110, 120}
	g.cfg.Jigglypuff.Combo.Air.FoeBounceDistance = []float64{40, 40, 150}
	airSwing(g, 0)
	f, p := &g.P[1], &g.P[0]
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.VZ = f.Z+20, 400, 300
	airSwing(g, 1)
	g.P[0].X, g.P[0].Y = f.X-30, f.Y
	p.Z, p.VZ, f.VZ = f.Z+20, 400, 300
	airSwing(g, 2)
	landedX, set := 0.0, false
	for i := 0; i < int(6*tickRate) && f.ArcOn; i++ {
		g.Step(dt)
		if f.Z == 0 && !set && f.VZ > 0 { // just bounced
			landedX, set = f.X, true
		}
	}
	if !set {
		t.Fatal("the kicked foe never bounced")
	}
	if got := f.X - landedX; math.Abs(got-150) > 15 {
		t.Fatalf("it drifted %v during the bounce, want about 150", got)
	}
}
