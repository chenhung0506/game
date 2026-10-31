package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// ---- Magikarp and the Gyarados it evolves into ----

// magi is Magikarp (slot 0, at 300,250) against foe (slot 1, at 900,250).
func magi(foe int) *Game { return duel(DefaultConfig(), magikarp, foe) }

// gya is Gyarados (slot 0, at 300,250, full HP) against foe (slot 1, at 900,250).
func gya(cfg Config, foe int) *Game {
	g := duel(cfg, magikarp, foe)
	g.P[0].Kind = gyarados
	g.P[0].HP = g.maxHP(0)
	return g
}

// press holds the input for one tick and lets go.
func press(g *Game, slot int, in Input) []Event {
	g.P[slot].In = in
	ev := g.Step(dt)
	g.P[slot].In = Input{}
	return ev
}

func TestMagikarpJumpGoesUpHangsAtTheTopAndComesDownInExactlyTheConfiguredTime(t *testing.T) {
	cfg := DefaultConfig()
	mj := cfg.Magikarp.Jump
	g := magi(pikachu)
	press(g, 0, Input{Skill: true})
	zs := []float64{g.P[0].Z}
	for i := 0; i < int((mj.Duration+0.5)*tickRate); i++ {
		g.Step(dt)
		zs = append(zs, g.P[0].Z)
	}
	peak, air, top := 0.0, 0, 0
	for i, z := range zs {
		peak = math.Max(peak, z)
		if z > 0 {
			air++
		}
		if math.Abs(z-mj.Height) < 1e-9 {
			top++
		}
		if i > 0 && z < zs[i-1]-1e-9 && zs[i-1] < mj.Height-1e-9 && i < len(zs)/4 {
			t.Fatalf("it came down before the top: z[%d] = %v after %v", i, z, zs[i-1])
		}
	}
	if math.Abs(peak-mj.Height) > 1e-9 {
		t.Fatalf("peak %v, want %v", peak, mj.Height)
	}
	if want := int(mj.Duration * tickRate); air < want-1 || air > want {
		t.Fatalf("%d ticks in the air, want %d (duration %v s)", air, want, mj.Duration)
	}
	if want := int(mj.HighestPointDuration * tickRate); top < want-1 || top > want+1 {
		t.Fatalf("%d ticks at the top, want about %d (highest_point_duration %v s)", top, want, mj.HighestPointDuration)
	}
	if g.P[0].Z != 0 || g.P[0].HopLeft != 0 {
		t.Fatalf("after landing: Z %v, HopLeft %v", g.P[0].Z, g.P[0].HopLeft)
	}
}

func TestMagikarpJumpHurtsNobodyAndIsOutOfReachOfGroundSkills(t *testing.T) {
	g := magi(pikachu)
	g.P[0].X, g.P[0].Y = 600, 250
	g.P[1].X, g.P[1].Y = 700, 250 // right beside it
	press(g, 0, Input{Skill: true})
	steps(g, 20) // at the top by now
	if g.P[0].Z < 100 {
		t.Fatalf("Z = %v, the jump should be high up", g.P[0].Z)
	}
	hp, hp1 := g.P[0].HP, g.P[1].HP
	g.P[1].In = Input{Skill: true, DX: -1} // Volt Tackle straight through where it stands: it reaches only 0..60
	steps(g, 8)
	g.P[1].In = Input{}
	if g.P[0].HP != hp || g.P[0].ParalyzeT != 0 {
		t.Fatalf("HP %d -> %d, paralyzed %v: a ground skill hit a jumping Magikarp", hp, g.P[0].HP, g.P[0].ParalyzeT)
	}
	steps(g, 80)
	if g.P[1].HP != hp1 {
		t.Fatalf("the opponent lost HP (%d -> %d) to a Magikarp's landing", hp1, g.P[1].HP)
	}
}

