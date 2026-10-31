package main

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"testing"
)

// vulpixConfig pins the numbers these tests reason about.
func vulpixConfig() Config {
	cfg := DefaultConfig()
	fs, is, fb := &cfg.Vulpix.FrostField, &cfg.Vulpix.IceStorm, &cfg.Vulpix.FreezeBeam
	fs.Reach, is.Reach, fb.Reach = Reach{0, 60}, Reach{0, 60}, Reach{0, 60}
	fs.SprayDuration, fs.Duration, fs.Slow, fs.Boost, fs.Radius, fs.Cooldown = 1.5, 6, 0.5, 0.5, 40, 10
	fs.SlideSpeed, fs.ContactDamage, fs.ExplosionRadius, fs.ExplosionDamage = 400, 10, 100, 15
	is.Radius, is.Interval, is.Damage, is.Strikes, is.FollowSpeed, is.Cooldown, is.FreezeHits, is.FreezeDuration = 80, 0.5, 8, 6, 60, 9, 3, 1.5
	fb.Speed, fb.Cooldown, fb.ControlDuration, fb.Length, fb.PillarRadius, fb.PillarHP, fb.PillarDuration = 240, 12, 3, 450, 24, 25, 8
	return cfg
}

// vg is Vulpix (slot 0, at 300,250) against Snorlax (slot 1, at 900,250).
func vg(cfg Config) *Game { return duel(cfg, vulpix, snorlax) }

func hold(g *Game, slot int, in Input, n int) {
	g.P[slot].In = in
	steps(g, n)
	g.P[slot].In = Input{}
}

func pillars(g *Game, owner int) (n int) {
	for _, b := range g.Balls {
		if b.K == ballPillar && b.Owner == owner {
			n++
		}
	}
	return n
}

// ---- Frost Field ----

// ride plays the Frost Field from the press to the end of the ride, keeping the stick as given, and returns every
// event it produced.
func ride(g *Game, slot int, in Input) (events []Event) {
	fs := g.cfg.Vulpix.FrostField
	g.P[slot].In = in
	for i := 0; i < int(fs.SprayDuration*tickRate)+10; i++ {
		events = append(events, g.Step(dt)...)
		g.P[slot].In.Skill = false // one press
		if g.P[slot].FrostT == 0 {
			break // the ride is over (it ran out, or the iceberg burst)
		}
	}
	g.P[slot].In = Input{}
	return events
}

func count(events []Event, kind string) (n int) {
	for _, e := range events {
		if e.T == kind {
			n++
		}
	}
	return n
}

func TestIcebergSlidesAtSlideSpeedAndLeavesAnUnbrokenTrailThatMelts(t *testing.T) {
	cfg := vulpixConfig()
	cfg.Vulpix.FrostField.SprayDuration = 1.55 // not a whole number of ticks: the last step has to be clipped
	fs := cfg.Vulpix.FrostField
	g := vg(cfg)
	g.P[0].X, g.P[0].Y = 300, 100
	ev := ride(g, 0, Input{Skill: true, DY: 1})
	if want := 100 + fs.SlideSpeed*fs.SprayDuration; math.Abs(g.P[0].Y-want) > 1e-6 || g.P[0].X != 300 {
		t.Fatalf("Vulpix ended at %v,%v, want 300,%v: slide_speed x spray_duration along the stick", g.P[0].X, g.P[0].Y, want)
	}
	ys := []float64{}
	for _, f := range g.Frost {
		if f.Owner != 0 || f.X != 300 {
			t.Fatalf("frost %+v is not Vulpix's or is off its path", f)
		}
		ys = append(ys, f.Y)
	}
	if len(ys) < int(fs.SlideSpeed*fs.SprayDuration/fs.Radius) {
		t.Fatalf("%d bits of frost along %v, want one every %v", len(ys), fs.SlideSpeed*fs.SprayDuration, fs.Radius)
	}
	sort.Float64s(ys)
	if ys[0] != 100 {
		t.Fatalf("the trail starts at %v, want where the ride began (100)", ys[0])
	}
	for i := 1; i < len(ys); i++ {
		if ys[i]-ys[i-1] > fs.Radius+1e-6 {
			t.Fatalf("a gap of %v in the trail (radius %v)", ys[i]-ys[i-1], fs.Radius)
		}
	}
	if ys[len(ys)-1] < g.P[0].Y-fs.Radius {
		t.Fatalf("the trail stops at %v, short of Vulpix at %v", ys[len(ys)-1], g.P[0].Y)
	}
	if count(ev, "iceburst") != 1 {
		t.Fatalf("%d bursts at the end of the ride, want 1", count(ev, "iceburst"))
	}
	steps(g, int((fs.Duration+0.1)*tickRate))
	if len(g.Frost) != 0 {
		t.Fatalf("%d bits of frost left after they should have melted", len(g.Frost))
	}
}

func TestIcebergCannotStopAndKeepsTheWayVulpixFaces(t *testing.T) {
	cfg := vulpixConfig()
	fs := cfg.Vulpix.FrostField
	g := vg(cfg)
	g.P[1].Y = 700 // out of the way
	g.P[0].FX, g.P[0].FY = 0, 1
	ride(g, 0, Input{Skill: true}) // no stick at all
	if want := 250 + fs.SlideSpeed*fs.SprayDuration; math.Abs(g.P[0].Y-want) > 1e-6 {
		t.Fatalf("Vulpix ended at y=%v, want %v: it slides the way it faces", g.P[0].Y, want)
	}
}

