package main

import "testing"

// slashConfig is the Rowlet of rowletConfig with the numbers asked for: 6 slashes at 650, radius 8, damage 2, mass 20, hitting heights 0..15.
func slashConfig() Config {
	cfg := rowletConfig()
	sl := &cfg.Rowlet.Tailwind.Slash
	sl.Reach = Reach{MinHeight: 0, MaxHeight: 15}
	sl.Count, sl.Speed, sl.Radius, sl.HP, sl.Damage, sl.Mass, sl.Scale = 6, 650, 8, 1, 2, 20, 0.5
	return cfg
}

func TestTheDriftThrowsTheConfiguredCountOfSlashesSpreadAlongTheRun(t *testing.T) {
	cfg := slashConfig()
	g := rg(cfg)
	g.P[1].X, g.P[1].Y = 1500, 700 // far: nothing reaches it during the run
	g.P[0].In = Input{Skill: true, DX: 1}
	var at []int // the tick each slash appeared on
	seen := 0
	ticks := int(cfg.Rowlet.Tailwind.Duration*tickRate) + 2
	for n := 0; n < ticks; n++ {
		g.Step(dt)
		g.P[0].In = Input{}
		for ; seen < len(ballsOfKind(g, ballSlash)); seen++ {
			at = append(at, n)
		}
	}
	if len(at) != cfg.Rowlet.Tailwind.Slash.Count {
		t.Fatalf("%d slashes thrown, want %d", len(at), cfg.Rowlet.Tailwind.Slash.Count)
	}
	if at[0] == at[len(at)-1] || at[len(at)-1] < int(cfg.Rowlet.Tailwind.Duration*tickRate)-2 {
		t.Fatalf("slashes appeared on ticks %v: they should be spread along the whole run, the last as it ends", at)
	}
	if g.P[0].SlashN != cfg.Rowlet.Tailwind.Slash.Count {
		t.Fatalf("SlashN = %d", g.P[0].SlashN)
	}
}

func TestACountOfZeroThrowsNoSlashesAndASecondDriftThrowsASecondSet(t *testing.T) {
	cfg := slashConfig()
	cfg.Rowlet.Tailwind.Slash.Count = 0
	g := rg(cfg)
	g.P[0].In = Input{Skill: true, DX: 1}
	for range tickRate {
		g.Step(dt)
		g.P[0].In = Input{}
	}
	if n := len(ballsOfKind(g, ballSlash)); n != 0 {
		t.Fatalf("%d slashes with a count of 0", n)
	}
	cfg = slashConfig()
	cfg.Rowlet.Tailwind.Cooldown = 0.1
	g = rg(cfg)
	g.P[1].X, g.P[1].Y = 1500, 700
	for round := 0; round < 2; round++ {
		g.P[0].In = Input{Skill: true, DX: 1}
		for range tickRate {
			g.Step(dt)
			g.P[0].In = Input{}
		}
	}
	if n := g.P[0].SlashN; n != cfg.Rowlet.Tailwind.Slash.Count {
		t.Fatalf("the second drift threw %d, want a fresh %d", n, cfg.Rowlet.Tailwind.Slash.Count)
	}
}

func TestASlashFliesAtTheOpponentAndHurtsItOnceForTheConfiguredDamage(t *testing.T) {
	cfg := slashConfig()
	g := rg(cfg) // Rowlet at 300,250; the foe at 900,250
	g.P[0].Y, g.P[1].Y = 250, 250
	g.Balls = append(g.Balls, Ball{K: ballSlash, Owner: 0, X: 320, Y: 250, DX: 1, DY: 0, HP: 1})
	hp := g.P[1].HP
	for range tickRate {
		g.Step(dt)
	}
	if got := hp - g.P[1].HP; got != cfg.Rowlet.Tailwind.Slash.Damage {
		t.Fatalf("the foe lost %d HP, want %d", got, cfg.Rowlet.Tailwind.Slash.Damage)
	}
	if n := len(ballsOfKind(g, ballSlash)); n != 0 {
		t.Fatalf("%d slashes left after it hit: it should be spent", n)
	}
}

