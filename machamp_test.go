package main

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// mach is Machamp (slot 0, at 300,250, full HP) against foe (slot 1, at 900,250).
func mach(cfg Config, foe int) *Game {
	g := duel(cfg, ash, foe)
	g.P[0].HP = g.maxHP(0)
	return g
}

// flyer makes slot 1 a Charizard in the air at `height` (it stays there: the flight is long).
func flyer(cfg Config, height float64) *Game {
	cfg.Charizard.Fly.Height = height
	cfg.Charizard.Fly.Duration = 100
	g := mach(cfg, charizard)
	g.P[1].In.Skill = true
	g.Step(dt)
	g.P[1].In = Input{}
	steps(g, 20)
	return g
}

func stoneCount(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballStone {
			n++
		}
	}
	return n
}

func fistCount(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballFist {
			n++
		}
	}
	return n
}

// ---- Tree slam ----

func TestTreeSlamJumpsTheConfiguredDistanceAndHeightThenHoldsTheTreeUpAndSmashesIt(t *testing.T) {
	cfg := DefaultConfig()
	ts := cfg.Ash.TreeSlam
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 1900, 900 // out of the way
	x0 := g.P[0].X
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	g.P[0].In = Input{}
	peak := g.P[0].Z
	n := 1
	for g.P[0].TreePhase == treeJump {
		g.Step(dt)
		n++
		peak = math.Max(peak, g.P[0].Z)
	}
	if got := g.P[0].X - x0; math.Abs(got-ts.JumpDistance) > 1e-6 {
		t.Fatalf("jumped %v, want %v", got, ts.JumpDistance)
	}
	if want := int(math.Round(ts.JumpDuration * tickRate)); n != want {
		t.Fatalf("jump took %d ticks, want %d", n, want)
	}
	if math.Abs(peak-ts.JumpHeight) > ts.JumpHeight*0.05 {
		t.Fatalf("peak %v, want about %v", peak, ts.JumpHeight)
	}
	if g.P[0].Z != 0 || g.P[0].TreePhase != treeSmash {
		t.Fatalf("Z %v phase %d after landing, want the tree held up", g.P[0].Z, g.P[0].TreePhase)
	}
	// Held up: rooted, and the smash comes when the charge has run out.
	g.P[0].In = Input{DX: 1}
	x1 := g.P[0].X
	smashes := 0
	for tick := 0; tick < int((ts.ChargeDuration+ts.SwingDuration)*tickRate)+3 && smashes == 0; tick++ {
		smashes += count(g.Step(dt), "tree") // it is still rooted on the tick of the smash
	}
	if g.P[0].X != x1 {
		t.Fatalf("walked %v while holding the tree", g.P[0].X-x1)
	}
	if smashes != 1 || g.P[0].TreePhase != treeNone {
		t.Fatalf("%d smashes, phase %d, want one and the tree down", smashes, g.P[0].TreePhase)
	}
}

func TestTreeSlamGoesTheWayHeldElseTheWayItFaces(t *testing.T) {
	g := mach(DefaultConfig(), snorlax)
	g.P[0].FX, g.P[0].FY = 0, 1 // faces down
	g.P[0].In = Input{Skill: true}
	g.Step(dt)
	if g.P[0].TreeX != 0 || g.P[0].TreeY != 1 {
		t.Fatalf("direction %v,%v, want 0,1: the way it faces", g.P[0].TreeX, g.P[0].TreeY)
	}
}

func TestTreeShapeIsATrunkWithACanopyAtItsTip(t *testing.T) {
	const length, width, leaf, angle = 300.0, 70.0, 180.0, 180.0
	for _, c := range []struct {
		name   string
		bx, by float64 // the body, from the foot of the trunk, the tree pointing right
		hit    bool
	}{
		{"on the trunk", 150, 0, true},
		{"beside the trunk, inside its width", 150, 30, true},
		{"beside the trunk, outside it", 150, 70, false},
		{"just behind the foot", -5, 0, true},
		{"well behind the foot", -40, 0, false},
		{"canopy, straight past the tip", 300 + 150, 0, true},
		{"canopy, far edge", 300 + 190, 0, true},
		{"canopy, too far", 300 + 220, 0, false},
		{"canopy, off to the side", 300, 170, true},
		{"canopy, off to the side too far", 300, 210, false},
		{"beside the trunk but past the canopy diameter line, out of the trunk", 250, 120, false},
	} {
		if got := treeHits(0, 0, 1, 0, c.bx, c.by, 20, length, width, leaf, angle); got != c.hit {
			t.Errorf("%s: hit = %v, want %v", c.name, got, c.hit)
		}
	}
	// A narrower canopy: 90 degrees leaves the sides out.
	if treeHits(0, 0, 1, 0, 300, 170, 5, length, width, leaf, 90) {
		t.Error("a 90 degree canopy reached sideways")
	}
	// It points where it is told: down.
	if !treeHits(0, 0, 0, 1, 0, 150, 20, length, width, leaf, angle) || treeHits(0, 0, 0, 1, 150, 0, 20, length, width, leaf, angle) {
		t.Error("a tree smashed down the screen hit sideways")
	}
}

