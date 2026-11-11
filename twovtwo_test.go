package main

import "testing"

// A 2v2 is four slots: 0 and 2 fight for the left, 1 and 3 for the right.
func newTeamTestGame(kinds ...int) *Game {
	g := NewTeamGame(DefaultConfig(), modeWeb, len(kinds))
	for i, k := range kinds {
		g.P[i].Kind = k
	}
	g.Reset()
	return g
}

func TestATeamGameStartsFourFightersTwoPerSideFacingEachOther(t *testing.T) {
	g := newTeamTestGame(pikachu, snorlax, pikachu, snorlax)
	if len(g.P) != 4 {
		t.Fatalf("%d fighters, want 4", len(g.P))
	}
	for i := range g.P {
		if want := i % 2; g.side(i) != want {
			t.Errorf("slot %d is on side %d, want %d", i, g.side(i), want)
		}
		if left := g.P[i].X < g.court.Length/2; left != (g.side(i) == 0) {
			t.Errorf("slot %d starts at x=%v: the wrong half for its side", i, g.P[i].X)
		}
	}
	if g.P[0].Y == g.P[2].Y || g.P[1].Y == g.P[3].Y {
		t.Fatalf("teammates start on the same spot: y %v and %v, %v and %v", g.P[0].Y, g.P[2].Y, g.P[1].Y, g.P[3].Y)
	}
	if got := g.foes(0); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("foes of slot 0 = %v, want [1 3]", got)
	}
}

func TestADuelStillHasTheOneOpponentWhoeverIsStanding(t *testing.T) {
	g := newTestGame()
	g.P[snorlax].HP = 0
	if got := g.foes(pikachu); len(got) != 1 || got[0] != snorlax {
		t.Fatalf("foes in a duel = %v, want the fallen opponent still named", got)
	}
}

func TestASideLosesWhenBothItsFightersHaveFallen(t *testing.T) {
	g := newTeamTestGame(pikachu, snorlax, pikachu, snorlax)
	if g.Winner() != -1 {
		t.Fatal("a fresh 2v2 has a winner")
	}
	g.P[1].HP = 0
	if g.Winner() != -1 {
		t.Fatal("one fallen fighter ended the match")
	}
	g.P[3].HP = 0
	if g.Winner() != 0 {
		t.Fatalf("winner = %d, want side 0", g.Winner())
	}
	g.P[0].HP, g.P[2].HP = 0, 0
	if g.Winner() != 0 {
		t.Fatalf("everyone fallen: winner = %d, want the deterministic side 0", g.Winner())
	}
}

func TestTheNearestFoeIsTheOneAimedAtAndAFallenOneIsSkipped(t *testing.T) {
	g := newTeamTestGame(pikachu, snorlax, pikachu, snorlax)
	g.P[0].X, g.P[0].Y = 500, 400
	g.P[1].X, g.P[1].Y = 900, 400 // 400 away
	g.P[3].X, g.P[3].Y = 650, 400 // 150 away
	g.P[2].X, g.P[2].Y = 520, 400 // a teammate right beside: never a target
	if got := g.nearestFoe(0); got != 3 {
		t.Fatalf("nearest foe = %d, want 3", got)
	}
	g.P[3].HP = 0
	if got := g.nearestFoe(0); got != 1 {
		t.Fatalf("with 3 fallen, nearest foe = %d, want 1", got)
	}
	if got := g.foes(0); len(got) != 1 || got[0] != 1 {
		t.Fatalf("foes = %v, want [1]", got)
	}
}

// An aimed skill goes at the nearest foe, not at the first one.
func TestAnElectroBallGoesAtTheNearestFoe(t *testing.T) {
	g := newTeamTestGame(pikachu, snorlax, snorlax, snorlax)
	g.P[0].X, g.P[0].Y = 400, 400
	g.P[1].X, g.P[1].Y = 1200, 400 // far, straight ahead
	g.P[3].X, g.P[3].Y = 400, 700  // near, to the side
	g.P[2].X, g.P[2].Y = 100, 100
	g.P[0].In.Ball = true
	for i := 0; i < tickRate*2 && len(g.Balls) == 0; i++ {
		g.Step(dt)
	}
	if len(g.Balls) == 0 {
		t.Fatal("no ball was thrown")
	}
	if b := g.Balls[0]; b.DY < 0.9 {
		t.Fatalf("the ball flies along (%v, %v), want it at the nearest foe below", b.DX, b.DY)
	}
}

