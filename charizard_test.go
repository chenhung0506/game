package main

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"testing"
)

// czGame has Charizard in slot 0 at (300,250) facing right and Snorlax in slot 1 at (900,250), well apart.
// The fire's spread comes from a fixed generator, so a run is the same every time.
func czGame(cfg Config) *Game { return duel(cfg, charizard, snorlax) }

// duel is the same set-up for any two characters (SetKinds starts the round over, so it comes first).
func duel(cfg Config, a, b int) *Game {
	g := NewGame(cfg)
	g.SetKinds(a, b)
	g.rng = rand.New(rand.NewPCG(1, 2))
	g.P[0].X, g.P[0].Y = 300, 250
	g.P[1].X, g.P[1].Y = 900, 250
	return g
}

func steps(g *Game, n int) {
	for i := 0; i < n; i++ {
		g.Step(dt)
	}
}

func fires(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballFire {
			n++
		}
	}
	return n
}

// ---- the character a slot plays ----

func TestSetKindsGivesEachSlotItsCharactersHP(t *testing.T) {
	cfg := DefaultConfig()
	g := NewGame(cfg)
	if g.P[0].Kind != pikachu || g.P[1].Kind != snorlax {
		t.Fatalf("NewGame kinds = %d, %d, want pikachu in slot 0 and snorlax in slot 1", g.P[0].Kind, g.P[1].Kind)
	}
	g.SetKinds(charizard, pikachu)
	if g.P[0].HP != cfg.Charizard.MaxHP || g.P[1].HP != cfg.Pikachu.MaxHP {
		t.Fatalf("hp = %d, %d, want %d, %d", g.P[0].HP, g.P[1].HP, cfg.Charizard.MaxHP, cfg.Pikachu.MaxHP)
	}
	g.P[0].HP = 1
	g.Reset() // the next round keeps the characters
	if g.P[0].Kind != charizard || g.P[1].Kind != pikachu || g.P[0].HP != cfg.Charizard.MaxHP {
		t.Fatalf("after Reset: kinds %d, %d hp %d, want the same characters at full HP", g.P[0].Kind, g.P[1].Kind, g.P[0].HP)
	}
}

// A character's skills follow what it is, not which slot it is in.
func TestSkillsFollowTheCharacterNotTheSlot(t *testing.T) {
	g := NewGame(DefaultConfig())
	g.SetKinds(snorlax, pikachu) // swapped
	g.P[0].In.Skill = true       // Snorlax's jump
	g.P[1].In.Skill = true       // Pikachu's dash
	g.Step(dt)
	if g.P[0].Z <= 0 || g.P[1].DashT <= 0 {
		t.Fatalf("slot 0 Z=%v slot 1 DashT=%v, want Snorlax jumping from slot 0 and Pikachu dashing from slot 1", g.P[0].Z, g.P[1].DashT)
	}
}

func TestCharizardSnapshotKeys(t *testing.T) {
	b, _ := json.Marshal(Player{})
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"k", "ft", "fcd", "ht", "hcd", "gp", "gdx", "gdy", "gcd", "hd"} {
		if _, ok := m[k]; !ok {
			t.Errorf("snapshot is missing %q: %s", k, b)
		}
	}
	b, _ = json.Marshal(Ball{K: ballFire})
	json.Unmarshal(b, &m)
	if m["k"] != float64(ballFire) {
		t.Errorf("a fire ball's snapshot must carry its kind: %s", b)
	}
}

// ---- Fly ----

func TestFlyLastsItsDurationThenTheCooldownStartsOnLanding(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.Fly.Duration, cfg.Charizard.Fly.Cooldown, cfg.Charizard.Fly.Height = 2, 3, 90
	g := czGame(cfg)
	g.P[0].In.Skill = true
	g.Step(dt)
	g.P[0].In.Skill = false
	if g.P[0].FlyT <= 0 || g.P[0].Z <= 0 {
		t.Fatalf("FlyT=%v Z=%v after the press, want it airborne", g.P[0].FlyT, g.P[0].Z)
	}
	steps(g, 15) // 0.5 s: the climb takes flyRiseTime
	if math.Abs(g.P[0].Z-90) > 1e-6 {
		t.Fatalf("Z = %v while flying, want the configured height 90", g.P[0].Z)
	}
	if g.P[0].FlyCD != 0 {
		t.Fatalf("FlyCD = %v while flying, want the cooldown to wait for the landing", g.P[0].FlyCD)
	}
	steps(g, 45) // 2 s of flight in all
	if g.P[0].FlyT != 0 {
		t.Fatalf("FlyT = %v after the duration, want 0", g.P[0].FlyT)
	}
	steps(g, 20) // it comes down
	if g.P[0].Z != 0 || g.P[0].FlyCD <= 0 || g.P[0].FlyCD > 3 {
		t.Fatalf("Z=%v FlyCD=%v after landing, want it on the ground with the cooldown running", g.P[0].Z, g.P[0].FlyCD)
	}
	g.P[0].In.Skill = true
	g.Step(dt)
	if g.P[0].FlyT != 0 {
		t.Fatal("flew again during the cooldown")
	}
	steps(g, 100) // 3.3 s: the cooldown is over
	g.Step(dt)
	if g.P[0].FlyT <= 0 {
		t.Fatal("could not fly after the cooldown")
	}
}

