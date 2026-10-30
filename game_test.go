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
		cfg.Snorlax.Jump.Height, cfg.Snorlax.Jump.Duration = tc.height, tc.duration
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

// ---- Snorlax: air speed, jump cooldown ----

// In the air Snorlax steers at air_speed_first_half while rising and air_speed_second_half while
// falling; the moment it lands, at the walking speed.
func TestSnorlaxSteersAtOneSpeedRisingAndAnotherFalling(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second float64
	}{
		{"slow rise, fast fall", 60, 240},
		{"fast rise, slow fall", 240, 60},
		{"no steering while rising", 0, speed},
		{"no steering while falling", speed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig() // 0.8s jump: the halves are 0.4s each
			cfg.Snorlax.Jump.AirSpeedFirstHalf, cfg.Snorlax.Jump.AirSpeedSecondHalf = tc.first, tc.second
			g := NewGame(cfg)
			g.P[snorlax].X = 800
			g.P[snorlax].In = Input{Skill: true}
			g.Step(dt) // tick 1: take-off, no horizontal key yet
			g.P[snorlax].In = Input{DX: 1}

			rising, falling := 0, 0
			for n := 2; g.P[snorlax].Z > 0 && n < 100; n++ {
				before := g.P[snorlax].X
				g.Step(dt)
				moved := g.P[snorlax].X - before
				elapsed := float64(n-1) * dt // time since take-off when this tick starts
				var want float64
				switch {
				case elapsed < 0.4-dt/2:
					want, rising = tc.first, rising+1
				case elapsed > 0.4+dt/2:
					want, falling = tc.second, falling+1
				default:
					continue // the apex tick: which half it belongs to is down to rounding
				}
				if math.Abs(moved-want*dt) > 1e-9 {
					t.Fatalf("tick %d (%.3fs after take-off) moved %.4f, want %.4f", n, elapsed, moved, want*dt)
				}
			}
			if rising < 10 || falling < 10 {
				t.Fatalf("setup: checked %d rising and %d falling ticks, the jump should give ~11 of each", rising, falling)
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
	cfg.Snorlax.Jump.AirSpeedFirstHalf, cfg.Snorlax.Jump.AirSpeedSecondHalf = 0, 0
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
		cfg.Snorlax.Jump.Duration = duration
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
			cfg.Snorlax.Jump.Cooldown = tc.cooldown
			if got := takeoffs(cfg, 100); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("take-offs on ticks %v, want %v", got, tc.want)
			}
		})
	}
}

// The landing's reach is slam_radius, and the event tells the client so the effect grows with it.
func TestSlamReachFollowsConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		radius float64
		gap    float64 // distance to Pikachu
		hit    bool
	}{
		{"default 70: 60 away is hit", 70, 60, true},
		{"default 70: 90 away is not", 70, 90, false},
		{"widened to 200: 190 away is hit", 200, 190, true},
		{"widened to 200: 220 away is not", 200, 220, false}, // reach 200 plus Pikachu's body (16)
		{"shrunk to 30: 60 away is not", 30, 60, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.Jump.SlamRadius = tc.radius
			g := NewGame(cfg)
			g.P[snorlax].X, g.P[snorlax].Y = 800, 500
			g.P[pikachu].X, g.P[pikachu].Y = 800-tc.gap, 500
			g.P[snorlax].Z, g.P[snorlax].VZ = 1, -100 // lands on the next step
			var r float64
			for _, e := range g.Step(dt) {
				if e.T == "slam" {
					r = e.R
				}
			}
			if got := g.P[pikachu].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v, want %v", got, tc.hit)
			}
			if r != tc.radius {
				t.Fatalf("slam event reach = %v, want %v so the effect matches", r, tc.radius)
			}
		})
	}
}

// The landing is drawn with the earthquake effect, so the event tells the client how far it reaches.
func TestSlamEventCarriesItsReachToTheClient(t *testing.T) {
	g := newTestGame()
	g.P[snorlax].Z, g.P[snorlax].VZ = 1, -100 // lands on the next step
	var slam *Event
	for _, e := range g.Step(dt) {
		if e.T == "slam" {
			e := e
			slam = &e
		}
	}
	if slam == nil || slam.R != slamRadius {
		t.Fatalf("slam event = %+v, want reach %v", slam, slamRadius)
	}
}

