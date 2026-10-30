package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// rowletConfig pins the numbers these tests reason about.
func rowletConfig() Config {
	cfg := DefaultConfig()
	rw := &cfg.Rowlet
	rw.FlyHigh = 50
	dr, ar, tn := &rw.CrescentDrift, &rw.Arrows, &rw.Tornado
	dr.Radius, dr.Speed, dr.Duration, dr.Cooldown = 120, 600, 0.5, 4
	ar.Reach = anyHeight
	ar.Speed, ar.Radius, ar.HP, ar.Damage, ar.Count, ar.Angle, ar.SlowDuration, ar.Slow, ar.Cooldown, ar.Mass = 500, 8, 2, 6, 5, 12, 2, 0.4, 6, 20
	ar.ChargeDuration, ar.Scale = 0, 1
	tn.Reach = anyHeight
	tn.HP, tn.Speed, tn.Radius, tn.Duration, tn.Distance = 40, 90, 50, 8, 500
	tn.ChargeDuration, tn.RechargeDuration, tn.SpinInterval, tn.SpinDamage, tn.LandingDamage = 0.4, 0.4, 0.5, 4, 20
	tn.LiftHeight, tn.SpinDuration, tn.Cooldown, tn.Mass = 200, 3.2, 12, 100
	return cfg
}

// rg is Rowlet (slot 0, at 300,250) against Snorlax (slot 1, at 900,250).
func rg(cfg Config) *Game { return duel(cfg, rowlet, snorlax) }

func ballsOfKind(g *Game, k int) (out []*Ball) {
	for j := range g.Balls {
		if g.Balls[j].K == k {
			out = append(out, &g.Balls[j])
		}
	}
	return out
}

// stepsUntilTwirl runs until slot v is in the given phase of a tornado's hold (at most limit ticks) and returns the events seen.
func stepsUntilTwirl(g *Game, v, phase, limit int) (events []Event, ok bool) {
	for range limit {
		events = append(events, g.Step(dt)...)
		if g.P[v].Twirl == phase {
			return events, true
		}
	}
	return events, false
}

// ---- the body ----

func TestRowletStartsInTheAirAndClimbsBackAfterAFall(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	if g.P[0].Z != cfg.Rowlet.FlyHigh || g.P[1].Z != 0 {
		t.Fatalf("heights %v and %v, want Rowlet at fly_high %v and Snorlax on the ground", g.P[0].Z, g.P[1].Z, cfg.Rowlet.FlyHigh)
	}
	steps(g, 30)
	if g.P[0].Z != cfg.Rowlet.FlyHigh {
		t.Fatalf("Z=%v after standing, want it to stay at %v", g.P[0].Z, cfg.Rowlet.FlyHigh)
	}
	g.P[0].Z = 0 // knocked down
	steps(g, 3)
	if want := 3 * cfg.Rowlet.FlyHigh / flyRiseTime * dt; math.Abs(g.P[0].Z-want) > 1e-9 {
		t.Fatalf("Z=%v after 3 ticks of climbing, want %v (fly_high over %vs)", g.P[0].Z, want, flyRiseTime)
	}
	steps(g, 10)
	if g.P[0].Z != cfg.Rowlet.FlyHigh {
		t.Fatalf("Z=%v, want it back at %v", g.P[0].Z, cfg.Rowlet.FlyHigh)
	}
	g.P[0].Z, g.P[0].Held = 0, true // caught: whoever holds it decides its height
	steps(g, 10)
	if g.P[0].Z != 0 {
		t.Fatalf("Z=%v while held, want the climb to wait", g.P[0].Z)
	}
}

func TestSkillsBelowRowletsHeightMissItAndSkillsFromTheSkyHitIt(t *testing.T) {
	cfg := rowletConfig() // the electro ball hits at 0..12 by default, Thunder at any height; Rowlet is at 50
	g := duel(cfg, pikachu, rowlet)
	g.P[1].X = 700
	g.P[0].In.Ball = true
	steps(g, 3*tickRate)
	if g.P[1].HP != cfg.Rowlet.MaxHP {
		t.Fatalf("Rowlet HP %d, want the electro ball (reach 0..%v) to fly under it", g.P[1].HP, cfg.Pikachu.ElectroBall.MaxHeight)
	}
	g = duel(cfg, pikachu, rowlet)
	g.P[0].In.Thunder = true
	steps(g, 3*tickRate)
	if g.P[1].HP >= cfg.Rowlet.MaxHP {
		t.Fatalf("Rowlet HP %d, want Thunder to strike it whatever its height", g.P[1].HP)
	}
}

// ---- Crescent Drift ----