func TestIcebergBurstsAtTheEndOfTheRideAndHurtsWhatIsWithinTheExplosionRadius(t *testing.T) {
	cfg := vulpixConfig()
	fs := cfg.Vulpix.FrostField
	for _, c := range []struct {
		name  string
		foeY  float64
		taken int
	}{
		{"inside the radius, not touched on the way", 250 + 90, fs.ExplosionDamage}, // 90 from the end: more than radius+body (40+30), less than 100+30
		{"outside the radius", 250 + 140, 0},
	} {
		g := vg(cfg)
		g.P[1].X, g.P[1].Y = 900, c.foeY // the ride ends at (900, 250)
		hp := g.P[1].HP
		ev := ride(g, 0, Input{Skill: true, DX: 1})
		if got := hp - g.P[1].HP; got != c.taken || count(ev, "iceburst") != 1 {
			t.Errorf("%s: took %d (want %d), %d bursts", c.name, got, c.taken, count(ev, "iceburst"))
		}
		if math.Abs(g.P[0].X-(300+fs.SlideSpeed*fs.SprayDuration)) > 1e-6 {
			t.Errorf("%s: Vulpix ended at %v", c.name, g.P[0].X)
		}
	}
}

func TestIcebergBurstsOnTheOpponentWithBothBlowsAndTheRideIsOver(t *testing.T) {
	cfg := vulpixConfig()
	fs := cfg.Vulpix.FrostField
	g := vg(cfg)
	g.P[1].X = 600 // in the way
	hp := g.P[1].HP
	ev := ride(g, 0, Input{Skill: true, DX: 1})
	if got, want := hp-g.P[1].HP, fs.ContactDamage+fs.ExplosionDamage; got != want {
		t.Fatalf("Snorlax took %d, want contact %d + explosion %d", got, fs.ContactDamage, fs.ExplosionDamage)
	}
	if count(ev, "iceburst") != 1 {
		t.Fatalf("%d bursts, want 1", count(ev, "iceburst"))
	}
	touch := 600 - fs.Radius - cfg.Snorlax.Radius // where the iceberg first reaches it; the step that gets there ends at most one step (slide_speed*dt) further
	if g.P[0].FrostT != 0 || g.P[0].X < touch || g.P[0].X > touch+fs.SlideSpeed*dt+1e-6 {
		t.Fatalf("FrostT %v, Vulpix at x=%v: the ride should have ended at the touch (%v)", g.P[0].FrostT, g.P[0].X, touch)
	}
	for _, e := range ev {
		if e.T == "iceburst" && e.R != fs.ExplosionRadius {
			t.Fatalf("the burst says radius %v, want %v", e.R, fs.ExplosionRadius)
		}
	}
}

func TestTheBurstLeavesAPatchOfFrostAsWideAsTheExplosionThatSlowsTheOpponent(t *testing.T) {
	cfg := vulpixConfig()
	fs := cfg.Vulpix.FrostField
	for _, c := range []struct {
		name string
		foeX float64 // 600 = in the way (a contact burst), 1500 = out of the way (the ride runs out)
	}{{"on contact", 600}, {"at the end of the ride", 1500}} {
		g := vg(cfg)
		g.P[1].X, g.P[1].Y = c.foeX, 250
		if c.foeX > 900 {
			g.P[1].Y = 700
		}
		ride(g, 0, Input{Skill: true, DX: 1})
		var big *Frost
		for i := range g.Frost {
			if g.Frost[i].R > 0 {
				if big != nil {
					t.Fatalf("%s: two bursts' worth of frost", c.name)
				}
				big = &g.Frost[i]
			}
		}
		if big == nil || big.R != fs.ExplosionRadius || big.Owner != 0 || big.Left > fs.Duration || big.Left < fs.Duration-0.2 ||
			math.Hypot(big.X-g.P[0].X, big.Y-g.P[0].Y) > 1e-9 {
			t.Fatalf("%s: patch %+v, want one %v wide owned by Vulpix where it burst (%v,%v), lasting %v", c.name, big, fs.ExplosionRadius, g.P[0].X, g.P[0].Y, fs.Duration)
		}
		// the opponent walks slower anywhere inside it, and not outside it
		g.Frost = []Frost{*big}
		for _, p := range []struct {
			dx   float64
			slow bool
		}{{big.R - 1, true}, {big.R + 1, false}} {
			g.P[1].X, g.P[1].Y, g.P[1].Z = big.X+p.dx, big.Y, 0
			want := 1.0
			if p.slow {
				want = 1 - fs.Slow
			}
			if f := g.frostFactor(1); math.Abs(f-want) > 1e-9 {
				t.Errorf("%s: factor %v at %v from the middle, want %v", c.name, f, p.dx, want)
			}
		}
	}
}

func TestNoExplosionRadiusMeansNoFrostFromTheBurst(t *testing.T) {
	cfg := vulpixConfig()
	cfg.Vulpix.FrostField.ExplosionRadius = 0
	g := vg(cfg)
	g.P[1].Y = 700
	ride(g, 0, Input{Skill: true, DX: 1})
	for _, f := range g.Frost {
		if f.R != 0 {
			t.Fatalf("a patch %+v although the explosion has no radius", f)
		}
	}
}

func TestIcebergPassesOverAnOpponentAboveTheBand(t *testing.T) {
	cfg := vulpixConfig()
	cfg.Vulpix.FrostField.Reach = Reach{MinHeight: 0, MaxHeight: 15}
	g := duel(cfg, vulpix, rowlet) // Rowlet flies at fly_high, above the band
	g.P[1].X, g.P[1].Y = 600, 250
	if cfg.Rowlet.FlyHigh <= cfg.Vulpix.FrostField.MaxHeight {
		t.Fatal("the scene needs a Rowlet above the band")
	}
	hp := g.P[1].HP
	ev := ride(g, 0, Input{Skill: true, DX: 1})
	if g.P[1].HP != hp || g.P[0].X < 300+cfg.Vulpix.FrostField.SlideSpeed*cfg.Vulpix.FrostField.SprayDuration-1 {
		t.Fatalf("Rowlet HP %d -> %d, Vulpix at %v: the iceberg should have gone under it", hp, g.P[1].HP, g.P[0].X)
	}
	if count(ev, "iceburst") != 1 {
		t.Fatalf("%d bursts", count(ev, "iceburst"))
	}
}

