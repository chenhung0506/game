package main

import (
	"math"
	"net/http/httptest"
	"testing"
)

func glaGame(cfg Config) *Game {
	g := duel(cfg, vulpix, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 500, 450, 1500, 450
	g.P[0].Charge = cfg.Vulpix.Ultimate.Max
	g.P[1].HP = 100000
	return g
}

func TestGlaciateNeedsAFullChargeEmptiesItAndRootsVulpixWhileItGathers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Vulpix.Glaciate.RingDuration = 0.5
	g := glaGame(cfg)
	g.P[0].Charge = cfg.Vulpix.Ultimate.Max - 1
	gpress(g, Input{Ult: true})
	if g.P[0].UltT != 0 {
		t.Fatal("it started with the charge not full")
	}
	g.P[0].Charge = cfg.Vulpix.Ultimate.Max
	gpress(g, Input{Ult: true})
	if g.P[0].Charge > 1 || g.P[0].UltT <= 0 { // (a tick of the charge filling again is not the press failing to empty it)
		t.Fatalf("charge %v UltT %v: the press should empty the charge and begin the gathering", g.P[0].Charge, g.P[0].UltT)
	}
	x0 := g.P[0].X
	g.P[0].In = Input{DX: 1}
	steps(g, 5)
	if g.P[0].X != x0 {
		t.Fatal("Vulpix walked while it gathered")
	}
	g.P[0].In = Input{}
	if len(g.Casts) != 0 {
		t.Fatal("the icicles began before the gathering was over")
	}
	steps(g, secs(0.5))
	if len(g.Casts) != 1 && len(g.Icicles) == 0 {
		t.Fatal("the gathering ended and nothing was called")
	}
}

func TestGlaciateCallsCountIciclesEvenlyIntoACircleAheadOfVulpixAndEachLeavesAnIcicleStanding(t *testing.T) {
	cfg := DefaultConfig()
	gl := &cfg.Vulpix.Glaciate
	gl.RingDuration, gl.Duration, gl.Count, gl.Radius, gl.Distance = 0.3, 2, 10, 200, 300
	gl.Icicle.FallDuration, gl.Icicle.Radius, gl.Icicle.HP, gl.Icicle.Scale, gl.Icicle.Duration = 0.5, 25, 17, 1.5, 20
	g := glaGame(cfg)
	gpress(g, Input{Ult: true})
	var cast *Event
	var slams []Event
	var firstSlam, lastSlam int
	for n := 1; n < secs(4); n++ {
		for _, e := range g.Step(dt) {
			switch e.T {
			case "glacast":
				e := e
				cast = &e
			case "glaslam":
				slams = append(slams, e)
				if firstSlam == 0 {
					firstSlam = n
				}
				lastSlam = n
			}
		}
	}
	if cast == nil || cast.X != 800 || cast.Y != 450 || cast.R != gl.Radius {
		t.Fatalf("glacast %+v: want the circle's middle 300 (distance) from Vulpix toward the foe, which is further (800,450), radius %v", cast, gl.Radius)
	}
	if len(slams) != gl.Count {
		t.Fatalf("%d icicles landed, want %d", len(slams), gl.Count)
	}
	for _, s := range slams {
		if d := math.Hypot(s.X-cast.X, s.Y-cast.Y); d > gl.Radius+1e-6 {
			t.Fatalf("an icicle landed %v from the middle, outside the radius %v", d, gl.Radius)
		}
	}
	// the first one is called as the gathering ends, the last one (count-1) gaps later; each takes fall_duration to come down
	if span := float64(lastSlam-firstSlam) * dt; math.Abs(span-gl.Duration*float64(gl.Count-1)/float64(gl.Count)) > 3*dt {
		t.Fatalf("the icicles landed over %.2fs, want about %.2fs: they are not called evenly over duration", span, gl.Duration*float64(gl.Count-1)/float64(gl.Count))
	}
	standing := 0
	for _, b := range g.Balls {
		if b.K == ballPillar {
			standing++
			if b.Rad != gl.Icicle.Radius || b.Scale != gl.Icicle.Scale || b.HP != gl.Icicle.HP || b.Owner != 0 {
				t.Fatalf("an icicle %+v does not carry its own radius, scale and HP", b)
			}
		}
	}
	if standing != gl.Count {
		t.Fatalf("%d icicles standing, want %d", standing, gl.Count)
	}
}

