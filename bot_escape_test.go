package main

import (
	"math"
	"testing"
)

// escapeKeys says whether a decision pushed a key that moves the body (a skill, the toss, the kick).
func escapeKeys(in Input) bool { return in.Skill || in.Toss || in.Kick }

// runBot plays the bot in slot 0 for `secs` seconds and says whether it ever pressed a movement skill, and how much HP it lost.
func runBot(g *Game, b *Bot, secs float64) (pressed bool, lost int) {
	hp0 := g.P[0].HP
	for tt := 0.0; tt < secs; tt += dt {
		g.P[0].In = b.Decide(g, 0)
		pressed = pressed || escapeKeys(g.P[0].In)
		g.Step(dt)
	}
	return pressed, hp0 - g.P[0].HP
}

// An icicle that lands on the bot, with too little time to walk out of its reach, is got away from by the skill each character has that moves the body.
func TestTheBotUsesItsMovementSkillWhenAnIcicleIsAboutToLandAndWalkingCannotGetOut(t *testing.T) {
	cfg := liveConfig(t)
	for _, kind := range []int{pikachu, snorlax, chikorita, ash, machamp, charizard, vulpix, gengar, rowlet} {
		g := duel(cfg, kind, snorlax)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 450, 1700, 450
		g.P[0].HP, g.P[1].HP = 100000, 100000
		g.Icicles = []icicle{{Owner: 1, X: 900, Y: 450, T: cfg.Vulpix.Glaciate.Icicle.FallDuration}}
		pressed, lost := runBot(g, newBot(kind, 1), 2)
		if !pressed || lost != 0 {
			t.Errorf("%s: pressed %v, lost %d HP: it should have got out of the way with its movement skill", NAMEOF[kind], pressed, lost)
		}
		// the same icicle, a body that does not move: it is hit, so the test above says something
		g = duel(cfg, kind, snorlax)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 450, 1700, 450
		g.P[0].HP, g.P[1].HP = 100000, 100000
		g.Icicles = []icicle{{Owner: 1, X: 900, Y: 450, T: cfg.Vulpix.Glaciate.Icicle.FallDuration}}
		steps(g, secs(2))
		if g.P[0].HP == 100000 && kind != gengar && kind != rowlet {
			t.Errorf("%s: a body that stood still was not hurt by the icicle", NAMEOF[kind])
		}
	}
}

