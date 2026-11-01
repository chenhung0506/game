package main

import "testing"

// ---- the left stick does not steer an aimed skill ----

func TestElectroBallAimedWithTheRightStickIgnoresTheLeftStickWhileItCharges(t *testing.T) {
	cfg := DefaultConfig()
	g := duel(cfg, pikachu, snorlax)
	g.P[0].X, g.P[0].Y = 600, 500
	g.P[1].X, g.P[1].Y = 1400, 500
	g.P[0].In = Input{Ball: true, CastX: 0, CastY: -1, AimX: 1, DX: 1}
	g.Step(dt)
	for n := 0; n < int(cfg.Pikachu.ElectroBall.ChargeTime*tickRate)+5 && len(g.Balls) == 0; n++ {
		g.P[0].In = Input{AimX: 1, DX: 1} // the left stick is held to the right all the while
		g.Step(dt)
	}
	if len(g.Balls) != 1 {
		t.Fatalf("%d balls, want the one that was thrown", len(g.Balls))
	}
	if b := g.Balls[0]; b.DX > 1e-9 || b.DY > -0.99 {
		t.Fatalf("ball goes %v,%v, want straight up as aimed, not along the left stick", b.DX, b.DY)
	}
}

func TestVineWhipAimedWithTheRightStickIgnoresTheLeftStick(t *testing.T) {
	g := duel(DefaultConfig(), chikorita, snorlax)
	g.P[0].X, g.P[0].Y = 600, 500
	g.P[1].X, g.P[1].Y = 1400, 500
	g.P[0].In = Input{Vine: true, CastX: 0, CastY: -1, AimX: 1}
	g.Step(dt)
	for n := 0; n < 10; n++ {
		g.P[0].In = Input{AimX: 1}
		g.Step(dt)
	}
	if p := g.P[0]; p.VineX != 0 || p.VineY != -1 {
		t.Fatalf("vines go %v,%v, want straight up as aimed", p.VineX, p.VineY)
	}
}
