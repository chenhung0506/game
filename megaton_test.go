package main

import (
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

// megaDuel is Ash (slot 0, at 300,250) against a Snorlax (slot 1) `d` ahead of it on the same line.
func megaDuel(cfg Config, d float64) *Game {
	cfg.Ash.MegatonPunch.JumpDuration = 0 // the tests that use it are about the gather and the fist; the leap has its own tests
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+d, 250
	return g
}

func TestMegatonPunchGathersRootedThenHurtsStunsAndKnocksBackWhatItReaches(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := megaDuel(cfg, 300)
	press(g, 0, Input{Mega: true, DX: 1}) // the stick does not move it: it is rooted
	n := 1
	for g.P[0].MegaT > 0 {
		if g.P[1].HP != g.maxHP(1) || g.P[1].StunT > 0 {
			t.Fatal("it hit before the charge was over")
		}
		if g.P[0].X != 300 {
			t.Fatalf("Ash walked while it gathered: x = %v", g.P[0].X)
		}
		g.P[0].In = Input{DX: 1}
		g.Step(dt)
		n++
	}
	if want := int(math.Ceil(mp.ChargeDuration / dt)); n != want {
		t.Fatalf("charge took %d ticks, want %d", n, want)
	}
	if g.P[1].HP != g.maxHP(1)-mp.Damage {
		t.Fatalf("HP %d, want %d", g.P[1].HP, g.maxHP(1)-mp.Damage)
	}
	if math.Abs(g.P[1].StunT-mp.StunDuration) > 2*dt {
		t.Fatalf("stun %v, want %v", g.P[1].StunT, mp.StunDuration)
	}
	if got := g.P[1].X - 600; math.Abs(got-mp.Knockback) > 1e-6 {
		t.Fatalf("knocked back %v, want %v", got, mp.Knockback)
	}
	x := g.P[1].X
	for g.P[1].StunT > 0 { // stunned: deaf to its keys
		g.P[1].In = Input{DX: 1}
		g.Step(dt)
		if g.P[1].X != x {
			t.Fatal("a stunned Snorlax walked")
		}
	}
}

func TestMegatonPunchFliesTheLineTheStickHoldsSoABodyBesideItIsMissed(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := megaDuel(cfg, 300)
	press(g, 0, Input{Mega: true, AimX: 1})
	g.P[1].Y = 250 + mp.Radius + g.radius(1) + 10 // out of the line the stick holds
	for range int(mp.ChargeDuration/dt) + 2 {
		g.P[0].In = Input{AimX: 1}
		g.Step(dt)
	}
	if g.P[1].HP != g.maxHP(1) || g.P[1].StunT > 0 {
		t.Fatal("hit a body that was beside the line")
	}
	g = megaDuel(cfg, mp.Distance+g.radius(1)+mp.Radius+20)
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("hit a body beyond its reach")
	}
}

func TestMegatonPunchHasItsHeightBandAndCooldownAndBelongsToAshAlone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.JumpDuration = 0
	mp := cfg.Ash.MegatonPunch
	g := flyer(cfg, mp.MaxHeight+30)
	g.P[1].X, g.P[1].Y = 600, 250
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("hit something above its band")
	}
	g.P[0].MegaT = 0
	press(g, 0, Input{Mega: true})
	if g.P[0].MegaT != 0 {
		t.Fatal("a second punch started during the cooldown")
	}
	g.P[0].MegaCD = 0
	press(g, 0, Input{Mega: true})
	if g.P[0].MegaT == 0 {
		t.Fatal("it would not start once the cooldown was over")
	}

	m := megaDuel(cfg, 300)
	m.SetKinds(machamp, snorlax)
	press(m, 0, Input{Mega: true})
	if m.P[0].MegaT != 0 || m.P[0].MegaCD != 0 {
		t.Fatal("Machamp threw the Megaton Punch")
	}
}

func TestBeingStunnedCutsTheMegatonChargeShort(t *testing.T) {
	g := megaDuel(DefaultConfig(), 300)
	press(g, 0, Input{Mega: true})
	g.P[0].StunT = 0.5
	steps(g, 3)
	if g.P[0].MegaT != 0 || g.P[1].HP != g.maxHP(1) {
		t.Fatal("the punch went on through a stun")
	}
}

func TestAnyOtherSkillWaitsWhileTheMegatonPunchGathers(t *testing.T) {
	g := megaDuel(DefaultConfig(), 300)
	press(g, 0, Input{Mega: true})
	press(g, 0, Input{Skill: true, Comet: true, Rock: true})
	if g.P[0].TreePhase != treeNone || g.P[0].CometT != 0 || g.P[0].RockT != 0 {
		t.Fatal("another skill started in the middle of the charge")
	}
}

