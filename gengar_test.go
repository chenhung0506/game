package main

import (
	"math"
	"reflect"
	"testing"
)

// gen is Gengar (slot 0, at 300,250) against foe (slot 1, at 900,250).
func gen(cfg Config, foe int) *Game {
	g := duel(cfg, gengar, foe)
	g.P[0].HP, g.P[1].HP = g.maxHP(0), g.maxHP(1)
	return g
}

func gpress(g *Game, in Input) {
	g.P[0].In = in
	g.Step(dt)
	g.P[0].In = Input{}
}

func TestShadowSneakPutsGengarBelowTheGroundForItsDurationThenBringsItBackAndStartsTheCooldown(t *testing.T) {
	cfg := DefaultConfig()
	ss := cfg.Gengar.ShadowSneak
	g := gen(cfg, snorlax)
	gpress(g, Input{Skill: true})
	if g.P[0].Z != ss.Height || g.P[0].Z >= 0 || g.P[0].SneakT <= 0 {
		t.Fatalf("Z %v sneak %v, want it below the ground at %v", g.P[0].Z, g.P[0].SneakT, ss.Height)
	}
	n := 1
	for g.P[0].SneakT > 0 && n < 1000 {
		g.Step(dt)
		n++
	}
	if want := int(math.Round(ss.Duration * tickRate)); n != want {
		t.Fatalf("it was under for %d ticks, want %d", n, want)
	}
	if g.P[0].Z != 0 {
		t.Fatalf("Z %v after the sneak, want 0", g.P[0].Z)
	}
	gpress(g, Input{Skill: true})
	if g.P[0].SneakT != 0 {
		t.Fatal("it sneaked again during the cooldown")
	}
}

func TestShadowSneakIsSteeredAtTheTwoSpeeds(t *testing.T) {
	cfg := DefaultConfig()
	ss := &cfg.Gengar.ShadowSneak
	ss.Duration, ss.ShadowSpeedFirstHalf, ss.ShadowSpeedSecondHalf = 2, 300, 100
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{Skill: true})
	x := g.P[0].X
	for n := 0; n < int(0.5*tickRate); n++ { // well inside the first half
		g.P[0].In = Input{DX: 1}
		g.Step(dt)
	}
	first := (g.P[0].X - x) / (float64(int(0.5*tickRate)) * dt)
	for g.P[0].SneakT > 1.0 { // up to the second half
		g.P[0].In = Input{DX: 1}
		g.Step(dt)
	}
	x = g.P[0].X
	for n := 0; n < int(0.5*tickRate); n++ {
		g.P[0].In = Input{DX: 1}
		g.Step(dt)
	}
	second := (g.P[0].X - x) / (float64(int(0.5*tickRate)) * dt)
	if math.Abs(first-300) > 1 || math.Abs(second-100) > 1 {
		t.Fatalf("steered at %v then %v, want 300 then 100", first, second)
	}
}

func TestNoSkillThatReachesOnlyFromTheGroundUpCanTouchAGengarUnderIt(t *testing.T) {
	cfg := DefaultConfig()
	var walk func(v reflect.Value, path string)
	n := 0
	walk = func(v reflect.Value, path string) {
		if v.Type() == reflect.TypeOf(Reach{}) {
			n++
			p := &Player{Z: cfg.Gengar.ShadowSneak.Height}
			if r := v.Interface().(Reach); r.hits(p) {
				t.Errorf("%s: %+v reaches %v below the ground", path, r, p.Z)
			}
			return
		}
		if v.Kind() == reflect.Struct {
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i), path+"."+v.Type().Field(i).Name)
			}
		}
	}
	walk(reflect.ValueOf(cfg), "cfg")
	if n < 20 {
		t.Fatalf("only %d height bands found: the walk missed them", n)
	}
	// a band set down to -10 does reach it
	if !(Reach{MinHeight: -10, MaxHeight: 60}).hits(&Player{Z: -10}) {
		t.Fatal("a band down to -10 should reach a Gengar at -10")
	}
}

func TestAGengarUnderTheGroundIsNotHurtByWhatRunsAtTheGround(t *testing.T) {
	cfg := DefaultConfig()
	g := duel(cfg, snorlax, gengar) // Snorlax in slot 0 beams the Gengar in slot 1
	g.P[1].X, g.P[1].Y = 700, 250
	g.P[1].In = Input{Skill: true}
	g.Step(dt)
	g.P[1].In = Input{}
	hp := g.P[1].HP
	g.P[0].In = Input{Beam: true}
	steps(g, 90)
	if g.P[1].HP != hp {
		t.Fatalf("HP %d -> %d: the beam reached a Gengar below the ground", hp, g.P[1].HP)
	}
}

