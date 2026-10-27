package main

import (
	"encoding/json"
	"math"
	"testing"
)

const dt = 1.0 / tickRate

func newTestGame() *Game { return NewGame(DefaultConfig()) }

// ---- Snorlax: jump and slam ----

func TestSlamHitsGroundedOpponent(t *testing.T) {
	g := newTestGame()
	g.P[snorlax].X, g.P[snorlax].Y = 300, 250
	g.P[pikachu].X, g.P[pikachu].Y = 330, 250 // inside slamRadius
	g.P[snorlax].In.Skill = true

	for i := 0; i < tickRate*2 && g.P[pikachu].HP == maxHP; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].HP != maxHP-slamDamage {
		t.Fatalf("opponent HP = %d, want %d", g.P[pikachu].HP, maxHP-slamDamage)
	}
}

func TestSlamNeverDamagesWhileStillRising(t *testing.T) {
	g := newTestGame()
	g.P[snorlax].X, g.P[pikachu].X = 300, 310
	g.P[snorlax].Y, g.P[pikachu].Y = 250, 250
	g.P[snorlax].In.Skill = true
	g.Step(dt)
	if g.P[snorlax].Z <= 0 || g.P[pikachu].HP != maxHP {
		t.Fatalf("Z=%v HP=%d: damage must wait for the landing frame", g.P[snorlax].Z, g.P[pikachu].HP)
	}
}

func TestAirborneOpponentDodgesSlam(t *testing.T) {
	g := newTestGame()
	g.P[snorlax].X, g.P[pikachu].X = 300, 310
	g.P[snorlax].Y, g.P[pikachu].Y = 250, 250
	g.P[snorlax].Z, g.P[snorlax].VZ = 1, -100 // lands on the next step
	g.P[pikachu].Z, g.P[pikachu].VZ = 100, 0  // in the air
	g.Step(dt)
	if g.P[snorlax].Z != 0 {
		t.Fatal("setup: attacker should have landed")
	}
	if g.P[pikachu].HP != maxHP {
		t.Fatalf("airborne opponent took damage: HP=%d", g.P[pikachu].HP)
	}
}

// The configured height and duration must be what the player actually gets.
func TestJumpHeightAndDurationFollowConfig(t *testing.T) {
	for _, tc := range []struct{ height, duration float64 }{
		{72, 0.8}, {100, 1.0}, {40, 0.5}, {150, 1.2},
	} {
		cfg := DefaultConfig()
		cfg.Snorlax.JumpHeight, cfg.Snorlax.JumpDuration = tc.height, tc.duration
		g := NewGame(cfg)
		g.P[snorlax].In.Skill = true

		peak, airTime := 0.0, 0.0
		for n := 1; n <= 10*tickRate; n++ {
			g.Step(dt)
			g.P[snorlax].In.Skill = false // one press
			peak = math.Max(peak, g.P[snorlax].Z)
			if g.P[snorlax].Z == 0 {
				airTime = float64(n) * dt
				break
			}
		}
		if math.Abs(peak-tc.height) > tc.height*0.05 {
			t.Errorf("height %v/%vs: peak = %.1f", tc.height, tc.duration, peak)
		}
		if math.Abs(airTime-tc.duration) > dt+1e-9 {
			t.Errorf("height %v/%vs: air time = %.3f", tc.height, tc.duration, airTime)
		}
	}
}

// ---- Pikachu: dash ----

func TestDashCoversConfiguredDistanceInConfiguredTime(t *testing.T) {
	for _, tc := range []struct{ distance, duration float64 }{
		{220, 0.2}, {120, 0.35}, {300, 0.1}, // 0.35 and 0.1 are not multiples of a tick
	} {
		cfg := DefaultConfig()
		cfg.Pikachu.DashDistance, cfg.Pikachu.DashDuration = tc.distance, tc.duration
		g := NewGame(cfg)
		g.P[pikachu].X = 100
		g.P[pikachu].In = Input{DX: 1, Skill: true}

		steps := 0
		for ; steps < 60; steps++ {
			g.Step(dt)
			g.P[pikachu].In.Skill = false
			g.P[pikachu].In.DX = 0 // released keys must not matter mid-dash
			if g.P[pikachu].DashT == 0 {
				steps++
				break
			}
		}
		if got := g.P[pikachu].X - 100; math.Abs(got-tc.distance) > 0.01 {
			t.Errorf("%vu/%vs: moved %.3f", tc.distance, tc.duration, got)
		}
		if want := int(math.Ceil(tc.duration/dt - 1e-9)); steps != want {
			t.Errorf("%vu/%vs: took %d ticks, want %d", tc.distance, tc.duration, steps, want)
		}
	}
}

