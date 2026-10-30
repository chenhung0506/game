package main

import "testing"

// clashConfig pins the projectile numbers the tests reason about (the values of config.json).
func clashConfig() Config {
	cfg := DefaultConfig()
	cfg.Chikorita.RazorLeaf.LeafHP, cfg.Chikorita.RazorLeaf.Damage = 1, 2
	cfg.Charizard.FireBreath.HP, cfg.Charizard.FireBreath.Damage = 5, 3
	cfg.Pikachu.ElectroBall.HP, cfg.Pikachu.ElectroBall.ContactDamage = 15, 10
	return cfg
}

// clashGame is a Pikachu / Charizard / Chikorita arena with the players far from where the test projectiles meet.
func clashGame() *Game {
	g := duel(clashConfig(), pikachu, chikorita)
	g.P[0].X, g.P[0].Y = 100, 100
	g.P[1].X, g.P[1].Y = 1100, 100
	return g
}

// leafAt is a leaf of owner o flying along dx (+1 or -1), fireAt a fire ball, ballAt an Electro Ball.
func leafAt(g *Game, o int, x, y, dx float64) Ball {
	return Ball{K: ballLeaf, Owner: o, X: x, Y: y, DX: dx, HP: g.cfg.Chikorita.RazorLeaf.LeafHP}
}
func fireAt(g *Game, o int, x, y, dx float64) Ball {
	return Ball{K: ballFire, Owner: o, X: x, Y: y, DX: dx, HP: g.cfg.Charizard.FireBreath.HP, Left: 500}
}
func ballAt(g *Game, o int, x, y, dx float64) Ball {
	return Ball{Owner: o, X: x, Y: y, DX: dx, HP: g.cfg.Pikachu.ElectroBall.HP}
}

func TestFastLeafAndFireBallHurtEachOtherEvenWhenTheyCrossInOneStep(t *testing.T) {
	g := clashGame()
	cfg := g.cfg
	// After this step the leaf is at 621.7 and the fire ball at 601.7: they have swapped sides, 20 apart,
	// which is more than the 18 of their radii. Only the swept check sees the clash.
	g.Balls = []Ball{leafAt(g, 0, 600, 300, 1), fireAt(g, 1, 610, 300, -1)}
	g.stepBalls(dt)
	if len(g.Balls) != 1 || g.Balls[0].K != ballFire {
		t.Fatalf("balls = %+v, want only the fire ball (the leaf had 1 HP)", g.Balls)
	}
	if want := cfg.Charizard.FireBreath.HP - cfg.Chikorita.RazorLeaf.Damage; g.Balls[0].HP != want {
		t.Fatalf("fire HP = %d, want %d (HP minus the leaf's damage)", g.Balls[0].HP, want)
	}
}

func TestEnoughLeavesBreakAFireBall(t *testing.T) {
	cfg := clashConfig()
	need := (cfg.Charizard.FireBreath.HP + cfg.Chikorita.RazorLeaf.Damage - 1) / cfg.Chikorita.RazorLeaf.Damage
	cfg.Chikorita.RazorLeaf.LeafHP = 1000 // the leaves survive, so only the fire ball's HP matters
	for _, n := range []int{need - 1, need} {
		g := duel(cfg, pikachu, chikorita)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 100, 100, 1100, 100
		g.Balls = []Ball{fireAt(g, 0, 600, 300, 1)}
		for k := 0; k < n; k++ {
			g.Balls = append(g.Balls, leafAt(g, 1, 640, 300, -1))
		}
		g.stepBalls(dt)
		fire := false
		for _, b := range g.Balls {
			fire = fire || b.K == ballFire
		}
		if broken := !fire; broken != (n == need) {
			t.Fatalf("%d leaves: fire ball broken = %v, want %v", n, broken, n == need)
		}
	}
}

