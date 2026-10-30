package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---- Pikachu's Volt Tackle: a chain of dashes that paralyzes, and how it clashes with the heavier skills ----

// voltGame has Pikachu at (500,250) and a Snorlax at (560,250), a body-width away along the dash.
func voltGame(cfg Config) *Game {
	g := duel(cfg, pikachu, snorlax)
	g.P[0].X, g.P[0].Y = 500, 250
	g.P[1].X, g.P[1].Y = 560, 250
	return g
}

// dashStarts holds the space bar for n ticks and returns the ticks (1-based) on which a dash began.
func dashStarts(g *Game, n int) []int {
	var starts []int
	g.P[0].In = Input{DX: 0, DY: 0, Skill: true}
	g.P[0].FX, g.P[0].FY = 0, -1 // dash up the court, away from the opponent
	for tick := 1; tick <= n; tick++ {
		was := g.P[0].DashT
		g.Step(dt)
		if was == 0 && g.P[0].DashT > 0 {
			starts = append(starts, tick)
		}
	}
	return starts
}

func TestVoltTackleChainsMaxUsesThenCoolsDown(t *testing.T) {
	g := voltGame(DefaultConfig()) // 3 uses, a 0.2s dash (6 ticks), a 0.6s cooldown (18 ticks)
	got := dashStarts(g, 35) // the second chain's first dash is the fourth
	if len(got) != 4 || got[0] != 1 || got[1] != 7 || got[2] != 13 || got[3] < 31 || got[3] > 32 {
		t.Fatalf("dashes began on ticks %v, want 1, 7, 13 back to back, then the cooldown (0.6s from the third) until ~31", got)
	}
}

func TestVoltTackleCooldownStartsWhenTheWindowRunsOut(t *testing.T) {
	cfg := DefaultConfig() // 3 uses within 2s
	g := voltGame(cfg)
	press := func() {
		g.P[0].In = Input{Skill: true}
		g.P[0].FX, g.P[0].FY = 0, -1
		g.Step(dt)
		g.P[0].In = Input{}
	}
	press()
	if p := g.P[0]; p.VoltLeft != 2 || p.DashCD != 0 {
		t.Fatalf("after the first use: %d uses left, cooldown %v, want 2 and none yet", p.VoltLeft, p.DashCD)
	}
	steps(g, 50) // 1.7s in: the window is still open
	press()
	if p := g.P[0]; p.DashT == 0 || p.VoltLeft != 1 {
		t.Fatalf("a second use inside the window: dash %v, %d left, want it to go with 1 left", p.DashT, p.VoltLeft)
	}

	g = voltGame(cfg)
	press()
	steps(g, 70) // 2.3s in: the window (2s) has run out with two uses unspent
	if p := g.P[0]; p.VoltLeft != 0 || !(p.DashCD > 0 && p.DashCD < cfg.Pikachu.VoltTackle.Cooldown) {
		t.Fatalf("after the window: %d uses left, cooldown %v, want the chain over and the cooldown running", p.VoltLeft, p.DashCD)
	}
	press()
	if g.P[0].DashT != 0 {
		t.Fatal("a use while the cooldown runs must not start a dash")
	}
	steps(g, 30)
	press()
	if p := g.P[0]; p.DashT == 0 || p.VoltLeft != cfg.Pikachu.VoltTackle.MaxUses-1 {
		t.Fatalf("after the cooldown: dash %v, %d left, want a fresh chain", p.DashT, p.VoltLeft)
	}
}

func TestVoltTackleHitsOncePerDashAndParalyzes(t *testing.T) {
	cfg := DefaultConfig() // dashes 220 through a Snorlax 60 ahead, and stays overlapping for several ticks
	g := voltGame(cfg)
	g.P[0].In = Input{DX: 1, Skill: true}
	g.Step(dt)
	g.P[0].In = Input{}
	if p := g.P[1]; p.ParalyzeT < cfg.Pikachu.VoltTackle.ParalyzeDuration-0.1 || p.ParalyzeT > cfg.Pikachu.VoltTackle.ParalyzeDuration {
		t.Fatalf("paralysis %v, want about %v", p.ParalyzeT, cfg.Pikachu.VoltTackle.ParalyzeDuration)
	}
	steps(g, 10)
	if want := cfg.Snorlax.MaxHP - cfg.Pikachu.VoltTackle.Damage; g.P[1].HP != want {
		t.Fatalf("Snorlax HP %d, want %d: one hit per dash", g.P[1].HP, want)
	}

	// Paralysis stops walking and the movement skills (jump); the other skills still work.
	x := g.P[1].X
	g.P[1].In = Input{DX: 1, Skill: true}
	steps(g, 5)
	if g.P[1].X != x || g.P[1].Z != 0 || g.P[1].VZ != 0 {
		t.Fatalf("paralyzed Snorlax moved to x=%v z=%v", g.P[1].X, g.P[1].Z)
	}
	g.P[1].In = Input{Beam: true}
	g.Step(dt)
	if g.P[1].BeamT == 0 {
		t.Fatal("a paralyzed Snorlax should still be able to start Hyper Beam")
	}
	g.P[1].In = Input{}
	g.P[1].BeamT, g.P[1].SweepT, g.P[1].RechargeT = 0, 0, 0
	steps(g, 60) // 2s: it wears off
	if g.P[1].ParalyzeT != 0 {
		t.Fatalf("paralysis %v after 2s, want it gone", g.P[1].ParalyzeT)
	}
	g.P[1].In = Input{DX: 1}
	g.Step(dt)
	if g.P[1].X <= x {
		t.Fatal("Snorlax should walk again once the paralysis is over")
	}
}