func TestFlyingMovesAtTheFlySpeed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.WalkSpeed, cfg.Charizard.Fly.Speed = 100, 300
	run := func(fly bool) float64 {
		g := czGame(cfg)
		g.P[0].In.Skill = fly
		g.Step(dt)
		g.P[0].In = Input{DX: 1}
		x0 := g.P[0].X
		steps(g, 30)
		return g.P[0].X - x0
	}
	if w, f := run(false), run(true); math.Abs(w-100) > 4 || math.Abs(f-300) > 4 {
		t.Fatalf("moved %v walking and %v flying in a second, want about 100 and 300", w, f)
	}
}

// With their bands raised to cover the flight height, the Electro Ball and the Ice Punch reach a flying
// Charizard; the ground attacks (default band 0..groundZ) still miss it.
func TestFlyingCharizardIsHitByBallAndPunchButNotByGroundAttacks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.MaxHeight, cfg.Snorlax.IcePunch.MaxHeight = 100, 100 // above fly.height (90)
	fly := func() *Game {
		g := duel(cfg, charizard, pikachu)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 500, 250, 300, 250
		g.P[0].In.Skill = true
		g.Step(dt)
		g.P[0].In = Input{}
		steps(g, 10)
		if g.P[0].Z < groundZ {
			t.Fatal("setup: Charizard should be well in the air")
		}
		return g
	}
	t.Run("an Electro Ball hits it", func(t *testing.T) {
		g := fly()
		g.Balls = append(g.Balls, Ball{Owner: 1, X: 420, Y: 250, DX: 1, HP: 30})
		steps(g, 20)
		if g.P[0].HP >= DefaultConfig().Charizard.MaxHP {
			t.Fatal("the ball flew under the flying Charizard")
		}
	})
	t.Run("an Ice Punch freezes it", func(t *testing.T) {
		g := duel(cfg, snorlax, charizard)
		g.P[0].X, g.P[1].X = 300, 450
		g.P[1].In.Skill = true
		g.Step(dt)
		g.P[1].In = Input{}
		steps(g, 10)
		g.P[0].AimX, g.P[0].AimY = 1, 0
		g.P[0].In.Punch = true
		steps(g, 30)
		if g.P[1].FrozenT <= 0 {
			t.Fatal("the punch went under the flying Charizard")
		}
	})
	t.Run("the landing shock of Snorlax misses it", func(t *testing.T) {
		g := duel(DefaultConfig(), snorlax, charizard)
		g.P[0].X, g.P[1].X = 300, 330
		g.P[1].In.Skill = true
		g.Step(dt)
		g.P[1].In = Input{}
		steps(g, 10)
		g.P[0].Z, g.P[0].VZ = 1, -100 // Snorlax lands on the next step
		g.Step(dt)
		if g.P[1].HP != DefaultConfig().Charizard.MaxHP {
			t.Fatalf("HP = %d, want the shock to pass under the flier", g.P[1].HP)
		}
	})
}

// ---- Fire Breath ----

// slowFire keeps the fire balls near where they were breathed, so a test can count and measure them.
func slowFire() Config {
	cfg := DefaultConfig()
	cfg.Charizard.FireBreath.Speed, cfg.Charizard.FireBreath.Length = 1, 1e6
	return cfg
}