func TestDriftRunsAnArcAroundACentreOnTheOpponentsSideAndEndsHeadingAround(t *testing.T) {
	cfg := rowletConfig()
	dr := cfg.Rowlet.CrescentDrift
	turn := dr.Speed * dr.Duration / dr.Radius // 2.5 rad
	for _, tc := range []struct {
		foeY, side float64
	}{{500, 1}, {0, -1}} {
		g := rg(cfg)
		g.P[1].Y = tc.foeY
		g.P[0].In = Input{Skill: true, DX: 1}
		g.Step(dt)
		for n := 1; n < int(dr.Duration*tickRate); n++ {
			g.P[0].In = Input{DX: -1, DY: -1} // the heading was locked when it started
			g.Step(dt)
		}
		p := g.P[0]
		cx, cy := 300.0, 250+tc.side*dr.Radius // the circle's centre lies on the opponent's side of the heading
		wx, wy := cx+dr.Radius*math.Sin(turn), cy-tc.side*dr.Radius*math.Cos(turn)
		if p.DriftT != 0 || math.Hypot(p.X-wx, p.Y-wy) > 1e-6 {
			t.Fatalf("side %v: at %v,%v with %v of drift left, want %v,%v on the circle of radius %v round %v,%v", tc.side, p.X, p.Y, p.DriftT, wx, wy, dr.Radius, cx, cy)
		}
		if h := math.Atan2(p.DriftY, p.DriftX); math.Abs(math.Remainder(h-tc.side*turn, 2*math.Pi)) > 1e-6 {
			t.Fatalf("side %v: ended heading %v rad, want %v", tc.side, h, tc.side*turn)
		}
		if p.Z != cfg.Rowlet.FlyHigh {
			t.Fatalf("Z=%v after the drift, want it still in the air", p.Z)
		}
	}
}

func TestDriftWithNoKeysGoesTheWayItFacesAndCooldownBlocksASecondOne(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.P[0].In.Skill = true
	g.Step(dt)
	if g.P[0].DriftX != math.Cos(cfg.Rowlet.CrescentDrift.Speed/cfg.Rowlet.CrescentDrift.Radius*dt) || g.P[0].X <= 300 {
		t.Fatalf("drift heading %v,%v at x=%v, want it to start towards +x, the way slot 0 faces", g.P[0].DriftX, g.P[0].DriftY, g.P[0].X)
	}
	steps(g, 20) // over, and the key is still held
	if g.P[0].DriftT != 0 {
		t.Fatal("a held key started a second drift inside the cooldown")
	}
	x := g.P[0].X
	steps(g, 5)
	if g.P[0].X != x {
		t.Fatal("it moved with no keys held")
	}
	steps(g, int(cfg.Rowlet.CrescentDrift.Cooldown*tickRate)-26-1) // the cooldown counts from the first tick: it ends at tick 121
	if g.P[0].DriftT != 0 {
		t.Fatalf("DriftT=%v a tick before the cooldown ends", g.P[0].DriftT)
	}
	steps(g, 2)
	if g.P[0].DriftT == 0 {
		t.Fatalf("DriftCD=%v and no drift: the held key should start one when the cooldown ends", g.P[0].DriftCD)
	}
}

func TestDriftRootsWalkingIsCutByFreezeAndStaysInTheCourt(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	g.P[0].FrozenT = 1
	g.Step(dt)
	if g.P[0].DriftT != 0 {
		t.Fatalf("DriftT=%v after freezing, want the drift cut short", g.P[0].DriftT)
	}
	g = rg(cfg)
	g.P[0].X, g.P[0].Y = 5, 5 // in the corner, heading into the wall
	g.P[0].In = Input{Skill: true, DX: -1}
	steps(g, 20)
	if p := g.P[0]; math.IsNaN(p.X) || p.X < 0 || p.Y < 0 || p.X > g.court.Length || p.Y > g.court.Width {
		t.Fatalf("at %v,%v: out of the court", p.X, p.Y)
	}
}

func TestPillarsStillBlockARowletInTheAir(t *testing.T) {
	cfg := rowletConfig()
	cfg.Vulpix.FreezeBeam.Reach = Reach{0, 80}
	g := duel(cfg, rowlet, vulpix)
	g.Balls = append(g.Balls, Ball{K: ballPillar, Owner: 1, X: 400, Y: 250, HP: 25, Life: 8})
	g.P[0].X = 300
	g.P[0].In.DX = 1
	steps(g, 3*tickRate)
	if min := g.cfg.Vulpix.FreezeBeam.PillarRadius + g.radius(0); g.P[0].X > 400-min+1e-6 {
		t.Fatalf("x=%v, want it stopped %v short of the pillar at 400", g.P[0].X, min)
	}
}

// ---- Arrows ----

