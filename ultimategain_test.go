package main

import "testing"

// The charge an ultimate gets from the damage it deals is set apart from the charge the other skills' damage gives (gain_dealt and gain_ultimate_dealt).

func TestAnUltimatesDamageGivesItsOwnRateOfChargeAndTheOthersTheirs(t *testing.T) {
	cfg := DefaultConfig()
	u := &cfg.Pikachu.Ultimate
	u.GainTaken, u.GainPerSecond, u.GainDealt, u.GainUltDealt = 0, 0, 1, 5
	for name, ult := range map[string]bool{"a skill's damage": false, "an ultimate's": true} {
		g := duel(cfg, pikachu, snorlax)
		hp0 := [maxSlots]int{g.P[0].HP, g.P[1].HP}
		g.ultNow = ult
		g.hit(&g.P[1], 10)
		g.ultNow = false
		g.gainCharge(hp0, dt)
		want := 10.0
		if ult {
			want = 50
		}
		if g.P[0].Charge != want {
			t.Fatalf("%s: charge %v, want %v", name, g.P[0].Charge, want)
		}
		if g.ultLost != [maxSlots]int{} {
			t.Fatal("the ultimate damage of this tick was not forgotten")
		}
	}
	// both in one tick: the 10 are a skill's and the 4 an ultimate's
	g := duel(cfg, pikachu, snorlax)
	hp0 := [maxSlots]int{g.P[0].HP, g.P[1].HP}
	g.hit(&g.P[1], 10)
	g.ultNow = true
	g.hit(&g.P[1], 4)
	g.ultNow = false
	g.gainCharge(hp0, dt)
	if want := 10*1.0 + 4*5; g.P[0].Charge != want {
		t.Fatalf("charge %v, want %v", g.P[0].Charge, want)
	}
}

func TestBraveBirdsDamageCountsAsAnUltimatesAndANormalSkillsDoesNot(t *testing.T) {
	cfg := DefaultConfig()
	u := &cfg.Rowlet.Ultimate
	u.GainTaken, u.GainPerSecond, u.GainDealt, u.GainUltDealt = 0, 0, 1, 3
	bb := &cfg.Rowlet.BraveBird
	bb.RingDuration, bb.ChargeDuration, bb.Damage, bb.LaunchHeight, bb.Tornado.Duration = 0.1, 0.1, 30, 0, 0
	g := bbGame(cfg, 300)
	gpress(g, Input{Ult: true})
	for n := 0; n < 300 && g.P[0].Charge < 1 && g.P[1].HP == 100000; n++ {
		g.Step(dt)
	}
	if g.P[1].HP != 100000-30 {
		t.Fatalf("the foe lost %d, want 30", 100000-g.P[1].HP)
	}
	if g.P[0].Charge != 30*3 {
		t.Fatalf("charge %v, want 30 x gain_ultimate_dealt (3) = 90 (the max is %v)", g.P[0].Charge, u.Max)
	}
}

func TestTheGigantamaxMachampsSkillsCountAsAnUltimatesAndTheNormalOnesDoNot(t *testing.T) {
	cfg := DefaultConfig()
	u := &cfg.Machamp.Ultimate
	u.GainTaken, u.GainPerSecond, u.GainDealt, u.GainUltDealt, u.Max = 0, 0, 1, 3, 1000
	cfg.Machamp.Uppercut.SlideDistance, cfg.Machamp.Uppercut.SlideDuration = 0, 0
	cfg.Machamp.Gigantamax.Uppercut = cfg.Machamp.Uppercut
	for _, gmax := range []bool{false, true} {
		g := kickDuel(cfg, 300)
		g.P[1].HP = 100000
		if gmax {
			g.P[0].GmaxT = 100
		}
		press(g, 0, Input{Skill: true})
		steps(g, 40)
		dealt := 100000 - g.P[1].HP
		if dealt <= 0 {
			t.Fatalf("gmax %v: the uppercut did nothing", gmax)
		}
		rate := 1.0
		if gmax {
			rate = 3
		}
		if want := float64(dealt) * rate; g.P[0].Charge != want {
			t.Fatalf("gmax %v: charge %v, want %v (%d damage at %v)", gmax, g.P[0].Charge, want, dealt, rate)
		}
	}
}

func TestTheUltimateDamageRateIsChecked(t *testing.T) {
	c := DefaultConfig()
	c.Ash.Ultimate.GainUltDealt = -1
	if c.validate() == nil {
		t.Fatal("a negative rate was accepted")
	}
	if DefaultConfig().Pikachu.Ultimate.GainUltDealt != DefaultConfig().Pikachu.Ultimate.GainDealt {
		t.Fatal("by default an ultimate's damage gives what any other does, so nothing changes until the config says otherwise")
	}
}
