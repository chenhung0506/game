package main

import (
	"fmt"
	"math"
)

// Jigglypuff: a ball of a body with two skills (see Config.Jigglypuff).
//
// The main button (Skill) is a chain of blows, one per press: on the ground five hand blows, in the air three stomps/kicks. Each press starts a swing
// that takes `duration[n]` seconds (rooted, the fan it will hit drawn on the ground for the opponent to see) and lands at the end of it. A press made
// while a swing is still on is remembered and starts the next one the moment it lands. The chain has `use_window` seconds from its first press; when
// it is finished, runs out, or (in the air) a stomp misses or Jigglypuff lands, the cooldown starts.
//
// The second skill (Roll) rolls along the stick and kicks whoever it reaches at 45 degrees; both fly up together.
//
// A foe that is stomped or kicked is a ball: Held, and flown by stepArcs (gravity, a bounce when it lands if it has one coming).

// rollTime is how long the roll takes: its distance at its speed.
// slapStepTime is how long the step forward after a ground blow takes.
const slapStepTime = 0.12

func (rk rollKickCfg) rollTime() float64 {
	if rk.RollSpeed <= 0 {
		return 0
	}
	return rk.RollDistance / rk.RollSpeed
}

// stepJigglypuff runs both skills and reports whether Jigglypuff is rooted this tick (swinging or rolling: the walking code stays out of it).
func (g *Game) stepJigglypuff(i int, in Input, dt float64) ([]Event, bool) {
	var events []Event
	p := &g.P[i]
	jp := g.cfg.Jigglypuff
	for _, t := range []*float64{&p.SlapCD, &p.RollCD, &p.SlapRecT} {
		if *t > 0 {
			if *t -= dt; *t < 1e-9 {
				*t = 0
			}
		}
	}
	press := in.Skill && !p.SlapHeld
	p.SlapHeld = in.Skill
	if press {
		p.SlapQ = true
	}
	rollPress := in.Roll && !p.RollHeld
	p.RollHeld = in.Roll

	// the roll: along the stick (or where it faces), then the kick
	if rollPress && p.RollT == 0 && p.RollCD == 0 && p.SlapT == 0 && !p.Held && p.Z == 0 && !p.JpFly { // (between the blows of a chain too: use_window does not lock the roll out)
		rk := jp.RollKick
		dx, dy := float64(in.DX), float64(in.DY)
		if dx == 0 && dy == 0 {
			dx, dy = float64(p.FX), float64(p.FY)
		}
		n := math.Hypot(dx, dy)
		p.RollX, p.RollY = dx/n, dy/n
		p.RollT, p.RollCD = rk.rollTime(), rk.Cooldown
		p.SlapQ = false
		if p.SlapChain { // rolling out of a chain ends it, with no cooldown, so the air chain after the kick starts fresh
			p.endChain(0)
		}
		if p.RollT == 0 {
			events = append(events, g.rollKick(i)...)
		}
	}
	if p.RollT > 0 {
		rk := jp.RollKick
		step := math.Min(dt, p.RollT)
		p.X = math.Min(math.Max(p.X+p.RollX*rk.RollSpeed*step, 0), g.court.Length)
		p.Y = math.Min(math.Max(p.Y+p.RollY*rk.RollSpeed*step, 0), g.court.Width)
		if p.RollT -= dt; p.RollT < 1e-9 {
			p.RollT = 0
			events = append(events, g.rollKick(i)...)
		}
	}

	// the step forward after a blow: a short slide along the aim that stops at a foe's body
	if p.StepLeft > 0 {
		step := math.Min(p.StepLeft, p.StepV*dt)
		nx, ny := math.Min(math.Max(p.X+p.StepDX*step, 0), g.court.Length), math.Min(math.Max(p.Y+p.StepDY*step, 0), g.court.Width)
		blocked := false
		for _, j := range g.foes(i) {
			if f := &g.P[j]; math.Hypot(f.X-nx, f.Y-ny) < g.radius(i)+g.radius(j) && f.Z <= p.Z+10 {
				blocked = true
			}
		}
		if blocked {
			p.StepLeft = 0
		} else {
			p.X, p.Y, p.StepLeft = nx, ny, p.StepLeft-step
		}
	}
	// the chain: a swing in progress lands, or the next press starts one
	if p.SlapT > 0 {
		if p.SlapT -= dt; p.SlapT < 1e-9 {
			p.SlapT = 0
			events = append(events, g.comboStrike(i)...)
		}
	}
	if p.SlapQ && p.SlapT == 0 && p.RollT == 0 && !p.Held {
		if p.SlapChain || p.SlapCD == 0 {
			if !p.SlapChain {
				p.SlapChain, p.SlapAir, p.SlapN, p.SlapWin = true, p.Z > 0 || p.JpFly, 0, jp.Combo.UseWindow
			}
			p.SlapQ = false
			p.SlapX, p.SlapY = g.castDir(i, in)
			p.SlapT = p.comboDuration(jp, p.SlapN)
			if p.SlapN == 0 && !p.SlapAir && jp.Combo.Ground.LungeDistance > 0 { // the first blow: it closes in while the hand comes down
				p.StepLeft, p.StepDX, p.StepDY = jp.Combo.Ground.LungeDistance, p.SlapX, p.SlapY
				p.StepV = p.StepLeft / math.Max(p.SlapT, slapStepTime)
			}
			if p.SlapT == 0 {
				events = append(events, g.comboStrike(i)...)
			}
		}
	}
	if p.SlapT == 0 && !(p.SlapChain && p.SlapQ) {
		p.SlapQ = false // a press that could not start anything is not kept
	}
	if p.SlapChain && p.SlapT == 0 {
		if p.SlapWin -= dt; p.SlapWin < 1e-9 {
			p.endChain(jp.Combo.Cooldown)
		}
	}
	return events, p.SlapT > 0 || p.RollT > 0 // rooted through the tick it ends in, free from the next
}

