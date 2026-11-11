package main

import "math"

// Tree slam phases.
const (
	treeNone  = iota
	treeJump  // leaping along the direction chosen
	treeSmash // landed, holding the tree up overhead with its shape on show
	treeSwing // the tree coming down like an axe; it hits when the swing ends
)

// High Jump Kick phases.
const (
	kickNone  = iota
	kickSlide // sliding along the stick
	kickFly   // leaping at the opponent
	kickWait  // standing still between the slide and the leap (kick_delay); the leap is aimed when it ends
)

// pinned is whether a vine or a paralysis holds the player on the spot: a skill it is in the middle of goes on (its timers
// run, its fist or kick still comes), but it does not move.
func (p *Player) pinned() bool { return p.BoundT > 0 || p.ParalyzeT > 0 }

// isBrawler is whether kind is one of the two that fight with the tree, the fists and the rocks.
func isBrawler(kind int) bool { return kind == ash || kind == machamp }

// brawler is the numbers slot i plays by: Machamp's own, else Ash's (a stone or a fist is looked up by its owner).
func (g *Game) brawler(i int) *BrawlerCfg {
	if g.P[i].Kind == machamp {
		return &g.cfg.Machamp
	}
	return &g.cfg.Ash
}

// dropMachamp ends whatever Machamp was doing (it was frozen, stunned or caught).
func (g *Game) dropMachamp(i int) {
	p := &g.P[i]
	if (p.TreePhase == treeJump || p.RockT > 0 || p.MegaJT > 0 || p.KickPhase == kickFly) && !p.Held {
		p.Z = 0
	}
	p.TreePhase, p.TreeT, p.CometT, p.CometN, p.RockT, p.MegaT = treeNone, 0, 0, 0, 0, 0
	p.MegaJT = 0
	p.KickPhase, p.KickT = kickNone, 0
}

// machampAirSpeed is how fast Machamp is steered in the Rock Tomb jump: the first speed until the middle, the second after.
func (g *Game) machampAirSpeed(i int) float64 {
	p := &g.P[i]
	rt := g.brawler(i).RockTomb
	if rt.Duration-p.RockT < rt.Duration/2 {
		return rt.AirSpeedFirstHalf
	}
	return rt.AirSpeedSecondHalf
}