func TestQuakeEventCarriesItsReachToTheClient(t *testing.T) {
	b, _ := json.Marshal(Event{T: "slam", X: 1, Y: 2, R: 100})
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
		cfg.Pikachu.VoltTackle.Distance, cfg.Pikachu.VoltTackle.Duration = tc.distance, tc.duration
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
			if d := math.Hypot(dx, dy); math.Abs(d-DefaultConfig().Pikachu.VoltTackle.Distance) > 0.01 {
				t.Fatalf("diagonal dash covered %.2f, want %v (no diagonal bonus)", d, DefaultConfig().Pikachu.VoltTackle.Distance)
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
	cfg := DefaultConfig()             // 0.2s dash, 0.6s cooldown
	cfg.Pikachu.VoltTackle.MaxUses = 1 // a single use: the cooldown follows it at once
	g := NewGame(cfg)
	g.P[pikachu].X = 50
	g.P[pikachu].In = Input{DX: 1, Skill: true} // space held the whole time

	// 0.5s: the first dash is over (0.2s) but the cooldown (0.6s) is not.
	for i := 0; i < tickRate/2; i++ {
		g.Step(dt)
		g.P[pikachu].In.DX = 0 // stop walking so only dashes move it
	}
	if got := g.P[pikachu].X - 50; math.Abs(got-cfg.Pikachu.VoltTackle.Distance) > 0.01 {
		t.Fatalf("moved %.2f in 0.5s, want exactly one dash (%v)", got, cfg.Pikachu.VoltTackle.Distance)
	}
	// Once the cooldown has passed, the held key dashes again.
	for i := 0; i < tickRate/2; i++ {
		g.Step(dt)
	}
	if got := g.P[pikachu].X - 50; got < 2*cfg.Pikachu.VoltTackle.Distance-0.01 {
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
	g.P[pikachu].X, g.P[pikachu].Y = 580, 330 // 76 degrees off the fan
	n := chargeTicks(cfg) + sweepTicks(cfg) + int(math.Ceil(hb.RechargeDuration/dt)) - 2
	for i := 0; i < n; i++ { // every key held for the whole sequence
		g.P[snorlax].In = Input{DX: 1, Skill: true}
		g.Step(dt)
		if g.P[snorlax].X != 600 || g.P[snorlax].Z != 0 {
			t.Fatalf("tick %d: Snorlax at x=%v z=%v, want rooted", i, g.P[snorlax].X, g.P[snorlax].Z)
		}
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
	for _, k := range []string{"x", "y", "z", "hp", "bt", "st", "rt", "bx", "by", "ba", "tn", "ti", "sx", "sy", "dcd", "bcd", "jcd", "tcd", "kt", "ecd", "fx", "fy", "ut", "ud", "ur", "udx", "udy", "pcd", "fz", "ax", "ay"} {
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
			cfg.Snorlax.Radius = 1 // so the miss is decided by the 60 radius of the strike
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
			cfg.Snorlax.Radius = 0              // a point target: the area alone decides
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

// Pikachu stands still from the cast until the tick of the last bolt (ticks 1..60 with the defaults).
func TestThunderRootsPikachuUntilTheLastBolt(t *testing.T) {
	g := thunderGame(DefaultConfig())
	x, y := g.P[pikachu].X, g.P[pikachu].Y
	for tick := 1; tick <= 60; tick++ {
		in := Input{DX: -1, DY: 1, Skill: true} // trying to walk and to dash
		in.Thunder = tick == 1
		g.P[pikachu].In = in
		g.Step(dt)
		if g.P[pikachu].X != x || g.P[pikachu].Y != y {
			t.Fatalf("tick %d: Pikachu moved to (%v,%v) while casting", tick, g.P[pikachu].X, g.P[pikachu].Y)
		}
	}
	if g.P[pikachu].ThunderLeft != 0 {
		t.Fatal("setup: the last bolt should have struck on tick 60")
	}
	g.P[pikachu].In = Input{DX: -1}
	g.Step(dt)
	if got := x - g.P[pikachu].X; math.Abs(got-speed*dt) > 1e-9 {
		t.Fatalf("after the cast Pikachu moved %.4f, want %.4f", got, speed*dt)
	}
}

// The root is only for Pikachu; Snorlax is never rooted by the cast.
func TestThunderRootDoesNotStopSnorlax(t *testing.T) {
	g := thunderGame(DefaultConfig())
	castThunder(g)
	x := g.P[snorlax].X
	g.P[snorlax].In = Input{DX: 1}
	g.Step(dt)
	if g.P[snorlax].X <= x {
		t.Fatal("Snorlax must keep moving while Pikachu casts")
	}
}

func TestThunderCannotBeStackedAndAllowsTheNextCastWhenDone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Thunder.Cooldown = 0 // so only the "one cast at a time" rule is in the way
	g := thunderGame(cfg)
	castThunder(g)
	for i := 0; i < 10; i++ {
		g.P[pikachu].In = Input{Thunder: true} // mashing the thunder key
		g.Step(dt)
	}
	if g.P[pikachu].ThunderLeft != cfg.Pikachu.Thunder.Strikes {
		t.Fatalf("ThunderLeft = %d, want an unchanged %d: pressing again must not restart or add bolts",
			g.P[pikachu].ThunderLeft, cfg.Pikachu.Thunder.Strikes)
	}

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

// Every strike is also fired up into the sky from where Pikachu stands, on the tick the bolt lands.
func TestThunderFiresOneSkyBoltPerStrikeFromPikachu(t *testing.T) {
	for _, strikes := range []int{1, 3, 5} {
		cfg := DefaultConfig()
		cfg.Pikachu.Thunder.Strikes = strikes
		g := thunderGame(cfg)
		g.P[pikachu].X, g.P[pikachu].Y = 333, 444
		castThunder(g)
		var sky, ground []int
		for tick := 2; tick <= 6*tickRate; tick++ {
			for _, e := range g.Step(dt) {
				switch e.T {
				case "skybolt":
					sky = append(sky, tick)
					if e.X != 333 || e.Y != 444 {
						t.Fatalf("sky bolt at (%v,%v), want it from Pikachu at (333,444)", e.X, e.Y)
					}
				case "thunder":
					ground = append(ground, tick)
				}
			}
		}
		if len(sky) != strikes || fmt.Sprint(sky) != fmt.Sprint(ground) {
			t.Fatalf("strikes=%d: sky bolts on ticks %v, ground bolts on %v; want one sky bolt per ground bolt", strikes, sky, ground)
		}
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
	if arenaW != 2000 || arenaH != 1000 {
		t.Fatalf("arena = %vx%v, want 2000x1000 (long enough for a phone held upright)", arenaW, arenaH)
	}
}

// ---- body size: a skill hits when its area touches the target's body ----

// Every skill reaches its own radius plus the target's body: at 130 apart, a reach of 100 misses a
// body of radius 10 and touches one of radius 40.
func TestBodyRadiusWidensWhatEverySkillCanHit(t *testing.T) {
	const gap = 130.0
	newG := func(r float64) *Game {
		cfg := DefaultConfig()
		cfg.Pikachu.Radius, cfg.Snorlax.Radius = r, r
		cfg.Pikachu.Thunder.Strikes, cfg.Pikachu.Thunder.FollowSpeed, cfg.Pikachu.Thunder.Radius = 1, 0, 100
		cfg.Snorlax.Jump.SlamRadius = 100
		hb := &cfg.Snorlax.HyperBeam
		hb.Length, hb.Width, hb.SweepAngle = 100, 20, 0 // a short fixed beam
		g := NewGame(cfg)
		g.P[pikachu].X, g.P[pikachu].Y = 800, 500
		g.P[snorlax].X, g.P[snorlax].Y = 800+gap, 500
		return g
	}
	skills := map[string]func(g *Game) bool{
		"thunder": func(g *Game) bool {
			castThunder(g) // the shadow appears on Snorlax and, not chasing, stays there
			g.P[snorlax].X += gap
			for i := 0; i < 2*tickRate; i++ {
				g.Step(dt)
			}
			return g.P[snorlax].HP < maxHP
		},
		"slam": func(g *Game) bool {
			g.P[snorlax].Z, g.P[snorlax].VZ = 1, -100 // lands on the next step
			g.Step(dt)
			return g.P[pikachu].HP < maxHP
		},
		"hyper beam": func(g *Game) bool {
			stepBeam(g)
			for i := 0; i < 3*tickRate; i++ {
				g.Step(dt)
			}
			return g.P[pikachu].HP < maxHP
		},
	}
	for name, hits := range skills {
		t.Run(name, func(t *testing.T) {
			if hits(newG(10)) {
				t.Fatal("a body of radius 10 is out of reach")
			}
			if !hits(newG(40)) {
				t.Fatal("a body of radius 40 must be touched")
			}
		})
	}
}

// ---- court, hit points and walking speed come from the config ----

// The view a room is opened from picks its court: a phone gets the tall one, the web page the wide one.
func TestCourtSizeDecidesSpawnAndWalls(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Court.Mobile = CourtSize{Width: 600, Length: 1200}
	cfg.Court.Web = CourtSize{Width: 900, Length: 1600}
	for _, tc := range []struct {
		mode string
		w, l float64
	}{{modeMobile, 600, 1200}, {modeWeb, 900, 1600}} {
		g := NewGameFor(cfg, tc.mode)
		if a, b := g.P[pikachu], g.P[snorlax]; a.X != tc.l/2-200 || b.X != tc.l/2+200 || a.Y != tc.w/2 || b.Y != tc.w/2 {
			t.Fatalf("%s: spawns at (%v,%v) and (%v,%v), want 200 either side of the middle of a %vx%v court", tc.mode, a.X, a.Y, b.X, b.Y, tc.l, tc.w)
		}
		g.P[pikachu].In = Input{DX: -1, DY: -1}
		g.P[snorlax].In = Input{DX: 1, DY: 1}
		for i := 0; i < 30*tickRate; i++ {
			g.Step(dt)
		}
		if a, b := g.P[pikachu], g.P[snorlax]; a.X != 0 || a.Y != 0 || b.X != tc.l || b.Y != tc.w {
			t.Fatalf("%s: after walking into the corners: (%v,%v) and (%v,%v), want (0,0) and (%v,%v)", tc.mode, a.X, a.Y, b.X, b.Y, tc.l, tc.w)
		}
	}
}

func TestMaxHPIsPerCharacter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.MaxHP, cfg.Snorlax.MaxHP = 60, 250
	g := NewGame(cfg)
	if g.P[pikachu].HP != 60 || g.P[snorlax].HP != 250 {
		t.Fatalf("HP = %d and %d, want 60 and 250", g.P[pikachu].HP, g.P[snorlax].HP)
	}
	g.Reset()
	if g.P[pikachu].HP != 60 || g.P[snorlax].HP != 250 {
		t.Fatalf("after Reset HP = %d and %d, want 60 and 250", g.P[pikachu].HP, g.P[snorlax].HP)
	}
}

func TestWalkSpeedIsPerCharacter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.WalkSpeed, cfg.Snorlax.WalkSpeed = 90, 240
	g := NewGame(cfg)
	x0, x1 := g.P[pikachu].X, g.P[snorlax].X
	g.P[pikachu].In, g.P[snorlax].In = Input{DX: -1}, Input{DX: 1}
	g.Step(dt)
	if got := x0 - g.P[pikachu].X; math.Abs(got-90*dt) > 1e-9 {
		t.Fatalf("Pikachu walked %.4f, want %.4f", got, 90*dt)
	}
	if got := g.P[snorlax].X - x1; math.Abs(got-240*dt) > 1e-9 {
		t.Fatalf("Snorlax walked %.4f, want %.4f", got, 240*dt)
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

// ---- Hyper Beam: knockback and repeated hits ----

// longBeam is a fixed beam (no sweep angle) that stays on its line for `seconds`, so a target in it can be hit again.
func longBeam(seconds, interval, knockback float64) *Game {
	cfg := DefaultConfig()
	hb := &cfg.Snorlax.HyperBeam
	hb.SweepAngle, hb.SweepDuration, hb.HitInterval, hb.Knockback = 0, seconds, interval, knockback
	hb.Damage, hb.Length = 10, 550
	return beamGame(cfg) // Snorlax at (600,250) facing left, Pikachu at (300,250) in the line
}

// hitTicks fires the beam (the press is tick 1) and returns the ticks on which Pikachu lost HP.
func hitTicks(g *Game, until int) []int {
	var ticks []int
	stepBeam(g)
	hp := g.P[pikachu].HP
	for tick := 2; tick <= until; tick++ {
		g.Step(dt)
		if g.P[pikachu].HP < hp {
			ticks = append(ticks, tick)
		}
		hp = g.P[pikachu].HP
	}
	return ticks
}

func TestBeamKnocksTheTargetStraightAwayFromSnorlax(t *testing.T) {
	g := longBeam(0.5, 0, 100)
	g.P[pikachu].X, g.P[pikachu].Y = 400, 250 // 200 to Snorlax's left, on the line
	hitTicks(g, 4*tickRate)
	if p := g.P[pikachu]; p.X != 300 || p.Y != 250 {
		t.Fatalf("Pikachu at (%v,%v), want it pushed 100 away from Snorlax to (300,250)", p.X, p.Y)
	}
}

func TestBeamKnockbackFollowsTheLineFromSnorlaxNotTheWorldAxes(t *testing.T) {
	g := longBeam(0.5, 0, 100)
	g.P[pikachu].X, g.P[pikachu].Y = 600-200*math.Cos(math.Pi/6), 250+200*math.Sin(math.Pi/6) // 30 degrees off, 200 away
	g.P[snorlax].X, g.P[snorlax].Y = 600, 250
	stepBeam(g)
	for i := 0; i < 2*tickRate && g.P[pikachu].HP == maxHP; i++ {
		g.Step(dt)
	}
	want := math.Hypot(g.P[pikachu].X-600, g.P[pikachu].Y-250)
	if math.Abs(want-300) > 1e-6 {
		t.Fatalf("Pikachu is %.3f from Snorlax, want 300 (200 + 100 along the same line)", want)
	}
}

func TestBeamKnockbackZeroLeavesTheTargetWhereItWas(t *testing.T) {
	g := longBeam(0.5, 0, 0)
	hitTicks(g, 3*tickRate)
	if p := g.P[pikachu]; p.X != 300 || p.HP == maxHP {
		t.Fatalf("Pikachu x=%v HP=%d, want it hit and not moved", p.X, p.HP)
	}
}

func TestBeamKnockbackStopsAtTheWall(t *testing.T) {
	g := longBeam(0.5, 0, 500)
	g.P[pikachu].X = 100
	stepBeam(g)
	for i := 0; i < 3*tickRate && g.P[pikachu].HP == maxHP; i++ {
		g.Step(dt)
	}
	// Looked at on the very tick of the hit: that is what the clients are sent.
	if g.P[pikachu].HP == maxHP {
		t.Fatal("setup: Pikachu was never hit")
	}
	if x := g.P[pikachu].X; x != 0 {
		t.Fatalf("Pikachu x = %v in the snapshot of the hit, want it stopped by the wall at 0", x)
	}
}

func TestBeamKnockbackWithNoDirectionUsesTheBeamAim(t *testing.T) {
	g := longBeam(0.5, 0, 100)
	g.P[pikachu].X, g.P[pikachu].Y = 600, 250 // on top of Snorlax, which faces left
	hitTicks(g, 3*tickRate)
	if p := g.P[pikachu]; p.X != 500 || p.Y != 250 {
		t.Fatalf("Pikachu at (%v,%v), want it pushed 100 along the aim to (500,250)", p.X, p.Y)
	}
}

// A target that stays in the beam is hit again every hit_interval, the first hit being at once.
func TestBeamHitsAgainEveryIntervalWhileTheTargetStaysIn(t *testing.T) {
	g := longBeam(2.0, 0.5, 0) // 60 ticks of beam, a hit every 15
	got := hitTicks(g, 4*tickRate)
	chargeEnd := chargeTicks(DefaultConfig()) + 1 // the sweep starts the tick after the charge ends
	if len(got) != 4 {
		t.Fatalf("hits on ticks %v, want 4 (every 0.5s of a 2s beam)", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i]-got[i-1] != 15 {
			t.Fatalf("hits on ticks %v, want them 15 ticks (0.5s) apart", got)
		}
	}
	if got[0] > chargeEnd+1 {
		t.Fatalf("first hit on tick %d, want it as soon as the beam is out (about tick %d)", got[0], chargeEnd)
	}
}

// The case that matters: each hit throws the target further out, and it is hit again only while it is still within the beam's reach.
func TestBeamRepeatHitsStopOnceKnockbackCarriesTheTargetOutOfReach(t *testing.T) {
	g := longBeam(3.0, 0.2, 100) // reach: 550 + Pikachu's body 16
	got := hitTicks(g, 5*tickRate)
	// Pikachu starts 300 away: hit at 300, 400, 500 (each inside 566), then it stands 600 away.
	if len(got) != 3 {
		t.Fatalf("hits on ticks %v, want 3", got)
	}
	if x := g.P[pikachu].X; x != 0 {
		t.Fatalf("Pikachu x = %v, want 0: three knockbacks of 100 from x=300", x)
	}
	if g.P[pikachu].HP != maxHP-30 {
		t.Fatalf("HP = %d, want %d", g.P[pikachu].HP, maxHP-30)
	}
}

func TestBeamIntervalZeroHitsOnlyOncePerSweep(t *testing.T) {
	g := longBeam(2.0, 0, 0)
	if got := hitTicks(g, 4*tickRate); len(got) != 1 {
		t.Fatalf("hits on ticks %v, want exactly one for a lingering target", got)
	}
}

// Every sweep starts afresh: the previous sweep's interval must not delay the first hit of the next.
func TestBeamIntervalDoesNotCarryOverToTheNextSweep(t *testing.T) {
	cfg := DefaultConfig()
	hb := &cfg.Snorlax.HyperBeam
	hb.SweepAngle, hb.SweepDuration, hb.HitInterval, hb.Cooldown, hb.RechargeDuration = 0, 0.1, 5, 0, 0
	hb.Damage = 10
	g := beamGame(cfg)
	if got := hitTicks(g, 2*tickRate); len(got) != 1 {
		t.Fatalf("first sweep hit on ticks %v, want 1", got)
	}
	hp := g.P[pikachu].HP
	stepBeam(g)
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].HP != hp-10 {
		t.Fatalf("second sweep: HP %d -> %d, want one more hit despite the 5s interval", hp, g.P[pikachu].HP)
	}
}

// ---- Pikachu: Electro Ball ----

// ballGame has Pikachu at (600,500) facing right and Snorlax at (1400,500), 800 apart.
func ballGame(cfg Config) *Game {
	g := NewGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = 600, 500
	g.P[snorlax].X, g.P[snorlax].Y = 1400, 500
	return g
}

func pressBall(g *Game) []Event {
	g.P[pikachu].In = Input{Ball: true}
	ev := g.Step(dt)
	g.P[pikachu].In = Input{}
	return ev
}

func explodeEvents(ev []Event) (n int, last Event) {
	for _, e := range ev {
		if e.T == "explode" {
			n, last = n+1, e
		}
	}
	return
}

func TestBallIsThrownWhenTheChargeEnds(t *testing.T) {
	g := ballGame(DefaultConfig()) // 0.5s = 15 ticks
	pressBall(g)                   // tick 1
	for tick := 2; tick <= 14; tick++ {
		g.Step(dt)
		if len(g.Balls) != 0 {
			t.Fatalf("a ball exists on tick %d, before the charge ended", tick)
		}
	}
	g.Step(dt) // tick 15
	if len(g.Balls) != 1 {
		t.Fatalf("balls after the charge = %d, want 1", len(g.Balls))
	}
	if b := g.Balls[0]; b.HP != DefaultConfig().Pikachu.ElectroBall.HP {
		t.Fatalf("ball HP = %d, want the configured %d", b.HP, DefaultConfig().Pikachu.ElectroBall.HP)
	}
}

func TestPikachuIsRootedWhileChargingTheBall(t *testing.T) {
	g := ballGame(DefaultConfig())
	x, y := g.P[pikachu].X, g.P[pikachu].Y
	for tick := 1; tick <= 15; tick++ { // up to and including the tick of the throw
		g.P[pikachu].In = Input{DX: -1, DY: 1, Skill: true, Ball: tick == 1}
		g.Step(dt)
		if g.P[pikachu].X != x || g.P[pikachu].Y != y {
			t.Fatalf("tick %d: Pikachu moved to (%v,%v) while charging", tick, g.P[pikachu].X, g.P[pikachu].Y)
		}
	}
	g.P[pikachu].In = Input{DX: -1}
	g.Step(dt)
	if g.P[pikachu].X >= x {
		t.Fatal("Pikachu must be free to move again once the ball is thrown")
	}
}

func TestBallFliesStraightAtTheConfiguredSpeedWhereTheAimPoints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.Speed = 300
	g := ballGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 600, 100 // straight above Pikachu
	g.P[pikachu].In = Input{Ball: true}
	g.Step(dt)
	g.P[pikachu].In = Input{}
	for i := 0; i < 14; i++ {
		g.Step(dt)
	}
	if len(g.Balls) != 1 {
		t.Fatal("setup: no ball")
	}
	b0 := g.Balls[0]
	if b0.X != 600 || b0.Y >= 500 {
		t.Fatalf("ball starts at (%v,%v), want it straight above Pikachu", b0.X, b0.Y)
	}
	g.P[pikachu].In = Input{}
	g.Step(dt)
	b1 := g.Balls[0]
	if b1.X != 600 || math.Abs((b0.Y-b1.Y)-300*dt) > 1e-9 {
		t.Fatalf("ball moved (%v,%v) -> (%v,%v), want 300*dt = %v straight up", b0.X, b0.Y, b1.X, b1.Y, 300*dt)
	}
}

func TestBallExplodesOnTheOpponentAndKnocksItAway(t *testing.T) {
	cfg := DefaultConfig()
	g := ballGame(cfg)
	pressBall(g)
	var boom Event
	var n int
	for tick := 2; tick <= 3*tickRate && n == 0; tick++ {
		n, boom = explodeEvents(g.Step(dt))
	}
	bc := cfg.Pikachu.ElectroBall
	if n != 1 || boom.R != bc.ExplosionRadius {
		t.Fatalf("explosions = %d with reach %v, want one with reach %v", n, boom.R, bc.ExplosionRadius)
	}
	if len(g.Balls) != 0 {
		t.Fatal("an exploded ball must be gone")
	}
	if g.P[snorlax].HP != maxHP-bc.ExplosionDamage {
		t.Fatalf("Snorlax HP = %d, want %d", g.P[snorlax].HP, maxHP-bc.ExplosionDamage)
	}
	// The ball touched Snorlax from the left, so the blast pushes it straight right by the knockback.
	if want := 1400 + bc.Knockback; g.P[snorlax].X != want || g.P[snorlax].Y != 500 {
		t.Fatalf("Snorlax at (%v,%v), want (%v,500)", g.P[snorlax].X, g.P[snorlax].Y, want)
	}
	if boom.X >= 1400 || boom.X < 1400-bc.Radius-cfg.Snorlax.Radius-bc.Speed*dt {
		t.Fatalf("it exploded at x=%v, want it where it met Snorlax's body", boom.X)
	}
}

func TestBallExplodesAtTheWall(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.P[snorlax].X, g.P[snorlax].Y = 600, 100 // straight above Pikachu, but in the air: the ball flies under it to the north wall
	g.P[pikachu].In = Input{Ball: true}
	g.Step(dt)
	g.P[pikachu].In = Input{}
	var boom Event
	var n int
	for tick := 2; tick <= 6*tickRate && n == 0; tick++ {
		g.P[snorlax].Z, g.P[snorlax].VZ = 100, 0 // held up
		n, boom = explodeEvents(g.Step(dt))
	}
	if n != 1 || boom.Y != 0 || len(g.Balls) != 0 {
		t.Fatalf("explosions = %d at y=%v with %d balls left, want one at the wall (y=0)", n, boom.Y, len(g.Balls))
	}
	if g.P[snorlax].HP != maxHP {
		t.Fatal("a blast at the far wall must not reach Snorlax")
	}
}

func TestBallWithNoHPLeftExplodesWhereItIs(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.Balls = []Ball{{X: 1300, Y: 500, DX: -1, HP: 0}} // 100 from Snorlax, inside the 90 + 24 reach
	n, boom := explodeEvents(g.Step(dt))
	if n != 1 || boom.X != 1300 {
		t.Fatalf("explosions = %d at x=%v, want one right where the ball hung", n, boom.X)
	}
	if g.P[snorlax].HP != maxHP-DefaultConfig().Pikachu.ElectroBall.ExplosionDamage {
		t.Fatalf("Snorlax HP = %d, the blast should have reached it", g.P[snorlax].HP)
	}
}

func TestSnorlaxSkillsTakeHPOffTheBall(t *testing.T) {
	t.Run("a slam", func(t *testing.T) {
		g := ballGame(DefaultConfig())
		g.P[snorlax].X, g.P[snorlax].Y = 1000, 500
		g.Balls = []Ball{{X: 1000, Y: 560, DX: 0, DY: 1, HP: 30}}
		g.P[snorlax].Z, g.P[snorlax].VZ = 1, -100 // lands on the next step
		g.Step(dt)
		if len(g.Balls) != 1 || g.Balls[0].HP != 30-slamDamage {
			t.Fatalf("balls = %+v, want one left with %d HP", g.Balls, 30-slamDamage)
		}
	})
	t.Run("a beam, once per sweep", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Snorlax.HyperBeam.SweepAngle, cfg.Snorlax.HyperBeam.SweepDuration = 0, 1
		cfg.Snorlax.HyperBeam.Damage = 12
		g := ballGame(cfg)
		g.P[snorlax].X, g.P[snorlax].Y = 1400, 500
		g.P[pikachu].X, g.P[pikachu].Y = 1000, 500 // the beam aims at Pikachu, along the line the ball sits on
		g.Balls = []Ball{{X: 1200, Y: 500, DX: 0, DY: 0, HP: 30}}
		cfg.Pikachu.ElectroBall.Speed = 0
		stepBeam(g)
		for i := 0; i < 3*tickRate && len(g.Balls) == 1; i++ {
			g.P[pikachu].X, g.P[pikachu].Y = 1000, 500
			g.Step(dt)
		}
		if len(g.Balls) != 1 || g.Balls[0].HP != 18 {
			t.Fatalf("balls = %+v, want one left with 18 HP: a 1s sweep must hit it once, not every tick", g.Balls)
		}
	})
	t.Run("a beam that finishes it", func(t *testing.T) {
		g := ballGame(DefaultConfig())
		g.P[pikachu].X, g.P[pikachu].Y = 1000, 500
		g.Balls = []Ball{{X: 1200, Y: 500, HP: 30}}
		stepBeam(g)
		var blasts int
		for i := 0; i < 3*tickRate; i++ {
			n, _ := explodeEvents(g.Step(dt))
			blasts += n
		}
		if blasts != 1 || len(g.Balls) != 0 {
			t.Fatalf("blasts = %d, balls = %d: a 40-damage beam must shoot down a 30 HP ball", blasts, len(g.Balls))
		}
	})
}

func TestBlastReachIsItsRadiusPlusTheTargetsBody(t *testing.T) {
	cfg := DefaultConfig() // reach 90, Snorlax body 24
	for _, tc := range []struct {
		gap float64
		hit bool
	}{{113, true}, {115, false}} {
		g := ballGame(cfg)
		g.P[snorlax].X, g.P[snorlax].Y = 1000+tc.gap, 500
		g.Balls = []Ball{{X: 1000, Y: 500, HP: 0}}
		g.Step(dt)
		if got := g.P[snorlax].HP < maxHP; got != tc.hit {
			t.Fatalf("gap %v: hit = %v, want %v", tc.gap, got, tc.hit)
		}
	}
}

func TestPikachuIsImmuneToItsOwnBlast(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.Balls = []Ball{{X: g.P[pikachu].X, Y: g.P[pikachu].Y, HP: 0}}
	g.Step(dt)
	if g.P[pikachu].HP != maxHP {
		t.Fatalf("Pikachu HP = %d, its own ball must not hurt it", g.P[pikachu].HP)
	}
}

// Like the other ground attacks, the ball flies under an airborne opponent and its blast misses one.
func TestBallPassesUnderAnAirborneOpponent(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.P[snorlax].Z, g.P[snorlax].VZ = 100, 0
	g.Balls = []Ball{{X: 1300, Y: 500, DX: 1, HP: 30}}
	for i := 0; i < 10; i++ {
		g.P[snorlax].Z = 100 // hold it up
		if n, _ := explodeEvents(g.Step(dt)); n != 0 {
			t.Fatal("the ball exploded on an opponent that was in the air")
		}
	}
	if g.P[snorlax].HP != maxHP {
		t.Fatal("an airborne opponent took damage from the ball")
	}
}

func TestFastBallCannotJumpOverTheOpponentBetweenTwoTicks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.Speed = 30000 // 1000 units a tick
	g := ballGame(cfg)
	g.Balls = []Ball{{X: 700, Y: 500, DX: 1, HP: 30}}
	n, _ := explodeEvents(g.Step(dt))
	if n != 1 || g.P[snorlax].HP == maxHP {
		t.Fatalf("explosions = %d, Snorlax HP = %d: a ball crossing Snorlax within one tick must still hit it", n, g.P[snorlax].HP)
	}
}

func TestBallCooldownCountsFromTheStartOfTheCharge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.Cooldown = 2 // 60 ticks from the press
	g := ballGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 1900, 900
	var thrownOn []int
	g.P[pikachu].In = Input{Ball: true} // held the whole time
	prev := 0
	for tick := 1; tick <= 130; tick++ {
		g.Step(dt)
		if len(g.Balls) > prev {
			thrownOn = append(thrownOn, tick)
		}
		prev = len(g.Balls)
	}
	// press on tick 1, thrown on tick 15; the next press is allowed on tick 61, thrown on tick 75
	if len(thrownOn) < 2 || thrownOn[0] != 15 || thrownOn[1] != 75 {
		t.Fatalf("balls thrown on ticks %v, want 15 then 75", thrownOn)
	}
}

func TestBallAndThunderAreOneAtATime(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.P[pikachu].In = Input{Thunder: true}
	g.Step(dt)
	g.P[pikachu].In = Input{Ball: true}
	for i := 0; i < 20; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].BallT != 0 || len(g.Balls) != 0 {
		t.Fatal("a ball started charging in the middle of a Thunder cast")
	}

	g = ballGame(DefaultConfig())
	pressBall(g)
	g.P[pikachu].In = Input{Thunder: true}
	g.Step(dt)
	if g.P[pikachu].ThunderLeft != 0 {
		t.Fatal("Thunder started in the middle of a ball charge")
	}
}