// comboDuration is how long the swing n of the running chain takes.
func (p *Player) comboDuration(jp jigglypuffCfg, n int) float64 {
	if p.SlapAir {
		return jp.Combo.Air.Duration[n]
	}
	return jp.Combo.Ground.Duration[n]
}

// endChain stops the chain and starts its cooldown.
func (p *Player) endChain(cooldown float64) {
	p.SlapChain, p.SlapAir, p.SlapN, p.SlapT, p.SlapQ, p.SlapWin = false, false, 0, 0, false, 0
	p.SlapCD = cooldown
}

// dropJigglypuff cuts both skills short (stunned, frozen, held): a chain that was running starts its cooldown.
func (g *Game) dropJigglypuff(i int) {
	p := &g.P[i]
	if p.SlapChain {
		p.endChain(g.cfg.Jigglypuff.Combo.Cooldown)
	}
	p.SlapT, p.SlapQ, p.RollT, p.StepLeft = 0, false, 0, 0
}

// comboStrike is the swing landing: the ground fan, or the air stomp.
func (g *Game) comboStrike(i int) []Event {
	p := &g.P[i]
	c := g.cfg.Jigglypuff.Combo
	n := p.SlapN
	var events []Event
	if p.SlapAir {
		hit, ev := g.stomp(i, n)
		events = ev
		if n++; !hit || n >= len(c.Air.Duration) {
			p.endChain(c.Cooldown)
		} else {
			p.SlapN = n
		}
		return events
	}
	events = g.slap(i, n)
	if n++; n >= len(c.Ground.Duration) {
		p.endChain(c.Cooldown)
	} else {
		p.SlapN = n
	}
	return events
}