func TestIcebergCooldownBlocksASecondRideAndCountsFromTheStart(t *testing.T) {
	cfg := vulpixConfig()
	g := vg(cfg)
	g.P[1].Y = 700
	ride(g, 0, Input{Skill: true, DX: 1})
	x := g.P[0].X
	g.P[0].In = Input{Skill: true}
	steps(g, 5)
	if g.P[0].FrostT != 0 || g.P[0].X != x {
		t.Fatalf("a second ride started during the cooldown (FrostT %v, x %v -> %v)", g.P[0].FrostT, x, g.P[0].X)
	}
	if cd := g.P[0].FrostCD; cd <= 0 || cd > cfg.Vulpix.FrostField.Cooldown-cfg.Vulpix.FrostField.SprayDuration+0.2 {
		t.Fatalf("FrostCD = %v: it counts from the start of the ride", cd)
	}
}

func TestFrostIsCappedAndTheOldestGoesFirst(t *testing.T) {
	g := vg(vulpixConfig())
	for i := 0; i < maxFrost+10; i++ {
		g.layFrost(0, float64(i), 0)
	}
	if len(g.Frost) != maxFrost || g.Frost[0].X != 10 || g.Frost[maxFrost-1].X != float64(maxFrost+9) {
		t.Fatalf("%d bits of frost, first at x=%v last at x=%v", len(g.Frost), g.Frost[0].X, g.Frost[len(g.Frost)-1].X)
	}
}

func TestFrostSpeedsUpTheOwnerAndSlowsTheOtherPlayerOnlyInsideThePatchAndTheBand(t *testing.T) {
	cfg := vulpixConfig()
	fs := cfg.Vulpix.FrostField
	walk := func(kind int, onOwn, onFoe bool, z float64) float64 {
		g := duel(cfg, vulpix, kind)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 500, 100, 500, 400
		if onOwn {
			g.Frost = append(g.Frost, Frost{Owner: 0, X: 500, Y: 400, Left: 5}) // slot 1 stands on Vulpix's frost
		}
		if onFoe {
			g.Frost = append(g.Frost, Frost{Owner: 1, X: 500, Y: 400, Left: 5}) // slot 1 stands on its own
		}
		g.P[1].Z = z
		g.P[1].In.DX = 1
		x0 := g.P[1].X
		steps(g, 4)                       // short enough to stay on a patch
		return (g.P[1].X - x0) / (4 * dt) // speed
	}
	base := walk(pikachu, false, false, 0)
	if want := cfg.Pikachu.WalkSpeed; math.Abs(base-want) > 1 {
		t.Fatalf("base speed %v, want %v", base, want)
	}
	if got, want := walk(pikachu, true, false, 0), base*(1-fs.Slow); math.Abs(got-want) > 1 {
		t.Fatalf("on the opponent's frost: %v, want %v", got, want)
	}
	if got, want := walk(pikachu, false, true, 0), base*(1+fs.Boost); math.Abs(got-want) > 1 {
		t.Fatalf("on its own frost: %v, want %v", got, want)
	}
	if got, want := walk(pikachu, true, true, 0), base*(1+fs.Boost)*(1-fs.Slow); math.Abs(got-want) > 1 {
		t.Fatalf("on both: %v, want %v", got, want)
	}
	if got := walk(pikachu, true, true, 100); math.Abs(got-base) > 1 {
		t.Fatalf("above the band (z=100): %v, want %v", got, base)
	}
}

func TestFrostOnlyCountsWhereTheBodyCentreIs(t *testing.T) {
	cfg := vulpixConfig()
	g := vg(cfg)
	g.Frost = []Frost{{Owner: 1, X: 300 + cfg.Vulpix.FrostField.Radius + 1, Y: 250, Left: 5}}
	if f := g.frostFactor(0); f != 1 {
		t.Fatalf("factor %v just outside the patch, want 1", f)
	}
	g.Frost[0].X = 300 + cfg.Vulpix.FrostField.Radius - 1
	if f := g.frostFactor(0); math.Abs(f-0.5) > 1e-9 {
		t.Fatalf("factor %v just inside the foe's patch, want 0.5", f)
	}
}

func TestFrostAndPillarsAreClearedWhenTheRoundResets(t *testing.T) {
	g := vg(vulpixConfig())
	g.Frost = []Frost{{Left: 3}}
	g.Balls = []Ball{{K: ballPillar, HP: 5, Life: 3}}
	g.Reset()
	if len(g.Frost) != 0 || len(g.Balls) != 0 {
		t.Fatalf("frost %v balls %v survived a reset", g.Frost, g.Balls)
	}
}

// ---- Ice Storm ----

func TestIceStormStartsOnVulpixChasesAndFreezesOnEveryNthHit(t *testing.T) {
	cfg := vulpixConfig()
	is := cfg.Vulpix.IceStorm
	g := vg(cfg)
	g.P[1].X, g.P[1].Y = 340, 250 // inside the radius from the start
	g.P[1].Kind = pikachu
	g.P[0].In.Storm = true
	steps(g, 1)
	g.P[0].In = Input{}
	if g.P[0].StormLeft != is.Strikes || g.P[0].StormCD <= 0 {
		t.Fatalf("StormLeft %d StormCD %v after the cast", g.P[0].StormLeft, g.P[0].StormCD)
	}
	if math.Abs(g.P[0].StormX-300) > is.FollowSpeed*dt+1e-9 {
		t.Fatalf("the storm began at %v, want on Vulpix (300)", g.P[0].StormX)
	}
	hits, frozenAfter := 0, []int{}
	hp := g.P[1].HP
	for tick := 0; tick < 5*tickRate && g.P[0].StormLeft > 0; tick++ {
		g.P[1].FrozenT = 0 // see only what a strike sets
		g.Step(dt)
		if g.P[1].HP < hp {
			hits++
			if g.P[1].HP != hp-is.Damage {
				t.Fatalf("a hit took %d HP, want %d", hp-g.P[1].HP, is.Damage)
			}
			hp = g.P[1].HP
			if g.P[1].FrozenT > 0 {
				frozenAfter = append(frozenAfter, hits)
				if g.P[1].FrozenT < is.FreezeDuration-dt-1e-9 || g.P[1].FrozenT > is.FreezeDuration { // the same tick already thawed it by dt
					t.Fatalf("frozen for %v, want %v", g.P[1].FrozenT, is.FreezeDuration)
				}
			}
		}
	}
	if hits != is.Strikes {
		t.Fatalf("%d hits, want %d strikes", hits, is.Strikes)
	}
	if len(frozenAfter) != 2 || frozenAfter[0] != 3 || frozenAfter[1] != 6 {
		t.Fatalf("frozen after hits %v, want 3 and 6", frozenAfter)
	}
}