func TestFireBreathBreathesTheQuantityEvenlyOverTheDuration(t *testing.T) {
	cfg := slowFire()
	cfg.Charizard.FireBreath.Duration, cfg.Charizard.FireBreath.Quantity = 4, 40
	g := czGame(cfg)
	g.P[0].In.Fire = true
	g.Step(dt)
	g.P[0].In.Fire = false
	if n := fires(g); n > 1 {
		t.Fatalf("%d fire balls on the first tick, want it to start gently", n)
	}
	steps(g, 59) // 2 s in
	if n := fires(g); n < 19 || n > 21 {
		t.Fatalf("%d fire balls after half the duration, want about half of 40", n)
	}
	steps(g, 60) // 4 s: the last tick of the breath
	if n := fires(g); n != 40 {
		t.Fatalf("%d fire balls after the full duration, want exactly the quantity 40", n)
	}
	steps(g, 60)
	if n := fires(g); n != 40 || g.P[0].BreathT != 0 {
		t.Fatalf("%d fire balls and BreathT=%v after the breath, want it over with nothing more breathed", n, g.P[0].BreathT)
	}
}

func TestFireBreathCooldownStartsWithTheBreath(t *testing.T) {
	cfg := slowFire()
	cfg.Charizard.FireBreath.Duration, cfg.Charizard.FireBreath.Quantity, cfg.Charizard.FireBreath.Cooldown = 1, 10, 3
	g := czGame(cfg)
	g.P[0].In.Fire = true
	steps(g, 60) // held down through the breath and 1 s more: the cooldown (3 s, from the start) is still running
	if g.P[0].BreathT != 0 || g.P[0].BreathCD <= 0 {
		t.Fatalf("BreathT=%v BreathCD=%v, want the breath over and the cooldown running", g.P[0].BreathT, g.P[0].BreathCD)
	}
	if n := fires(g); n != 10 {
		t.Fatalf("%d fire balls, want 10: no second breath during the cooldown", n)
	}
	steps(g, 40) // 3.3 s since the start
	if g.P[0].BreathT <= 0 {
		t.Fatal("no new breath after the cooldown although the key is still held")
	}
}

func TestFireBallsAreAimedAtTheOpponentWithinTheRandomOffset(t *testing.T) {
	for _, off := range []float64{0, 5, 10} {
		cfg := slowFire()
		cfg.Charizard.FireBreath.Quantity, cfg.Charizard.FireBreath.Duration, cfg.Charizard.FireBreath.RandomOffsetAngle = 60, 3, off
		g := czGame(cfg)
		g.P[1].X, g.P[1].Y = 900, 550 // the opponent is not straight ahead
		want := math.Atan2(300, 600)
		g.P[0].In.Fire = true
		steps(g, 91)
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, b := range g.Balls {
			d := math.Remainder(math.Atan2(b.DY, b.DX)-want, 2*math.Pi) * 180 / math.Pi
			lo, hi = math.Min(lo, d), math.Max(hi, d)
			if math.Abs(math.Hypot(b.DX, b.DY)-1) > 1e-9 {
				t.Fatalf("direction %v,%v is not a unit vector", b.DX, b.DY)
			}
		}
		if len(g.Balls) != 60 {
			t.Fatalf("offset %v: %d fire balls, want 60", off, len(g.Balls))
		}
		if off == 0 && (hi > 1e-6 || lo < -1e-6) {
			t.Fatalf("offset 0: angles %v..%v degrees, want every ball dead straight", lo, hi)
		}
		// 60 balls spread both ways, using most of the allowed range and never beyond it
		if off > 0 && (hi > off || lo < -off || hi < 0.6*off || lo > -0.6*off) {
			t.Fatalf("offset %v: angles %v..%v degrees, want both sides used and none beyond +-%v", off, lo, hi, off)
		}
	}
}

func TestFireBallsFollowWhereTheOpponentIsNow(t *testing.T) {
	cfg := slowFire()
	cfg.Charizard.FireBreath.RandomOffsetAngle = 0
	g := czGame(cfg)
	g.P[0].In.Fire = true
	steps(g, 10)
	first := g.Balls[0]
	g.P[1].X, g.P[1].Y = 300, 1000 // the opponent runs to straight below
	steps(g, 30)
	last := g.Balls[len(g.Balls)-1]
	if first.DX < 0.99 || last.DY < 0.99 {
		t.Fatalf("first ball direction %v,%v and last %v,%v, want right at first, then down toward the opponent", first.DX, first.DY, last.DX, last.DY)
	}
}

