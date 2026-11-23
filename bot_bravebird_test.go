package main

import (
	"math"
	"testing"
)

// diveAtBot sets up a Pikachu (slot 0) and a Rowlet (slot 1) that starts a Brave Bird dive on the next tick. The dive is fixed on the place
// Pikachu stands when it starts (lock_distance is huge), and slow enough to be walked away from. It returns the game and the HP Pikachu has.
func diveAtBot(cfg Config) *Game {
	bb := &cfg.Rowlet.BraveBird
	bb.RingDuration, bb.ChargeDuration, bb.SkyHeight, bb.DiveSpeed, bb.LockDistance = 0.1, 0.1, 300, 450, 1e6
	bb.ExplosionRadius, bb.Damage, bb.LaunchHeight, bb.LaunchDuration = 160, 30, 150, 1
	bb.Tornado.Duration, bb.PullDuration = 0, 0
	g := duel(cfg, pikachu, rowlet)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 700, 450, 1400, 450
	g.P[1].Charge = cfg.Rowlet.Ultimate.Max
	g.P[1].In = Input{Ult: true}
	return g
}

func TestTheBotWalksOutOfTheSpotABraveBirdIsDivingOn(t *testing.T) {
	cfg := liveConfig(t)
	for seed := uint64(1); seed <= 10; seed++ {
		g := diveAtBot(cfg)
		b := newBot(pikachu, seed)
		hp0 := g.P[0].HP
		for tt := 0.0; tt < 4; tt += dt {
			g.P[0].In = b.Decide(g, 0)
			g.Step(dt)
			g.P[1].In = Input{}
		}
		if g.P[0].HP < hp0 || g.P[0].Juggle {
			t.Fatalf("seed %d: the bird landed on the bot (HP %d -> %d)", seed, hp0, g.P[0].HP)
		}
	}
	// Control: a body that stands still is hit, so the test above says something.
	g := diveAtBot(cfg)
	hp0 := g.P[0].HP
	for tt := 0.0; tt < 4; tt += dt {
		g.Step(dt)
		g.P[1].In = Input{}
	}
	if g.P[0].HP >= hp0 {
		t.Fatal("the bird did not hurt a body that stood still: the dive in this test is not aimed at it")
	}
}

func TestTheBotWalksOutOfTheReachOfBraveBirdsPull(t *testing.T) {
	cfg := liveConfig(t)
	const gravity, reach = 60.0, 300.0 // slower than the bot walks, so leaving is possible
	for seed := uint64(1); seed <= 10; seed++ {
		g := duel(cfg, pikachu, rowlet)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 800, 450, 1500, 450
		g.Wells = []pullWell{{Owner: 1, X: 700, Y: 450, T: 4, R: reach, Gravity: gravity, Reach: cfg.Rowlet.BraveBird.Reach, Juggled: true}}
		b := newBot(pikachu, seed)
		for tt := 0.0; tt < 3; tt += dt {
			g.P[0].In = b.Decide(g, 0)
			g.Step(dt)
		}
		if d := math.Hypot(g.P[0].X-700, g.P[0].Y-450); d <= reach {
			t.Fatalf("seed %d: the bot is still %.0f from the middle of the pull (reach %.0f)", seed, d, reach)
		}
	}
	// A Shadow Ball's well is not dodged (only Brave Bird's is): the bot is left to the old behaviour.
	g := duel(cfg, pikachu, gengar)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 800, 450, 1500, 450
	g.Wells = []pullWell{{Owner: 1, X: 700, Y: 450, T: 4, R: reach, Gravity: gravity, Reach: cfg.Gengar.ShadowBall.Reach}}
	b := newBot(pikachu, 1)
	for n := 0; n < botReact+2; n++ { // it sees what was there botReact ticks ago, so give it a past
		b.Decide(g, 0)
	}
	if th := b.look(g, 0).th; th.kind != thNone {
		t.Fatalf("the bot sees a threat (kind %d) in a Shadow Ball's well", th.kind)
	}
}