func TestOnlyPikachuThrowsBalls(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.P[snorlax].In = Input{Ball: true}
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if len(g.Balls) != 0 {
		t.Fatal("Snorlax threw an Electro Ball")
	}
}

func TestBallSnapshotKeys(t *testing.T) {
	b, _ := json.Marshal(Ball{X: 1, Y: 2, HP: 3})
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"x", "y", "hp"} {
		if _, ok := m[k]; !ok {
			t.Errorf("ball is missing %q: %s", k, b)
		}
	}
}

// ---- Hyper Beam against an Electro Ball: repeated hits, pushed back by the difference in mass ----

// beamVsBall: Snorlax (600,250) fires a fixed beam at Pikachu (300,250); the ball hangs on the line.
func beamVsBall(seconds, interval, beamMass, ballMass, ballSpeed float64, ball Ball) *Game {
	cfg := DefaultConfig()
	hb := &cfg.Snorlax.HyperBeam
	hb.SweepAngle, hb.SweepDuration, hb.HitInterval, hb.Mass = 0, seconds, interval, beamMass
	hb.Damage, hb.Length = 10, 550
	cfg.Pikachu.ElectroBall.Mass, cfg.Pikachu.ElectroBall.Speed = ballMass, ballSpeed
	g := beamGame(cfg)
	if ball.HP == 0 {
		ball.HP = 100000
	}
	g.Balls = []Ball{ball}
	return g
}

