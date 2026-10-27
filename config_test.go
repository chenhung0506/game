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
	p := writeConfig(t, `{"pikachu":{"dash_distance":300},"snorlax":{"jump_height":100}}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.Pikachu.DashDistance, want.Snorlax.JumpHeight = 300, 100
	if cfg != want {
		t.Fatalf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	for name, body := range map[string]string{
		"misspelt key":      `{"pikachu":{"dash_distence":300}}`,
		"zero distance":     `{"pikachu":{"dash_distance":0}}`,
		"negative duration": `{"pikachu":{"dash_duration":-0.1}}`,
		"zero height":       `{"snorlax":{"jump_height":0}}`,
		"zero jump time":    `{"snorlax":{"jump_duration":0}}`,
		"negative cooldown": `{"pikachu":{"dash_cooldown":-1}}`,
		"zero charge time":  `{"snorlax":{"hyper_beam":{"charge_duration":0}}}`,
		"negative recharge": `{"snorlax":{"hyper_beam":{"recharge_duration":-1}}}`,
		"zero beam damage":  `{"snorlax":{"hyper_beam":{"damage":0}}}`,
		"zero beam length":  `{"snorlax":{"hyper_beam":{"length":0}}}`,
		"zero beam width":   `{"snorlax":{"hyper_beam":{"width":0}}}`,
		"misspelt beam key": `{"snorlax":{"hyper_beam":{"dammage":50}}}`,
		"zero sweep time":   `{"snorlax":{"hyper_beam":{"sweep_duration":0}}}`,
		"negative angle":    `{"snorlax":{"hyper_beam":{"sweep_angle":-1}}}`,
		"angle over 180":    `{"snorlax":{"hyper_beam":{"sweep_angle":181}}}`,
		"angle as text":     `{"snorlax":{"hyper_beam":{"sweep_angle":"30"}}}`,
		"not json":          `dash_distance = 3`,
		"wrong type":        `{"pikachu":{"dash_distance":"far"}}`,
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
		`{"pikachu":{"dash_cooldown":0}}`,
		`{"snorlax":{"hyper_beam":{"recharge_duration":0}}}`,
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
