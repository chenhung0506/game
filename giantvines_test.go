package main

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// gvGame is Chikorita (slot 0, at 300,250) against a Snorlax `d` ahead of it, with a full charge and nothing in the way.
func gvGame(cfg Config, d float64) *Game {
	g := duel(cfg, chikorita, snorlax)
	g.P[1].X, g.P[1].Y = 300+d, 250
	g.P[1].HP = 100000
	g.P[0].Charge = cfg.Chikorita.Ultimate.Max
	return g
}

func gvCast(g *Game) { gpress(g, Input{Ult: true}) }

func vinesOf(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballGiantVine {
			n++
		}
	}
	return
}

func headOf(g *Game) *Ball {
	for i := range g.Balls {
		if g.Balls[i].K == ballGiantHead {
			return &g.Balls[i]
		}
	}
	return nil
}

func TestGiantVinesNeedAFullChargeAndEmptyIt(t *testing.T) {
	cfg := DefaultConfig()
	g := gvGame(cfg, 600)
	g.P[0].Charge = cfg.Chikorita.Ultimate.Max - 1
	gvCast(g)
	steps(g, 3)
	if headOf(g) != nil || g.giantOut(0) {
		t.Fatal("the head ran out with the charge not full")
	}
	g.P[0].Charge = cfg.Chikorita.Ultimate.Max
	gvCast(g)
	if headOf(g) == nil || g.P[0].Charge >= 1 {
		t.Fatalf("head %v, charge %v: the press did not start it and empty the charge", headOf(g), g.P[0].Charge)
	}
}

func TestTheGiantVinesHeadRunsAtTheFoeAtTheFollowSpeedAndSurfacesUnderItWhereItTouches(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.FollowSpeed, gv.HeadRadius = 300, 30
	g := gvGame(cfg, 600)
	gvCast(g)
	h := headOf(g)
	if h == nil {
		t.Fatal("no head")
	}
	x0 := h.X
	steps(g, 3)
	if got := headOf(g).X - x0; got < 300*3*dt*0.95 || got > 300*3*dt*1.05 {
		t.Fatalf("the head ran %v in 3 ticks, want about %v", got, 300*3*dt)
	}
	for n := 0; n < 4*tickRate && headOf(g) != nil; n++ {
		g.Step(dt)
	}
	if headOf(g) != nil || vinesOf(g) != gv.Count {
		t.Fatalf("head %v, %d vines: it should have surfaced as %d vines at the foe", headOf(g), vinesOf(g), gv.Count)
	}
	for _, b := range g.Balls {
		if b.K != ballGiantVine {
			continue
		}
		if d := dist(&Player{X: b.X, Y: b.Y}, &g.P[1]); d < gv.RingRadius-1 || d > gv.RingRadius+1 {
			t.Fatalf("a vine stands %v from the foe, want the ring of %v", d, gv.RingRadius)
		}
	}
}

func TestTheGiantVinesHeadSurfacesWhereItIsWhenItsTimeIsUp(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.FollowSpeed, gv.TerminalTime = 100, 1
	g := gvGame(cfg, 1500)
	gvCast(g)
	steps(g, int(1*tickRate)+2)
	if headOf(g) != nil || vinesOf(g) != gv.Count {
		t.Fatalf("head %v, %d vines after terminal_time", headOf(g), vinesOf(g))
	}
	for _, b := range g.Balls {
		if d := dist(&Player{X: b.X, Y: b.Y}, &Player{X: 300 + 100, Y: 250}); d > gv.RingRadius+30 {
			t.Fatalf("a vine is %v from where the head gave up (about x=400), more than the ring", d)
		}
	}
}

func TestEachGiantVineLocksOnTheFoeAsItComesOutThenHitsOnceItLandsAndSlows(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.Count, gv.Interval, gv.ChargeDuration, gv.SwingDuration, gv.Damage = 4, 0.5, 0.4, 0.2, 7
	g := gvGame(cfg, 100) // the head is on it at once
	hp := g.P[1].HP
	gvCast(g)
	var rose, slammed int
	for n := 0; n < 8*tickRate; n++ {
		for _, e := range g.Step(dt) {
			switch e.T {
			case "gvrise":
				rose++
			case "gvslam":
				slammed++
			}
		}
		if slammed == 1 && g.P[1].HP == hp {
			t.Fatal("the first vine landed and did not hurt a foe standing at the middle of the ring")
		}
	}
	if rose != 4 || slammed != 4 {
		t.Fatalf("%d vines came out and %d landed, want 4 and 4", rose, slammed)
	}
	if lost := hp - g.P[1].HP; lost != 4*7 {
		t.Fatalf("the foe lost %d HP, want %d (7 for each of 4 vines)", lost, 4*7)
	}
}