func TestBeamPushesABallBackByTheDifferenceInMass(t *testing.T) {
	g := beamVsBall(0.5, 0, 130, 100, 0, Ball{X: 450, Y: 250})
	stepBeam(g)
	for i := 0; i < 3*tickRate && g.Balls[0].HP == 100000; i++ {
		g.Step(dt)
	}
	if b := g.Balls[0]; b.HP != 100000-10 || b.X != 420 || b.Y != 250 {
		t.Fatalf("ball %+v, want HP %d and pushed 130-100 = 30 away from Snorlax to (420,250)", b, 100000-10)
	}
}

func TestBeamPushesTheBallAgainEveryHitInterval(t *testing.T) {
	g := beamVsBall(2.0, 0.5, 130, 100, 0, Ball{X: 450, Y: 250}) // 60 ticks of beam: a hit every 15
	stepBeam(g)
	var pushedOn []int
	prev := g.Balls[0].X
	for tick := 2; tick <= 4*tickRate; tick++ {
		g.Step(dt)
		if g.Balls[0].X != prev {
			pushedOn = append(pushedOn, tick)
		}
		prev = g.Balls[0].X
	}
	if len(pushedOn) != 4 {
		t.Fatalf("pushed on ticks %v, want 4 (every 0.5s of a 2s beam)", pushedOn)
	}
	for i := 1; i < len(pushedOn); i++ {
		if pushedOn[i]-pushedOn[i-1] != 15 {
			t.Fatalf("pushed on ticks %v, want them 15 ticks (0.5s) apart", pushedOn)
		}
	}
	if x := g.Balls[0].X; x != 450-4*30 {
		t.Fatalf("ball x = %v, want 4 pushes of 30 from 450", x)
	}
	if hp := g.Balls[0].HP; hp != 100000-40 {
		t.Fatalf("ball HP = %d, want one 10 damage per hit", hp)
	}
}

