package main

import "math"

// The Gigantamax Gengar's skills (see GengarGmaxCfg): while it lasts the main button is the Lick, K the Devour and J the Ghost Fire breathed forward. Everything it does then is
// its ultimate's (stepGengar says so).

// Lick phases: gathering, the tongue going out, the foe being dragged in, the tongue coming back empty, the bite, and the rest after.
const (
	lickNone = iota
	lickCharge
	lickOut
	lickPull
	lickBack
	lickBite
	lickRest
)

// ghostFireOf is the numbers a Ghost Fire flame runs on: Gengar's own, or the Gigantamax Gengar's (a flame it breathed keeps them after it has changed back).
func (g *Game) ghostFireOf(ult bool) GhostFireCfg {
	if ult {
		return g.cfg.Gengar.Gigantamax.GhostFire.GhostFireCfg
	}
	return g.cfg.Gengar.GhostFire
}

// lickPrey is the foe the tongue holds, if it holds one.
func (g *Game) lickPrey(p *Player) *Player {
	if p.LickPrey == 0 {
		return nil
	}
	return &g.P[p.LickPrey-1]
}

// dropLick ends a lick (Gengar was stunned, frozen or caught, or its Gigantamax ran out): the tongue is gone and the foe is let go, to wake from its stun in its own time.
func (g *Game) dropLick(p *Player) {
	p.LickPhase, p.LickT, p.LickD, p.LickPrey = lickNone, 0, 0, 0
}

// endGengarGmax is what is left of the Gigantamax Gengar's skills when it is over: the tongue and the devouring both stop.
func (g *Game) endGengarGmax(p *Player) {
	g.dropLick(p)
	p.DevourT, p.DevourSilT = 0, 0
}

// stepGengarGmax runs the Gigantamax Gengar's three skills. It says whether Gengar is rooted (a lick, from gathering to the end of its rest).
func (g *Game) stepGengarGmax(i int, in Input, dt float64) ([]Event, bool) {
	p, other := &g.P[i], &g.P[g.nearestFoe(i)]
	gg := g.cfg.Gengar.Gigantamax
	var events []Event
	g.stepDevour(i, dt) // the field it left goes on whatever Gengar does
	if p.LickPhase != lickNone {
		return g.stepLick(i, dt)
	}
	if p.Z != 0 { // lifted by something else
		return nil, false
	}
	// ---- Lick: gather, aimed at the foe all the while ----
	if in.Skill && p.LickCD == 0 {
		lk := gg.Lick
		p.LickCD, p.LickD, p.LickPrey = lk.Cooldown, 0, 0
		p.LickX, p.LickY = g.lickAim(p, other)
		p.LickPhase, p.LickT = lickCharge, lk.ChargeDuration
		return g.stepLick(i, 0) // a gathering of no time sends the tongue out at once
	}
	// ---- Devour ----
	if in.SBall && p.DevourCD == 0 {
		dv := gg.Devour
		p.DevourCD, p.DevourT, p.DevourSilT = dv.Cooldown, dv.GravityDuration, dv.SilenceDuration
		events = append(events, Event{T: "devour", X: p.X, Y: p.Y, R: dv.GravityRadius, S: i, L: dv.SilenceRadius, I: math.Max(dv.GravityDuration, dv.SilenceDuration)})
	}
	// ---- Ghost Fire, breathed forward: the flames fan out over `ghost_fire_angle` degrees, centred on the foe ----
	if in.GFire && p.GFireCD == 0 {
		gf := gg.GhostFire
		p.GFireCD = gf.Cooldown
		cx, cy := g.lickAim(p, other)
		half := gf.Angle * math.Pi / 360 // half of the fan, in radians
		off := g.radius(i) + gf.Radius
		for n := 0; n < gf.Count && n < maxGhostFires; n++ {
			a := 0.0
			if gf.Count > 1 {
				a = -half + 2*half*float64(n)/float64(gf.Count-1)
			}
			ux, uy := cx*math.Cos(a)-cy*math.Sin(a), cx*math.Sin(a)+cy*math.Cos(a)
			g.Balls = append(g.Balls, Ball{K: ballGhost, Owner: i, X: p.X + ux*off, Y: p.Y + uy*off, DX: ux, DY: uy, Left: math.Max(0, gf.RingRadius-off), HP: gf.HP, Ult: true})
		}
	}
	return events, false
}