func TestDashDirection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     Input
		wantDX float64 // sign of expected movement
		wantDY float64
	}{
		{"held left", Input{DX: -1, Skill: true}, -1, 0},
		{"held up", Input{DY: -1, Skill: true}, 0, -1},
		{"held diagonal", Input{DX: 1, DY: 1, Skill: true}, 1, 1},
		{"no keys faces right at start", Input{Skill: true}, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGame()
			g.P[pikachu].X, g.P[pikachu].Y = 400, 250
			g.P[pikachu].In = tc.in
			for i := 0; i < 60; i++ { // press once, release everything, let the dash finish
				g.Step(dt)
				g.P[pikachu].In = Input{}
				if g.P[pikachu].DashT == 0 {
					break
				}
			}
			sign := func(v float64) float64 {
				if math.Abs(v) < 1e-9 {
					return 0
				}
				return math.Copysign(1, v)
			}
			dx, dy := g.P[pikachu].X-400, g.P[pikachu].Y-250
			if sign(dx) != tc.wantDX || sign(dy) != tc.wantDY {
				t.Fatalf("moved (%.1f, %.1f), want direction (%v, %v)", dx, dy, tc.wantDX, tc.wantDY)
			}
			if d := math.Hypot(dx, dy); math.Abs(d-DefaultConfig().Pikachu.DashDistance) > 0.01 {
				t.Fatalf("diagonal dash covered %.2f, want %v (no diagonal bonus)", d, DefaultConfig().Pikachu.DashDistance)
			}
		})
	}
}

func TestDashWithNoKeysUsesLastFacing(t *testing.T) {
	g := newTestGame()
	g.P[pikachu].X, g.P[pikachu].Y = 400, 250
	g.P[pikachu].In = Input{DX: -1}
	g.Step(dt) // walk left once, so Pikachu now faces left
	x := g.P[pikachu].X
	g.P[pikachu].In = Input{Skill: true}
	for i := 0; i < tickRate; i++ {
		g.Step(dt)
		g.P[pikachu].In.Skill = false
	}
	if g.P[pikachu].X >= x {
		t.Fatalf("x went %.1f -> %.1f, want a dash to the left", x, g.P[pikachu].X)
	}
}

func TestDashStopsAtArenaWall(t *testing.T) {
	g := newTestGame()
	g.P[pikachu].X = arenaW - 10
	g.P[pikachu].In = Input{DX: 1, Skill: true}
	for i := 0; i < tickRate; i++ {
		g.Step(dt)
		g.P[pikachu].In.Skill = false
	}
	if g.P[pikachu].X != arenaW {
		t.Fatalf("x = %v, want clamped to %v", g.P[pikachu].X, arenaW)
	}
}

func TestDashCooldownBlocksChainDashing(t *testing.T) {
	cfg := DefaultConfig() // 0.2s dash, 0.6s cooldown
	g := NewGame(cfg)
	g.P[pikachu].X = 50
	g.P[pikachu].In = Input{DX: 1, Skill: true} // space held the whole time

	// 0.5s: the first dash is over (0.2s) but the cooldown (0.6s) is not.
	for i := 0; i < tickRate/2; i++ {
		g.Step(dt)
		g.P[pikachu].In.DX = 0 // stop walking so only dashes move it
	}
	if got := g.P[pikachu].X - 50; math.Abs(got-cfg.Pikachu.DashDistance) > 0.01 {
		t.Fatalf("moved %.2f in 0.5s, want exactly one dash (%v)", got, cfg.Pikachu.DashDistance)
	}
	// Once the cooldown has passed, the held key dashes again.
	for i := 0; i < tickRate/2; i++ {
		g.Step(dt)
	}
	if got := g.P[pikachu].X - 50; got < 2*cfg.Pikachu.DashDistance-0.01 {
		t.Fatalf("moved %.2f after the cooldown, want a second dash", got)
	}
}

