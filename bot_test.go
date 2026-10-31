package main

import (
	"encoding/json"
	"testing"
	"time"
)

// ---- the computer player ----

func hasAct(in Input) bool { return in.Skill || in.Beam || in.Thunder || in.Ball || in.Punch }

// firstAct runs the bot for n ticks on a scene that does not change and returns the first decision that presses a
// skill key; if it never does, ok is false and the last decision (the stick) is returned.
func firstAct(b *Bot, g *Game, me, n int) (in Input, ok bool) {
	for i := 0; i < n; i++ {
		if in = b.Decide(g, me); hasAct(in) {
			return in, true
		}
	}
	return in, false
}

// botDuel is Pikachu in slot 0 at (300, 250) against Snorlax in slot 1 at (900, 250), both with skills ready.
func botDuel() *Game { return duel(DefaultConfig(), pikachu, snorlax) }

func TestDir8PicksTheNearestOfEightDirections(t *testing.T) {
	for _, c := range []struct {
		vx, vy float64
		dx, dy int
	}{
		{1, 0, 1, 0}, {1, 1, 1, 1}, {0, 1, 0, 1}, {-1, 1, -1, 1}, {-1, 0, -1, 0},
		{-1, -1, -1, -1}, {0, -1, 0, -1}, {1, -1, 1, -1},
		{10, 3, 1, 0}, {10, 6, 1, 1}, {3, -10, 0, -1}, {0, 0, 0, 0},
	} {
		if dx, dy := dir8(c.vx, c.vy); dx != c.dx || dy != c.dy {
			t.Errorf("dir8(%v,%v) = (%d,%d), want (%d,%d)", c.vx, c.vy, dx, dy, c.dx, c.dy)
		}
	}
}

func TestPikachuDodgesAHyperBeamWithAVoltTackle(t *testing.T) {
	g := botDuel()
	g.P[0].X = 600 // 300 from Snorlax, straight along its beam
	g.P[1].BeamT, g.P[1].BeamX, g.P[1].BeamY = 0.5, -1, 0
	in, ok := firstAct(newBot(pikachu, 1), g, 0, 30)
	if !ok || !in.Skill || in.DX != 0 || in.DY == 0 {
		t.Fatalf("decision = %+v (pressed %v), want a Volt Tackle at right angles to the beam", in, ok)
	}
}

func TestPikachuLeavesTheBeamOnFootWhenTheDashIsSpent(t *testing.T) {
	g := botDuel()
	g.P[0].X, g.P[0].DashCD = 600, 5
	g.P[1].BeamT, g.P[1].BeamX, g.P[1].BeamY = 0.5, -1, 0
	in, ok := firstAct(newBot(pikachu, 1), g, 0, 30)
	if ok || in.DY == 0 {
		t.Fatalf("decision = %+v (pressed %v), want walking out of the beam without a skill", in, ok)
	}
}

func TestPikachuIsNotAfraidOfABeamItIsOutsideOf(t *testing.T) {
	g := botDuel()
	g.P[0].X, g.P[0].Y = 600, 700 // far to the side of a beam that points along the court
	g.P[1].BeamT, g.P[1].BeamX, g.P[1].BeamY = 0.5, -1, 0
	g.P[0].BallCD = 5
	g.P[0].ThunderCD = 5
	if in, ok := firstAct(newBot(pikachu, 1), g, 0, 30); ok && in.Skill {
		t.Fatalf("it dodged a beam that cannot reach it: %+v", in)
	}
}

func TestPikachuRushesWithTheRestOfTheChainAfterADodge(t *testing.T) {
	g := botDuel()
	g.P[0].X = 780 // 120 from Snorlax, on the same row
	g.P[0].VoltLeft, g.P[0].VoltT = 2, 1.5
	in, ok := firstAct(newBot(pikachu, 1), g, 0, 30)
	if !ok || !in.Skill || in.DX != 1 || in.DY != 0 {
		t.Fatalf("decision = %+v (pressed %v), want a Volt Tackle straight at Snorlax", in, ok)
	}
}