func TestIceStormChasesTheOpponentAtFollowSpeedAndMissesOutsideTheBandOrRadius(t *testing.T) {
	cfg := vulpixConfig()
	is := cfg.Vulpix.IceStorm
	g := vg(cfg) // the opponent is 600 away
	g.P[0].In.Storm = true
	steps(g, 1)
	g.P[0].In = Input{}
	x0 := g.P[0].StormX
	steps(g, tickRate)
	if got := g.P[0].StormX - x0; math.Abs(got-is.FollowSpeed) > 1 {
		t.Fatalf("the storm moved %v in a second, want follow_speed %v", got, is.FollowSpeed)
	}
	if g.P[1].HP != cfg.Snorlax.MaxHP {
		t.Fatal("it hit from 500 away")
	}
	// the opponent leaps out of the band: strikes pass underneath it
	g = vg(cfg)
	g.P[1].X, g.P[1].Y, g.P[1].Z = 320, 250, 100
	g.P[0].In.Storm = true
	steps(g, 1)
	g.P[0].In = Input{}
	for k := 0; k < 4*tickRate; k++ {
		g.P[1].Z = 100
		g.Step(dt)
	}
	if g.P[1].HP != cfg.Snorlax.MaxHP || g.P[0].StormLeft != 0 {
		t.Fatalf("HP %d StormLeft %d: strikes at a body above the band must miss and still be used up", g.P[1].HP, g.P[0].StormLeft)
	}
}

func TestIceStormDoesNotRootVulpixAndHasACooldown(t *testing.T) {
	cfg := vulpixConfig()
	g := vg(cfg)
	g.P[0].In = Input{Storm: true, DX: -1}
	steps(g, tickRate)
	if g.P[0].X > 300-cfg.Vulpix.WalkSpeed*0.9 {
		t.Fatalf("Vulpix at %v: it should walk on while the storm runs", g.P[0].X)
	}
	left := g.P[0].StormLeft
	g.P[0].In = Input{Storm: true}
	steps(g, 3)
	if g.P[0].StormLeft > left {
		t.Fatal("a new cast was taken while the storm ran")
	}
	g.P[0].StormLeft = 0
	steps(g, 3)
	if g.P[0].StormLeft != 0 {
		t.Fatal("a new cast was taken during the cooldown")
	}
}

func TestIceStormStrikesWearDownTheOpponentsProjectilesInItsArea(t *testing.T) {
	cfg := vulpixConfig()
	cfg.Vulpix.IceStorm.FollowSpeed = 0 // the storm stays where it began
	is := cfg.Vulpix.IceStorm
	g := duel(cfg, vulpix, pikachu)
	g.P[1].X, g.P[1].Y = 1800, 900           // far from the storm: no body is hit, the balls still are
	ring := func(k int) (float64, float64) { // seven spots round the storm's middle, far enough apart not to clash with each other
		a := float64(k) * 2 * math.Pi / 7
		return g.P[0].X + math.Cos(a)*55, g.P[0].Y + math.Sin(a)*55
	}
	spot := func(k int, b Ball) Ball { b.X, b.Y = ring(k); return b }
	g.Balls = []Ball{
		spot(0, Ball{Owner: 1, HP: cfg.Pikachu.ElectroBall.HP}),                                // electro, inside
		spot(1, Ball{K: ballFire, Owner: 1, HP: 50, Left: 400}),                                // fire, inside
		spot(2, Ball{K: ballLeaf, Owner: 1, HP: 50}),                                           // leaf, inside
		spot(3, Ball{K: ballLeaf, Owner: 1, HP: 50, Wait: true, Delay: 9}),                     // a leaf still waiting, inside
		{Owner: 1, X: g.P[0].X + is.Radius + 200, Y: g.P[0].Y, HP: cfg.Pikachu.ElectroBall.HP}, // electro, outside
		spot(5, Ball{K: ballFire, Owner: 0, HP: 50, Left: 400}),                                // the storm owner's own, inside
		spot(6, Ball{K: ballPillar, Owner: 1, HP: 50, Life: 30}),                               // the other player's pillar, inside
	}
	g.P[0].In.Storm = true
	steps(g, 1)
	g.P[0].In = Input{}
	for k := 0; k < int(is.Interval*tickRate)+2 && g.P[0].StormLeft == is.Strikes; k++ {
		g.Step(dt) // up to the first strike
	}
	if g.P[0].StormLeft != is.Strikes-1 || len(g.Balls) != 7 {
		t.Fatalf("StormLeft %d with %d balls: the first strike should just have landed and nothing else changed", g.P[0].StormLeft, len(g.Balls))
	}
	want := []int{cfg.Pikachu.ElectroBall.HP - is.Damage, 50 - is.Damage, 50 - is.Damage, 50 - is.Damage, cfg.Pikachu.ElectroBall.HP, 50, 50 - is.Damage}
	for j, w := range want {
		if got := g.Balls[j].HP; got != w {
			t.Errorf("ball %d (%+v): HP %d, want %d", j, g.Balls[j], got, w)
		}
	}
}

// ---- Freeze Beam ----

