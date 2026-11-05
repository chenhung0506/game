package main

import (
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

// kickDuel is Machamp (slot 0, at 300,250) against a Snorlax (slot 1) `d` ahead of it on the same line.
func kickDuel(cfg Config, d float64) *Game {
	g := duel(cfg, machamp, snorlax)
	g.P[0].HP = g.maxHP(0)
	g.P[1].X, g.P[1].Y = 300+d, 250
	return g
}

func TestHighJumpKickSlidesAlongTheStickTheConfiguredDistanceInTheConfiguredTime(t *testing.T) {
	cfg := DefaultConfig()
	hk := cfg.Machamp.HighJumpKick
	g := kickDuel(cfg, 700)
	g.P[1].Y = 900 // far off the line: this is about the slide
	press(g, 0, Input{Skill: true, DY: 1})
	n := 1
	for g.P[0].KickPhase == kickSlide {
		if g.P[0].X != 300 || g.P[0].Z != 0 {
			t.Fatalf("slid sideways or rose: %v z=%v", g.P[0].X, g.P[0].Z)
		}
		g.P[0].In = Input{DX: -1} // the stick moves on: the slide keeps its way
		g.Step(dt)
		n++
	}
	if want := int(math.Ceil(hk.SlideDuration / dt)); n != want {
		t.Fatalf("slide took %d ticks, want %d", n, want)
	}
	if got := g.P[0].Y - 250; math.Abs(got-hk.SlideDistance) > 1e-6 {
		t.Fatalf("slid %v, want %v", got, hk.SlideDistance)
	}
	if g.P[0].KickPhase != kickFly {
		t.Fatalf("phase %d after the slide, want the kick", g.P[0].KickPhase)
	}
	// the kick goes at where the opponent stands as the slide ends
	wx, wy := unit(g.P[1].X-g.P[0].X, g.P[1].Y-g.P[0].Y)
	if math.Abs(g.P[0].KickX-wx) > 1e-9 || math.Abs(g.P[0].KickY-wy) > 1e-9 {
		t.Fatalf("kick aimed %v,%v, want %v,%v", g.P[0].KickX, g.P[0].KickY, wx, wy)
	}
}

func TestHighJumpKickWithNoStickSlidesTheWayItFaces(t *testing.T) {
	cfg := DefaultConfig()
	g := kickDuel(cfg, 1500)
	g.P[1].Y = 900
	press(g, 0, Input{Skill: true})
	steps(g, int(math.Ceil(cfg.Machamp.HighJumpKick.SlideDuration/dt))-1)
	if g.P[0].X <= 300 || g.P[0].Y != 250 {
		t.Fatalf("at %v,%v: it should have slid the way it faces (+x)", g.P[0].X, g.P[0].Y)
	}
}

func TestHighJumpKickHurtsStunsAndKicksAwayWhatItReachesAndStopsAtItsBody(t *testing.T) {
	cfg := DefaultConfig()
	hk := cfg.Machamp.HighJumpKick
	g := kickDuel(cfg, 600)
	press(g, 0, Input{Skill: true, DX: 1})
	peak := 0.0
	for n := 0; n < 200 && g.P[1].HP == g.maxHP(1); n++ {
		peak = math.Max(peak, g.P[0].Z)
		g.P[0].In = Input{}
		g.Step(dt)
	}
	if g.P[1].HP != g.maxHP(1)-hk.Damage {
		t.Fatalf("HP %d, want %d", g.P[1].HP, g.maxHP(1)-hk.Damage)
	}
	if math.Abs(g.P[1].StunT-hk.StunDuration) > 2*dt {
		t.Fatalf("stun %v, want %v", g.P[1].StunT, hk.StunDuration)
	}
	if got := g.P[1].X - g.P[0].X; got < g.radius(0)+g.radius(1)-1e-6 {
		t.Fatalf("Machamp ended inside the body: gap %v", got)
	}
	if g.P[0].KickPhase != kickNone || g.P[0].Z != 0 {
		t.Fatalf("still kicking after the hit: phase %d z=%v", g.P[0].KickPhase, g.P[0].Z)
	}
	if peak <= 0 {
		t.Fatal("the kick never left the ground")
	}
	// kicked away: from where the bodies touched
	if got := g.P[1].X - g.P[0].X; got < hk.Knockback {
		t.Fatalf("pushed to %v from it, want at least %v", got, hk.Knockback)
	}
	x := g.P[1].X
	for g.P[1].StunT > 0 { // stunned: deaf to its keys
		g.P[1].In = Input{DX: -1}
		g.Step(dt)
		if g.P[1].X != x {
			t.Fatal("a stunned Snorlax walked")
		}
	}
}

func TestHighJumpKickThatFindsNobodyEndsAtItsDistanceAndSteppingOutOfTheLineDodgesIt(t *testing.T) {
	cfg := DefaultConfig()
	hk := cfg.Machamp.HighJumpKick
	g := kickDuel(cfg, hk.SlideDistance+hk.KickDistance+g0radii(cfg)+hk.Radius+80) // out of reach
	press(g, 0, Input{Skill: true, DX: 1})
	steps(g, 100)
	if g.P[1].HP != g.maxHP(1) || g.P[1].StunT > 0 {
		t.Fatal("hit a body beyond its reach")
	}
	if g.P[0].KickPhase != kickNone || g.P[0].Z != 0 {
		t.Fatalf("still kicking: phase %d z=%v", g.P[0].KickPhase, g.P[0].Z)
	}
	if got := g.P[0].X - 300; math.Abs(got-(hk.SlideDistance+hk.KickDistance)) > 1e-6 {
		t.Fatalf("travelled %v, want slide+kick = %v", got, hk.SlideDistance+hk.KickDistance)
	}
	// a body that leaves the line while the kick flies is missed: the way is fixed as the slide ends
	g = kickDuel(cfg, 600)
	press(g, 0, Input{Skill: true, DX: 1})
	for g.P[0].KickPhase != kickFly {
		g.Step(dt)
	}
	g.P[1].Y = 250 + hk.Radius + g.radius(0) + g.radius(1) + 20
	steps(g, 60)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("the kick turned after a body that stepped aside")
	}
}

