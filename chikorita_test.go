package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// chGame has Chikorita in slot 0 at (300,250) and Snorlax in slot 1 at (900,250).
func chGame(cfg Config) *Game { return duel(cfg, chikorita, snorlax) }

func hopRight(g *Game) {
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	g.P[0].In = Input{}
}

// stepsUntil runs until the phase is p (or gives up) and returns the ticks it took.
func stepsUntil(g *Game, phase int) int {
	for n := 1; n <= 600; n++ {
		g.Step(dt)
		if g.P[0].LeafPhase == phase {
			return n
		}
	}
	return -1
}

func TestChikoritaHopsTheConfiguredDistanceAndHeightInTheWalkingDirection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.LeafSlash.JumpDistance, cfg.Chikorita.LeafSlash.JumpDuration, cfg.Chikorita.LeafSlash.JumpHeight = 200, 0.5, 60
	g := chGame(cfg)
	g.P[1].X, g.P[1].Y = 1900, 900 // far away: the slashes that follow do not matter here
	g.P[0].In = Input{Skill: true, DX: 1, DY: 1}
	g.Step(dt)
	g.P[0].In = Input{}
	peak := g.P[0].Z
	for g.P[0].LeafPhase == leafHop {
		g.Step(dt)
		peak = math.Max(peak, g.P[0].Z)
	}
	dx, dy := g.P[0].X-300, g.P[0].Y-250
	if math.Abs(dx-200/math.Sqrt2) > 0.5 || math.Abs(dy-200/math.Sqrt2) > 0.5 {
		t.Fatalf("hopped (%.2f, %.2f), want 200 along the held diagonal", dx, dy)
	}
	if g.P[0].Z != 0 || math.Abs(peak-60) > 3 {
		t.Fatalf("peak %.1f, Z after %.1f, want a hop that peaks near 60 and lands", peak, g.P[0].Z)
	}
}

func TestChikoritaHopWithNoKeysGoesTheWayItFaces(t *testing.T) {
	g := chGame(DefaultConfig()) // slot 0 faces right
	g.P[0].In.Skill = true
	g.Step(dt)
	g.P[0].In = Input{}
	stepsUntil(g, leafSlash)
	if g.P[0].X < 300+200 || math.Abs(g.P[0].Y-250) > 1e-6 {
		t.Fatalf("at %v,%v, want it hopped right", g.P[0].X, g.P[0].Y)
	}
}

func TestLandingFromTheHopHurtsNobody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.LeafSlash.Damage = 0 // the slashes after the hop are harmless, so any damage is the landing's
	g := chGame(cfg)
	g.P[1].X = 520 // where it lands, right on top of the opponent
	hopRight(g)
	steps(g, 40)
	if g.P[1].HP != cfg.Snorlax.MaxHP {
		t.Fatalf("HP = %d, the hop must not hurt", g.P[1].HP)
	}
}

func TestChikoritaSlashesTheConfiguredNumberOfTimesAndHitsOnceEach(t *testing.T) {
	for _, n := range []int{1, 3, 5} {
		cfg := DefaultConfig()
		cfg.Chikorita.LeafSlash.Slashes, cfg.Chikorita.LeafSlash.Damage = n, 7
		cfg.Chikorita.LeafSlash.SlashStep = 0 // stays put, so every swing reaches
		g := chGame(cfg)
		g.P[1].X = 300 + cfg.Chikorita.LeafSlash.JumpDistance + 60
		hopRight(g)
		steps(g, 200)
		if want := cfg.Snorlax.MaxHP - 7*n; g.P[1].HP != want {
			t.Errorf("%d slashes: HP = %d, want %d", n, g.P[1].HP, want)
		}
		if g.P[0].LeafPhase != leafNone {
			t.Errorf("%d slashes: still in phase %d", n, g.P[0].LeafPhase)
		}
	}
}