func TestFreezeBeamStartsAsFarFromVulpixAsStartDistanceSays(t *testing.T) {
	for _, d := range []float64{0, 40, 130, 450} {
		cfg := vulpixConfig()
		cfg.Vulpix.FreezeBeam.StartDistance = d
		g := vg(cfg)
		g.P[0].AimX, g.P[0].AimY = 0, 1 // not along the court's length, so x and y are not mixed up
		g.P[0].In.Freeze = true
		steps(g, 1)
		if p := g.P[0]; p.FBeamX != 300 || math.Abs(p.FBeamY-(250+d)) > 1e-9 {
			t.Errorf("start_distance %v: the beam starts at %v,%v, want 300,%v", d, p.FBeamX, p.FBeamY, 250+d)
		}
	}
}

func TestFreezeBeamRootsVulpixAndTheStickMovesItsEndAtSpeed(t *testing.T) {
	cfg := vulpixConfig()
	fb := cfg.Vulpix.FreezeBeam
	g := vg(cfg)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.P[0].In.Freeze = true
	steps(g, 1)
	if want := 300 + fb.StartDistance; math.Abs(g.P[0].FBeamX-want) > 1e-9 || g.P[0].FBeamY != 250 {
		t.Fatalf("the beam starts at %v,%v, want %v,250: start_distance along the aim", g.P[0].FBeamX, g.P[0].FBeamY, want)
	}
	if g.P[0].FBeamT <= 0 || g.P[0].FBeamCD <= 0 {
		t.Fatalf("FBeamT %v FBeamCD %v", g.P[0].FBeamT, g.P[0].FBeamCD)
	}
	g.P[0].In = Input{DY: 1}
	steps(g, tickRate/2)
	if g.P[0].X != 300 || g.P[0].Y != 250 {
		t.Fatalf("Vulpix walked to %v,%v while it breathed the beam", g.P[0].X, g.P[0].Y)
	}
	if got := g.P[0].FBeamY - 250; math.Abs(got-fb.Speed*0.5) > 1.5 {
		t.Fatalf("the end moved %v in half a second, want %v", got, fb.Speed*0.5)
	}
	if got := math.Hypot(g.P[0].FBeamX-300, g.P[0].FBeamY-250); got > fb.Length+1e-6 {
		t.Fatalf("the end is %v from the body, more than length %v", got, fb.Length)
	}
}

func TestFreezeBeamEndStaysWithinLengthAndInsideTheCourt(t *testing.T) {
	cfg := vulpixConfig()
	fb := cfg.Vulpix.FreezeBeam
	g := vg(cfg)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.P[0].In.Freeze = true
	steps(g, 1)
	g.P[0].In = Input{DX: 1}
	steps(g, tickRate) // 240 more would reach 525 > length
	if got := math.Hypot(g.P[0].FBeamX-300, g.P[0].FBeamY-250); math.Abs(got-fb.Length) > 1e-6 {
		t.Fatalf("the end is %v from the body, want exactly length %v", got, fb.Length)
	}
	g = vg(cfg)
	g.P[0].X, g.P[0].Y = 300, 30
	g.P[0].AimX, g.P[0].AimY = 0, -1
	g.P[0].In.Freeze = true
	steps(g, 1)
	if g.P[0].FBeamY < 0 {
		t.Fatalf("the end left the court: y = %v", g.P[0].FBeamY)
	}
}

func TestFreezeBeamLeavesAPillarEverySoOftenAlongTheWayWithoutPilingThemUp(t *testing.T) {
	cfg := vulpixConfig()
	fb := cfg.Vulpix.FreezeBeam
	g := vg(cfg)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.P[0].In.Freeze = true
	steps(g, 1)
	if pillars(g, 0) != 1 {
		t.Fatalf("%d pillars at the start, want 1 where the beam first touches the ground", pillars(g, 0))
	}
	steps(g, 5) // the end stands still: still one
	if pillars(g, 0) != 1 {
		t.Fatalf("%d pillars while the end stood still, want 1", pillars(g, 0))
	}
	g.P[0].In = Input{DY: 1}
	steps(g, tickRate) // 240 along
	n := pillars(g, 0)
	gap := fb.PillarRadius * 1.5
	if want := int(fb.Speed/gap) + 1; n < want-1 || n > want+1 {
		t.Fatalf("%d pillars after a %v-long sweep, want about %d", n, fb.Speed, want)
	}
	for _, b := range g.Balls {
		if b.K == ballPillar && (b.HP != fb.PillarHP || b.Life <= 0 || b.Life > fb.PillarDuration) {
			t.Fatalf("pillar %+v: want HP %d and a life up to %v", b, fb.PillarHP, fb.PillarDuration)
		}
	}
	g.P[0].In = Input{DY: -1} // back over the same ground: nothing new
	steps(g, tickRate/2)
	if pillars(g, 0) != n {
		t.Fatalf("pillars grew %d -> %d while sweeping back over them", n, pillars(g, 0))
	}
}

func TestAFastEndLeavesNoGapBetweenPillars(t *testing.T) {
	cfg := vulpixConfig()
	cfg.Vulpix.FreezeBeam.Speed = 1500 // 50 per tick, more than two pillars wide
	g := vg(cfg)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.P[0].In.Freeze = true
	steps(g, 1)
	g.P[0].In = Input{DY: 1}
	steps(g, 8)
	var ys []float64
	for _, b := range g.Balls {
		if b.K == ballPillar {
			ys = append(ys, b.Y)
		}
	}
	sort.Float64s(ys)
	if len(ys) < 8 {
		t.Fatalf("only %d pillars along a 400-long sweep", len(ys))
	}
	for k := 1; k < len(ys); k++ {
		if gap := ys[k] - ys[k-1]; gap > 2*cfg.Vulpix.FreezeBeam.PillarRadius {
			t.Fatalf("pillars %v and %v are %v apart, more than a pillar's width: a body could slip through", ys[k-1], ys[k], gap)
		}
	}
}