func TestMegatonPunchWithNoChargeLandsOnTheTickItIsPressed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.ChargeDuration = 0
	g := megaDuel(cfg, 300)
	press(g, 0, Input{Mega: true})
	if g.P[1].HP != g.maxHP(1)-cfg.Ash.MegatonPunch.Damage {
		t.Fatal("no immediate hit")
	}
}

func TestTheTreeStunsWhatItLandsOnForTheConfiguredTime(t *testing.T) {
	for _, kind := range []int{ash} { // Machamp kicks instead of felling a tree
		cfg := DefaultConfig()
		cfg.Ash.TreeSlam.StunDuration = 1.7
		ts := cfg.brawlerOf(kind).TreeSlam
		g := duel(cfg, kind, snorlax)
		g.P[1].X, g.P[1].Y = 300+ts.JumpDistance+150, 250 // on the trunk once it has landed
		press(g, 0, Input{Skill: true, DX: 1})
		for g.P[0].TreePhase != treeNone {
			if g.P[1].StunT > 0 {
				t.Fatal("stunned before the tree came down")
			}
			g.Step(dt)
		}
		if g.P[1].HP != g.maxHP(1)-ts.Damage || math.Abs(g.P[1].StunT-1.7) > 2*dt {
			t.Fatalf("kind %d: HP %d stun %v", kind, g.P[1].HP, g.P[1].StunT)
		}
	}
}

func TestATreeThatMissesDoesNotStun(t *testing.T) {
	g := duel(DefaultConfig(), ash, snorlax)
	g.P[1].X, g.P[1].Y = 1800, 900
	press(g, 0, Input{Skill: true, DX: 1})
	steps(g, 90)
	if g.P[1].StunT != 0 {
		t.Fatal("stunned by a tree that missed")
	}
}

