package main

import "math"

// Chikorita's ultimate, Giant Vines (see GiantVines in config.go): a thick vine runs underground after the foe, and where it surfaces
// a ring of giant vines bursts out of the ground and brings itself down on the foe one after another, like a ring of Tree Slams that
// are all trunk and no canopy.

// maxGiantVines bounds the vines of one cast, whatever the config says.
const maxGiantVines = 40

// What a giant vine is doing (Ball.Phase).
const (
	vineWait  = iota // still underground, waiting for its turn (Ball.Delay)
	vineUp           // standing up, locked on the foe, its shape on the ground
	vineSwing        // coming down
	vineLie          // lying where it landed, until it is gone
)

// giantOut is whether slot i is still in the middle of a Giant Vines: its head is running or a vine has yet to land. It cannot move meanwhile.
func (g *Game) giantOut(i int) bool {
	for _, b := range g.Balls {
		if b.Owner == i && (b.K == ballGiantHead || (b.K == ballGiantVine && b.Phase != vineLie)) {
			return true
		}
	}
	return false
}

// stepGiantCast is Chikorita's Z: with a full charge, and nothing else going on, the press empties the charge and sends the head out. It says
// whether Chikorita is rooted by the skill this tick.
func (g *Game) stepGiantCast(i int, in Input, u Ultimate) ([]Event, bool) {
	p := &g.P[i]
	out := g.giantOut(i)
	if out || !in.Ult || p.Charge < u.Max || p.LeafPhase != leafNone || p.vineBusy() || p.Z != 0 || p.Held {
		return nil, out
	}
	p.Charge = 0
	dx, dy := unit(g.foeNearTo(i, p.X, p.Y).X-p.X, g.foeNearTo(i, p.X, p.Y).Y-p.Y)
	if dx == 0 && dy == 0 {
		dx, dy = float64(p.FX), float64(p.FY)
		dx, dy = unit(dx, dy)
	}
	g.Balls = append(g.Balls, Ball{K: ballGiantHead, Owner: i, Side: i + 1, X: p.X, Y: p.Y, DX: dx, DY: dy, Life: g.cfg.Chikorita.GiantVines.TerminalTime})
	return []Event{{T: "gvcast", X: p.X, Y: p.Y, K: p.Kind}}, true
}

// stepGiantHead runs the thick vine under the ground: straight at the nearest foe, never past it. It surfaces at the foe, once it touches it,
// or where it is when its time is up; then the vines are born (the second result) and the head is gone.
func (g *Game) stepGiantHead(b *Ball, dt float64) (alive bool, born []Ball, events []Event) {
	gv := g.cfg.Chikorita.GiantVines
	foe := g.foeNearTo(b.Owner, b.X, b.Y)
	dx, dy := foe.X-b.X, foe.Y-b.Y
	d := math.Hypot(dx, dy)
	if d > 1e-9 {
		step := math.Min(d, gv.FollowSpeed*dt)
		b.DX, b.DY = dx/d, dy/d
		b.X, b.Y = b.X+b.DX*step, b.Y+b.DY*step
	}
	cx, cy := b.X, b.Y
	touched := gv.Reach.hits(foe) && math.Hypot(foe.X-b.X, foe.Y-b.Y) <= gv.HeadRadius+g.radius(g.slotOf(foe))
	if b.Life -= dt; !touched && b.Life > 1e-9 {
		return true, nil, nil
	}
	if touched {
		cx, cy = foe.X, foe.Y // it comes up under the foe, which the ring then closes round
	}
	start := math.Atan2(b.DY, b.DX) + math.Pi // the first vine is where the head came from
	for n := 0; n < gv.Count && n < maxGiantVines; n++ {
		a := start + 2*math.Pi*float64(n)/float64(gv.Count)
		born = append(born, Ball{
			K: ballGiantVine, Owner: b.Owner, Side: b.Side, Idx: n, Phase: vineWait, Delay: float64(n) * gv.Interval,
			X: math.Min(math.Max(cx+math.Cos(a)*gv.RingRadius, 0), g.court.Length),
			Y: math.Min(math.Max(cy+math.Sin(a)*gv.RingRadius, 0), g.court.Width),
		})
	}
	return false, born, []Event{{T: "gvplant", X: cx, Y: cy, R: gv.RingRadius}}
}

// stepGiantVine runs one giant vine through its turns: it comes up and locks on the nearest foe as it is then, stands up with its shape on show,
// comes down, hits whoever is in the shape when it lands, lies there and goes. A turn of no length is passed in the same tick.
func (g *Game) stepGiantVine(b *Ball, dt float64) (alive bool, events []Event) {
	gv := g.cfg.Chikorita.GiantVines
	left := dt
	for {
		timer := &b.Life
		if b.Phase == vineWait {
			timer = &b.Delay
		}
		take := math.Min(left, *timer)
		*timer, left = *timer-take, left-take
		if *timer > 1e-9 {
			return true, events
		}
		switch b.Phase {
		case vineWait:
			foe := g.foeNearTo(b.Owner, b.X, b.Y)
			if ux, uy := unit(foe.X-b.X, foe.Y-b.Y); ux != 0 || uy != 0 {
				b.DX, b.DY = ux, uy
			} else {
				b.DX = 1
			}
			b.Phase, b.Life = vineUp, gv.ChargeDuration
			events = append(events, Event{T: "gvrise", X: b.X, Y: b.Y, DX: b.DX, DY: b.DY, R: gv.TrunkLength})
		case vineUp:
			b.Phase, b.Life = vineSwing, gv.SwingDuration
		case vineSwing:
			for _, j := range g.foes(b.Owner) {
				if foe := &g.P[j]; gv.Reach.hits(foe) && treeHits(b.X, b.Y, b.DX, b.DY, foe.X, foe.Y, g.radius(j), gv.TrunkLength, gv.TrunkWidth, 0, 0) {
					g.hit(foe, gv.Damage)
					g.stun(foe, gv.StunDuration)
				}
			}
			b.Phase, b.Life = vineLie, gv.Linger
			events = append(events, Event{T: "gvslam", X: b.X, Y: b.Y, DX: b.DX, DY: b.DY, R: gv.TrunkLength})
		default:
			return false, events
		}
	}
}