// The point of the design: each hit throws the ball back a little, but it still flies on, so while
// the push per hit (here 10) stays under what it flies between hits (150 * 0.2 = 30) it gets ahead.
func TestBallKeepsAdvancingThroughTheBeamWhileBeingPushedBack(t *testing.T) {
	g := beamVsBall(3.0, 0.2, 110, 100, 150, Ball{X: 100, Y: 250, DX: 1})
	stepBeam(g)
	const ticks = 54 // 18 of charge, then 1.2s of beam
	for i := 1; i < ticks; i++ {
		g.Step(dt)
	}
	b := g.Balls[0]
	hits := (100000 - b.HP) / 10
	if hits < 5 {
		t.Fatalf("hits = %d, want the beam to have hit the ball every 0.2s", hits)
	}
	if want := 100 + 150*dt*ticks - 10*float64(hits); math.Abs(b.X-want) > 1e-6 {
		t.Fatalf("ball x = %v, want %v: flight minus %d pushes of 10", b.X, want, hits)
	}
	if b.X <= 100 {
		t.Fatalf("ball x = %v: it must still have moved forward overall", b.X)
	}
}

func TestBeamDoesNotPushABallThatIsAsHeavyOrHeavier(t *testing.T) {
	for _, tc := range []struct{ beam, ball float64 }{{100, 150}, {100, 100}, {0, 0}} {
		g := beamVsBall(0.5, 0, tc.beam, tc.ball, 0, Ball{X: 450, Y: 250})
		stepBeam(g)
		for i := 0; i < 2*tickRate; i++ {
			g.Step(dt)
		}
		if b := g.Balls[0]; b.X != 450 || b.HP != 100000-10 {
			t.Fatalf("beam %v ball %v: ball %+v, want it hit (HP -10) but not moved", tc.beam, tc.ball, b)
		}
	}
}