func TestTreeSmashHurtsOnceAndOnlyWithinItsHeightBandAndCoolsDown(t *testing.T) {
	cfg := DefaultConfig()
	ts := cfg.Ash.TreeSlam
	run := func(foeHeight float64) *Game {
		g := mach(cfg, snorlax)
		if foeHeight > 0 {
			g = flyer(cfg, foeHeight)
		}
		g.P[0].X, g.P[0].Y = 300, 250
		g.P[1].X, g.P[1].Y = 300+ts.JumpDistance+150, 250 // on the trunk once it has landed
		g.P[0].In = Input{Skill: true, DX: 1}
		steps(g, 1)
		g.P[0].In = Input{}
		steps(g, 60)
		return g
	}
	if g := run(0); g.P[1].HP != g.maxHP(1)-ts.Damage {
		t.Fatalf("HP %d, want %d: one smash, one hit", g.P[1].HP, g.maxHP(1)-ts.Damage)
	}
	if g := run(ts.MaxHeight + 20); g.P[1].HP != cfg.Charizard.MaxHP {
		t.Fatalf("HP %d: the tree hit something above its band", g.P[1].HP)
	}
	g := run(0)
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	if g.P[0].TreePhase != treeNone || g.P[0].Z != 0 {
		t.Fatal("a second tree slam started during the cooldown")
	}
}

func TestTreeSlamWithNoChargeComesDownOnTheLandingTick(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.TreeSlam.ChargeDuration = 0
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+cfg.Ash.TreeSlam.JumpDistance+100, 250
	g.P[0].In = Input{Skill: true, DX: 1}
	for n := 0; n < 40 && g.P[1].HP == g.maxHP(1); n++ {
		g.Step(dt)
	}
	if g.P[1].HP == g.maxHP(1) || g.P[0].TreePhase != treeNone {
		t.Fatal("no smash on landing with charge_duration 0")
	}
}

func TestBeingFrozenCutsAMachampSkillShort(t *testing.T) {
	g := mach(DefaultConfig(), snorlax)
	g.P[0].In = Input{Skill: true, DX: 1}
	steps(g, 5)
	g.P[0].FrozenT = 1
	g.Step(dt)
	if p := g.P[0]; p.TreePhase != treeNone || p.Z != 0 || p.TreeT != 0 {
		t.Fatalf("phase %d Z %v after being frozen mid-jump", p.TreePhase, p.Z)
	}
}

// ---- Comet Punch ----

func TestCometPunchRunsAtTheOpponentThrowingEveryFistAheadOfIt(t *testing.T) {
	cfg := DefaultConfig()
	cp := cfg.Ash.CometPunch
	cfg.Ash.CometPunch.FistDuration = 100 // they stay, so they can be counted
	cfg.Ash.CometPunch.Damage = 0
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 1900, 250 // far: the whole run is spent running
	x0 := g.P[0].X
	g.P[0].In = Input{Comet: true}
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, int(cp.Duration*tickRate)+3)
	if got := g.P[0].X - x0; math.Abs(got-cp.Distance) > 1e-6 {
		t.Fatalf("ran %v, want %v", got, cp.Distance)
	}
	if n := fistCount(g); n != cp.Quantity {
		t.Fatalf("%d fists, want %d", n, cp.Quantity)
	}
	for _, b := range g.Balls {
		if b.K != ballFist {
			continue
		}
		// ahead of the body edge, in the scatter band, within the offset angle of the heading (to the right)
		d := math.Hypot(b.X-300, b.Y-250) // from where the run began is close enough: the band is wide, the run long
		if b.DX < math.Cos(cp.RandomOffsetAngle*math.Pi/180)-1e-9 {
			t.Fatalf("a fist heads %v,%v, outside +-%v degrees of the run", b.DX, b.DY, cp.RandomOffsetAngle)
		}
		if b.X < 300+cp.ScatterMinDistance-1 || d > cp.Distance+cfg.Ash.Radius+cp.ScatterMaxDistance+1 {
			t.Fatalf("a fist at %v,%v is not ahead of the runner", b.X, b.Y)
		}
		if b.HP != cp.HP {
			t.Fatalf("fist HP %d, want %d", b.HP, cp.HP)
		}
	}
}

