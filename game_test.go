package main

import (
	"encoding/json"
	"fmt"
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

// ---- Snorlax: air speed, jump cooldown, earthquake reach ----

// While Snorlax is above the ground it steers at air_speed; the moment it lands, at the walking speed.
func TestSnorlaxSteersInTheAirAtAirSpeed(t *testing.T) {
	for _, tc := range []struct {
		name string
		air  float64
	}{{"slower than walking", 60}, {"as fast as walking", speed}, {"no steering at all", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.AirSpeed = tc.air
			g := NewGame(cfg)
			g.P[snorlax].X = 1000
			g.P[snorlax].In = Input{Skill: true}
			g.Step(dt) // take-off; no horizontal key yet
			g.P[snorlax].In = Input{DX: 1}

			airTicks, moved := 0, 0.0
			for g.P[snorlax].Z > 0 && airTicks < 100 {
				before := g.P[snorlax].X
				g.Step(dt)
				airTicks, moved = airTicks+1, moved+g.P[snorlax].X-before
			}
			if airTicks < 15 {
				t.Fatalf("setup: only %d airborne ticks, the jump should last ~24", airTicks)
			}
			if want := tc.air * dt * float64(airTicks); math.Abs(moved-want) > 1e-6 {
				t.Fatalf("moved %.3f in the air, want %.3f (%v u/s for %d ticks)", moved, want, tc.air, airTicks)
			}

			x := g.P[snorlax].X // landed: walking speed again
			g.Step(dt)
			if got := g.P[snorlax].X - x; math.Abs(got-speed*dt) > 1e-9 {
				t.Fatalf("moved %.4f on the ground, want %.4f", got, speed*dt)
			}
		})
	}
}

func TestAirSpeedOnlyAffectsSnorlax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.AirSpeed = 0
	g := NewGame(cfg)
	x := g.P[pikachu].X
	g.P[pikachu].In = Input{DX: 1}
	g.Step(dt)
	if got := g.P[pikachu].X - x; math.Abs(got-speed*dt) > 1e-9 {
		t.Fatalf("Pikachu moved %.4f, want its normal %.4f", got, speed*dt)
	}
}

// When the configured duration is a whole number of ticks, the landing must happen on
// that very tick. Float rounding used to leave the height at +3e-15 and cost one extra tick.
func TestJumpLandsOnTheTickTheConfiguredDurationEnds(t *testing.T) {
	for _, duration := range []float64{0.5, 0.8, 1.0, 1.2} {
		cfg := DefaultConfig()
		cfg.Snorlax.JumpDuration = duration
		g := NewGame(cfg)
		g.P[snorlax].In = Input{Skill: true}
		g.Step(dt) // tick 1: take-off
		g.P[snorlax].In = Input{}
		landed := 0
		for tick := 2; tick <= 100 && landed == 0; tick++ {
			g.Step(dt)
			if g.P[snorlax].Z == 0 {
				landed = tick
			}
		}
		if want := int(math.Round(duration * tickRate)); landed != want {
			t.Errorf("duration %vs: landed on tick %d, want %d", duration, landed, want)
		}
	}
}

// takeoffs holds the jump key down and returns the ticks on which Snorlax leaves the ground.
func takeoffs(cfg Config, ticks int) []int {
	g := NewGame(cfg)
	g.P[snorlax].In = Input{Skill: true}
	var out []int
	for tick := 1; tick <= ticks; tick++ {
		was := g.P[snorlax].Z
		g.Step(dt)
		if was == 0 && g.P[snorlax].Z > 0 {
			out = append(out, tick)
		}
	}
	return out
}