func TestPikachuDoesNotDashAtSomethingItCannotReach(t *testing.T) {
	g := botDuel()
	g.P[0].VoltLeft, g.P[0].VoltT = 2, 1.5 // 600 away
	if in, ok := firstAct(newBot(pikachu, 1), g, 0, 30); ok && in.Skill {
		t.Fatalf("dashed from 600 away: %+v", in)
	}
}

func TestPikachuCastsThunderOnAParalyzedOpponent(t *testing.T) {
	g := botDuel()
	g.P[1].ParalyzeT = 1.4
	in, ok := firstAct(newBot(pikachu, 1), g, 0, 30)
	if !ok || !in.Thunder {
		t.Fatalf("decision = %+v (pressed %v), want Thunder", in, ok)
	}
}

func TestPikachuCastsThunderOnAnOpponentRecoveringFromItsBeam(t *testing.T) {
	g := botDuel()
	g.P[1].RechargeT = 2
	if in, ok := firstAct(newBot(pikachu, 1), g, 0, 30); !ok || !in.Thunder {
		t.Fatalf("decision = %+v (pressed %v), want Thunder", in, ok)
	}
}

func TestPikachuHoldsThunderWhileSomethingIsComingAtIt(t *testing.T) {
	g := botDuel()
	g.P[0].X = 600
	g.P[1].SweepT, g.P[1].BeamX, g.P[1].BeamY = 1, -1, 0 // Snorlax is rooted for seconds, but Pikachu is in the fan it is sweeping
	if in, ok := firstAct(newBot(pikachu, 1), g, 0, 30); ok && in.Thunder {
		t.Fatalf("it cast Thunder inside the beam: %+v", in)
	}
}

func TestPikachuThrowsElectroBallsFromAMidRange(t *testing.T) {
	g := botDuel() // 600 apart
	in, ok := firstAct(newBot(pikachu, 1), g, 0, 30)
	if !ok || !in.Ball || in.Skill {
		t.Fatalf("decision = %+v (pressed %v), want an Electro Ball", in, ok)
	}
	if in.AimX < 0.99 {
		t.Fatalf("aim (%v,%v), want straight at Snorlax", in.AimX, in.AimY)
	}
}

func TestPikachuKeepsOutOfIcePunchRange(t *testing.T) {
	g := botDuel()
	g.P[0].BallCD, g.P[0].ThunderCD, g.P[0].DashCD = 5, 5, 5
	g.P[0].X = 600 // 300 from Snorlax: inside a punch (350 + 60)
	in, _ := firstAct(newBot(pikachu, 1), g, 0, 30)
	if in.DX >= 0 {
		t.Fatalf("stick (%d,%d): Pikachu should back away from Snorlax, which is to its right", in.DX, in.DY)
	}
}

func TestPikachuStaysPutWhileItCastsThunder(t *testing.T) {
	g := botDuel()
	g.P[0].ThunderLeft = 2
	if in, ok := firstAct(newBot(pikachu, 1), g, 0, 30); ok || in.DX != 0 || in.DY != 0 {
		t.Fatalf("decision = %+v while rooted", in)
	}
}

func TestPikachuAvoidsWallsWhileCircling(t *testing.T) {
	g := botDuel()
	g.P[0].BallCD, g.P[0].ThunderCD, g.P[0].DashCD = 5, 5, 5
	g.P[0].X, g.P[0].Y = 600, 20 // against the top wall
	g.P[1].X, g.P[1].Y = 900, 20
	for _, strafe := range []float64{1, -1} {
		b := newBot(pikachu, 1)
		b.strafe, b.strafeT = strafe, 99
		in, _ := firstAct(b, g, 0, 8)
		if in.DY < 0 {
			t.Fatalf("strafe %v: stick (%d,%d) pushes into the wall", strafe, in.DX, in.DY)
		}
	}
}

