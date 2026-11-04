package main

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

// vulpixDuel is Vulpix (slot 0, at 700,250) against a Snorlax that does not walk off (slot 1, at 700+d,250), the skills ready.
func vulpixDuel(d float64) *Game {
	g := duel(DefaultConfig(), vulpix, snorlax)
	g.P[0].X, g.P[1].X = 700, 700+d
	g.P[1].JumpCD = 99
	return g
}

func TestTheVulpixBotOpensWithTheIceStormOnAnOpponentWithinItsReach(t *testing.T) {
	g := vulpixDuel(300)
	in, ok := firstAct(newBot(vulpix, 1), g, 0, 200)
	if !ok || !in.Storm || in.Skill || in.Freeze {
		t.Fatalf("decision %+v (pressed %v), want the Ice Storm", in, ok)
	}
}

func TestTheVulpixBotRidesTheIcebergAtAnOpponentThatIsFarAndWallsOneThatIsNear(t *testing.T) {
	g := vulpixDuel(600)
	g.P[0].StormCD = 99
	in, ok := firstAct(newBot(vulpix, 2), g, 0, 400)
	if !ok || !in.Skill || in.DX != 1 {
		t.Fatalf("decision %+v (pressed %v), want a ride at the opponent (+x)", in, ok)
	}
	g = vulpixDuel(250)
	g.P[0].StormCD, g.P[0].FrostCD = 99, 99
	in, ok = firstAct(newBot(vulpix, 3), g, 0, 600)
	if !ok || !in.Freeze || in.AimX <= 0 {
		t.Fatalf("decision %+v (pressed %v), want the Freeze Beam aimed at the opponent", in, ok)
	}
}

func TestTheVulpixBotSteersTheRideAndTheBeamAtTheOpponentAndPressesNothingMeanwhile(t *testing.T) {
	g := vulpixDuel(400)
	g.P[0].FrostT = 2 // riding
	b := newBot(vulpix, 4)
	for i := 0; i < 12; i++ {
		in := b.Decide(g, 0)
		if hasAct(in) || (in.DX == 0 && i >= 4) {
			t.Fatalf("riding: %+v, want the stick at the opponent and no key", in)
		}
	}
	g = vulpixDuel(400)
	g.P[0].FBeamT = 3 // the beam is out
	b = newBot(vulpix, 5)
	for i := 0; i < 12; i++ {
		in := b.Decide(g, 0)
		if hasAct(in) || (in.DX != 1 && i >= 4) {
			t.Fatalf("beam out: %+v, want the stick toward the opponent and no key", in)
		}
	}
}

func TestAVulpixBotFightsAndLandsBlowsOnAnOpponentThatStandsStillWithKeysOfItsOwnOnly(t *testing.T) {
	for seed := uint64(1); seed <= 3; seed++ {
		g := vulpixDuel(500)
		b := newBot(vulpix, seed)
		for range 30 * 30 {
			in := b.Decide(g, 0)
			if in.Thunder || in.Beam || in.Ball || in.Punch || in.Razor || in.Vine || in.Comet || in.Rock || in.Mega || in.Kick || in.Toss || in.GFire || in.SBall || in.Fire {
				t.Fatalf("Vulpix's bot pressed a key that is not its: %+v", in)
			}
			g.P[0].In = in
			g.Step(dt)
		}
		if g.P[1].HP >= g.maxHP(1) {
			t.Fatalf("seed %d: the bot did no harm in 30 seconds", seed)
		}
	}
}

func TestAGengarBotAndAVulpixBotFightItOutWithTheLiveNumbersEachUsingItsSkills(t *testing.T) {
	cfg := liveConfig(t)
	for seed := uint64(1); seed <= 3; seed++ {
		g := duel(cfg, gengar, vulpix)
		g.P[0].HP, g.P[1].HP = g.maxHP(0), g.maxHP(1)
		bots := [2]*Bot{newBot(gengar, seed), newBot(vulpix, seed+10)}
		var keys [2]int
		for range 60 * 30 {
			for me := range bots {
				in := bots[me].Decide(g, me)
				if hasAct(in) || in.Storm || in.Freeze {
					keys[me]++
				}
				g.P[me].In = in
			}
			g.Step(dt)
			if g.P[0].HP <= 0 || g.P[1].HP <= 0 {
				break
			}
		}
		if keys[0] == 0 || keys[1] == 0 {
			t.Fatalf("seed %d: skills pressed gengar %d vulpix %d, want both to fight", seed, keys[0], keys[1])
		}
		if g.P[0].HP == g.maxHP(0) && g.P[1].HP == g.maxHP(1) {
			t.Fatalf("seed %d: nobody was hurt in a minute", seed)
		}
	}
}

func TestAPersonCanPlayGengarOrVulpixAgainstTheComputerPlayingTheOther(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	for _, c := range []struct{ me, bot int }{{gengar, vulpix}, {vulpix, gengar}} {
		conn := dialPlay(t, srv, "computer")
		next(t, conn, "hi")
		if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"bot":%d}`, c.bot))); err != nil {
			t.Fatal(err)
		}
		pick(t, conn, c.me)
		for {
			ps := next(t, conn, "s")["p"].([]any)
			if int(ps[0].(map[string]any)["k"].(float64)) == c.me && int(ps[1].(map[string]any)["k"].(float64)) == c.bot {
				break
			}
		}
		conn.Close()
	}
}