func TestJumpCooldownCountsFromTakeoff(t *testing.T) {
	// A jump lasts 0.8s = 24 ticks, so Snorlax is back on the ground on tick 24.
	for _, tc := range []struct {
		name     string
		cooldown float64
		want     []int
	}{
		{"1.5s: the next jump is allowed 45 ticks after take-off", 1.5, []int{1, 46, 91}},
		{"no cooldown: jumps again the tick after landing", 0, []int{1, 25, 49, 73, 97}},
		{"shorter than the jump: the landing is what is waited for", 0.5, []int{1, 25, 49, 73, 97}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.JumpCooldown = tc.cooldown
			if got := takeoffs(cfg, 100); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("take-offs on ticks %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSnorlaxQuakeReachFollowsConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		radius float64
		gap    float64 // distance to Pikachu
		hit    bool
	}{
		{"default 100: 90 away is hit", 100, 90, true},
		{"default 100: 150 away is not", 100, 150, false},
		{"widened to 200: 190 away is hit", 200, 190, true},
		{"widened to 200: 210 away is not", 200, 210, false},
		{"shrunk to 40: 90 away is not", 40, 90, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.QuakeRadius = tc.radius
			g := NewGame(cfg)
			g.P[snorlax].X, g.P[snorlax].Y = 800, 500
			g.P[pikachu].X, g.P[pikachu].Y = 800-tc.gap, 500
			g.P[snorlax].In = Input{Attack: true}
			events := g.Step(dt)
			if got := g.P[pikachu].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v, want %v", got, tc.hit)
			}
			var r float64
			for _, e := range events {
				if e.T == "quake" {
					r = e.R
				}
			}
			if r != tc.radius {
				t.Fatalf("the quake event carries reach %v, want %v so the effect can match it", r, tc.radius)
			}
		})
	}
}

// The configurable reach is Snorlax's; Pikachu's own earthquake keeps its fixed one.
func TestPikachuQuakeReachIsNotAffectedBySnorlaxConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.QuakeRadius = 300
	g := NewGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = 800, 500
	g.P[snorlax].X, g.P[snorlax].Y = 800+150, 500 // inside 300, outside Pikachu's 100
	g.P[pikachu].In = Input{Attack: true}
	events := g.Step(dt)
	if g.P[snorlax].HP != maxHP {
		t.Fatal("Pikachu's earthquake must not use Snorlax's widened reach")
	}
	for _, e := range events {
		if e.T == "quake" && e.R != quakeRadius {
			t.Fatalf("Pikachu's quake event carries reach %v, want %v", e.R, quakeRadius)
		}
	}
}