func TestGiantVinesHitOnlyWhatIsInTheirShape(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.Count, gv.TrunkLength, gv.TrunkWidth, gv.RingRadius = 1, 100, 20, 200 // a single short vine that cannot reach the middle of its ring
	g := gvGame(cfg, 100)
	hp := g.P[1].HP
	gvCast(g)
	steps(g, 8*tickRate)
	if g.P[1].HP != hp {
		t.Fatalf("a vine 100 long hit a foe 200 away: lost %d", hp-g.P[1].HP)
	}
}

func TestGiantVinesSlowWhatTheyHitAndNeverStunIt(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.SlowDuration, gv.Slow = 2, 0.6
	g := gvGame(cfg, 100)
	gvCast(g)
	slowed := false
	for n := 0; n < 8*tickRate; n++ {
		g.Step(dt)
		if g.P[1].StunT != 0 {
			t.Fatal("a giant vine stunned the foe")
		}
		if g.P[1].SlowT > 0 && g.P[1].SlowK == 0.6 {
			slowed = true
		}
	}
	if !slowed {
		t.Fatal("the foe was never slowed by 0.6")
	}
	// and the walking is what it says
	g = gvGame(cfg, 600)
	g.P[1].SlowT, g.P[1].SlowK = 5, 0.6
	x0 := g.P[1].X
	g.P[1].In = Input{DX: 1}
	steps(g, tickRate)
	if got, want := g.P[1].X-x0, cfg.Snorlax.WalkSpeed*0.4; got < want*0.95 || got > want*1.05 {
		t.Fatalf("a foe slowed by 0.6 walked %v in a second, want about %v", got, want)
	}
}

func TestGiantVinesWithNoSlowDoNotSlow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.GiantVines.SlowDuration = 0
	g := gvGame(cfg, 100)
	gvCast(g)
	for n := 0; n < 8*tickRate; n++ {
		g.Step(dt)
		if g.P[1].SlowT != 0 || g.P[1].StunT != 0 {
			t.Fatal("the foe was slowed or stunned with slow_duration 0")
		}
	}
	if g.P[1].HP == 100000 {
		t.Fatal("and it was not hurt either")
	}
}