func TestDashIgnoresSteeringWhileActive(t *testing.T) {
	g := newTestGame()
	g.P[pikachu].X, g.P[pikachu].Y = 400, 250
	g.P[pikachu].In = Input{DX: 1, Skill: true}
	g.Step(dt)
	g.P[pikachu].In = Input{DY: 1} // try to turn mid-dash
	for i := 0; i < 3; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].Y != 250 {
		t.Fatalf("y = %v, dash should keep its direction", g.P[pikachu].Y)
	}
}

// ---- each character only has its own skill ----

func TestSkillsAreNotShared(t *testing.T) {
	g := newTestGame()
	g.P[pikachu].X, g.P[pikachu].Y = 200, 250
	g.P[snorlax].X, g.P[snorlax].Y = 600, 250
	g.P[pikachu].In = Input{Skill: true}
	g.P[snorlax].In = Input{Skill: true}
	g.Step(dt)
	if g.P[pikachu].Z != 0 {
		t.Fatalf("Pikachu jumped (Z=%v)", g.P[pikachu].Z)
	}
	if g.P[snorlax].X != 600 || g.P[snorlax].Y != 250 {
		t.Fatalf("Snorlax dashed to (%v, %v)", g.P[snorlax].X, g.P[snorlax].Y)
	}
}

// ---- Snorlax: Hyper Beam (charge -> sweep from +angle to -angle -> recharge) ----

// beamGame puts Snorlax at (600, 250) facing left with Pikachu 300 units away
// on the same row, i.e. dead ahead.
func beamGame(cfg Config) *Game {
	g := NewGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 600, 250
	g.P[pikachu].X, g.P[pikachu].Y = 300, 250
	return g
}

// at returns the point r units from Snorlax (600, 250) at bearing b degrees from
// its facing (left). Positive is counter-clockwise on screen, which from a
// left-facing Snorlax means down-left.
func at(b, r float64) (x, y float64) {
	rad := b * math.Pi / 180
	return 600 - r*math.Cos(rad), 250 + r*math.Sin(rad)
}

func chargeTicks(cfg Config) int {
	return int(math.Ceil(cfg.Snorlax.HyperBeam.ChargeDuration/dt - 1e-9))
}

func sweepTicks(cfg Config) int {
	return int(math.Ceil(cfg.Snorlax.HyperBeam.SweepDuration/dt - 1e-9))
}

// stepBeam presses the beam key for one tick and then lets go.
func stepBeam(g *Game) {
	g.P[snorlax].In = Input{Beam: true}
	g.Step(dt)
	g.P[snorlax].In = Input{}
}

// hitTick fires one beam and returns the tick (the press is tick 1) on which
// Pikachu first lost HP, or 0 if it never did.
func hitTick(g *Game) int {
	stepBeam(g)
	for tick := 2; tick <= 6*tickRate; tick++ {
		g.Step(dt)
		if g.P[pikachu].HP < maxHP {
			return tick
		}
	}
	return 0
}

func TestBeamRunsChargeThenSweepThenRecharge(t *testing.T) {
	cfg := DefaultConfig()
	hb := cfg.Snorlax.HyperBeam
	g := beamGame(cfg) // Pikachu stands dead ahead, so any early damage would show
	s := &g.P[snorlax]

	stepBeam(g) // tick 1
	if s.BeamT == 0 || s.SweepT != 0 {
		t.Fatalf("after the press: BeamT=%v SweepT=%v, want charging", s.BeamT, s.SweepT)
	}
	for tick := 2; tick < chargeTicks(cfg); tick++ {
		g.Step(dt)
		if s.SweepT != 0 || g.P[pikachu].HP != maxHP {
			t.Fatalf("tick %d: the beam must not start or hurt before the charge is full", tick)
		}
	}
	g.Step(dt) // the tick the charge completes
	if s.BeamT != 0 || s.SweepT == 0 || s.BeamA != hb.SweepAngle {
		t.Fatalf("charge done: BeamT=%v SweepT=%v BeamA=%v, want a sweep starting at +%v", s.BeamT, s.SweepT, s.BeamA, hb.SweepAngle)
	}
	for i := 0; i < sweepTicks(cfg); i++ {
		g.Step(dt)
	}
	if s.SweepT != 0 || s.BeamA != -hb.SweepAngle || s.RechargeT == 0 {
		t.Fatalf("sweep done: SweepT=%v BeamA=%v RechargeT=%v, want it ending at -%v and recharging", s.SweepT, s.BeamA, s.RechargeT, hb.SweepAngle)
	}
}