func TestCometPunchStopsAtTheOpponentsBodyAndEachFistHurtsOnceThenVanishes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.CometPunch.Knockback = 0 // the opponent stays put, so the stop at its body can be measured
	cp := cfg.Ash.CometPunch
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 400, 250 // 100 away: closer than the run is long
	g.P[0].In = Input{Comet: true}
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 90)
	gap := g.P[1].X - g.P[0].X
	if math.Abs(gap-(cfg.Ash.Radius+cfg.Snorlax.Radius)) > 1e-6 {
		t.Fatalf("stopped %v from the opponent, want its body's edge %v", gap, cfg.Ash.Radius+cfg.Snorlax.Radius)
	}
	lost := g.maxHP(1) - g.P[1].HP
	if lost == 0 || lost%cp.Damage != 0 || lost > cp.Quantity*cp.Damage {
		t.Fatalf("lost %d HP, want a whole number of fists' damage (%d each, at most %d fists)", lost, cp.Damage, cp.Quantity)
	}
	if fistCount(g) != 0 {
		t.Fatalf("%d fists left after they hit or ran out", fistCount(g))
	}
}

func TestCometPunchRootsMachampAndHasItsOwnCooldownAndHeightBand(t *testing.T) {
	cfg := DefaultConfig()
	cp := cfg.Ash.CometPunch
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 1900, 900
	g.P[0].In = Input{Comet: true, DY: 1} // asking to walk down while it runs at the foe
	steps(g, 10)
	if g.P[0].Y > 250+1e-9 && g.P[1].Y < g.P[0].Y {
		t.Fatalf("Y %v: it walked where it was steered instead of running at the opponent", g.P[0].Y)
	}
	g.P[0].In = Input{}
	steps(g, int(cp.Duration*tickRate)+2)
	g.P[0].In = Input{Comet: true}
	g.Step(dt)
	if g.P[0].CometT != 0 {
		t.Fatal("a second flurry started during the cooldown")
	}
	// Above the band a fist passes by.
	cfg2 := DefaultConfig()
	g = flyer(cfg2, cfg2.Ash.CometPunch.MaxHeight+30)
	g.P[1].X, g.P[1].Y = 400, 250
	g.P[0].In = Input{Comet: true}
	g.Step(dt)
	g.P[0].In = Input{}
	steps(g, 90)
	if g.P[1].HP != cfg2.Charizard.MaxHP {
		t.Fatalf("HP %d: fists hit something above their band", g.P[1].HP)
	}
}

func TestAFistIsShotDownLikeAFireBallAndShootsBackAtWhatHitsIt(t *testing.T) {
	cfg := DefaultConfig()
	g := mach(cfg, pikachu)
	g.P[1].X, g.P[1].Y = 1900, 900
	g.Balls = []Ball{
		{K: ballFist, Owner: 0, X: 500, Y: 250, DX: 1, HP: cfg.Ash.CometPunch.HP, Life: 5},
		{K: ballElectro, Owner: 1, X: 503, Y: 250, DX: -1, HP: cfg.Pikachu.ElectroBall.HP},
	}
	for _, b := range g.Balls {
		_ = b
	}
	g.Balls[0].X0, g.Balls[0].Y0, g.Balls[1].X0, g.Balls[1].Y0 = 500, 250, 503, 250
	g.clashBalls()
	hurtFist := cfg.Ash.CometPunch.HP - g.Balls[0].HP
	if hurtFist != cfg.Pikachu.ElectroBall.ContactDamage {
		t.Fatalf("the fist lost %d, want the ball's contact damage %d", hurtFist, cfg.Pikachu.ElectroBall.ContactDamage)
	}
	if g.Balls[1].HP != cfg.Pikachu.ElectroBall.HP-cfg.Ash.CometPunch.Damage {
		t.Fatalf("the ball's HP %d, want it worn down by the fist's damage", g.Balls[1].HP)
	}
}

// ---- Rock Tomb ----

