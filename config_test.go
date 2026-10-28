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
	p := writeConfig(t, `{"pikachu":{"dash":{"distance":300}},"snorlax":{"jump":{"height":100}}}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.Pikachu.Dash.Distance, want.Snorlax.Jump.Height = 300, 100
	if cfg != want {
		t.Fatalf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	for name, body := range map[string]string{
		"misspelt key":               `{"pikachu":{"dash":{"distence":300}}}`,
		"zero distance":              `{"pikachu":{"dash":{"distance":0}}}`,
		"negative duration":          `{"pikachu":{"dash":{"duration":-0.1}}}`,
		"zero height":                `{"snorlax":{"jump":{"height":0}}}`,
		"zero jump time":             `{"snorlax":{"jump":{"duration":0}}}`,
		"negative cooldown":          `{"pikachu":{"dash":{"cooldown":-1}}}`,
		"zero charge time":           `{"snorlax":{"hyper_beam":{"charge_duration":0}}}`,
		"negative recharge":          `{"snorlax":{"hyper_beam":{"recharge_duration":-1}}}`,
		"zero beam damage":           `{"snorlax":{"hyper_beam":{"damage":0}}}`,
		"zero beam length":           `{"snorlax":{"hyper_beam":{"length":0}}}`,
		"zero beam width":            `{"snorlax":{"hyper_beam":{"width":0}}}`,
		"misspelt beam key":          `{"snorlax":{"hyper_beam":{"dammage":50}}}`,
		"zero sweep time":            `{"snorlax":{"hyper_beam":{"sweep_duration":0}}}`,
		"negative angle":             `{"snorlax":{"hyper_beam":{"sweep_angle":-1}}}`,
		"angle over 180":             `{"snorlax":{"hyper_beam":{"sweep_angle":181}}}`,
		"angle as text":              `{"snorlax":{"hyper_beam":{"sweep_angle":"30"}}}`,
		"zero thunder delay":         `{"pikachu":{"thunder":{"delay":0}}}`,
		"zero thunder radius":        `{"pikachu":{"thunder":{"radius":0}}}`,
		"zero thunder damage":        `{"pikachu":{"thunder":{"damage":0}}}`,
		"zero thunder strikes":       `{"pikachu":{"thunder":{"strikes":0}}}`,
		"fractional strikes":         `{"pikachu":{"thunder":{"strikes":2.5}}}`,
		"negative interval":          `{"pikachu":{"thunder":{"interval":-0.1}}}`,
		"negative follow speed":      `{"pikachu":{"thunder":{"follow_speed":-1}}}`,
		"negative thunder cooldown":  `{"pikachu":{"thunder":{"cooldown":-1}}}`,
		"negative knockback":         `{"snorlax":{"hyper_beam":{"knockback":-1}}}`,
		"negative hit interval":      `{"snorlax":{"hyper_beam":{"hit_interval":-0.1}}}`,
		"negative beam cooldown":     `{"snorlax":{"hyper_beam":{"cooldown":-1}}}`,
		"zero slam radius":           `{"snorlax":{"jump":{"slam_radius":0}}}`,
		"retired quake_radius key":   `{"snorlax":{"quake_radius":100}}`,
		"old flat dash key":          `{"pikachu":{"dash_distance":100}}`,
		"old flat jump key":          `{"snorlax":{"jump_height":100}}`,
		"zero mobile court width":    `{"court":{"mobile":{"width":0}}}`,
		"zero web court width":       `{"court":{"web":{"width":0}}}`,
		"mobile court too short":     `{"court":{"mobile":{"length":399}}}`,
		"web court too short":        `{"court":{"web":{"length":399}}}`,
		"old flat court key":         `{"court":{"width":600}}`,
		"zero pikachu radius":        `{"pikachu":{"radius":0}}`,
		"negative snorlax radius":    `{"snorlax":{"radius":-1}}`,
		"zero pikachu hp":            `{"pikachu":{"max_hp":0}}`,
		"zero snorlax hp":            `{"snorlax":{"max_hp":0}}`,
		"zero pikachu speed":         `{"pikachu":{"walk_speed":0}}`,
		"negative snorlax speed":     `{"snorlax":{"walk_speed":-5}}`,
		"fractional hp":              `{"pikachu":{"max_hp":99.5}}`,
		"negative air speed rising":  `{"snorlax":{"jump":{"air_speed_first_half":-1}}}`,
		"negative air speed falling": `{"snorlax":{"jump":{"air_speed_second_half":-1}}}`,
		"retired air_speed key":      `{"snorlax":{"air_speed":100}}`,
		"negative jump cooldown":     `{"snorlax":{"jump":{"cooldown":-1}}}`,
		"misspelt snorlax key":       `{"snorlax":{"jump":{"air_spead":10}}}`,
		"misspelt thunder key":       `{"pikachu":{"thunder":{"dealy":1}}}`,
		"not json":                   `dash_distance = 3`,
		"wrong type":                 `{"pikachu":{"dash":{"distance":"far"}}}`,
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
		`{"pikachu":{"dash":{"cooldown":0}}}`,
		`{"snorlax":{"hyper_beam":{"recharge_duration":0}}}`,
		`{"pikachu":{"thunder":{"cooldown":0}}}`,
		`{"snorlax":{"jump":{"cooldown":0}}}`,
		`{"snorlax":{"jump":{"air_speed_first_half":0}}}`,  // no steering while rising
		`{"snorlax":{"jump":{"air_speed_second_half":0}}}`, // no steering while falling
		`{"snorlax":{"hyper_beam":{"cooldown":0}}}`,
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