func TestArrowsFanOutEvenlyAroundTheOpponent(t *testing.T) {
	cfg := rowletConfig()
	ar := cfg.Rowlet.Arrows
	g := rg(cfg)
	g.P[1].Y = 450
	g.P[0].In.Arrow = true
	g.Step(dt)
	arrows := ballsOfKind(g, ballArrow)
	if len(arrows) != ar.Count {
		t.Fatalf("%d arrows, want %d", len(arrows), ar.Count)
	}
	toFoe := math.Atan2(g.P[1].Y-g.P[0].Y, g.P[1].X-g.P[0].X)
	for k, b := range arrows {
		want := toFoe + (float64(k)-float64(ar.Count-1)/2)*ar.Angle*math.Pi/180
		if got := math.Atan2(b.DY, b.DX); math.Abs(math.Remainder(got-want, 2*math.Pi)) > 1e-9 || b.HP != ar.HP || b.Owner != 0 {
			t.Fatalf("arrow %d goes %v rad with HP %d for slot %d, want %v rad, HP %d, slot 0", k, got, b.HP, b.Owner, want, ar.HP)
		}
	}
	g.Step(dt)
	if len(ballsOfKind(g, ballArrow)) != ar.Count || g.P[0].ArrowCD <= 0 {
		t.Fatal("a held key shot a second volley inside the cooldown")
	}
}

func TestAnArrowHurtsAndSlowsAndIsSpent(t *testing.T) {
	cfg := rowletConfig()
	ar := cfg.Rowlet.Arrows
	g := rg(cfg) // 600 away: only the middle arrow of the fan (12 degrees apart) is on target
	g.P[0].In.Arrow = true
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 60)
	if g.P[1].HP != cfg.Snorlax.MaxHP-ar.Damage {
		t.Fatalf("HP %d, want %d: one arrow lands", g.P[1].HP, cfg.Snorlax.MaxHP-ar.Damage)
	}
	if g.P[1].SlowT <= 0 || g.P[1].SlowT > ar.SlowDuration {
		t.Fatalf("SlowT=%v, want a slow of up to %v s", g.P[1].SlowT, ar.SlowDuration)
	}
	for _, b := range ballsOfKind(g, ballArrow) { // the ones that missed fly on; the one that landed is spent
		if math.Abs(b.DY) < 1e-9 {
			t.Fatalf("the arrow that flew straight at the opponent is still flying at %v,%v", b.X, b.Y)
		}
	}
	x := g.P[1].X
	g.P[1].In.DX = 1
	g.Step(dt)
	if got, want := g.P[1].X-x, cfg.Snorlax.WalkSpeed*(1-ar.Slow)*dt; math.Abs(got-want) > 1e-9 {
		t.Fatalf("slowed step %v, want %v (walk speed less %v%%)", got, want, ar.Slow*100)
	}
	g.P[1].SlowT = 0
	x = g.P[1].X
	g.Step(dt)
	if got, want := g.P[1].X-x, cfg.Snorlax.WalkSpeed*dt; math.Abs(got-want) > 1e-9 {
		t.Fatalf("step %v after the slow ended, want %v", got, want)
	}
	for tick := 0; g.P[1].SlowT == 0 && tick < 1; tick++ { // the slow runs down by itself
		g.P[1].SlowT = 0.1
		steps(g, 4)
	}
	if g.P[1].SlowT != 0 {
		t.Fatalf("SlowT=%v after 4 ticks of a 0.1 s slow", g.P[1].SlowT)
	}
}

func TestArrowsOnlyHurtTheOpponentInAMirror(t *testing.T) {
	cfg := rowletConfig()
	g := duel(cfg, rowlet, rowlet)
	g.P[0].In.Arrow = true
	steps(g, 90)
	if g.P[0].HP != cfg.Rowlet.MaxHP || g.P[0].SlowT != 0 || g.P[1].HP >= cfg.Rowlet.MaxHP || g.P[1].SlowT <= 0 {
		t.Fatalf("HP %d / %d, slow %v / %v: the arrows must hurt and slow slot 1 only", g.P[0].HP, g.P[1].HP, g.P[0].SlowT, g.P[1].SlowT)
	}
}

func TestArrowsPassAnOpponentOutsideTheirReach(t *testing.T) {
	cfg := rowletConfig()
	cfg.Rowlet.Arrows.Reach = Reach{0, 10}
	g := duel(cfg, rowlet, rowlet) // the other Rowlet is at 50
	g.P[0].In.Arrow = true
	steps(g, 90)
	if g.P[1].HP != cfg.Rowlet.MaxHP {
		t.Fatalf("HP %d, want the arrows (reach 0..10) to fly under a Rowlet at %v", g.P[1].HP, cfg.Rowlet.FlyHigh)
	}
}