func TestAnAnythingBelowZeroBandDoesReachAGengarUnderTheGround(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snorlax.HyperBeam.MinHeight = -20
	g := duel(cfg, snorlax, gengar)
	g.P[1].X, g.P[1].Y = 700, 250
	g.P[1].In = Input{Skill: true}
	g.Step(dt)
	g.P[1].In = Input{}
	hp := g.P[1].HP
	g.P[0].In = Input{Beam: true}
	steps(g, 90)
	if g.P[1].HP >= hp {
		t.Fatal("a beam whose band starts at -20 did not reach a Gengar at -10")
	}
}

func TestTheShadowSilencesAnOpponentTouchingItAndOnlyThat(t *testing.T) {
	cfg := DefaultConfig()
	ss := cfg.Gengar.ShadowSneak
	g := gen(cfg, pikachu)
	g.P[1].X, g.P[1].Y = 300+ss.ShadowRadius, 250 // inside the shadow
	gpress(g, Input{Skill: true})
	g.P[1].In = Input{Thunder: true}
	g.Step(dt)
	if g.P[1].ThunderLeft > 0 {
		t.Fatal("an opponent under the shadow cast Thunder")
	}
	g.P[1].In = Input{DX: 1}
	x := g.P[1].X
	g.Step(dt)
	if g.P[1].X <= x {
		t.Fatal("an opponent under the shadow could not even walk")
	}
	g = gen(cfg, pikachu)
	g.P[1].X, g.P[1].Y = 300+ss.ShadowRadius+g.radius(1)+40, 250 // just outside it
	gpress(g, Input{Skill: true})
	g.P[1].In = Input{Thunder: true}
	g.Step(dt)
	if g.P[1].ThunderLeft == 0 {
		t.Fatal("an opponent outside the shadow could not cast Thunder")
	}
}

func TestTheShadowDoesNotSilenceAnOpponentOutOfItsBandAndSilenceLingersBriefly(t *testing.T) {
	cfg := DefaultConfig()
	ss := cfg.Gengar.ShadowSneak
	g := gen(cfg, charizard)
	g.P[1].X, g.P[1].Y, g.P[1].Z = 300+ss.ShadowRadius, 250, ss.MaxHeight+30 // flying above it
	gpress(g, Input{Skill: true})
	if g.P[1].SilentT > 0 {
		t.Fatal("an opponent high above the shadow was silenced")
	}
	g.P[1].Z = 0
	g.Step(dt)
	if g.P[1].SilentT <= 0 {
		t.Fatal("the shadow did not silence the opponent on the ground")
	}
	g.P[1].X = 2000
	for n := 0; n < 8; n++ { // a few ticks: longer than the linger
		g.Step(dt)
	}
	if g.P[1].SilentT != 0 {
		t.Fatalf("silence lasts %v after leaving the shadow", g.P[1].SilentT)
	}
}

func TestGengarUsesNoOtherSkillWhileUnderTheGround(t *testing.T) {
	g := gen(DefaultConfig(), snorlax)
	gpress(g, Input{Skill: true})
	gpress(g, Input{GFire: true, SBall: true})
	if len(g.Ghosts) != 0 || len(g.Shadows) != 0 {
		t.Fatal("it cast a skill from under the ground")
	}
}

func TestFreezingOrCatchingAGengarBringsItUp(t *testing.T) {
	g := gen(DefaultConfig(), snorlax)
	gpress(g, Input{Skill: true})
	g.P[0].FrozenT = 1
	g.Step(dt)
	if g.P[0].SneakT != 0 || g.P[0].Z != 0 {
		t.Fatalf("sneak %v Z %v after being frozen", g.P[0].SneakT, g.P[0].Z)
	}
}

func TestBurningGroundDoesNotBurnAGengarUnderIt(t *testing.T) {
	cfg := DefaultConfig()
	g := gen(cfg, snorlax)
	g.P[1].X = 2000
	g.Fires = []FireZone{{Owner: 1, X: 300, Y: 250, R: 200, Left: 5}}
	hp := g.P[0].HP
	gpress(g, Input{Skill: true})
	steps(g, 30)
	if g.P[0].HP != hp {
		t.Fatalf("HP %d -> %d: the fire burned a Gengar under the ground", hp, g.P[0].HP)
	}
}

// ---- Ghost Fire ----