func TestMagikarpJumpCoolsDownFromTheTakeOff(t *testing.T) {
	cfg := DefaultConfig()
	g := magi(pikachu)
	var takeoffs []int
	for tick := 1; tick <= int(7*tickRate); tick++ {
		before := g.P[0].HopLeft
		g.P[0].In = Input{Skill: true} // held down all the time
		g.Step(dt)
		if before == 0 && g.P[0].HopLeft > 0 {
			takeoffs = append(takeoffs, tick)
		}
	}
	if len(takeoffs) != 3 || takeoffs[0] != 1 {
		t.Fatalf("take-offs at ticks %v, want three, the first at once", takeoffs)
	}
	for i := 1; i < len(takeoffs); i++ {
		if gap := float64(takeoffs[i]-takeoffs[i-1]) / tickRate; math.Abs(gap-cfg.Magikarp.Jump.Cooldown) > 2*dt {
			t.Fatalf("take-offs %v apart: %v s, want the cooldown %v", takeoffs, gap, cfg.Magikarp.Jump.Cooldown)
		}
	}
}

func TestMagikarpIsSteeredAtOneSpeedForTheFirstHalfOfTheJumpAndAnotherForTheSecond(t *testing.T) {
	cfg := DefaultConfig()
	mj := cfg.Magikarp.Jump
	g := magi(pikachu)
	g.P[0].X = 100
	speed := func() float64 { x0 := g.P[0].X; press(g, 0, Input{DX: 1}); return (g.P[0].X - x0) / dt }
	if got := speed(); math.Abs(got-cfg.Magikarp.WalkSpeed) > 1e-6 {
		t.Fatalf("on the ground %v, want walk_speed %v", got, cfg.Magikarp.WalkSpeed)
	}
	g.P[0].X = 100
	press(g, 0, Input{Skill: true})
	first := speed() // a tick or two into the jump
	steps(g, int(mj.Duration*tickRate*0.75))
	second := speed()
	if math.Abs(first-mj.AirSpeedFirstHalf) > 1e-6 || math.Abs(second-mj.AirSpeedSecondHalf) > 1e-6 {
		t.Fatalf("air speeds %v then %v, want %v then %v", first, second, mj.AirSpeedFirstHalf, mj.AirSpeedSecondHalf)
	}
}

func TestMagikarpEvolvesOnTheFifthLandingAndKeepsItsShareOfHP(t *testing.T) {
	cfg := DefaultConfig()
	g := magi(pikachu)
	g.P[0].HP = g.maxHP(0) / 2
	var evolved []Event
	landings := 0
	for tick := 0; tick < 25*tickRate && g.P[0].Kind == magikarp; tick++ {
		before := g.P[0].HopLeft
		g.P[0].In = Input{Skill: true}
		ev := g.Step(dt)
		if before > 0 && g.P[0].HopLeft == 0 {
			landings++
		}
		evolved = append(evolved, ev...)
	}
	if g.P[0].Kind != gyarados || landings != cfg.Magikarp.EvolveHops || g.P[0].Hops != cfg.Magikarp.EvolveHops {
		t.Fatalf("kind %d after %d landings (Hops %d), want Gyarados exactly at the %dth", g.P[0].Kind, landings, g.P[0].Hops, cfg.Magikarp.EvolveHops)
	}
	if count(evolved, "evolve") != 1 {
		t.Fatalf("%d evolve events", count(evolved, "evolve"))
	}
	if want := cfg.Gyarados.MaxHP / 2; g.P[0].HP != want {
		t.Fatalf("HP %d, want half of Gyarados's %d: the share it had", g.P[0].HP, cfg.Gyarados.MaxHP)
	}
	if g.radius(0) != cfg.Gyarados.Radius || g.cfg.walkSpeedOf(g.P[0].Kind) != cfg.Gyarados.WalkSpeed {
		t.Fatalf("radius %v, walk speed %v: it should have Gyarados's own", g.radius(0), g.cfg.walkSpeedOf(g.P[0].Kind))
	}
	// from now on the main button is the wave, not the jump
	press(g, 0, Input{Skill: true})
	if g.P[0].HopLeft != 0 || g.P[0].WaveT == 0 {
		t.Fatalf("HopLeft %v WaveT %v: Gyarados should hop for its wave", g.P[0].HopLeft, g.P[0].WaveT)
	}
}