func TestQuakeEventCarriesItsReachToTheClient(t *testing.T) {
	b, _ := json.Marshal(Event{T: "quake", X: 1, Y: 2, R: 100})
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["r"] != float64(100) {
		t.Fatalf("event JSON = %s, want an \"r\" key", b)
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

// hitTickAt fires one beam while Pikachu stands dead ahead (so the aim locks
// straight left), then moves Pikachu to bearing b, distance r from that aim.
// It returns the tick on which Pikachu first lost HP, or 0.
func hitTickAt(cfg Config, b, r float64) int {
	g := beamGame(cfg)
	stepBeam(g) // tick 1: the aim locks on Pikachu at (300, 250)
	g.P[pikachu].X, g.P[pikachu].Y = at(b, r)
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
	tp, tm := hitTickAt(DefaultConfig(), 20, 300), hitTickAt(DefaultConfig(), -20, 300)
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
			if got := hitTickAt(DefaultConfig(), tc.b, tc.r) != 0; got != tc.hit {
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
	stepBeam(g)                                   // the aim locks straight left
	g.P[pikachu].X, g.P[pikachu].Y = at(-25, 300) // near the end of the sweep
	for i := 1; i < chargeTicks(cfg)+5; i++ {     // charge done, a third of the sweep gone
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

// The beam is aimed at wherever the opponent stands when it is used, at any angle,
// whatever Snorlax faces and whichever keys are held.
func TestBeamAimsStraightAtTheOpponent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target [2]float64 // Pikachu; Snorlax stands at (800, 500)
		hit    bool
	}{
		{"to the right", [2]float64{1100, 500}, true},
		{"straight up", [2]float64{800, 200}, true},
		{"an odd angle (17 degrees)", [2]float64{1230, 625}, true},
		{"an odd angle (123 degrees)", [2]float64{650, 745}, true},
		{"up-left", [2]float64{560, 260}, true},
		{"out of reach: aimed, but too far", [2]float64{1500, 500}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGame(DefaultConfig())
			g.P[snorlax].X, g.P[snorlax].Y = 800, 500
			g.P[pikachu].X, g.P[pikachu].Y = tc.target[0], tc.target[1]
			g.P[snorlax].In = Input{DX: -1, DY: 1, Beam: true} // walking away from it, facing the other way
			g.Step(dt)

			dx, dy := tc.target[0]-800, tc.target[1]-500
			n := math.Hypot(dx, dy)
			if bx, by := g.P[snorlax].BeamX, g.P[snorlax].BeamY; math.Abs(bx-dx/n) > 1e-9 || math.Abs(by-dy/n) > 1e-9 {
				t.Fatalf("aim = (%.4f, %.4f), want (%.4f, %.4f)", bx, by, dx/n, dy/n)
			}
			g.P[snorlax].In = Input{}
			for i := 0; i < 2*tickRate; i++ {
				g.Step(dt)
			}
			if got := g.P[pikachu].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v, want %v", got, tc.hit)
			}
		})
	}
}

// The aim is taken when the beam is used. Running to the other side afterwards
// does not drag the beam along, and steering keys do not re-aim it.
func TestBeamAimIsLockedWhenUsed(t *testing.T) {
	g := beamGame(DefaultConfig())
	stepBeam(g)                               // aims straight left, at Pikachu (300, 250)
	g.P[pikachu].X, g.P[pikachu].Y = 900, 250 // runs to the far side of Snorlax
	g.P[snorlax].In = Input{DX: 1}            // and Snorlax tries to turn toward it
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if g.P[snorlax].BeamX != -1 || g.P[snorlax].BeamY != 0 {
		t.Fatalf("aim = (%v, %v), want it still locked straight left", g.P[snorlax].BeamX, g.P[snorlax].BeamY)
	}
	if g.P[pikachu].HP != maxHP {
		t.Fatalf("HP = %d: the beam went where Pikachu had been, so it must miss", g.P[pikachu].HP)
	}
}

// With no distance to the opponent there is no direction to aim; the facing is used, never NaN.
func TestBeamOnTopOfTheOpponentFallsBackToTheFacing(t *testing.T) {
	g := beamGame(DefaultConfig())
	g.P[pikachu].X, g.P[pikachu].Y = 600, 250 // exactly on Snorlax
	stepBeam(g)
	bx, by := g.P[snorlax].BeamX, g.P[snorlax].BeamY
	if bx != -1 || by != 0 || math.IsNaN(bx) || math.IsNaN(by) {
		t.Fatalf("aim = (%v, %v), want the facing (-1, 0)", bx, by)
	}
}

// ---- cooldowns count from the moment a skill is used ----

func TestBeamCooldownCountsFromTheMomentItIsUsed(t *testing.T) {
	cfg := DefaultConfig() // cooldown 5s: longer than the beam's own 2.6s cycle
	hb := cfg.Snorlax.HyperBeam
	if hb.Cooldown != 5 {
		t.Fatalf("setup: default cooldown = %v, want 5", hb.Cooldown)
	}
	g := beamGame(cfg)
	g.P[snorlax].In = Input{Beam: true} // key held the whole time
	var starts []int
	prev := false
	for tick := 1; tick <= 8*tickRate; tick++ {
		g.Step(dt)
		if charging := g.P[snorlax].BeamT > 0; charging && !prev {
			starts = append(starts, tick)
		} else {
			prev = charging
			continue
		}
		prev = true
	}
	// Used on tick 1; the 5s cooldown (150 ticks) is over on tick 151.
	if len(starts) != 2 || starts[0] != 1 || starts[1] != 151 {
		t.Fatalf("beams started on ticks %v, want [1 151]", starts)
	}
}

func TestThunderCooldownCountsFromTheMomentTheCastStarts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cooldown float64
		until    int // how many ticks to watch
		want     []int
	}{
		// First cast: bolts on 30, 45, 60. The next cast starts when the cooldown is over.
		{"cooldown 4s: the next cast starts on tick 121", 4, 185, []int{30, 45, 60, 150, 165, 180}},
		{"no cooldown: the next cast starts as soon as the first ends (tick 61)", 0, 125, []int{30, 45, 60, 90, 105, 120}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Pikachu.Thunder.Cooldown = tc.cooldown
			g := thunderGame(cfg)
			g.P[pikachu].In = Input{Thunder: true} // key held the whole time
			ticks := boltTicks(g, 1, tc.until)
			if fmt.Sprint(ticks) != fmt.Sprint(tc.want) {
				t.Fatalf("bolts at ticks %v, want %v", ticks, tc.want)
			}
		})
	}
}

