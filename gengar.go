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

// The two ways a Shadow Ball is used (config gengar.shadow_ball.mode).
const (
	shadowCharge = "charge" // rooted for charge_time, then it flies straight like the Electro Ball and drags the opponent in where it bursts
	shadowThrow  = "throw"  // it is lobbed to where it lands
)

// pullWell is where a Shadow Ball burst, dragging the opponent toward its middle for as long as it lasts.
type pullWell struct {
	Owner int
	X, Y  float64
	T     float64 // seconds left
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
	p.SBallT = 0 // a charge is lost
}

// sneakSpeed is how fast Gengar is steered under the ground: the first speed until the middle of the sneak, the second after.
func (g *Game) sneakSpeed(p *Player) float64 {
	ss := g.cfg.Gengar.ShadowSneak
	if ss.Duration-p.SneakT < ss.Duration/2 {
		return ss.ShadowSpeedFirstHalf
	}
	return ss.ShadowSpeedSecondHalf
}

// stepGengar runs Gengar's three skills. It says whether Gengar is rooted (charging a Shadow Ball).
func (g *Game) stepGengar(i int, in Input, dt float64) ([]Event, bool) {
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

	// ---- Shadow Ball, charging: rooted, and no other skill meanwhile ----
	if p.SBallT > 0 {
		if p.SBallLock { // the aim chosen at the cast, whatever the stick says meanwhile
			p.AimX, p.AimY = p.SBallDX, p.SBallDY
		} else if dx, dy := other.X-p.X, other.Y-p.Y; math.Hypot(dx, dy) > 1e-9 { // on top of it there is no direction: the aim stays
			n := math.Hypot(dx, dy)
			p.AimX, p.AimY = dx/n, dy/n
		}
		if p.SBallT -= dt; p.SBallT < 1e-9 {
			p.SBallT = 0
			g.launchShadowBall(i)
		}
		return nil, true // rooted through the tick it leaves, free from the next
	}

	// ---- Shadow Sneak ----
	if in.Skill && p.SneakT == 0 && p.SneakCD == 0 && p.Z == 0 {
		p.SneakT, p.SneakCD = ss.Duration, ss.Cooldown
	}
	if p.SneakT > 0 {
		p.Z = ss.Height
		if p.SneakT -= dt; p.SneakT < 1e-9 {
			p.SneakT, p.Z = 0, 0
			return nil, false // up again: no skill on the tick it comes up
		}
		if ss.Reach.hits(other) && math.Hypot(other.X-p.X, other.Y-p.Y) <= ss.ShadowRadius+g.radius(1-i) {
			other.SilentT = silenceLinger
		}
		return nil, false
	}
	if p.Z != 0 {
		return nil, false // lifted by something else
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
	if in.SBall && p.SBallCD == 0 && sb.Mode == shadowCharge {
		p.SBallCD, p.SBallT, p.SBallLock = sb.Cooldown, sb.ChargeTime, false
		if n := math.Hypot(in.CastX, in.CastY); n > 1e-9 && !math.IsInf(n, 0) { // an aim was chosen at the cast
			p.SBallDX, p.SBallDY, p.SBallLock = in.CastX/n, in.CastY/n, true
			p.AimX, p.AimY = p.SBallDX, p.SBallDY
		} else if dx, dy := other.X-p.X, other.Y-p.Y; math.Hypot(dx, dy) > 1e-9 {
			n := math.Hypot(dx, dy)
			p.AimX, p.AimY = dx/n, dy/n
		}
		if p.SBallT == 0 { // no charge: it leaves at once
			g.launchShadowBall(i)
		}
		return nil, p.SBallT > 0
	}
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
	return nil, false
}

// launchShadowBall sends a charged Shadow Ball out along the aim, starting just clear of Gengar's body.
func (g *Game) launchShadowBall(i int) {
	p, sb := &g.P[i], g.cfg.Gengar.ShadowBall
	off := g.radius(i) + sb.Radius + 1
	g.Balls = append(g.Balls, Ball{K: ballShadow, Owner: i, X: p.X + p.AimX*off, Y: p.Y + p.AimY*off, DX: p.AimX, DY: p.AimY, HP: sb.HP})
}

// shadowBlast is what a Shadow Ball does where it comes down or bursts: the blast hurts, the flames fly out of it, and (charge
// mode) the opponent in its reach is dragged toward its middle for `pull_duration` seconds.
func (g *Game) shadowBlast(owner int, x, y float64, pull bool) []Event {
	sb, foe := g.cfg.Gengar.ShadowBall, &g.P[1-owner]
	events := []Event{{T: "gslam", X: x, Y: y, R: sb.SlamRadius}}
	if sb.Reach.hits(foe) && math.Hypot(foe.X-x, foe.Y-y) <= sb.SlamRadius+g.radius(1-owner) {
		g.hit(foe, sb.Damage)
	}
	hurt := new(bool)
	for n := 0; n < sb.GhostFireCount; n++ {
		a := 2 * math.Pi * float64(n) / float64(sb.GhostFireCount)
		ux, uy := math.Cos(a), math.Sin(a)
		g.addGhost(ghostFire{Owner: owner, X: x, Y: y, DX: ux, DY: uy, Max: sb.GhostFireSpreadDistance, Speed: sb.GhostFireSpreadSpeed,
			R: sb.GhostFireRadius, Dmg: sb.GhostFireDamage, Reach: sb.Reach, Hurt: hurt})
	}
	if pull && sb.PullDuration > 0 && sb.Gravity > 0 {
		g.Wells = append(g.Wells, pullWell{Owner: owner, X: x, Y: y, T: sb.PullDuration})
		events = append(events, Event{T: "gwell", X: x, Y: y, R: sb.SlamRadius})
	}
	return events
}

// stepWells drags the opponent toward the middle of every burst that is still pulling: at `gravity` pixels a second, never past
// the middle, for the opponent whose body touches the reach of the blast at the heights the skill hits. A planted dummy, and one
// carried off by something else, stay where they are.
func (g *Game) stepWells(dt float64) {
	sb := g.cfg.Gengar.ShadowBall
	kept := g.Wells[:0]
	for _, w := range g.Wells {
		foe := &g.P[1-w.Owner]
		if d := math.Hypot(w.X-foe.X, w.Y-foe.Y); foe.Kind != dummy && !foe.Held && sb.Reach.hits(foe) && d > 1e-9 && d <= sb.SlamRadius+g.radius(1-w.Owner) {
			step := math.Min(sb.Gravity*dt, d)
			foe.X, foe.Y = foe.X+(w.X-foe.X)/d*step, foe.Y+(w.Y-foe.Y)/d*step
		}
		if w.T -= dt; w.T > 1e-9 {
			kept = append(kept, w)
		}
	}
	g.Wells = kept
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
		if b.T += dt; b.T < sb.Duration-1e-9 {
			u := b.T / sb.Duration
			b.X, b.Y = b.X0+(b.TX-b.X0)*u, b.Y0+(b.TY-b.Y0)*u
			b.Z = 4 * sb.Height * u * (1 - u)
			kept = append(kept, b)
			continue
		}
		// it comes down: the blast, then the flames
		events = append(events, g.shadowBlast(b.Owner, b.TX, b.TY, false)...)
	}
	g.Shadows = kept
	return events
}
