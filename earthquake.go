package main

import "math"

// maxQuakes is the most earthquakes shaking at once (the oldest stops when one more starts).
const maxQuakes = 8

// quakeLinger is how long a foe stays slowed after it has left an earthquake (or in the moment between two ticks of it).
const quakeLinger = 0.3

// quake is the ground Snorlax's landing left shaking: where, how much longer, and the seconds to its next shock.
type quake struct {
	Owner int
	X, Y  float64
	Left  float64
	Next  float64
}

// tickQuakes runs the earthquakes: every foe on the ground in one is slowed all the while, and on every shock (`earthquake_interval`) hurt. They go on whatever becomes of whoever
// landed them, and are cleared with the round.
func (g *Game) tickQuakes(dt float64) {
	if len(g.Quakes) == 0 {
		return
	}
	jp := g.cfg.Snorlax.Jump
	kept := g.Quakes[:0]
	for _, q := range g.Quakes {
		shock := false
		if q.Next -= dt; q.Next <= 1e-9 {
			shock, q.Next = true, q.Next+jp.EarthquakeInterval
		}
		for _, j := range g.foes(q.Owner) {
			foe := &g.P[j]
			if !jp.Reach.hits(foe) || math.Hypot(foe.X-q.X, foe.Y-q.Y) > jp.EarthquakeRadius+g.radius(j) {
				continue
			}
			if jp.EarthquakeSlow > 0 {
				g.slow(foe, quakeLinger, jp.EarthquakeSlow)
			}
			if shock {
				g.hit(foe, jp.EarthquakeDamage)
			}
		}
		if q.Left -= dt; q.Left > 1e-9 {
			kept = append(kept, q)
		}
	}
	g.Quakes = kept
}