func TestElectroBallBreaksFireBallAndLeafButSurvivesThem(t *testing.T) {
	g := clashGame()
	cfg := g.cfg
	g.Balls = []Ball{ballAt(g, 0, 600, 300, 1), fireAt(g, 1, 640, 300, -1), leafAt(g, 1, 640, 320, -1)}
	g.Balls[2].Y = 300
	g.stepBalls(dt)
	if len(g.Balls) != 1 || g.Balls[0].K != ballElectro {
		t.Fatalf("balls = %+v, want only the Electro Ball", g.Balls)
	}
	want := cfg.Pikachu.ElectroBall.HP - cfg.Charizard.FireBreath.Damage - cfg.Chikorita.RazorLeaf.Damage
	if g.Balls[0].HP != want {
		t.Fatalf("electro HP = %d, want %d", g.Balls[0].HP, want)
	}
}

func TestEnoughLeavesShootDownAnElectroBallWhichThenExplodes(t *testing.T) {
	cfg := clashConfig()
	eb := cfg.Pikachu.ElectroBall
	need := (eb.HP + cfg.Chikorita.RazorLeaf.Damage - 1) / cfg.Chikorita.RazorLeaf.Damage
	g := duel(cfg, pikachu, chikorita)
	g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 100, 100, 1100, 100
	g.Balls = []Ball{ballAt(g, 0, 600, 300, 1)}
	for k := 0; k < need; k++ {
		g.Balls = append(g.Balls, leafAt(g, 1, 640, 300, -1))
		g.Balls[k+1].HP = 1000
	}
	if ev := g.stepBalls(dt); len(ev) != 0 {
		t.Fatalf("exploded in the same step: %+v", ev)
	}
	if g.Balls[0].K != ballElectro || g.Balls[0].HP > 0 {
		t.Fatalf("balls[0] = %+v, want a spent Electro Ball waiting to explode", g.Balls[0])
	}
	ev := g.stepBalls(dt)
	if len(ev) == 0 || ev[0].T != "explode" {
		t.Fatalf("events next step = %+v, want the explosion", ev)
	}
}

func TestBallsOfTheSameOwnerAndWaitingLeavesDoNotClash(t *testing.T) {
	g := clashGame()
	g.Balls = []Ball{fireAt(g, 0, 600, 300, 1), leafAt(g, 0, 600, 300, 1)}
	g.stepBalls(dt)
	if len(g.Balls) != 2 || g.Balls[0].HP != g.cfg.Charizard.FireBreath.HP {
		t.Fatalf("same owner clashed: %+v", g.Balls)
	}
	wait := leafAt(g, 1, 600, 300, 0)
	wait.Wait, wait.Delay = true, 5
	g.Balls = []Ball{fireAt(g, 0, 600, 300, 1), wait}
	g.stepBalls(dt)
	if len(g.Balls) != 2 || g.Balls[0].HP != g.cfg.Charizard.FireBreath.HP || g.Balls[1].HP != wait.HP {
		t.Fatalf("a waiting leaf clashed: %+v", g.Balls)
	}
}

func TestBallClashDoesNotDependOnTheListOrder(t *testing.T) {
	run := func(swap bool) []Ball {
		g := clashGame()
		a, b := fireAt(g, 0, 600, 300, 1), leafAt(g, 1, 640, 300, -1)
		b.HP = 5
		if swap {
			g.Balls = []Ball{b, a}
		} else {
			g.Balls = []Ball{a, b}
		}
		g.stepBalls(dt)
		return g.Balls
	}
	x, y := run(false), run(true)
	if len(x) != len(y) {
		t.Fatalf("%+v vs %+v", x, y)
	}
	hp := map[int]int{}
	for _, b := range x {
		hp[b.K] = b.HP
	}
	for _, b := range y {
		if hp[b.K] != b.HP {
			t.Fatalf("%+v vs %+v", x, y)
		}
	}
}

func TestBallsPassingByWithoutTouchingDoNotClash(t *testing.T) {
	g := clashGame()
	g.Balls = []Ball{fireAt(g, 0, 600, 300, 1), leafAt(g, 1, 640, 360, -1)} // 60 apart sideways
	g.stepBalls(dt)
	if len(g.Balls) != 2 || g.Balls[0].HP != g.cfg.Charizard.FireBreath.HP {
		t.Fatalf("clashed from afar: %+v", g.Balls)
	}
}