func TestAJumpThatIsCutShortDoesNotCountAndTheHolderDecidesTheHeight(t *testing.T) {
	g := magi(pikachu)
	press(g, 0, Input{Skill: true})
	steps(g, 10)
	g.P[0].Held = true // a tornado has it
	z := g.P[0].Z
	steps(g, 5)
	if g.P[0].HopLeft != 0 || g.P[0].Hops != 0 || g.P[0].Z != z {
		t.Fatalf("HopLeft %v Hops %d Z %v (was %v): the jump should be dropped, uncounted, and the height left to the holder", g.P[0].HopLeft, g.P[0].Hops, g.P[0].Z, z)
	}
}

func TestEveryRoundStartsAsMagikarpAgain(t *testing.T) {
	g := magi(pikachu)
	g.P[0].Hops = 3
	g.evolve(0)
	g.Reset()
	if g.P[0].Kind != magikarp || g.P[0].Hops != 0 || g.P[0].HP != g.maxHP(0) || g.maxHP(0) != g.cfg.Magikarp.MaxHP {
		t.Fatalf("kind %d hops %d HP %d: a new round should start from a fresh Magikarp", g.P[0].Kind, g.P[0].Hops, g.P[0].HP)
	}
}

func TestGyaradosCannotBePickedOnlyMagikarpCan(t *testing.T) {
	r := teamRoom()
	if r.choose(0, gyarados) || r.pick[0] != -1 {
		t.Fatal("Gyarados was picked: it is only what Magikarp becomes")
	}
	r2 := &room{g: NewGame(DefaultConfig()), winner: -1, pick: [2]int{-1, -1}}
	r2.c = [2]*client{{send: make(chan []byte, 8)}, {send: make(chan []byte, 8)}}
	if !r2.choose(0, magikarp) || r2.pick[0] != magikarp {
		t.Fatal("Magikarp could not be picked")
	}
}

func TestATeamSurvivorComesBackAsMagikarpWithoutMoreHPThanItCanHold(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{magikarp, 0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{0, 1, 2, 3, 4, 5, 6})
	r.g.evolve(0) // it has evolved by the end of the bout
	r.g.P[0].HP = r.g.cfg.Gyarados.MaxHP - 5
	now := time.Now()
	knockOut(r, 1, now) // the other side's first falls
	r.step(now.Add(roundReset + time.Second))
	if r.g.P[0].Kind != magikarp || r.g.P[0].HP != r.g.cfg.Magikarp.MaxHP {
		t.Fatalf("kind %d HP %d, want a Magikarp with at most its own %d", r.g.P[0].Kind, r.g.P[0].HP, r.g.cfg.Magikarp.MaxHP)
	}
}

// ---- Gyarados: the wave ----

func TestGyaradosHopsRootedAndTheLandingSendsAWaveThatHurtsTheOpponentOnce(t *testing.T) {
	cfg := DefaultConfig()
	wv := cfg.Gyarados.Wave
	for _, c := range []struct {
		name string
		d    float64 // distance between the two, centre to centre
		hit  bool
	}{{"close", 150, true}, {"at the edge of the wave", wv.Radius + cfg.Snorlax.Radius - 5, true}, {"just out of reach", wv.Radius + cfg.Snorlax.Radius + 5, false}} {
		g := gya(cfg, snorlax)
		g.P[1].X = 300 + c.d
		hp := g.P[1].HP
		ev := press(g, 0, Input{Skill: true, DX: 1})
		steps(g, 3)
		if g.P[0].Z <= 0 || g.P[0].Z > wv.JumpHeight+1e-9 || g.P[0].X != 300 {
			t.Fatalf("%s: mid-hop Z %v X %v: it should be in the air, rooted", c.name, g.P[0].Z, g.P[0].X)
		}
		for i := 0; i < int(wv.JumpDuration*tickRate)+2; i++ {
			g.P[0].In = Input{}
			ev = append(ev, g.Step(dt)...)
		}
		if count(ev, "wave") != 1 || g.P[0].Z != 0 {
			t.Fatalf("%s: %d wave events, Z %v after the hop", c.name, count(ev, "wave"), g.P[0].Z)
		}
		steps(g, int(2*tickRate)) // plenty of time for the wave to cross the court
		got := hp - g.P[1].HP
		if (c.hit && got != wv.Damage) || (!c.hit && got != 0) {
			t.Errorf("%s: took %d, want %d (once)", c.name, got, map[bool]int{true: wv.Damage, false: 0}[c.hit])
		}
		if len(g.Waves) != 0 {
			t.Errorf("%s: %d waves left after they should have run out", c.name, len(g.Waves))
		}
	}
}