func TestSlashSwingsAlternateAndHitInTheMiddleOfEachSwing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.LeafSlash.SlashInterval, cfg.Chikorita.LeafSlash.SlashStep = 0.4, 0 // 12 ticks a swing
	g := chGame(cfg)
	g.P[1].X = 300 + cfg.Chikorita.LeafSlash.JumpDistance + 60
	hopRight(g)
	stepsUntil(g, leafSlash)
	var hits []int // tick (from the start of the slash) of each hit, and the swing it was in
	var swing []int
	hp := g.P[1].HP
	for tick := 1; tick <= 60 && g.P[0].LeafPhase == leafSlash; tick++ {
		g.Step(dt)
		if g.P[1].HP < hp {
			hits, swing = append(hits, tick), append(swing, g.P[0].LeafN)
			hp = g.P[1].HP
		}
	}
	if len(hits) != cfg.Chikorita.LeafSlash.Slashes {
		t.Fatalf("hits at ticks %v, want %d of them", hits, cfg.Chikorita.LeafSlash.Slashes)
	}
	for n, tk := range hits {
		if want := 12*n + 6; tk < want-1 || tk > want+1 || swing[n] != n {
			t.Errorf("hit %d at tick %d in swing %d, want about tick %d, in swing %d", n, tk, swing[n], want, n)
		}
	}
}

func TestEachSwingCarriesChikoritaTowardsTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.LeafSlash.Slashes, cfg.Chikorita.LeafSlash.SlashStep = 3, 50
	cfg.Chikorita.LeafSlash.SlashRadius = 1
	g := chGame(cfg)
	g.P[1].X, g.P[1].Y = 1500, 900 // up and to the right
	hopRight(g)
	stepsUntil(g, leafSlash)
	x0, y0 := g.P[0].X, g.P[0].Y
	dx, dy := g.P[1].X-x0, g.P[1].Y-y0
	n := math.Hypot(dx, dy)
	for g.P[0].LeafPhase == leafSlash {
		g.Step(dt)
	}
	moved := math.Hypot(g.P[0].X-x0, g.P[0].Y-y0)
	if math.Abs(moved-150) > 3 {
		t.Fatalf("moved %.1f over three swings, want 3 x 50", moved)
	}
	if cos := ((g.P[0].X-x0)*dx + (g.P[0].Y-y0)*dy) / (moved * n); cos < 0.99 {
		t.Fatalf("it did not go towards the opponent (cos %.3f)", cos)
	}
}

func TestSlashAdvanceStopsWhereTheBodiesTouch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.LeafSlash.SlashStep, cfg.Chikorita.LeafSlash.Slashes = 500, 4
	g := chGame(cfg)
	g.P[1].X = 300 + cfg.Chikorita.LeafSlash.JumpDistance + 100
	hopRight(g)
	steps(g, 100)
	if gap := g.P[1].X - g.P[0].X; gap < cfg.Chikorita.Radius+cfg.Snorlax.Radius-1e-6 {
		t.Fatalf("gap %.2f, it walked into the opponent", gap)
	}
}

func TestSlashRadiusDecidesWhoIsHit(t *testing.T) {
	for _, tc := range []struct {
		gap float64
		hit bool
	}{{80, true}, {115, false}} { // radius 90 plus Snorlax's body (24)
		cfg := DefaultConfig()
		cfg.Chikorita.LeafSlash.SlashStep = 0
		g := chGame(cfg)
		g.P[1].X = 300 + cfg.Chikorita.LeafSlash.JumpDistance + tc.gap
		hopRight(g)
		steps(g, 100)
		if got := g.P[1].HP < cfg.Snorlax.MaxHP; got != tc.hit {
			t.Errorf("opponent %v away: hit = %v, want %v", tc.gap, got, tc.hit)
		}
	}
}

func TestSlashHitsOnlyWithinItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Chikorita.LeafSlash.MaxHeight, cfg.Chikorita.LeafSlash.SlashStep = 50, 0
			g := chGame(cfg)
			g.P[1].X = 300 + cfg.Chikorita.LeafSlash.JumpDistance + 50
			hopRight(g)
			for i := 0; i < 100; i++ {
				g.P[1].Z, g.P[1].VZ = tc.z, 0
				g.Step(dt)
			}
			if got := g.P[1].HP < cfg.Snorlax.MaxHP; got != tc.hit {
				t.Fatalf("hit = %v at height %v, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

func TestLeafSlashCooldownCountsFromTheStartOfTheHop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.LeafSlash.Cooldown = 3
	g := chGame(cfg)
	g.P[1].X, g.P[1].Y = 1900, 900
	hopRight(g)
	if g.P[0].LeafCD < 2.9 || g.P[0].LeafPhase != leafHop {
		t.Fatalf("LeafCD = %v phase %d, want the cooldown running from the take-off", g.P[0].LeafCD, g.P[0].LeafPhase)
	}
	for g.P[0].LeafPhase != leafNone {
		g.Step(dt)
	}
	g.P[0].In.Skill = true
	g.Step(dt)
	if g.P[0].LeafPhase != leafNone {
		t.Fatal("used again while the cooldown was running")
	}
	g.P[0].In.Skill = false
	steps(g, 3*tickRate) // 3 s from the take-off, minus the hop and the slashes already gone
	hopRight(g)
	if g.P[0].LeafPhase != leafHop {
		t.Fatal("not ready again after the cooldown")
	}
}

func TestChikoritaIsRootedByItsOwnSkillButNotByTheHopDirection(t *testing.T) {
	g := chGame(DefaultConfig())
	g.P[1].X, g.P[1].Y = 1900, 900
	hopRight(g)
	x := g.P[0].X
	g.P[0].In = Input{DX: -1} // pushing the stick the other way mid-hop changes nothing
	g.Step(dt)
	if g.P[0].X <= x {
		t.Fatal("the hop's direction must be locked")
	}
}

func TestFrozenChikoritaDropsTheHopAndTheSlash(t *testing.T) {
	g := chGame(DefaultConfig())
	g.P[1].X, g.P[1].Y = 1900, 900
	hopRight(g)
	steps(g, 3)
	if g.P[0].Z <= 0 {
		t.Fatal("setup: should be in the air")
	}
	g.P[0].FrozenT = 1
	g.Step(dt)
	if g.P[0].Z != 0 || g.P[0].LeafPhase != leafNone {
		t.Fatalf("Z = %v phase %d, want it stopped on the ground", g.P[0].Z, g.P[0].LeafPhase)
	}
	g.P[0].FrozenT = 0
	hopRight(g) // cooldown is still running
	if g.P[0].LeafPhase != leafNone {
		t.Fatal("the cooldown should still hold")
	}
}

func TestChikoritaSnapshotCarriesTheLeafState(t *testing.T) {
	g := chGame(DefaultConfig())
	hopRight(g)
	b, _ := json.Marshal(g.P[0])
	for _, k := range []string{`"lp":1`, `"ln":0`, `"lk":`, `"lcd":`, `"k":3`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("snapshot %s lacks %s", b, k)
		}
	}
}

func TestChikoritaMirrorMatchSlashesOnlyTheOpponent(t *testing.T) {
	g := duel(DefaultConfig(), chikorita, chikorita)
	g.P[1].X = 520
	g.P[0].In = Input{Skill: true, DX: 1}
	g.P[1].In = Input{Skill: true, DX: -1}
	g.Step(dt)
	g.P[0].In, g.P[1].In = Input{}, Input{}
	steps(g, 120)
	if g.P[0].HP == g.cfg.Chikorita.MaxHP && g.P[1].HP == g.cfg.Chikorita.MaxHP {
		t.Fatal("nobody was slashed")
	}
	if g.P[0].HP <= 0 && g.P[1].HP <= 0 {
		t.Fatal("both died from one exchange")
	}
}