func TestGhostFireLetsOutAnEvenRingThatSpreadsToTheRingRadius(t *testing.T) {
	cfg := DefaultConfig()
	gf := cfg.Gengar.GhostFire
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{GFire: true})
	if len(g.Ghosts) != gf.Count {
		t.Fatalf("%d flames, want %d", len(g.Ghosts), gf.Count)
	}
	cx, cy := g.P[0].X, g.P[0].Y
	var far float64
	off := g.radius(0) + gf.Radius
	for n := 0; n < int(((gf.RingRadius-off)/gf.SpreadSpeed)*tickRate)-3; n++ {
		g.Step(dt)
	}
	for _, f := range g.Ghosts {
		if f.Mode != ghostOut {
			t.Fatalf("flame %+v already turned back", f)
		}
		far = math.Max(far, math.Hypot(f.X-cx, f.Y-cy))
	}
	if far > gf.RingRadius+1 || far < gf.RingRadius-5*gf.SpreadSpeed*dt {
		t.Fatalf("the farthest flame is %v out, want about %v", far, gf.RingRadius)
	}
	steps(g, 6)
	back := 0
	for _, f := range g.Ghosts {
		if f.Mode == ghostBack {
			back++
		}
	}
	if back != gf.Count {
		t.Fatalf("%d of %d flames are coming back", back, gf.Count)
	}
}

func TestGhostFiresFollowGengarWhereverItGoesAndAreGoneWhenTheyReachIt(t *testing.T) {
	cfg := DefaultConfig()
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{GFire: true})
	for n := 0; n < 400 && len(g.Ghosts) > 0; n++ {
		g.P[0].In = Input{DY: 1} // it keeps walking
		g.Step(dt)
		if g.P[0].Y >= g.court.Width {
			g.P[0].Y = g.court.Width
		}
		for _, f := range g.Ghosts {
			if f.Mode == ghostBack {
				d0 := math.Hypot(f.X-g.P[0].X, f.Y-g.P[0].Y)
				if d0 > 2000 {
					t.Fatalf("a flame at %v,%v is far from Gengar at %v,%v", f.X, f.Y, g.P[0].X, g.P[0].Y)
				}
			}
		}
	}
	if len(g.Ghosts) != 0 {
		t.Fatalf("%d flames never got back to Gengar", len(g.Ghosts))
	}
}

func TestEachGhostFireHurtsTheOpponentOnceAndOnlyInItsBand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gengar.GhostFire.Damage = 5
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+150, 250 // inside the ring
	hp := g.P[1].HP
	gpress(g, Input{GFire: true})
	steps(g, 250)
	lost := hp - g.P[1].HP
	if lost <= 0 || lost > 5*cfg.Gengar.GhostFire.Count {
		t.Fatalf("lost %d, want a hit or two from each of at most %d flames", lost, cfg.Gengar.GhostFire.Count)
	}
	if lost%5 != 0 {
		t.Fatalf("lost %d, want whole hits of 5", lost)
	}
	g = gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y, g.P[1].Z = 300+150, 250, 200 // far above them
	hp = g.P[1].HP
	gpress(g, Input{GFire: true})
	for n := 0; n < 250; n++ {
		g.P[1].Z = 200
		g.Step(dt)
	}
	if g.P[1].HP != hp {
		t.Fatalf("flames hurt an opponent far above them: %d -> %d", hp, g.P[1].HP)
	}
}

// ---- Shadow Ball ----

func TestShadowBallLandsOnTheOpponentWhenNoAimWasChosenAndHurtsOnlyWhenItLands(t *testing.T) {
	cfg := DefaultConfig()
	sb := cfg.Gengar.ShadowBall
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+300, 250 // within the range
	hp := g.P[1].HP
	gpress(g, Input{SBall: true})
	if len(g.Shadows) != 1 || math.Abs(g.Shadows[0].TX-g.P[1].X) > 1e-6 {
		t.Fatalf("balls %+v, want one to land on the opponent at %v", g.Shadows, g.P[1].X)
	}
	for n := 0; n < int(sb.Duration*tickRate)-3; n++ {
		g.Step(dt)
		if g.P[1].HP != hp {
			t.Fatal("hurt before the ball landed")
		}
	}
	if z := g.Shadows[0].Z; z <= 0 || z > sb.Height+1e-6 {
		t.Fatalf("height %v in flight, want it up in the air", z)
	}
	steps(g, 8)
	if g.P[1].HP >= hp {
		t.Fatal("the ball did not hurt where it landed")
	}
	if len(g.Shadows) != 0 {
		t.Fatal("the ball is still in the air")
	}
}

func TestShadowBallAtAnOpponentOutOfRangeLandsAsFarAsItGoes(t *testing.T) {
	cfg := DefaultConfig()
	sb := cfg.Gengar.ShadowBall
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 300+sb.Speed*sb.Duration+500, 250
	gpress(g, Input{SBall: true})
	if got := g.Shadows[0].TX - 300; math.Abs(got-sb.Speed*sb.Duration) > 1e-6 {
		t.Fatalf("lands %v away, want the range %v", got, sb.Speed*sb.Duration)
	}
}