func TestAnArrowAndAnOpponentsProjectileTakeEachOthersDamageOff(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.Balls = append(g.Balls,
		Ball{K: ballArrow, Owner: 0, X: 500, Y: 250, DX: 1, HP: cfg.Rowlet.Arrows.HP},
		Ball{K: ballFire, Owner: 1, X: 520, Y: 250, DX: -1, Left: 500, HP: 10})
	g.Step(dt)
	arrows, fires := ballsOfKind(g, ballArrow), ballsOfKind(g, ballFire)
	if len(arrows) != 0 || len(fires) != 1 || fires[0].HP != 10-cfg.Rowlet.Arrows.Damage {
		t.Fatalf("%d arrows and %d fire balls left (fire HP %v): the arrow (damage %d) should be spent and take %d off the fire ball",
			len(arrows), len(fires), fires, cfg.Rowlet.Arrows.Damage, cfg.Rowlet.Arrows.Damage)
	}
}

// ---- Tornado ----

func TestTornadoChargesRootedThenAppearsInFrontAndRecoversRooted(t *testing.T) {
	cfg := rowletConfig()
	tn := cfg.Rowlet.Tornado
	g := rg(cfg)
	g.P[0].In = Input{Twister: true, DX: 1}
	g.Step(dt)
	g.P[0].In.Twister = false
	for n := 1; n < int(tn.ChargeDuration*tickRate)-1; n++ {
		g.Step(dt)
		if g.P[0].X != 300 || len(ballsOfKind(g, ballTornado)) != 0 {
			t.Fatalf("tick %d of the charge: x=%v with %d tornados, want it rooted and nothing yet", n, g.P[0].X, len(ballsOfKind(g, ballTornado)))
		}
	}
	steps(g, 2)
	tor := ballsOfKind(g, ballTornado)
	if len(tor) != 1 || tor[0].Owner != 0 || tor[0].HP != tn.HP || g.P[0].TwistRecT <= 0 {
		t.Fatalf("%d tornados (%+v) and %v of recovery after the charge, want one in front of slot 0", len(tor), tor, g.P[0].TwistRecT)
	}
	if want := 300 + g.radius(0) + tn.Radius; tor[0].X < want || tor[0].X > want+tn.Speed*2*dt+1e-9 || tor[0].Y != 250 {
		t.Fatalf("tornado at %v,%v, want it just in front of Rowlet (x about %v)", tor[0].X, tor[0].Y, want)
	}
	x := g.P[0].X
	steps(g, int(tn.RechargeDuration*tickRate)-3)
	if g.P[0].X != x {
		t.Fatalf("moved %v during the recovery", g.P[0].X-x)
	}
	g.P[0].In.DX = 1
	steps(g, 6)
	if g.P[0].X <= x {
		t.Fatal("still rooted after the recovery")
	}
	g.P[0].In = Input{Twister: true}
	steps(g, 20)
	if g.P[0].TwistT != 0 || len(ballsOfKind(g, ballTornado)) > 1 {
		t.Fatal("a second tornado started inside the cooldown")
	}
}

func TestTornadoChasesTheOpponentWithinItsDistanceAndLife(t *testing.T) {
	cfg := rowletConfig()
	tn := &cfg.Rowlet.Tornado
	tn.ChargeDuration, tn.RechargeDuration = 0, 0
	g := rg(cfg)
	g.P[0].In.Twister = true
	g.Step(dt)
	g.P[0].In = Input{}
	b := ballsOfKind(g, ballTornado)[0]
	x0, y0 := b.X, b.Y
	g.P[1].X, g.P[1].Y = 1300, 500 // far off to the side: the tornado turns towards where it stands, and never gets there
	steps(g, 30)
	b = ballsOfKind(g, ballTornado)[0]
	if moved := math.Hypot(b.X-x0, b.Y-y0); math.Abs(moved-tn.Speed*30*dt) > 1e-6 || b.Y <= y0 || b.X <= x0 {
		t.Fatalf("moved %v to %v,%v in a second, want %v towards the opponent", moved, b.X, b.Y, tn.Speed)
	}
	steps(g, int(tn.Distance/tn.Speed*tickRate)) // the distance runs out well before its life does
	if n := len(ballsOfKind(g, ballTornado)); n != 0 {
		t.Fatalf("%d tornados after travelling %v, want it gone", n, tn.Distance)
	}
	// life: it is slow enough that its time runs out first
	cfg.Rowlet.Tornado.Speed, cfg.Rowlet.Tornado.Duration = 20, 1
	g = rg(cfg)
	g.P[0].In.Twister = true
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, tickRate-3)
	if len(ballsOfKind(g, ballTornado)) != 1 {
		t.Fatal("the tornado ended before its duration")
	}
	steps(g, 6)
	if len(ballsOfKind(g, ballTornado)) != 0 {
		t.Fatal("the tornado outlived its duration")
	}
}