func TestAFireBallHurtsOnceAndIsSpent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.FireBreath.RandomOffsetAngle = 0
	g := czGame(cfg)
	g.P[1].X = 600
	g.P[0].In.Fire = true
	steps(g, 160) // the whole breath and the time for the last ball to arrive
	if want := cfg.Snorlax.MaxHP - 40*cfg.Charizard.FireBreath.Damage; g.P[1].HP != want {
		t.Fatalf("Snorlax HP = %d, want %d: every one of the 40 balls lands once for %d", g.P[1].HP, want, cfg.Charizard.FireBreath.Damage)
	}
	if fires(g) != 0 {
		t.Fatalf("%d fire balls left, want them spent on the hit", fires(g))
	}
}

// Fire is breathed from above: an opponent in the air is hit all the same.
func TestFireHitsAnAirborneOpponent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.FireBreath.RandomOffsetAngle = 0
	g := czGame(cfg)
	g.P[1].X, g.P[1].Z, g.P[1].VZ = 600, 100, 0
	g.P[1].Kind = snorlax
	g.P[0].In.Fire = true
	for i := 0; i < 30; i++ {
		g.P[1].Z = 100 // held up, so the physics does not bring it down
		g.Step(dt)
	}
	if g.P[1].HP == cfg.Snorlax.MaxHP {
		t.Fatal("the fire flew under an airborne opponent")
	}
}

func TestAFireBallEndsAtItsLengthAndAtWalls(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.FireBreath.Length, cfg.Charizard.FireBreath.Speed, cfg.Charizard.FireBreath.RandomOffsetAngle = 100, 300, 0
	g := czGame(cfg)
	g.P[1].Y = 900 // out of the way: the breath is aimed at it, so aim by hand
	g.Balls = []Ball{{K: ballFire, Owner: 0, Left: 100, X: 500, Y: 250, DX: 1, HP: 10}}
	x := 500.0
	for i := 0; i < 15 && len(g.Balls) > 0; i++ {
		g.Step(dt)
		if len(g.Balls) > 0 {
			x = g.Balls[0].X
		}
	}
	if len(g.Balls) != 0 || math.Abs(x-590) > 11 {
		t.Fatalf("after 15 ticks: %d balls, last x %v, want it gone after 100 units (x 600)", len(g.Balls), x)
	}
	g.Balls = []Ball{{K: ballFire, Owner: 0, Left: 5000, X: g.court.Length - 5, Y: 250, DX: 1, HP: 10}}
	g.Step(dt)
	if len(g.Balls) != 0 {
		t.Fatal("the fire ball went through the wall")
	}
}

func TestFireBallsLoseHPToTheOpponentsSkills(t *testing.T) {
	cfg := DefaultConfig()
	t.Run("Hyper Beam", func(t *testing.T) {
		g := duel(cfg, snorlax, charizard)
		g.P[0].X, g.P[1].X = 200, 900
		g.Balls = []Ball{{K: ballFire, Owner: 1, Left: 500, X: 400, Y: 250, DX: -1, HP: 100}}
		g.P[0].In.Beam = true
		steps(g, 40)
		if len(g.Balls) == 1 && g.Balls[0].HP >= 100 {
			t.Fatal("the beam did not hurt the fire ball")
		}
	})
	t.Run("Snorlax landing shock", func(t *testing.T) {
		g := duel(cfg, snorlax, charizard)
		g.P[0].X, g.P[1].X = 300, 900
		g.P[0].Z, g.P[0].VZ = 1, -100
		g.Balls = []Ball{{K: ballFire, Owner: 1, Left: 500, X: 370, Y: 250, DX: -1, HP: 100}} // 70 away: in the shock, not yet on Snorlax
		g.Step(dt)
		if g.Balls[0].HP != 100-slamDamage {
			t.Fatalf("fire HP = %d, want %d", g.Balls[0].HP, 100-slamDamage)
		}
	})
	t.Run("Ice Punch", func(t *testing.T) {
		g := duel(cfg, snorlax, charizard)
		g.P[0].X, g.P[1].X, g.P[1].Y = 300, 900, 1000
		g.Balls = []Ball{{K: ballFire, Owner: 1, Left: 500, X: 400, Y: 250, DX: -1, HP: 100}}
		g.P[0].AimX, g.P[0].AimY = 1, 0
		g.P[0].In.Punch = true
		steps(g, 30)
		if len(g.Balls) == 1 && g.Balls[0].HP != 100-cfg.Snorlax.IcePunch.Damage {
			t.Fatalf("fire HP = %d, want %d (one punch, one hit)", g.Balls[0].HP, 100-cfg.Snorlax.IcePunch.Damage)
		}
	})
	t.Run("Electro Ball blast", func(t *testing.T) {
		g := duel(cfg, pikachu, charizard)
		g.P[0].X, g.P[1].X = 300, 1500
		g.Balls = []Ball{
			{Owner: 0, X: 600, Y: 250, DX: 1, HP: 0}, // shot down: blows up where it is
			{K: ballFire, Owner: 1, Left: 500, X: 650, Y: 250, DX: -1, HP: 100},
		}
		g.Step(dt)
		var fire *Ball
		for i := range g.Balls {
			if g.Balls[i].K == ballFire {
				fire = &g.Balls[i]
			}
		}
		if fire == nil || fire.HP != 100-cfg.Pikachu.ElectroBall.ExplosionDamage {
			t.Fatalf("fire = %+v, want it to lose the blast's %d HP", fire, cfg.Pikachu.ElectroBall.ExplosionDamage)
		}
	})
}