// stepMachamp runs Machamp's three skills and reports whether it is rooted this tick (the walking code stays out of it:
// the tree jump and the flurry move it themselves, the smash holds it still; the Rock Tomb jump is steered like a walk).
func (g *Game) stepMachamp(i int, in Input, dt float64) ([]Event, bool) {
	var events []Event
	oi := g.nearestFoe(i) // what it runs at, aims at and closes in on is the nearest foe
	p, other := &g.P[i], &g.P[oi]
	ts, cp, rt := g.brawler(i).TreeSlam, g.brawler(i).CometPunch, g.brawler(i).RockTomb
	mp, hk := g.brawler(i).MegatonPunch, g.brawler(i).HighJumpKick
	for _, cd := range []*float64{&p.TreeCD, &p.CometCD, &p.RockCD, &p.MegaCD, &p.KickCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}
	idle := p.TreePhase == treeNone && p.CometT == 0 && p.RockT == 0 && p.MegaT == 0 && p.MegaJT == 0 && p.KickPhase == kickNone && p.Z == 0

	// ---- Tree slam: jump, then pull up a tree and smash it down ----
	if in.Skill && idle && p.TreeCD == 0 && p.Kind != machamp { // Machamp kicks instead
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

	// ---- High Jump Kick (Machamp's Skill; Ash's Kick, for the computer): slide along the stick, then leap at the opponent ----
	if ((in.Skill && p.Kind == machamp) || (in.Kick && p.Kind == ash)) && idle && p.KickCD == 0 {
		dx, dy := float64(in.DX), float64(in.DY)
		if dx == 0 && dy == 0 {
			dx, dy = float64(p.FX), float64(p.FY)
		}
		p.KickX, p.KickY = unit(dx, dy)
		p.KickPhase, p.KickT, p.KickCD = kickSlide, hk.SlideDuration, hk.Cooldown
		if hk.SlideDuration == 0 || hk.SlideDistance == 0 {
			p.KickT = 0
		}
		idle = false
	}
	flying := p.KickPhase == kickFly // the kick leaves on the tick after the slide ends
	if p.KickPhase == kickWait {
		if p.KickT -= dt; p.KickT < 1e-9 {
			p.KickT, p.KickPhase, p.KickD = 0, kickFly, 0
			if ux, uy := unit(other.X-p.X, other.Y-p.Y); ux != 0 || uy != 0 {
				p.KickX, p.KickY = ux, uy // the kick goes at where the opponent stands as the wait ends
			}
		}
	}
	if p.KickPhase == kickSlide {
		step := math.Min(dt, p.KickT) // the last step is clipped so the distance is exact
		if p.KickT > 0 && !p.pinned() {
			v := hk.SlideDistance / hk.SlideDuration
			p.X = math.Min(math.Max(p.X+p.KickX*v*step, 0), g.court.Length)
			p.Y = math.Min(math.Max(p.Y+p.KickY*v*step, 0), g.court.Width)
		}
		if p.KickT -= step; p.KickT < 1e-9 {
			if hk.KickDelay > 0 {
				p.KickT, p.KickPhase = hk.KickDelay, kickWait
			} else {
				p.KickT, p.KickPhase, p.KickD = 0, kickFly, 0
				if ux, uy := unit(other.X-p.X, other.Y-p.Y); ux != 0 || uy != 0 {
					p.KickX, p.KickY = ux, uy // the kick goes at where the opponent stands as the slide ends
				}
			}
		}
	}
	if flying {
		events = append(events, g.kickStep(i, dt)...)
	}
	if p.KickPhase != kickNone {
		rooted = true
	}

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
		adv := math.Min(cp.Distance/cp.Duration*step, math.Max(0, dist(p, other)-g.radius(i)-g.radius(oi)))
		if p.BoundT > 0 || p.ParalyzeT > 0 { // pinned by a vine or paralyzed: the fists are thrown from where it stands
			adv = 0
		}
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
				K: ballFist, Owner: i, Side: i + 1, DX: dx, DY: dy, HP: cp.HP, Life: cp.FistDuration,
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
				g.Balls = append(g.Balls, Ball{K: ballStone, Owner: i, Side: i + 1, X: x, Y: y, HP: rt.StoneHP, Life: rt.StoneDuration})
			}
		} else {
			u := 1 - p.RockT/rt.Duration
			p.Z = 4 * rt.Height * u * (1 - u)
		}
	}

	// ---- Megaton Punch (Ash only): leap along the stick, then stand and gather, then one huge fist ----
	if in.Mega && p.Kind == ash && idle && p.MegaCD == 0 {
		p.MegaCD = mp.Cooldown
		if mp.JumpDuration > 0 {
			p.MegaJX, p.MegaJY = unit(float64(in.DX), float64(in.DY)) // no stick: it leaps on the spot
			p.MegaJT = mp.JumpDuration
		} else {
			events = append(events, g.megatonGather(i)...)
		}
	}
	if p.MegaJT > 0 {
		step := math.Min(dt, p.MegaJT) // the last step is clipped so the distance is exact
		v := mp.JumpDistance / mp.JumpDuration
		if !p.pinned() {
			p.X = math.Min(math.Max(p.X+p.MegaJX*v*step, 0), g.court.Length)
			p.Y = math.Min(math.Max(p.Y+p.MegaJY*v*step, 0), g.court.Width)
		}
		if p.MegaJT -= step; p.MegaJT < 1e-9 {
			p.MegaJT, p.Z = 0, 0
			events = append(events, g.megatonGather(i)...) // the fist is aimed from where it landed
		} else {
			u := 1 - p.MegaJT/mp.JumpDuration
			p.Z = 4 * mp.JumpHeight * u * (1 - u)
		}
		rooted = true
	}
	if p.MegaT > 0 {
		rooted = true
		// the stick turns the fist while it gathers, to any angle the pad gives; with no stick it follows the opponent
		ax, ay := 0.0, 0.0
		if n := math.Hypot(in.AimX, in.AimY); n > 1e-9 && !math.IsInf(n, 0) {
			ax, ay = in.AimX/n, in.AimY/n
		} else {
			ax, ay = unit(float64(in.DX), float64(in.DY))
		}
		if ax == 0 && ay == 0 {
			ax, ay = unit(other.X-p.X, other.Y-p.Y)
		}
		if ax != 0 || ay != 0 {
			p.MegaX, p.MegaY = ax, ay
		}
		if p.MegaT -= dt; p.MegaT < 1e-9 {
			p.MegaT = 0
			events = append(events, g.megatonPunch(i)...)
		}
	}
	return events, rooted
}

