package main

import "math"

// burstCast is one Burst Punch that has come out: where its fist is, the seconds since, how many of its explosions have gone off, and how many times each foe has been caught by them
// (the more it was, the higher the next one throws it).
type burstCast struct {
	Owner int
	X, Y  float64
	T     float64
	N     int
	Hits  [maxSlots]int
}

// burstPunch brings the fist out `distance` ahead of the player along the direction it was aimed: the explosions go off round that point from now on (stepBursts).
func (g *Game) burstPunch(i int) []Event {
	p, bp := &g.P[i], g.cfg.Machamp.Gigantamax.BurstPunch
	x := math.Min(math.Max(p.X+p.BpX*bp.Distance, 0), g.court.Length)
	y := math.Min(math.Max(p.Y+p.BpY*bp.Distance, 0), g.court.Width)
	g.Bursts = append(g.Bursts, burstCast{Owner: i, X: x, Y: y})
	return []Event{{T: "bpunch", S: i, X: x, Y: y, R: bp.Radius, DX: p.BpX, DY: p.BpY, L: bp.Duration}}
}

// stepBursts lets off the explosions that are due: `count` of them spread evenly over `duration` seconds (the first the moment the fist comes out), each at a random place in the
// cylinder round the fist. A cast goes on to its end whatever becomes of whoever threw it.
func (g *Game) stepBursts(dt float64) []Event {
	if len(g.Bursts) == 0 {
		return nil
	}
	defer g.ultimateDamage()() // only the Gigantamax Machamp has it, and all it does then is its ultimate's
	var events []Event
	bp := g.cfg.Machamp.Gigantamax.BurstPunch
	kept := g.Bursts[:0]
	for _, c := range g.Bursts {
		due := bp.Count
		if bp.Duration > 0 {
			due = min(bp.Count, int(c.T/bp.Duration*float64(bp.Count))+1)
		}
		for ; c.N < due; c.N++ {
			events = append(events, g.explode(&c)...)
		}
		if c.T += dt; c.N < bp.Count {
			kept = append(kept, c)
		}
	}
	g.Bursts = kept
	return events
}

// explode lets off one of a cast's explosions at a random point of its cylinder: every foe it touches (the sphere of `explosion_radius`; a body under the ground is out of reach)
// takes the damage and is thrown up, higher for each time it has been caught in this cast. (c.N is the number of this one.)
func (g *Game) explode(c *burstCast) []Event {
	bp := g.cfg.Machamp.Gigantamax.BurstPunch
	a, r := g.rng.Float64()*2*math.Pi, bp.Radius*math.Sqrt(g.rng.Float64())
	// the cylinder is filled from the ground up: the n-th of `count` explosions is at a random height of the n-th slice of it
	x, y, z := c.X+math.Cos(a)*r, c.Y+math.Sin(a)*r, (float64(c.N)+g.rng.Float64())/float64(bp.Count)*bp.Height
	for _, j := range g.foes(c.Owner) {
		foe := &g.P[j]
		if foe.Z < 0 || math.Hypot(foe.X-x, foe.Y-y) > bp.ExplosionRadius+g.radius(j) || math.Abs(foe.Z-z) > bp.ExplosionRadius {
			continue
		}
		g.hit(foe, bp.ExplosionDamage)
		c.Hits[j]++
		g.relaunch(foe, math.Min(bp.ExplosionHeight+float64(c.Hits[j]-1)*bp.HeightGain, bp.LaunchCap), bp.LaunchDuration)
	}
	return []Event{{T: "bpboom", S: c.Owner, X: x, Y: y, Z: z, R: bp.ExplosionRadius}}
}

// relaunch throws f up to `height` and holds it there `duration` seconds: one that is up already (or coming down) is thrown on from where it is, higher if this is higher, and its
// time in the air starts again. A body something else holds (a tornado) is left to it.
func (g *Game) relaunch(f *Player, height, duration float64) {
	if !f.Juggle {
		g.launch(f, height, duration)
		return
	}
	f.JuggleH, f.JuggleT = math.Max(f.JuggleH, height), duration
}
