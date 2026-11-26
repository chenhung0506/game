package main

import "math"

// Jigglypuff's Sing (see singCfg): a ring of music notes that chase the nearest foe and, by touching it, mark it until it falls asleep.

// maxNotes bounds the notes of one song, whatever the config says.
const maxNotes = 24

// sing lets the notes out round Jigglypuff slot i.
func (g *Game) sing(i int) []Event {
	p, sg := &g.P[i], g.cfg.Jigglypuff.Sing
	off := g.radius(i) + sg.Radius
	for n := 0; n < sg.Count && n < maxNotes; n++ {
		a := 2 * math.Pi * float64(n) / float64(sg.Count)
		ux, uy := math.Cos(a), math.Sin(a)
		g.Balls = append(g.Balls, Ball{K: ballNote, Owner: i, Side: i + 1, Idx: n % 3, X: p.X + ux*off, Y: p.Y + uy*off, BaseX: p.X + ux*off, BaseY: p.Y + uy*off, DX: ux, DY: uy, Left: math.Max(0, sg.RingRadius-off), HP: sg.HP, Life: sg.Duration})
	}
	return []Event{{T: "sing", X: p.X, Y: p.Y, R: sg.RingRadius, S: i}}
}

// noteTarget is the foe a note goes for: the nearest one that is awake (when they are all asleep, the nearest).
func (g *Game) noteTarget(owner int, x, y float64) *Player {
	var best *Player
	bd := math.Inf(1)
	for _, j := range g.foes(owner) {
		if f := &g.P[j]; f.DozeT <= 0 {
			if d := math.Hypot(f.X-x, f.Y-y); d < bd {
				best, bd = f, d
			}
		}
	}
	if best == nil {
		return g.foeNearTo(owner, x, y)
	}
	return best
}

// stepNote moves one note (flying out, then after the nearest foe) and spends it on the foe it touches. It says whether the note is still there.
func (g *Game) stepNote(b *Ball, dt float64) (bool, []Event) {
	if b.HP <= 0 {
		return false, nil
	}
	sg := g.cfg.Jigglypuff.Sing
	x0, y0 := b.X, b.Y
	if !b.Back {
		step := math.Min(sg.SpreadSpeed*dt, b.Left)
		b.BaseX, b.BaseY, b.Left = b.BaseX+b.DX*step, b.BaseY+b.DY*step, b.Left-step
		b.Back = b.Left <= 1e-9 // from here on it is on its way to the foe
		b.Travel += sg.SpreadSpeed * dt
	} else {
		foe := g.noteTarget(b.Owner, b.BaseX, b.BaseY)
		if dx, dy := foe.X-b.BaseX, foe.Y-b.BaseY; math.Hypot(dx, dy) > 1e-9 {
			d := math.Hypot(dx, dy)
			step := math.Min(d, sg.ConvergenceSpeed*dt)
			b.DX, b.DY = dx/d, dy/d
			b.BaseX, b.BaseY = b.BaseX+b.DX*step, b.BaseY+b.DY*step
		}
		b.Travel += sg.ConvergenceSpeed * dt // (the S goes on even when the straight path has got to the foe, so the note sweeps across it)
	}
	b.X, b.Y = b.BaseX, b.BaseY
	if sg.SwayWidth > 0 && sg.SwayLength > 0 { // weave to either side of the straight path, an S as it goes
		off := sg.SwayWidth * math.Sin(2*math.Pi*b.Travel/sg.SwayLength)
		b.X, b.Y = b.BaseX-b.DY*off, b.BaseY+b.DX*off
	}
	b.X, b.Y = math.Min(math.Max(b.X, 0), g.court.Length), math.Min(math.Max(b.Y, 0), g.court.Width)
	if foe, ok := g.touchedFoe(b.Owner, func(f *Player, rad float64) bool {
		return sg.Reach.hits(f) && segDist(f.X, f.Y, x0, y0, b.X, b.Y) <= sg.Radius+rad
	}); ok {
		return false, g.markFoe(foe, b.Idx)
	}
	if sg.Duration > 0 {
		if b.Life -= dt; b.Life <= 1e-9 {
			return false, nil
		}
	}
	return true, nil
}

// markFoe puts one more mark over f's head, and the foe to sleep once it has marks_to_sleep of them. A sleeping foe takes none (the note is spent for nothing).
func (g *Game) markFoe(f *Player, kind int) []Event {
	sg := g.cfg.Jigglypuff.Sing
	if f.DozeT > 0 {
		return nil
	}
	f.Marks++
	f.MarkT = sg.MarkDuration
	events := []Event{{T: "mark", X: f.X, Y: f.Y, K: kind}}
	if f.Marks >= sg.MarksToSleep {
		f.Marks, f.MarkT = 0, 0
		if g.stun(f, sg.SleepDuration) { // (a foe in the middle of a big skill's gathering cannot be put to sleep)
			f.DozeT = sg.SleepDuration
			events = append(events, Event{T: "doze", X: f.X, Y: f.Y})
		}
	}
	return events
}

// stepMarks runs out the marks over every player's head and the sleep they brought.
func (g *Game) stepMarks(dt float64) {
	for i := range g.P {
		p := &g.P[i]
		if p.DozeT > 0 {
			if p.DozeT -= dt; p.DozeT < 1e-9 {
				p.DozeT = 0
			}
		}
		if p.MarkT > 0 {
			if p.MarkT -= dt; p.MarkT < 1e-9 {
				p.MarkT, p.Marks = 0, 0
			}
		}
	}
}