func TestTheConfigRefusesNonsenseMegatonAndStunNumbers(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"distance":    func(c *Config) { c.Ash.MegatonPunch.Distance = 0 },
		"radius":      func(c *Config) { c.Ash.MegatonPunch.Radius = 0 },
		"stun":        func(c *Config) { c.Ash.MegatonPunch.StunDuration = -1 },
		"charge":      func(c *Config) { c.Ash.MegatonPunch.ChargeDuration = -1 },
		"knockback":   func(c *Config) { c.Ash.MegatonPunch.Knockback = -1 },
		"fist_scale":  func(c *Config) { c.Ash.MegatonPunch.FistScale = 0 },
		"tree stun":   func(c *Config) { c.Ash.TreeSlam.StunDuration = -1 },
		"mach stun":   func(c *Config) { c.Machamp.TreeSlam.StunDuration = -1 },
		"height band": func(c *Config) { c.Ash.MegatonPunch.MaxHeight = -5 },
	} {
		c := DefaultConfig()
		edit(&c)
		if c.validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// ---- the computer ----

func TestAshBotThrowsTheMegatonPunchAtAnOpponentThatCannotMove(t *testing.T) {
	g := ashDuel()
	g.P[0].CometCD, g.P[0].TreeCD, g.P[0].RockCD = 99, 99, 99
	g.P[1].StunT = 5
	b := newBot(ash, 1)
	in, ok := firstAct(b, g, 0, 30)
	if !ok || !in.Mega {
		t.Fatalf("decision %+v: the Megaton Punch was not thrown at a stunned opponent", in)
	}
}

// The fist follows its target, so stepping aside is no use: the computer runs out of its reach instead.
func TestTheBotRunsAwayFromAMegatonPunchThatIsGathering(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.JumpDuration = 0
	mp := cfg.Ash.MegatonPunch
	for _, kind := range []int{pikachu, snorlax, chikorita, ash, gengar} {
		for seed := uint64(1); seed <= 5; seed++ {
			g := duel(cfg, kind, ash)
			g.P[1].X, g.P[1].Y = 1300, 450
			g.P[0].X, g.P[0].Y = 1300-300, 450 // in the line
			g.P[1].CometCD, g.P[1].TreeCD, g.P[1].RockCD = 99, 99, 99
			press(g, 1, Input{Mega: true})
			b := newBot(kind, seed)
			for i := 0; i < 8; i++ {
				b.observe(g, 0)
			}
			if th := b.look(g, 0).th; th.kind != thPunch {
				t.Fatalf("%s: threat %+v, want the punch", NAMEOF[kind], th)
			}
			x0 := g.P[0].X
			survive(g, b, 0, mp.ChargeDuration*0.9)
			if x0-g.P[0].X < 30 {
				t.Fatalf("%s seed %d: moved %v away from the fist", NAMEOF[kind], seed, x0-g.P[0].X)
			}
		}
	}
}

// ---- Ghost Fire ----

func TestTheBotKeepsOutOfGhostFireFarBetterThanOneThatWalksStraightAtGengar(t *testing.T) {
	cfg := DefaultConfig()
	for _, kind := range []int{pikachu, snorlax, chikorita, ash} {
		walked, dodged := 0, 0
		for _, d := range []float64{120, 180, 240, 320} {
			for seed := uint64(1); seed <= 4; seed++ {
				for _, bot := range []bool{false, true} {
					g := duel(cfg, kind, gengar)
					g.P[1].X, g.P[1].Y = 1300, 450
					g.P[0].X, g.P[0].Y = 1300-d, 450
					g.P[1].SneakCD, g.P[1].SBallCD = 99, 99
					press(g, 1, Input{GFire: true})
					b := newBot(kind, seed)
					hp := g.P[0].HP
					for range 8 {
						b.observe(g, 0)
					}
					for range 90 {
						if bot {
							g.P[0].In = b.Decide(g, 0)
						} else {
							g.P[0].In = Input{DX: 1}
						}
						g.Step(dt)
					}
					if bot {
						dodged += hp - g.P[0].HP
					} else {
						walked += hp - g.P[0].HP
					}
				}
			}
		}
		if walked == 0 {
			t.Fatalf("%s: walking at Gengar lost nothing, so the scene proves nothing", NAMEOF[kind])
		}
		if dodged*2 > walked {
			t.Errorf("%s: lost %d HP, against %d for walking straight at it: it hardly dodges", NAMEOF[kind], dodged, walked)
		}
	}
}

// ---- the leap before the gather ----

func TestMegatonPunchLeapsAlongTheStickAndOnlyGathersOnceItHasLanded(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 900, 250
	press(g, 0, Input{Mega: true, DX: 0, DY: 1}) // stick down, the opponent is to the right
	peak := 0.0
	n := 1
	for g.P[0].MegaJT > 0 {
		if g.P[0].MegaT != 0 {
			t.Fatal("it was gathering in the air")
		}
		peak = math.Max(peak, g.P[0].Z)
		g.P[0].In = Input{DX: -1} // the stick moved on: the leap keeps the direction it began with
		g.Step(dt)
		n++
	}
	if want := int(math.Ceil(mp.JumpDuration / dt)); n != want {
		t.Fatalf("leap took %d ticks, want %d", n, want)
	}
	if g.P[0].X != 300 || math.Abs(g.P[0].Y-(250+mp.JumpDistance)) > 1e-6 || g.P[0].Z != 0 {
		t.Fatalf("landed at %v,%v z=%v, want 300,%v on the ground", g.P[0].X, g.P[0].Y, g.P[0].Z, 250+mp.JumpDistance)
	}
	if peak < mp.JumpHeight*0.9 {
		t.Fatalf("it only rose %v of %v", peak, mp.JumpHeight)
	}
	if g.P[0].MegaT <= 0 {
		t.Fatal("the charge did not begin on landing")
	}
	// the stick was still held (left) as it landed, and it turns the fist
	if g.P[0].MegaX != -1 || g.P[0].MegaY != 0 {
		t.Fatalf("aim %v,%v, want the stick's way", g.P[0].MegaX, g.P[0].MegaY)
	}
}

func TestMegatonPunchWithoutTheStickLeapsOnTheSpotAndNoLeapGathersAtOnce(t *testing.T) {
	cfg := DefaultConfig()
	g := megaDuelLeap(cfg, 300)
	press(g, 0, Input{Mega: true})
	steps(g, int(cfg.Ash.MegatonPunch.JumpDuration/dt)+1)
	if g.P[0].X != 300 || g.P[0].Y != 250 || g.P[0].MegaT <= 0 {
		t.Fatalf("at %v,%v charge %v", g.P[0].X, g.P[0].Y, g.P[0].MegaT)
	}
	cfg.Ash.MegatonPunch.JumpDuration = 0
	g = megaDuelLeap(cfg, 300)
	press(g, 0, Input{Mega: true, DX: 1})
	if g.P[0].MegaJT != 0 || g.P[0].MegaT <= 0 || g.P[0].X != 300 {
		t.Fatal("with no leap it should gather where it stands")
	}
}

func megaDuelLeap(cfg Config, d float64) *Game {
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+d, 250
	return g
}

func TestTheLeapStaysInTheCourtAndCannotBeStartedOrSkippedBySomethingElse(t *testing.T) {
	g := megaDuelLeap(DefaultConfig(), 300)
	g.P[0].X = 5
	press(g, 0, Input{Mega: true, DX: -1})
	steps(g, 40)
	if g.P[0].X < 0 {
		t.Fatalf("left the court: x = %v", g.P[0].X)
	}
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.JumpHeight = 0 // flat leap: only the leap's own state keeps the others out
	g = megaDuelLeap(cfg, 300)
	press(g, 0, Input{Mega: true})
	press(g, 0, Input{Skill: true, Comet: true, Rock: true})
	if g.P[0].TreePhase != treeNone || g.P[0].CometT != 0 || g.P[0].RockT != 0 {
		t.Fatal("another skill started in the middle of the leap")
	}
}

func TestBeingStunnedInTheMiddleOfTheLeapBringsItDownAndCancelsThePunch(t *testing.T) {
	g := megaDuelLeap(DefaultConfig(), 300)
	press(g, 0, Input{Mega: true, DX: 1})
	steps(g, 3)
	if g.P[0].Z <= 0 {
		t.Fatal("not in the air")
	}
	g.P[0].StunT = 0.5
	steps(g, 3)
	if g.P[0].MegaJT != 0 || g.P[0].MegaT != 0 || g.P[0].Z != 0 {
		t.Fatalf("leap %v charge %v z %v after the stun", g.P[0].MegaJT, g.P[0].MegaT, g.P[0].Z)
	}
}

func TestAPersonsAshHasNoCometPunchButTheComputersDoes(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	c := dialPlay(t, srv, "dummy")
	next(t, c, "hi")
	pick(t, c, ash)
	c.WriteMessage(websocket.TextMessage, []byte(`{"comet":true}`))
	for i := 0; i < 30; i++ {
		p := next(t, c, "s")["p"].([]any)[0].(map[string]any)
		if p["cpt"] != nil && p["cpt"].(float64) > 0 || p["cpcd"] != nil && p["cpcd"].(float64) > 0 {
			t.Fatalf("a person's Ash threw the Comet Punch: %v", p)
		}
	}
	g := duel(DefaultConfig(), ash, snorlax)
	press(g, 0, Input{Comet: true})
	if g.P[0].CometT == 0 {
		t.Fatal("the game itself must still let Ash use it, for the computer")
	}
}

func TestTheStickTurnsTheMegatonFistWhileItGathersAndWithoutItTheFistFollowsTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	g := megaDuel(cfg, 300) // the opponent is straight ahead, on the right
	press(g, 0, Input{Mega: true})
	if g.P[0].MegaX != 1 || g.P[0].MegaY != 0 {
		t.Fatalf("starts aimed at %v,%v, want the opponent", g.P[0].MegaX, g.P[0].MegaY)
	}
	g.P[0].In = Input{DY: 1}
	g.Step(dt)
	if g.P[0].MegaX != 0 || g.P[0].MegaY != 1 {
		t.Fatalf("aim %v,%v after the stick went down", g.P[0].MegaX, g.P[0].MegaY)
	}
	g.P[0].In = Input{AimX: 0.9239, AimY: 0.3827} // 22.5 degrees: the pad is not limited to eight ways
	g.Step(dt)
	if math.Abs(g.P[0].MegaX-0.9239) > 1e-3 || math.Abs(g.P[0].MegaY-0.3827) > 1e-3 {
		t.Fatalf("aim %v,%v, want 22.5 degrees", g.P[0].MegaX, g.P[0].MegaY)
	}
	g.P[0].In = Input{}
	g.P[1].Y = 450 // the opponent moves; with no stick the fist follows it
	g.Step(dt)
	if want := 200 / math.Hypot(300, 200); math.Abs(g.P[0].MegaY-want) > 1e-6 || g.P[0].MegaX <= 0 {
		t.Fatalf("aim %v,%v does not follow the opponent", g.P[0].MegaX, g.P[0].MegaY)
	}
	g.P[0].In = Input{DY: 1}
	g.Step(dt)
	if g.P[0].MegaX != 0 || g.P[0].MegaY != 1 {
		t.Fatal("the keyboard's way did not turn it")
	}
	if g.P[0].X != 300 || g.P[0].Y != 250 {
		t.Fatal("turning the fist walked Ash")
	}
	steps(g, int(cfg.Ash.MegatonPunch.ChargeDuration/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("the fist hit the opponent it was turned away from")
	}
	if g.P[0].Y <= 250 {
		t.Fatal("Ash did not lunge the way it was turned")
	}
}