func TestBallIntervalZeroIsOneHitAndOnePushPerSweep(t *testing.T) {
	g := beamVsBall(2.0, 0, 130, 100, 0, Ball{X: 450, Y: 250})
	stepBeam(g)
	for i := 0; i < 4*tickRate; i++ {
		g.Step(dt)
	}
	if b := g.Balls[0]; b.HP != 100000-10 || b.X != 420 {
		t.Fatalf("ball %+v, want a single hit and a single push for a lingering ball", b)
	}
}

func TestBallPushedIntoTheWallBlowsUp(t *testing.T) {
	g := beamVsBall(0.5, 0, 130, 100, 0, Ball{X: 20, Y: 250})
	g.cfg.Snorlax.HyperBeam.Length = 700 // reach the far end of the court
	stepBeam(g)
	var boom Event
	var n int
	for i := 0; i < 2*tickRate && n == 0; i++ {
		n, boom = explodeEvents(g.Step(dt))
	}
	if n != 1 || boom.X != 0 || len(g.Balls) != 0 {
		t.Fatalf("explosions = %d at x=%v, balls left %d: pushed against the wall it should explode there", n, boom.X, len(g.Balls))
	}
}

// A new sweep hits at once: the previous sweep's interval must not hold the first hit back.
func TestBallIntervalDoesNotCarryOverToTheNextSweep(t *testing.T) {
	g := beamVsBall(0.1, 5, 130, 100, 0, Ball{X: 450, Y: 250})
	hb := &g.cfg.Snorlax.HyperBeam
	hb.Cooldown, hb.RechargeDuration = 0, 0
	for sweep := 1; sweep <= 2; sweep++ {
		stepBeam(g)
		for i := 0; i < 2*tickRate; i++ {
			g.Step(dt)
		}
		if want := 100000 - 10*sweep; g.Balls[0].HP != want {
			t.Fatalf("after sweep %d the ball has %d HP, want %d: each sweep hits it once, despite the 5s interval", sweep, g.Balls[0].HP, want)
		}
	}
}

// The tick is 1/30 s, so a hit_interval that is not a whole number of ticks must still give the right
// number of hits over time: 0.05s used to become 0.067s (every 2nd tick) and 0.08s 0.1s (every 3rd).
func TestHitIntervalIsAnAverageNotRoundedUpToATick(t *testing.T) {
	const sweep = 3.0 // seconds of beam, 90 ticks
	for _, interval := range []float64{0.05, 0.08, 0.1, 0.25} {
		want := sweep / interval
		t.Run(fmt.Sprint(interval), func(t *testing.T) {
			g := longBeam(sweep, interval, 0)
			g.cfg.Snorlax.HyperBeam.Damage = 1
			if got := float64(len(hitTicks(g, 5*tickRate))); math.Abs(got-want) > 1.5 {
				t.Fatalf("a player in a %vs beam was hit %v times, want about %v (every %vs)", sweep, got, want, interval)
			}
			b := beamVsBall(sweep, interval, 0, 0, 0, Ball{X: 450, Y: 250})
			stepBeam(b)
			for i := 0; i < 5*tickRate; i++ {
				b.Step(dt)
			}
			if got := float64(100000-b.Balls[0].HP) / 10; math.Abs(got-want) > 1.5 {
				t.Fatalf("a ball in a %vs beam was hit %v times, want about %v (every %vs)", sweep, got, want, interval)
			}
		})
	}
}

// Below one tick there is nothing finer to give: one hit per tick, never more.
func TestHitIntervalBelowOneTickMeansOneHitPerTick(t *testing.T) {
	g := longBeam(1.0, 0.01, 0) // 30 ticks of beam
	g.cfg.Snorlax.HyperBeam.Damage = 1
	got := hitTicks(g, 4*tickRate)
	if len(got) < 29 || len(got) > 30 {
		t.Fatalf("hits = %d, want one per tick of the 30-tick beam", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i]-got[i-1] != 1 {
			t.Fatalf("hits on ticks %v, want every tick", got)
		}
	}
}

// ---- Snorlax: Ice Punch ----

// punchGame has Snorlax at (1000,500) facing left and Pikachu at (800,500), 200 away.
func punchGame(cfg Config) *Game {
	g := NewGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 1000, 500
	g.P[pikachu].X, g.P[pikachu].Y = 800, 500
	return g
}

// pressPunch holds the punch key for one tick with the given direction held, then lets go.
func pressPunch(g *Game, dx, dy int) {
	g.P[snorlax].In = Input{Punch: true, DX: dx, DY: dy}
	g.Step(dt)
	g.P[snorlax].In = Input{DX: dx, DY: dy}
}

func TestIcePunchChargesThenDashesTheConfiguredDistanceAtTheConfiguredSpeed(t *testing.T) {
	cfg := DefaultConfig() // charge 0.4s (12 ticks), 250 at 900/s (30 a tick), recharge 0.6s
	g := punchGame(cfg)
	g.P[pikachu].X = 100 // out of the way
	pressPunch(g, -1, 0) // tick 1
	var moved []int
	prev := g.P[snorlax].X
	for tick := 2; tick <= 80; tick++ {
		g.Step(dt)
		if g.P[snorlax].X != prev {
			moved = append(moved, tick)
		}
		prev = g.P[snorlax].X
	}
	// the charge runs ticks 1..12, so the dash covers ticks 13..21: eight steps of 30 and a last one of 10
	// (the keys are still held when the recharge ends, which is why it walks again from tick 39)
	if len(moved) < 9 || moved[0] != 13 || moved[8] != 21 || (len(moved) > 9 && moved[9] != 39) {
		t.Fatalf("Snorlax moved on ticks %v, want the dash on 13..21 and nothing until the recharge ends", moved)
	}
	g2 := punchGame(DefaultConfig())
	g2.P[pikachu].X = 100
	pressPunch(g2, -1, 0)
	g2.P[snorlax].In = Input{}
	for tick := 2; tick <= 40; tick++ {
		g2.Step(dt)
	}
	if x := g2.P[snorlax].X; math.Abs(x-750) > 1e-9 {
		t.Fatalf("Snorlax ended at x=%v, want 750 (250 to the left)", x)
	}
}

func TestIcePunchRootsSnorlaxUntilTheRechargeEnds(t *testing.T) {
	g := punchGame(DefaultConfig())
	g.P[pikachu].X = 100
	g.P[snorlax].In = Input{Punch: true, DX: -1}
	g.Step(dt)                                               // tick 1
	g.P[snorlax].In = Input{DX: -1, Skill: true, Beam: true} // the same direction, so the aim does not change
	// the dash covers ticks 13..21 and the recharge (18 ticks) ends on tick 39
	for tick := 2; tick <= 38; tick++ {
		x := g.P[snorlax].X
		g.Step(dt)
		if g.P[snorlax].Z != 0 || g.P[snorlax].BeamT != 0 || (tick > 21 && g.P[snorlax].X != x) || g.P[snorlax].Y != 500 {
			t.Fatalf("tick %d: Snorlax z=%v beamT=%v x %v -> %v y=%v: it must not jump, fire or walk during the punch", tick, g.P[snorlax].Z, g.P[snorlax].BeamT, x, g.P[snorlax].X, g.P[snorlax].Y)
		}
	}
	g.P[snorlax].In = Input{DX: 1}
	x := g.P[snorlax].X
	g.Step(dt) // tick 39
	if g.P[snorlax].X <= x {
		t.Fatal("Snorlax must walk again once the recharge is over")
	}
}