func TestSnorlaxIsRootedThroughChargeSweepAndRecharge(t *testing.T) {
	cfg := DefaultConfig()
	hb := cfg.Snorlax.HyperBeam
	g := beamGame(cfg)
	stepBeam(g)                               // the aim locks straight left
	g.P[pikachu].X, g.P[pikachu].Y = 580, 330 // inside quake range (82 away) but 76 degrees off the fan
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
	cfg := DefaultConfig()
	cfg.Snorlax.HyperBeam.Cooldown = 0 // so only the beam's own phases are in the way
	g := beamGame(cfg)                 // charge 0.6 + sweep 0.5 + recharge 1.5 = 2.6s per beam
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
	if hitTick(beamGame(cfg)) == 0 {
		t.Fatal("a fixed beam must hit what is dead ahead")
	}
	if hitTickAt(cfg, 10, 300) != 0 { // asin(25/300) = 4.8 degrees of reach
		t.Fatal("a fixed beam must not reach 10 degrees off its line")
	}
}

// One tick can cover the whole arc; the target must not slip between two positions.
func TestVeryFastSweepStillCoversTheWholeFan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.HyperBeam.SweepDuration = 0.001 // far shorter than a tick
	for _, b := range []float64{-28, -10, 0, 10, 28} {
		if hitTickAt(cfg, b, 350) == 0 {
			t.Errorf("target at %v degrees was skipped by a one-tick sweep", b)
		}
	}
	cfg.Snorlax.HyperBeam.SweepAngle = 180 // a full circle
	if hitTickAt(cfg, 180, 300) == 0 {     // right behind Snorlax
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
	for _, k := range []string{"x", "y", "z", "hp", "cd", "bt", "st", "rt", "bx", "by", "ba", "tn", "ti", "sx", "sy"} {
		if _, ok := m[k]; !ok {
			t.Errorf("snapshot is missing %q: %s", k, b)
		}
	}
}

// ---- Pikachu: Thunder (a shadow chases the opponent, bolts hit wherever it is) ----

// thunderGame has Pikachu at (200,250) and Snorlax at (600,250), well apart.
func thunderGame(cfg Config) *Game { return NewGame(cfg) }

// castThunder presses the thunder key for one tick (tick 1) and lets go.
func castThunder(g *Game) []Event {
	g.P[pikachu].In = Input{Thunder: true}
	ev := g.Step(dt)
	g.P[pikachu].In = Input{}
	return ev
}

func thunderEvents(ev []Event) (n int, last Event) {
	for _, e := range ev {
		if e.T == "thunder" {
			n, last = n+1, e
		}
	}
	return
}

// boltTicks steps from tick `from` to `to` (the cast press is tick 1) and returns
// the ticks on which a bolt struck.
func boltTicks(g *Game, from, to int) []int {
	var ticks []int
	for tick := from; tick <= to; tick++ {
		if n, _ := thunderEvents(g.Step(dt)); n > 0 {
			ticks = append(ticks, tick)
		}
	}
	return ticks
}

func TestThunderShadowAppearsOnTheOpponentThenBoltsFollowTheSchedule(t *testing.T) {
	cfg := DefaultConfig() // delay 1.0s, interval 0.5s, 3 strikes
	g := thunderGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 640, 310
	if n, _ := thunderEvents(castThunder(g)); n != 0 {
		t.Fatal("a bolt must not strike on the tick of the cast")
	}
	p := &g.P[pikachu]
	if p.ThunderLeft != cfg.Pikachu.Thunder.Strikes || p.ShadowX != 640 || p.ShadowY != 310 {
		t.Fatalf("after the cast: left=%d shadow=(%v,%v), want %d bolts and the shadow on the opponent (640,310)",
			p.ThunderLeft, p.ShadowX, p.ShadowY, cfg.Pikachu.Thunder.Strikes)
	}

	ticks := boltTicks(g, 2, 4*tickRate)
	// delay 1.0s = tick 30, then every 0.5s = 15 ticks.
	if want := []int{30, 45, 60}; len(ticks) != 3 || ticks[0] != want[0] || ticks[1] != want[1] || ticks[2] != want[2] {
		t.Fatalf("bolts at ticks %v, want %v", ticks, want)
	}
	if p.ThunderLeft != 0 {
		t.Fatalf("ThunderLeft = %d after the last bolt, want the shadow gone", p.ThunderLeft)
	}
}

