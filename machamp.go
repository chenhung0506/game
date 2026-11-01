package main

import "math"

// Tree slam phases.
const (
	treeNone  = iota
	treeJump  // leaping along the direction chosen
	treeSmash // landed, holding the tree up overhead with its shape on show
	treeSwing // the tree coming down like an axe; it hits when the swing ends
)

// dropMachamp ends whatever Machamp was doing (it was frozen, stunned or caught).
func (g *Game) dropMachamp(i int) {
	p := &g.P[i]
	if (p.TreePhase == treeJump || p.RockT > 0) && !p.Held {
		p.Z = 0
	}
	p.TreePhase, p.TreeT, p.CometT, p.CometN, p.RockT = treeNone, 0, 0, 0, 0
}

// machampAirSpeed is how fast Machamp is steered in the Rock Tomb jump: the first speed until the middle, the second after.
func (g *Game) machampAirSpeed(p *Player) float64 {
	rt := g.cfg.Machamp.RockTomb
	if rt.Duration-p.RockT < rt.Duration/2 {
		return rt.AirSpeedFirstHalf
	}
	return rt.AirSpeedSecondHalf
}

// stepMachamp runs Machamp's three skills and reports whether it is rooted this tick (the walking code stays out of it:
// the tree jump and the flurry move it themselves, the smash holds it still; the Rock Tomb jump is steered like a walk).
func (g *Game) stepMachamp(i int, in Input, dt float64) ([]Event, bool) {
	var events []Event
	p, other := &g.P[i], &g.P[1-i]
	ts, cp, rt := g.cfg.Machamp.TreeSlam, g.cfg.Machamp.CometPunch, g.cfg.Machamp.RockTomb
	for _, cd := range []*float64{&p.TreeCD, &p.CometCD, &p.RockCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}
	idle := p.TreePhase == treeNone && p.CometT == 0 && p.RockT == 0 && p.Z == 0

	// ---- Tree slam: jump, then pull up a tree and smash it down ----
	if in.Skill && idle && p.TreeCD == 0 {
		dx, dy := float64(in.DX), float64(in.DY)
		if dx == 0 && dy == 0 {
			dx, dy = float64(p.FX), float64(p.FY)
		}
		p.TreeX, p.TreeY = unit(dx, dy)
		p.TreePhase, p.TreeT, p.TreeCD = treeJump, ts.JumpDuration, ts.Cooldown
		idle = false
	}
	switch p.TreePhase {
	case treeJump:
		step := math.Min(dt, p.TreeT) // the last step is clipped so the distance is exact
		v := ts.JumpDistance / ts.JumpDuration
		p.X = math.Min(math.Max(p.X+p.TreeX*v*step, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+p.TreeY*v*step, 0), g.court.Width)
		if p.TreeT -= step; p.TreeT < 1e-9 {
			p.Z, p.TreePhase, p.TreeT = 0, treeSmash, ts.ChargeDuration
			if n := math.Hypot(other.X-p.X, other.Y-p.Y); n > 1e-9 { // the tree goes down where the opponent stands, not the way it leapt
				p.TreeX, p.TreeY = (other.X-p.X)/n, (other.Y-p.Y)/n
			}
			events = append(events, g.advanceTree(i)...) // a phase of zero seconds is skipped
		} else {
			u := 1 - p.TreeT/ts.JumpDuration
			p.Z = 4 * ts.JumpHeight * u * (1 - u)
		}
	case treeSmash, treeSwing:
		if p.TreeT -= dt; p.TreeT < 1e-9 {
			p.TreeT = 0
			events = append(events, g.advanceTree(i)...)
		}
	}
	rooted := p.TreePhase != treeNone || len(events) > 0 // through the tick of the smash, free from the next

	// ---- Comet Punch: run at the opponent, throwing fists ahead ----
	if in.Comet && idle && p.CometCD == 0 {
		p.CometT, p.CometN, p.CometCD = cp.Duration, 0, cp.Cooldown
		idle = false
	}
	if p.CometT > 0 {
		rooted = true
		fx, fy := unit(other.X-p.X, other.Y-p.Y)
		if math.Hypot(other.X-p.X, other.Y-p.Y) < 1e-9 {
			fx, fy = unit(float64(p.FX), float64(p.FY))
		}
		step := math.Min(dt, p.CometT)
		adv := math.Min(cp.Distance/cp.Duration*step, math.Max(0, dist(p, other)-g.radius(i)-g.radius(1-i)))
		p.X = math.Min(math.Max(p.X+fx*adv, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+fy*adv, 0), g.court.Width)
		if p.CometT -= step; p.CometT < 1e-9 {
			p.CometT = 0
		}
		due := cp.Quantity // by the end every fist has been thrown
		if p.CometT > 0 {
			due = int(float64(cp.Quantity)*(cp.Duration-p.CometT)/cp.Duration + 1e-9)
		}
		for ; p.CometN < due; p.CometN++ {
			a := (g.rng.Float64()*2 - 1) * cp.RandomOffsetAngle * math.Pi / 180
			ca, sa := math.Cos(a), math.Sin(a)
			dx, dy := fx*ca-fy*sa, fx*sa+fy*ca
			d := g.radius(i) + cp.ScatterMinDistance + g.rng.Float64()*(cp.ScatterMaxDistance-cp.ScatterMinDistance)
			g.Balls = append(g.Balls, Ball{
				K: ballFist, Owner: i, DX: dx, DY: dy, HP: cp.HP, Life: cp.FistDuration,
				X: math.Min(math.Max(p.X+dx*d, 0), g.court.Length), Y: math.Min(math.Max(p.Y+dy*d, 0), g.court.Width),
			})
		}
	}

	// ---- Rock Tomb: a jump (steered like a walk), and the landing raises a ring of stones ----
	if in.Rock && idle && p.RockCD == 0 {
		p.RockT, p.RockCD = rt.Duration, rt.Cooldown
	}
	if p.RockT > 0 {
		if p.RockT -= dt; p.RockT < 1e-9 {
			p.RockT, p.Z = 0, 0
			events = append(events, Event{T: "slam", X: p.X, Y: p.Y, R: rt.RingRadius})
			for n := 0; n < rt.Count; n++ {
				a := 2 * math.Pi * float64(n) / float64(rt.Count)
				x, y := p.X+math.Cos(a)*rt.RingRadius, p.Y+math.Sin(a)*rt.RingRadius
				if x < 0 || x > g.court.Length || y < 0 || y > g.court.Width {
					continue // out of the court
				}
				g.Balls = append(g.Balls, Ball{K: ballStone, Owner: i, X: x, Y: y, HP: rt.StoneHP, Life: rt.StoneDuration})
			}
		} else {
			u := 1 - p.RockT/rt.Duration
			p.Z = 4 * rt.Height * u * (1 - u)
		}
	}
	return events, rooted
}