func TestFireBallsAreDeadAtZeroHP(t *testing.T) {
	g := czGame(DefaultConfig())
	g.Balls = []Ball{{K: ballFire, Owner: 0, Left: 500, X: 400, Y: 250, DX: 1, HP: 0}}
	g.Step(dt)
	if len(g.Balls) != 0 {
		t.Fatal("a fire ball at 0 HP kept flying")
	}
}

// A heavier beam or punch shoves a fire ball back by the difference in mass.
func TestFireBallMassDecidesHowFarTheBeamPushesIt(t *testing.T) {
	push := func(mass float64) float64 {
		cfg := DefaultConfig()
		cfg.Charizard.FireBreath.Mass, cfg.Snorlax.HyperBeam.Mass = mass, 130
		cfg.Charizard.FireBreath.Speed = 0 // it hangs in the air, so the beam is sure to find it
		g := duel(cfg, snorlax, charizard)
		g.P[0].X, g.P[1].X = 200, 1500
		g.Balls = []Ball{{K: ballFire, Owner: 1, Left: 5000, X: 400, Y: 250, DX: 0, DY: 1, HP: 1000}}
		g.P[0].In.Beam = true
		for i := 0; i < 40 && g.Balls[0].X == 400; i++ { // until the first hit
			g.Step(dt)
		}
		return g.Balls[0].X - 400
	}
	if a, b := push(30), push(100); math.Abs(a-100) > 1 || math.Abs(b-30) > 1 {
		t.Fatalf("pushed %v by mass 30 and %v by mass 100, want 100 and 30 (130 - mass)", a, b)
	}
	if c := push(200); c != 0 {
		t.Fatalf("pushed %v by a ball heavier than the beam, want 0", c)
	}
}

func TestCharizardCanWalkAndFlyWhileBreathing(t *testing.T) {
	g := czGame(slowFire())
	g.P[0].In = Input{Fire: true, DX: 1}
	x0 := g.P[0].X
	steps(g, 30)
	if g.P[0].X-x0 < 100 {
		t.Fatalf("moved %v while breathing, want it free to walk", g.P[0].X-x0)
	}
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	g.P[0].In = Input{DX: 1}
	if g.P[0].BreathT <= 0 || g.P[0].FlyT <= 0 {
		t.Fatalf("BreathT=%v FlyT=%v, want flying to start without stopping the breath", g.P[0].BreathT, g.P[0].FlyT)
	}
}

func TestFrozenCharizardStopsBreathing(t *testing.T) {
	g := czGame(slowFire())
	g.P[0].In.Fire = true
	steps(g, 20)
	g.P[0].FrozenT = 1
	g.Step(dt)
	n := fires(g)
	steps(g, 10)
	if g.P[0].BreathT != 0 || fires(g) != n {
		t.Fatalf("BreathT=%v, %d -> %d fire balls: a frozen Charizard must stop breathing", g.P[0].BreathT, n, fires(g))
	}
}

// ---- Seismic Toss ----

// tossGame has Charizard facing right with Snorlax right ahead, inside reach.
func tossGame(cfg Config) *Game {
	g := czGame(cfg)
	g.P[1].X = 450
	g.P[0].AimX, g.P[0].AimY = 1, 0
	return g
}

func startToss(g *Game) {
	g.P[0].In.Toss = true
	g.Step(dt)
	g.P[0].In.Toss = false
}