func TestTheWaveSpreadsAtItsSpeedUpToItsRadius(t *testing.T) {
	cfg := DefaultConfig()
	wv := cfg.Gyarados.Wave
	g := gya(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 1800, 900 // far away
	press(g, 0, Input{Skill: true})
	steps(g, int(wv.JumpDuration*tickRate)+1)
	if len(g.Waves) != 1 {
		t.Fatalf("%d waves", len(g.Waves))
	}
	r0 := g.Waves[0].R
	steps(g, 3)
	if got := g.Waves[0].R - r0; math.Abs(got-3*wv.Speed*dt) > 1e-6 {
		t.Fatalf("grew %v in 3 ticks, want %v", got, 3*wv.Speed*dt)
	}
	steps(g, int(2*tickRate))
	if len(g.Waves) != 0 {
		t.Fatalf("the wave never ended")
	}
}

func TestAWaveGoesUnderAnOpponentInTheAirAndIsSpentAnyway(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.Wave.Reach = Reach{MinHeight: 0, MaxHeight: 15}
	g := gya(cfg, rowlet) // Rowlet flies at fly_high (30), above that band
	g.P[1].X = 500
	if g.P[1].Z <= cfg.Gyarados.Wave.MaxHeight {
		t.Fatal("the scene needs a Rowlet above the band")
	}
	hp := g.P[1].HP
	press(g, 0, Input{Skill: true})
	steps(g, 3*tickRate)
	if g.P[1].HP != hp {
		t.Fatalf("Rowlet lost %d HP to a wave that runs along the ground", hp-g.P[1].HP)
	}
}

func TestWaveHasACooldownFromTheStartOfTheHopAndNeedsTheGround(t *testing.T) {
	cfg := DefaultConfig()
	g := gya(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 1800, 900
	press(g, 0, Input{Skill: true})
	steps(g, 3*tickRate)
	press(g, 0, Input{Skill: true})
	if g.P[0].WaveT != 0 {
		t.Fatal("a second hop began during the cooldown")
	}
	steps(g, int((cfg.Gyarados.Wave.Cooldown-3)*tickRate)+2)
	press(g, 0, Input{Skill: true})
	if g.P[0].WaveT == 0 {
		t.Fatalf("no hop after the cooldown (WaveCD %v)", g.P[0].WaveCD)
	}
}

// ---- Gyarados: the tail whip ----

func TestTailWhipKnocksBackAndStunsWhoeverIsInTheFanAlongTheAim(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.TailWhip.ChargeDuration = 0 // these are about the whip itself: no wind-up
	tw := cfg.Gyarados.TailWhip
	deg := func(a float64) (float64, float64) { return math.Cos(a * math.Pi / 180), math.Sin(a * math.Pi / 180) }
	for _, c := range []struct {
		name  string
		angle float64 // where the opponent is, from the aim
		d     float64
		hit   bool
	}{
		{"straight ahead", 0, 150, true},
		{"inside the fan", tw.Angle/2 - 5, 150, true},
		{"outside the fan", tw.Angle/2 + 10, 150, false},
		{"behind", 180, 150, false},
		{"too far", 0, tw.Length + cfg.Snorlax.Radius + 10, false},
		{"just in reach", 0, tw.Length + cfg.Snorlax.Radius - 5, true},
	} {
		g := gya(cfg, snorlax)
		g.P[0].AimX, g.P[0].AimY = 1, 0
		ux, uy := deg(c.angle)
		g.P[1].X, g.P[1].Y = 800+c.d*ux, 500+c.d*uy
		g.P[0].X, g.P[0].Y = 800, 500
		x0, y0 := g.P[1].X, g.P[1].Y
		ev := press(g, 0, Input{Tail: true})
		moved := math.Hypot(g.P[1].X-x0, g.P[1].Y-y0)
		if c.hit {
			if math.Abs(moved-tw.Knockback) > 1e-6 || math.Abs(g.P[1].StunT-tw.StunDuration) > dt+1e-9 {
				t.Errorf("%s: knocked back %v (want %v), stunned %v (want %v)", c.name, moved, tw.Knockback, g.P[1].StunT, tw.StunDuration)
			}
		} else if moved != 0 || g.P[1].StunT != 0 {
			t.Errorf("%s: knocked back %v, stunned %v: it was not in the fan", c.name, moved, g.P[1].StunT)
		}
		if count(ev, "tail") != 1 {
			t.Errorf("%s: %d tail events", c.name, count(ev, "tail"))
		}
	}
}

func TestTailWhipFollowsTheAimNotTheOpponent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.TailWhip.ChargeDuration = 0 // these are about the whip itself: no wind-up
	g := gya(cfg, snorlax)
	g.P[0].AimX, g.P[0].AimY = 0, 1 // aimed at the floor of the court, the opponent is to the right
	g.P[1].X, g.P[1].Y = 450, 250
	press(g, 0, Input{Tail: true})
	if g.P[1].StunT != 0 {
		t.Fatal("it hit an opponent that is not where it aims")
	}
}

func TestTailWhipCoolsDownAndIsHeldBackAboveItsBand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.TailWhip.ChargeDuration = 0 // these are about the whip itself: no wind-up
	g := gya(cfg, snorlax)
	g.P[1].X = 450
	press(g, 0, Input{Tail: true})
	g.P[1].StunT, g.P[1].X = 0, 450
	press(g, 0, Input{Tail: true})
	if g.P[1].StunT != 0 {
		t.Fatal("a second whip during the cooldown")
	}
	steps(g, int(cfg.Gyarados.TailWhip.Cooldown*tickRate))
	g.P[1].X = 450
	press(g, 0, Input{Tail: true})
	if g.P[1].StunT == 0 {
		t.Fatal("no whip after the cooldown")
	}

	cfg.Gyarados.TailWhip.Reach = Reach{MinHeight: 0, MaxHeight: 15}
	g = gya(cfg, rowlet)
	g.P[1].X = 450
	press(g, 0, Input{Tail: true})
	if g.P[1].StunT != 0 {
		t.Fatal("it whipped a Rowlet that flies above its band")
	}
}

