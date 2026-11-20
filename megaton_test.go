package main

import (
	"math"
	"net/http/httptest"
	"testing"
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
	pressMega(g, 0, Input{Mega: true, DX: 1}) // the stick does not move it: it is rooted
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
	if want := int(math.Ceil((mp.RingDuration + mp.BallDuration) / dt)); n != want {
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
	pressMega(g, 0, Input{Mega: true, AimX: 1})
	g.P[1].Y = 250 + mp.Radius + g.radius(1) + 10 // out of the line the stick holds
	for range int((mp.RingDuration+mp.BallDuration)/dt) + 2 {
		g.P[0].In = Input{AimX: 1}
		g.Step(dt)
	}
	if g.P[1].HP != g.maxHP(1) || g.P[1].StunT > 0 {
		t.Fatal("hit a body that was beside the line")
	}
	g = megaDuel(cfg, mp.Distance+g.radius(1)+mp.Radius+20)
	pressMega(g, 0, Input{Mega: true})
	steps(g, int((mp.RingDuration+mp.BallDuration)/dt)+2)
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
	pressMega(g, 0, Input{Mega: true})
	steps(g, int((mp.RingDuration+mp.BallDuration)/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("hit something above its band")
	}
	g.P[0].MegaT = 0
	pressMega(g, 0, Input{Mega: true})
	if g.P[0].MegaT != 0 {
		t.Fatal("a second punch started during the cooldown")
	}
	g.P[0].MegaCD = 0
	pressMega(g, 0, Input{Mega: true})
	if g.P[0].MegaT == 0 {
		t.Fatal("it would not start once the cooldown was over")
	}

	m := megaDuel(cfg, 300)
	m.SetKinds(machamp, snorlax)
	pressMega(m, 0, Input{Mega: true})
	if m.P[0].MegaT != 0 || m.P[0].MegaCD != 0 {
		t.Fatal("Machamp threw the Megaton Punch")
	}
}

func TestBeingStunnedCutsTheMegatonChargeShort(t *testing.T) {
	g := megaDuel(DefaultConfig(), 300)
	pressMega(g, 0, Input{Mega: true})
	g.P[0].StunT = 0.5
	steps(g, 3)
	if g.P[0].MegaT != 0 || g.P[1].HP != g.maxHP(1) {
		t.Fatal("the punch went on through a stun")
	}
}

func TestAnyOtherSkillWaitsWhileTheMegatonPunchGathers(t *testing.T) {
	g := megaDuel(DefaultConfig(), 300)
	pressMega(g, 0, Input{Mega: true})
	press(g, 0, Input{Skill: true, Comet: true, Rock: true})
	if g.P[0].TreePhase != treeNone || g.P[0].CometT != 0 || g.P[0].RockT != 0 {
		t.Fatal("another skill started in the middle of the charge")
	}
}

func TestMegatonPunchWithNoChargeLandsOnTheTickItIsPressed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.RingDuration, cfg.Ash.MegatonPunch.BallDuration = 0, 0
	g := megaDuel(cfg, 300)
	pressMega(g, 0, Input{Mega: true})
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
		"charge":      func(c *Config) { c.Ash.MegatonPunch.RingDuration = -1 },
		"ball":        func(c *Config) { c.Ash.MegatonPunch.BallDuration = -1 },
		"bolt start":  func(c *Config) { c.Ash.MegatonPunch.BoltStart = -1 },
		"bolt gap":    func(c *Config) { c.Ash.MegatonPunch.BoltInterval = -1 },
		"bolt count":  func(c *Config) { c.Ash.MegatonPunch.BoltCount = 21 },
		"bolt none":   func(c *Config) { c.Ash.MegatonPunch.BoltCount = -1 },
		"follow size": func(c *Config) { c.Ash.MegatonPunch.BallFollowScale = 0 },
		"follow time": func(c *Config) { c.Ash.MegatonPunch.BallFollowDuration = -1 },
		"hit size":    func(c *Config) { c.Ash.MegatonPunch.BallHitScale = 0 },
		"hit time":    func(c *Config) { c.Ash.MegatonPunch.BallHitDuration = -1 },
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
	g.P[0].Charge = g.cfg.Ash.Ultimate.Max // it is his ultimate
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
			pressMega(g, 1, Input{Mega: true})
			b := newBot(kind, seed)
			for i := 0; i < 8; i++ {
				b.observe(g, 0)
			}
			if th := b.look(g, 0).th; th.kind != thPunch {
				t.Fatalf("%s: threat %+v, want the punch", NAMEOF[kind], th)
			}
			x0 := g.P[0].X
			survive(g, b, 0, (mp.RingDuration+mp.BallDuration)*0.9)
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
	pressMega(g, 0, Input{Mega: true, DX: 0, DY: 1}) // stick down, the opponent is to the right
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
	pressMega(g, 0, Input{Mega: true})
	steps(g, int(cfg.Ash.MegatonPunch.JumpDuration/dt)+1)
	if g.P[0].X != 300 || g.P[0].Y != 250 || g.P[0].MegaT <= 0 {
		t.Fatalf("at %v,%v charge %v", g.P[0].X, g.P[0].Y, g.P[0].MegaT)
	}
	cfg.Ash.MegatonPunch.JumpDuration = 0
	g = megaDuelLeap(cfg, 300)
	pressMega(g, 0, Input{Mega: true, DX: 1})
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
	pressMega(g, 0, Input{Mega: true, DX: -1})
	steps(g, 40)
	if g.P[0].X < 0 {
		t.Fatalf("left the court: x = %v", g.P[0].X)
	}
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.JumpHeight = 0 // flat leap: only the leap's own state keeps the others out
	g = megaDuelLeap(cfg, 300)
	pressMega(g, 0, Input{Mega: true})
	press(g, 0, Input{Skill: true, Comet: true, Rock: true})
	if g.P[0].TreePhase != treeNone || g.P[0].CometT != 0 || g.P[0].RockT != 0 {
		t.Fatal("another skill started in the middle of the leap")
	}
}

func TestBeingStunnedInTheMiddleOfTheLeapBringsItDownAndCancelsThePunch(t *testing.T) {
	g := megaDuelLeap(DefaultConfig(), 300)
	pressMega(g, 0, Input{Mega: true, DX: 1})
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

func TestAPersonsKeysMeanTheirOwnCharactersSkills(t *testing.T) {
	// Ash: the main button (sent as Skill) is the kick, the second one the tree, Z the Megaton Punch, and the rock has no button.
	in := personsInput(ash, Input{Skill: true, Tree: true, Comet: true, Rock: true, Ult: true, Kick: true, Mega: true})
	if !in.Kick || !in.Skill || !in.Comet || !in.Mega || in.Rock || in.Tree {
		t.Fatalf("Ash: %+v", in)
	}
	in = personsInput(ash, Input{Skill: true})
	if !in.Kick || in.Skill || in.Mega {
		t.Fatalf("Ash, only the main button: %+v", in)
	}
	// Machamp: Skill is the Uppercut and K (sent as Rock) the High Jump Kick; the others are left as they were (and the computer's Kick, Tree and Night Shade are not a person's)
	in = personsInput(machamp, Input{Skill: true, Tree: true, Kick: true, Mega: true, Rock: true, Shade: true, DX: 7})
	if !in.Skill || !in.Kick || in.Tree || !in.Mega || in.Rock || in.Shade || in.DX != 1 { // the K button (Rock) is its kick, and the Rock Tomb has no key
		t.Fatalf("Machamp: %+v", in)
	}
}

func TestTheMegatonPunchIsAshsUltimateAndNeedsTheChargeFull(t *testing.T) {
	cfg := DefaultConfig()
	g := megaDuel(cfg, 300)
	g.P[0].Charge = cfg.Ash.Ultimate.Max - 1
	press(g, 0, Input{Mega: true})
	steps(g, 5)
	if g.P[0].MegaT != 0 || g.P[0].MegaJT != 0 || g.P[0].MegaCD != 0 {
		t.Fatal("the Megaton Punch started with the charge not full")
	}
	g.P[0].Charge = cfg.Ash.Ultimate.Max
	g.P[0].MegaCD = 0
	press(g, 0, Input{Mega: true})
	if g.P[0].Charge >= 1 || (g.P[0].MegaT == 0 && g.P[0].MegaJT == 0) {
		t.Fatalf("charge %v, gathering %v, leap %v: the cast did not start and empty the charge", g.P[0].Charge, g.P[0].MegaT, g.P[0].MegaJT)
	}
	// the charge fills like Pikachu's: by the damage taken and dealt, and by time
	cfg.Ash.Ultimate = Ultimate{Max: 100, GainTaken: 2, GainDealt: 3, GainPerSecond: 0}
	g = megaDuel(cfg, 300)
	hp0 := [maxSlots]int{g.P[0].HP, g.P[1].HP}
	g.P[0].HP -= 5
	g.P[1].HP -= 4
	g.gainCharge(hp0, dt)
	if g.P[0].Charge != 2*5+3*4 {
		t.Fatalf("charge %v, want %v", g.P[0].Charge, 2*5+3*4)
	}
}

func TestTheStickTurnsTheMegatonFistWhileItGathersAndWithoutItTheFistFollowsTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	g := megaDuel(cfg, 300) // the opponent is straight ahead, on the right
	pressMega(g, 0, Input{Mega: true})
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
	steps(g, int((cfg.Ash.MegatonPunch.RingDuration+cfg.Ash.MegatonPunch.BallDuration)/dt)+2)
	if g.P[1].HP != g.maxHP(1) {
		t.Fatal("the fist hit the opponent it was turned away from")
	}
	if g.P[0].Y <= 250 {
		t.Fatal("Ash did not lunge the way it was turned")
	}
}

// The gather is the rings' time plus the ball's, and the page gets both parts and the bolts.
func TestMegatonChargeIsRingPlusBallAndTheBoltsReachThePage(t *testing.T) {
	cfg := DefaultConfig()
	mp := &cfg.Ash.MegatonPunch
	mp.RingDuration, mp.BallDuration, mp.BoltStart, mp.BoltCount, mp.BoltInterval, mp.BoltOffsets = 0.3, 0.7, 0.2, 3, 0.25, []float64{-80, 120}
	mp.JumpDuration = 0
	g := megaDuel(cfg, 300)
	pressMega(g, 0, Input{Mega: true})
	if got := g.P[0].MegaT; math.Abs(got-(1.0-dt)) > 1e-9 { // one tick of it has already run
		t.Fatalf("charge = %v, want ring + ball = 1 less a tick", got)
	}
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	m := next(t, dialPlay(t, srv, "dummy"), "hi")["mega"].(map[string]any)
	offs, _ := m["boffs"].([]any)
	if m["charge"] != 1.0 || m["ring"] != 0.3 || m["bstart"] != 0.2 || m["bcount"] != 3.0 || m["bint"] != 0.25 || len(offs) != 2 || offs[0] != -80.0 || offs[1] != 120.0 {
		t.Fatalf("mega = %v", m)
	}
}

// The ball that goes forward with Ash needs to know whose it is, and the page gets its sizes and times apart from the burst's.
func TestMegatonEventNamesTheThrowerAndTheBallSettingsReachThePage(t *testing.T) {
	cfg := DefaultConfig()
	mp := &cfg.Ash.MegatonPunch
	mp.BallFollowScale, mp.BallFollowDuration, mp.BallHitScale, mp.BallHitDuration = 1.5, 0.4, 2.5, 0.9
	mp.RingDuration, mp.BallDuration = 0, 0
	for slot := 0; slot < 2; slot++ { // thrown at once, by Ash in either seat
		g := megaDuel(cfg, 300)
		if slot == 1 {
			g.P[0].Kind, g.P[1].Kind = snorlax, ash
			g.P[0].X, g.P[1].X = 600, 300
		}
		ev := pressMega(g, slot, Input{Mega: true})
		if len(ev) == 0 || ev[0].T != "mega" || ev[0].S != slot {
			t.Fatalf("slot %d: events = %+v", slot, ev)
		}
	}
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	m := next(t, dialPlay(t, srv, "dummy"), "hi")["mega"].(map[string]any)
	if m["fwscale"] != 1.5 || m["fwdur"] != 0.4 || m["htscale"] != 2.5 || m["htdur"] != 0.9 {
		t.Fatalf("mega = %v", m)
	}
}

func TestMegatonPunchHeldByAVineLeapsAndLungesNowhere(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 900, 250
	pressMega(g, 0, Input{Mega: true, DY: 1})
	steps(g, 2) // in the air already
	x, y := g.P[0].X, g.P[0].Y
	g.P[0].BoundT = 10
	steps(g, int((mp.JumpDuration+mp.RingDuration+mp.BallDuration)/dt)+3)
	if g.P[0].X != x || g.P[0].Y != y || g.P[0].Z != 0 {
		t.Fatalf("at %v,%v z=%v, want it held at %v,%v", g.P[0].X, g.P[0].Y, g.P[0].Z, x, y)
	}
	if g.P[0].MegaT != 0 || g.P[0].MegaJT != 0 {
		t.Fatal("the punch did not run its course")
	}
}

// pressMega is press for a Megaton Punch: it is Ash's ultimate, so the charge is made full first.
func pressMega(g *Game, i int, in Input) []Event {
	g.P[i].Charge = g.cfg.Ash.Ultimate.Max
	return press(g, i, in)
}