func TestThunderEveryBoltDoesItsDamageToAStandingTarget(t *testing.T) {
	cfg := DefaultConfig()
	g := thunderGame(cfg)
	castThunder(g)
	for i := 0; i < 4*tickRate; i++ {
		g.Step(dt)
	}
	th := cfg.Pikachu.Thunder
	if want := maxHP - th.Damage*th.Strikes; g.P[snorlax].HP != want {
		t.Fatalf("HP = %d, want %d (%d bolts x %d)", g.P[snorlax].HP, want, th.Strikes, th.Damage)
	}
}

// Snorlax walks at 180 u/s. A shadow slower than that falls behind and the bolt misses;
// one that is at least as fast stays on top of it.
func TestThunderShadowFollowSpeedDecidesWhetherYouCanOutrunIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		speed float64
		hit   bool
	}{
		{"slower shadow is outrun (lag 80 > radius 60)", 100, false},
		{"just fast enough to keep up", 200, true},
		{"shadow that never moves", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Pikachu.Thunder.FollowSpeed = tc.speed
			cfg.Pikachu.Thunder.Strikes = 1
			g := thunderGame(cfg)
			castThunder(g)
			for i := 0; i < 36; i++ { // past the bolt at tick 30, and short of the wall (y = 250 + 180 * 1.2)
				g.P[snorlax].In = Input{DY: 1} // keep walking down the whole time
				g.Step(dt)
			}
			if got := g.P[snorlax].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v, want %v", got, tc.hit)
			}
		})
	}
}

func TestThunderShadowChasesWithoutOvershooting(t *testing.T) {
	cfg := DefaultConfig()
	g := thunderGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 600, 250
	castThunder(g)
	g.P[snorlax].X, g.P[snorlax].Y = 620, 250 // 20 away; one tick of chasing covers 120*dt = 4
	g.Step(dt)
	if got := g.P[pikachu].ShadowX; math.Abs(got-(600+cfg.Pikachu.Thunder.FollowSpeed*dt)) > 1e-9 {
		t.Fatalf("shadow x = %v, want one tick of chasing", got)
	}
	g.P[snorlax].X = g.P[pikachu].ShadowX + 1 // closer than one step: it must land exactly, not overshoot
	g.Step(dt)
	if g.P[pikachu].ShadowX != g.P[snorlax].X {
		t.Fatalf("shadow x = %v, want it exactly on the target at %v", g.P[pikachu].ShadowX, g.P[snorlax].X)
	}
}

func TestThunderHitAreaIsTheConfiguredRadius(t *testing.T) {
	for _, tc := range []struct {
		name string
		off  float64 // how far the target stands from where the shadow ends up
		hit  bool
	}{{"inside", 55, true}, {"on the edge", 60, true}, {"just outside", 65, false}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Pikachu.Thunder.FollowSpeed = 0 // the shadow stays where it appeared
			cfg.Pikachu.Thunder.Strikes = 1
			g := thunderGame(cfg)
			castThunder(g)
			g.P[snorlax].X += tc.off
			for i := 0; i < 2*tickRate; i++ {
				g.Step(dt)
			}
			if got := g.P[snorlax].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v, want %v", got, tc.hit)
			}
		})
	}
}

// Lightning comes from the sky, unlike the ground attacks: jumping does not dodge it.
func TestThunderHitsAnAirborneTarget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Thunder.Strikes = 1
	g := thunderGame(cfg)
	castThunder(g)
	g.P[snorlax].Z = 1000 // still falling when the bolt arrives
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if g.P[snorlax].HP != maxHP-cfg.Pikachu.Thunder.Damage {
		t.Fatalf("HP = %d, an airborne target must still be hit", g.P[snorlax].HP)
	}
}