func TestTornadoCarriesTheOpponentUpSpinsItAndDropsItForLandingDamage(t *testing.T) {
	cfg := rowletConfig()
	tn := cfg.Rowlet.Tornado
	g := rg(cfg)
	g.P[1].X = 420
	g.P[0].In.Twister = true
	events, ok := stepsUntilTwirl(g, 1, twirlSpin, 40)
	g.P[0].In = Input{}
	if !ok {
		t.Fatal("the tornado never caught an opponent standing next to it")
	}
	b := ballsOfKind(g, ballTornado)[0]
	v := &g.P[1]
	if !b.Caught || !v.Held || b.Life > tn.SpinDuration || v.X != b.X || v.Y != b.Y {
		t.Fatalf("caught=%v held=%v life=%v, body at %v,%v tornado at %v,%v: want it held and pinned in the tornado", b.Caught, v.Held, b.Life, v.X, v.Y, b.X, b.Y)
	}
	// a held body takes no orders
	v.In = Input{DX: 1, Skill: true, Beam: true, Punch: true}
	x, y := v.X, v.Y
	steps(g, int(tn.SpinDuration/2*tickRate)+1)
	if v.X != x || v.Y != y || v.Z != tn.LiftHeight {
		t.Fatalf("at %v,%v,%v half way through the spin, want %v,%v pinned and lifted to %v", v.X, v.Y, v.Z, x, y, tn.LiftHeight)
	}
	dmgSoFar := cfg.Snorlax.MaxHP - v.HP
	if dmgSoFar%tn.SpinDamage != 0 || dmgSoFar == 0 {
		t.Fatalf("%d damage half way, want some whole number of spin ticks of %d", dmgSoFar, tn.SpinDamage)
	}
	v.In = Input{}
	more, ok := stepsUntilTwirl(g, 1, twirlNone, 200)
	events = append(events, more...)
	if !ok || v.Held || v.Z != 0 {
		t.Fatalf("twirl %d held=%v z=%v after the spin, want it landed", v.Twirl, v.Held, v.Z)
	}
	if want := cfg.Snorlax.MaxHP - 6*tn.SpinDamage - tn.LandingDamage; v.HP != want { // spin 3.2 s at 0.5 s: ticks at 0.5 .. 3.0
		t.Fatalf("HP %d, want %d: 6 spin ticks of %d and a landing of %d", v.HP, want, tn.SpinDamage, tn.LandingDamage)
	}
	slams := 0
	for _, e := range events {
		if e.T == "slam" && e.X == v.X && e.Y == v.Y {
			slams++
		}
	}
	if slams != 1 {
		t.Fatalf("%d landing shocks, want 1 at the body", slams)
	}
	if n := len(ballsOfKind(g, ballTornado)); n != 0 {
		t.Fatalf("%d tornados after the spin, want it gone", n)
	}
	v.In.DX = 1
	g.Step(dt)
	if v.X <= x {
		t.Fatal("it cannot walk after landing")
	}
}

func TestBreakingTheTornadoLetsItsCaptiveFallAtOnce(t *testing.T) {
	cfg := rowletConfig()
	tn := cfg.Rowlet.Tornado
	g := rg(cfg)
	g.P[1].X = 420
	g.P[0].In.Twister = true
	stepsUntilTwirl(g, 1, twirlSpin, 40)
	g.P[0].In = Input{}
	steps(g, 15) // half a second up
	z := g.P[1].Z
	if z <= 0 || z >= tn.LiftHeight {
		t.Fatalf("Z=%v after half a second, want it on its way up", z)
	}
	ballsOfKind(g, ballTornado)[0].HP = 0
	g.Step(dt)
	if g.P[1].Twirl != twirlFall || len(ballsOfKind(g, ballTornado)) != 0 || g.P[1].Z >= z {
		t.Fatalf("twirl %d, %d tornados, Z %v -> %v: want it falling with the tornado gone", g.P[1].Twirl, len(ballsOfKind(g, ballTornado)), z, g.P[1].Z)
	}
	hp := g.P[1].HP
	_, ok := stepsUntilTwirl(g, 1, twirlNone, 30)
	if !ok || g.P[1].HP != hp-tn.LandingDamage || g.P[1].Held || g.P[1].Z != 0 {
		t.Fatalf("landed=%v HP %d -> %d held=%v z=%v, want the landing damage %d", ok, hp, g.P[1].HP, g.P[1].Held, g.P[1].Z, tn.LandingDamage)
	}
}

func TestATornadoLeavesAnOpponentThatIsAlreadyHeldAlone(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.P[1].X, g.P[1].Held = 420, true // say, caught in a toss
	g.P[0].In.Twister = true
	steps(g, 40)
	if g.P[1].Twirl != twirlNone || ballsOfKind(g, ballTornado)[0].Caught {
		t.Fatal("a tornado took a body that something else holds")
	}
}