func TestVoltTackleMissesWhoeverIsOutsideItsHeightBand(t *testing.T) {
	for _, tc := range []struct {
		name string
		max  float64
		hit  bool
	}{{"band stops below the target", 12, false}, {"band reaches the target", 200, true}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Pikachu.VoltTackle.MaxHeight = tc.max
			g := voltGame(cfg)
			g.P[1].Z = 100 // mid-jump
			g.P[0].In = Input{DX: 1, Skill: true}
			g.Step(dt)
			if hit := g.P[1].ParalyzeT > 0; hit != tc.hit || (g.P[1].HP < cfg.Snorlax.MaxHP) != tc.hit {
				t.Fatalf("paralyzed=%v hp=%d, want hit=%v", g.P[1].ParalyzeT > 0, g.P[1].HP, tc.hit)
			}
		})
	}
}

// clash puts a Pikachu mid-Volt-Tackle (slot pi) face to face with another character running its own dash, in
// either slot order, since whoever is stepped first must not decide the result.
func clash(cfg Config, other int, swap bool) (g *Game, pi, oi int) {
	pi, oi = 0, 1
	a, b := pikachu, other
	if swap {
		pi, oi, a, b = 1, 0, other, pikachu
	}
	g = duel(cfg, a, b)
	g.P[pi].X, g.P[pi].Y = 500, 250
	g.P[oi].X, g.P[oi].Y = 560, 250
	vt := cfg.Pikachu.VoltTackle
	g.P[pi].DashT, g.P[pi].DashX, g.P[pi].VoltLeft, g.P[pi].VoltT = vt.Duration, 1, vt.MaxUses-1, vt.UseWindow
	switch other {
	case snorlax:
		g.P[oi].PunchLeft, g.P[oi].PunchDX, g.P[oi].PunchID = cfg.Snorlax.IcePunch.Distance, -1, 1
	case charizard:
		g.P[oi].TossPhase, g.P[oi].TossLeft, g.P[oi].TossDX = tossDash, cfg.Charizard.SeismicToss.Length, -1
	}
	return g, pi, oi
}

func TestIcePunchBeatsAVoltTackle(t *testing.T) {
	cfg := DefaultConfig()
	for _, swap := range []bool{false, true} {
		g, pi, oi := clash(cfg, snorlax, swap)
		steps(g, 3)
		p, s := g.P[pi], g.P[oi]
		if p.FrozenT == 0 || p.DashT != 0 || p.HP != cfg.Pikachu.MaxHP-cfg.Snorlax.IcePunch.Damage {
			t.Errorf("swap=%v: Pikachu frozen %v dash %v hp %d, want it frozen, stopped and punched", swap, p.FrozenT, p.DashT, p.HP)
		}
		if s.ParalyzeT != 0 || s.HP != cfg.Snorlax.MaxHP {
			t.Errorf("swap=%v: Snorlax paralyzed %v hp %d, want it untouched by the lighter tackle", swap, s.ParalyzeT, s.HP)
		}
	}
}

func TestSeismicTossBeatsAVoltTackle(t *testing.T) {
	cfg := DefaultConfig()
	for _, swap := range []bool{false, true} {
		g, pi, oi := clash(cfg, charizard, swap)
		steps(g, 3)
		p, c := g.P[pi], g.P[oi]
		if !p.Held || c.TossPhase != tossRise || p.DashT != 0 {
			t.Errorf("swap=%v: Pikachu held=%v dash=%v, Charizard phase %d, want it caught and lifted", swap, p.Held, p.DashT, c.TossPhase)
		}
		if c.ParalyzeT != 0 || c.HP != cfg.Charizard.MaxHP || p.ParalyzeT != 0 {
			t.Errorf("swap=%v: Charizard paralyzed %v hp %d: the heavier toss takes no paralysis", swap, c.ParalyzeT, c.HP)
		}
	}
}

