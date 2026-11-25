package main

import (
	"math"
	"testing"
)

func botLook(t *testing.T, g *Game, kind int) (*Bot, *bctx) {
	t.Helper()
	b := newBot(kind, 1)
	for i := 0; i < 8; i++ {
		b.observe(g, 0)
	}
	return b, b.look(g, 0)
}

// cometDuel is a bot of `kind` in slot 0 and a Machamp in slot 1 that has `gap` pixels between them and is in the middle of a Comet Punch, with fists lying around the bot.
func cometDuel(t *testing.T, kind int, gap float64) *Game {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		t.Fatal(err)
	}
	g := duel(cfg, kind, machamp)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 800, 500, 800+gap, 500
	g.P[1].CometT = cfg.Machamp.CometPunch.Duration
	for k := 0; k < 6; k++ { // the fists thrown so far, ahead of the Machamp (they point at the bot)
		g.Balls = append(g.Balls, Ball{K: ballFist, Owner: 1, Side: 2, DX: -1, X: 800 + gap - 40 - float64(k)*15, Y: 500 + float64(k%3-1)*20, HP: cfg.Machamp.CometPunch.HP, Life: cfg.Machamp.CometPunch.FistDuration})
	}
	return g
}

func TestTheBotDoesNotDodgeEachFistOfACometPunchButWalksAwayFromThePuncher(t *testing.T) {
	g := cometDuel(t, pikachu, 250)
	_, c := botLook(t, g, pikachu)
	if len(c.hazards()) != 0 {
		t.Fatalf("%d hazards: the fists hang where they are, they are not projectiles", len(c.hazards()))
	}
	if c.th.kind != thChase || c.th.ax >= 0 || c.th.ay != 0 {
		t.Fatalf("threat %+v, want to walk straight away from the Machamp (to the left)", c.th)
	}
}

func TestTheBotUsesItsMovementSkillWhenWalkingAwayCannotOutrunACometPunch(t *testing.T) {
	g := cometDuel(t, pikachu, 120) // it runs faster than the bot walks, and is already close
	b, c := botLook(t, g, pikachu)
	if !c.cant {
		t.Fatal("walking away was thought enough")
	}
	if in, ok := b.escapeBySkill(c); !ok || !in.Skill || in.DX >= 0 {
		t.Fatalf("escape %+v (%v), want the Volt Tackle to the left", in, ok)
	}
}

func TestTheBotIgnoresACometPunchThatCannotReachIt(t *testing.T) {
	g := cometDuel(t, pikachu, 1500)
	g.Balls = nil
	if _, c := botLook(t, g, pikachu); c.th.kind != thNone {
		t.Fatalf("threat %+v from a Comet Punch a court away", c.th)
	}
}

// ringOfPillars walls the point (x, y) in with a foe's pillars.
func ringOfPillars(g *Game, owner int, x, y, ring float64, n int) {
	fb := g.cfg.Vulpix.FreezeBeam
	for k := 0; k < n; k++ {
		a := 2 * math.Pi * float64(k) / float64(n)
		g.Balls = append(g.Balls, Ball{K: ballPillar, Owner: owner, X: x + ring*math.Cos(a), Y: y + ring*math.Sin(a), HP: fb.PillarHP, Life: 60})
	}
}

func alivePillars(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballPillar && b.HP > 0 {
			n++
		}
	}
	return n
}

func TestABotShutInByPillarsBreaksOutOfThem(t *testing.T) {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range botKinds {
		g := duel(cfg, kind, vulpix)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 500, 1400, 500
		g.P[0].Charge, g.P[1].Charge = 0, 0
		ringOfPillars(g, 1, 900, 500, 70, 14)
		start := alivePillars(g)
		b := newBot(kind, 3)
		out := false
		for k := 0; k < 30*20 && !out; k++ {
			g.P[0].In = b.Decide(g, 0)
			g.P[1].In = Input{}
			g.Step(dt)
			out = alivePillars(g) < start || (g.P[0].X-900)*(g.P[0].X-900)+(g.P[0].Y-500)*(g.P[0].Y-500) > 200*200
		}
		if !out {
			t.Errorf("%s: still shut in by %d pillars after 20 s", NAMEOF[kind], alivePillars(g))
		}
	}
}

func TestABotWithAWayOutDoesNotBreakAnything(t *testing.T) {
	cfg, _ := LoadConfig("config.json")
	g := duel(cfg, pikachu, vulpix)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 500, 1400, 500
	ringOfPillars(g, 1, 900, 500, 70, 3) // three pillars: plenty of room between them
	if _, c := botLook(t, g, pikachu); c != nil {
		if _, ok := c.walledIn(); ok {
			t.Fatal("it thinks it is shut in with three pillars around it")
		}
	}
}