// advanceTree moves the tree on from a phase whose time is up (and past any that last no time): held up, then swung
// down, then the hit.
func (g *Game) advanceTree(i int) []Event {
	var events []Event
	p, ts := &g.P[i], g.cfg.Machamp.TreeSlam
	for p.TreePhase != treeNone && p.TreeT == 0 {
		switch p.TreePhase {
		case treeSmash:
			p.TreePhase, p.TreeT = treeSwing, ts.SwingDuration
		case treeSwing:
			p.TreePhase = treeNone
			events = append(events, g.smashTree(i)...)
		}
	}
	return events
}

// smashTree brings the tree down toward where the opponent stood as it landed and hurts the opponent it lands on, once.
func (g *Game) smashTree(i int) []Event {
	p, other, ts := &g.P[i], &g.P[1-i], g.cfg.Machamp.TreeSlam
	if ts.Reach.hits(other) && treeHits(p.X, p.Y, p.TreeX, p.TreeY, other.X, other.Y, g.radius(1-i), ts.TrunkLength, ts.TrunkWidth, ts.LeafRadius, ts.LeafAngle) {
		g.hit(other, ts.Damage)
	}
	return []Event{{T: "tree", X: p.X, Y: p.Y, R: ts.TrunkLength, DX: p.TreeX, DY: p.TreeY}}
}

// treeHits is whether a body of radius r at (bx, by) touches a tree smashed down from (x, y) along the unit vector
// (ux, uy): the trunk, a rectangle `length` long and `width` wide from (x, y), or the canopy, the sector of radius
// `leaf` and `angle` degrees (centred on the same way) round the trunk's tip.
func treeHits(x, y, ux, uy, bx, by, r, length, width, leaf, angle float64) bool {
	t, s := (bx-x)*ux+(by-y)*uy, -(bx-x)*uy+(by-y)*ux // along the trunk, and across it
	if math.Hypot(math.Max(0, math.Max(-t, t-length)), math.Max(0, math.Abs(s)-width/2)) <= r {
		return true
	}
	d := math.Hypot(t-length, s)
	if d > leaf+r {
		return false
	}
	return d <= r || math.Abs(math.Atan2(s, t-length)) <= angle/2*math.Pi/180
}