func TestIcePunchBeatsASeismicToss(t *testing.T) {
	cfg := DefaultConfig()
	for _, swap := range []bool{false, true} {
		si, ci := 0, 1
		a, b := snorlax, charizard
		if swap {
			si, ci, a, b = 1, 0, charizard, snorlax
		}
		g := duel(cfg, a, b)
		g.P[si].X, g.P[si].Y = 500, 250
		g.P[ci].X, g.P[ci].Y = 560, 250
		g.P[si].PunchLeft, g.P[si].PunchDX, g.P[si].PunchID = cfg.Snorlax.IcePunch.Distance, 1, 1
		g.P[ci].TossPhase, g.P[ci].TossLeft, g.P[ci].TossDX = tossDash, cfg.Charizard.SeismicToss.Length, -1
		steps(g, 3)
		s, c := g.P[si], g.P[ci]
		if c.FrozenT == 0 || c.TossPhase != tossNone || c.HP != cfg.Charizard.MaxHP-cfg.Snorlax.IcePunch.Damage {
			t.Errorf("swap=%v: Charizard frozen %v phase %d hp %d, want it frozen with the toss cancelled", swap, c.FrozenT, c.TossPhase, c.HP)
		}
		if s.Held || s.Z != 0 || s.HP != cfg.Snorlax.MaxHP {
			t.Errorf("swap=%v: Snorlax held=%v z=%v hp=%d, want it left alone: no lift, no slam", swap, s.Held, s.Z, s.HP)
		}
	}
}

func TestEqualMassesCancelEachOther(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.SeismicToss.Mass = cfg.Snorlax.IcePunch.Mass
	for _, swap := range []bool{false, true} {
		si, ci := 0, 1
		a, b := snorlax, charizard
		if swap {
			si, ci, a, b = 1, 0, charizard, snorlax
		}
		g := duel(cfg, a, b)
		g.P[si].X, g.P[si].Y = 500, 250
		g.P[ci].X, g.P[ci].Y = 560, 250
		g.P[si].PunchLeft, g.P[si].PunchDX, g.P[si].PunchID = cfg.Snorlax.IcePunch.Distance, 1, 1
		g.P[ci].TossPhase, g.P[ci].TossLeft, g.P[ci].TossDX = tossDash, cfg.Charizard.SeismicToss.Length, -1
		steps(g, 3)
		if s, c := g.P[si], g.P[ci]; c.FrozenT != 0 || s.Held || c.Held || s.FrozenT != 0 || s.PunchLeft != 0 || c.TossPhase != tossNone || s.HP != cfg.Snorlax.MaxHP || c.HP != cfg.Charizard.MaxHP {
			t.Errorf("swap=%v: punch left %v, toss phase %d, frozen %v/%v, hp %d/%d: equal masses should stop both and hurt neither", swap, s.PunchLeft, c.TossPhase, s.FrozenT, c.FrozenT, s.HP, c.HP)
		}
	}
}

func TestAHeavierTackleWinsWhenTheConfigSaysSo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.VoltTackle.Mass = cfg.Snorlax.IcePunch.Mass + 1
	g, pi, oi := clash(cfg, snorlax, false)
	steps(g, 3)
	if g.P[oi].ParalyzeT == 0 || g.P[pi].FrozenT != 0 || g.P[oi].PunchLeft != 0 {
		t.Fatalf("Snorlax paralyzed %v, punch left %v, Pikachu frozen %v: the heavier tackle should win", g.P[oi].ParalyzeT, g.P[oi].PunchLeft, g.P[pi].FrozenT)
	}
}

func TestDefaultMassesOrderThePunchTossTackle(t *testing.T) {
	c := DefaultConfig()
	if !(c.Snorlax.IcePunch.Mass > c.Charizard.SeismicToss.Mass && c.Charizard.SeismicToss.Mass > c.Pikachu.VoltTackle.Mass) {
		t.Fatalf("masses punch %v, toss %v, tackle %v, want punch > toss > tackle", c.Snorlax.IcePunch.Mass, c.Charizard.SeismicToss.Mass, c.Pikachu.VoltTackle.Mass)
	}
	r, err := LoadConfig("config.json")
	if err != nil {
		t.Fatal(err)
	}
	if !(r.Snorlax.IcePunch.Mass > r.Charizard.SeismicToss.Mass && r.Charizard.SeismicToss.Mass > r.Pikachu.VoltTackle.Mass) {
		t.Fatalf("config.json masses punch %v, toss %v, tackle %v, want punch > toss > tackle", r.Snorlax.IcePunch.Mass, r.Charizard.SeismicToss.Mass, r.Pikachu.VoltTackle.Mass)
	}
}

func TestVoltTackleStateIsInTheSnapshot(t *testing.T) {
	g := voltGame(DefaultConfig())
	g.P[0].In = Input{DX: 1, Skill: true}
	g.Step(dt)
	g.P[1].ParalyzeT = 1
	b, _ := json.Marshal(g.P)
	for _, k := range []string{`"vd":`, `"vn":`, `"vw":`, `"pz":`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("snapshot lacks %s", k)
		}
	}
}