func TestThunderDoesNotRootPikachuAndCannotBeStacked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Thunder.Cooldown = 0 // so only the "one cast at a time" rule is in the way
	g := thunderGame(cfg)
	castThunder(g)
	x := g.P[pikachu].X
	for i := 0; i < 10; i++ {
		g.P[pikachu].In = Input{DX: -1, Thunder: true} // walking, and mashing the thunder key
		g.Step(dt)
	}
	if g.P[pikachu].X >= x {
		t.Fatal("Pikachu must be free to move while the bolts are coming")
	}
	if g.P[pikachu].ThunderLeft != cfg.Pikachu.Thunder.Strikes {
		t.Fatalf("ThunderLeft = %d, want an unchanged %d: pressing again must not restart or add bolts",
			g.P[pikachu].ThunderLeft, cfg.Pikachu.Thunder.Strikes)
	}

	// Once the cast is over, the next one is allowed.
	g.P[pikachu].In = Input{}
	for i := 0; i < 3*tickRate; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].ThunderLeft != 0 {
		t.Fatal("setup: the cast should have finished")
	}
	castThunder(g)
	if g.P[pikachu].ThunderLeft != cfg.Pikachu.Thunder.Strikes {
		t.Fatal("a finished cast must allow a new one")
	}
}

func TestOnlyPikachuCanCastThunder(t *testing.T) {
	g := thunderGame(DefaultConfig())
	g.P[snorlax].In = Input{Thunder: true}
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if g.P[snorlax].ThunderLeft != 0 || g.P[pikachu].HP != maxHP {
		t.Fatal("Snorlax cast thunder")
	}
}

func TestThunderIntervalZeroDropsAllBoltsOnTheSameTick(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Thunder.Interval = 0
	g := thunderGame(cfg)
	castThunder(g)
	var perTick []int
	for i := 0; i < 2*tickRate; i++ {
		if n, _ := thunderEvents(g.Step(dt)); n > 0 {
			perTick = append(perTick, n)
		}
	}
	if len(perTick) != 1 || perTick[0] != cfg.Pikachu.Thunder.Strikes {
		t.Fatalf("bolts per striking tick = %v, want a single tick with all %d", perTick, cfg.Pikachu.Thunder.Strikes)
	}
}

// Holding the key must not push the first bolt back: a restart would reset the delay every tick.
func TestThunderHoldingTheKeyDoesNotRestartTheCast(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Thunder.Cooldown = 0 // so only the "one cast at a time" rule is in the way
	g := thunderGame(cfg)
	g.P[pikachu].In = Input{Thunder: true} // held from tick 1 onward
	var ticks []int
	for tick := 1; tick <= 89; tick++ { // the cast ends on tick 60; the held key starts the next one on tick 61
		if n, _ := thunderEvents(g.Step(dt)); n > 0 {
			ticks = append(ticks, tick)
		}
	}
	if want := []int{30, 45, 60}; len(ticks) != 3 || ticks[0] != want[0] || ticks[1] != want[1] || ticks[2] != want[2] {
		t.Fatalf("bolts at ticks %v, want %v", ticks, want)
	}
}

func TestThunderFollowsConfig(t *testing.T) {
	cfg := DefaultConfig()
	th := &cfg.Pikachu.Thunder
	th.Delay, th.Interval, th.Strikes, th.Damage = 2.0, 1.0, 2, 33
	g := thunderGame(cfg)
	castThunder(g)
	ticks := boltTicks(g, 2, 5*tickRate)
	if len(ticks) != 2 || ticks[0] != 2*tickRate || ticks[1] != 3*tickRate {
		t.Fatalf("bolts at ticks %v, want [60 90] for delay 2s, interval 1s, 2 strikes", ticks)
	}
	if g.P[snorlax].HP != maxHP-66 {
		t.Fatalf("HP = %d, want %d", g.P[snorlax].HP, maxHP-66)
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

func TestPlayersSpawnCentredInTheArenaFourHundredApart(t *testing.T) {
	g := newTestGame()
	a, b := g.P[pikachu], g.P[snorlax]
	if a.Y != arenaH/2 || b.Y != arenaH/2 {
		t.Fatalf("spawn rows = %v and %v, want the middle row %v", a.Y, b.Y, arenaH/2)
	}
	if mid := (a.X + b.X) / 2; mid != arenaW/2 || b.X-a.X != 400 {
		t.Fatalf("spawn x = %v and %v, want them 400 apart around the middle %v", a.X, b.X, arenaW/2)
	}
	if arenaW != 1600 || arenaH != 1000 {
		t.Fatalf("arena = %vx%v, want 1600x1000 (twice the original 800x500)", arenaW, arenaH)
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