// lickAim is the unit direction Gengar aims at: the foe, or where it faces when it is on top of it.
func (g *Game) lickAim(p, other *Player) (float64, float64) {
	if ux, uy := unit(other.X-p.X, other.Y-p.Y); ux != 0 || uy != 0 {
		return ux, uy
	}
	if ux, uy := unit(float64(p.FX), float64(p.FY)); ux != 0 || uy != 0 {
		return ux, uy
	}
	return 1, 0
}

// stepLick moves a lick on by dt: the phases run one after the other, a phase of no time is passed through at once. Gengar is rooted for all of it.
func (g *Game) stepLick(i int, dt float64) ([]Event, bool) {
	p := &g.P[i]
	lk := g.cfg.Gengar.Gigantamax.Lick
	var events []Event
	if p.LickPhase == lickCharge {
		if ux, uy := g.lickAim(p, &g.P[g.nearestFoe(i)]); ux != 0 || uy != 0 {
			p.LickX, p.LickY = ux, uy // the tongue follows the foe while it gathers
		}
		if p.LickT -= dt; p.LickT < 1e-9 {
			p.LickPhase, p.LickT = lickOut, 0
			events = append(events, Event{T: "lick", X: p.X, Y: p.Y, R: lk.Length, DX: p.LickX, DY: p.LickY, S: i})
		}
	}
	if p.LickPhase == lickOut {
		p.LickD = math.Min(p.LickD+lk.Speed*dt, lk.Length)
		if j, ok := g.tongueTouches(i, p.LickD); ok {
			events = append(events, g.lickHit(i, j)...)
		} else if p.LickD >= lk.Length-1e-9 {
			p.LickPhase = lickBack
		}
	}
	if p.LickPhase == lickPull {
		events = append(events, g.pullPrey(i, dt)...)
	}
	if p.LickPhase == lickBack {
		if p.LickD -= lk.Speed * dt; p.LickD <= 1e-9 {
			p.LickD, p.LickPhase, p.LickT = 0, lickRest, lk.RechargeDuration
		}
	}
	if p.LickPhase == lickBite {
		if p.LickT -= dt; p.LickT < 1e-9 {
			p.LickPhase, p.LickT, p.LickPrey = lickRest, lk.RechargeDuration, 0
		}
	}
	if p.LickPhase == lickRest {
		if p.LickT -= dt; p.LickT < 1e-9 {
			g.dropLick(p) // rooted through the tick it ends, free from the next
		}
	}
	return events, true
}

// tongueTouches finds the foe the tongue, `d` long from Gengar along its line and `width` wide, touches first (the nearest along the line): one at the heights it reaches, that is not carried off by something else.
func (g *Game) tongueTouches(i int, d float64) (int, bool) {
	p, lk := &g.P[i], g.cfg.Gengar.Gigantamax.Lick
	tx, ty := p.X+p.LickX*d, p.Y+p.LickY*d
	best, bestAlong := -1, math.Inf(1)
	for _, j := range g.foes(i) {
		foe := &g.P[j]
		if !lk.Reach.hits(foe) || foe.Held || foe.HP <= 0 || segDist(foe.X, foe.Y, p.X, p.Y, tx, ty) > lk.Width/2+g.radius(j) {
			continue
		}
		if along := (foe.X-p.X)*p.LickX + (foe.Y-p.Y)*p.LickY; along < bestAlong {
			best, bestAlong = j, along
		}
	}
	return best, best >= 0
}