func TestRockTombJumpsAndSteersFasterOrSlowerByHalvesAndRaisesTheRingOnLanding(t *testing.T) {
	cfg := DefaultConfig()
	rt := cfg.Ash.RockTomb
	cfg.Ash.RockTomb.AirSpeedFirstHalf, cfg.Ash.RockTomb.AirSpeedSecondHalf = 300, 60
	g := mach(cfg, snorlax)
	g.P[0].X, g.P[0].Y = 1000, 500
	g.P[1].X, g.P[1].Y = 1900, 900
	g.P[0].In = Input{Rock: true, DX: 1}
	var first, second, peak float64
	ticks := int(math.Round(rt.Duration * tickRate))
	for n := 1; n <= ticks; n++ {
		x := g.P[0].X
		g.Step(dt)
		peak = math.Max(peak, g.P[0].Z)
		if d := g.P[0].X - x; n <= ticks/2-1 {
			first = d / dt
		} else if n > ticks/2+1 && n < ticks {
			second = d / dt
		}
	}
	if math.Abs(first-300) > 1e-6 || math.Abs(second-60) > 1e-6 {
		t.Fatalf("air speeds %v then %v, want 300 then 60", first, second)
	}
	if math.Abs(peak-rt.Height) > rt.Height*0.05 {
		t.Fatalf("peak %v, want about %v", peak, rt.Height)
	}
	if g.P[0].Z != 0 || g.P[0].RockT != 0 {
		t.Fatalf("Z %v RockT %v after the jump", g.P[0].Z, g.P[0].RockT)
	}
	if n := stoneCount(g); n != rt.Count {
		t.Fatalf("%d stones, want %d", n, rt.Count)
	}
	for _, b := range g.Balls {
		if b.K != ballStone {
			continue
		}
		if d := math.Hypot(b.X-g.P[0].X, b.Y-g.P[0].Y); math.Abs(d-rt.RingRadius) > cfg.Ash.WalkSpeed*dt+1e-6 { // centred on where it was a tick before it stood
			t.Fatalf("a stone %v from Machamp, want %v", d, rt.RingRadius)
		}
		if b.HP != rt.StoneHP || math.Abs(b.Life-rt.StoneDuration) > dt+1e-9 { // it has already ticked once, on the landing step
			t.Fatalf("stone HP %d life %v, want %d and %v", b.HP, b.Life, rt.StoneHP, rt.StoneDuration)
		}
	}
}

func TestRockTombLeavesOutStonesThatWouldBeOutsideTheCourt(t *testing.T) {
	cfg := DefaultConfig()
	g := mach(cfg, snorlax)
	g.P[0].X, g.P[0].Y = 100, 500 // near the left wall: the left of the ring is outside
	g.P[1].X, g.P[1].Y = 1900, 900
	g.P[0].In = Input{Rock: true}
	steps(g, int(cfg.Ash.RockTomb.Duration*tickRate)+2)
	n := stoneCount(g)
	if n == 0 || n >= cfg.Ash.RockTomb.Count {
		t.Fatalf("%d stones, want some but not all", n)
	}
	for _, b := range g.Balls {
		if b.K == ballStone && (b.X < 0 || b.X > g.court.Length || b.Y < 0 || b.Y > g.court.Width) {
			t.Fatalf("a stone outside the court at %v,%v", b.X, b.Y)
		}
	}
}

func TestStonesBlockTheOpponentButNotMachampOrWhatFliesOverThem(t *testing.T) {
	cfg := DefaultConfig()
	rt := cfg.Ash.RockTomb
	g := mach(cfg, snorlax)
	g.Balls = []Ball{{K: ballStone, Owner: 0, X: 600, Y: 250, HP: rt.StoneHP, Life: 5}}
	g.P[1].X, g.P[1].Y = 600+rt.StoneRadius+cfg.Snorlax.Radius+30, 250 // to its right, walking left into it
	g.P[1].In = Input{DX: -1}
	steps(g, 60)
	if gap := math.Hypot(g.P[1].X-600, g.P[1].Y-250); gap < rt.StoneRadius+cfg.Snorlax.Radius-1e-6 {
		t.Fatalf("the opponent walked into the stone: %v from its middle", gap)
	}
	g.P[0].X, g.P[0].Y = 600+rt.StoneRadius+cfg.Ash.Radius+30, 250
	g.P[0].In = Input{DX: -1}
	steps(g, 60)
	if g.P[0].X > 600 {
		t.Fatalf("Machamp is held by its own stone at %v", g.P[0].X)
	}
	// Above the band a body passes over.
	g = flyer(cfg, rt.MaxHeight+20)
	g.Balls = []Ball{{K: ballStone, Owner: 0, X: 600, Y: 250, HP: rt.StoneHP, Life: 5}}
	g.P[1].X, g.P[1].Y = 600+rt.StoneRadius+cfg.Charizard.Radius+30, 250
	g.P[1].In = Input{DX: -1}
	steps(g, 60)
	if g.P[1].X > 600 {
		t.Fatal("the stone held something above its band")
	}
}