// A teammate in the way is passed through, a foe behind it is hit.
func TestABallFliesThroughATeammateAndHurtsTheFoeBehindIt(t *testing.T) {
	g := newTeamTestGame(pikachu, snorlax, snorlax, snorlax)
	g.P[0].X, g.P[0].Y = 400, 400
	g.P[2].X, g.P[2].Y = 520, 400 // the teammate, in the way
	g.P[1].X, g.P[1].Y = 800, 400
	g.P[3].X, g.P[3].Y = 1500, 1000
	g.Balls = append(g.Balls, Ball{Owner: 0, X: 430, Y: 400, DX: 1, DY: 0, HP: g.cfg.Pikachu.ElectroBall.HP})
	mateHP, foeHP := g.P[2].HP, g.P[1].HP
	for i := 0; i < tickRate*2 && g.P[1].HP == foeHP; i++ {
		g.Step(dt)
	}
	if g.P[1].HP >= foeHP {
		t.Fatal("the foe behind the teammate was not hurt")
	}
	if g.P[2].HP != mateHP {
		t.Fatalf("the teammate took %d damage from its own side's ball", mateHP-g.P[2].HP)
	}
}

// A landing shock hurts every foe in reach and no teammate.
func TestASlamHurtsBothFoesAndSparesTheTeammate(t *testing.T) {
	g := newTeamTestGame(snorlax, pikachu, pikachu, pikachu)
	g.P[0].X, g.P[0].Y = 600, 500
	g.P[1].X, g.P[1].Y = 650, 500 // inside the slam
	g.P[3].X, g.P[3].Y = 600, 550 // inside the slam too
	g.P[2].X, g.P[2].Y = 560, 500 // the teammate, inside it as well
	g.P[0].In.Skill = true
	for range tickRate * 2 {
		g.Step(dt)
	}
	for _, j := range []int{1, 3} {
		if g.P[j].HP != g.maxHP(j)-slamDamage {
			t.Errorf("foe %d HP = %d, want %d", j, g.P[j].HP, g.maxHP(j)-slamDamage)
		}
	}
	if g.P[2].HP != g.maxHP(2) {
		t.Fatalf("the teammate lost %d HP to its own side's slam", g.maxHP(2)-g.P[2].HP)
	}
}

// A fallen fighter of a 2v2 neither acts nor is a target.
func TestAFallenFighterOfATeamGameIsInert(t *testing.T) {
	g := newTeamTestGame(snorlax, pikachu, pikachu, pikachu)
	g.P[3].HP = 0
	g.P[3].In.Skill = true
	g.P[0].X, g.P[0].Y = 600, 500
	g.P[3].X, g.P[3].Y = 640, 500
	g.P[1].X, g.P[1].Y = 1600, 1000
	g.P[2].X, g.P[2].Y = 100, 100
	for range tickRate {
		g.Step(dt)
	}
	if g.P[3].DashT != 0 || g.P[3].DashCD != 0 || g.P[3].VoltLeft != 0 {
		t.Fatal("the fallen fighter used its skill")
	}
	g.P[0].In.Skill = true
	for range tickRate * 2 {
		g.Step(dt)
	}
	if g.P[3].HP != 0 || g.P[3].X != 640 {
		t.Fatalf("the fallen fighter was hurt or pushed: HP %d, x %v", g.P[3].HP, g.P[3].X)
	}
}

// ---- the computer in a 2v2 ----

// teamFight plays four bots to a finish (or the time limit).
func teamFight(cfg Config, seed uint64, kinds [4]int, limit float64) (winner int, secs float64, g *Game) {
	g = NewTeamGame(cfg, modeWeb, 4)
	for i, k := range kinds {
		g.P[i].Kind = k
	}
	g.Reset()
	var bots [4]*Bot
	for i, k := range kinds {
		bots[i] = newBot(k, seed+uint64(i)*1000)
	}
	for t := 0.0; t < limit; t += dt {
		for i, bt := range bots {
			if g.P[i].HP > 0 {
				g.P[i].In = bt.Decide(g, i)
			}
		}
		g.Step(dt)
		if w := g.Winner(); w >= 0 {
			return w, t, g
		}
	}
	return -1, limit, g
}

func TestFourBotsFightA2v2ToAFinishAndOnlyTheLosingSideFalls(t *testing.T) {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		t.Fatal(err)
	}
	teams := [][4]int{
		{pikachu, snorlax, ash, charizard},
		{snorlax, pikachu, vulpix, gengar},
		{chikorita, machamp, pikachu, snorlax},
	}
	finished := 0
	for n, kinds := range teams {
		for seed := uint64(1); seed <= 3; seed++ {
			w, secs, g := teamFight(cfg, seed*7+uint64(n), kinds, 240)
			if w < 0 {
				continue
			}
			finished++
			for i := range g.P {
				if g.side(i) == w && g.P[i].HP > 0 {
					goto standing
				}
			}
			t.Errorf("teams %v seed %d: side %d won after %.0fs with nobody of it standing", kinds, seed, w, secs)
		standing:
			for i := range g.P {
				if g.side(i) == w {
					continue
				}
				if g.P[i].HP != 0 {
					t.Errorf("teams %v seed %d: the losing side still has slot %d on %d HP", kinds, seed, i, g.P[i].HP)
				}
			}
		}
	}
	t.Logf("%d of 9 team fights finished", finished)
	if finished < 6 { // most of the 9 fights must end inside the time limit
		t.Fatalf("only %d of 9 team fights finished in 240s: the bots get stuck", finished)
	}
}
