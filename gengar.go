package main

import "math"

// Gengar.
//
// Shadow Sneak (Skill): it slips into the ground for `duration` seconds. Its height is then `height` (below zero), so only
// a skill whose band starts that low can touch it. A black shadow `shadow_radius` wide marks where it is, and an opponent
// touching the shadow cannot start a skill. It is steered at the shadow's own speeds (first half, second half) meanwhile,
// and uses no other skill.
//
// Ghost Fire (GFire): purple flames fly out all round it, then come back to it wherever it has gone. They are projectiles
// with HP (the opponent's balls and skills wear them down) and each hurts at most once.
//
// Shadow Ball (SBall): a ball is lobbed, and what it does it does where it lands: it hurts, and flames fly out of it that
// do not come back, and the whole burst hurts at most once.

// silenceLinger is how long an opponent stays unable to use a skill after it last touched the shadow.
const silenceLinger = 0.15

// ghostFire is one flame a Shadow Ball sends out where it lands: it flies out and is gone, and has no HP. All the flames of
// one ball share Hurt, so however many of them touch the opponent it is hurt once. Gengar's own Ghost Fire flames are
// projectiles in Game.Balls instead (ballGhost).
type ghostFire struct {
	Owner int     `json:"o"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	DX    float64 `json:"-"`
	DY    float64 `json:"-"`
	Dist  float64 `json:"-"` // how far it has flown out
	Max   float64 `json:"-"` // how far out it goes
	Speed float64 `json:"-"`
	R     float64 `json:"-"`
	Dmg   int     `json:"-"`
	Reach Reach   `json:"-"`
	Hurt  *bool   `json:"-"` // set once any flame of its ball has hurt the opponent
}

// shadowBall is a Shadow Ball in the air.
type shadowBall struct {
	Owner int     `json:"o"`
	X     float64 `json:"x"` // where it is on the ground, below itself
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	TX    float64 `json:"tx"` // where it will land
	TY    float64 `json:"ty"`
	T     float64 `json:"t"` // seconds it has flown
	X0    float64 `json:"-"`
	Y0    float64 `json:"-"`
}

// maxGhostFires bounds the flames on the court, whatever the config says.
const maxGhostFires = 400

// dropGengar ends whatever Gengar was doing (frozen, stunned or caught): it comes up out of the ground.
func (g *Game) dropGengar(i int) {
	p := &g.P[i]
	if p.SneakT > 0 {
		p.SneakT = 0
		if !p.Held {
			p.Z = 0
		}
	}
}

// sneakSpeed is how fast Gengar is steered under the ground: the first speed until the middle of the sneak, the second after.
func (g *Game) sneakSpeed(p *Player) float64 {
	ss := g.cfg.Gengar.ShadowSneak
	if ss.Duration-p.SneakT < ss.Duration/2 {
		return ss.ShadowSpeedFirstHalf
	}
	return ss.ShadowSpeedSecondHalf
}

// stepGengar runs Gengar's three skills.
func (g *Game) stepGengar(i int, in Input, dt float64) []Event {
	p, other := &g.P[i], &g.P[1-i]
	gs := g.cfg.Gengar
	ss, gf, sb := gs.ShadowSneak, gs.GhostFire, gs.ShadowBall
	for _, cd := range []*float64{&p.SneakCD, &p.GFireCD, &p.SBallCD} {
		if *cd > 0 {
			if *cd -= dt; *cd < 1e-9 {
				*cd = 0
			}
		}
	}

	// ---- Shadow Sneak ----
	if in.Skill && p.SneakT == 0 && p.SneakCD == 0 && p.Z == 0 {
		p.SneakT, p.SneakCD = ss.Duration, ss.Cooldown
	}
	if p.SneakT > 0 {
		p.Z = ss.Height
		if p.SneakT -= dt; p.SneakT < 1e-9 {
			p.SneakT, p.Z = 0, 0
			return nil // up again: no skill on the tick it comes up
		}
		if ss.Reach.hits(other) && math.Hypot(other.X-p.X, other.Y-p.Y) <= ss.ShadowRadius+g.radius(1-i) {
			other.SilentT = silenceLinger
		}
		return nil
	}
	if p.Z != 0 {
		return nil // lifted by something else
	}

	// ---- Ghost Fire ----
	if in.GFire && p.GFireCD == 0 {
		p.GFireCD = gf.Cooldown
		off := g.radius(i) + gf.Radius
		for n := 0; n < gf.Count && n < maxGhostFires; n++ {
			a := 2 * math.Pi * float64(n) / float64(gf.Count)
			ux, uy := math.Cos(a), math.Sin(a)
			g.Balls = append(g.Balls, Ball{K: ballGhost, Owner: i, X: p.X + ux*off, Y: p.Y + uy*off, DX: ux, DY: uy, Left: math.Max(0, gf.RingRadius-off), HP: gf.HP})
		}
	}

	// ---- Shadow Ball ----
	if in.SBall && p.SBallCD == 0 {
		p.SBallCD = sb.Cooldown
		reach := sb.Speed * sb.Duration
		var tx, ty float64
		if n := math.Hypot(in.CastX, in.CastY); n > 1e-9 && !math.IsInf(n, 0) { // an aim was chosen: it lands that far out along it
			d := reach * math.Min(n, 1) // its length is how far, as a share of the range: a pull of half lands halfway
			tx, ty = p.X+in.CastX/n*d, p.Y+in.CastY/n*d
		} else { // a tap: on the opponent, or as near as it can get
			ux, uy := g.castDir(i, in)
			d := math.Min(math.Hypot(other.X-p.X, other.Y-p.Y), reach)
			tx, ty = p.X+ux*d, p.Y+uy*d
		}
		tx, ty = math.Min(math.Max(tx, 0), g.court.Length), math.Min(math.Max(ty, 0), g.court.Width)
		g.Shadows = append(g.Shadows, shadowBall{Owner: i, X: p.X, Y: p.Y, X0: p.X, Y0: p.Y, TX: tx, TY: ty})
	}
	return nil
}

func (g *Game) addGhost(f ghostFire) {
	g.Ghosts = append(g.Ghosts, f)
	if over := len(g.Ghosts) - maxGhostFires; over > 0 {
		g.Ghosts = g.Ghosts[over:]
	}
}

// stepGhosts flies the flames a Shadow Ball sent out; the burst hurts the opponent once if any of them touches it.
func (g *Game) stepGhosts(dt float64) {
	kept := g.Ghosts[:0]
	for _, f := range g.Ghosts {
		foe := &g.P[1-f.Owner]
		x0, y0 := f.X, f.Y
		step := math.Min(f.Speed*dt, f.Max-f.Dist)
		f.X, f.Y, f.Dist = f.X+f.DX*step, f.Y+f.DY*step, f.Dist+step
		if !*f.Hurt && f.Reach.hits(foe) && segDist(foe.X, foe.Y, x0, y0, f.X, f.Y) <= f.R+g.radius(1-f.Owner) {
			*f.Hurt = true
			g.hit(foe, f.Dmg)
		}
		if f.Dist < f.Max-1e-9 { // the last step still gets to hit
			kept = append(kept, f)
		}
	}
	g.Ghosts = kept
}

// stepShadowBalls flies the lobbed balls and settles them where they land.
func (g *Game) stepShadowBalls(dt float64) []Event {
	var events []Event
	sb := g.cfg.Gengar.ShadowBall
	kept := g.Shadows[:0]
	for _, b := range g.Shadows {
		foe := &g.P[1-b.Owner]
		if b.T += dt; b.T < sb.Duration-1e-9 {
			u := b.T / sb.Duration
			b.X, b.Y = b.X0+(b.TX-b.X0)*u, b.Y0+(b.TY-b.Y0)*u
			b.Z = 4 * sb.Height * u * (1 - u)
			kept = append(kept, b)
			continue
		}
		// it comes down: the blast, then the flames
		events = append(events, Event{T: "gslam", X: b.TX, Y: b.TY, R: sb.SlamRadius})
		if sb.Reach.hits(foe) && math.Hypot(foe.X-b.TX, foe.Y-b.TY) <= sb.SlamRadius+g.radius(1-b.Owner) {
			g.hit(foe, sb.Damage)
		}
		hurt := new(bool)
		for n := 0; n < sb.GhostFireCount; n++ {
			a := 2 * math.Pi * float64(n) / float64(sb.GhostFireCount)
			ux, uy := math.Cos(a), math.Sin(a)
			g.addGhost(ghostFire{Owner: b.Owner, X: b.TX, Y: b.TY, DX: ux, DY: uy, Max: sb.GhostFireSpreadDistance, Speed: sb.GhostFireSpreadSpeed,
				R: sb.GhostFireRadius, Dmg: sb.GhostFireDamage, Reach: sb.Reach, Hurt: hurt})
		}
	}
	g.Shadows = kept
	return events
}