func TestSeismicTossRunsItsLengthAtItsSpeedAndMissesAFarOpponent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.SeismicToss.Length, cfg.Charizard.SeismicToss.Speed = 280, 700
	g := czGame(cfg) // the opponent is 600 away: out of reach
	g.P[0].AimX, g.P[0].AimY = 1, 0
	startToss(g)
	if g.P[0].TossPhase != tossDash {
		t.Fatalf("phase %d after the press, want the dash", g.P[0].TossPhase)
	}
	steps(g, 30)
	if math.Abs(g.P[0].X-580) > 1e-6 || g.P[0].TossPhase != tossNone || g.P[1].Held {
		t.Fatalf("X=%v phase=%d held=%v, want it to stop at 300+280 with nobody caught", g.P[0].X, g.P[0].TossPhase, g.P[1].Held)
	}
	if g.P[0].TossCD <= 0 {
		t.Fatal("a miss still costs the cooldown")
	}
	startToss(g)
	if g.P[0].TossPhase != tossNone {
		t.Fatal("tossed again during the cooldown")
	}
}

func TestSeismicTossCarriesTheOpponentUpAndSlamsItDown(t *testing.T) {
	cfg := DefaultConfig()
	ts := &cfg.Charizard.SeismicToss
	ts.Speed, ts.High, ts.Damage, ts.Radius = 600, 180, 35, 45
	g := tossGame(cfg)
	startToss(g)
	var victim *Player = &g.P[1]
	for i := 0; i < 40 && !victim.Held; i++ {
		g.Step(dt)
	}
	if !victim.Held || g.P[0].TossPhase != tossRise {
		t.Fatalf("held=%v phase=%d, want the opponent caught", victim.Held, g.P[0].TossPhase)
	}
	hp := victim.HP
	// While carried the opponent obeys no key, and goes up with Charizard.
	victim.In = Input{DX: 1, DY: 1}
	x := victim.X
	maxZ := 0.0
	var slam *Event
	for i := 0; i < 60 && g.P[0].TossPhase != tossNone; i++ {
		for _, e := range g.Step(dt) {
			if e.T == "slam" {
				e := e
				slam = &e
			}
		}
		if victim.Held {
			if math.Abs(victim.Z-g.P[0].Z) > 1e-9 {
				t.Fatalf("carried opponent at Z %v, Charizard at %v", victim.Z, g.P[0].Z)
			}
			if victim.HP != hp {
				t.Fatal("damaged before the landing")
			}
		}
		maxZ = math.Max(maxZ, g.P[0].Z)
	}
	if math.Abs(maxZ-180) > 1e-6 {
		t.Fatalf("highest point %v, want the configured high 180", maxZ)
	}
	if victim.HP != hp-35 || victim.Held || victim.Z != 0 || g.P[0].Z != 0 {
		t.Fatalf("HP %d (want %d) held=%v Z=%v/%v, want a landing for exactly 35 damage, both on the ground", victim.HP, hp-35, victim.Held, victim.Z, g.P[0].Z)
	}
	if slam == nil || slam.R != 45 {
		t.Fatalf("slam event %+v, want one of the configured radius 45 (it is Snorlax's landing effect)", slam)
	}
	if victim.X != x && math.Abs(victim.X-x) > 200 {
		t.Fatalf("the victim travelled to %v from %v while held", victim.X, x)
	}
	// ...and the held keys act again once it is free.
	x = victim.X
	g.Step(dt)
	if victim.X == x {
		t.Fatal("the opponent stayed deaf to its keys after landing (keys were still held)")
	}
}

func TestSeismicTossTakesTimeFromSpeedAndHigh(t *testing.T) {
	ticks := func(speed, high float64) int {
		cfg := DefaultConfig()
		cfg.Charizard.SeismicToss.Speed, cfg.Charizard.SeismicToss.High = speed, high
		g := tossGame(cfg)
		startToss(g)
		n := 0
		for g.P[0].TossPhase != tossNone && n < 1000 {
			g.Step(dt)
			n++
		}
		return n
	}
	// the run takes the same, but a higher throw takes longer: up and down at the same speed
	if a, b := ticks(600, 120), ticks(600, 360); b-a < 15 {
		t.Fatalf("%d ticks for high 120 and %d for high 360, want the higher throw to take longer", a, b)
	}
	if a, b := ticks(300, 200), ticks(900, 200); a <= b {
		t.Fatalf("%d ticks at speed 300 and %d at 900, want the faster toss to be quicker", a, b)
	}
}

