package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Config holds the per-character tuning values. Distances are in game units
// (the arena is 800x500) and times are in seconds.
type Config struct {
	Pikachu struct {
		DashDistance float64 `json:"dash_distance"`
		DashDuration float64 `json:"dash_duration"`
		DashCooldown float64 `json:"dash_cooldown"`
		Thunder      struct {
			Delay       float64 `json:"delay"`        // shadow appears -> first bolt
			Radius      float64 `json:"radius"`       // strike area
			Damage      int     `json:"damage"`       // per bolt
			Strikes     int     `json:"strikes"`      // bolts per cast
			Interval    float64 `json:"interval"`     // between bolts
			FollowSpeed float64 `json:"follow_speed"` // how fast the shadow chases the target; 0 = it stays put
			Cooldown    float64 `json:"cooldown"`     // before the next cast; counted from the start of this one
		} `json:"thunder"`
	} `json:"pikachu"`
	Snorlax struct {
		JumpHeight   float64 `json:"jump_height"`
		JumpDuration float64 `json:"jump_duration"`
		HyperBeam    struct {
			ChargeDuration   float64 `json:"charge_duration"`   // rooted and aim locked on the opponent; the warning fan shows
			SweepAngle       float64 `json:"sweep_angle"`       // degrees each side of the facing; the beam goes +angle -> -angle
			SweepDuration    float64 `json:"sweep_duration"`    // seconds the beam takes to cross that arc
			RechargeDuration float64 `json:"recharge_duration"` // rooted after the sweep
			Cooldown         float64 `json:"cooldown"`          // before the next beam; counted from the moment it is used
			Damage           int     `json:"damage"`            // a sweep hits a target at most once
			Length           float64 `json:"length"`            // reach (radius of the fan)
			Width            float64 `json:"width"`
		} `json:"hyper_beam"`
	} `json:"snorlax"`
}

func DefaultConfig() Config {
	var c Config
	c.Pikachu.DashDistance = 220
	c.Pikachu.DashDuration = 0.2
	c.Pikachu.DashCooldown = 0.6
	c.Pikachu.Thunder.Delay = 1.0
	c.Pikachu.Thunder.Radius = 60
	c.Pikachu.Thunder.Damage = 15
	c.Pikachu.Thunder.Strikes = 3
	c.Pikachu.Thunder.Interval = 0.5
	c.Pikachu.Thunder.FollowSpeed = 120
	c.Pikachu.Thunder.Cooldown = 4
	c.Snorlax.JumpHeight = 72
	c.Snorlax.JumpDuration = 0.8
	c.Snorlax.HyperBeam.ChargeDuration = 0.6
	c.Snorlax.HyperBeam.SweepAngle = 30
	c.Snorlax.HyperBeam.SweepDuration = 0.5
	c.Snorlax.HyperBeam.RechargeDuration = 1.5
	c.Snorlax.HyperBeam.Cooldown = 5
	c.Snorlax.HyperBeam.Damage = 40
	c.Snorlax.HyperBeam.Length = 450
	c.Snorlax.HyperBeam.Width = 50
	return c
}

// LoadConfig reads path over the defaults, so a file may set only some values.
// A missing file means "use the defaults"; an unreadable or invalid one is an
// error, so a typo never silently falls back to different numbers.
func LoadConfig(path string) (Config, error) {
	c := DefaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // catches misspelt keys
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func (c Config) validate() error {
	for _, f := range []struct {
		name string
		v    float64
		min  float64 // exclusive lower bound
	}{
		{"pikachu.dash_distance", c.Pikachu.DashDistance, 0},
		{"pikachu.dash_duration", c.Pikachu.DashDuration, 0},
		{"pikachu.thunder.delay", c.Pikachu.Thunder.Delay, 0},
		{"pikachu.thunder.radius", c.Pikachu.Thunder.Radius, 0},
		{"pikachu.thunder.damage", float64(c.Pikachu.Thunder.Damage), 0},
		{"pikachu.thunder.strikes", float64(c.Pikachu.Thunder.Strikes), 0},
		{"snorlax.jump_height", c.Snorlax.JumpHeight, 0},
		{"snorlax.jump_duration", c.Snorlax.JumpDuration, 0},
		{"snorlax.hyper_beam.charge_duration", c.Snorlax.HyperBeam.ChargeDuration, 0},
		{"snorlax.hyper_beam.sweep_duration", c.Snorlax.HyperBeam.SweepDuration, 0},
		{"snorlax.hyper_beam.damage", float64(c.Snorlax.HyperBeam.Damage), 0},
		{"snorlax.hyper_beam.length", c.Snorlax.HyperBeam.Length, 0},
		{"snorlax.hyper_beam.width", c.Snorlax.HyperBeam.Width, 0},
	} {
		if !(f.v > f.min) { // also rejects NaN
			return fmt.Errorf("%s must be > %v, got %v", f.name, f.min, f.v)
		}
	}
	if !(c.Pikachu.DashCooldown >= 0) {
		return fmt.Errorf("pikachu.dash_cooldown must be >= 0, got %v", c.Pikachu.DashCooldown)
	}
	if !(c.Pikachu.Thunder.Interval >= 0) {
		return fmt.Errorf("pikachu.thunder.interval must be >= 0, got %v", c.Pikachu.Thunder.Interval)
	}
	if !(c.Pikachu.Thunder.Cooldown >= 0) {
		return fmt.Errorf("pikachu.thunder.cooldown must be >= 0, got %v", c.Pikachu.Thunder.Cooldown)
	}
	if !(c.Snorlax.HyperBeam.Cooldown >= 0) {
		return fmt.Errorf("snorlax.hyper_beam.cooldown must be >= 0, got %v", c.Snorlax.HyperBeam.Cooldown)
	}
	if !(c.Pikachu.Thunder.FollowSpeed >= 0) {
		return fmt.Errorf("pikachu.thunder.follow_speed must be >= 0, got %v", c.Pikachu.Thunder.FollowSpeed)
	}
	if a := c.Snorlax.HyperBeam.SweepAngle; !(a >= 0 && a <= 180) { // 0 = a fixed beam, 180 = a full circle
		return fmt.Errorf("snorlax.hyper_beam.sweep_angle must be between 0 and 180, got %v", a)
	}
	if !(c.Snorlax.HyperBeam.RechargeDuration >= 0) {
		return fmt.Errorf("snorlax.hyper_beam.recharge_duration must be >= 0, got %v", c.Snorlax.HyperBeam.RechargeDuration)
	}
	return nil
}

// A jump that peaks at h and lands after T seconds is a parabola with
// v0 = 4h/T and g = 8h/T^2.
func (c Config) jumpV() float64 { return 4 * c.Snorlax.JumpHeight / c.Snorlax.JumpDuration }
func (c Config) gravity() float64 {
	return 8 * c.Snorlax.JumpHeight / (c.Snorlax.JumpDuration * c.Snorlax.JumpDuration)
}