// slap is a hand coming down: everyone in the fan fixed when the swing started is hurt, knocked back and stunned.
func (g *Game) slap(i, n int) []Event {
	p := &g.P[i]
	gr := g.cfg.Jigglypuff.Combo.Ground
	p.SlapRecT, p.JpPose = 0.2, 0 // for the page: the hand is down (0: the right hand did it)
	if n == 1 || n == 2 || n == 3 {
		p.JpPose = 3 // the left hand did (the last of the quick right-left pair, for blows 3 and 4)
	}
	ux, uy := p.SlapX, p.SlapY
	for _, j := range g.foes(i) {
		other := &g.P[j]
		dx, dy := other.X-p.X, other.Y-p.Y
		d := math.Hypot(dx, dy)
		if gr.Reach.hits(other) && d <= gr.Length+g.radius(j) && (d < 1e-9 || (dx*ux+dy*uy)/d >= math.Cos(gr.Angle/2*math.Pi/180)) {
			g.hit(other, gr.Damage[n])
			g.knockBack(p.X, p.Y, other, gr.Knockback[n], ux, uy)
			if gr.StunDuration[n] > 0 {
				g.stun(other, gr.StunDuration[n])
			}
		}
	}
	if d := gr.StepDistance[n]; d > 0 {
		p.StepLeft, p.StepV, p.StepDX, p.StepDY = d, d/slapStepTime, ux, uy
	}
	return []Event{{T: "slap", X: p.X, Y: p.Y, R: gr.Length, DX: ux, DY: uy, K: n}}
}

// stomp is an air swing landing (n: 0 and 1 stomp, 2 kicks): it says whether it hit anyone.
func (g *Game) stomp(i, n int) (bool, []Event) {
	p := &g.P[i]
	jp := g.cfg.Jigglypuff
	air := jp.Combo.Air
	p.SlapRecT, p.JpPose = 0.2, 1+n/2
	var hit bool
	var events []Event
	for _, j := range g.foes(i) {
		f := &g.P[j]
		dx, dy := f.X-p.X, f.Y-p.Y
		d := math.Hypot(dx, dy)
		below := f.Z <= p.Z+10 // a stomp needs the foe under it; the kick (the third blow) takes a ball flying anywhere within `radius` above or below
		if n == 2 {
			below = math.Abs(f.Z-p.Z) <= air.Radius
		}
		if d > air.Radius+g.radius(j) || !air.Reach.hits(f) || !below || (f.Held && !f.ArcOn) {
			continue
		}
		ux, uy := p.SlapX, p.SlapY
		if d > 1e-9 {
			ux, uy = dx/d, dy/d
		}
		hit = true
		g.hit(f, air.Damage[n])
		g.knockBack(p.X, p.Y, f, air.Knockback[n], ux, uy)
		if n < 2 { // driven down: it lands, then bounces up like a ball
			f.ArcOn, f.Held, f.VZ, f.ArcVX, f.ArcVY = true, true, -air.SlamSpeed, 0, 0
			f.ArcBounce, f.ArcDist, f.ArcDX, f.ArcDY = air.FoeBounceHeight[n], air.FoeBounceDistance[n], ux, uy
			f.ArcG = 0 // a stomped foe is a ball under `gravity`
			events = append(events, Event{T: "stomp", X: f.X, Y: f.Y, Z: f.Z, R: air.Radius, K: n})
		} else { // kicked away: it falls, and bounces up when it lands, `foe_bounce_height[2]` high, drifting `foe_bounce_distance[2]` along the kick
			f.ArcBounce, f.ArcDist, f.ArcDX, f.ArcDY, f.ArcVX, f.ArcVY, f.VZ = air.FoeBounceHeight[2], air.FoeBounceDistance[2], ux, uy, 0, 0, math.Min(f.VZ, 0)
			f.ArcOn, f.Held = true, true
			f.ArcG = jp.SelfGravity // kicked away: it falls like Jigglypuff does
			events = append(events, Event{T: "stomp", X: f.X, Y: f.Y, Z: f.Z, R: air.Radius, K: n, DX: ux, DY: uy})
		}
	}
	if hit && n < 2 { // and it bounces higher off the foe
		p.VZ, p.JpFly = math.Sqrt(2*jp.SelfGravity*math.Max(0, air.SelfBounceHeight[n]-p.Z)), true
		if d := air.SelfBounceDistance[n]; d > 0 && p.VZ > 0 { // it drifts the way the blow was aimed, over the whole flight (up to the top and down again to the ground)
			t := (p.VZ + math.Sqrt(p.VZ*p.VZ+2*jp.SelfGravity*p.Z)) / jp.SelfGravity
			p.JpVX, p.JpVY = p.SlapX*d/t, p.SlapY*d/t
		}
	}
	return hit, events
}