func TestChikoritaConfigLoadsAndRefusesBadValues(t *testing.T) {
	c, err := loadJSON(t, `{"chikorita":{"leaf_slash":{"slashes":6,"jump_distance":300,"slash_step":10,"slash_radius":120,"leaf_scale":4,"max_height":30}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if l := c.Chikorita.LeafSlash; l.Slashes != 6 || l.JumpDistance != 300 || l.SlashStep != 10 || l.SlashRadius != 120 || l.LeafScale != 4 || l.MaxHeight != 30 {
		t.Fatalf("loaded %+v", l)
	}
	for name, body := range map[string]string{
		"no slashes":      `{"chikorita":{"leaf_slash":{"slashes":0}}}`,
		"zero interval":   `{"chikorita":{"leaf_slash":{"slash_interval":0}}}`,
		"zero distance":   `{"chikorita":{"leaf_slash":{"jump_distance":0}}}`,
		"negative step":   `{"chikorita":{"leaf_slash":{"slash_step":-1}}}`,
		"zero radius":     `{"chikorita":{"leaf_slash":{"slash_radius":0}}}`,
		"band reversed":   `{"chikorita":{"leaf_slash":{"min_height":9,"max_height":3}}}`,
		"zero leaf scale": `{"chikorita":{"leaf_slash":{"leaf_scale":0}}}`,
		"zero body":       `{"chikorita":{"radius":0}}`,
		"unknown field":   `{"chikorita":{"leaf_slash":{"sweeps":3}}}`,
	} {
		if _, err := loadJSON(t, body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// ---- Razor Leaf ----

func leaves(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballLeaf {
			n++
		}
	}
	return
}

func castRazor(g *Game) {
	g.P[0].In = Input{Razor: true}
	g.Step(dt)
	g.P[0].In = Input{}
}

func TestRazorLeafScattersTheConfiguredLeavesAroundChikorita(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.RazorLeaf.Count, cfg.Chikorita.RazorLeaf.ScatterRadius = 30, 150
	cfg.Chikorita.RazorLeaf.MinDelay, cfg.Chikorita.RazorLeaf.MaxDelay = 5, 6 // none leaves during this test
	cfg.Chikorita.RazorLeaf.LeafHP = 7
	g := chGame(cfg)
	castRazor(g)
	if leaves(g) != 30 {
		t.Fatalf("%d leaves, want 30", leaves(g))
	}
	near, far := 1e9, 0.0
	for _, b := range g.Balls {
		d := math.Hypot(b.X-g.P[0].X, b.Y-g.P[0].Y)
		near, far = math.Min(near, d), math.Max(far, d)
		if d > 150+1e-6 || d < cfg.Chikorita.Radius || !b.Wait || b.HP != 7 || b.Delay < 5-1e-6 || b.Delay > 6 {
			t.Fatalf("leaf at distance %.1f, wait %v, hp %d, delay %.2f", d, b.Wait, b.HP, b.Delay)
		}
	}
	if far-near < 60 {
		t.Fatalf("distances only span %.1f..%.1f, want some near and some far", near, far)
	}
}

func TestRazorLeavesShootAtWhereTheOpponentStandsWhenTheirDelayIsUp(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.RazorLeaf.Count, cfg.Chikorita.RazorLeaf.Damage = 5, 3
	cfg.Chikorita.RazorLeaf.MinDelay, cfg.Chikorita.RazorLeaf.MaxDelay = 0.5, 0.5
	cfg.Chikorita.RazorLeaf.Speed, cfg.Chikorita.RazorLeaf.ScatterRadius = 800, 60
	g := chGame(cfg)
	g.P[1].Y = 250
	castRazor(g)
	steps(g, 10) // 0.33 s: still waiting, whatever happened to the opponent
	if g.P[1].HP != cfg.Snorlax.MaxHP {
		t.Fatal("a leaf struck before its delay was up")
	}
	for _, b := range g.Balls {
		if !b.Wait {
			t.Fatal("a leaf left early")
		}
	}
	g.P[1].X, g.P[1].Y = 900, 600 // moves before they pick their target
	steps(g, 60)
	if want := cfg.Snorlax.MaxHP - 5*3; g.P[1].HP != want || leaves(g) != 0 {
		t.Fatalf("HP = %d, leaves left %d, want %d and none: all five should have hit once", g.P[1].HP, leaves(g), want)
	}
}

func TestRazorLeafDelaysAreSpreadBetweenMinAndMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.RazorLeaf.Count = 60
	cfg.Chikorita.RazorLeaf.MinDelay, cfg.Chikorita.RazorLeaf.MaxDelay = 0.3, 1.2
	g := chGame(cfg)
	castRazor(g)
	lo, hi := 9.0, 0.0
	for _, b := range g.Balls {
		lo, hi = math.Min(lo, b.Delay), math.Max(hi, b.Delay)
	}
	if lo < 0.3-dt-1e-6 || hi > 1.2 || hi-lo < 0.5 {
		t.Fatalf("delays %.2f..%.2f, want them spread inside 0.3..1.2", lo, hi)
	}
}

func TestRazorLeafMissesAnOpponentOutsideItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Chikorita.RazorLeaf.Count, cfg.Chikorita.RazorLeaf.MaxHeight = 3, 50
			cfg.Chikorita.RazorLeaf.MinDelay, cfg.Chikorita.RazorLeaf.MaxDelay = 0.1, 0.1
			g := chGame(cfg)
			castRazor(g)
			for i := 0; i < 80; i++ {
				g.P[1].Z, g.P[1].VZ = tc.z, 0
				g.Step(dt)
			}
			if got := g.P[1].HP < cfg.Snorlax.MaxHP; got != tc.hit {
				t.Fatalf("hit = %v at height %v, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

func TestRazorLeavesLoseHPToTheOpponentsSkills(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.RazorLeaf.Count, cfg.Chikorita.RazorLeaf.LeafHP, cfg.Chikorita.RazorLeaf.ScatterRadius = 4, 5, 50
	cfg.Chikorita.RazorLeaf.MinDelay, cfg.Chikorita.RazorLeaf.MaxDelay = 5, 5
	g := chGame(cfg)
	castRazor(g)
	// an Electro Ball going off on top of them burns every leaf in its blast
	g.Balls = append(g.Balls, Ball{Owner: 1, X: g.P[0].X, Y: g.P[0].Y, DX: 1, HP: 0})
	steps(g, 3)
	if n := leaves(g); n != 0 {
		t.Fatalf("%d leaves left after a blast over all of them", n)
	}
}

func TestRazorLeafCooldownAndExclusion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.RazorLeaf.Count, cfg.Chikorita.RazorLeaf.Cooldown = 2, 4
	cfg.Chikorita.RazorLeaf.MinDelay, cfg.Chikorita.RazorLeaf.MaxDelay = 9, 9
	g := chGame(cfg)
	castRazor(g)
	castRazor(g)
	if leaves(g) != 2 {
		t.Fatalf("%d leaves, a second cast inside the cooldown must do nothing", leaves(g))
	}
	steps(g, 4*tickRate)
	castRazor(g)
	if leaves(g) != 4 {
		t.Fatalf("%d leaves, want 4 after the cooldown", leaves(g))
	}
	// not in mid-hop
	g = chGame(cfg)
	hopRight(g)
	castRazor(g)
	if leaves(g) != 0 {
		t.Fatal("scattered leaves in mid-air")
	}
}

// ---- Vine Whip ----

// vineFoe puts the opponent r away from Chikorita at bearing b degrees off its aim (to the right, counter-clockwise on screen is positive).
func vineFoe(g *Game, b, r float64) {
	rad := b * math.Pi / 180
	g.P[1].X, g.P[1].Y = g.P[0].X+r*math.Cos(rad), g.P[0].Y-r*math.Sin(rad)
}

func castVine(g *Game) {
	g.P[0].In = Input{Vine: true}
	g.Step(dt)
	g.P[0].In = Input{}
}

func TestVinesSwingInFromBothSidesAndMeetOnTheAim(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.VineWhip.SweepAngle, cfg.Chikorita.VineWhip.SweepDuration = 70, 0.5
	g := chGame(cfg)
	g.P[1].X, g.P[1].Y = 1900, 900
	castVine(g)
	if a := g.P[0].VineA; a < 60 || a > 70 || g.P[0].VineT <= 0 {
		t.Fatalf("VineA = %v, VineT = %v right after the cast, want the vines near 70 degrees", a, g.P[0].VineT)
	}
	prev := g.P[0].VineA
	for g.P[0].VineT > 0 {
		g.Step(dt)
		if g.P[0].VineA > prev+1e-9 {
			t.Fatalf("the vines went outwards: %v after %v", g.P[0].VineA, prev)
		}
		prev = g.P[0].VineA
	}
	if g.P[0].VineA != 0 {
		t.Fatalf("VineA = %v when the swing ends, want 0: they meet on the aim", g.P[0].VineA)
	}
}

func TestVineWhipHitsWhatEitherVineSweepsOverAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		bearing, dist float64
		hit           bool
	}{
		{0, 100, true}, {60, 100, true}, {-60, 100, true}, // the whole fan between the start angles
		{110, 100, false}, {-110, 100, false}, // beside it
		{0, 400, false}, // too far for the vine
	} {
		cfg := DefaultConfig()
		g := chGame(cfg)
		g.P[0].AimX, g.P[0].AimY = 1, 0
		vineFoe(g, tc.bearing, tc.dist)
		castVine(g)
		steps(g, 30)
		if got := g.P[1].HP < cfg.Snorlax.MaxHP; got != tc.hit {
			t.Errorf("foe at %v degrees, %v away: hit = %v, want %v", tc.bearing, tc.dist, got, tc.hit)
		}
	}
}

func TestVineWhipHurtsOnceEvenWhenBothVinesMeetOnTheTarget(t *testing.T) {
	cfg := DefaultConfig()
	g := chGame(cfg)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	vineFoe(g, 0, 100)
	castVine(g)
	steps(g, 30)
	if want := cfg.Snorlax.MaxHP - cfg.Chikorita.VineWhip.Damage; g.P[1].HP != want {
		t.Fatalf("HP = %d, want %d: one hit per swing", g.P[1].HP, want)
	}
}

func TestVineWhipFollowsTheAimAndLocksIt(t *testing.T) {
	cfg := DefaultConfig()
	g := chGame(cfg)
	g.P[0].In = Input{AimX: 0, AimY: 1} // aim straight down
	g.Step(dt)
	g.P[0].In = Input{Vine: true, AimX: 0, AimY: 1}
	g.Step(dt)
	g.P[0].In = Input{AimX: 1, AimY: 0} // turning the stick afterwards changes nothing
	if g.P[0].VineX != 0 || g.P[0].VineY != 1 {
		t.Fatalf("vine aim = %v,%v, want 0,1", g.P[0].VineX, g.P[0].VineY)
	}
	g.P[1].X, g.P[1].Y = g.P[0].X, g.P[0].Y+100
	steps(g, 30)
	if g.P[1].HP == cfg.Snorlax.MaxHP {
		t.Fatal("the vines did not go where it aimed")
	}
}

func TestVineBindStopsWalkingAndMovementSkillsButNotOthers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.VineWhip.BindDuration = 1
	g := chGame(cfg)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	vineFoe(g, 0, 100)
	castVine(g)
	steps(g, 15)
	if g.P[1].BoundT <= 0 {
		t.Fatalf("BoundT = %v, want the foe bound", g.P[1].BoundT)
	}
	x := g.P[1].X
	g.P[1].In = Input{DX: 1, Skill: true} // walk, jump
	steps(g, 10)
	if g.P[1].X != x || g.P[1].Z != 0 {
		t.Fatalf("moved to x=%v z=%v while bound", g.P[1].X, g.P[1].Z)
	}
	g.P[1].In = Input{Beam: true} // standing skills still work
	g.Step(dt)
	if g.P[1].BeamT <= 0 {
		t.Fatal("the bound Snorlax could not use Hyper Beam")
	}
	g.P[1].In = Input{}
	steps(g, 5*tickRate) // the beam's own charge, sweep and recharge root it for a while
	g.P[1].In = Input{DX: 1}
	g.Step(dt)
	if g.P[1].X <= x {
		t.Fatal("still bound after bind_duration")
	}
}

func TestVineBindBlocksEachMovementSkill(t *testing.T) {
	cfg := DefaultConfig()
	for _, tc := range []struct {
		name  string
		kind  int
		in    Input
		moved func(g *Game) bool
	}{
		{"pikachu dash", pikachu, Input{Skill: true, DX: 1}, func(g *Game) bool { return g.P[1].DashT > 0 || g.P[1].DashCD > 0 }},
		{"snorlax jump", snorlax, Input{Skill: true}, func(g *Game) bool { return g.P[1].Z > 0 || g.P[1].JumpCD > 0 }},
		{"snorlax ice punch", snorlax, Input{Punch: true}, func(g *Game) bool { return g.P[1].PunchT > 0 || g.P[1].PunchCD > 0 }},
		{"charizard fly", charizard, Input{Skill: true}, func(g *Game) bool { return g.P[1].FlyT > 0 }},
		{"charizard toss", charizard, Input{Toss: true}, func(g *Game) bool { return g.P[1].TossPhase != tossNone }},
		{"chikorita leaf slash", chikorita, Input{Skill: true}, func(g *Game) bool { return g.P[1].LeafPhase != leafNone }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := duel(cfg, chikorita, tc.kind)
			g.P[0].AimX, g.P[0].AimY = 1, 0
			g.P[1].X, g.P[1].Y = g.P[0].X+100, g.P[0].Y
			g.P[1].BoundT = 5
			g.P[1].In = tc.in
			steps(g, 10)
			if tc.moved(g) {
				t.Fatal("a bound player used a movement skill")
			}
			g.P[1].BoundT = 0
			steps(g, 3)
			if !tc.moved(g) {
				t.Fatal("setup: the skill should work once the bind is over")
			}
		})
	}
}

func TestChikoritaIsRootedWhileTheVinesSwing(t *testing.T) {
	g := chGame(DefaultConfig())
	g.P[1].X, g.P[1].Y = 1900, 900
	castVine(g)
	x := g.P[0].X
	g.P[0].In = Input{DX: 1}
	g.Step(dt)
	if g.P[0].X != x {
		t.Fatal("walked while the vines were swinging")
	}
	castVineAgain := g.P[0].VineCD
	if castVineAgain <= 0 {
		t.Fatal("the cooldown should be running")
	}
}

func TestVineWhipCannotStartInTheMiddleOfALeafSlash(t *testing.T) {
	g := chGame(DefaultConfig())
	g.P[1].X, g.P[1].Y = 1900, 900
	hopRight(g)
	castVine(g)
	if g.P[0].VineT != 0 {
		t.Fatal("swung the vines mid-hop")
	}
}

func TestChikoritaSnapshotCarriesVineAndRazorState(t *testing.T) {
	g := chGame(DefaultConfig())
	g.P[1].X, g.P[1].Y = 1900, 900
	castVine(g)
	g.P[1].BoundT = 1
	b, _ := json.Marshal(g.P)
	for _, k := range []string{`"vt":`, `"va":`, `"vx":`, `"vy":`, `"vcd":`, `"rcd":`, `"bd":`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("snapshot lacks %s", k)
		}
	}
}

func TestNewChikoritaSkillConfigLoadsAndRefusesBadValues(t *testing.T) {
	c, err := loadJSON(t, `{"chikorita":{"razor_leaf":{"count":12,"scatter_radius":200,"min_delay":0.1,"max_delay":0.4,"leaf_hp":9,"leaf_scale":2.5,"max_height":80},
		"vine_whip":{"length":200,"sweep_angle":80,"sweep_duration":0.5,"bind_duration":3}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if r := c.Chikorita.RazorLeaf; r.Count != 12 || r.ScatterRadius != 200 || r.MinDelay != 0.1 || r.MaxDelay != 0.4 || r.LeafHP != 9 || r.LeafScale != 2.5 || r.MaxHeight != 80 {
		t.Fatalf("razor_leaf = %+v", r)
	}
	if v := c.Chikorita.VineWhip; v.Length != 200 || v.SweepAngle != 80 || v.SweepDuration != 0.5 || v.BindDuration != 3 {
		t.Fatalf("vine_whip = %+v", v)
	}
	for name, body := range map[string]string{
		"no leaves":       `{"chikorita":{"razor_leaf":{"count":0}}}`,
		"too many":        `{"chikorita":{"razor_leaf":{"count":201}}}`,
		"delays reversed": `{"chikorita":{"razor_leaf":{"min_delay":2,"max_delay":1}}}`,
		"zero leaf scale": `{"chikorita":{"razor_leaf":{"leaf_scale":0}}}`,
		"no leaf hp":      `{"chikorita":{"razor_leaf":{"leaf_hp":0}}}`,
		"zero scatter":    `{"chikorita":{"razor_leaf":{"scatter_radius":0}}}`,
		"zero length":     `{"chikorita":{"vine_whip":{"length":0}}}`,
		"zero angle":      `{"chikorita":{"vine_whip":{"sweep_angle":0}}}`,
		"angle too wide":  `{"chikorita":{"vine_whip":{"sweep_angle":181}}}`,
		"zero duration":   `{"chikorita":{"vine_whip":{"sweep_duration":0}}}`,
		"negative bind":   `{"chikorita":{"vine_whip":{"bind_duration":-1}}}`,
		"vine band":       `{"chikorita":{"vine_whip":{"min_height":5,"max_height":1}}}`,
	} {
		if _, err := loadJSON(t, body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