// kickStep flies the kick on one tick along the direction it was aimed: it stops at the first foe's body it meets, which it
// hurts, stuns and kicks away, or at the end of its distance.
func (g *Game) kickStep(i int, dt float64) []Event {
	p, hk := &g.P[i], g.brawler(i).HighJumpKick
	s := math.Min(hk.KickSpeed*dt, hk.KickDistance-p.KickD)
	if p.pinned() { // held on the spot: the leap runs its course but gets nowhere, and so touches nobody
		p.KickD += s
		if p.KickD >= hk.KickDistance-1e-9 {
			p.KickPhase, p.Z = kickNone, 0
		} else {
			u := p.KickD / hk.KickDistance
			p.Z = 4 * hk.KickHeight * u * (1 - u)
		}
		return nil
	}
	// The first moment, along the path, that the bodies (and the leg's `radius`) touch: where the circle round a foe
	// of that size meets the line. Of the foes in the way, the nearest along the path is the one it meets.
	var other *Player
	for _, j := range g.foes(i) {
		foe := &g.P[j]
		R := g.radius(i) + g.radius(j) + hk.Radius
		rx, ry := foe.X-p.X, foe.Y-p.Y
		along, side := rx*p.KickX+ry*p.KickY, math.Abs(rx*p.KickY-ry*p.KickX)
		if hk.Reach.hits(foe) && along > 0 && side <= R {
			if c := along - math.Sqrt(R*R-side*side); c <= s {
				s, other = math.Max(c, 0), foe
			}
		}
	}
	hit := other != nil
	p.X = math.Min(math.Max(p.X+p.KickX*s, 0), g.court.Length)
	p.Y = math.Min(math.Max(p.Y+p.KickY*s, 0), g.court.Width)
	p.KickD += s
	if hit || p.KickD >= hk.KickDistance-1e-9 {
		p.KickPhase, p.Z = kickNone, 0
	} else {
		u := p.KickD / hk.KickDistance
		p.Z = 4 * hk.KickHeight * u * (1 - u)
	}
	if !hit {
		return nil
	}
	g.hit(other, hk.Damage)
	other.StunT = math.Max(other.StunT, hk.StunDuration)
	g.knockBack(p.X, p.Y, other, hk.Knockback, p.KickX, p.KickY)
	return []Event{{T: "kickhit", X: other.X, Y: other.Y, DX: p.KickX, DY: p.KickY}}
}

// megatonGather locks the direction of the punch on the opponent and starts the charge (a charge of zero throws it at once).
func (g *Game) megatonGather(i int) []Event {
	p, other, mp := &g.P[i], &g.P[g.nearestFoe(i)], g.brawler(i).MegatonPunch
	p.MegaX, p.MegaY = unit(other.X-p.X, other.Y-p.Y)
	if p.MegaX == 0 && p.MegaY == 0 {
		p.MegaX, p.MegaY = unit(float64(p.FX), float64(p.FY))
	}
	if p.MegaT = mp.RingDuration + mp.BallDuration; p.MegaT == 0 {
		return g.megatonPunch(i)
	}
	return nil
}

// megatonPunch throws the fist along the direction locked when the charge began: it hurts, stuns and knocks back the
// opponent it touches, once.
func (g *Game) megatonPunch(i int) []Event {
	p, mp := &g.P[i], g.brawler(i).MegatonPunch
	ev := Event{T: "mega", S: i, X: p.X, Y: p.Y, R: mp.Distance, DX: p.MegaX, DY: p.MegaY}
	var hits []int // every foe the fist touches
	for _, j := range g.foes(i) {
		if foe := &g.P[j]; mp.Reach.hits(foe) && segDist(foe.X, foe.Y, p.X, p.Y, p.X+p.MegaX*mp.Distance, p.Y+p.MegaY*mp.Distance) <= mp.Radius+g.radius(j) {
			hits = append(hits, j)
		}
	}
	// Ash is carried forward with the fist, and stops against the nearest body that is in the way.
	adv := math.Min(mp.LungeDistance, mp.Distance)
	if p.pinned() {
		adv = 0 // held on the spot: the fist flies from where it stands
	} else {
		for _, j := range hits {
			adv = math.Min(adv, math.Max(0, dist(p, &g.P[j])-g.radius(i)-g.radius(j)))
		}
	}
	p.X = math.Min(math.Max(p.X+p.MegaX*adv, 0), g.court.Length)
	p.Y = math.Min(math.Max(p.Y+p.MegaY*adv, 0), g.court.Width)
	ev.L = adv
	events := []Event{ev}
	for _, j := range hits {
		other := &g.P[j]
		g.hit(other, mp.Damage)
		other.StunT = math.Max(other.StunT, mp.StunDuration)
		g.knockBack(p.X, p.Y, other, mp.Knockback, p.MegaX, p.MegaY)
		events = append(events, Event{T: "megahit", X: other.X, Y: other.Y, DX: p.MegaX, DY: p.MegaY})
	}
	return events
}

// advanceTree moves the tree on from a phase whose time is up (and past any that last no time): held up, then swung
// down, then the hit.
func (g *Game) advanceTree(i int) []Event {
	var events []Event
	p, ts := &g.P[i], g.brawler(i).TreeSlam
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
	p, ts := &g.P[i], g.brawler(i).TreeSlam
	for _, j := range g.foes(i) {
		if other := &g.P[j]; ts.Reach.hits(other) && treeHits(p.X, p.Y, p.TreeX, p.TreeY, other.X, other.Y, g.radius(j), ts.TrunkLength, ts.TrunkWidth, ts.LeafRadius, ts.LeafAngle) {
			g.hit(other, ts.Damage)
			other.StunT = math.Max(other.StunT, ts.StunDuration)
		}
	}
	return []Event{{T: "tree", X: p.X, Y: p.Y, R: ts.TrunkLength, DX: p.TreeX, DY: p.TreeY, K: p.Kind}}
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