func TestAnIcicleHurtsAndSlowsWhoeverIsWithinDamageRadiusWhereItLandsAndBlocksThemAfterwards(t *testing.T) {
	cfg := DefaultConfig()
	gl := &cfg.Vulpix.Glaciate
	gl.RingDuration, gl.Duration, gl.Count, gl.Radius, gl.Distance = 0, 1, 1, 1, 300 // one icicle, onto the foe
	gl.Icicle.FallDuration, gl.Icicle.Damage, gl.Icicle.DamageRadius, gl.Icicle.Slow, gl.Icicle.SlowDuration = 0.3, 11, 90, 0.6, 2
	gl.Icicle.Radius = 40
	g := glaGame(cfg)
	g.P[1].X, g.P[1].Y = 800, 450 // exactly where the circle is
	hp0 := g.P[1].HP
	gpress(g, Input{Ult: true})
	steps(g, secs(0.3)+3)
	if g.P[1].HP != hp0-gl.Icicle.Damage {
		t.Fatalf("foe HP %d -> %d, want %d taken", hp0, g.P[1].HP, gl.Icicle.Damage)
	}
	if g.P[1].SlowT <= 0 || g.P[1].SlowK != gl.Icicle.Slow {
		t.Fatalf("slow %v for %v, want %v", g.P[1].SlowK, g.P[1].SlowT, gl.Icicle.Slow)
	}
	var ic *Ball
	for i := range g.Balls {
		if g.Balls[i].K == ballPillar {
			ic = &g.Balls[i]
		}
	}
	if ic == nil {
		t.Fatal("no icicle standing where it landed")
	}
	steps(g, 3)
	if d := math.Hypot(g.P[1].X-ic.X, g.P[1].Y-ic.Y); d < gl.Icicle.Radius+g.radius(1)-1e-6 {
		t.Fatalf("the foe is %v from the icicle: it should be pushed out to %v", d, gl.Icicle.Radius+g.radius(1))
	}

	// A foe out of damage_radius is not hurt, though the icicle still stands.
	g = glaGame(cfg)
	g.P[1].X, g.P[1].Y = 800+gl.Icicle.DamageRadius+g.radius(1)+30, 450
	hp0 = g.P[1].HP
	gpress(g, Input{Ult: true})
	steps(g, secs(0.3)+3)
	if g.P[1].HP != hp0 || g.P[1].SlowT != 0 {
		t.Fatalf("a foe outside damage_radius was hurt (HP %d -> %d) or slowed", hp0, g.P[1].HP)
	}
}

func TestVulpixItselfWalksThroughItsOwnIcicles(t *testing.T) {
	cfg := DefaultConfig()
	g := glaGame(cfg)
	g.Balls = []Ball{{K: ballPillar, Owner: 0, X: g.P[0].X, Y: g.P[0].Y, HP: 30, Life: 10, Rad: 40}}
	x, y := g.P[0].X, g.P[0].Y
	g.Step(dt)
	if g.P[0].X != x || g.P[0].Y != y {
		t.Fatal("Vulpix was pushed by its own icicle")
	}
}

func TestAFrozenVulpixDoesNotCallAGlaciate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Vulpix.Glaciate.RingDuration = 0.5
	g := glaGame(cfg)
	gpress(g, Input{Ult: true})
	g.P[0].FrozenT = 1
	steps(g, secs(1))
	if len(g.Casts) != 0 || len(g.Icicles) != 0 {
		t.Fatal("a Vulpix that was frozen while it gathered still called icicles")
	}
}

func TestGlaciateNumbersAreCheckedAndSentToThePage(t *testing.T) {
	for name, set := range map[string]func(*Config){
		"count 0":           func(c *Config) { c.Vulpix.Glaciate.Count = 0 },
		"too many":          func(c *Config) { c.Vulpix.Glaciate.Count = maxIcicles + 1 },
		"radius 0":          func(c *Config) { c.Vulpix.Glaciate.Radius = 0 },
		"duration 0":        func(c *Config) { c.Vulpix.Glaciate.Duration = 0 },
		"icicle hp 0":       func(c *Config) { c.Vulpix.Glaciate.Icicle.HP = 0 },
		"icicle radius":     func(c *Config) { c.Vulpix.Glaciate.Icicle.Radius = 0 },
		"icicle scale":      func(c *Config) { c.Vulpix.Glaciate.Icicle.Scale = 0 },
		"slow over 1":       func(c *Config) { c.Vulpix.Glaciate.Icicle.Slow = 1.5 },
		"negative ring":     func(c *Config) { c.Vulpix.Glaciate.RingDuration = -1 },
		"charge max 0":      func(c *Config) { c.Vulpix.Ultimate.Max = 0 },
		"reach upside down": func(c *Config) { c.Vulpix.Glaciate.MinHeight, c.Vulpix.Glaciate.MaxHeight = 100, 50 },
	} {
		cfg := DefaultConfig()
		set(&cfg)
		if cfg.validate() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	cfg := DefaultConfig()
	cfg.Vulpix.Glaciate.Icicle.Scale = 2.5
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	hi := next(t, dialIn(t, srv, "mobile"), "hi")
	if v := hi["glaciate"].(map[string]any); v["scale"] != 2.5 || v["dur"] != cfg.Vulpix.Glaciate.Duration {
		t.Fatalf("hi.glaciate = %v", v)
	}
}

// Whatever the way (walking, or the skill that moves the body when walking is too slow), the bot is not under the icicle when it lands.
func TestTheBotIsNotHurtByAnIcicleItSawComing(t *testing.T) {
	cfg := liveConfig(t)
	g := duel(cfg, snorlax, vulpix)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 450, 1500, 450
	g.Icicles = []icicle{{Owner: 1, X: 900, Y: 450, T: cfg.Vulpix.Glaciate.Icicle.FallDuration}}
	hp0 := g.P[0].HP
	b := newBot(snorlax, 1)
	for tt := 0.0; tt < 1.5; tt += dt {
		g.P[0].In = b.Decide(g, 0)
		g.Step(dt)
	}
	if g.P[0].HP != hp0 {
		t.Fatalf("the bot lost %d HP to an icicle it saw coming", hp0-g.P[0].HP)
	}
}