func TestAStunnedPlayerCanNeitherMoveNorActUntilItWearsOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.TailWhip.ChargeDuration = 0 // these are about the whip itself: no wind-up
	tw := cfg.Gyarados.TailWhip
	g := gya(cfg, pikachu)
	g.P[1].X, g.P[1].Y = 450, 250
	press(g, 0, Input{Tail: true})
	if g.P[1].StunT == 0 {
		t.Fatal("not stunned")
	}
	x := g.P[1].X
	g.P[1].In = Input{DX: 1, Ball: true, Skill: true}
	steps(g, int((tw.StunDuration-0.2)*tickRate))
	if g.P[1].X != x || g.P[1].BallT != 0 || g.P[1].DashT != 0 {
		t.Fatalf("it moved or acted while stunned: x %v -> %v, BallT %v DashT %v", x, g.P[1].X, g.P[1].BallT, g.P[1].DashT)
	}
	steps(g, int(0.4*tickRate)+1)
	if g.P[1].StunT != 0 || g.P[1].X == x && g.P[1].BallT == 0 && g.P[1].DashT == 0 {
		t.Fatalf("StunT %v, still doing nothing after it should have worn off", g.P[1].StunT)
	}
}

// ---- Gyarados: the Hyper Beam is Snorlax's ----

func TestGyaradosFiresItsOwnHyperBeamAndHasNoPunchOrJump(t *testing.T) {
	cfg := DefaultConfig()
	hb := cfg.Gyarados.HyperBeam
	g := gya(cfg, pikachu)
	g.P[1].X, g.P[1].Y = 450, 250
	hp := g.P[1].HP
	press(g, 0, Input{Beam: true, Punch: true}) // the page sends both with K / J: only the beam is Gyarados's
	if g.P[0].BeamT == 0 || g.P[0].PunchT != 0 {
		t.Fatalf("BeamT %v PunchT %v", g.P[0].BeamT, g.P[0].PunchT)
	}
	x := g.P[0].X
	g.P[0].In = Input{DX: 1}
	steps(g, int((hb.ChargeDuration+hb.SweepDuration)*tickRate)+3)
	if g.P[0].X != x {
		t.Fatal("Gyarados walked while it charged and swept the beam")
	}
	if g.P[1].HP >= hp {
		t.Fatalf("the beam did not hurt the opponent in front of it (HP %d)", g.P[1].HP)
	}
	g = gya(cfg, pikachu)
	press(g, 0, Input{Tail: true, Punch: true})
	if g.P[0].PunchT != 0 || g.P[0].PunchCD != 0 || g.P[0].JumpCD != 0 {
		t.Fatal("Gyarados used a Snorlax skill")
	}
}

