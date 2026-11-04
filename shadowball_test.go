package main

import (
	"math"
	"net/http/httptest"
	"testing"
)

// chargeDuel is Gengar (slot 0, 300,250) against a Snorlax that cannot walk off (slot 1, 300+d,250), with the charged Shadow Ball.
func chargeDuel(cfg Config, d float64) *Game {
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+d, 250
	return g
}

func TestShadowBallRootsGengarWhileItChargesThenAStraightBallLeavesTowardTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	sb := cfg.Gengar.ShadowBall
	g := chargeDuel(cfg, 500)
	gpress(g, Input{SBall: true})
	n := 1
	for ; g.P[0].SBallT > 0; n++ {
		if len(g.Balls) != 0 {
			t.Fatalf("a ball left after %d ticks, before the %v s charge ended", n, sb.ChargeTime)
		}
		g.P[0].In = Input{DX: 1, Skill: true, GFire: true} // walking and other skills do nothing meanwhile
		x := g.P[0].X
		g.Step(dt)
		if g.P[0].X != x || len(g.Ghosts) != 0 || g.P[0].SneakT != 0 {
			t.Fatalf("tick %d: moved or used another skill while charging", n)
		}
	}
	if want := int(math.Ceil(sb.ChargeTime / dt)); n-1 != want { // n counts the press too
		t.Fatalf("charge took %d ticks after the press, want %d", n-1, want)
	}
	if len(g.Balls) != 1 || g.Balls[0].K != ballShadow || g.Balls[0].HP != sb.HP || g.Balls[0].DX != 1 || g.Balls[0].DY != 0 {
		t.Fatalf("balls = %+v, want one straight at the opponent with %d HP", g.Balls, sb.HP)
	}
	g.P[0].In = Input{DX: 1}
	x := g.P[0].X
	g.Step(dt)
	if g.P[0].X <= x {
		t.Fatal("still rooted after the ball left")
	}
}

func TestShadowBallKeepsTheAimChosenAtTheCastAndOtherwiseFollowsTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	g := chargeDuel(cfg, 500)
	gpress(g, Input{SBall: true, CastX: 0, CastY: -2})
	g.P[1].Y = 450 // the opponent moves: the chosen aim does not
	steps(g, int(cfg.Gengar.ShadowBall.ChargeTime/dt)+2)
	if b := g.Balls[0]; b.DX != 0 || b.DY != -1 {
		t.Fatalf("ball direction = %v,%v, want 0,-1 as cast", b.DX, b.DY)
	}
	g = chargeDuel(cfg, 500)
	gpress(g, Input{SBall: true}) // no aim: it follows
	g.P[1].X, g.P[1].Y = 300, 650
	steps(g, int(cfg.Gengar.ShadowBall.ChargeTime/dt)+2)
	if b := g.Balls[0]; math.Abs(b.DX) > 1e-9 || math.Abs(b.DY-1) > 1e-9 {
		t.Fatalf("ball direction = %v,%v, want it turned to the opponent below", b.DX, b.DY)
	}
}

func TestShadowBallBurstsOnTheOpponentHurtsOnceAndDragsItToTheMiddleAtTheGravity(t *testing.T) {
	cfg := DefaultConfig()
	sb := &cfg.Gengar.ShadowBall
	sb.GhostFireCount = 0 // only the blast and the drag
	g := chargeDuel(cfg, 500)
	g.P[1].Kind = dummy // it cannot be dragged: see the next test
	g.P[1].Kind = snorlax
	hp := g.P[1].HP
	gpress(g, Input{SBall: true})
	var burst *Event
	for k := 0; k < 200 && burst == nil; k++ {
		for _, e := range g.Step(dt) {
			if e.T == "gslam" {
				e := e
				burst = &e
			}
		}
	}
	if burst == nil || len(g.Balls) != 0 {
		t.Fatal("the ball never burst on the opponent")
	}
	if g.P[1].HP != hp-sb.Damage {
		t.Fatalf("HP = %d, want %d: hurt once by the blast", g.P[1].HP, hp-sb.Damage)
	}
	if len(g.Wells) != 1 {
		t.Fatalf("wells = %+v, want one", g.Wells)
	}
	cx, cy := g.Wells[0].X, g.Wells[0].Y
	before := math.Hypot(g.P[1].X-cx, g.P[1].Y-cy)
	g.Step(dt)
	after := math.Hypot(g.P[1].X-cx, g.P[1].Y-cy)
	if got := before - after; math.Abs(got-math.Min(sb.Gravity*dt, before)) > 1e-6 {
		t.Fatalf("dragged %v in a tick, want gravity %v x dt", got, sb.Gravity)
	}
	steps(g, int(sb.PullDuration/dt)+3)
	if len(g.Wells) != 0 {
		t.Fatal("the drag outlasted pull_duration")
	}
	if d := math.Hypot(g.P[1].X-cx, g.P[1].Y-cy); d > 1e-6 {
		t.Fatalf("ended %v from the middle, want right on it, never past it", d)
	}
	if g.P[1].HP != hp-sb.Damage {
		t.Fatalf("HP = %d: hurt more than once", g.P[1].HP)
	}
}

