package main

import "testing"

const dt = 1.0 / tickRate

func TestSlamHitsGroundedOpponent(t *testing.T) {
	g := NewGame()
	g.P[0].X, g.P[0].Y = 300, 250
	g.P[1].X, g.P[1].Y = 330, 250 // inside slamRadius
	g.P[0].In.Jump = true

	hit := false
	for i := 0; i < tickRate*2 && !hit; i++ {
		g.Step(dt)
		hit = g.P[1].HP < maxHP
	}
	if g.P[1].HP != maxHP-slamDamage {
		t.Fatalf("opponent HP = %d, want %d", g.P[1].HP, maxHP-slamDamage)
	}
}

func TestSlamNeverDamagesWhileStillRising(t *testing.T) {
	g := NewGame()
	g.P[0].X, g.P[1].X = 300, 310
	g.P[0].Y, g.P[1].Y = 250, 250
	g.P[0].In.Jump = true
	g.Step(dt)
	if g.P[0].Z <= 0 || g.P[1].HP != maxHP {
		t.Fatalf("Z=%v HP=%d: damage must wait for the landing frame", g.P[0].Z, g.P[1].HP)
	}
}

func TestAirborneOpponentDodgesSlam(t *testing.T) {
	g := NewGame()
	g.P[0].X, g.P[1].X = 300, 310
	g.P[0].Y, g.P[1].Y = 250, 250
	g.P[0].Z, g.P[0].VZ = 1, -100 // lands on the next step
	g.P[1].Z, g.P[1].VZ = 100, 0  // in the air
	g.Step(dt)
	if g.P[0].Z != 0 {
		t.Fatal("setup: attacker should have landed")
	}
	if g.P[1].HP != maxHP {
		t.Fatalf("airborne opponent took damage: HP=%d", g.P[1].HP)
	}
}

func TestQuakeCannotReachAirborneButHitsGrounded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		z     float64
		wantH int
	}{
		{"airborne", 100, maxHP},
		{"grounded", 0, maxHP - quakeDamage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGame()
			g.P[0].X, g.P[1].X = 300, 350
			g.P[0].Y, g.P[1].Y = 250, 250
			g.P[1].Z, g.P[1].VZ = tc.z, 0
			g.P[0].In.Attack = true
			g.Step(dt)
			if g.P[1].HP != tc.wantH {
				t.Fatalf("HP = %d, want %d", g.P[1].HP, tc.wantH)
			}
		})
	}
}

func TestQuakeCooldownLimitsHits(t *testing.T) {
	g := NewGame()
	g.P[0].X, g.P[1].X = 300, 350
	g.P[0].Y, g.P[1].Y = 250, 250
	g.P[0].In.Attack = true
	for i := 0; i < 5; i++ { // 5 ticks = 0.17s, well inside the 0.8s cooldown
		g.Step(dt)
	}
	if g.P[1].HP != maxHP-quakeDamage {
		t.Fatalf("HP = %d, want exactly one hit", g.P[1].HP)
	}
}

func TestDiagonalMoveIsNotFaster(t *testing.T) {
	g := NewGame()
	x0, y0 := g.P[0].X, g.P[0].Y
	g.P[0].In.DX, g.P[0].In.DY = 1, 1
	g.Step(1)
	d := dist(&Player{X: x0, Y: y0}, &g.P[0])
	if d < speed-0.001 || d > speed+0.001 {
		t.Fatalf("diagonal distance = %v, want %v", d, speed)
	}
}

func TestWinner(t *testing.T) {
	g := NewGame()
	if g.Winner() != -1 {
		t.Fatal("no winner at start")
	}
	g.P[1].HP = 0
	if g.Winner() != 0 {
		t.Fatalf("winner = %d, want 0", g.Winner())
	}
}