// snorlaxFacing is Snorlax in slot 1 at (900, 250) facing Pikachu at (300, 250).
func snorlaxFacing() *Game { return botDuel() }

func TestSnorlaxJumpsOverAnApproachingBall(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X = 300
	// 260 away at 400/s: 0.65 s, minus what it takes to notice, is the middle of the jump
	g.Balls = []Ball{{K: ballElectro, Owner: 0, X: 640, Y: 250, DX: 1, HP: 30}}
	in, ok := firstAct(newBot(snorlax, 1), g, 1, 30)
	if !ok || !in.Skill {
		t.Fatalf("decision = %+v (pressed %v), want the Jump", in, ok)
	}
}

func TestSnorlaxDoesNotJumpAtABallThatIsStillFarOrGoingAway(t *testing.T) {
	for name, ball := range map[string]Ball{
		"far":  {K: ballElectro, Owner: 0, X: 100, Y: 250, DX: 1, HP: 30},
		"away": {K: ballElectro, Owner: 0, X: 640, Y: 250, DX: -1, HP: 30},
		"wide": {K: ballElectro, Owner: 0, X: 640, Y: 100, DX: 1, HP: 30},
		"own":  {K: ballElectro, Owner: 1, X: 640, Y: 250, DX: 1, HP: 30},
	} {
		g := snorlaxFacing()
		g.P[0].PunchCD = 0
		g.Balls = []Ball{ball}
		b := newBot(snorlax, 1)
		b.rng = newBot(snorlax, 1).rng
		// stay out of the way of its other moves: nothing else can press the jump from 600 away except the
		// 25% engage, which is for 170..330 only
		if in, ok := firstAct(b, g, 1, 30); ok && in.Skill {
			t.Errorf("%s: it jumped: %+v", name, in)
		}
	}
}

func TestSnorlaxDoesNotJumpWhenTheBallIsAlreadyOnTopOfIt(t *testing.T) {
	g := snorlaxFacing()
	g.Balls = []Ball{{K: ballElectro, Owner: 0, X: 840, Y: 250, DX: 1, HP: 30}} // 60 away: 0.15 s, the jump would be too late
	in, ok := firstAct(newBot(snorlax, 1), g, 1, 30)
	if ok && in.Skill {
		t.Fatalf("jumped when it could no longer get above the ball: %+v", in)
	}
	if in.DY == 0 {
		t.Fatalf("stick %+v, want a step out of the way", in)
	}
}

func TestSnorlaxStepsAsideWhenItCannotJump(t *testing.T) {
	g := snorlaxFacing()
	g.P[1].JumpCD = 3
	g.Balls = []Ball{{K: ballElectro, Owner: 0, X: 640, Y: 250, DX: 1, HP: 30}}
	in, ok := firstAct(newBot(snorlax, 1), g, 1, 30)
	if ok || in.DY == 0 {
		t.Fatalf("decision = %+v (pressed %v), want a step out of the ball's path", in, ok)
	}
}

func TestSnorlaxSteersItsJumpAtTheOpponent(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X, g.P[0].Y = 600, 400
	b := newBot(snorlax, 1)
	g.P[1].Z, g.P[1].VZ = 50, 100 // in the air
	in, _ := firstAct(b, g, 1, 8)
	if in.DX >= 0 || in.DY <= 0 {
		t.Fatalf("stick (%d,%d): the jump should head toward Pikachu, up and to the left", in.DX, in.DY)
	}
}