func TestIcePunchGoesWhereTheKeysPointWhenTheChargeEnds(t *testing.T) {
	g := punchGame(DefaultConfig())
	g.P[pikachu].X, g.P[pikachu].Y = 100, 100
	pressPunch(g, -1, 0)
	g.P[snorlax].In = Input{DY: 1} // turn to face down while charging
	for i := 0; i < 11; i++ {
		g.Step(dt)
	}
	x0, y0 := g.P[snorlax].X, g.P[snorlax].Y
	for i := 0; i < 12; i++ {
		g.Step(dt)
	}
	if g.P[snorlax].X != x0 || math.Abs(g.P[snorlax].Y-(y0+250)) > 1e-9 {
		t.Fatalf("Snorlax went (%v,%v) -> (%v,%v), want straight down 250", x0, y0, g.P[snorlax].X, g.P[snorlax].Y)
	}
}

func TestIcePunchFreezesWhatItTouchesOnceAndHurtsIt(t *testing.T) {
	cfg := DefaultConfig()
	g := punchGame(cfg)
	pressPunch(g, -1, 0)
	var freezes int
	for tick := 2; tick <= 40; tick++ {
		for _, e := range g.Step(dt) {
			if e.T == "freeze" {
				freezes++
			}
		}
	}
	ip := cfg.Snorlax.IcePunch
	if freezes != 1 {
		t.Fatalf("freeze events = %d, want exactly one for one punch", freezes)
	}
	if g.P[pikachu].HP != maxHP-ip.Damage {
		t.Fatalf("Pikachu HP = %d, want %d: one hit", g.P[pikachu].HP, maxHP-ip.Damage)
	}
	if g.P[pikachu].FrozenT <= 0 || g.P[pikachu].FrozenT > ip.FreezeDuration {
		t.Fatalf("Pikachu frozen for %v more seconds, want some of the %v", g.P[pikachu].FrozenT, ip.FreezeDuration)
	}
}

func TestFrozenPikachuCannotActAndMovesAgainOnTheTickItThaws(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.IcePunch.FreezeDuration = 1.0 // 30 ticks
	g := punchGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = 800, 500
	pressPunch(g, -1, 0)
	frozenAt := 0
	for tick := 2; tick <= 80 && frozenAt == 0; tick++ {
		g.Step(dt)
		if g.P[pikachu].FrozenT > 0 {
			frozenAt = tick
		}
	}
	if frozenAt == 0 {
		t.Fatal("setup: Pikachu was never frozen")
	}
	g.P[pikachu].In = Input{DX: -1, Skill: true, Thunder: true, Ball: true} // everything held down from now on
	x := g.P[pikachu].X
	for i := 0; i < 29; i++ { // the 30 frozen ticks, the first of them being the one that froze it
		g.Step(dt)
		if g.P[pikachu].X != x || g.P[pikachu].ThunderLeft != 0 || g.P[pikachu].BallT != 0 || g.P[pikachu].DashT != 0 {
			t.Fatalf("frozen tick %d: Pikachu x=%v thunder=%d ball=%v dash=%v, want it unable to act", i+1, g.P[pikachu].X, g.P[pikachu].ThunderLeft, g.P[pikachu].BallT, g.P[pikachu].DashT)
		}
	}
	g.Step(dt) // the 30th frozen tick
	g.Step(dt) // the thaw: the keys still held take effect again without being pressed anew
	if g.P[pikachu].ThunderLeft == 0 {
		t.Fatal("Pikachu must start the Thunder it still holds the key for once it thaws")
	}
}

func TestFreezeCutsAChargeAndADashShort(t *testing.T) {
	g := punchGame(DefaultConfig())
	g.P[pikachu].BallT, g.P[pikachu].DashT = 0.3, 0.1
	g.P[pikachu].FrozenT = 1
	g.Step(dt)
	if g.P[pikachu].BallT != 0 || g.P[pikachu].DashT != 0 {
		t.Fatalf("ball charge %v, dash %v: freezing must stop both", g.P[pikachu].BallT, g.P[pikachu].DashT)
	}
}

func TestIcePunchReachIsItsRadiusPlusTheBodyAlongTheWholePath(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float64 // where Pikachu stands; the dash runs from (1000,500) to (750,500)
		hit  bool
	}{
		{"on the path", 850, 500, true},
		{"beside the path, inside radius 40 + body 16", 850, 500 + 55, true},
		{"beside the path, just outside", 850, 500 + 57, false},
		{"beyond the end of the dash, inside the reach", 750 - 55, 500, true},
		{"beyond the end of the dash, outside", 750 - 57, 500, false},
		{"behind Snorlax", 1100, 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := punchGame(DefaultConfig())
			g.P[pikachu].X, g.P[pikachu].Y = tc.x, tc.y
			pressPunch(g, -1, 0)
			for i := 0; i < 40; i++ {
				g.Step(dt)
			}
			if got := g.P[pikachu].FrozenT > 0 || g.P[pikachu].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v, want %v", got, tc.hit)
			}
		})
	}
}

func TestFastFistCannotJumpOverABodyBetweenTwoTicks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.IcePunch.Speed, cfg.Snorlax.IcePunch.Distance = 30000, 400 // the whole dash in one tick
	g := punchGame(cfg)
	pressPunch(g, -1, 0)
	for i := 0; i < 30; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].FrozenT <= 0 {
		t.Fatal("a fist crossing Pikachu within a single tick must still freeze it")
	}
}

func TestIcePunchMissesAnAirborneTarget(t *testing.T) {
	g := punchGame(DefaultConfig())
	pressPunch(g, -1, 0)
	for i := 0; i < 40; i++ {
		g.P[pikachu].Z = 100
		g.Step(dt)
	}
	if g.P[pikachu].FrozenT > 0 || g.P[pikachu].HP != maxHP {
		t.Fatal("the fist is a ground attack and must pass under an airborne target")
	}
}

func TestIcePunchPushesABallAlongTheDashByTheMassDifference(t *testing.T) {
	cfg := DefaultConfig() // punch mass 150, ball mass 0 by default: set it
	cfg.Pikachu.ElectroBall.Mass, cfg.Pikachu.ElectroBall.Speed = 100, 0
	g := punchGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = 100, 100
	g.Balls = []Ball{{X: 900, Y: 545, HP: 30}} // beside the path: inside the fist's 40 + 14, outside Snorlax's own body 24 + 14
	pressPunch(g, -1, 0)
	for i := 0; i < 60; i++ {
		g.Step(dt)
	}
	if len(g.Balls) != 1 {
		t.Fatalf("balls = %d, want the ball hit once and left (HP 30 - 10) but still alive", len(g.Balls))
	}
	if b := g.Balls[0]; b.HP != 30-cfg.Snorlax.IcePunch.Damage || b.X != 850 || b.Y != 545 {
		t.Fatalf("ball %+v, want HP %d and pushed 150-100 = 50 along the dash to (850,545)", b, 30-cfg.Snorlax.IcePunch.Damage)
	}
}

func TestIcePunchCooldownCountsFromTheStartOfTheCharge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.IcePunch.Cooldown = 2 // 60 ticks
	g := punchGame(cfg)
	g.P[pikachu].X, g.P[pikachu].Y = 100, 100
	g.P[snorlax].In = Input{Punch: true} // held the whole time
	var started []int
	prevID := 0
	for tick := 1; tick <= 100; tick++ {
		g.Step(dt)
		if g.P[snorlax].PunchID > prevID {
			started = append(started, tick)
		}
		prevID = g.P[snorlax].PunchID
	}
	// press on tick 1, the dash starts on tick 12 (the charge ends); the next press is allowed on tick 61, dash on tick 72
	if len(started) != 2 || started[0] != 12 || started[1] != 72 {
		t.Fatalf("dashes started on ticks %v, want 12 and 72", started)
	}
}

