package main

import (
	"math"
	"testing"
	"time"
)

// ---- 放水: a handicap that cuts HP and walking speed ----

func TestEaseCutsHPAndSurvivesReset(t *testing.T) {
	cfg := DefaultConfig()
	g := NewGame(cfg)
	g.Ease = [2]int{30, 0}
	g.Reset()
	if want := cfg.maxHPOf(pikachu) * 70 / 100; g.P[0].HP != want {
		t.Fatalf("HP %d, want %d (30%% off)", g.P[0].HP, want)
	}
	if g.P[1].HP != cfg.maxHPOf(snorlax) {
		t.Fatalf("the other player's HP %d, want it untouched", g.P[1].HP)
	}
	g.P[0].HP = 1
	g.Reset()
	if g.P[0].HP != cfg.maxHPOf(pikachu)*70/100 {
		t.Fatal("the handicap was lost on Reset")
	}
	g.Ease = [2]int{90, 0}
	g.SetKinds(snorlax, pikachu)
	if want := cfg.maxHPOf(snorlax) * 10 / 100; g.P[0].HP != want {
		t.Fatalf("90%% off a Snorlax: HP %d, want %d", g.P[0].HP, want)
	}
}

func TestEaseSlowsWalkingFlyingAndAirSteering(t *testing.T) {
	cfg := DefaultConfig()
	walked := func(kind, ease int, setup func(p *Player)) float64 {
		g := duel(cfg, kind, pikachu)
		g.Ease = [2]int{ease, 0}
		g.P[0].X, g.P[0].Y = 600, 250
		g.P[1].X, g.P[1].Y = 100, 450
		if setup != nil {
			setup(&g.P[0])
		}
		g.P[0].In = Input{DX: 1}
		x := g.P[0].X
		steps(g, 10)
		return g.P[0].X - x
	}
	for _, kind := range []int{pikachu, snorlax, charizard, chikorita, vulpix} {
		full, half := walked(kind, 0, nil), walked(kind, 50, nil)
		if full <= 0 || math.Abs(half-full/2) > 0.01 {
			t.Errorf("kind %d: walked %.2f at 0%% and %.2f at 50%%, want half", kind, full, half)
		}
	}
	fly := func(p *Player) { p.FlyT, p.Z = 5, 30 }
	if full, half := walked(charizard, 0, fly), walked(charizard, 50, fly); math.Abs(half-full/2) > 0.01 {
		t.Errorf("flying: %.2f and %.2f, want half", full, half)
	}
	air := func(p *Player) { p.Z, p.VZ = 40, 50 }
	if full, half := walked(snorlax, 0, air), walked(snorlax, 50, air); math.Abs(half-full/2) > 0.01 {
		t.Errorf("jumping: %.2f and %.2f, want half", full, half)
	}
}

func TestEaseMustBeATenPercentStepUpToNinety(t *testing.T) {
	for pct, ok := range map[int]bool{0: true, 10: true, 50: true, 90: true, 100: false, 95: false, 5: false, -10: false, 150: false} {
		if validEase(pct) != ok {
			t.Errorf("validEase(%d) = %v, want %v", pct, !ok, ok)
		}
	}
}

func TestRoomTakesEaseOnlyBeforeAMatchAndAppliesItAtTheStart(t *testing.T) {
	r := teamRoom()
	if r.setEase(0, 35) || r.setEase(0, 100) || r.ease[0] != 0 {
		t.Fatal("a bad handicap was taken")
	}
	if !r.setEase(0, 40) || !r.setEase(1, 10) {
		t.Fatal("a good handicap was refused")
	}
	r.choose(0, pikachu)
	r.choose(1, snorlax)
	cfg := DefaultConfig()
	if r.g.P[0].HP != cfg.maxHPOf(pikachu)*60/100 || r.g.P[1].HP != cfg.maxHPOf(snorlax)*90/100 {
		t.Fatalf("HP %d and %d, want 40%% and 10%% off", r.g.P[0].HP, r.g.P[1].HP)
	}
	if r.setEase(0, 0) {
		t.Fatal("the handicap changed in the middle of a match")
	}
	m := snapOf(t, r.step(time.Now()))
	if m.EZ != [2]int{40, 10} {
		t.Fatalf("snapshot ez %v, want [40 10]", m.EZ)
	}
}

func TestTeamBoutsKeepTheHandicap(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setEase(1, 50)
	r.setOrder(0, []int{0, 1, 2, 3, 4})
	r.setOrder(1, []int{0, 1, 2, 3, 4})
	now := time.Now()
	r.step(now)
	knockOut(r, 1, now)
	m := snapOf(t, r.step(now.Add(roundReset+time.Millisecond)))
	if want := DefaultConfig().maxHPOf(1) * 50 / 100; m.P[1].Kind != 1 || m.P[1].HP != want {
		t.Fatalf("slot 1 plays %d with %d HP, want its second character with %d (half)", m.P[1].Kind, m.P[1].HP, want)
	}
}
