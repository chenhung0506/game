package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Every attack has a band of heights (min_height..max_height) in which it hurts. These tests put the target at
// a height and ask whether the skill got it, with the band set to 0..50: 40 is inside, 60 is above it.

var bandHeights = []struct {
	name string
	z    float64
	hit  bool
}{{"on the ground", 0, true}, {"inside the band", 40, true}, {"above the band", 60, false}}

func TestHyperBeamHitsOnlyWithinItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.HyperBeam.MaxHeight = 50
			g := beamGame(cfg)
			g.P[pikachu].Z = tc.z
			if got := hitTick(g) != 0; got != tc.hit {
				t.Fatalf("hit = %v at height %v with a 0..50 band, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

// The user's example: the same beam against a flying Charizard, only fly.height differs.
func TestFlyHeightAgainstTheBeamBand(t *testing.T) {
	for _, tc := range []struct {
		fly float64
		hit bool
	}{{40, true}, {60, false}} {
		cfg := DefaultConfig()
		cfg.Snorlax.HyperBeam.MaxHeight = 50
		cfg.Charizard.Fly.Height = tc.fly
		g := duel(cfg, snorlax, charizard)
		g.P[0].X, g.P[1].X = 600, 300
		g.P[1].In.Skill = true
		g.Step(dt)
		g.P[1].In = Input{}
		steps(g, 10) // up at fly.height
		g.P[0].In.Beam = true
		g.Step(dt)
		g.P[0].In = Input{}
		hit := false
		for i := 0; i < 5*tickRate; i++ {
			g.Step(dt)
			hit = hit || g.P[1].HP < cfg.Charizard.MaxHP
		}
		if hit != tc.hit {
			t.Errorf("fly.height %v: beam hit = %v, want %v", tc.fly, hit, tc.hit)
		}
	}
}

func TestMinHeightKeepsAttackFromHittingLowerTargets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.HyperBeam.MinHeight, cfg.Snorlax.HyperBeam.MaxHeight = 20, 50
	for _, tc := range []struct {
		z   float64
		hit bool
	}{{0, false}, {40, true}, {60, false}} {
		g := beamGame(cfg)
		g.P[pikachu].Z = tc.z
		if got := hitTick(g) != 0; got != tc.hit {
			t.Errorf("height %v with a 20..50 band: hit = %v, want %v", tc.z, got, tc.hit)
		}
	}
}

func TestIcePunchHitsOnlyWithinItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.IcePunch.MaxHeight = 50
			g := punchGame(cfg)
			pressPunch(g, -1, 0)
			for i := 0; i < 40; i++ {
				g.P[pikachu].Z = tc.z
				g.Step(dt)
			}
			if got := g.P[pikachu].FrozenT > 0 || g.P[pikachu].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v at height %v, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

func TestElectroBallBodyAndBlastFollowTheirHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Pikachu.ElectroBall.MaxHeight = 50
			g := ballGame(cfg)
			g.Balls = []Ball{{X: 1300, Y: 500, DX: 1, HP: 30}}
			exploded := false
			for i := 0; i < 10; i++ {
				g.P[snorlax].Z, g.P[snorlax].VZ = tc.z, 0
				n, _ := explodeEvents(g.Step(dt))
				exploded = exploded || n > 0
			}
			if exploded != tc.hit || (g.P[snorlax].HP < maxHP) != tc.hit {
				t.Fatalf("exploded = %v, HP = %d at height %v, want a hit = %v", exploded, g.P[snorlax].HP, tc.z, tc.hit)
			}
		})
	}
}

func TestElectroBallContactDamageAddsToTheBlast(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.ContactDamage = 7
	g := ballGame(cfg)
	g.Balls = []Ball{{X: 1300, Y: 500, DX: 1, HP: 30}}
	steps(g, 10)
	want := maxHP - 7 - cfg.Pikachu.ElectroBall.ExplosionDamage
	if g.P[snorlax].HP != want {
		t.Fatalf("HP = %d, want %d (contact 7 + blast %d)", g.P[snorlax].HP, want, cfg.Pikachu.ElectroBall.ExplosionDamage)
	}
}

func TestElectroBallContactDamageIsNotDealtWhenTheBallIsShotDownOrHitsAWall(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.ContactDamage = 7
	cfg.Pikachu.ElectroBall.ExplosionRadius = 20 // too small to reach the opponent
	g := ballGame(cfg)
	g.P[snorlax].X = 1000
	g.Balls = []Ball{{X: 300, Y: 500, DX: -1, HP: 30}} // flies away from the opponent into the wall
	steps(g, 30)
	if g.P[snorlax].HP != maxHP {
		t.Fatalf("HP = %d, nobody touched the ball", g.P[snorlax].HP)
	}
}