func TestAGiantVineIsARootThickAtTheFootAndThinAtTheTip(t *testing.T) {
	// 300 long, 100 wide at the foot and 20 at the tip, along +x from the origin; a body of radius 5
	hit := func(x, y float64) bool { return vineHits(0, 0, 1, 0, x, y, 5, 300, 100, 20) }
	for _, c := range []struct {
		x, y float64
		want bool
	}{
		{10, 0, true}, {10, 45, true}, // wide at the foot
		{290, 0, true}, {290, 14, true},
		{290, 40, false}, // the same offset that is well inside near the foot is outside near the tip
		{150, 30, true}, {150, 50, false},
		{-3, 0, true}, {-20, 0, false}, // a little behind the foot (the body's radius), not far
		{303, 0, true}, {320, 0, false}, // and past the tip
	} {
		if got := hit(c.x, c.y); got != c.want {
			t.Errorf("body at (%v, %v): hit %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestTheGiantVinesComeOutInAnyOrderNotRoundTheRing(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.Count, gv.Interval = 10, 0.5
	orders := map[string]bool{}
	for seed := uint64(1); seed <= 8; seed++ {
		g := gvGame(cfg, 100)
		g.rng = rand.New(rand.NewPCG(seed, 9))
		gvCast(g)
		for headOf(g) != nil {
			g.Step(dt)
		}
		var delays []float64
		seen := map[float64]bool{}
		for _, b := range g.Balls { // in ring order
			if b.K == ballGiantVine {
				delays = append(delays, b.Delay)
				if seen[b.Delay] {
					t.Fatalf("two vines come out together at %v", b.Delay)
				}
				seen[b.Delay] = true
			}
		}
		if len(delays) != 10 {
			t.Fatalf("%d vines", len(delays))
		}
		orders[fmt.Sprint(delays)] = true
		if seen[0] == false || !seen[4.5] {
			t.Fatalf("the turns are not 0, 0.5, ... 4.5: %v", delays)
		}
	}
	if len(orders) < 4 {
		t.Fatalf("only %d different orders in 8 casts", len(orders))
	}
}

func TestChikoritaCannotMoveOrUseASkillUntilTheLastVineHasLanded(t *testing.T) {
	cfg := DefaultConfig()
	gv := &cfg.Chikorita.GiantVines
	gv.Count, gv.Interval, gv.ChargeDuration, gv.SwingDuration, gv.Linger = 3, 0.5, 0.4, 0.2, 5
	g := gvGame(cfg, 100)
	gvCast(g)
	x0 := g.P[0].X
	landed := 0
	for n := 0; n < 10*tickRate && landed < 3; n++ {
		g.P[0].In = Input{DX: -1, Razor: true, Vine: true, Skill: true}
		for _, e := range g.Step(dt) {
			if e.T == "gvslam" {
				landed++
			}
		}
		if landed < 3 && (g.P[0].X != x0 || g.P[0].LeafPhase != leafNone || g.P[0].RazorCD != 0 || g.P[0].VineCD != 0) {
			t.Fatalf("it moved to %v or used a skill (leaf %d, razor cd %v, vine cd %v) before the last vine landed", g.P[0].X, g.P[0].LeafPhase, g.P[0].RazorCD, g.P[0].VineCD)
		}
	}
	if landed != 3 {
		t.Fatalf("%d vines landed", landed)
	}
	g.P[0].In = Input{DX: -1}
	steps(g, 3) // the vines still lie there (linger 5), but the skill is over
	if g.P[0].X >= x0 {
		t.Fatalf("still rooted after the last vine landed: x %v", g.P[0].X)
	}
}

func TestAFoeOutsideTheVinesHeightBandIsNotHit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Chikorita.GiantVines.MinHeight = 40 // a foe on the ground is below it
	g := gvGame(cfg, 100)
	gvCast(g)
	steps(g, 8*tickRate)
	if g.P[1].HP != 100000 {
		t.Fatal("a vine hit a foe outside its band")
	}
}

func TestGiantVinesConfigIsValidated(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"no vines":      func(c *Config) { c.Chikorita.GiantVines.Count = 0 },
		"too many":      func(c *Config) { c.Chikorita.GiantVines.Count = maxGiantVines + 1 },
		"negative slow": func(c *Config) { c.Chikorita.GiantVines.SlowDuration = -1 },
		"slow over 1":   func(c *Config) { c.Chikorita.GiantVines.Slow = 1.5 },
		"tip wider":     func(c *Config) { c.Chikorita.GiantVines.TipWidth = c.Chikorita.GiantVines.TrunkWidth + 1 },
		"no speed":      func(c *Config) { c.Chikorita.GiantVines.FollowSpeed = 0 },
		"no time":       func(c *Config) { c.Chikorita.GiantVines.TerminalTime = 0 },
		"no charge":     func(c *Config) { c.Chikorita.Ultimate.Max = 0 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := DefaultConfig()
	c.Chikorita.GiantVines.SlowDuration, c.Chikorita.GiantVines.Interval, c.Chikorita.GiantVines.ChargeDuration = 0, 0, 0
	if err := c.validate(); err != nil {
		t.Errorf("zeros that are allowed were refused: %v", err)
	}
}

func TestTheChikoritaBotCastsGiantVinesOnlyWhenChargedAndTheFoeIsNear(t *testing.T) {
	cfg := liveConfig(t)
	for _, c := range []struct {
		charged bool
		d       float64
		want    bool
	}{{true, 400, true}, {false, 400, false}, {true, 1800, false}} {
		g := gvGame(cfg, c.d)
		if !c.charged {
			g.P[0].Charge = 0
		}
		g.P[0].CometCD = 0
		b := newBot(chikorita, 1)
		cast := false
		for n := 0; n < 3*tickRate && !cast; n++ {
			in := b.Decide(g, 0)
			cast = in.Ult
			g.P[0].In = Input{}
			g.Step(dt)
		}
		if cast != c.want {
			t.Errorf("charged %v at %v: cast %v, want %v", c.charged, c.d, cast, c.want)
		}
	}
}

func TestABotWalksOutOfTheShapeOfAGiantVine(t *testing.T) {
	cfg := liveConfig(t)
	gv := cfg.Chikorita.GiantVines
	g := duel(cfg, snorlax, chikorita)
	g.P[0].X, g.P[0].Y = 700, 500
	g.P[1].X, g.P[1].Y = 500, 500
	g.Balls = append(g.Balls, Ball{K: ballGiantVine, Owner: 1, Side: 2, X: 500, Y: 500, DX: 1, Phase: vineUp, Life: gv.ChargeDuration}) // standing up, aimed along the line at the bot
	b := newBot(snorlax, 1)
	hits := func(p *Player) bool {
		return vineHits(500, 500, 1, 0, p.X, p.Y, g.radius(0), gv.TrunkLength, gv.TrunkWidth, gv.TipWidth)
	}
	if !hits(&g.P[0]) {
		t.Fatal("the scene is wrong: the bot is not in the shape")
	}
	for n := 0; n < 3*tickRate && hits(&g.P[0]); n++ {
		g.P[0].In = b.Decide(g, 0)
		g.Step(dt)
		g.Balls[0].Life = gv.ChargeDuration // hold the vine up: this is about what the bot does while it is
	}
	if hits(&g.P[0]) {
		t.Fatal("the bot stayed in the shape of the vine")
	}
}