func TestAStoneCrumblesWhenItsTimeIsUpAndIsWornDownByProjectiles(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.RockTomb.StoneDuration = 1
	g := mach(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 1900, 900
	g.Balls = []Ball{{K: ballStone, Owner: 0, X: 600, Y: 250, HP: 100, Life: 1}}
	steps(g, tickRate+2)
	if stoneCount(g) != 0 {
		t.Fatal("the stone outlived stone_duration")
	}
	// A projectile that runs into it is spent, and the stone loses that projectile's damage.
	g = mach(cfg, pikachu)
	g.Balls = []Ball{
		{K: ballStone, Owner: 0, X: 600, Y: 250, HP: 100, Life: 5},
		{K: ballElectro, Owner: 1, X: 603, Y: 250, DX: -1, HP: cfg.Pikachu.ElectroBall.HP},
	}
	g.Balls[0].X0, g.Balls[0].Y0, g.Balls[1].X0, g.Balls[1].Y0 = 600, 250, 603, 250
	g.clashBalls()
	if g.Balls[0].HP != 100-cfg.Pikachu.ElectroBall.ContactDamage {
		t.Fatalf("stone HP %d, want it to lose the ball's damage", g.Balls[0].HP)
	}
	if g.Balls[1].HP > 0 {
		t.Fatalf("the ball kept %d HP after hitting a stone", g.Balls[1].HP)
	}
}

func TestOnlyOneMachampSkillRunsAtATime(t *testing.T) {
	g := mach(DefaultConfig(), snorlax)
	g.P[1].X, g.P[1].Y = 1900, 900
	g.P[0].In = Input{Skill: true, Comet: true, Rock: true, DX: 1}
	g.Step(dt)
	p := g.P[0]
	if p.TreePhase != treeJump || p.CometT != 0 || p.RockT != 0 {
		t.Fatalf("tree %d comet %v rock %v: the main skill should have the tick and the others wait", p.TreePhase, p.CometT, p.RockT)
	}
}

// ---- config, snapshot, picking ----

func TestMachampIsPickableAndGyaradosIsStillOnlyAForm(t *testing.T) {
	if kinds != 9 || ash != 7 || machamp != 8 || gyarados != 9 || forms != 10 {
		t.Fatalf("kinds %d ash %d gyarados %d forms %d", kinds, ash, gyarados, forms)
	}
	r := teamRoom()
	if !r.choose(0, ash) || r.choose(1, gyarados) {
		t.Fatal("Machamp should be pickable and Gyarados not")
	}
}

func TestMachampConfigIsValidated(t *testing.T) {
	for name, mut := range map[string]func(c *Config){
		"jump distance":    func(c *Config) { c.Ash.TreeSlam.JumpDistance = 0 },
		"jump duration":    func(c *Config) { c.Ash.TreeSlam.JumpDuration = 0 },
		"trunk length":     func(c *Config) { c.Ash.TreeSlam.TrunkLength = 0 },
		"trunk width":      func(c *Config) { c.Ash.TreeSlam.TrunkWidth = 0 },
		"leaf radius":      func(c *Config) { c.Ash.TreeSlam.LeafRadius = 0 },
		"leaf angle":       func(c *Config) { c.Ash.TreeSlam.LeafAngle = 361 },
		"trunk scale":      func(c *Config) { c.Ash.TreeSlam.TrunkScale = 0 },
		"fist hp":          func(c *Config) { c.Ash.CometPunch.HP = 0 },
		"fist quantity":    func(c *Config) { c.Ash.CometPunch.Quantity = 0 },
		"too many fists":   func(c *Config) { c.Ash.CometPunch.Quantity = 201 },
		"scatter order":    func(c *Config) { c.Ash.CometPunch.ScatterMaxDistance = c.Ash.CometPunch.ScatterMinDistance - 1 },
		"offset angle":     func(c *Config) { c.Ash.CometPunch.RandomOffsetAngle = 91 },
		"fist duration":    func(c *Config) { c.Ash.CometPunch.FistDuration = 0 },
		"rock duration":    func(c *Config) { c.Ash.RockTomb.Duration = 0 },
		"stone radius":     func(c *Config) { c.Ash.RockTomb.StoneRadius = 0 },
		"stone hp":         func(c *Config) { c.Ash.RockTomb.StoneHP = 0 },
		"stone duration":   func(c *Config) { c.Ash.RockTomb.StoneDuration = 0 },
		"too many stones":  func(c *Config) { c.Ash.RockTomb.Count = 101 },
		"stone band":       func(c *Config) { c.Ash.RockTomb.MaxHeight = -1 },
		"ring radius":      func(c *Config) { c.Ash.RockTomb.RingRadius = -1 },
		"tree band":        func(c *Config) { c.Ash.TreeSlam.MinHeight = 90 },
		"ash max hp":       func(c *Config) { c.Ash.MaxHP = 0 },
		"punch cooldown":   func(c *Config) { c.Ash.CometPunch.Cooldown = -1 },
		"air speed second": func(c *Config) { c.Ash.RockTomb.AirSpeedSecondHalf = -1 },
	} {
		c := DefaultConfig()
		mut(&c)
		if c.validate() == nil {
			t.Errorf("%s: a bad value was accepted", name)
		}
	}
	if err := DefaultConfig().validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEveryPlayerFieldHasItsOwnJSONKey(t *testing.T) {
	seen := map[string]string{}
	ty := reflect.TypeOf(Player{})
	for i := 0; i < ty.NumField(); i++ {
		tag := strings.Split(ty.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if other, dup := seen[tag]; dup {
			t.Errorf("%s and %s share the JSON key %q: encoding/json would drop both", ty.Field(i).Name, other, tag)
		}
		seen[tag] = ty.Field(i).Name
	}
}

func TestMachampStateReachesTheSnapshot(t *testing.T) {
	g := mach(DefaultConfig(), snorlax)
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	b, _ := json.Marshal(g.P[0])
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"trp", "trt", "trx", "try", "trcd", "cpt", "cpcd", "rkt", "rkcd"} {
		if _, ok := m[k]; !ok {
			t.Errorf("snapshot has no %q", k)
		}
	}
}

func TestTreeSmashesTowardTheOpponentNotTheWayItLeapt(t *testing.T) {
	cfg := DefaultConfig()
	g := mach(cfg, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 300, 250, 900, 250
	g.P[0].In = Input{Skill: true, DY: 1} // leaps down, the opponent is to the right
	for n := 0; n < 200 && g.P[0].TreePhase != treeSmash; n++ {
		g.Step(dt)
		g.P[0].In = Input{}
	}
	if g.P[0].TreePhase != treeSmash {
		t.Fatal("never landed")
	}
	dx, dy := g.P[1].X-g.P[0].X, g.P[1].Y-g.P[0].Y
	n := math.Hypot(dx, dy)
	if math.Abs(g.P[0].TreeX-dx/n) > 1e-9 || math.Abs(g.P[0].TreeY-dy/n) > 1e-9 {
		t.Fatalf("tree points %v,%v, want %v,%v: at the opponent", g.P[0].TreeX, g.P[0].TreeY, dx/n, dy/n)
	}
}

func TestTreeSwingsDownAfterBeingHeldAndHitsOnlyWhenTheSwingEnds(t *testing.T) {
	cfg := DefaultConfig()
	ts := cfg.Ash.TreeSlam
	g := mach(cfg, snorlax)
	g.P[0].In = Input{Skill: true, DX: 1}
	g.Step(dt)
	g.P[0].In = Input{}
	for g.P[0].TreePhase == treeJump {
		g.Step(dt)
	}
	held, swung, hp := 0, 0, g.P[1].HP
	for n := 0; n < 200 && g.P[0].TreePhase != treeNone; n++ {
		switch g.P[0].TreePhase {
		case treeSmash:
			held++
		case treeSwing:
			swung++
		}
		if g.P[1].HP != hp {
			t.Fatalf("hurt during phase %d, before the swing ended", g.P[0].TreePhase)
		}
		g.Step(dt)
	}
	if want := int(math.Round(ts.ChargeDuration * tickRate)); held != want {
		t.Fatalf("held %d ticks, want %d", held, want)
	}
	if want := int(math.Round(ts.SwingDuration * tickRate)); swung != want {
		t.Fatalf("swung %d ticks, want %d", swung, want)
	}
	if g.P[1].HP >= hp {
		t.Fatal("the swing never hit")
	}
}

func TestTreeSwingOfZeroHitsRightAfterTheHold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.TreeSlam.SwingDuration = 0
	g := mach(cfg, snorlax)
	g.P[0].In = Input{Skill: true, DX: 1}
	hit := 0
	for n := 0; n < 100 && hit == 0; n++ {
		hit += count(g.Step(dt), "tree")
		g.P[0].In = Input{}
		if g.P[0].TreePhase == treeSwing {
			t.Fatal("entered the swing phase with no time for it")
		}
	}
	if hit != 1 {
		t.Fatalf("%d hits, want 1", hit)
	}
}

// ---- Machamp: the same moves as Ash, with numbers of its own ----

func TestMachampValidatesItsOwnSectionToo(t *testing.T) {
	for name, mut := range map[string]func(c *Config){
		"leaf angle":     func(c *Config) { c.Machamp.TreeSlam.LeafAngle = 361 },
		"too many fists": func(c *Config) { c.Machamp.CometPunch.Quantity = 201 },
		"too many rocks": func(c *Config) { c.Machamp.RockTomb.Count = 101 },
		"max hp":         func(c *Config) { c.Machamp.MaxHP = 0 },
		"stone hp":       func(c *Config) { c.Machamp.RockTomb.StoneHP = 0 },
	} {
		c := DefaultConfig()
		mut(&c)
		if err := c.validate(); err == nil || !strings.Contains(err.Error(), "machamp.") {
			t.Fatalf("%s: error %v, want one naming the machamp section", name, err)
		}
	}
}

func TestMachampIsAshWithNumbersALittleLower(t *testing.T) {
	c := DefaultConfig()
	a, m := c.Ash, c.Machamp
	if !(m.MaxHP < a.MaxHP && m.WalkSpeed < a.WalkSpeed && m.TreeSlam.Damage < a.TreeSlam.Damage && m.TreeSlam.Cooldown > a.TreeSlam.Cooldown &&
		m.TreeSlam.TrunkLength < a.TreeSlam.TrunkLength && m.CometPunch.Quantity < a.CometPunch.Quantity && m.RockTomb.Count < a.RockTomb.Count &&
		m.RockTomb.Cooldown > a.RockTomb.Cooldown && m.RockTomb.StoneHP < a.RockTomb.StoneHP) {
		t.Fatalf("Machamp %+v is not a notch below Ash %+v", m, a)
	}
	if c.maxHPOf(machamp) != m.MaxHP || c.maxHPOf(ash) != a.MaxHP || c.walkSpeedOf(machamp) != m.WalkSpeed || c.radiusOf(machamp) != m.Radius {
		t.Fatal("the HP, speed or size of a Machamp are not its own")
	}
}

// Each of the two reads its own section: change one's numbers and only that one's skills change.
func TestEachBrawlerPlaysByItsOwnNumbers(t *testing.T) {
	for _, kind := range []int{ash, machamp} {
		cfg := DefaultConfig()
		own, other := &cfg.Ash, &cfg.Machamp
		if kind == machamp {
			own, other = other, own
		}
		own.TreeSlam.Damage, other.TreeSlam.Damage = 11, 77
		own.CometPunch.Quantity, other.CometPunch.Quantity = 7, 150
		own.RockTomb.Count, other.RockTomb.Count = 6, 60
		own.RockTomb.RingRadius, other.RockTomb.RingRadius = 300, 400
		// the tree
		g := duel(cfg, kind, snorlax)
		g.P[0].HP, g.P[1].HP = g.maxHP(0), g.maxHP(1)
		hp := g.P[1].HP
		g.P[0].In = Input{Skill: true, DX: 1}
		for n := 0; n < 200 && g.P[1].HP == hp; n++ {
			g.Step(dt)
			g.P[0].In = Input{}
		}
		if got := hp - g.P[1].HP; got != 11 {
			t.Fatalf("kind %d: the tree did %d, want its own 11", kind, got)
		}
		// the stones
		g = duel(cfg, kind, snorlax)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 700, 500, 5000, 5000
		g.P[0].In = Input{Rock: true}
		for n := 0; n < 60; n++ { // jump and land
			g.Step(dt)
			g.P[0].In = Input{}
		}
		if got := stoneCount(g); got < 1 || got > 6 {
			t.Fatalf("kind %d: %d stones, want at most its own 6", kind, got)
		}
		// the fists
		g = duel(cfg, kind, snorlax)
		g.P[0].In = Input{Comet: true}
		total := 0
		for n := 0; n < 100; n++ {
			g.Step(dt)
			g.P[0].In = Input{}
			if f := fistCount(g); f > total {
				total = f
			}
		}
		if total == 0 || total > 7 {
			t.Fatalf("kind %d: %d fists at most, want up to its own 7", kind, total)
		}
	}
}

// ---- a fist shoves the opponent's projectiles back ----

// shoved puts a fist of slot 0 and a projectile of slot 1 on top of each other and says how far the projectile ends up from where it was.
func shoved(cfg Config, kind int, owner int) float64 {
	g := mach(cfg, snorlax)
	g.Balls = []Ball{
		{K: ballFist, Owner: 0, X: 500, Y: 300, X0: 500, Y0: 300, HP: 20},
		{K: kind, Owner: owner, X: 510, Y: 300, X0: 530, Y0: 300, HP: 1000, DX: -1},
	}
	g.clashBalls()
	return g.Balls[1].X - 510
}

func TestFistShovesProjectilesByTheMassItHasOverThemAndTheConfiguredDistance(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.Mass, cfg.Charizard.FireBreath.Mass, cfg.Chikorita.RazorLeaf.Mass, cfg.Rowlet.Arrows.Mass = 50, 20, 20, 20 // as in config.json
	cfg.Machamp.CometPunch.Mass, cfg.Machamp.CometPunch.BallKnockback = 100, 120
	cfg.Ash.CometPunch.Mass, cfg.Ash.CometPunch.BallKnockback = 100, 120
	for name, c := range map[string]struct {
		kind int
		mass float64
	}{
		"electro ball": {ballElectro, cfg.Pikachu.ElectroBall.Mass},
		"razor leaf":   {ballLeaf, cfg.Chikorita.RazorLeaf.Mass},
		"fire ball":    {ballFire, cfg.Charizard.FireBreath.Mass},
		"arrow":        {ballArrow, cfg.Rowlet.Arrows.Mass},
	} {
		want := 120 * (100 - c.mass) / 100
		if got := shoved(cfg, c.kind, 1); math.Abs(got-want) > 1e-6 {
			t.Fatalf("%s: shoved %v, want %v", name, got, want)
		}
	}
	// the distance is the setting: double it and the shove doubles
	cfg.Ash.CometPunch.BallKnockback = 240
	if want := 240 * (100 - cfg.Pikachu.ElectroBall.Mass) / 100; math.Abs(shoved(cfg, ballElectro, 1)-want) > 1e-6 {
		t.Fatalf("with 240: shoved %v, want %v", shoved(cfg, ballElectro, 1), want)
	}
}

func TestFistDoesNotShoveWhatIsNotLighterOrIsItsOwn(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.ElectroBall.Mass, cfg.Charizard.FireBreath.Mass = 50, 20
	cfg.Ash.CometPunch.BallKnockback = 120
	cfg.Ash.CometPunch.Mass = cfg.Pikachu.ElectroBall.Mass // as heavy as the ball
	if got := shoved(cfg, ballElectro, 1); got != 0 {
		t.Fatalf("an equally heavy fist shoved the ball %v", got)
	}
	cfg.Ash.CometPunch.Mass = 100
	cfg.Ash.CometPunch.BallKnockback = 0
	if got := shoved(cfg, ballElectro, 1); got != 0 {
		t.Fatalf("ball_knockback 0 still shoved %v", got)
	}
	cfg.Ash.CometPunch.BallKnockback = 120
	if got := shoved(cfg, ballElectro, 0); got != 0 { // its own player's ball: no clash at all
		t.Fatalf("a fist shoved its own player's ball %v", got)
	}
	cfg.Ash.CometPunch.Mass = 10 // the default: lighter than every projectile
	if got := shoved(cfg, ballFire, 1); got != 0 {
		t.Fatalf("a light fist shoved a fire ball %v", got)
	}
}

func TestAFistThatLandsPushesTheOpponentBackByTheConfiguredDistance(t *testing.T) {
	for _, kb := range []float64{10, 40} {
		cfg := DefaultConfig()
		cfg.Ash.CometPunch.Knockback = kb
		g := mach(cfg, snorlax)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 300, 250, 400, 250
		g.Balls = []Ball{{K: ballFist, Owner: 0, X: 380, Y: 250, DX: 1, HP: 20, Life: 1}}
		hp := g.P[1].HP
		g.stepBalls(dt)
		if g.P[1].HP >= hp {
			t.Fatal("the fist did not hurt")
		}
		if got := g.P[1].X - 400; math.Abs(got-kb) > 1e-6 || g.P[1].Y != 250 {
			t.Fatalf("knockback %v: pushed %v, want it straight away from the fist", kb, got)
		}
	}
	cfg := DefaultConfig()
	g := mach(cfg, snorlax)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 300, 250, 400, 250
	g.P[1].Held = true // pinned by a tornado: it stays where it is held
	g.Balls = []Ball{{K: ballFist, Owner: 0, X: 380, Y: 250, DX: 1, HP: 20, Life: 1}}
	g.stepBalls(dt)
	if g.P[1].X != 400 {
		t.Fatalf("a held body was pushed to %v", g.P[1].X)
	}
}

// A ring of stones whose band reaches higher than a jump goes keeps the jumper in; a lower band is jumped over.
func TestARingWhoseBandIsHigherThanSnorlaxsJumpKeepsItIn(t *testing.T) {
	for _, tc := range []struct {
		maxHeight float64
		escapes   bool
	}{{120, false}, {60, true}} {
		cfg, err := LoadConfig("config.json") // the jump the game really has (its height and the speed it is steered at)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Ash.RockTomb.MaxHeight = tc.maxHeight
		g := duel(cfg, ash, snorlax)
		g.P[0].X, g.P[0].Y = 1000, 500
		g.P[1].X, g.P[1].Y = 1000, 500
		rt := cfg.Ash.RockTomb
		for n := 0; n < rt.Count; n++ {
			a := 2 * math.Pi * float64(n) / float64(rt.Count)
			g.Balls = append(g.Balls, Ball{K: ballStone, Owner: 0, X: 1000 + math.Cos(a)*rt.RingRadius, Y: 500 + math.Sin(a)*rt.RingRadius, HP: 1 << 20, Life: 1000})
		}
		furthest := 0.0
		for tick := 0; tick < int(10*tickRate); tick++ {
			g.P[1].In = Input{Skill: true, DX: 1} // jump and walk at the wall, again and again
			g.Step(dt)
			furthest = math.Max(furthest, math.Hypot(g.P[1].X-1000, g.P[1].Y-500))
		}
		if out := furthest > rt.RingRadius+rt.StoneRadius; out != tc.escapes {
			t.Fatalf("band up to %v: got out %v (furthest %.0f from the middle), want %v", tc.maxHeight, out, furthest, tc.escapes)
		}
	}
}