func TestARowletCarriedByATornadoClimbsBackToItsHeightAfterLanding(t *testing.T) {
	cfg := rowletConfig()
	g := duel(cfg, rowlet, rowlet)
	g.P[1].X = 420
	g.P[0].In.Twister = true
	stepsUntilTwirl(g, 1, twirlSpin, 40)
	g.P[0].In = Input{}
	if _, ok := stepsUntilTwirl(g, 1, twirlNone, 200); !ok {
		t.Fatal("never landed")
	}
	if g.P[1].Z != 0 {
		t.Fatalf("Z=%v on landing, want it on the ground first", g.P[1].Z)
	}
	steps(g, 10)
	if g.P[1].Z != cfg.Rowlet.FlyHigh {
		t.Fatalf("Z=%v, want it back at %v", g.P[1].Z, cfg.Rowlet.FlyHigh)
	}
}

func TestATornadoIsWornDownByProjectilesAndPassesPillars(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.Balls = append(g.Balls,
		Ball{K: ballTornado, Owner: 0, X: 500, Y: 250, HP: cfg.Rowlet.Tornado.HP, Left: 500, Life: 8},
		Ball{K: ballFire, Owner: 1, X: 540, Y: 250, DX: -1, Left: 500, HP: 10})
	g.Step(dt)
	tor, fires := ballsOfKind(g, ballTornado), ballsOfKind(g, ballFire)
	if len(tor) != 1 || tor[0].HP != cfg.Rowlet.Tornado.HP-cfg.Charizard.FireBreath.Damage || len(fires) != 1 || fires[0].HP != 10-cfg.Rowlet.Tornado.SpinDamage {
		t.Fatalf("tornado %+v, fire %+v: each should have lost the other's damage", tor, fires)
	}
	g = rg(cfg)
	g.Balls = append(g.Balls,
		Ball{K: ballTornado, Owner: 0, X: 500, Y: 250, HP: cfg.Rowlet.Tornado.HP, Left: 500, Life: 8},
		Ball{K: ballPillar, Owner: 1, X: 500, Y: 250, HP: 25, Life: 8})
	g.Step(dt)
	if tor := ballsOfKind(g, ballTornado); len(tor) != 1 || tor[0].HP != cfg.Rowlet.Tornado.HP {
		t.Fatalf("tornado %+v after sweeping through a pillar, want it untouched", tor)
	}
}

func TestFreezingCutsTheTornadoChargeAndLeavesOneAlreadyLetGo(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.P[0].In.Twister = true
	g.Step(dt)
	g.P[0].In = Input{}
	g.P[0].FrozenT = 1
	steps(g, 30)
	if len(ballsOfKind(g, ballTornado)) != 0 {
		t.Fatal("a frozen Rowlet still let a tornado go")
	}
	g = rg(cfg)
	g.P[0].In.Twister = true
	steps(g, 14)
	g.P[0].In = Input{}
	g.P[0].FrozenT = 5
	steps(g, 3)
	if len(ballsOfKind(g, ballTornado)) != 1 {
		t.Fatal("freezing Rowlet stopped a tornado that was already out")
	}
}

// ---- the round, the wire and the config ----

func TestARoundStartedOverClearsTheTwirlAndTheSlow(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.P[1].Twirl, g.P[1].Held, g.P[1].SlowT = twirlSpin, true, 5
	g.Reset()
	if p := g.P[1]; p.Twirl != 0 || p.Held || p.SlowT != 0 || p.Z != 0 {
		t.Fatalf("after Reset: %+v", p)
	}
	if g.P[0].Z != cfg.Rowlet.FlyHigh {
		t.Fatalf("Z=%v after Reset, want Rowlet in the air", g.P[0].Z)
	}
}

func TestRowletSnapshotKeys(t *testing.T) {
	g := rg(rowletConfig())
	raw, _ := json.Marshal(g.P[0])
	for _, k := range []string{`"mt":`, `"mcd":`, `"acd":`, `"at":`, `"sl":`, `"qt":`, `"qr":`, `"qcd":`, `"tw":`} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("player json lacks %s: %s", k, raw)
		}
	}
	raw, _ = json.Marshal(Ball{K: ballTornado, Caught: true, Life: 2})
	if !strings.Contains(string(raw), `"cg":true`) || !strings.Contains(string(raw), `"lf":2`) {
		t.Errorf("tornado json %s lacks cg or lf", raw)
	}
}