func TestSnorlaxPunchesRightAfterLanding(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ { // not left to chance: every seed does it
		g := snorlaxFacing()
		g.P[0].X = 700 // 200 away
		b := newBot(snorlax, seed)
		g.P[1].Z, g.P[1].VZ = 50, -100
		for i := 0; i < 2*botEvery; i++ { // in the air: no skill keys
			if in := b.Decide(g, 1); hasAct(in) {
				t.Fatalf("pressed %+v in the air", in)
			}
		}
		g.P[1].Z, g.P[1].VZ = 0, 0 // down
		var in Input
		for i := 0; i < botEvery; i++ { // exactly one decision
			in = b.Decide(g, 1)
		}
		if !in.Punch || in.AimX > -0.99 {
			t.Fatalf("seed %d: decision = %+v, want an Ice Punch aimed at Pikachu", seed, in)
		}
	}
}

func TestSnorlaxPunchesOnlyInRange(t *testing.T) {
	g := snorlaxFacing() // 600 away
	b := newBot(snorlax, 1)
	b.comboT = 5
	if in, ok := firstAct(b, g, 1, 30); ok && in.Punch {
		t.Fatalf("punched from 600 away: %+v", in)
	}
}

func TestSnorlaxBeamsAFrozenOpponentOnlyFromCloseRange(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X = 800 // 100 away
	g.P[0].FrozenT = 1.4
	in, ok := firstAct(newBot(snorlax, 1), g, 1, 30)
	if !ok || !in.Beam {
		t.Fatalf("decision = %+v (pressed %v), want the Hyper Beam point-blank", in, ok)
	}

	g = snorlaxFacing()
	g.P[0].X = 600 // 300 away: first walk up to it
	g.P[0].FrozenT = 1.4
	g.P[1].PunchCD = 5
	in, ok = firstAct(newBot(snorlax, 1), g, 1, 30)
	if ok || in.DX >= 0 {
		t.Fatalf("decision = %+v (pressed %v), want a step toward the frozen opponent and no beam yet", in, ok)
	}
}

func TestSnorlaxDoesNotRootItselfWithABeamWhileParalyzed(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X = 800 // right next to it
	g.P[1].ParalyzeT = 1.5
	if in, ok := firstAct(newBot(snorlax, 1), g, 1, 30); ok || in.DX != 0 || in.DY != 0 {
		t.Fatalf("decision = %+v (pressed %v): paralyzed, the beam would only leave it rooted for seconds more", in, ok)
	}
}

func TestAPikachuThatIsOnlyStandingThereIsNoThreatToSnorlax(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X = 800 // close, with its dash ready
	b := newBot(snorlax, 1)
	for i := 0; i < botReact+1; i++ {
		b.observe(g, 1)
	}
	if th := b.look(g, 1).th; th.kind != thNone {
		t.Fatalf("threat %+v", th)
	}
	g.P[0].VoltLeft = 2 // in the middle of a chain it is
	b.observe(g, 1)
	for i := 0; i < botReact; i++ {
		b.observe(g, 1)
	}
	if th := b.look(g, 1).th; th.kind != thRush {
		t.Fatalf("threat %+v, want a rush", th)
	}
	g.P[0].FrozenT = 1 // but not when it is frozen
	for i := 0; i <= botReact; i++ {
		b.observe(g, 1)
	}
	if th := b.look(g, 1).th; th.kind != thNone {
		t.Fatalf("a frozen Pikachu is a threat: %+v", th)
	}
}

func TestSnorlaxLeadsTheIcePunchAtAMovingOpponent(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X, g.P[0].Y = 700, 150 // 200 to the left; it walks to larger y, 5 px a tick
	b := newBot(snorlax, 1)
	for i := 0; i < 60; i++ {
		g.P[0].Y += 5
		in := b.Decide(g, 1)
		if !in.Punch {
			continue
		}
		seen := b.hist[0].foe // what the aim was made from
		directX, directY := unit(seen.X-g.P[1].X, seen.Y-g.P[1].Y)
		if in.AimX >= 0 || in.AimY < directY+0.05 {
			t.Fatalf("aim (%.2f,%.2f), straight at where Pikachu was seen is (%.2f,%.2f): it does not lead", in.AimX, in.AimY, directX, directY)
		}
		return
	}
	t.Fatal("it never punched")
}