// Facing 0 degrees, the beam goes from +30 to -30: a target on the +30 side is
// hit before one on the -30 side.
func TestSweepGoesFromPlusAngleToMinusAngle(t *testing.T) {
	plus := beamGame(DefaultConfig())
	plus.P[pikachu].X, plus.P[pikachu].Y = at(20, 300)
	minus := beamGame(DefaultConfig())
	minus.P[pikachu].X, minus.P[pikachu].Y = at(-20, 300)

	tp, tm := hitTick(plus), hitTick(minus)
	if tp == 0 || tm == 0 {
		t.Fatalf("both targets are inside the fan: hit ticks +20=%d -20=%d", tp, tm)
	}
	if tp >= tm {
		t.Fatalf("+20 hit at tick %d, -20 at tick %d: the sweep must reach +20 first", tp, tm)
	}
	if first := chargeTicks(DefaultConfig()); tp <= first {
		t.Fatalf("hit at tick %d, but the sweep only starts after tick %d", tp, first)
	}
}

func TestSweepCoversExactlyTheFan(t *testing.T) { // defaults: +-30 degrees, length 450, width 50
	for _, tc := range []struct {
		name string
		b, r float64
		hit  bool
	}{
		{"dead ahead", 0, 300, true},
		{"inside, +side", 25, 300, true},
		{"inside, -side", -25, 300, true},
		{"on the +30 edge", 30, 300, true},
		{"on the -30 edge", -30, 300, true},
		{"just past the edge, within the beam's width", 34, 300, true}, // asin(25/300) = 4.8 degrees
		{"well past the +edge", 45, 300, false},
		{"well past the -edge", -45, 300, false},
		{"off to the side", 90, 300, false},
		{"behind Snorlax", 180, 300, false},
		{"inside the reach", 0, 440, true},
		{"beyond the reach", 0, 470, false},
		{"standing on Snorlax", 0, 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := beamGame(DefaultConfig())
			g.P[pikachu].X, g.P[pikachu].Y = at(tc.b, tc.r)
			if got := hitTick(g) != 0; got != tc.hit {
				t.Fatalf("hit = %v, want %v (bearing %v, distance %v)", got, tc.hit, tc.b, tc.r)
			}
		})
	}
}

func TestOneSweepHitsATargetOnlyOnce(t *testing.T) {
	cfg := DefaultConfig()
	g := beamGame(cfg)
	stepBeam(g)
	for i := 0; i < 3*tickRate; i++ { // the whole sweep and well beyond
		g.Step(dt)
	}
	if want := maxHP - cfg.Snorlax.HyperBeam.Damage; g.P[pikachu].HP != want {
		t.Fatalf("HP = %d, want exactly one hit (%d)", g.P[pikachu].HP, want)
	}
}

// Stepping out of the fan before the beam gets there dodges it.
func TestSweepCanBeDodgedBeforeItArrives(t *testing.T) {
	cfg := DefaultConfig()
	g := beamGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = at(-25, 300) // near the end of the sweep
	stepBeam(g)
	for i := 1; i < chargeTicks(cfg)+5; i++ { // charge done, a third of the sweep gone
		g.Step(dt)
	}
	if g.P[pikachu].HP != maxHP {
		t.Fatal("setup: the beam should not have reached -25 yet")
	}
	g.P[pikachu].X, g.P[pikachu].Y = 100, 250 // out of reach
	for i := 0; i < tickRate; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].HP != maxHP {
		t.Fatalf("HP = %d, a dodged beam must not hit", g.P[pikachu].HP)
	}
}

func TestFanFollowsTheFacingAndItIsLockedDuringCharge(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        Input
		target    [2]float64 // Pikachu position
		wantHit   bool
		steerWith Input // keys held during the charge: must not re-aim the beam
	}{
		{"facing right", Input{DX: 1, Beam: true}, [2]float64{650, 250}, true, Input{DY: -1}},
		{"facing up", Input{DY: -1, Beam: true}, [2]float64{400, 100}, true, Input{DX: 1}},
		{"facing up-left", Input{DX: -1, DY: -1, Beam: true}, [2]float64{190, 40}, true, Input{DX: 1}},
		{"facing right, target on the left", Input{DX: 1, Beam: true}, [2]float64{150, 250}, false, Input{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGame(DefaultConfig())
			g.P[snorlax].X, g.P[snorlax].Y = 400, 250
			g.P[pikachu].X, g.P[pikachu].Y = tc.target[0], tc.target[1]
			g.P[snorlax].In = tc.in
			g.Step(dt)
			g.P[snorlax].In = tc.steerWith
			for i := 0; i < 2*tickRate; i++ {
				g.Step(dt)
			}
			if got := g.P[pikachu].HP < maxHP; got != tc.wantHit {
				t.Fatalf("hit = %v, want %v", got, tc.wantHit)
			}
		})
	}
}

