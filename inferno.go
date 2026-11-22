package main

import "math"

// The Gigantamax Charizard's Inferno (the main button, in place of Fly; see InfernoCfg): Charizard stands rooted for `summon_duration` seconds while a bird of fire grows in the sky above it
// (a Ball of kind ballFirebird in phase 0, rising to `sky_height`), then the bird dives at the nearest foe the way Rowlet's Brave Bird does (stepBraveBird: it aims at where the foe stands until it is
// `lock_distance` from it, then the way is fixed) and bursts where it lands: foes in the burst take the damage and are thrown up (launch, stepJuggle), and the ground is left burning (a FireZone
// with Inf, which tickFires burns by the Inferno's own numbers). Everything it does is the ultimate's (g.ultNow). The bird has no HP, so no projectile meets it, and it goes on if Charizard changes back.

// Phases of a bird of fire (Ball.Phase).
const (
	birdCalled = 0 // growing in the sky above Charizard, which is rooted (Player.InfT)
	birdDiving = 1
)

// startInferno begins the calling: Charizard is rooted for the summon time and the bird starts to grow above it. The cooldown (the same one Fly has: it is the same button) starts now.
func (g *Game) startInferno(i int, in InfernoCfg) {
	p := &g.P[i]
	p.InfT, p.FlyCD, p.BreathT = in.SummonDuration, in.Cooldown, 0
	b := Ball{K: ballFirebird, Owner: i, X: p.X, Y: p.Y, Z: p.Z, Life: in.SummonDuration, Phase: birdCalled, Ult: true, HP: in.HP}
	b.DX, b.DY = float64(p.FX), float64(p.FY)
	g.Balls = append(g.Balls, b)
}

// dropInferno ends the calling of a Charizard that was frozen, stunned or caught: the bird it was still calling is gone.
func (g *Game) dropInferno(i int) {
	p := &g.P[i]
	if p.InfT == 0 {
		return
	}
	p.InfT = 0
	kept := g.Balls[:0]
	for _, b := range g.Balls {
		if b.K == ballFirebird && b.Owner == i && b.Phase == birdCalled {
			continue
		}
		kept = append(kept, b)
	}
	g.Balls = kept
}

// stepFirebird moves one bird of fire a tick and says whether it is still there.
func (g *Game) stepFirebird(b *Ball, dt float64) ([]Event, bool) {
	in := g.cfg.Charizard.Gigantamax.Inferno
	owner := &g.P[b.Owner]
	if b.Phase == birdCalled { // it grows above Charizard, which is rooted and calling it, and nothing can hurt it yet
		b.HP = in.HP
		b.X, b.Y = owner.X, owner.Y
		if b.Life -= dt; b.Life > 1e-9 && in.SummonDuration > 0 {
			b.Z = in.SkyHeight * (1 - b.Life/in.SummonDuration)
			return nil, true
		}
		b.Life, b.Phase, b.Z = 0, birdDiving, in.SkyHeight
		foe := &g.P[g.nearestFoe(b.Owner)]
		b.AnchorX, b.AnchorY = foe.X, foe.Y
		return nil, true
	}
	if b.HP <= 0 { // shot down on the way: it is gone, and nothing bursts
		return []Event{{T: "infpop", X: b.X, Y: b.Y, R: in.Radius, K: charizard}}, false
	}
	total := math.Hypot(math.Hypot(b.AnchorX-b.X, b.AnchorY-b.Y), b.Z)
	if total > in.LockDistance { // it follows the foe until it is that close
		foe := &g.P[g.nearestFoe(b.Owner)]
		b.AnchorX, b.AnchorY = foe.X, foe.Y
		total = math.Hypot(math.Hypot(b.AnchorX-b.X, b.AnchorY-b.Y), b.Z)
	}
	if dx, dy := b.AnchorX-b.X, b.AnchorY-b.Y; math.Hypot(dx, dy) > 1e-6 {
		n := math.Hypot(dx, dy)
		b.DX, b.DY = dx/n, dy/n // for the page: which way it faces
	}
	if step := in.DiveSpeed * dt; step < total-1e-9 {
		f := step / total
		b.X += (b.AnchorX - b.X) * f
		b.Y += (b.AnchorY - b.Y) * f
		b.Z -= b.Z * f
		return nil, true
	}
	// down: it bursts
	b.X, b.Y, b.Z = b.AnchorX, b.AnchorY, 0
	defer g.ultimateDamage()()
	for _, j := range g.foes(b.Owner) {
		f := &g.P[j]
		if !in.Reach.hits(f) || math.Hypot(f.X-b.X, f.Y-b.Y) > in.ExplosionRadius+g.radius(j) {
			continue
		}
		g.hit(f, in.Damage)
		if !f.Held {
			g.launch(f, in.LaunchHeight, in.LaunchDuration)
		}
	}
	if in.FireRadius > 0 && in.FireDuration > 0 {
		g.Fires = append(g.Fires, FireZone{Owner: b.Owner, X: b.X, Y: b.Y, R: in.FireRadius, Left: in.FireDuration, Ult: true, Inf: true})
		if over := len(g.Fires) - maxFires; over > 0 {
			g.Fires = g.Fires[over:]
		}
	}
	return []Event{{T: "infboom", X: b.X, Y: b.Y, R: in.ExplosionRadius, K: charizard}}, false
}