func TestElectroBallContactDamageDefaultsToNone(t *testing.T) {
	g := ballGame(DefaultConfig())
	g.Balls = []Ball{{X: 1300, Y: 500, DX: 1, HP: 30}}
	steps(g, 10)
	if g.P[snorlax].HP != maxHP-DefaultConfig().Pikachu.ElectroBall.ExplosionDamage {
		t.Fatalf("HP = %d, want only the blast", g.P[snorlax].HP)
	}
}

func TestThunderHitsOnlyWithinItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Pikachu.Thunder.Strikes, cfg.Pikachu.Thunder.MaxHeight = 1, 50
			g := thunderGame(cfg)
			castThunder(g)
			for i := 0; i < 2*tickRate; i++ {
				g.P[snorlax].Z, g.P[snorlax].VZ = tc.z, 0 // held up: no falling
				g.Step(dt)
			}
			if got := g.P[snorlax].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v at height %v, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

func TestLandingShockHitsOnlyWithinItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Snorlax.Jump.MaxHeight = 50
			g := NewGame(cfg)
			g.P[snorlax].X, g.P[pikachu].X = 300, 310
			g.P[snorlax].Y, g.P[pikachu].Y = 250, 250
			g.P[snorlax].Z, g.P[snorlax].VZ = 1, -100 // lands on the next step
			g.P[pikachu].Z = tc.z
			g.Step(dt)
			if got := g.P[pikachu].HP < maxHP; got != tc.hit {
				t.Fatalf("hit = %v at height %v, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

func TestFireBallsHitOnlyWithinTheirHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Charizard.FireBreath.RandomOffsetAngle, cfg.Charizard.FireBreath.MaxHeight = 0, 50
			g := czGame(cfg)
			g.P[1].X = 600
			g.P[0].In.Fire = true
			for i := 0; i < 30; i++ {
				g.P[1].Z, g.P[1].VZ = tc.z, 0 // held up: no falling
				g.Step(dt)
			}
			if got := g.P[1].HP < cfg.Snorlax.MaxHP; got != tc.hit {
				t.Fatalf("hit = %v at height %v, want %v", got, tc.z, tc.hit)
			}
		})
	}
}

func TestSeismicTossCatchesOnlyWithinItsHeightBand(t *testing.T) {
	for _, tc := range bandHeights {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Charizard.SeismicToss.MaxHeight = 50
			g := tossGame(cfg)
			startToss(g)
			for i := 0; i < 30; i++ {
				g.P[1].Z, g.P[1].VZ = tc.z, 0
				g.Step(dt)
				if g.P[1].Held {
					break
				}
			}
			if g.P[1].Held != tc.hit {
				t.Fatalf("caught = %v at height %v, want %v", g.P[1].Held, tc.z, tc.hit)
			}
		})
	}
}

// ---- config ----

func loadJSON(t *testing.T, body string) (Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(p)
}

func TestHeightBandsAndContactDamageLoadFromConfig(t *testing.T) {
	c, err := loadJSON(t, `{"snorlax":{"hyper_beam":{"min_height":5,"max_height":50}},
		"pikachu":{"electro_ball":{"contact_damage":9}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if b := c.Snorlax.HyperBeam; b.MinHeight != 5 || b.MaxHeight != 50 {
		t.Errorf("beam band = %v..%v, want 5..50", b.MinHeight, b.MaxHeight)
	}
	if b := c.Snorlax.IcePunch; b.MinHeight != 0 || b.MaxHeight != groundZ {
		t.Errorf("ice punch band = %v..%v, want the defaults 0..%v", b.MinHeight, b.MaxHeight, groundZ)
	}
	if c.Pikachu.ElectroBall.ContactDamage != 9 {
		t.Errorf("contact_damage = %d, want 9", c.Pikachu.ElectroBall.ContactDamage)
	}
}

func TestInvalidHeightBandsAreRefused(t *testing.T) {
	for name, body := range map[string]string{
		"min above max":  `{"snorlax":{"hyper_beam":{"min_height":60,"max_height":50}}}`,
		"negative min":   `{"charizard":{"fire_breath":{"min_height":-1}}}`,
		"max below min":  `{"pikachu":{"thunder":{"min_height":5,"max_height":2}}}`,
		"negative blast": `{"pikachu":{"electro_ball":{"contact_damage":-1}}}`,
	} {
		if _, err := loadJSON(t, body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
