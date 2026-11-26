package main

import "testing"

// jigBotScene is a Jigglypuff (slot 0) with a Snorlax (slot 1) `gap` in front of it, the bot looking at a scene that does not change.
func jigBotScene(gap float64) (*Bot, *Game) {
	g := duel(DefaultConfig(), jigglypuff, snorlax)
	g.P[1].X, g.P[1].Y = g.P[0].X+gap, g.P[0].Y
	return newBot(jigglypuff, 5), g
}

func TestTheJigglypuffBotSlapsAFoeInMelee(t *testing.T) {
	b, g := jigBotScene(60)
	in, ok := firstAct(b, g, 0, 60)
	if !ok || !in.Skill {
		t.Fatalf("it should slap a foe in front of it: %+v (pressed %v)", in, ok)
	}
}

func TestTheJigglypuffBotRollsIntoAFoeAFewStepsOff(t *testing.T) {
	rolled := 0
	for seed := uint64(1); seed <= 20; seed++ {
		g := duel(DefaultConfig(), jigglypuff, snorlax)
		g.P[1].X, g.P[1].Y = g.P[0].X+170, g.P[0].Y
		b := newBot(jigglypuff, seed)
		if in, ok := firstAct(b, g, 0, 120); ok && in.Roll {
			rolled++
			if in.AimX < 0.9 {
				t.Fatalf("it should roll at the foe: aim (%v, %v)", in.AimX, in.AimY)
			}
		}
	}
	if rolled == 0 {
		t.Fatal("it never rolled at a foe 170 away")
	}
}

func TestTheJigglypuffBotSingsAsSoonAsTheSongIsCharged(t *testing.T) {
	b, g := jigBotScene(300)
	g.P[0].Charge = g.cfg.Jigglypuff.Ultimate.Max
	in, ok := firstAct(b, g, 0, 60)
	if !ok || !in.Ult {
		t.Fatalf("it should sing with the charge full and the foe in reach: %+v", in)
	}
}

func TestTheJigglypuffBotJumpsOnAFoeInTheAirFromTheAir(t *testing.T) {
	jumped := 0
	for seed := uint64(1); seed <= 30; seed++ {
		g := duel(DefaultConfig(), jigglypuff, pikachu)
		g.P[1].X, g.P[1].Y = g.P[0].X+90, g.P[0].Y
		g.P[0].Z, g.P[0].JpFly = 120, true
		g.P[1].Z = 100
		b := newBot(jigglypuff, seed)
		if in, ok := firstAct(b, g, 0, 120); ok && in.Jump {
			jumped++
		}
	}
	if jumped == 0 {
		t.Fatal("it never jumped on a foe in the air")
	}
}

func TestTheJigglypuffBotIsNotFooledByAFoeAsleepOrStunned(t *testing.T) {
	b, g := jigBotScene(60)
	g.P[0].StunT = 1 // it is the stunned one: it presses nothing
	if in, ok := firstAct(b, g, 0, 60); ok {
		t.Fatalf("a stunned bot pressed %+v", in)
	}
}

func TestAJigglypuffBotFightsWithItsOwnSkillsAgainstTheLiveNumbers(t *testing.T) {
	cfg := liveConfig(t)
	for seed := uint64(1); seed <= 3; seed++ {
		g := duel(cfg, jigglypuff, snorlax)
		g.P[0].X, g.P[1].X = 700, 1300
		g.Reset()
		bots := [2]*Bot{newBot(jigglypuff, seed), newBot(snorlax, seed+10)}
		keys := map[string]int{}
		for range 90 * 30 {
			for me := range bots {
				in := bots[me].Decide(g, me)
				if me == 0 {
					for name, on := range map[string]bool{"skill": in.Skill, "roll": in.Roll, "jump": in.Jump, "ult": in.Ult} {
						if on {
							keys[name]++
						}
					}
				}
				g.P[me].In = in
			}
			g.Step(dt)
			if g.P[0].HP <= 0 || g.P[1].HP <= 0 {
				break
			}
		}
		if keys["skill"] == 0 || keys["roll"] == 0 {
			t.Fatalf("seed %d: keys pressed %v, want it to slap and roll", seed, keys)
		}
		if g.P[1].HP == g.maxHP(1) {
			t.Fatalf("seed %d: the Jigglypuff bot did no harm in 90 s", seed)
		}
	}
}