// Where walking is enough, the skill is kept: it has a cooldown and the walk costs nothing.
func TestTheBotJustWalksOutOfAnIcicleItHasTimeToWalkOutOf(t *testing.T) {
	cfg := liveConfig(t)
	g := duel(cfg, pikachu, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 1000, 450, 1700, 450
	g.P[0].HP, g.P[1].HP = 100000, 100000
	dmgr := cfg.Vulpix.Glaciate.Icicle.DamageRadius
	g.Icicles = []icicle{{Owner: 1, X: 1000 - (dmgr - 30), Y: 450, T: 1.5}} // just inside its reach, and plenty of time
	b := newBot(pikachu, 1)
	for n := 0; n < botReact+2; n++ { // it sees what was there botReact ticks ago, so give it a past
		b.Decide(g, 0)
		g.Step(dt)
	}
	c := b.look(g, 0)
	if c.th.kind == thNone || c.cant {
		t.Fatalf("threat %+v, cant %v: it should see the icicle and know it can walk out", c.th, c.cant)
	}
	if _, ok := b.escapeBySkill(c); ok {
		t.Fatal("it used a movement skill where walking was enough")
	}
	// the same icicle with no time left: it cannot, and says so
	g.Icicles = []icicle{{Owner: 1, X: 1000 - (dmgr - 30), Y: 450, T: 0.3}}
	if c := b.look(g, 0); !c.cant {
		t.Fatalf("threat %+v: with 0.3 s left it should know walking will not do", c.th)
	}
}

// A storm that is faster than the bot and already at its heels cannot be walked away from: the bot dashes. A slow one is walked away from.
func TestTheBotUsesItsMovementSkillAgainstAChaserFasterThanItAtItsHeels(t *testing.T) {
	cfg := liveConfig(t)
	storm := func(speed float64) (bool, float64) {
		c := cfg
		c.Vulpix.IceStorm.FollowSpeed = speed
		g := duel(c, pikachu, vulpix)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 450, 1700, 450
		g.P[0].HP, g.P[1].HP = 100000, 100000
		g.P[1].StormLeft, g.P[1].StormIn, g.P[1].StormX, g.P[1].StormY = c.Vulpix.IceStorm.Strikes, c.Vulpix.IceStorm.Interval, 900+c.Vulpix.IceStorm.Radius+g.radius(0)+50, 450
		pressed, _ := runBot(g, newBot(pikachu, 1), 1)
		return pressed, math.Hypot(g.P[0].X-g.P[1].StormX, g.P[0].Y-g.P[1].StormY)
	}
	if pressed, _ := storm(cfg.Pikachu.WalkSpeed * 2); !pressed {
		t.Error("a storm twice as fast as the bot walks, at its heels: the bot did not dash")
	}
	if pressed, _ := storm(cfg.Pikachu.WalkSpeed * 0.3); pressed {
		t.Error("a storm that is slow: the bot dashed when it could have walked")
	}
}

func TestTheBotKeepsItsMovementSkillWhenItCannotUseIt(t *testing.T) {
	cfg := liveConfig(t)
	g := duel(cfg, pikachu, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 450, 1700, 450
	g.P[0].HP, g.P[1].HP = 100000, 100000
	g.P[0].FrozenT = 5 // frozen: nothing starts
	g.Icicles = []icicle{{Owner: 1, X: 900, Y: 450, T: cfg.Vulpix.Glaciate.Icicle.FallDuration}}
	if pressed, _ := runBot(g, newBot(pikachu, 1), 1); pressed {
		t.Fatal("a frozen bot pressed a skill")
	}
}

// A bird of fire and a Brave Bird coming down on the bot are too big to walk out of in the time they take, so they too are got away from by skill.
func TestTheBotUsesItsMovementSkillAgainstABirdOfFireAndABraveBirdOnTheWay(t *testing.T) {
	cfg := liveConfig(t)
	in := cfg.Charizard.Gigantamax.Inferno
	bb := cfg.Rowlet.BraveBird
	for _, c := range []struct {
		name  string
		setup func(g *Game)
	}{
		{"bird of fire", func(g *Game) {
			g.Balls = []Ball{{K: ballFirebird, Owner: 1, Phase: birdDiving, X: 900 + in.DiveSpeed*0.5, Y: 450, Z: 200, AnchorX: 900, AnchorY: 450, HP: in.HP}}
		}},
		{"brave bird", func(g *Game) {
			g.P[1].BirdDive, g.P[1].BirdTX, g.P[1].BirdTY, g.P[1].X, g.P[1].Y, g.P[1].Z = true, 900, 450, 900+bb.DiveSpeed*0.5, 450, 200
		}},
	} {
		kind, foe := pikachu, charizard
		if c.name == "brave bird" {
			foe = rowlet
		}
		g := duel(cfg, kind, foe)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 900, 450, 1700, 450
		g.P[0].HP, g.P[1].HP = 100000, 100000
		c.setup(g)
		b := newBot(kind, 1)
		for n := 0; n < botReact+2; n++ {
			b.observe(g, 0)
			b.ticks++
		}
		cc := b.look(g, 0)
		if cc.th.kind == thNone || !cc.cant {
			t.Errorf("%s: threat %+v cant %v: walking cannot get out of it in time, and the bot should know", c.name, cc.th, cc.cant)
			continue
		}
		if _, ok := b.escapeBySkill(cc); !ok {
			t.Errorf("%s: the bot did not reach for its movement skill", c.name)
		}
	}
}