func g0radii(cfg Config) float64 { return cfg.radiusOf(machamp) + cfg.radiusOf(snorlax) }

func TestHighJumpKickHasItsHeightBandAndCooldownAndMachampHasNoTree(t *testing.T) {
	cfg := DefaultConfig()
	hk := cfg.Machamp.HighJumpKick
	g := kickDuel(cfg, 600)
	g.P[1].Z = hk.MaxHeight + 30 // above its band (held there: the Snorlax is on the ground, so Z is reset each tick; set it each time)
	press(g, 0, Input{Skill: true, DX: 1})
	for range 60 {
		g.P[1].Z = hk.MaxHeight + 30
		g.Step(dt)
	}
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("hit something above its band")
	}
	if g.P[0].TreePhase != treeNone || g.P[0].TreeCD != 0 {
		t.Fatal("Machamp started the tree")
	}
	press(g, 0, Input{Skill: true})
	if g.P[0].KickPhase != kickNone {
		t.Fatal("a second kick started during the cooldown")
	}
	g.P[0].KickCD = 0
	press(g, 0, Input{Skill: true})
	if g.P[0].KickPhase == kickNone {
		t.Fatal("it would not start once the cooldown was over")
	}
	a := duel(cfg, ash, snorlax)
	press(a, 0, Input{Skill: true, DX: 1})
	if a.P[0].KickPhase != kickNone || a.P[0].TreePhase == treeNone {
		t.Fatal("Ash's skill is still the tree")
	}
}

func TestBeingStunnedOrFrozenInTheMiddleOfTheKickBringsMachampDown(t *testing.T) {
	for _, cut := range []string{"stun", "freeze"} {
		g := kickDuel(DefaultConfig(), 1500)
		g.P[1].Y = 900
		press(g, 0, Input{Skill: true, DX: 1})
		for g.P[0].KickPhase != kickFly {
			g.Step(dt)
		}
		steps(g, 3)
		if g.P[0].Z <= 0 {
			t.Fatal("not in the air")
		}
		if cut == "stun" {
			g.P[0].StunT = 0.5
		} else {
			g.P[0].FrozenT = 0.5
		}
		steps(g, 3)
		if g.P[0].KickPhase != kickNone || g.P[0].Z != 0 {
			t.Fatalf("%s: phase %d z %v", cut, g.P[0].KickPhase, g.P[0].Z)
		}
	}
}

