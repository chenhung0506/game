package main

import "testing"

// A round that ends while a character is Gigantamax must start the next one at the ordinary maximum, not at the Gigantamax one
// (the HP used to be taken before the new Player replaced the old, whose GmaxT still made it Gigantamax).
func TestResetWhileGigantamaxStartsAtTheOrdinaryMaxHP(t *testing.T) {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int{machamp, charizard, gengar} {
		g := duel(cfg, kind, pikachu)
		g.P[0].GmaxT = 10
		g.Reset()
		if want := g.cfg.maxHPOf(kind); g.P[0].HP != want || g.maxHP(0) != want {
			t.Errorf("kind %d: HP %d, max %d after Reset, want %d", kind, g.P[0].HP, g.maxHP(0), want)
		}
	}
}

// Machamp has neither the Tree Slam nor the Rock Tomb, whatever the config says about the Rock Tomb.
func TestMachampHasNoRockTombOrTreeSlam(t *testing.T) {
	cfg := DefaultConfig() // the default has the Rock Tomb enabled
	cfg.Machamp.RockTomb.Enabled = true
	g := duel(cfg, machamp, pikachu)
	g.P[0].In = Input{Rock: true, Skill: false}
	steps(g, 3)
	if g.P[0].RockT != 0 || g.P[0].RockCD != 0 {
		t.Fatalf("RockT %v RockCD %v: Machamp started a Rock Tomb", g.P[0].RockT, g.P[0].RockCD)
	}
	g.P[0].In = Input{Tree: true}
	steps(g, 3)
	if g.P[0].TreePhase != treeNone || g.P[0].TreeCD != 0 {
		t.Fatal("Machamp started a Tree Slam")
	}
}