func TestAnotherPlayersPillarsDoNotStopOneFromRisingAndTwoOwnersPillarsDoNotBreakEachOther(t *testing.T) {
	cfg := vulpixConfig()
	g := duel(cfg, vulpix, vulpix)
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.Balls = []Ball{{K: ballPillar, Owner: 1, X: 525, Y: 250, HP: 9, Life: 5}} // exactly where slot 0's beam will start
	g.P[0].In.Freeze = true
	steps(g, 1)
	if pillars(g, 0) != 1 {
		t.Fatalf("slot 0 raised %d pillars, want 1: only its own count against a new one", pillars(g, 0))
	}
	for _, b := range g.Balls {
		if b.HP != 9 && b.HP != cfg.Vulpix.FreezeBeam.PillarHP {
			t.Fatalf("overlapping pillars of two owners hurt each other: %+v", g.Balls)
		}
	}
	if len(g.Balls) != 2 {
		t.Fatalf("%d balls, want both pillars: %+v", len(g.Balls), g.Balls)
	}
}

func TestPillarsAreCappedWhateverTheConfigSays(t *testing.T) {
	cfg := vulpixConfig()
	g := duel(cfg, vulpix, vulpix)
	for k := 0; k < maxPillars; k++ {
		g.Balls = append(g.Balls, Ball{K: ballPillar, Owner: 1, X: 1500, Y: float64(k), HP: 5, Life: 5})
	}
	g.P[0].In.Freeze = true
	steps(g, 3)
	if pillars(g, 0) != 0 || len(g.Balls) != maxPillars {
		t.Fatalf("%d balls, %d of slot 0's: the cap of %d must hold", len(g.Balls), pillars(g, 0), maxPillars)
	}
}

func TestFreezeBeamEndsAfterTheControlTimeHasACooldownAndHurtsNobody(t *testing.T) {
	cfg := vulpixConfig()
	fb := cfg.Vulpix.FreezeBeam
	g := vg(cfg)
	g.P[1].X, g.P[1].Y = 520, 250 // right on the line the beam starts along
	g.P[0].AimX, g.P[0].AimY = 1, 0
	g.P[0].In.Freeze = true
	steps(g, 1)
	g.P[0].In = Input{}
	steps(g, int(fb.ControlDuration*tickRate)+3)
	if g.P[0].FBeamT != 0 {
		t.Fatalf("FBeamT = %v, the beam should be over", g.P[0].FBeamT)
	}
	if g.P[1].HP != cfg.Snorlax.MaxHP || g.P[1].FrozenT != 0 {
		t.Fatalf("the beam hurt the opponent: HP %d frozen %v", g.P[1].HP, g.P[1].FrozenT)
	}
	g.P[0].In.Freeze = true
	steps(g, 3)
	if g.P[0].FBeamT != 0 {
		t.Fatal("a second beam started during the cooldown")
	}
	g.P[0].X = 100
	g.P[0].In = Input{DX: 1}
	steps(g, 3)
	if g.P[0].X <= 100 {
		t.Fatal("Vulpix is still rooted after its beam")
	}
}

func TestFrozenVulpixLosesItsBeamAndItsRideWithoutABurst(t *testing.T) {
	cfg := vulpixConfig()
	g := vg(cfg)
	g.P[0].In = Input{Freeze: true}
	steps(g, 2)
	if g.P[0].FBeamT == 0 {
		t.Fatal("the beam did not start")
	}
	g.P[0].FrozenT = 1
	steps(g, 1)
	if g.P[0].FBeamT != 0 {
		t.Fatalf("FBeamT %v after freezing", g.P[0].FBeamT)
	}

	g = vg(cfg)
	g.P[1].Y = 700
	g.P[0].In = Input{Skill: true, DX: 1}
	steps(g, 2)
	if g.P[0].FrostT == 0 {
		t.Fatal("the ride did not start")
	}
	g.P[0].FrozenT = 1
	var ev []Event
	for i := 0; i < 3; i++ {
		ev = append(ev, g.Step(dt)...)
	}
	if g.P[0].FrostT != 0 || count(ev, "iceburst") != 0 {
		t.Fatalf("FrostT %v, %d bursts after freezing: the iceberg should just be gone", g.P[0].FrostT, count(ev, "iceburst"))
	}
}

func TestBoundOrParalyzedVulpixLosesItsRideWithoutABurst(t *testing.T) {
	for name, set := range map[string]func(p *Player){"bound": func(p *Player) { p.BoundT = 1 }, "paralyzed": func(p *Player) { p.ParalyzeT = 1 }} {
		g := vg(vulpixConfig())
		g.P[1].Y = 700
		g.P[0].In = Input{Skill: true, DX: 1}
		steps(g, 2)
		set(&g.P[0])
		var ev []Event
		for i := 0; i < 3; i++ {
			ev = append(ev, g.Step(dt)...)
		}
		if g.P[0].FrostT != 0 || count(ev, "iceburst") != 0 {
			t.Errorf("%s: FrostT %v, %d bursts", name, g.P[0].FrostT, count(ev, "iceburst"))
		}
	}
}

func TestTheRideAndTheBeamDoNotStartOnTopOfEachOther(t *testing.T) {
	g := vg(vulpixConfig())
	g.P[1].Y = 700
	g.P[0].In = Input{Skill: true, Freeze: true, DX: 1}
	steps(g, 3)
	if g.P[0].FrostT == 0 || g.P[0].FBeamT != 0 {
		t.Fatalf("FrostT %v FBeamT %v: pressed together, the ride wins", g.P[0].FrostT, g.P[0].FBeamT)
	}

	g = vg(vulpixConfig())
	g.P[1].Y = 700
	g.P[0].In = Input{Freeze: true}
	steps(g, 3)
	g.P[0].In = Input{Skill: true, DX: 1}
	steps(g, 3)
	if g.P[0].FBeamT == 0 || g.P[0].FrostT != 0 {
		t.Fatalf("FBeamT %v FrostT %v: the beam roots Vulpix, so there is no ride in the middle of it", g.P[0].FBeamT, g.P[0].FrostT)
	}
}