func TestSnorlaxIsRootedThroughChargeSweepAndRecharge(t *testing.T) {
	cfg := DefaultConfig()
	hb := cfg.Snorlax.HyperBeam
	g := beamGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = 580, 330 // inside quake range (82 away) but 76 degrees off the fan
	stepBeam(g)
	n := chargeTicks(cfg) + sweepTicks(cfg) + int(math.Ceil(hb.RechargeDuration/dt)) - 2
	for i := 0; i < n; i++ { // every key held for the whole sequence
		g.P[snorlax].In = Input{DX: 1, Skill: true, Attack: true}
		g.Step(dt)
		if g.P[snorlax].X != 600 || g.P[snorlax].Z != 0 {
			t.Fatalf("tick %d: Snorlax at x=%v z=%v, want rooted", i, g.P[snorlax].X, g.P[snorlax].Z)
		}
	}
	if g.P[pikachu].HP != maxHP {
		t.Fatalf("Pikachu HP = %d: no quake may come out while rooted", g.P[pikachu].HP)
	}
	for i := 0; i < 6; i++ {
		g.P[snorlax].In = Input{DX: 1}
		g.Step(dt)
	}
	if g.P[snorlax].X <= 600 {
		t.Fatalf("x = %v, want Snorlax free to move after the recharge", g.P[snorlax].X)
	}
}

func TestHoldingBeamKeyCannotSkipTheSweepOrRecharge(t *testing.T) {
	g := beamGame(DefaultConfig()) // charge 0.6 + sweep 0.5 + recharge 1.5 = 2.6s per beam
	g.P[snorlax].In = Input{Beam: true}
	starts, prev := 0, false
	for i := 0; i < 6*tickRate; i++ { // 6s
		g.Step(dt)
		charging := g.P[snorlax].BeamT > 0
		if charging && !prev {
			starts++
		}
		prev = charging
	}
	if starts != 3 {
		t.Fatalf("beams started in 6s = %d, want 3 (at ~0s, ~2.6s and ~5.2s)", starts)
	}
}

func TestSweepAngleZeroIsAFixedBeam(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.HyperBeam.SweepAngle = 0
	ahead, aside := beamGame(cfg), beamGame(cfg)
	aside.P[pikachu].X, aside.P[pikachu].Y = at(10, 300) // asin(25/300) = 4.8 degrees of reach
	if hitTick(ahead) == 0 {
		t.Fatal("a fixed beam must hit what is dead ahead")
	}
	if hitTick(aside) != 0 {
		t.Fatal("a fixed beam must not reach 10 degrees off its line")
	}
}

// One tick can cover the whole arc; the target must not slip between two positions.
func TestVeryFastSweepStillCoversTheWholeFan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.HyperBeam.SweepDuration = 0.001 // far shorter than a tick
	for _, b := range []float64{-28, -10, 0, 10, 28} {
		g := beamGame(cfg)
		g.P[pikachu].X, g.P[pikachu].Y = at(b, 350)
		if hitTick(g) == 0 {
			t.Errorf("target at %v degrees was skipped by a one-tick sweep", b)
		}
	}
	cfg.Snorlax.HyperBeam.SweepAngle = 180 // a full circle
	g := beamGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = at(180, 300) // right behind Snorlax
	if hitTick(g) == 0 {
		t.Error("a 180 degree sweep must reach the target behind Snorlax")
	}
}

func TestBeamCannotHitAirborneTarget(t *testing.T) {
	g := beamGame(DefaultConfig())
	g.P[pikachu].Z = 1000 // stays airborne for the whole test
	if hitTick(g) != 0 {
		t.Fatalf("HP = %d, airborne target must not be hit", g.P[pikachu].HP)
	}
}

func TestBeamCannotStartInTheAirOrForPikachu(t *testing.T) {
	g := beamGame(DefaultConfig())
	g.P[snorlax].Z, g.P[snorlax].VZ = 50, 0
	stepBeam(g)
	if g.P[snorlax].BeamT != 0 {
		t.Fatal("Snorlax started a beam while airborne")
	}

	g = beamGame(DefaultConfig())
	g.P[pikachu].In = Input{Beam: true}
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if p := g.P[pikachu]; p.BeamT != 0 || p.SweepT != 0 || p.RechargeT != 0 {
		t.Fatal("Pikachu used the beam")
	}
}