func TestShadowBallGravityZeroDoesNotDragAndANearbyOpponentIsDraggedFromAFar(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.ShadowBall.Gravity = 0
	g := chargeDuel(cfg, 500)
	g.Balls = []Ball{{K: ballShadow, Owner: 0, X: 600, Y: 250, DX: 1, HP: 0}} // shot down at once: it bursts where it hangs
	g.Step(dt)
	if len(g.Wells) != 0 {
		t.Fatal("a well with no gravity")
	}
	cfg = DefaultConfig()
	sb := cfg.Gengar.ShadowBall
	g = chargeDuel(cfg, 500)
	g.Balls = []Ball{{K: ballShadow, Owner: 0, X: 800 - sb.SlamRadius + 10, Y: 250, DX: 1, HP: 0}} // bursts a bit short of the opponent, who is inside the reach
	g.Step(dt)
	x := g.P[1].X
	g.Step(dt)
	if g.P[1].X >= x {
		t.Fatalf("x %v -> %v: not dragged toward the burst", x, g.P[1].X)
	}
	g = chargeDuel(cfg, 700) // out of the blast's reach
	g.Balls = []Ball{{K: ballShadow, Owner: 0, X: 600, Y: 250, DX: 1, HP: 0}}
	g.Step(dt)
	x = g.P[1].X
	g.Step(dt)
	if g.P[1].X != x {
		t.Fatal("dragged from beyond the blast's reach")
	}
}

func TestShadowBallHasHPAndIsShotDownEarlyByOtherProjectiles(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Charizard.FireBreath.Damage = 10 // what a fire ball takes off what it runs into
	sb := cfg.Gengar.ShadowBall
	g := chargeDuel(cfg, 700)
	g.Balls = []Ball{
		{K: ballShadow, Owner: 0, X: 400, Y: 250, DX: 1, HP: sb.HP},
		{K: ballFire, Owner: 1, X: 440, Y: 250, DX: -1, HP: 20, Left: 500},
	}
	g.Step(dt)
	var orb *Ball
	for i := range g.Balls {
		if g.Balls[i].K == ballShadow {
			orb = &g.Balls[i]
		}
	}
	if orb == nil || orb.HP != sb.HP-cfg.Charizard.FireBreath.Damage {
		t.Fatalf("orb = %+v, want it worn down by the ball it met", orb)
	}
	g = chargeDuel(cfg, 700) // worn right down by what it meets: it stays one step with no HP, then bursts where it is
	g.Balls = []Ball{
		{K: ballShadow, Owner: 0, X: 400, Y: 250, DX: 1, HP: 5},
		{K: ballFire, Owner: 1, X: 440, Y: 250, DX: -1, HP: 20, Left: 500},
	}
	g.Step(dt)
	if len(g.Wells) != 0 || g.Balls[0].K != ballShadow || g.Balls[0].HP > 0 {
		t.Fatalf("balls %+v wells %+v: a shot-down orb should wait one step", g.Balls, g.Wells)
	}
	g.Step(dt)
	for _, b := range g.Balls {
		if b.K == ballShadow {
			t.Fatalf("a shot-down orb is still flying: %+v", b)
		}
	}
	if len(g.Wells) != 1 {
		t.Fatalf("wells = %+v: a shot-down orb bursts where it is", g.Wells)
	}
}