func TestTheConfigRefusesNonsenseKickNumbers(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"kick distance": func(c *Config) { c.Machamp.HighJumpKick.KickDistance = 0 },
		"kick speed":    func(c *Config) { c.Machamp.HighJumpKick.KickSpeed = 0 },
		"slide":         func(c *Config) { c.Machamp.HighJumpKick.SlideDistance = -1 },
		"stun":          func(c *Config) { c.Machamp.HighJumpKick.StunDuration = -1 },
		"knockback":     func(c *Config) { c.Machamp.HighJumpKick.Knockback = -1 },
		"height band":   func(c *Config) { c.Machamp.HighJumpKick.MaxHeight = -5 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The kick is fast, so a slow body cannot get away; but a quick one must dodge it far more often than one that stands still.
func TestTheBotKeepsOutOfTheLineOfAHighJumpKickFarBetterThanOneThatStandsStill(t *testing.T) {
	cfg := DefaultConfig()
	for _, kind := range []int{pikachu, gengar} { // the slow ones (140 px/s) cannot get clear of a 1000 px/s kick in time
		stood, dodged := 0, 0
		for _, d := range []float64{380, 450, 520} {
			for seed := uint64(1); seed <= 5; seed++ {
				for _, bot := range []bool{false, true} {
					g := duel(cfg, kind, machamp)
					g.P[1].X, g.P[1].Y = 1300, 450
					g.P[0].X, g.P[0].Y = 1300-d, 450 // in the line
					press(g, 1, Input{Skill: true})
					b := newBot(kind, seed)
					hp := g.P[0].HP
					for range 8 {
						b.observe(g, 0)
					}
					for range 36 {
						if bot {
							g.P[0].In = b.Decide(g, 0)
						} else {
							g.P[0].In = Input{}
						}
						g.Step(dt)
					}
					if bot {
						dodged += hp - g.P[0].HP
					} else {
						stood += hp - g.P[0].HP
					}
				}
			}
		}
		if stood == 0 {
			t.Fatalf("%s: standing still lost nothing, so the scene proves nothing", NAMEOF[kind])
		}
		if dodged*2 > stood {
			t.Errorf("%s: lost %d HP, against %d for standing still: it hardly dodges", NAMEOF[kind], dodged, stood)
		}
	}
}

// ---- the computer plays Machamp, and the computer's Ash kicks too ----

func TestTheComputersAshKicksWithItsOwnNumbersAndAPersonsKeyForItIsDropped(t *testing.T) {
	cfg := DefaultConfig()
	hk := cfg.Ash.HighJumpKick
	g := duel(cfg, ash, snorlax)
	g.P[1].HP = g.maxHP(1)
	press(g, 0, Input{Kick: true, DX: 1})
	if g.P[0].KickPhase != kickSlide || g.P[0].TreePhase != treeNone {
		t.Fatalf("Kick: kick phase %d tree phase %d", g.P[0].KickPhase, g.P[0].TreePhase)
	}
	g.P[1].X, g.P[1].Y = 300+hk.SlideDistance+200, 250
	steps(g, 100)
	if g.P[1].HP != g.maxHP(1)-hk.Damage {
		t.Fatalf("HP %d, want Ash's own damage %d", g.maxHP(1)-g.P[1].HP, hk.Damage)
	}
	// Machamp's key is Skill, and the kick key does nothing for it
	m := duel(cfg, machamp, snorlax)
	press(m, 0, Input{Kick: true})
	if m.P[0].KickPhase != kickNone {
		t.Fatal("Machamp kicked with the computer's key")
	}

	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	c := dialPlay(t, srv, "dummy")
	next(t, c, "hi")
	pick(t, c, ash)
	c.WriteMessage(websocket.TextMessage, []byte(`{"kick":true,"skill":false}`))
	for i := 0; i < 30; i++ {
		p := next(t, c, "s")["p"].([]any)[0].(map[string]any)
		if p["kkp"].(float64) > 0 || p["kkcd"].(float64) > 0 {
			t.Fatalf("a person's Ash kicked: %v", p["kkp"])
		}
	}
}

func TestTheMachampBotKicksAtAFarOpponentAndNeverReachesForTheTreeOrAKeyItDoesNotHave(t *testing.T) {
	cfg := DefaultConfig()
	kicked := 0
	for seed := uint64(1); seed <= 20; seed++ {
		g := duel(cfg, machamp, snorlax)
		g.P[1].X, g.P[1].Y = 700, 250
		g.P[1].JumpCD = 99
		g.P[0].CometCD, g.P[0].RockCD = 99, 99
		b := newBot(machamp, seed)
		for i := 0; i < 90; i++ {
			in := b.Decide(g, 0)
			if in.Mega || in.Kick || in.Thunder || in.Beam {
				t.Fatalf("Machamp's bot pressed a key that is not its: %+v", in)
			}
			if in.Skill {
				kicked++
				break
			}
		}
	}
	if kicked < 5 {
		t.Fatalf("it kicked in %d of 20 scenes", kicked)
	}
}

func TestTheAshBotKicksToo(t *testing.T) {
	cfg := DefaultConfig()
	kicked := 0
	for seed := uint64(1); seed <= 20; seed++ {
		g := duel(cfg, ash, snorlax)
		g.P[1].X, g.P[1].Y = 700, 250
		g.P[1].JumpCD = 99
		g.P[0].CometCD, g.P[0].RockCD, g.P[0].TreeCD, g.P[0].MegaCD = 99, 99, 99, 99
		b := newBot(ash, seed)
		for i := 0; i < 90; i++ {
			if in := b.Decide(g, 0); in.Kick {
				kicked++
				break
			}
		}
	}
	if kicked < 5 {
		t.Fatalf("it kicked in %d of 20 scenes", kicked)
	}
}

func TestAMachampBotFightsAndLandsBlowsOnAnOpponentThatStandsStill(t *testing.T) {
	for _, kind := range []int{ash, machamp} {
		g := duel(DefaultConfig(), kind, snorlax)
		g.P[1].JumpCD = 99
		b := newBot(kind, 3)
		for range 30 * 30 {
			g.P[0].In = b.Decide(g, 0)
			g.Step(dt)
		}
		if g.P[1].HP >= g.maxHP(1) {
			t.Fatalf("%s's bot did no harm in 30 seconds", NAMEOF[kind])
		}
	}
}

func TestShowWarningOfTheKickIsSentToThePageAndIsOnForAshOnlyByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Ash.HighJumpKick.ShowWarning || cfg.Machamp.HighJumpKick.ShowWarning {
		t.Fatal("defaults: the names are for Ash's kick, not Machamp's")
	}
	cfg.Ash.HighJumpKick.ShowWarning, cfg.Machamp.HighJumpKick.ShowWarning = false, true
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	kw := next(t, dialPlay(t, srv, "dummy"), "hi")["kickwarn"].(map[string]any)
	if kw["ash"] != float64(0) || kw["machamp"] != float64(1) {
		t.Fatalf("kickwarn = %v", kw)
	}
}

func TestHighJumpKickWaitsKickDelayAfterTheSlideAndAimsWhenTheWaitEnds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Machamp.HighJumpKick.KickDelay = 0.5
	hk := cfg.Machamp.HighJumpKick
	g := kickDuel(cfg, 700)
	press(g, 0, Input{Skill: true, DY: 1})
	for g.P[0].KickPhase == kickSlide {
		g.Step(dt)
	}
	if g.P[0].KickPhase != kickWait {
		t.Fatalf("phase %d after the slide, want the wait", g.P[0].KickPhase)
	}
	x, y, n := g.P[0].X, g.P[0].Y, 0
	g.P[1].X, g.P[1].Y = 900, 100 // the opponent moves while it waits
	for g.P[0].KickPhase == kickWait {
		if g.P[0].X != x || g.P[0].Y != y || g.P[0].Z != 0 {
			t.Fatal("moved during the wait")
		}
		g.Step(dt)
		n++
	}
	if want := int(math.Ceil(hk.KickDelay / dt)); n != want {
		t.Fatalf("wait took %d ticks, want %d", n, want)
	}
	wx, wy := unit(g.P[1].X-g.P[0].X, g.P[1].Y-g.P[0].Y)
	if g.P[0].KickPhase != kickFly || math.Abs(g.P[0].KickX-wx) > 1e-9 || math.Abs(g.P[0].KickY-wy) > 1e-9 {
		t.Fatalf("phase %d aim %v,%v, want the kick aimed at %v,%v", g.P[0].KickPhase, g.P[0].KickX, g.P[0].KickY, wx, wy)
	}
}