func TestBeamFollowsConfig(t *testing.T) {
	cfg := DefaultConfig()
	hb := &cfg.Snorlax.HyperBeam
	hb.ChargeDuration, hb.SweepDuration, hb.Damage, hb.Length = 1.0, 1.0, 55, 100 // 100 < 300: out of reach
	g := beamGame(cfg)
	if hitTick(g) != 0 {
		t.Fatalf("HP = %d, length 100 must not reach 300 away", g.P[pikachu].HP)
	}

	hb.Length = 450
	g = beamGame(cfg)
	stepBeam(g)
	sweepStart, sweepEnd := 0, 0
	for tick := 2; tick <= 4*tickRate; tick++ {
		g.Step(dt)
		if sweepStart == 0 && g.P[snorlax].SweepT > 0 {
			sweepStart = tick
		}
		if sweepStart != 0 && sweepEnd == 0 && g.P[snorlax].SweepT == 0 {
			sweepEnd = tick
		}
	}
	if sweepStart != tickRate { // 1.0s charge = 30 ticks
		t.Fatalf("sweep started at tick %d, want %d for a 1.0s charge", sweepStart, tickRate)
	}
	if got := sweepEnd - sweepStart; got != tickRate { // 1.0s sweep = 30 ticks
		t.Fatalf("sweep lasted %d ticks, want %d for a 1.0s sweep", got, tickRate)
	}
	if g.P[pikachu].HP != maxHP-55 {
		t.Fatalf("HP = %d, want %d", g.P[pikachu].HP, maxHP-55)
	}
}

// The client reads these keys; a wrong struct tag would silently drop them.
func TestPlayerSnapshotCarriesBeamState(t *testing.T) {
	b, err := json.Marshal(Player{})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"x", "y", "z", "hp", "cd", "bt", "st", "rt", "bx", "by", "ba"} {
		if _, ok := m[k]; !ok {
			t.Errorf("snapshot is missing %q: %s", k, b)
		}
	}
}

// ---- shared mechanics ----

func TestQuakeCannotReachAirborneButHitsGrounded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		z     float64
		wantH int
	}{
		{"airborne", 100, maxHP},
		{"grounded", 0, maxHP - quakeDamage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGame()
			g.P[pikachu].X, g.P[snorlax].X = 300, 350
			g.P[pikachu].Y, g.P[snorlax].Y = 250, 250
			g.P[snorlax].Z, g.P[snorlax].VZ = tc.z, 0
			g.P[pikachu].In.Attack = true
			g.Step(dt)
			if g.P[snorlax].HP != tc.wantH {
				t.Fatalf("HP = %d, want %d", g.P[snorlax].HP, tc.wantH)
			}
		})
	}
}

func TestQuakeCooldownLimitsHits(t *testing.T) {
	g := newTestGame()
	g.P[pikachu].X, g.P[snorlax].X = 300, 350
	g.P[pikachu].Y, g.P[snorlax].Y = 250, 250
	g.P[pikachu].In.Attack = true
	for i := 0; i < 5; i++ { // 5 ticks = 0.17s, well inside the 0.8s cooldown
		g.Step(dt)
	}
	if g.P[snorlax].HP != maxHP-quakeDamage {
		t.Fatalf("HP = %d, want exactly one hit", g.P[snorlax].HP)
	}
}

func TestDiagonalMoveIsNotFaster(t *testing.T) {
	g := newTestGame()
	x0, y0 := g.P[pikachu].X, g.P[pikachu].Y
	g.P[pikachu].In.DX, g.P[pikachu].In.DY = 1, 1
	g.Step(1)
	d := dist(&Player{X: x0, Y: y0}, &g.P[pikachu])
	if d < speed-0.001 || d > speed+0.001 {
		t.Fatalf("diagonal distance = %v, want %v", d, speed)
	}
}

func TestWinner(t *testing.T) {
	g := newTestGame()
	if g.Winner() != -1 {
		t.Fatal("no winner at start")
	}
	g.P[1].HP = 0
	if g.Winner() != 0 {
		t.Fatalf("winner = %d, want 0", g.Winner())
	}
}