func TestShadowBallChargeIsLostWhenGengarIsStunnedAndTheCooldownStillRuns(t *testing.T) {
	cfg := DefaultConfig()
	g := chargeDuel(cfg, 500)
	gpress(g, Input{SBall: true})
	g.P[0].StunT = 1
	steps(g, int(cfg.Gengar.ShadowBall.ChargeTime/dt)+3)
	if len(g.Balls) != 0 || g.P[0].SBallT != 0 {
		t.Fatalf("balls %+v charge %v: a stunned Gengar still threw it", g.Balls, g.P[0].SBallT)
	}
	if g.P[0].SBallCD == 0 {
		t.Fatal("no cooldown after the cast")
	}
}

func TestShadowBallModeAndItsNumbersAreValidated(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"mode":    func(c *Config) { c.Gengar.ShadowBall.Mode = "lob" },
		"empty":   func(c *Config) { c.Gengar.ShadowBall.Mode = "" },
		"charge":  func(c *Config) { c.Gengar.ShadowBall.ChargeTime = -1 },
		"radius":  func(c *Config) { c.Gengar.ShadowBall.Radius = 0 },
		"hp":      func(c *Config) { c.Gengar.ShadowBall.HP = 0 },
		"mass":    func(c *Config) { c.Gengar.ShadowBall.Mass = -1 },
		"gravity": func(c *Config) { c.Gengar.ShadowBall.Gravity = -1 },
		"pull":    func(c *Config) { c.Gengar.ShadowBall.PullDuration = -1 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := DefaultConfig()
	c.Gengar.ShadowBall.Mode = shadowThrow
	if err := c.validate(); err != nil {
		t.Errorf("throw mode refused: %v", err)
	}
}

func TestTheCharacterOfAShadowBallTravelsToThePage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.ShadowBall.PullDuration, cfg.Gengar.ShadowBall.ChargeTime = 1.7, 0.6
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	m := next(t, dialPlay(t, srv, "dummy"), "hi")["gengar"].(map[string]any)
	if m["charge"] != 1.0 || m["ctime"] != 0.6 || m["pull"] != 1.7 || m["orb"] != cfg.Gengar.ShadowBall.Radius {
		t.Fatalf("gengar = %v", m)
	}
}

func TestAShadowBallAndAnElectroBallThatMeetBothBurstWhateverHPTheyHave(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.ContactDamage = 0 // it does not even wear the other down: meeting is enough
	g := chargeDuel(cfg, 900)
	g.Balls = []Ball{
		{K: ballShadow, Owner: 0, X: 400, Y: 250, DX: 1, HP: cfg.Gengar.ShadowBall.HP},
		{K: ballElectro, Owner: 1, X: 460, Y: 250, DX: -1, HP: cfg.Pikachu.ElectroBall.HP},
	}
	g.Step(dt)
	if len(g.Balls) != 2 || g.Balls[0].HP > 0 || g.Balls[1].HP > 0 {
		t.Fatalf("balls %+v: both should be set off", g.Balls)
	}
	seen := map[string]bool{}
	for _, e := range g.Step(dt) {
		seen[e.T] = true
	}
	if len(g.Balls) != 0 || !seen["explode"] || !seen["gslam"] {
		t.Fatalf("balls %+v events %v: want both to burst", g.Balls, seen)
	}
	// the same owner's balls pass through each other, and a ball of another kind only wears it down
	g = chargeDuel(cfg, 900)
	cfg.Charizard.FireBreath.Damage = 3
	g.cfg = cfg
	g.Balls = []Ball{
		{K: ballShadow, Owner: 0, X: 400, Y: 250, DX: 1, HP: cfg.Gengar.ShadowBall.HP},
		{K: ballFire, Owner: 1, X: 440, Y: 250, DX: -1, HP: 20, Left: 500},
	}
	g.Step(dt)
	if g.Balls[0].K != ballShadow || g.Balls[0].HP != cfg.Gengar.ShadowBall.HP-3 {
		t.Fatalf("balls %+v: a fire ball should only wear the orb down by its damage", g.Balls)
	}
}