func TestSnorlaxWalksUpToAFarOpponent(t *testing.T) {
	g := snorlaxFacing()
	in, _ := firstAct(newBot(snorlax, 1), g, 1, 8)
	if in.DX >= 0 {
		t.Fatalf("stick (%d,%d), want walking toward Pikachu on the left", in.DX, in.DY)
	}
}

func TestSnorlaxWalksOutOfAThunderShadow(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].ThunderLeft, g.P[0].ThunderIn = 3, 0.5
	g.P[0].ShadowX, g.P[0].ShadowY = 900, 200 // right on top of it
	in, ok := firstAct(newBot(snorlax, 1), g, 1, 30)
	if ok || in.DY <= 0 {
		t.Fatalf("decision = %+v (pressed %v), want a step away from the shadow (down: it is above)", in, ok)
	}
}

func TestSnorlaxJumpsWhenPikachuDashes(t *testing.T) {
	g := snorlaxFacing()
	g.P[0].X = 800 // 100 away
	g.P[0].DashT, g.P[0].VoltLeft = 0.1, 2
	g.P[0].BallCD = 5
	in, ok := firstAct(newBot(snorlax, 1), g, 1, 30)
	if !ok || !in.Skill {
		t.Fatalf("decision = %+v (pressed %v), want a jump over the dash", in, ok)
	}
}

// ---- what makes it beatable ----

func TestSkillKeysArePressedForOneTickAndTheStickIsHeld(t *testing.T) {
	g := botDuel()
	b := newBot(pikachu, 1)
	pressed, held := 0, 0
	var stick [2]int
	for i := 0; i < 8*botEvery; i++ {
		in := b.Decide(g, 0)
		if hasAct(in) {
			pressed++
			if i%botEvery != botEvery-1 {
				t.Fatalf("a key was pressed on tick %d, which is not a decision tick", i)
			}
			g.P[0].BallCD = 0 // keep asking for the ball
			continue
		}
		if i >= botEvery && i%botEvery != botEvery-1 {
			if held == 0 {
				stick = [2]int{in.DX, in.DY}
			} else if stick != [2]int{in.DX, in.DY} {
				t.Fatalf("the stick changed between decisions: %v -> (%d,%d)", stick, in.DX, in.DY)
			}
			held++
		}
	}
	if pressed == 0 || pressed > 8 {
		t.Fatalf("pressed %d times in 8 decisions", pressed)
	}
}

func TestTheBotNoticesAThreatOnlyAfterItsReactionTime(t *testing.T) {
	g := snorlaxFacing()
	b := newBot(snorlax, 1)
	for i := 0; i < botReact+botEvery; i++ { // nothing to see
		b.Decide(g, 1)
	}
	g.Balls = []Ball{{K: ballElectro, Owner: 0, X: 640, Y: 250, DX: 1, HP: 30}}
	for i := 1; i <= botReact+2*botEvery; i++ {
		if in := b.Decide(g, 1); in.Skill {
			if i < botReact {
				t.Fatalf("it jumped %d ticks after the ball appeared, before it could have seen it", i)
			}
			return
		}
	}
	t.Fatal("it never reacted")
}

func TestOnlyPikachuAndSnorlaxCanBePlayedByTheComputer(t *testing.T) {
	for k := 0; k < kinds; k++ {
		if got, want := botCanPlay(k), k == pikachu || k == snorlax; got != want {
			t.Errorf("botCanPlay(%d) = %v, want %v", k, got, want)
		}
	}
}

// ---- whole fights ----

// fight plays two bots to a finish (or the time limit) and says who won (-1 for neither) and how long it took.
func fight(cfg Config, seed uint64, a, b int, limit float64) (winner int, secs float64, g *Game) {
	g = duel(cfg, a, b)
	g.P[0].X, g.P[1].X = 700, 1300 // closer to the middle of the court than duel puts them
	g.Reset()
	bots := [2]*Bot{newBot(a, seed), newBot(b, seed+1000)}
	for t := 0.0; t < limit; t += dt {
		for i, bt := range bots {
			g.P[i].In = bt.Decide(g, i)
		}
		g.Step(dt)
		if w := g.Winner(); w >= 0 {
			return w, t, g
		}
	}
	return -1, limit, g
}