func TestASlashOnlyHurtsWhatIsWithinItsHeights(t *testing.T) {
	for _, tc := range []struct {
		z    float64
		hurt bool
	}{{0, true}, {15, true}, {16, false}, {80, false}} {
		cfg := slashConfig()
		g := duel(cfg, rowlet, pikachu) // Pikachu has no gravity of its own: it stays at the height it is put at
		g.P[1].Z = tc.z
		g.Balls = append(g.Balls, Ball{K: ballSlash, Owner: 0, X: 320, Y: 250, DX: 1, DY: 0, HP: 1})
		hp := g.P[1].HP
		for range tickRate {
			g.Step(dt)
		}
		if hurt := g.P[1].HP < hp; hurt != tc.hurt {
			t.Errorf("a foe at height %v: hurt = %v, want %v", tc.z, hurt, tc.hurt)
		}
	}
}

func TestASlashFlyingAtTheNearestFoeNeverHurtsATeammateOnTheWay(t *testing.T) {
	cfg := slashConfig()
	g := NewTeamGame(cfg, modeWeb, 4)
	g.P[0].Kind, g.P[2].Kind, g.P[1].Kind, g.P[3].Kind = rowlet, snorlax, snorlax, snorlax
	g.Reset()
	g.P[0].X, g.P[0].Y = 300, 400
	g.P[2].X, g.P[2].Y = 500, 400 // the teammate, in the line
	g.P[1].X, g.P[1].Y = 900, 400
	g.P[3].X, g.P[3].Y = 1500, 1000
	g.Balls = append(g.Balls, Ball{K: ballSlash, Owner: 0, X: 320, Y: 400, DX: 1, DY: 0, HP: 1})
	mate, foe := g.P[2].HP, g.P[1].HP
	for range tickRate * 2 {
		g.Step(dt)
	}
	if g.P[2].HP != mate {
		t.Fatalf("the teammate lost %d HP to its own slash", mate-g.P[2].HP)
	}
	if g.P[1].HP >= foe {
		t.Fatal("the foe behind the teammate was not hurt")
	}
}

func TestAnAirSlashHasTheMassAndHPOfItsConfigAndCanBeShotDown(t *testing.T) {
	cfg := slashConfig()
	g := rg(cfg)
	b := &Ball{K: ballSlash, Owner: 0}
	if g.ballMass(b) != 20 || g.ballRadius(b) != 8 || g.ballDamage(b) != 2 {
		t.Fatalf("mass %v radius %v damage %v, want 20, 8, 2", g.ballMass(b), g.ballRadius(b), g.ballDamage(b))
	}
	g.Balls = append(g.Balls, Ball{K: ballSlash, Owner: 0, X: 320, Y: 250, DX: 1, HP: 0})
	g.Step(dt)
	if n := len(ballsOfKind(g, ballSlash)); n != 0 {
		t.Fatal("a slash with no HP left flew on")
	}
}

func TestTheConfigRefusesABadAirSlash(t *testing.T) {
	for name, edit := range map[string]func(c *Config){
		"zero speed":    func(c *Config) { c.Rowlet.Tailwind.Slash.Speed = 0 },
		"zero radius":   func(c *Config) { c.Rowlet.Tailwind.Slash.Radius = 0 },
		"zero hp":       func(c *Config) { c.Rowlet.Tailwind.Slash.HP = 0 },
		"negative":      func(c *Config) { c.Rowlet.Tailwind.Slash.Count = -1 },
		"too many":      func(c *Config) { c.Rowlet.Tailwind.Slash.Count = 101 },
		"no scale":      func(c *Config) { c.Rowlet.Tailwind.Slash.Scale = 0 },
		"backward band": func(c *Config) { c.Rowlet.Tailwind.Slash.Reach = Reach{MinHeight: 20, MaxHeight: 10} },
	} {
		cfg := DefaultConfig()
		edit(&cfg)
		if err := cfg.validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