// rollKick is the end of the roll: whoever is within reach is kicked up and away at 45 degrees, and Jigglypuff flies up with them.
func (g *Game) rollKick(i int) []Event {
	p := &g.P[i]
	jp := g.cfg.Jigglypuff
	rk := jp.RollKick
	var hit bool
	for _, j := range g.foes(i) {
		f := &g.P[j]
		if !rk.Reach.hits(f) || math.Hypot(f.X-p.X, f.Y-p.Y) > g.radius(i)+g.radius(j)+rk.KickRadius || (f.Held && !f.ArcOn) {
			continue
		}
		hit = true
		g.hit(f, rk.Damage)
		g.stun(f, rk.StunDuration)
		v := math.Sqrt(2 * jp.SelfGravity * rk.KickHeight)
		t := 2 * v / jp.SelfGravity
		f.ArcOn, f.Held, f.VZ, f.ArcBounce, f.ArcG = true, true, v, 0, jp.SelfGravity
		f.ArcVX, f.ArcVY = p.RollX*rk.KickDistance/t, p.RollY*rk.KickDistance/t
	}
	events := []Event{{T: "rollkick", X: p.X, Y: p.Y, R: rk.KickRadius, DX: p.RollX, DY: p.RollY}}
	if hit {
		p.SlapRecT, p.JpPose, p.SlapX, p.SlapY = 0.4, 2, p.RollX, p.RollY // for the page: the kick
		p.VZ, p.JpFly = math.Sqrt(2*jp.SelfGravity*rk.SelfHeight), true
		t := 2 * p.VZ / jp.SelfGravity
		p.JpVX, p.JpVY = p.RollX*rk.SelfDistance/t, p.RollY*rk.SelfDistance/t
	}
	return events
}

// stepArcs flies the bodies Jigglypuff threw (a stomped foe is a ball under `gravity`, a kicked one under `self_gravity`, with a bounce when it lands if one is due) and Jigglypuff's own
// flights (a ball too: it ends when it lands, and an air chain with it).
func (g *Game) stepArcs(dt float64) []Event {
	var events []Event
	gr, sg := g.cfg.Jigglypuff.Gravity, g.cfg.Jigglypuff.SelfGravity
	for i := range g.P {
		p := &g.P[i]
		if p.ArcOn {
			gr := gr
			if p.ArcG > 0 {
				gr = p.ArcG
			}
			p.Held = true
			p.Z += p.VZ*dt - 0.5*gr*dt*dt
			p.VZ -= gr * dt
			p.X = math.Min(math.Max(p.X+p.ArcVX*dt, 0), g.court.Length)
			p.Y = math.Min(math.Max(p.Y+p.ArcVY*dt, 0), g.court.Width)
			if p.Z <= 0 {
				p.Z = 0
				if p.ArcBounce > 0 {
					p.VZ = math.Sqrt(2 * gr * p.ArcBounce)
					t := 2 * p.VZ / gr
					p.ArcVX, p.ArcVY = p.ArcDX*p.ArcDist/t, p.ArcDY*p.ArcDist/t
					p.ArcBounce = 0
					events = append(events, Event{T: "bounce", X: p.X, Y: p.Y})
				} else {
					p.ArcOn, p.Held, p.VZ, p.ArcVX, p.ArcVY, p.ArcG = false, false, 0, 0, 0, 0
				}
			}
		}
		if p.JpFly {
			if p.Held { // something else has it now
				p.JpFly, p.JpVX, p.JpVY = false, 0, 0
				continue
			}
			p.Z += p.VZ*dt - 0.5*sg*dt*dt
			p.VZ -= sg * dt
			p.X = math.Min(math.Max(p.X+p.JpVX*dt, 0), g.court.Length)
			p.Y = math.Min(math.Max(p.Y+p.JpVY*dt, 0), g.court.Width)
			if p.Z <= 0 {
				p.Z, p.VZ, p.JpFly, p.JpVX, p.JpVY = 0, 0, false, 0, 0
				if p.SlapChain && p.SlapAir && p.SlapT == 0 { // (a blow already on its way still lands: coming down in the middle of the third swing must not lose the kick)
					p.endChain(g.cfg.Jigglypuff.Combo.Cooldown) // landing ends the air chain
				}
			}
		}
	}
	return events
}

