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
