package main

import "math"

// The ultimate (Z): a third skill that starts every round locked and has to be charged (see Ultimate in config.go). Only the
// characters whose ultimate is written have one; the others' pads and keys stay as they were.
//
// Pikachu's is Thousand Volt: a ring of bolts that fly out, then chase the foe, push it back where they touch it and all
// explode when terminal_time is up.

// gatheringBall is whether p is in the second half of a big skill's gathering (the black and white ball swelling: Thousand Volt's
// `ball_duration`, the Megaton Punch's): from there to the end it cannot be stunned or frozen.
func (g *Game) gatheringBall(p *Player) bool {
	switch p.Kind {
	case pikachu:
		return p.UltT > 0 && p.UltT <= g.cfg.Pikachu.ThousandVolt.BallDuration
	case ash:
		return p.MegaT > 0 && p.MegaT <= g.cfg.Ash.MegatonPunch.BallDuration
	}
	return false
}

// freeze and stun keep p frozen or stunned for at least d seconds, unless it is gathering its ball (see gatheringBall): then nothing
// happens. They say whether it took, for the callers that also show the freeze.
func (g *Game) freeze(p *Player, d float64) bool {
	if g.gatheringBall(p) {
		return false
	}
	p.FrozenT = math.Max(p.FrozenT, d)
	return true
}

func (g *Game) stun(p *Player, d float64) bool {
	if g.gatheringBall(p) {
		return false
	}
	p.StunT = math.Max(p.StunT, d)
	return true
}

// maxVoltBolts bounds the bolts of one cast, whatever the config says.
const maxVoltBolts = 40

// ultimateOf is the charge rule of a character's ultimate, and whether that character has one.
func (c Config) ultimateOf(kind int) (Ultimate, bool) {
	switch kind {
	case pikachu:
		return c.Pikachu.Ultimate, true
	case ash:
		return c.Ash.Ultimate, true
	}
	return Ultimate{}, false
}

// gainCharge fills every ultimate by what happened over this tick: time passing, the HP each character lost (hp0 is everyone's
// HP when the tick began, so every source of damage counts and none needs to say who dealt it) and the HP its foes lost.
func (g *Game) gainCharge(hp0 [maxSlots]int, dt float64) {
	for i := range g.P {
		p := &g.P[i]
		u, ok := g.cfg.ultimateOf(p.Kind)
		if !ok || p.HP <= 0 {
			continue
		}
		gain := u.GainPerSecond * dt
		if lost := hp0[i] - p.HP; lost > 0 {
			gain += u.GainTaken * float64(lost)
		}
		for _, j := range g.foes(i) {
			if lost := hp0[j] - g.P[j].HP; lost > 0 {
				gain += u.GainDealt * float64(lost)
			}
		}
		p.Charge = math.Min(p.Charge+gain, u.Max)
	}
}

// stepUltimate runs Z for a character that has an ultimate: with a full charge the press empties the charge and starts the
// gathering, and when that is over the skill itself goes off. It says whether the character is rooted by the gathering.
func (g *Game) stepUltimate(i int, in Input, dt float64) ([]Event, bool) {
	p := &g.P[i]
	u, ok := g.cfg.ultimateOf(p.Kind)
	if !ok || p.Kind != pikachu { // Ash's is the Megaton Punch, started by stepMachamp
		return nil, false
	}
	held := p.Kind == pikachu && g.cfg.Pikachu.ThousandVolt.RootedWhileBolts && g.voltBoltsOut(i) // the bolts it let go are still out
	if p.UltT == 0 && in.Ult && p.Charge >= u.Max {
		p.Charge = 0
		p.UltT = g.cfg.Pikachu.ThousandVolt.RingDuration + g.cfg.Pikachu.ThousandVolt.BallDuration
		p.DashT = 0 // gathering roots Pikachu at once
		if p.UltT == 0 {
			return g.thousandVolt(i), false
		}
	}
	if p.UltT <= 0 {
		return nil, held
	}
	if p.UltT -= dt; p.UltT < 1e-9 {
		p.UltT = 0
		return g.thousandVolt(i), true // rooted through the tick it goes off, free from the next
	}
	return nil, true
}

// liftUltimate flips Pikachu up to jump_height while it gathers and keeps it there until its bolts are done, then lets it down. The rise and
// the fall each take ring_duration. A Pikachu a Charizard holds is moved by the toss instead.
func (g *Game) liftUltimate(i int, dt float64) {
	p, tv := &g.P[i], g.cfg.Pikachu.ThousandVolt
	if p.Kind != pikachu || p.Held {
		return
	}
	want := 0.0
	if p.UltT > 0 || (tv.RootedWhileBolts && g.voltBoltsOut(i)) {
		want = tv.JumpHeight
		p.Lifted = true
	}
	if !p.Lifted {
		return
	}
	defer func() { p.Lifted = p.Z > 0 }()
	if tv.RingDuration <= 0 {
		p.Z = want
		return
	}
	step := tv.JumpHeight / tv.RingDuration * dt
	p.Z = math.Min(math.Max(want, p.Z-step), p.Z+step)
}