func TestRowletConfigIsChecked(t *testing.T) {
	for name, edit := range map[string]func(c *Config){
		"slow above 1":      func(c *Config) { c.Rowlet.Arrows.Slow = 1.5 },
		"negative slow":     func(c *Config) { c.Rowlet.Arrows.Slow = -0.1 },
		"too many arrows":   func(c *Config) { c.Rowlet.Arrows.Count = 101 },
		"no arrows":         func(c *Config) { c.Rowlet.Arrows.Count = 0 },
		"no spin interval":  func(c *Config) { c.Rowlet.Tornado.SpinInterval = 0 },
		"no fly_high":       func(c *Config) { c.Rowlet.FlyHigh = 0 },
		"no drift radius":   func(c *Config) { c.Rowlet.CrescentDrift.Radius = 0 },
		"no lift":           func(c *Config) { c.Rowlet.Tornado.LiftHeight = 0 },
		"negative charge":   func(c *Config) { c.Rowlet.Tornado.ChargeDuration = -1 },
		"inverted reach":    func(c *Config) { c.Rowlet.Tornado.Reach = Reach{10, 5} },
		"inverted arrow":    func(c *Config) { c.Rowlet.Arrows.Reach = Reach{10, 5} },
		"negative landing":  func(c *Config) { c.Rowlet.Tornado.LandingDamage = -1 },
		"no tornado hp":     func(c *Config) { c.Rowlet.Tornado.HP = 0 },
		"no arrow scale":    func(c *Config) { c.Rowlet.Arrows.Scale = 0 },
		"negative aim":      func(c *Config) { c.Rowlet.Arrows.ChargeDuration = -1 },
		"no spin duration":  func(c *Config) { c.Rowlet.Tornado.SpinDuration = 0 },
		"negative cooldown": func(c *Config) { c.Rowlet.Arrows.Cooldown = -1 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := DefaultConfig().validate(); err != nil {
		t.Fatal(err)
	}
	c := DefaultConfig()
	if c.maxHPOf(rowlet) != c.Rowlet.MaxHP || c.walkSpeedOf(rowlet) != c.Rowlet.WalkSpeed || c.radiusOf(rowlet) != c.Rowlet.Radius {
		t.Fatal("the accessors do not know Rowlet")
	}
}

func TestDriftAndTornadoDoNotStartOverEachOther(t *testing.T) {
	cfg := rowletConfig()
	g := rg(cfg)
	g.P[0].In = Input{Skill: true, Twister: true, DX: 1}
	g.Step(dt)
	if g.P[0].DriftT == 0 || g.P[0].TwistT != 0 || g.P[0].TwistCD != 0 {
		t.Fatalf("drift %v, charge %v, cooldown %v: pressing both should drift only", g.P[0].DriftT, g.P[0].TwistT, g.P[0].TwistCD)
	}
	g = rg(cfg)
	g.P[0].In = Input{Twister: true, DX: 1}
	g.Step(dt)
	g.P[0].In = Input{Skill: true, DX: 1}
	steps(g, 3)
	if g.P[0].DriftT != 0 || g.P[0].DriftCD != 0 || g.P[0].X != 300 {
		t.Fatalf("drift %v at x=%v while charging a tornado, want it refused", g.P[0].DriftT, g.P[0].X)
	}
}

func TestTornadoStopsOnTheOpponentInsteadOfOvershooting(t *testing.T) {
	cfg := rowletConfig()
	cfg.Rowlet.Tornado.Speed = 6000            // 200 a tick
	cfg.Rowlet.Tornado.Reach = Reach{100, 200} // it never catches the one on the ground, so it just chases
	cfg.Rowlet.Tornado.ChargeDuration, cfg.Rowlet.Tornado.RechargeDuration = 0, 0
	g := rg(cfg)
	g.P[1].X = 500
	g.P[0].In.Twister = true
	steps(g, 3)
	if b := ballsOfKind(g, ballTornado); len(b) != 1 || b[0].X != 500 || b[0].Y != 250 {
		t.Fatalf("tornado %+v, want it sitting exactly on the opponent at 500,250", b)
	}
}

func TestArrowsAimForTheChargeThenLeaveFromWhereRowletAndTheOpponentAre(t *testing.T) {
	cfg := rowletConfig()
	cfg.Rowlet.Arrows.ChargeDuration = 0.5
	g := rg(cfg)
	g.P[0].In = Input{Arrow: true, DX: 1}
	g.Step(dt)
	if g.P[0].ArrowT <= 0 || len(ballsOfKind(g, ballArrow)) != 0 {
		t.Fatalf("ArrowT=%v and %d arrows after the first tick, want it aiming", g.P[0].ArrowT, len(ballsOfKind(g, ballArrow)))
	}
	x := g.P[0].X
	g.P[0].In = Input{DX: 1} // free to walk while aiming
	steps(g, 5)
	if g.P[0].X <= x || len(ballsOfKind(g, ballArrow)) != 0 {
		t.Fatalf("x %v -> %v with %d arrows: want it walking and still aiming", x, g.P[0].X, len(ballsOfKind(g, ballArrow)))
	}
	g.P[0].In = Input{Skill: true, DX: 1} // and free to drift
	g.Step(dt)
	if g.P[0].DriftT == 0 || g.P[0].ArrowT == 0 {
		t.Fatalf("drift %v, aim %v: a drift must not stop the aiming", g.P[0].DriftT, g.P[0].ArrowT)
	}
	g.P[1].Y = 450 // the opponent moves: the volley goes where it is when it leaves
	g.P[0].In = Input{}
	for n := 0; n < 30 && len(ballsOfKind(g, ballArrow)) == 0; n++ {
		g.Step(dt)
	}
	arrows := ballsOfKind(g, ballArrow)
	if len(arrows) != cfg.Rowlet.Arrows.Count {
		t.Fatalf("%d arrows after the charge, want %d", len(arrows), cfg.Rowlet.Arrows.Count)
	}
	mid := arrows[cfg.Rowlet.Arrows.Count/2]
	if want := math.Atan2(g.P[1].Y-mid.Y, g.P[1].X-mid.X); math.Abs(math.Atan2(mid.DY, mid.DX)-want) > 0.05 {
		t.Fatalf("middle arrow goes %v rad, want about %v at the opponent", math.Atan2(mid.DY, mid.DX), want)
	}
}

func TestFreezingCancelsTheArrowAim(t *testing.T) {
	cfg := rowletConfig()
	cfg.Rowlet.Arrows.ChargeDuration = 0.5
	g := rg(cfg)
	g.P[0].In.Arrow = true
	g.Step(dt)
	g.P[0].In = Input{}
	g.P[0].FrozenT = 2
	steps(g, 30)
	if g.P[0].ArrowT != 0 || len(ballsOfKind(g, ballArrow)) != 0 {
		t.Fatalf("ArrowT=%v, %d arrows: a frozen Rowlet should lose the volley", g.P[0].ArrowT, len(ballsOfKind(g, ballArrow)))
	}
}

func TestAHeavierPunchPushesTheTornadoByTheMassDifferenceAndALighterOneDoesNot(t *testing.T) {
	for _, tc := range []struct {
		mass    float64
		pushed  bool
		comment string
	}{{100, true, "punch 150 over tornado 100: pushed 50"}, {150, false, "equal mass: not pushed"}} {
		cfg := rowletConfig()
		cfg.Rowlet.Tornado.Mass = tc.mass
		cfg.Rowlet.Tornado.Speed = 1
		g := duel(cfg, snorlax, rowlet)
		g.Balls = append(g.Balls, Ball{K: ballTornado, Owner: 1, X: 400, Y: 250, HP: cfg.Rowlet.Tornado.HP, Left: 500, Life: 8})
		g.P[0].In = Input{Punch: true, DX: 1}
		steps(g, 40)
		x := ballsOfKind(g, ballTornado)[0].X
		if tc.pushed != (x > 440) {
			t.Fatalf("%s: tornado at x=%v", tc.comment, x)
		}
	}
}

func TestATornadoWearsDownTheOpponentsPillarsItTouchesEverySpinInterval(t *testing.T) {
	cfg := rowletConfig()
	cfg.Rowlet.Tornado.Speed = 1
	tn := cfg.Rowlet.Tornado
	g := duel(cfg, rowlet, vulpix)
	g.Balls = append(g.Balls,
		Ball{K: ballTornado, Owner: 0, X: 500, Y: 250, HP: tn.HP, Left: 500, Life: 8},
		Ball{K: ballPillar, Owner: 1, X: 520, Y: 250, HP: 100, Life: 20},
		Ball{K: ballPillar, Owner: 0, X: 480, Y: 250, HP: 100, Life: 20}, // its own: left alone
		Ball{K: ballPillar, Owner: 1, X: 900, Y: 250, HP: 100, Life: 20}) // too far
	g.Step(dt)
	hp := func() (near, own, far int) {
		ps := ballsOfKind(g, ballPillar)
		return ps[0].HP, ps[1].HP, ps[2].HP
	}
	if near, own, far := hp(); near != 100-tn.SpinDamage || own != 100 || far != 100 {
		t.Fatalf("pillar HP %d / %d / %d after touching, want %d / 100 / 100", near, own, far, 100-tn.SpinDamage)
	}
	steps(g, int(tn.SpinInterval*tickRate)-2)
	if near, _, _ := hp(); near != 100-tn.SpinDamage {
		t.Fatalf("pillar HP %d inside the interval, want it not hurt again yet", near)
	}
	steps(g, 3)
	if near, _, _ := hp(); near != 100-2*tn.SpinDamage {
		t.Fatalf("pillar HP %d after one interval, want %d", near, 100-2*tn.SpinDamage)
	}
	if tor := ballsOfKind(g, ballTornado); len(tor) != 1 || tor[0].HP != tn.HP {
		t.Fatalf("tornado %+v, want it unhurt by the pillar", tor)
	}
}