// ---- Pillars ----

func TestPillarBlocksTheOtherPlayerButNotItsOwnerAndNotAboveTheBand(t *testing.T) {
	cfg := vulpixConfig()
	fb := cfg.Vulpix.FreezeBeam
	try := func(slot int, z float64) float64 {
		g := vg(cfg)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 800, 100, 900, 250
		g.P[0].Kind, g.P[1].Kind = vulpix, vulpix
		g.Balls = []Ball{{K: ballPillar, Owner: 0, X: 600, Y: 250, HP: 5, Life: 20}}
		if slot == 0 {
			g.P[0].X, g.P[0].Y = 900, 250
		}
		g.P[slot].Z = z
		g.P[slot].In.DX = -1
		for k := 0; k < 3*tickRate; k++ {
			g.P[slot].Z = z
			g.Step(dt)
		}
		return g.P[slot].X
	}
	gap := cfg.Vulpix.Radius + fb.PillarRadius
	if x := try(1, 0); x < 600+gap-1e-6 || x > 600+gap+1 {
		t.Fatalf("the other player stopped at x=%v, want just outside the pillar (%v)", x, 600+gap)
	}
	if x := try(0, 0); x > 600-gap { // its owner walks through
		t.Fatalf("the owner stopped at %v: its own pillar must not block it", x)
	}
	if x := try(1, 100); x > 600-gap {
		t.Fatalf("a body at z=100, above the band, was blocked at %v", x)
	}
}

func TestAPillarMeltsAfterItsLifeAndFreesThePath(t *testing.T) {
	cfg := vulpixConfig()
	g := vg(cfg)
	g.Balls = []Ball{{K: ballPillar, Owner: 0, X: 600, Y: 250, HP: 5, Life: 1}}
	steps(g, tickRate-1)
	if pillars(g, 0) != 1 {
		t.Fatal("it melted early")
	}
	steps(g, 3)
	if pillars(g, 0) != 0 {
		t.Fatal("it did not melt")
	}
}

func TestPillarPushesOutAPlayerItLandsOn(t *testing.T) {
	cfg := vulpixConfig()
	g := vg(cfg)
	g.P[1].X, g.P[1].Y = 700, 250
	g.Balls = []Ball{{K: ballPillar, Owner: 0, X: 700, Y: 250, HP: 5, Life: 5}} // exactly on its centre
	g.Step(dt)
	if got := math.Hypot(g.P[1].X-700, g.P[1].Y-250); got < cfg.Snorlax.Radius+cfg.Vulpix.FreezeBeam.PillarRadius-1e-6 {
		t.Fatalf("still %v from the pillar's centre", got)
	}
}

func TestOpponentsProjectilesWearAPillarDownAndAreSpent(t *testing.T) {
	cfg := vulpixConfig()
	dmg := cfg.Charizard.FireBreath.Damage
	g := duel(cfg, vulpix, charizard)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 100, 100, 1100, 100
	g.Balls = []Ball{
		{K: ballPillar, Owner: 0, X: 600, Y: 300, HP: 2 * dmg, Life: 20}, // two fire balls break it
		{K: ballFire, Owner: 1, X: 640, Y: 300, DX: -1, HP: 5, Left: 400},
	}
	g.stepBalls(dt)
	if len(g.Balls) != 1 || g.Balls[0].K != ballPillar || g.Balls[0].HP != dmg {
		t.Fatalf("balls %+v: want the pillar with %d HP and no fire ball", g.Balls, dmg)
	}
	g.Balls = append(g.Balls, Ball{K: ballFire, Owner: 1, X: 640, Y: 300, DX: -1, HP: 5, Left: 400})
	g.stepBalls(dt)
	if len(g.Balls) != 0 {
		t.Fatalf("balls %+v: the second fire ball should have broken the pillar and gone", g.Balls)
	}
}

func TestOwnProjectilesAndWaitingLeavesPassThroughItsPillars(t *testing.T) {
	cfg := vulpixConfig()
	g := duel(cfg, vulpix, charizard)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 100, 100, 1100, 100
	g.Balls = []Ball{
		{K: ballPillar, Owner: 0, X: 600, Y: 300, HP: 6, Life: 20},
		{K: ballFire, Owner: 0, X: 640, Y: 300, DX: -1, HP: 5, Left: 400},
	}
	g.stepBalls(dt)
	if len(g.Balls) != 2 || g.Balls[0].HP != 6 {
		t.Fatalf("balls %+v: Vulpix's own projectile must not touch its pillar", g.Balls)
	}
	g.Balls = []Ball{{K: ballPillar, Owner: 0, X: 600, Y: 300, HP: 6, Life: 20}, {K: ballLeaf, Owner: 1, X: 600, Y: 300, Wait: true, Delay: 5, HP: 1}}
	g.stepBalls(dt)
	if g.Balls[0].HP != 6 {
		t.Fatalf("a leaf still waiting hurt the pillar: %+v", g.Balls)
	}
}

func TestAnElectroBallThatTouchesAPillarIsSpentAndExplodesNextStep(t *testing.T) {
	cfg := vulpixConfig()
	g := duel(cfg, vulpix, pikachu)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 100, 100, 1100, 100
	g.Balls = []Ball{
		{K: ballPillar, Owner: 0, X: 600, Y: 300, HP: 25, Life: 20},
		{Owner: 1, X: 700, Y: 300, DX: -1, HP: cfg.Pikachu.ElectroBall.HP},
	}
	g.stepBalls(dt) // 400/30 = 13 per tick: not there yet
	for k := 0; k < 12 && g.Balls[len(g.Balls)-1].HP > 0; k++ {
		g.stepBalls(dt)
	}
	var ball *Ball
	for j := range g.Balls {
		if g.Balls[j].K == ballElectro {
			ball = &g.Balls[j]
		}
	}
	if ball == nil || ball.HP > 0 {
		t.Fatalf("balls %+v: the electro ball should be spent on the pillar", g.Balls)
	}
	ev := g.stepBalls(dt)
	if len(ev) == 0 || ev[0].T != "explode" {
		t.Fatalf("events %+v, want the explosion next step", ev)
	}
}

