package main

import (
	"testing"
	"time"
)

// Every character that comes out has one ultimate (a Z skill or a Gigantamax): once it has used it, its charge does not fill again.
func TestACharacterHasOnlyOneUltimateAndItsChargeStaysEmptyAfterIt(t *testing.T) {
	for _, kind := range []int{pikachu, ash, chikorita, rowlet, machamp, charizard, vulpix} {
		cfg := DefaultConfig()
		// a flood of charge, so that anything but the rule would fill it again at once
		for _, u := range []*Ultimate{&cfg.Pikachu.Ultimate, &cfg.Ash.Ultimate, &cfg.Chikorita.Ultimate, &cfg.Rowlet.Ultimate, &cfg.Machamp.Ultimate, &cfg.Charizard.Ultimate, &cfg.Vulpix.Ultimate} {
			u.GainPerSecond = 100000
		}
		g := duel(cfg, kind, snorlax)
		g.P[0].X, g.P[0].Y, g.P[1].X, g.P[1].Y = 500, 450, 900, 450
		g.P[1].HP = 100000
		if g.UltUsed[0] {
			t.Fatalf("%s: it starts the round having used its ultimate", NAMEOF[kind])
		}
		u, _ := cfg.ultimateOf(kind)
		g.P[0].Charge = u.Max
		g.P[0].In = Input{Ult: true, Mega: true}
		g.Step(dt)
		g.P[0].In = Input{}
		if !g.UltUsed[0] {
			t.Fatalf("%s: it started its ultimate and is not marked as having used it", NAMEOF[kind])
		}
		steps(g, secs(2))
		if g.P[0].Charge != 0 {
			t.Fatalf("%s: the charge is %v two seconds after its ultimate: it filled again", NAMEOF[kind], g.P[0].Charge)
		}
	}
}

// The count is each character's own: one that has used its ultimate does not stop the other from charging and using its.
func TestEachCharacterCountsItsOwnUltimate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Pikachu.Ultimate.GainPerSecond = 100000
	cfg.Vulpix.Ultimate.GainPerSecond = 100000
	g := duel(cfg, pikachu, vulpix)
	g.P[0].HP, g.P[1].HP = 100000, 100000
	g.P[0].Charge = cfg.Pikachu.Ultimate.Max
	g.P[0].In = Input{Ult: true}
	g.Step(dt)
	g.P[0].In = Input{}
	if !g.UltUsed[0] || g.UltUsed[1] {
		t.Fatalf("used %v: only slot 0 has used its ultimate", g.UltUsed)
	}
	g.Step(dt)
	if g.P[1].Charge != cfg.Vulpix.Ultimate.Max {
		t.Fatalf("the other character's charge is %v: it should fill as usual", g.P[1].Charge)
	}
	g.P[1].In = Input{Ult: true}
	g.Step(dt)
	if !g.UltUsed[1] {
		t.Fatal("the other character could not use its own ultimate")
	}
}

func TestANewRoundGivesEveryoneTheirUltimateBack(t *testing.T) {
	g := duel(DefaultConfig(), pikachu, snorlax)
	g.UltUsed = [maxSlots]bool{true, true}
	g.Reset()
	if g.UltUsed != [maxSlots]bool{} {
		t.Fatalf("used %v after a new round", g.UltUsed)
	}
}

// In a team match a character that wins stays out for the next bout, with the ultimate it has used; the one that comes in for the one that fell has its own.
func TestInATeamMatchTheSurvivorKeepsItsUsedUltimateAndTheNewcomerHasItsOwn(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{3, 2, 1, 0, 4, 5})
	now := time.Now()
	r.step(now)
	r.g.UltUsed = [maxSlots]bool{true, true} // both have used theirs in this bout
	knockOut(r, 1, now)                      // slot 1's falls, slot 0's wins
	r.step(now.Add(roundReset + time.Millisecond))
	if !r.g.UltUsed[0] {
		t.Fatal("the winner got its ultimate back though it never left the field")
	}
	if r.g.UltUsed[1] {
		t.Fatal("the character that came in for the fallen one starts with its ultimate used")
	}
}
