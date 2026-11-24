package main

import "math"

// Snorlax's ultimate (Z, once the charge is full): Sleep. It gathers for `ring_duration`, then for `sleep_duration` seconds it sleeps, rooted and unable to use a skill, and after the first `delay` seconds of it gets back `health_hp` HP, a steady share each tick.
// Nothing that is done to it ends the sleep: a stun, a knock back, a blow that throws it up in the air, a freeze, a vine, a tornado all go on over it, and the sleep and the healing go on through them.

// sleepIdle is whether Snorlax is doing nothing that a sleep would cut across: it is on the ground and in none of its skills.
func (p *Player) sleepIdle() bool {
	return p.Z == 0 && !p.Held && p.FrozenT == 0 && p.StunT == 0 && p.BeamT == 0 && p.SweepT == 0 && p.RechargeT == 0 && p.PunchT == 0 && p.PunchLeft == 0 && p.PunchRecover == 0 && p.TailT == 0
}

// stepSleep runs Z for Snorlax: with a full charge the press empties it and Snorlax gathers for `ring_duration` (rooted, UltT, the rings closing in), then falls asleep; once `delay` seconds of the sleep have
// gone each tick of it heals. It says whether Snorlax is rooted (the gathering and all the time it sleeps).
func (g *Game) stepSleep(i int, in Input, u Ultimate, dt float64) ([]Event, bool) {
	p := &g.P[i]
	sl := g.cfg.Snorlax.Sleep
	var events []Event
	if p.UltT > 0 { // gathering: rooted until the time is up (a stun, a freeze or a catch ends it, see Step), and then it falls asleep
		if p.UltT -= dt; p.UltT >= 1e-9 {
			return nil, true
		}
		p.UltT = 0
		p.SleepT, p.SleepAcc = sl.SleepDuration, 0
		events = append(events, Event{T: "sleep", X: p.X, Y: p.Y, R: g.radius(i), K: p.Kind})
	} else if p.SleepT == 0 && in.Ult && p.Charge >= u.Max && p.sleepIdle() {
		p.Charge = 0
		if sl.RingDuration > 0 {
			p.UltT = sl.RingDuration
			return nil, true
		}
		p.SleepT, p.SleepAcc = sl.SleepDuration, 0
		events = append(events, Event{T: "sleep", X: p.X, Y: p.Y, R: g.radius(i), K: p.Kind})
	}
	if p.SleepT <= 0 {
		return events, false
	}
	// the healing happens over the last (sleep_duration - delay) seconds of the sleep: the part of this step that falls in it gets its share
	t0 := sl.SleepDuration - p.SleepT
	step := math.Min(dt, p.SleepT) // the last step is clipped, so what is given back is exactly health_hp
	if overlap := t0 + step - math.Max(t0, sl.Delay); overlap > 0 {
		p.SleepAcc += float64(sl.HealthHP) * overlap / (sl.SleepDuration - sl.Delay)
	}
	whole := int(p.SleepAcc + 1e-9)
	p.SleepAcc -= float64(whole)
	if p.SleepT -= step; p.SleepT < 1e-9 {
		p.SleepT = 0
		whole += int(math.Round(p.SleepAcc)) // what rounding left over
		p.SleepAcc = 0
		events = append(events, Event{T: "wake", X: p.X, Y: p.Y, R: g.radius(i), K: p.Kind})
	}
	g.heal(i, whole)
	return events, true
}

// heal gives slot i back n HP, up to its maximum. It is not damage taken back: nobody's ultimate charge counts it (gainCharge only counts HP that was lost).
func (g *Game) heal(i, n int) {
	if p := &g.P[i]; n > 0 && p.HP > 0 {
		p.HP = min(p.HP+n, g.maxHP(i))
	}
}