func TestBotsFightToAFinishWithoutGettingStuck(t *testing.T) {
	cfg, err := LoadConfig("config.json") // the numbers the game really runs with
	if err != nil {
		t.Fatal(err)
	}
	pairs := [][2]int{{pikachu, snorlax}, {snorlax, pikachu}, {pikachu, pikachu}, {snorlax, snorlax}}
	for _, pr := range pairs {
		wins, timeouts, total := [2]int{}, 0, 0.0
		const n = 20
		for seed := uint64(1); seed <= n; seed++ {
			w, secs, _ := fight(cfg, seed, pr[0], pr[1], 180)
			if w < 0 {
				timeouts++
				continue
			}
			wins[w]++
			total += secs
		}
		t.Logf("%s vs %s: wins %v, %d timeouts, %.0f s per finished fight", NAMEOF[pr[0]], NAMEOF[pr[1]], wins, timeouts, total/float64(n-timeouts+1))
		if timeouts > n/10 {
			t.Errorf("%v: %d of %d fights were still going after 180 s", pr, timeouts, n)
		}
	}
}

var NAMEOF = [...]string{"Pikachu", "Snorlax"}

// ---- in a room ----

// botRoom is a room with one person (slot idx) in it and no sockets.
func botRoom(idx int) *room {
	r := &room{g: NewGame(DefaultConfig()), winner: -1, pick: [2]int{-1, -1}}
	r.c[idx] = &client{send: make(chan []byte, 8)}
	r.running = true // the tests call step themselves
	return r
}

func TestAComputerOpponentStartsTheMatchAsSoonAsThePersonPicks(t *testing.T) {
	r := botRoom(0)
	if !r.setBot(0, snorlax) {
		t.Fatal("the computer was refused")
	}
	if r.pick[1] != snorlax || !r.present(1) || r.playing() {
		t.Fatalf("picks %v present %v playing %v", r.pick, r.present(1), r.playing())
	}
	if !r.choose(0, pikachu) || !r.playing() {
		t.Fatal("the match did not start when the person picked")
	}
	if r.g.P[0].Kind != pikachu || r.g.P[1].Kind != snorlax {
		t.Fatalf("kinds %d %d", r.g.P[0].Kind, r.g.P[1].Kind)
	}
}

func TestTheComputerPlaysItsSlot(t *testing.T) {
	r := botRoom(0)
	r.setBot(0, pikachu)
	r.choose(0, snorlax)
	x0 := r.g.P[1].X
	now := time.Now()
	for i := 0; i < 60; i++ {
		r.step(now)
	}
	if r.g.P[1].X == x0 && r.g.P[1].Y == r.g.court.Width/2 && len(r.g.Balls) == 0 {
		t.Fatal("the computer did nothing in two seconds")
	}
}

func TestTheComputerCanTakeEitherSlot(t *testing.T) {
	r := botRoom(1)
	if !r.setBot(1, pikachu) || r.bot[0] == nil || r.pick[0] != pikachu {
		t.Fatalf("bot %v pick %v", r.bot, r.pick)
	}
}

func TestTheComputerIsRefusedWhereItCannotPlay(t *testing.T) {
	r := botRoom(0)
	if r.setBot(0, charizard) || r.bot[1] != nil {
		t.Fatal("a character it cannot play was accepted")
	}
	r.c[1] = &client{send: make(chan []byte, 8)}
	if r.setBot(0, pikachu) {
		t.Fatal("it took the place of a person")
	}
	r.c[1] = nil
	r.setTeam(true)
	if r.setBot(0, pikachu) {
		t.Fatal("it joined a team match")
	}
	r.setTeam(false)
	r.setBot(0, pikachu)
	if r.setTeam(true) || r.team {
		t.Fatal("a team match was started with a computer in the room")
	}
	r.choose(0, snorlax)
	if r.setBot(0, snorlax) || r.setBot(0, -1) {
		t.Fatal("the computer was changed in the middle of a match")
	}
}