func TestSeismicTossGoesAlongTheAimAndStopsAtWalls(t *testing.T) {
	cfg := DefaultConfig()
	g := czGame(cfg)
	g.P[1].X, g.P[1].Y = 1500, 900
	g.P[0].X, g.P[0].Y = 300, 250
	g.P[0].AimX, g.P[0].AimY = 0, -1 // up the court, into the wall
	startToss(g)
	steps(g, 30)
	if g.P[0].Y != 0 || g.P[0].X != 300 {
		t.Fatalf("at %v,%v, want it to run straight up to the wall and stop there", g.P[0].X, g.P[0].Y)
	}
}

func TestSeismicTossMissesAJumpingOpponent(t *testing.T) {
	g := tossGame(DefaultConfig())
	g.P[1].Z, g.P[1].VZ = 60, 0 // in the air
	startToss(g)
	for i := 0; i < 30; i++ {
		g.P[1].Z = 60
		g.Step(dt)
	}
	if g.P[1].Held || g.P[1].HP != DefaultConfig().Snorlax.MaxHP {
		t.Fatal("a jumping opponent was caught by a ground run")
	}
}

// Charizard may toss from the air: the flight ends there, the run goes on at that height, the catch is
// carried up and slammed, and the flight cooldown starts once Charizard is back on the ground.
func TestSeismicTossWorksWhileFlying(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.Fly.Height = 30
	g := tossGame(cfg)
	g.P[0].In.Skill = true
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 10)
	if g.P[0].Z != 30 || g.P[0].FlyT <= 0 {
		t.Fatalf("setup: Z = %v, FlyT = %v, want it flying", g.P[0].Z, g.P[0].FlyT)
	}
	startToss(g)
	if g.P[0].TossPhase != tossDash || g.P[0].FlyT != 0 {
		t.Fatalf("phase %d, FlyT %v: the toss should have started and ended the flight", g.P[0].TossPhase, g.P[0].FlyT)
	}
	if g.P[0].Z != 30 {
		t.Fatalf("Z = %v, the run goes on at the flying height", g.P[0].Z)
	}
	caught := false
	for i := 0; i < 400 && g.P[0].TossPhase != tossNone; i++ {
		g.Step(dt)
		caught = caught || g.P[1].Held
	}
	if !caught || g.P[1].HP != cfg.Snorlax.MaxHP-cfg.Charizard.SeismicToss.Damage {
		t.Fatalf("caught = %v, HP = %d: the opponent should have been caught and slammed", caught, g.P[1].HP)
	}
	if g.P[0].Z != 0 || g.P[0].FlyCD != cfg.Charizard.Fly.Cooldown {
		t.Fatalf("Z = %v, FlyCD = %v, want it landed with the flight cooldown running", g.P[0].Z, g.P[0].FlyCD)
	}
}

// A toss from the air that catches nobody: Charizard comes down by itself and the cooldown starts on landing.
func TestMissedTossFromTheAirComesDown(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.SeismicToss.Length = 50 // far short of the opponent
	g := tossGame(cfg)
	g.P[1].X = 900
	g.P[0].In.Skill = true
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 10)
	startToss(g)
	steps(g, 60)
	if g.P[0].Z != 0 || g.P[0].FlyCD <= 0 || g.P[0].FlyT != 0 {
		t.Fatalf("Z = %v, FlyCD = %v, FlyT = %v: want it on the ground with the cooldown running", g.P[0].Z, g.P[0].FlyCD, g.P[0].FlyT)
	}
}

func TestSeismicTossRootsCharizardAndStopsOtherSkills(t *testing.T) {
	g := tossGame(DefaultConfig())
	startToss(g)
	g.P[0].In = Input{DY: 1, Fire: true, Skill: true} // walking, breathing and flying are all refused
	steps(g, 5)
	if g.P[0].Y != 250 || g.P[0].BreathT != 0 || g.P[0].FlyT != 0 {
		t.Fatalf("Y=%v BreathT=%v FlyT=%v, want it committed to the run", g.P[0].Y, g.P[0].BreathT, g.P[0].FlyT)
	}
}