func TestGyaradosCannotWhipOrHopWhileItsBeamRuns(t *testing.T) {
	cfg := DefaultConfig()
	g := gya(cfg, snorlax)
	g.P[1].X = 450
	press(g, 0, Input{Beam: true})
	press(g, 0, Input{Tail: true, Skill: true})
	if g.P[0].TailCD != 0 || g.P[0].WaveT != 0 || g.P[1].StunT != 0 {
		t.Fatalf("TailCD %v WaveT %v: it used a skill in the middle of its beam", g.P[0].TailCD, g.P[0].WaveT)
	}
}

// ---- the wires ----

func TestMagikarpAndGyaradosSnapshotKeysAndEventFields(t *testing.T) {
	b, _ := json.Marshal(Player{})
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"ot", "ocd", "hn", "wvt", "wvcd", "tlcd", "tlt", "tlx", "tly", "sn"} {
		if _, ok := m[k]; !ok {
			t.Errorf("snapshot is missing %q: %s", k, b)
		}
	}
	e, _ := json.Marshal(Event{T: "tail", X: 1, Y: 2, R: 3, DX: 0.5, DY: -0.5})
	if string(e) != `{"t":"tail","x":1,"y":2,"r":3,"dx":0.5,"dy":-0.5}` {
		t.Fatalf("event json %s", e)
	}
	e, _ = json.Marshal(Event{T: "slam", X: 1, Y: 2})
	if string(e) != `{"t":"slam","x":1,"y":2}` {
		t.Fatalf("event json %s: the direction should be left out when there is none", e)
	}
}