func TestTheComputerKeepsItsCharacterWhenEveryoneChoosesAgain(t *testing.T) {
	r := botRoom(0)
	r.setBot(0, snorlax)
	r.choose(0, pikachu)
	r.g.P[1].HP = 0
	r.step(time.Now()) // a result
	if !r.repick() {
		t.Fatal("repick refused")
	}
	if r.pick != [2]int{-1, snorlax} || r.playing() {
		t.Fatalf("picks %v playing %v", r.pick, r.playing())
	}
	r.setSize(3)
	if r.pick[1] != snorlax {
		t.Fatalf("a change of team size took the computer's pick: %v", r.pick)
	}
	r.choose(0, snorlax)
	if !r.playing() || r.g.P[0].HP <= 0 || r.g.P[1].HP <= 0 {
		t.Fatal("the next match did not start fresh")
	}
}

func TestTheComputerCanBeSentAway(t *testing.T) {
	r := botRoom(0)
	r.setBot(0, snorlax)
	if !r.setBot(0, -1) || r.present(1) || r.pick[1] != -1 {
		t.Fatalf("bot %v pick %v", r.bot, r.pick)
	}
}

func TestAPersonJoiningTakesTheComputersPlace(t *testing.T) {
	h := newHub(DefaultConfig())
	c0 := &client{send: make(chan []byte, 8)}
	r, idx, ok := h.join("a", modeWeb, c0)
	if !ok || idx != 0 {
		t.Fatal("first join failed")
	}
	r.mu.Lock()
	r.setBot(0, snorlax)
	r.choose(0, pikachu)
	r.mu.Unlock()
	c1 := &client{send: make(chan []byte, 8)}
	r2, idx2, ok := h.join("a", modeWeb, c1)
	if !ok || r2 != r || idx2 != 1 {
		t.Fatalf("second person: room same %v slot %d ok %v", r2 == r, idx2, ok)
	}
	if r.bot[1] != nil || r.pick[1] != -1 || r.playing() {
		t.Fatalf("the computer stayed: bot %v pick %v playing %v", r.bot, r.pick, r.playing())
	}
	if _, _, ok := h.join("a", modeWeb, &client{send: make(chan []byte, 8)}); ok {
		t.Fatal("a third person got in")
	}
}

func TestTheRoomIsGoneWhenTheOnlyPersonLeaves(t *testing.T) {
	h := newHub(DefaultConfig())
	r, idx, _ := h.join("a", modeWeb, &client{send: make(chan []byte, 8)})
	r.mu.Lock()
	r.setBot(idx, snorlax)
	r.choose(idx, pikachu)
	r.mu.Unlock()
	h.leave("a", r, idx)
	if _, ok := h.rooms["a"]; ok {
		t.Fatal("a room with only a computer in it stayed")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.playing() {
		t.Fatal("the match would carry on with nobody in the room")
	}
}

func TestTheLobbyTellsWhoTheComputerIs(t *testing.T) {
	r := botRoom(0)
	r.setBot(0, snorlax)
	r.broadcastLobby()
	var m struct {
		Here  [2]bool `json:"here"`
		Bot   [2]int  `json:"bot"`
		Picks [2]int  `json:"picks"`
	}
	if err := json.Unmarshal(<-r.c[0].send, &m); err != nil {
		t.Fatal(err)
	}
	if m.Here != [2]bool{true, true} || m.Bot != [2]int{-1, snorlax} || m.Picks != [2]int{-1, snorlax} {
		t.Fatalf("lobby %+v", m)
	}
}