// If Charizard is frozen while carrying, it lets go: the opponent drops unhurt.
func TestFrozenCharizardDropsItsCatch(t *testing.T) {
	g := tossGame(DefaultConfig())
	startToss(g)
	for i := 0; i < 40 && !g.P[1].Held; i++ {
		g.Step(dt)
	}
	steps(g, 3)
	hp := g.P[1].HP
	g.P[0].FrozenT = 1
	g.Step(dt)
	if g.P[1].Held || g.P[1].Z != 0 || g.P[1].HP != hp || g.P[0].TossPhase != tossNone || g.P[0].Z != 0 {
		t.Fatalf("held=%v Z=%v HP=%d (was %d) phase=%d: want both dropped, nobody hurt", g.P[1].Held, g.P[1].Z, g.P[1].HP, hp, g.P[0].TossPhase)
	}
}

// A caught Pikachu's dash and charge are cut short like a frozen one's.
func TestCaughtPlayerLosesItsDashAndCharge(t *testing.T) {
	g := duel(DefaultConfig(), charizard, pikachu)
	g.P[1].X = 450
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.P[1].BallT, g.P[1].DashT = 0.4, 0.1
	g.P[1].Held = true
	g.P[0].TossPhase = tossRise
	g.Step(dt)
	if g.P[1].BallT != 0 || g.P[1].DashT != 0 {
		t.Fatalf("BallT=%v DashT=%v, want both cancelled while caught", g.P[1].BallT, g.P[1].DashT)
	}
}

// The run is all fire: a breath in progress stops when the toss begins.
func TestSeismicTossStopsTheBreath(t *testing.T) {
	g := tossGame(slowFire())
	g.P[0].In.Fire = true
	steps(g, 10)
	g.P[0].In = Input{Toss: true}
	g.Step(dt)
	g.P[0].In = Input{}
	n := fires(g)
	steps(g, 10)
	if g.P[0].BreathT != 0 || fires(g) != n {
		t.Fatalf("BreathT=%v, %d -> %d fire balls: the breath should stop when the toss starts", g.P[0].BreathT, n, fires(g))
	}
}

// Mirror matches: with the same character on both sides every skill still works from either slot, and
// nothing hurts the one who used it.
func TestMirrorMatchesRunEverySkillWithoutHurtingTheUser(t *testing.T) {
	for kind := 0; kind < kinds; kind++ {
		g := duel(DefaultConfig(), kind, kind)
		g.P[1].X = 500                                                                                                                                       // close enough for every skill to reach
		all := Input{Skill: true, Beam: true, Thunder: true, Ball: true, Punch: true, Fire: true, Toss: kind != charizard, Storm: true, Freeze: true, DX: 1} // a toss pre-empts Charizard's breath and flight, so it has its own tests
		for tick := 0; tick < 20*tickRate; tick++ {
			g.P[0].In, g.P[1].In = all, all
			g.P[tick%2].In.DX = -1 // they do not stand still
			g.Step(dt)
			for i := range g.P {
				p := g.P[i]
				if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsNaN(p.Z) || p.X < 0 || p.X > g.court.Length || p.Y < 0 || p.Y > g.court.Width {
					t.Fatalf("kind %d tick %d: player %d at %v,%v,%v", kind, tick, i, p.X, p.Y, p.Z)
				}
			}
			if g.Winner() >= 0 {
				break
			}
		}
		if kind == chikorita {
			continue // its hop is aimed on the first tick, so this setup parts them: it has its own mirror test
		}
		if g.P[0].HP == g.cfg.maxHPOf(kind) && g.P[1].HP == g.cfg.maxHPOf(kind) {
			t.Fatalf("kind %d: nobody was hurt in 20 s of everything at once", kind)
		}
	}
}

func TestMirrorFireAndBallsHurtOnlyTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.FireBreath.RandomOffsetAngle = 0
	g := duel(cfg, charizard, charizard)
	g.P[1].X = 700        // inside the breath's length
	g.P[0].In.Fire = true // only slot 0 breathes
	steps(g, 200)
	if g.P[0].HP != cfg.Charizard.MaxHP || g.P[1].HP == cfg.Charizard.MaxHP {
		t.Fatalf("HP %d / %d: the breath must hurt slot 1 only", g.P[0].HP, g.P[1].HP)
	}
	g = duel(cfg, pikachu, pikachu)
	g.P[1].In.Ball = true // only slot 1 throws
	steps(g, 120)
	if g.P[1].HP != cfg.Pikachu.MaxHP || g.P[0].HP == cfg.Pikachu.MaxHP {
		t.Fatalf("HP %d / %d: the ball must hurt slot 0 only", g.P[0].HP, g.P[1].HP)
	}
}