func TestMagikarpAndGyaradosConfigIsLoadedAndRefusesBadValues(t *testing.T) {
	good := DefaultConfig()
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(c *Config){
		"magikarp.max_hp":                      func(c *Config) { c.Magikarp.MaxHP = 0 },
		"magikarp.evolve_hops":                 func(c *Config) { c.Magikarp.EvolveHops = 0 },
		"magikarp.jump.height":                 func(c *Config) { c.Magikarp.Jump.Height = 0 },
		"magikarp.jump.highest_point_duration": func(c *Config) { c.Magikarp.Jump.HighestPointDuration = c.Magikarp.Jump.Duration },
		"magikarp.jump.air_speed_second_half":  func(c *Config) { c.Magikarp.Jump.AirSpeedSecondHalf = 0 },
		"gyarados.radius":                      func(c *Config) { c.Gyarados.Radius = 0 },
		"gyarados.wave.speed":                  func(c *Config) { c.Gyarados.Wave.Speed = 0 },
		"gyarados.wave.jump_duration":          func(c *Config) { c.Gyarados.Wave.JumpDuration = 0 },
		"gyarados.wave":                        func(c *Config) { c.Gyarados.Wave.MinHeight, c.Gyarados.Wave.MaxHeight = 50, 10 },
		"gyarados.tail_whip.angle":             func(c *Config) { c.Gyarados.TailWhip.Angle = 361 },
		"gyarados.tail_whip.length":            func(c *Config) { c.Gyarados.TailWhip.Length = 0 },
		"gyarados.tail_whip.stun_duration":     func(c *Config) { c.Gyarados.TailWhip.StunDuration = -1 },
		"gyarados.tail_whip.charge_duration":   func(c *Config) { c.Gyarados.TailWhip.ChargeDuration = -1 },
		"gyarados.tail_whip.knockback":         func(c *Config) { c.Gyarados.TailWhip.Knockback = -1 },
		"gyarados.hyper_beam.length":           func(c *Config) { c.Gyarados.HyperBeam.Length = 0 },
		"gyarados.hyper_beam.damage":           func(c *Config) { c.Gyarados.HyperBeam.Damage = -1 },
		"gyarados.hyper_beam.sweep_angle":      func(c *Config) { c.Gyarados.HyperBeam.SweepAngle = 181 },
		"gyarados.hyper_beam":                  func(c *Config) { c.Gyarados.HyperBeam.MinHeight, c.Gyarados.HyperBeam.MaxHeight = 50, 10 },
		"snorlax.hyper_beam.sweep_angle":       func(c *Config) { c.Snorlax.HyperBeam.SweepAngle = 181 },
	} {
		c := good
		mut(&c)
		if err := c.validate(); err == nil || !contains(err.Error(), name) {
			t.Errorf("%s: error %v, want one naming it", name, err)
		}
	}
	cfg := good
	if cfg.maxHPOf(magikarp) != cfg.Magikarp.MaxHP || cfg.maxHPOf(gyarados) != cfg.Gyarados.MaxHP ||
		cfg.walkSpeedOf(magikarp) != cfg.Magikarp.WalkSpeed || cfg.walkSpeedOf(gyarados) != cfg.Gyarados.WalkSpeed ||
		cfg.radiusOf(magikarp) != cfg.Magikarp.Radius || cfg.radiusOf(gyarados) != cfg.Gyarados.Radius {
		t.Fatal("the accessors do not return each form's own HP, speed and radius")
	}
	if cfg.Magikarp.MaxHP == cfg.Gyarados.MaxHP || cfg.Magikarp.Radius == cfg.Gyarados.Radius {
		t.Fatal("the two forms should have different defaults")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// The two beams are separate settings: changing one leaves the other alone.
func TestGyaradosAndSnorlaxEachHaveTheirOwnHyperBeamSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.HyperBeam.Damage = 3
	cfg.Gyarados.HyperBeam.Length = 300
	cfg.Gyarados.HyperBeam.Cooldown = 9
	if cfg.beamOf(gyarados).Damage != 3 || cfg.beamOf(snorlax).Damage == 3 || cfg.beamOf(pikachu).Length == 300 {
		t.Fatal("beamOf mixes the two up")
	}
	hurt := func(kind int) int { // what a point-blank beam takes off Pikachu in front of it, over a whole sweep
		g := duel(cfg, magikarp, pikachu)
		g.P[0].Kind = kind
		g.P[0].HP, g.P[1].X, g.P[1].Y = g.maxHP(0), 450, 250
		hp := g.P[1].HP
		press(g, 0, Input{Beam: true})
		steps(g, int(3*tickRate))
		return hp - g.P[1].HP
	}
	if g, s := hurt(gyarados), hurt(snorlax); g == s || g == 0 || s == 0 {
		t.Fatalf("Gyarados took %d off Pikachu and Snorlax %d: each should follow its own damage", g, s)
	}
	g := duel(cfg, magikarp, pikachu)
	g.P[0].Kind = gyarados
	press(g, 0, Input{Beam: true})
	if g.P[0].BeamCD <= 8 || g.P[0].BeamCD > 9 {
		t.Fatalf("Gyarados's beam cooldown is %v, want its own 9", g.P[0].BeamCD)
	}
}

// ---- the tail whip's wind-up ----

func windUp(cfg Config, foe int) *Game {
	cfg.Gyarados.TailWhip.ChargeDuration = 0.5
	g := gya(cfg, foe)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 800, 500, 950, 500 // 150 away, straight ahead
	g.P[0].AimX, g.P[0].AimY = 1, 0
	return g
}

func TestTailWhipWindsUpRootedAndHitsOnlyWhenTheChargeIsOver(t *testing.T) {
	tw := DefaultConfig().Gyarados.TailWhip
	g := windUp(DefaultConfig(), snorlax)
	g.P[1].X = 950
	ev := press(g, 0, Input{Tail: true})
	if g.P[0].TailT <= 0 || g.P[0].TailCD <= 0 || count(ev, "tail") != 0 || g.P[1].StunT != 0 || g.P[1].X != 950 {
		t.Fatalf("TailT %v TailCD %v, %d tail events, foe stunned %v at x=%v: nothing should land at the press", g.P[0].TailT, g.P[0].TailCD, count(ev, "tail"), g.P[1].StunT, g.P[1].X)
	}
	x := g.P[0].X
	var events []Event
	for i := 0; i < int(0.5*tickRate)-2; i++ { // most of the wind-up, trying to walk away
		g.P[0].In = Input{DX: -1}
		events = append(events, g.Step(dt)...)
	}
	if g.P[0].X != x || count(events, "tail") != 0 || g.P[1].StunT != 0 {
		t.Fatalf("mid wind-up: x %v -> %v, %d tail events, stun %v: it should be rooted and not have hit yet", x, g.P[0].X, count(events, "tail"), g.P[1].StunT)
	}
	for i := 0; i < 4; i++ {
		g.P[0].In = Input{}
		events = append(events, g.Step(dt)...)
	}
	if count(events, "tail") != 1 || g.P[1].StunT <= 0 || math.Abs(g.P[1].X-(950+tw.Knockback)) > 1e-6 {
		t.Fatalf("after the wind-up: %d tail events, stun %v, x %v: want one hit that knocks back %v and stuns", count(events, "tail"), g.P[1].StunT, g.P[1].X, tw.Knockback)
	}
	if g.P[0].TailT != 0 {
		t.Fatalf("TailT %v after the hit", g.P[0].TailT)
	}
}

func TestTheFanIsFixedWhenKIsPressedAndWhoeverLeavesItIsNotHit(t *testing.T) {
	g := windUp(DefaultConfig(), snorlax)
	press(g, 0, Input{Tail: true})
	if math.Abs(g.P[0].TailX-1) > 1e-9 || g.P[0].TailY != 0 {
		t.Fatalf("the warning aim is (%v,%v), want the aim at the press (1,0)", g.P[0].TailX, g.P[0].TailY)
	}
	for i := 0; i < int(0.5*tickRate)+2; i++ { // the stick turns away, the opponent steps out of the fan
		g.P[0].In = Input{DY: 1}
		g.P[1].X, g.P[1].Y = 800, 800 // straight below, far outside a fan along (1,0)
		g.Step(dt)
	}
	if g.P[1].StunT != 0 {
		t.Fatal("it hit an opponent that had stepped out of the warned fan")
	}

	g = windUp(DefaultConfig(), snorlax)
	press(g, 0, Input{Tail: true})
	for i := 0; i < int(0.5*tickRate)+2; i++ {
		g.P[0].In = Input{DY: 1} // turning the stick does not turn the fan
		g.Step(dt)
	}
	if g.P[1].StunT == 0 {
		t.Fatal("the fan followed the stick: it should stay where it was warned")
	}
}

func TestAWindUpIsLostWhenFrozenStunnedOrCaught(t *testing.T) {
	for name, cut := range map[string]func(p *Player){"frozen": func(p *Player) { p.FrozenT = 1 }, "stunned": func(p *Player) { p.StunT = 1 }, "caught": func(p *Player) { p.Held = true }} {
		g := windUp(DefaultConfig(), snorlax)
		press(g, 0, Input{Tail: true})
		cut(&g.P[0])
		steps(g, int(0.8*tickRate))
		if g.P[0].TailT != 0 || g.P[1].StunT != 0 {
			t.Errorf("%s: TailT %v, foe stunned %v: the whip should have been cancelled", name, g.P[0].TailT, g.P[1].StunT)
		}
	}
}

func TestNothingElseStartsInTheMiddleOfAWindUp(t *testing.T) {
	g := windUp(DefaultConfig(), snorlax)
	press(g, 0, Input{Tail: true})
	press(g, 0, Input{Skill: true, Beam: true})
	if g.P[0].WaveT != 0 || g.P[0].BeamT != 0 {
		t.Fatalf("WaveT %v BeamT %v: a hop or a beam began in the middle of the wind-up", g.P[0].WaveT, g.P[0].BeamT)
	}
}

func TestTailWhipWithNoChargeHitsAtOnce(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gyarados.TailWhip.ChargeDuration = 0
	g := gya(cfg, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 800, 500, 950, 500
	g.P[0].AimX, g.P[0].AimY = 1, 0
	ev := press(g, 0, Input{Tail: true})
	if count(ev, "tail") != 1 || g.P[1].StunT == 0 || g.P[0].TailT != 0 {
		t.Fatalf("%d tail events, stun %v, TailT %v: with no charge it should land at the press", count(ev, "tail"), g.P[1].StunT, g.P[0].TailT)
	}
}