func TestShadowBallWithAnAimLandsAtMaxRangeThatWay(t *testing.T) {
	cfg := DefaultConfig()
	sb := cfg.Gengar.ShadowBall
	g := gen(cfg, snorlax)
	g.P[0].X, g.P[0].Y = 800, 500
	g.P[1].X, g.P[1].Y = 1500, 500 // the opponent is to the right, the aim is up
	gpress(g, Input{SBall: true, CastX: 0, CastY: -3})
	b := g.Shadows[0]
	if math.Abs(b.TX-800) > 1e-6 || math.Abs((500-b.TY)-math.Min(500, sb.Speed*sb.Duration)) > 1e-6 {
		t.Fatalf("lands at %v,%v, want straight up from 800,500 as far as the range or the wall", b.TX, b.TY)
	}
}

func TestShadowBallFlamesSpreadOutAndDoNotComeBack(t *testing.T) {
	cfg := DefaultConfig()
	sb := cfg.Gengar.ShadowBall
	g := gen(cfg, snorlax)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{SBall: true})
	for len(g.Ghosts) == 0 && len(g.Shadows) > 0 {
		g.Step(dt)
	}
	if len(g.Ghosts) != sb.GhostFireCount {
		t.Fatalf("%d flames, want %d", len(g.Ghosts), sb.GhostFireCount)
	}
	tx, ty := g.Ghosts[0].X, g.Ghosts[0].Y
	for n := 0; n < int((sb.GhostFireSpreadDistance/sb.GhostFireSpreadSpeed)*tickRate)+5; n++ {
		g.Step(dt)
		for _, f := range g.Ghosts {
			if f.Mode == ghostBack {
				t.Fatal("a flame of the ball turned back")
			}
			if math.Hypot(f.X-tx, f.Y-ty) > sb.GhostFireSpreadDistance+1 {
				t.Fatalf("a flame went %v out, past %v", math.Hypot(f.X-tx, f.Y-ty), sb.GhostFireSpreadDistance)
			}
		}
	}
	if len(g.Ghosts) != 0 {
		t.Fatalf("%d flames left after they had spread their distance", len(g.Ghosts))
	}
}

func TestShadowBallCooldownAndGhostFireCooldownAreTheirOwn(t *testing.T) {
	g := gen(DefaultConfig(), snorlax)
	g.P[1].X, g.P[1].Y = 2000, 900
	gpress(g, Input{SBall: true})
	gpress(g, Input{SBall: true})
	if len(g.Shadows) != 1 {
		t.Fatalf("%d balls: a second one came during the cooldown", len(g.Shadows))
	}
	gpress(g, Input{GFire: true}) // the other skill has not been used
	if len(g.Ghosts) == 0 {
		t.Fatal("Ghost Fire shared the Shadow Ball's cooldown")
	}
}

func TestGengarIsPickableAndItsConfigIsValidated(t *testing.T) {
	r := teamRoom()
	if !r.choose(0, gengar) {
		t.Fatal("a person could not pick Gengar")
	}
	for name, mut := range map[string]func(c *Config){
		"gengar.max_hp":                       func(c *Config) { c.Gengar.MaxHP = 0 },
		"gengar.shadow_sneak.duration":        func(c *Config) { c.Gengar.ShadowSneak.Duration = 0 },
		"gengar.shadow_sneak.shadow_radius":   func(c *Config) { c.Gengar.ShadowSneak.ShadowRadius = 0 },
		"gengar.ghost_fire.count":             func(c *Config) { c.Gengar.GhostFire.Count = 0 },
		"gengar.shadow_ball.duration":         func(c *Config) { c.Gengar.ShadowBall.Duration = 0 },
		"gengar.shadow_ball.speed":            func(c *Config) { c.Gengar.ShadowBall.Speed = 0 },
		"gengar.shadow_ball.ghost_fire_count": func(c *Config) { c.Gengar.ShadowBall.GhostFireCount = 0 },
		"gengar.ghost_fire":                   func(c *Config) { c.Gengar.GhostFire.MinHeight, c.Gengar.GhostFire.MaxHeight = 50, 10 },
	} {
		c := DefaultConfig()
		mut(&c)
		if err := c.validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := DefaultConfig()
	c.Gengar.ShadowBall.MinHeight = -20 // a band that reaches under the ground is allowed
	if err := c.validate(); err != nil {
		t.Fatalf("min_height -20 refused: %v", err)
	}
	c = DefaultConfig()
	c.Gengar.ShadowSneak.Height = 5
	if err := c.validate(); err == nil {
		t.Fatal("a sneak that goes up in the air was accepted")
	}
}