func TestIcePunchAndHyperBeamAreOneAtATimeAndNotInTheAir(t *testing.T) {
	g := punchGame(DefaultConfig())
	g.P[snorlax].In = Input{Beam: true}
	g.Step(dt)
	g.P[snorlax].In = Input{Punch: true}
	g.Step(dt)
	if g.P[snorlax].PunchT != 0 {
		t.Fatal("a punch started in the middle of a beam")
	}

	g = punchGame(DefaultConfig())
	pressPunch(g, -1, 0)
	g.P[snorlax].In = Input{Beam: true}
	g.Step(dt)
	if g.P[snorlax].BeamT != 0 {
		t.Fatal("a beam started in the middle of a punch")
	}

	g = punchGame(DefaultConfig())
	g.P[snorlax].Z, g.P[snorlax].VZ = 50, 0
	g.P[snorlax].In = Input{Punch: true}
	g.Step(dt)
	if g.P[snorlax].PunchT != 0 {
		t.Fatal("a punch started in the air")
	}
}

func TestOnlySnorlaxThrowsIcePunches(t *testing.T) {
	g := punchGame(DefaultConfig())
	g.P[pikachu].In = Input{Punch: true}
	for i := 0; i < 2*tickRate; i++ {
		g.Step(dt)
	}
	if g.P[pikachu].PunchT != 0 || g.P[pikachu].PunchID != 0 {
		t.Fatal("Pikachu threw an Ice Punch")
	}
}

// ---- aiming: the stick gives any angle, the keyboard 8 directions; walking stays 8-way ----

func deg(a float64) (float64, float64) {
	return math.Cos(a * math.Pi / 180), math.Sin(a * math.Pi / 180)
}

// throwBall has Pikachu charge a ball with the given input held, Snorlax standing at (sx, sy) but in the air,
// so the ball cannot stop on it, and returns the ball.
func throwBall(t *testing.T, in Input, sx, sy float64) Ball {
	t.Helper()
	g := ballGame(DefaultConfig())
	g.P[snorlax].X, g.P[snorlax].Y = sx, sy
	in.Ball = true
	g.P[pikachu].In = in
	for i := 0; i < 15; i++ {
		g.P[snorlax].Z, g.P[snorlax].VZ = 100, 0
		g.Step(dt)
	}
	if len(g.Balls) != 1 {
		t.Fatalf("balls = %d, want one after the charge", len(g.Balls))
	}
	return g.Balls[0]
}

// The ball goes at the opponent, whatever the stick or the keys say and wherever the opponent stands.
func TestBallGoesAtTheOpponentWhateverTheStickSays(t *testing.T) {
	for _, a := range []float64{0, 11.25, 67.5, 100, 200, 337.5} {
		ox, oy := deg(a)
		for _, in := range []Input{{}, {AimX: -ox, AimY: -oy}, {DX: -1, DY: 1}, {AimX: 1000, AimY: 3}} {
			b := throwBall(t, in, 600+ox*300, 500+oy*300) // Pikachu is at (600,500)
			if math.Abs(b.DX-ox) > 1e-9 || math.Abs(b.DY-oy) > 1e-9 || math.Abs(math.Hypot(b.DX, b.DY)-1) > 1e-9 {
				t.Fatalf("opponent at %v degrees, input %+v: ball direction (%.4f,%.4f), want (%.4f,%.4f)", a, in, b.DX, b.DY, ox, oy)
			}
		}
	}
}

// The aim keeps following the opponent through the charge, so the ball goes where it is when the charge ends.
func TestBallAimFollowsTheOpponentThroughTheCharge(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.P[snorlax].X, g.P[snorlax].Y = 1000, 500
	g.P[pikachu].In = Input{Ball: true}
	g.Step(dt)
	g.P[pikachu].In = Input{}
	g.P[snorlax].X, g.P[snorlax].Y = 600, 900 // it runs to straight below
	for i := 0; i < 14; i++ {
		g.P[snorlax].Z, g.P[snorlax].VZ = 100, 0
		g.Step(dt)
	}
	if len(g.Balls) != 1 || g.Balls[0].DX != 0 || g.Balls[0].DY != 1 {
		t.Fatalf("balls %+v, want one flying straight down at where Snorlax went", g.Balls)
	}
	if g.P[pikachu].AimY != 1 {
		t.Fatalf("aim (%v,%v) at the throw, want the dashed aim line on the opponent too", g.P[pikachu].AimX, g.P[pikachu].AimY)
	}
}

// Standing exactly on the opponent there is no direction: the ball goes along the aim it already had.
func TestBallOnTopOfTheOpponentKeepsTheOldAim(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.P[snorlax].X, g.P[snorlax].Y = g.P[pikachu].X, g.P[pikachu].Y
	g.P[pikachu].AimX, g.P[pikachu].AimY = 0, -1
	g.P[pikachu].In = Input{Ball: true}
	for i := 0; i < 15; i++ {
		g.P[snorlax].Z, g.P[snorlax].VZ = 100, 0
		g.Step(dt)
	}
	if len(g.Balls) != 1 || math.IsNaN(g.Balls[0].DX) || g.Balls[0].DY != -1 {
		t.Fatalf("balls %+v, want one along the old aim (0,-1), no NaN", g.Balls)
	}
}

// A ball belongs to whoever threw it, not to a slot: Pikachu in slot 1 still hurts slot 0, and not itself.
func TestBallTargetsTheOtherPlayerWhicheverSlotThrowsIt(t *testing.T) {
	g := NewGame(DefaultConfig())
	g.SetKinds(snorlax, pikachu)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 500, 500, 500
	g.P[1].In = Input{Ball: true}
	for i := 0; i < 60 && g.P[0].HP == g.cfg.Snorlax.MaxHP; i++ {
		g.Step(dt)
	}
	if g.P[0].HP >= g.cfg.Snorlax.MaxHP {
		t.Fatal("the ball never hurt the opponent in slot 0")
	}
	if g.P[1].HP != g.cfg.Pikachu.MaxHP {
		t.Fatalf("Pikachu hurt itself: HP %d", g.P[1].HP)
	}
}

func TestIcePunchDashesAlongTheStickAtAnyAngle(t *testing.T) {
	for _, a := range []float64{22.5, 67.5, 135 + 11.25} {
		g := punchGame(DefaultConfig())
		g.P[pikachu].X, g.P[pikachu].Y = 100, 100
		x0, y0 := g.P[snorlax].X, g.P[snorlax].Y
		ax, ay := deg(a)
		g.P[snorlax].In = Input{Punch: true, AimX: ax, AimY: ay}
		for i := 0; i < 40; i++ {
			g.Step(dt)
		}
		dx, dy := g.P[snorlax].X-x0, g.P[snorlax].Y-y0
		if math.Abs(dx-250*ax) > 1e-6 || math.Abs(dy-250*ay) > 1e-6 {
			t.Fatalf("aim %v degrees: dashed (%.3f,%.3f), want 250 along (%.4f,%.4f)", a, dx, dy, ax, ay)
		}
	}
}

// Only the aim is analog: the stick's exact angle must not change which way a character walks.
func TestWalkingStaysEightWayWhateverTheStickAim(t *testing.T) {
	g := newTestGame()
	x, y := g.P[pikachu].X, g.P[pikachu].Y
	ax, ay := deg(22.5)
	g.P[pikachu].In = Input{DX: 1, AimX: ax, AimY: ay}
	g.Step(dt)
	if g.P[pikachu].Y != y || math.Abs(g.P[pikachu].X-x-g.cfg.Pikachu.WalkSpeed*dt) > 1e-9 {
		t.Fatalf("Pikachu moved (%v,%v) -> (%v,%v), want it to walk straight along x", x, y, g.P[pikachu].X, g.P[pikachu].Y)
	}
}

func TestInputCarriesTheStickAimOverTheWire(t *testing.T) {
	var in Input
	if err := json.Unmarshal([]byte(`{"dx":1,"ax":0.38,"ay":-0.92}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.AimX != 0.38 || in.AimY != -0.92 {
		t.Fatalf("aim = (%v,%v), want (0.38,-0.92)", in.AimX, in.AimY)
	}
}