func TestPillarsAreWornDownByTheSameThingsThatHitABall(t *testing.T) {
	cfg := vulpixConfig()
	g := duel(cfg, vulpix, snorlax) // Snorlax's Hyper Beam sweeps the pillar standing in front of it
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 100, 600, 800, 250
	g.Balls = []Ball{{K: ballPillar, Owner: 0, X: 600, Y: 250, HP: 1000, Life: 30}}
	g.P[1].In.Beam = true
	steps(g, 2*tickRate)
	g.P[1].In = Input{}
	if g.Balls[0].HP >= 1000 {
		t.Fatalf("the beam did not hurt the pillar: HP %d", g.Balls[0].HP)
	}
	if g.Balls[0].X != 600 || g.Balls[0].Y != 250 {
		t.Fatalf("the beam pushed the pillar to %v,%v: pillars do not move", g.Balls[0].X, g.Balls[0].Y)
	}
}

// ---- Snapshot, config, mirror ----

func TestVulpixSnapshotCarriesItsState(t *testing.T) {
	g := vg(vulpixConfig())
	g.P[0].In = Input{Skill: true, Storm: true, Freeze: true}
	steps(g, 2)
	g.Balls = append(g.Balls, Ball{K: ballPillar, Owner: 0, X: 5, Y: 6, HP: 7, Life: 2.5})
	raw, _ := json.Marshal(g.P[0])
	for _, key := range []string{`"yt"`, `"ycd"`, `"wn"`, `"wx"`, `"wy"`, `"wcd"`, `"zt"`, `"zx"`, `"zy"`, `"zcd"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("player json lacks %s: %s", key, raw)
		}
	}
	b, _ := json.Marshal(g.Balls[len(g.Balls)-1])
	if !strings.Contains(string(b), `"lf":2.5`) || !strings.Contains(string(b), `"k":3`) {
		t.Fatalf("pillar json %s: want k 3 and lf 2.5", b)
	}
	f, _ := json.Marshal(Frost{Owner: 1, X: 2, Y: 3, Left: 4})
	if string(f) != `{"o":1,"x":2,"y":3,"l":4}` {
		t.Fatalf("frost json %s", f)
	}
}

func TestVulpixConfigLoadsAndRefusesBadValues(t *testing.T) {
	good := vulpixConfig()
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(c *Config){
		"vulpix.frost_field.slow":             func(c *Config) { c.Vulpix.FrostField.Slow = 1.5 },
		"vulpix.frost_field.slide_speed":      func(c *Config) { c.Vulpix.FrostField.SlideSpeed = 0 },
		"vulpix.frost_field.scale":            func(c *Config) { c.Vulpix.FrostField.Scale = 0 },
		"vulpix.frost_field.contact_damage":   func(c *Config) { c.Vulpix.FrostField.ContactDamage = -1 },
		"vulpix.frost_field.explosion_radius": func(c *Config) { c.Vulpix.FrostField.ExplosionRadius = -1 },
		"vulpix.frost_field.explosion_damage": func(c *Config) { c.Vulpix.FrostField.ExplosionDamage = -1 },
		"vulpix.frost_field.boost":            func(c *Config) { c.Vulpix.FrostField.Boost = -1 },
		"vulpix.ice_storm.freeze_hits":        func(c *Config) { c.Vulpix.IceStorm.FreezeHits = 0 }, // it would divide by zero
		"vulpix.ice_storm.strikes":            func(c *Config) { c.Vulpix.IceStorm.Strikes = 0 },
		"vulpix.freeze_beam.pillar_hp":        func(c *Config) { c.Vulpix.FreezeBeam.PillarHP = 0 },
		"vulpix.freeze_beam.length":           func(c *Config) { c.Vulpix.FreezeBeam.Length = 0 },
		"vulpix.freeze_beam.start_distance":   func(c *Config) { c.Vulpix.FreezeBeam.StartDistance = c.Vulpix.FreezeBeam.Length + 1 },
		"vulpix.freeze_beam":                  func(c *Config) { c.Vulpix.FreezeBeam.MinHeight, c.Vulpix.FreezeBeam.MaxHeight = 50, 10 },
		"vulpix.ice_storm":                    func(c *Config) { c.Vulpix.IceStorm.MinHeight = -1 },
	} {
		c := good
		mut(&c)
		if err := c.validate(); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: error %v, want one naming it", name, err)
		}
	}
}

func TestVulpixIsPickedUpByTheConfigAccessors(t *testing.T) {
	cfg := vulpixConfig()
	if cfg.maxHPOf(vulpix) != cfg.Vulpix.MaxHP || cfg.walkSpeedOf(vulpix) != cfg.Vulpix.WalkSpeed || cfg.radiusOf(vulpix) != cfg.Vulpix.Radius {
		t.Fatal("the accessors do not return Vulpix's values")
	}
}

func TestVulpixMirrorFrostIsEachOwnersBoostAndTheOtherOnesSlow(t *testing.T) {
	cfg := vulpixConfig()
	g := duel(cfg, vulpix, vulpix)
	g.Frost = []Frost{{Owner: 0, X: g.P[0].X, Y: g.P[0].Y, Left: 5}, {Owner: 1, X: g.P[0].X, Y: g.P[0].Y, Left: 5}}
	if f := g.frostFactor(0); math.Abs(f-1.5*0.5) > 1e-9 {
		t.Fatalf("slot 0 on both: %v", f)
	}
	if f := g.frostFactor(1); f != 1 {
		t.Fatalf("slot 1 stands on neither: %v", f)
	}
}
