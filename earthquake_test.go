package main

import "testing"

// quakeGame is a Snorlax (slot 1) about to jump on the spot, with a Pikachu `d` away on the same line.
func quakeGame(cfg Config, d float64) *Game {
	g := NewGame(cfg)
	g.P[snorlax].X, g.P[snorlax].Y = 300, 250
	g.P[pikachu].X, g.P[pikachu].Y = 300+d, 250
	g.P[pikachu].HP = 100000
	g.P[snorlax].In.Skill = true
	return g
}

// land steps until the landing has made an earthquake (or the time is up) and lets go of the button.
func land(g *Game) {
	for n := 0; n < tickRate*3 && len(g.Quakes) == 0; n++ {
		g.Step(dt)
	}
	g.P[snorlax].In = Input{}
}

func TestTheLandingDamageIsTheJumpsDamage(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Snorlax.Jump.Damage != slamDamage {
		t.Fatalf("the default is %d, want what it always was (%d)", cfg.Snorlax.Jump.Damage, slamDamage)
	}
	cfg.Snorlax.Jump.Damage = 33
	g := quakeGame(cfg, 30)
	for n := 0; n < tickRate*2 && g.P[pikachu].HP == 100000; n++ {
		g.Step(dt)
	}
	if got := 100000 - g.P[pikachu].HP; got != 33 {
		t.Fatalf("the landing did %d, want 33", got)
	}
}

func TestThereIsNoEarthquakeUnlessTheRadiusIsSet(t *testing.T) {
	g := quakeGame(DefaultConfig(), 30)
	land(g)
	steps(g, 60)
	if len(g.Quakes) != 0 {
		t.Fatal("the default config made an earthquake")
	}
}

func TestTheLandingLeavesAnEarthquakeThatHurtsAndSlowsAndEnds(t *testing.T) {
	cfg := DefaultConfig()
	jp := &cfg.Snorlax.Jump
	jp.EarthquakeRadius, jp.EarthquakeDuration, jp.EarthquakeDamage, jp.EarthquakeInterval, jp.EarthquakeSlow = jp.SlamRadius+200, 2, 7, 0.5, 0.4
	d := jp.SlamRadius + 100 // inside the earthquake, outside the landing shock
	g := quakeGame(cfg, d)
	var quakeEv *Event
	for n := 0; n < tickRate*3 && quakeEv == nil; n++ {
		for _, e := range g.Step(dt) {
			if e.T == "quake" {
				e := e
				quakeEv = &e
			}
		}
	}
	g.P[snorlax].In = Input{}
	if quakeEv == nil || quakeEv.R != jp.EarthquakeRadius || quakeEv.L != jp.EarthquakeDuration || quakeEv.I != jp.EarthquakeInterval || quakeEv.X != 300 {
		t.Fatalf("event %+v: it should tell the page where, how wide, how long and how often", quakeEv)
	}
	if g.P[pikachu].HP != 100000 {
		t.Fatal("the landing shock reached a foe outside its radius")
	}
	steps(g, tickRate/2+2) // the first shock
	if got := 100000 - g.P[pikachu].HP; got != jp.EarthquakeDamage {
		t.Fatalf("after one interval it took %d, want %d", got, jp.EarthquakeDamage)
	}
	if g.P[pikachu].SlowT <= 0 || g.P[pikachu].SlowK != jp.EarthquakeSlow {
		t.Fatalf("slow %v for %v: it should be slowed by the earthquake's share", g.P[pikachu].SlowK, g.P[pikachu].SlowT)
	}
	steps(g, tickRate*3)
	if len(g.Quakes) != 0 {
		t.Fatal("the earthquake never ended")
	}
	if got := 100000 - g.P[pikachu].HP; got != 4*jp.EarthquakeDamage {
		t.Fatalf("over %v seconds it took %d, want four shocks of %d", jp.EarthquakeDuration, got, jp.EarthquakeDamage)
	}
}

func TestTheEarthquakeLeavesAlonewhatIsOutsideItOrInTheAir(t *testing.T) {
	cfg := DefaultConfig()
	jp := &cfg.Snorlax.Jump
	jp.EarthquakeRadius, jp.EarthquakeDuration, jp.EarthquakeDamage, jp.EarthquakeInterval, jp.EarthquakeSlow = jp.SlamRadius+100, 2, 7, 0.5, 0.4
	g := quakeGame(cfg, jp.EarthquakeRadius+200) // outside it
	land(g)
	steps(g, tickRate*2)
	if g.P[pikachu].HP != 100000 || g.P[pikachu].SlowT > 0 {
		t.Fatal("a foe outside the earthquake was hurt or slowed")
	}
	g = quakeGame(cfg, jp.SlamRadius+50)
	land(g)
	g.P[pikachu].Z = jp.MaxHeight + 100 // up in the air: out of reach of a ground attack
	hp := g.P[pikachu].HP
	steps(g, tickRate/2)
	if g.P[pikachu].HP != hp {
		t.Fatal("the earthquake hurt a foe high in the air")
	}
}

func TestAnEarthquakeEndsWithTheRound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.Jump.EarthquakeRadius = cfg.Snorlax.Jump.SlamRadius + 100
	g := quakeGame(cfg, 30)
	land(g)
	g.Reset()
	if len(g.Quakes) != 0 {
		t.Fatal("an earthquake survived the round")
	}
}

func TestTheEarthquakeNumbersAreChecked(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"damage":            func(c *Config) { c.Snorlax.Jump.Damage = -1 },
		"smaller than slam": func(c *Config) { c.Snorlax.Jump.EarthquakeRadius = c.Snorlax.Jump.SlamRadius - 1 },
		"no duration":       func(c *Config) { c.Snorlax.Jump.EarthquakeDuration = 0 },
		"no interval":       func(c *Config) { c.Snorlax.Jump.EarthquakeInterval = 0 },
		"slow of 1":         func(c *Config) { c.Snorlax.Jump.EarthquakeSlow = 1 },
		"negative shock":    func(c *Config) { c.Snorlax.Jump.EarthquakeDamage = -1 },
	} {
		c := DefaultConfig()
		c.Snorlax.Jump.EarthquakeRadius = c.Snorlax.Jump.SlamRadius + 100
		edit(&c)
		if c.validate() == nil {
			t.Fatalf("%s: a nonsense number was accepted", name)
		}
	}
	if err := DefaultConfig().validate(); err != nil {
		t.Fatal(err)
	}
}