// validateJigglypuff checks the lists and numbers of Jigglypuff's skills.
func (c Config) validateJigglypuff() error {
	jp := c.Jigglypuff
	gr, ar, rk := jp.Combo.Ground, jp.Combo.Air, jp.RollKick
	lists := []struct {
		name string
		n    int
		v    []float64
	}{
		{"combo.ground.duration", 5, gr.Duration}, {"combo.ground.knockback", 5, gr.Knockback}, {"combo.ground.stun_duration", 5, gr.StunDuration}, {"combo.ground.step_distance", 5, gr.StepDistance},
		{"combo.air.duration", 3, ar.Duration}, {"combo.air.knockback", 3, ar.Knockback},
		{"combo.air.self_bounce_height", 2, ar.SelfBounceHeight}, {"combo.air.self_bounce_distance", 2, ar.SelfBounceDistance}, {"combo.air.foe_bounce_height", 3, ar.FoeBounceHeight}, {"combo.air.foe_bounce_distance", 3, ar.FoeBounceDistance},
	}
	for _, l := range lists {
		if len(l.v) != l.n {
			return fmt.Errorf("jigglypuff.%s must have %d entries, got %d", l.name, l.n, len(l.v))
		}
		for _, v := range l.v {
			if !(v >= 0) || math.IsInf(v, 0) {
				return fmt.Errorf("jigglypuff.%s must not be negative, got %v", l.name, v)
			}
		}
	}
	for name, d := range map[string][]int{"combo.ground.damage": gr.Damage, "combo.air.damage": ar.Damage} {
		want := 5
		if name == "combo.air.damage" {
			want = 3
		}
		if len(d) != want {
			return fmt.Errorf("jigglypuff.%s must have %d entries, got %d", name, want, len(d))
		}
		for _, v := range d {
			if v < 0 {
				return fmt.Errorf("jigglypuff.%s must not be negative, got %d", name, v)
			}
		}
	}
	if !(jp.Gravity > 0) || !(jp.SelfGravity > 0) || !(ar.SlamSpeed > 0) || !(gr.Length > 0) || !(ar.Radius > 0) || !(rk.RollSpeed > 0) {
		return fmt.Errorf("jigglypuff: gravity, combo.air.slam_speed, combo.ground.length and combo.air.radius must be positive, roll_kick.roll_speed positive")
	}
	if !(gr.PalmScale >= 0 && gr.PalmScale <= 10) {
		return fmt.Errorf("jigglypuff.combo.ground.palm_scale must be between 0 and 10, got %v", gr.PalmScale)
	}
	if !(gr.Angle > 0 && gr.Angle <= 360) {
		return fmt.Errorf("jigglypuff.combo.ground.angle must be between 0 (exclusive) and 360, got %v", gr.Angle)
	}
	for name, v := range map[string]float64{"combo.use_window": jp.Combo.UseWindow, "combo.cooldown": jp.Combo.Cooldown, "combo.ground.lunge_distance": gr.LungeDistance, "roll_kick.roll_distance": rk.RollDistance, "roll_kick.stun_duration": rk.StunDuration, "roll_kick.kick_radius": rk.KickRadius, "roll_kick.kick_distance": rk.KickDistance, "roll_kick.kick_height": rk.KickHeight,
		"roll_kick.self_height": rk.SelfHeight, "roll_kick.self_distance": rk.SelfDistance, "roll_kick.cooldown": rk.Cooldown} {
		if !(v >= 0) || math.IsInf(v, 0) {
			return fmt.Errorf("jigglypuff.%s must not be negative, got %v", name, v)
		}
	}
	if rk.Damage < 0 {
		return fmt.Errorf("jigglypuff.roll_kick.damage must not be negative, got %d", rk.Damage)
	}
	for name, r := range map[string]Reach{"combo.ground": gr.Reach, "combo.air": ar.Reach, "roll_kick": rk.Reach} {
		if r.MinHeight > r.MaxHeight {
			return fmt.Errorf("jigglypuff.%s.min_height must not exceed max_height", name)
		}
	}
	return nil
}
