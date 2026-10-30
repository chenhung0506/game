package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingFileUsesDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || cfg != DefaultConfig() {
		t.Fatalf("cfg=%+v err=%v, want defaults and no error", cfg, err)
	}
}

func TestPartialFileOverridesOnlyWhatItSets(t *testing.T) {
	p := writeConfig(t, `{"pikachu":{"volt_tackle":{"distance":300}},"snorlax":{"jump":{"height":100}}}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.Pikachu.VoltTackle.Distance, want.Snorlax.Jump.Height = 300, 100
	if cfg != want {
		t.Fatalf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	for name, body := range map[string]string{
		"misspelt key":                                 `{"pikachu":{"volt_tackle":{"distence":300}}}`,
		"zero distance":                                `{"pikachu":{"volt_tackle":{"distance":0}}}`,
		"negative duration":                            `{"pikachu":{"volt_tackle":{"duration":-0.1}}}`,
		"zero height":                                  `{"snorlax":{"jump":{"height":0}}}`,
		"zero jump time":                               `{"snorlax":{"jump":{"duration":0}}}`,
		"negative cooldown":                            `{"pikachu":{"volt_tackle":{"cooldown":-1}}}`,
		"zero charge time":                             `{"snorlax":{"hyper_beam":{"charge_duration":0}}}`,
		"negative recharge":                            `{"snorlax":{"hyper_beam":{"recharge_duration":-1}}}`,
		"zero beam damage":                             `{"snorlax":{"hyper_beam":{"damage":0}}}`,
		"zero beam length":                             `{"snorlax":{"hyper_beam":{"length":0}}}`,
		"zero beam width":                              `{"snorlax":{"hyper_beam":{"width":0}}}`,
		"misspelt beam key":                            `{"snorlax":{"hyper_beam":{"dammage":50}}}`,
		"zero sweep time":                              `{"snorlax":{"hyper_beam":{"sweep_duration":0}}}`,
		"negative angle":                               `{"snorlax":{"hyper_beam":{"sweep_angle":-1}}}`,
		"angle over 180":                               `{"snorlax":{"hyper_beam":{"sweep_angle":181}}}`,
		"angle as text":                                `{"snorlax":{"hyper_beam":{"sweep_angle":"30"}}}`,
		"zero thunder delay":                           `{"pikachu":{"thunder":{"delay":0}}}`,
		"zero thunder radius":                          `{"pikachu":{"thunder":{"radius":0}}}`,
		"zero thunder damage":                          `{"pikachu":{"thunder":{"damage":0}}}`,
		"zero thunder strikes":                         `{"pikachu":{"thunder":{"strikes":0}}}`,
		"fractional strikes":                           `{"pikachu":{"thunder":{"strikes":2.5}}}`,
		"negative interval":                            `{"pikachu":{"thunder":{"interval":-0.1}}}`,
		"negative follow speed":                        `{"pikachu":{"thunder":{"follow_speed":-1}}}`,
		"negative thunder cooldown":                    `{"pikachu":{"thunder":{"cooldown":-1}}}`,
		"zero ball charge":                             `{"pikachu":{"electro_ball":{"charge_time":0}}}`,
		"zero ball speed":                              `{"pikachu":{"electro_ball":{"speed":0}}}`,
		"zero ball radius":                             `{"pikachu":{"electro_ball":{"radius":0}}}`,
		"zero ball hp":                                 `{"pikachu":{"electro_ball":{"hp":0}}}`,
		"zero blast radius":                            `{"pikachu":{"electro_ball":{"explosion_radius":0}}}`,
		"zero blast damage":                            `{"pikachu":{"electro_ball":{"explosion_damage":0}}}`,
		"negative ball knockback":                      `{"pikachu":{"electro_ball":{"knockback":-1}}}`,
		"negative ball cooldown":                       `{"pikachu":{"electro_ball":{"cooldown":-1}}}`,
		"misspelt ball key":                            `{"pikachu":{"electro_ball":{"explosion_radious":5}}}`,
		"negative beam mass":                           `{"snorlax":{"hyper_beam":{"mass":-1}}}`,
		"negative ball mass":                           `{"pikachu":{"electro_ball":{"mass":-1}}}`,
		"zero volt uses":                               `{"pikachu":{"volt_tackle":{"max_uses":0}}}`,
		"zero volt window":                             `{"pikachu":{"volt_tackle":{"use_window":0}}}`,
		"negative paralysis":                           `{"pikachu":{"volt_tackle":{"paralyze_duration":-1}}}`,
		"negative volt damage":                         `{"pikachu":{"volt_tackle":{"damage":-1}}}`,
		"negative volt mass":                           `{"pikachu":{"volt_tackle":{"mass":-1}}}`,
		"negative toss mass":                           `{"charizard":{"seismic_toss":{"mass":-1}}}`,
		"volt band upside down":                        `{"pikachu":{"volt_tackle":{"min_height":50,"max_height":10}}}`,
		"old dash key":                                 `{"pikachu":{"dash":{"distance":100}}}`,
		"zero punch charge":                            `{"snorlax":{"ice_punch":{"charge_duration":0}}}`,
		"zero punch speed":                             `{"snorlax":{"ice_punch":{"speed":0}}}`,
		"zero punch distance":                          `{"snorlax":{"ice_punch":{"distance":0}}}`,
		"zero punch radius":                            `{"snorlax":{"ice_punch":{"radius":0}}}`,
		"zero freeze duration":                         `{"snorlax":{"ice_punch":{"freeze_duration":0}}}`,
		"negative punch recharge":                      `{"snorlax":{"ice_punch":{"recharge_duration":-1}}}`,
		"negative punch cooldown":                      `{"snorlax":{"ice_punch":{"cooldown":-1}}}`,
		"negative punch damage":                        `{"snorlax":{"ice_punch":{"damage":-1}}}`,
		"negative punch mass":                          `{"snorlax":{"ice_punch":{"mass":-1}}}`,
		"misspelt punch key":                           `{"snorlax":{"ice_punch":{"freeze_duraton":2}}}`,
		"negative knockback":                           `{"snorlax":{"hyper_beam":{"knockback":-1}}}`,
		"negative hit interval":                        `{"snorlax":{"hyper_beam":{"hit_interval":-0.1}}}`,
		"negative beam cooldown":                       `{"snorlax":{"hyper_beam":{"cooldown":-1}}}`,
		"zero slam radius":                             `{"snorlax":{"jump":{"slam_radius":0}}}`,
		"retired quake_radius key":                     `{"snorlax":{"quake_radius":100}}`,
		"old flat dash key":                            `{"pikachu":{"dash_distance":100}}`,
		"old flat jump key":                            `{"snorlax":{"jump_height":100}}`,
		"zero mobile court width":                      `{"court":{"mobile":{"width":0}}}`,
		"zero web court width":                         `{"court":{"web":{"width":0}}}`,
		"mobile court too short":                       `{"court":{"mobile":{"length":399}}}`,
		"web court too short":                          `{"court":{"web":{"length":399}}}`,
		"old flat court key":                           `{"court":{"width":600}}`,
		"zero pikachu radius":                          `{"pikachu":{"radius":0}}`,
		"negative snorlax radius":                      `{"snorlax":{"radius":-1}}`,
		"zero pikachu hp":                              `{"pikachu":{"max_hp":0}}`,
		"zero snorlax hp":                              `{"snorlax":{"max_hp":0}}`,
		"zero pikachu speed":                           `{"pikachu":{"walk_speed":0}}`,
		"negative snorlax speed":                       `{"snorlax":{"walk_speed":-5}}`,
		"fractional hp":                                `{"pikachu":{"max_hp":99.5}}`,
		"negative air speed rising":                    `{"snorlax":{"jump":{"air_speed_first_half":-1}}}`,
		"negative air speed falling":                   `{"snorlax":{"jump":{"air_speed_second_half":-1}}}`,
		"retired air_speed key":                        `{"snorlax":{"air_speed":100}}`,
		"negative jump cooldown":                       `{"snorlax":{"jump":{"cooldown":-1}}}`,
		"misspelt snorlax key":                         `{"snorlax":{"jump":{"air_spead":10}}}`,
		"misspelt thunder key":                         `{"pikachu":{"thunder":{"dealy":1}}}`,
		"charizard max_hp 0":                           `{"charizard":{"max_hp":0}}`,
		"charizard walk_speed 0":                       `{"charizard":{"walk_speed":0}}`,
		"charizard radius -1":                          `{"charizard":{"radius":-1}}`,
		"charizard fly duration 0":                     `{"charizard":{"fly":{"duration":0}}}`,
		"charizard fly speed 0":                        `{"charizard":{"fly":{"speed":0}}}`,
		"charizard fly height 0":                       `{"charizard":{"fly":{"height":0}}}`,
		"charizard fly cooldown -1":                    `{"charizard":{"fly":{"cooldown":-1}}}`,
		"charizard fire_breath speed 0":                `{"charizard":{"fire_breath":{"speed":0}}}`,
		"charizard fire_breath radius 0":               `{"charizard":{"fire_breath":{"radius":0}}}`,
		"charizard fire_breath hp 0":                   `{"charizard":{"fire_breath":{"hp":0}}}`,
		"charizard fire_breath duration 0":             `{"charizard":{"fire_breath":{"duration":0}}}`,
		"charizard fire_breath quantity 0":             `{"charizard":{"fire_breath":{"quantity":0}}}`,
		"charizard fire_breath length 0":               `{"charizard":{"fire_breath":{"length":0}}}`,
		"charizard fire_breath damage -1":              `{"charizard":{"fire_breath":{"damage":-1}}}`,
		"charizard fire_breath cooldown -1":            `{"charizard":{"fire_breath":{"cooldown":-1}}}`,
		"charizard fire_breath mass -1":                `{"charizard":{"fire_breath":{"mass":-1}}}`,
		"charizard fire_breath random_offset_angle -1": `{"charizard":{"fire_breath":{"random_offset_angle":-1}}}`,
		"charizard fire_breath random_offset_angle 91": `{"charizard":{"fire_breath":{"random_offset_angle":91}}}`,
		"charizard fire_breath quantity 2.5":           `{"charizard":{"fire_breath":{"quantity":2.5}}}`,
		"charizard seismic_toss speed 0":               `{"charizard":{"seismic_toss":{"speed":0}}}`,
		"charizard seismic_toss radius 0":              `{"charizard":{"seismic_toss":{"radius":0}}}`,
		"charizard seismic_toss length 0":              `{"charizard":{"seismic_toss":{"length":0}}}`,
		"charizard seismic_toss high 0":                `{"charizard":{"seismic_toss":{"high":0}}}`,
		"charizard seismic_toss damage -1":             `{"charizard":{"seismic_toss":{"damage":-1}}}`,
		"charizard seismic_toss cooldown -1":           `{"charizard":{"seismic_toss":{"cooldown":-1}}}`,
		"misspelt charizard key":                       `{"charizard":{"fire_breath":{"qantity":5}}}`,
		"misspelt fly key":                             `{"charizard":{"fly":{"durration":5}}}`,
		"not json":                                     `dash_distance = 3`,
		"wrong type":                                   `{"pikachu":{"volt_tackle":{"distance":"far"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, body)); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
}

func TestZeroCooldownAndRechargeAreAllowed(t *testing.T) {
	for _, body := range []string{
		`{"pikachu":{"volt_tackle":{"cooldown":0}}}`,
		`{"snorlax":{"hyper_beam":{"recharge_duration":0}}}`,
		`{"pikachu":{"thunder":{"cooldown":0}}}`,
		`{"charizard":{"fire_breath":{"random_offset_angle":0,"cooldown":0,"damage":0,"mass":0}}}`, // dead-straight, free, harmless fire
		`{"charizard":{"fly":{"cooldown":0},"seismic_toss":{"cooldown":0,"damage":0}}}`,
		`{"snorlax":{"jump":{"cooldown":0}}}`,
		`{"snorlax":{"jump":{"air_speed_first_half":0}}}`,  // no steering while rising
		`{"snorlax":{"jump":{"air_speed_second_half":0}}}`, // no steering while falling
		`{"snorlax":{"hyper_beam":{"cooldown":0}}}`,
		`{"snorlax":{"hyper_beam":{"mass":0}}}`, // weightless: never pushes a ball
		`{"pikachu":{"electro_ball":{"mass":0}}}`,
		`{"pikachu":{"electro_ball":{"knockback":0}}}`, // a blast that does not push
		`{"pikachu":{"electro_ball":{"cooldown":0}}}`,
		`{"snorlax":{"ice_punch":{"recharge_duration":0}}}`, // no recovery
		`{"snorlax":{"ice_punch":{"cooldown":0}}}`,
		`{"snorlax":{"ice_punch":{"damage":0}}}`, // a pure freeze
		`{"snorlax":{"ice_punch":{"mass":0}}}`,
		`{"pikachu":{"volt_tackle":{"paralyze_duration":0,"damage":0,"mass":0}}}`, // a harmless, weightless tackle
		`{"charizard":{"seismic_toss":{"mass":0}}}`,
		`{"snorlax":{"hyper_beam":{"knockback":0}}}`,     // no push
		`{"snorlax":{"hyper_beam":{"hit_interval":0}}}`,  // one hit per sweep
		`{"pikachu":{"thunder":{"interval":0}}}`,         // all bolts at once
		`{"pikachu":{"thunder":{"follow_speed":0}}}`,     // a shadow that stays put
		`{"snorlax":{"hyper_beam":{"sweep_angle":0}}}`,   // a fixed beam
		`{"snorlax":{"hyper_beam":{"sweep_angle":180}}}`, // a full circle
	} {
		if _, err := LoadConfig(writeConfig(t, body)); err != nil {
			t.Fatalf("%s should be valid: %v", body, err)
		}
	}
}

func TestNestedBeamValuesOverrideOnlyWhatTheyName(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `{"snorlax":{"hyper_beam":{"damage":99}}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.Snorlax.HyperBeam.Damage = 99
	if cfg != want {
		t.Fatalf("cfg = %+v, want %+v", cfg, want)
	}
}

// The checked-in config.json is what runs by default, so it must stay loadable.
func TestRepoConfigFileIsValid(t *testing.T) {
	if _, err := os.Stat("config.json"); err != nil { // LoadConfig alone would accept a missing file
		t.Fatal(err)
	}
	if _, err := LoadConfig("config.json"); err != nil {
		t.Fatal(err)
	}
}