// thousandVolt lets the ring of bolts out round Pikachu slot i.
func (g *Game) thousandVolt(i int) []Event {
	p, tv := &g.P[i], g.cfg.Pikachu.ThousandVolt
	off := g.radius(i) + tv.Radius
	burst := new([maxSlots]bool) // which foes the explosions have hurt: all the bolts of a cast share it, so each foe is hurt once
	for n := 0; n < tv.Count && n < maxVoltBolts; n++ {
		a := 2 * math.Pi * float64(n) / float64(tv.Count)
		ux, uy := math.Cos(a), math.Sin(a)
		reach := math.Max(off, tv.RingRadius) // where the bolt ends its flight out
		g.Balls = append(g.Balls, Ball{K: ballVolt, Idx: n, Owner: i, Side: i + 1, X: p.X + ux*off, Y: p.Y + uy*off, AnchorX: math.Min(math.Max(p.X+ux*reach, 0), g.court.Length), AnchorY: math.Min(math.Max(p.Y+uy*reach, 0), g.court.Width), DX: ux, DY: uy, Left: math.Max(0, tv.RingRadius-off), HP: tv.HP, Life: tv.TerminalTime, Burst: burst})
	}
	return []Event{{T: "ult", X: p.X, Y: p.Y, R: tv.RingRadius, K: p.Kind}}
}

// voltBoltsOut is whether slot i still has a Thousand Volt bolt in the air.
func (g *Game) voltBoltsOut(i int) bool {
	for _, b := range g.Balls {
		if b.K == ballVolt && b.Owner == i && b.HP > 0 {
			return true
		}
	}
	return false
}

// stepVoltBolt moves one Thousand Volt bolt, hurts the foe it touches and blows it up when its time is up. It says whether the bolt
// is still there.
func (g *Game) stepVoltBolt(b *Ball, dt float64) (bool, []Event) {
	if b.HP <= 0 {
		return false, nil
	}
	tv := g.cfg.Pikachu.ThousandVolt
	x0, y0 := b.X, b.Y
	if b.Left > 1e-9 { // still flying out
		step := math.Min(tv.SpreadSpeed*dt, b.Left)
		b.X, b.Y, b.Left = b.X+b.DX*step, b.Y+b.DY*step, b.Left-step
		b.X, b.Y = math.Min(math.Max(b.X, 0), g.court.Length), math.Min(math.Max(b.Y, 0), g.court.Width)
	} else { // straight at where the nearest foe is now, without overshooting it
		foe := g.foeNearTo(b.Owner, b.X, b.Y)
		dx, dy := foe.X-b.X, foe.Y-b.Y
		if d := math.Hypot(dx, dy); d > 1e-9 {
			step := math.Min(d, tv.FollowSpeed*dt)
			b.DX, b.DY = dx/d, dy/d
			b.X, b.Y = b.X+b.DX*step, b.Y+b.DY*step
		}
	}
	b.FoeCD = hitCDTick(b.FoeCD, dt)
	if b.FoeCD <= 1e-9 && (!b.Hit || tv.RehitInterval > 0) {
		if foe, ok := g.touchedFoe(b.Owner, func(f *Player, rad float64) bool {
			return tv.Reach.hits(f) && segDist(f.X, f.Y, x0, y0, b.X, b.Y) <= tv.Radius+rad
		}); ok {
			b.Hit, b.FoeCD = true, hitCDNext(b.FoeCD, tv.RehitInterval)
			g.hit(foe, tv.ContactDamage)
			g.knockBack(b.X, b.Y, foe, tv.Knockback, b.DX, b.DY)
			b.Hits++
		}
	}
	if b.Life -= dt; b.Life > 1e-9 && !(tv.HitsToExplode > 0 && b.Hits >= tv.HitsToExplode) {
		return true, nil
	}
	for _, j := range g.foes(b.Owner) {
		if foe := &g.P[j]; !b.Burst[j] && tv.Reach.hits(foe) && math.Hypot(foe.X-b.X, foe.Y-b.Y) <= tv.ExplosionRadius+g.radius(j) {
			b.Burst[j] = true
			g.hit(foe, tv.ExplosionDamage)
		}
	}
	return false, []Event{{T: "voltburst", X: b.X, Y: b.Y, R: tv.ExplosionRadius}}
}