// lickHit is what the tongue does to the foe it touches: it hurts and stuns it and starts dragging it in (a practice dummy too). A foe that cannot be stunned (it is gathering a big skill) is only hurt.
func (g *Game) lickHit(i, j int) []Event {
	p, foe, lk := &g.P[i], &g.P[j], g.cfg.Gengar.Gigantamax.Lick
	g.hit(foe, lk.Damage)
	events := []Event{{T: "lickhit", X: foe.X, Y: foe.Y, R: lk.Width, S: i}}
	if foe.HP <= 0 || !g.stun(foe, 0.3) {
		p.LickPhase = lickBack // it does not take: the tongue comes back
		return events
	}
	p.LickPrey, p.LickPhase = j+1, lickPull
	return events
}

// pullPrey drags the foe in to Gengar's body at `pull_speed`, keeping it stunned, and bites it when it is there.
func (g *Game) pullPrey(i int, dt float64) []Event {
	p, lk := &g.P[i], g.cfg.Gengar.Gigantamax.Lick
	foe := g.lickPrey(p)
	if foe == nil || foe.HP <= 0 || foe.Held {
		p.LickPhase, p.LickPrey = lickBack, 0
		return nil
	}
	if !g.stun(foe, 2*dt+0.05) { // it has begun a skill that nothing stops: it is let go
		p.LickPhase, p.LickPrey = lickBack, 0
		return nil
	}
	contact := g.radius(i) + g.radius(p.LickPrey-1)
	d := math.Hypot(p.X-foe.X, p.Y-foe.Y)
	if d > contact {
		step := math.Min(lk.PullSpeed*dt, d-contact)
		foe.X, foe.Y = foe.X+(p.X-foe.X)/d*step, foe.Y+(p.Y-foe.Y)/d*step
		d -= step
	}
	p.LickD = math.Max(0, d)
	if d > contact+1e-6 {
		return nil
	}
	// it is at the body: the bite
	p.LickPhase, p.LickT, p.LickD = lickBite, lk.BiteDuration, 0
	g.hit(foe, lk.BiteDamage)
	g.stun(foe, lk.BiteStunDuration)
	events := []Event{{T: "bite", X: foe.X, Y: foe.Y, R: g.radius(i), DX: p.LickX, DY: p.LickY, S: i}}
	if p.LickT == 0 { // a bite of no time: straight on to the rest
		p.LickPhase, p.LickT, p.LickPrey = lickRest, lk.RechargeDuration, 0
	}
	return events
}

// stepDevour drags every foe within `gravity_radius` toward Gengar and silences every one within `silence_radius`, for as long as each of the two lasts.
func (g *Game) stepDevour(i int, dt float64) {
	p, dv := &g.P[i], g.cfg.Gengar.Gigantamax.Devour
	if p.DevourT <= 0 && p.DevourSilT <= 0 {
		return
	}
	for _, j := range g.foes(i) {
		foe := &g.P[j]
		d := math.Hypot(p.X-foe.X, p.Y-foe.Y)
		if !dv.Reach.hits(foe) {
			continue
		}
		if p.DevourT > 0 && !foe.Held && d > 1e-9 && d <= dv.GravityRadius+g.radius(j) {
			if step := math.Min(dv.Gravity*dt, math.Max(0, d-g.radius(i)-g.radius(j))); step > 0 { // never into its body
				foe.X, foe.Y = foe.X+(p.X-foe.X)/d*step, foe.Y+(p.Y-foe.Y)/d*step
			}
		}
		if p.DevourSilT > 0 && d <= dv.SilenceRadius+g.radius(j) {
			foe.SilentT = math.Max(foe.SilentT, silenceLinger)
		}
	}
	if p.DevourT > 0 {
		if p.DevourT -= dt; p.DevourT < 1e-9 {
			p.DevourT = 0
		}
	}
	if p.DevourSilT > 0 {
		if p.DevourSilT -= dt; p.DevourSilT < 1e-9 {
			p.DevourSilT = 0
		}
	}
}
