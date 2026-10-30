package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// CourtSize is the playing field in game units.
type CourtSize struct {
	Width  float64 `json:"width"`
	Length float64 `json:"length"`
}

// Reach is the band of heights in which a skill can hurt its target: a target whose height Z is between
// min_height and max_height (both included) is hit, anyone above or below it is not. Every attack has one;
// it is read as part of the skill's own block. Height 0 is the ground; the numbers are the same units as
// fly.height and jump.height.
type Reach struct {
	MinHeight float64 `json:"min_height"`
	MaxHeight float64 `json:"max_height"`
}

// hits is whether the skill can hurt p at the height it is at now.
func (r Reach) hits(p *Player) bool { return p.Z >= r.MinHeight && p.Z <= r.MaxHeight }

// Config holds the tuning values. Distances are in game units and times are in seconds.
type Config struct {
	// Length runs between the two players (left-right on the web page, up-down on a phone), width
	// across. A room takes the court of the view its first player opened it in.
	Court struct {
		Mobile CourtSize `json:"mobile"`
		Web    CourtSize `json:"web"`
	} `json:"court"`
	Pikachu struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		Dash      struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Cooldown float64 `json:"cooldown"`
		} `json:"dash"`
		Thunder struct {
			Reach
			Delay       float64 `json:"delay"`        // shadow appears -> first bolt
			Radius      float64 `json:"radius"`       // strike area
			Damage      int     `json:"damage"`       // per bolt
			Strikes     int     `json:"strikes"`      // bolts per cast
			Interval    float64 `json:"interval"`     // between bolts
			FollowSpeed float64 `json:"follow_speed"` // how fast the shadow chases the target; 0 = it stays put
			Cooldown    float64 `json:"cooldown"`     // before the next cast; counted from the start of this one
		} `json:"thunder"`
		// Electro Ball: Pikachu charges, then throws a ball straight at the opponent (the aim follows it). It explodes when it touches
		// the opponent, when it reaches a wall, or when its HP runs out (the opponent's skills hit it).
		ElectroBall struct {
			Reach                   // the body and the blast hit anyone within these heights; above or below, the ball flies past
			ChargeTime      float64 `json:"charge_time"`      // rooted while charging
			Speed           float64 `json:"speed"`            // of the ball
			Radius          float64 `json:"radius"`           // size of the ball; it touches the opponent when the bodies overlap
			ContactDamage   int     `json:"contact_damage"`   // taken by the opponent the ball flies into, on top of the blast that follows
			HP              int     `json:"hp"`               // opponent skills that hit the ball take this off; at 0 it explodes
			ExplosionRadius float64 `json:"explosion_radius"` // reach of the blast, from the ball's centre
			ExplosionDamage int     `json:"explosion_damage"`
			Knockback       float64 `json:"knockback"` // the blast pushes the target this far straight away from its centre
			Cooldown        float64 `json:"cooldown"`  // before the next ball; counted from the start of the charge
			Mass            float64 `json:"mass"`      // see hyper_beam.mass: a heavier beam pushes the ball back by the difference
		} `json:"electro_ball"`
	} `json:"pikachu"`
	Snorlax struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		Jump      struct {
			Reach              // the landing shock: who it hurts, by height
			Height     float64 `json:"height"`
			Duration   float64 `json:"duration"`
			Cooldown   float64 `json:"cooldown"`    // before the next jump; counted from take-off
			SlamRadius float64 `json:"slam_radius"` // reach of the landing shock; the earthquake effect is drawn at this size
			// Steering speed in the air (walking is walk_speed); 0 = no steering. The first half of
			// the jump is the rise, the second half the fall.
			AirSpeedFirstHalf  float64 `json:"air_speed_first_half"`
			AirSpeedSecondHalf float64 `json:"air_speed_second_half"`
		} `json:"jump"`
		// Ice Punch: charge (rooted, the aim follows the direction held), dash `distance` at `speed`, then
		// recharge (rooted). The opponent it touches is frozen solid for freeze_duration.
		IcePunch struct {
			Reach
			ChargeDuration   float64 `json:"charge_duration"`
			RechargeDuration float64 `json:"recharge_duration"`
			Cooldown         float64 `json:"cooldown"`        // before the next punch; counted from the start of the charge
			Speed            float64 `json:"speed"`           // of the dash
			Distance         float64 `json:"distance"`        // how far the dash goes
			Radius           float64 `json:"radius"`          // size of the fist: it touches a body when the two overlap along the dash
			FreezeDuration   float64 `json:"freeze_duration"` // how long a hit opponent cannot move or act
			Damage           int     `json:"damage"`          // on the opponent, and on an Electro Ball it hits
			Mass             float64 `json:"mass"`            // against an Electro Ball: it is pushed mass - electro_ball.mass along the dash (nothing if lighter)
		} `json:"ice_punch"`
		HyperBeam struct {
			Reach
			ChargeDuration   float64 `json:"charge_duration"`   // rooted and aim locked on the opponent; the warning fan shows
			SweepAngle       float64 `json:"sweep_angle"`       // degrees each side of the facing; the beam goes +angle -> -angle
			SweepDuration    float64 `json:"sweep_duration"`    // seconds the beam takes to cross that arc
			RechargeDuration float64 `json:"recharge_duration"` // rooted after the sweep
			Cooldown         float64 `json:"cooldown"`          // before the next beam; counted from the moment it is used
			Damage           int     `json:"damage"`            // per hit
			Knockback        float64 `json:"knockback"`         // each hit pushes the target this far straight away from Snorlax; 0 = none
			HitInterval      float64 `json:"hit_interval"`      // a target still in the beam is hit again this often; 0 = a sweep hits only once
			Mass             float64 `json:"mass"`              // against an Electro Ball: each hit pushes it mass - electro_ball.mass away (nothing if lighter)
			Length           float64 `json:"length"`            // reach (radius of the fan)
			Width            float64 `json:"width"`
		} `json:"hyper_beam"`
	} `json:"snorlax"`
	Charizard struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		// Fly: the main button lifts Charizard for `duration` seconds. It only moves faster. Whether a skill still
		// hits it is up to that skill's min_height / max_height against `height`.
		Fly struct {
			Duration float64 `json:"duration"`
			Cooldown float64 `json:"cooldown"` // starts when it has landed
			Speed    float64 `json:"speed"`    // walking speed while flying
			Height   float64 `json:"height"`
		} `json:"fly"`
		// Fire Breath: for `duration` seconds Charizard spits `quantity` fire balls, evenly spaced, each at the
		// opponent's position when it leaves the mouth, turned by a random angle within +-random_offset_angle.
		// It can walk or fly while breathing. A fire ball hurts the first thing it touches and is gone; the
		// opponent's skills wear its HP down, and it ends at `length` or at a wall.
		FireBreath struct {
			Reach
			Speed             float64 `json:"speed"`
			Radius            float64 `json:"radius"`
			HP                int     `json:"hp"`
			Damage            int     `json:"damage"`              // per fire ball that lands
			Cooldown          float64 `json:"cooldown"`            // counted from the start of the breath
			Mass              float64 `json:"mass"`                // see hyper_beam.mass: a heavier beam or punch pushes it back by the difference
			Duration          float64 `json:"duration"`            // seconds the breath lasts
			Quantity          int     `json:"quantity"`            // fire balls in one breath
			RandomOffsetAngle float64 `json:"random_offset_angle"` // degrees: each ball is turned by a random angle in [-this, +this]
			Length            float64 `json:"length"`              // how far a fire ball flies
		} `json:"fire_breath"`
		// Seismic Toss: Charizard, on fire, runs `length` along the aim at `speed`. The body it touches (within
		// `radius`) is caught and cannot move or act: Charizard flies up `high` with it, then slams it down for
		// `damage`, with the same shock as Snorlax's landing (drawn `radius` wide). The rise and the fall go at `speed`.
		SeismicToss struct {
			Reach            // who the run can catch, by height
			Speed    float64 `json:"speed"`
			Radius   float64 `json:"radius"`
			Damage   int     `json:"damage"`
			Cooldown float64 `json:"cooldown"` // counted from the start of the run
			Length   float64 `json:"length"`
			High     float64 `json:"high"`
		} `json:"seismic_toss"`
	} `json:"charizard"`
	Chikorita struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		// Leaf Slash (the main button): Chikorita hops `jump_distance` the way it is walking, `jump_duration` seconds
		// in the air at the top `jump_height` (no damage on landing), then whips the leaf on its head `slashes` times,
		// `slash_interval` seconds each: the first from the upper left to the lower right, the second from the upper
		// right to the lower left, and so on. Each swing carries it `slash_step` towards the opponent and hits once,
		// at the middle of the swing, anyone within `slash_radius` (plus their body).
		LeafSlash struct {
			Reach
			JumpDistance  float64 `json:"jump_distance"`
			JumpDuration  float64 `json:"jump_duration"`
			JumpHeight    float64 `json:"jump_height"`
			Slashes       int     `json:"slashes"`
			SlashInterval float64 `json:"slash_interval"`
			SlashStep     float64 `json:"slash_step"`
			SlashRadius   float64 `json:"slash_radius"`
			LeafScale     float64 `json:"leaf_scale"` // how many times bigger the leaf on its head is drawn during the skill; looks only
			Damage        int     `json:"damage"`     // per swing that lands
			Cooldown      float64 `json:"cooldown"`   // counted from the start of the hop
		} `json:"leaf_slash"`
	} `json:"chikorita"`
}

func DefaultConfig() Config {
	var c Config
	c.Court.Mobile = CourtSize{Width: arenaH, Length: arenaW}
	c.Court.Web = c.Court.Mobile
	c.Pikachu.MaxHP = maxHP
	c.Pikachu.WalkSpeed = speed
	c.Pikachu.Radius = pikachuRadius
	c.Pikachu.Dash.Distance = 220
	c.Pikachu.Dash.Duration = 0.2
	c.Pikachu.Dash.Cooldown = 0.6
	c.Pikachu.Thunder.Reach = anyHeight
	c.Pikachu.ElectroBall.Reach = groundReach
	c.Snorlax.Jump.Reach = groundReach
	c.Snorlax.IcePunch.Reach = groundReach
	c.Snorlax.HyperBeam.Reach = groundReach
	c.Charizard.FireBreath.Reach = anyHeight
	c.Charizard.SeismicToss.Reach = groundReach
	c.Pikachu.Thunder.Delay = 1.0
	c.Pikachu.Thunder.Radius = 60
	c.Pikachu.Thunder.Damage = 15
	c.Pikachu.Thunder.Strikes = 3
	c.Pikachu.Thunder.Interval = 0.5
	c.Pikachu.Thunder.FollowSpeed = 120
	c.Pikachu.Thunder.Cooldown = 4
	c.Pikachu.ElectroBall.ChargeTime = 0.5
	c.Pikachu.ElectroBall.Speed = 400
	c.Pikachu.ElectroBall.Radius = 14
	c.Pikachu.ElectroBall.HP = 30
	c.Pikachu.ElectroBall.ExplosionRadius = 90
	c.Pikachu.ElectroBall.ExplosionDamage = 25
	c.Pikachu.ElectroBall.Knockback = 120
	c.Pikachu.ElectroBall.Cooldown = 3
	c.Snorlax.MaxHP = maxHP
	c.Snorlax.WalkSpeed = speed
	c.Snorlax.Radius = snorlaxRadius
	c.Snorlax.Jump.Height = 72
	c.Snorlax.Jump.Duration = 0.8
	c.Snorlax.Jump.Cooldown = 1.5
	c.Snorlax.Jump.SlamRadius = slamRadius
	c.Snorlax.Jump.AirSpeedFirstHalf = speed // the same as walking, i.e. no change until it is tuned
	c.Snorlax.Jump.AirSpeedSecondHalf = speed
	c.Snorlax.IcePunch.ChargeDuration = 0.4
	c.Snorlax.IcePunch.RechargeDuration = 0.6
	c.Snorlax.IcePunch.Cooldown = 5
	c.Snorlax.IcePunch.Speed = 900
	c.Snorlax.IcePunch.Distance = 250
	c.Snorlax.IcePunch.Radius = 40
	c.Snorlax.IcePunch.FreezeDuration = 1.5
	c.Snorlax.IcePunch.Damage = 10
	c.Snorlax.IcePunch.Mass = 150
	c.Snorlax.HyperBeam.ChargeDuration = 0.6
	c.Snorlax.HyperBeam.SweepAngle = 30
	c.Snorlax.HyperBeam.SweepDuration = 0.5
	c.Snorlax.HyperBeam.RechargeDuration = 1.5
	c.Snorlax.HyperBeam.Cooldown = 5
	c.Snorlax.HyperBeam.Damage = 40
	c.Snorlax.HyperBeam.Length = 450
	c.Snorlax.HyperBeam.Width = 50
	c.Charizard.MaxHP = 120
	c.Charizard.WalkSpeed = 150
	c.Charizard.Radius = 26
	c.Charizard.Fly.Duration = 6
	c.Charizard.Fly.Cooldown = 8
	c.Charizard.Fly.Speed = 260
	c.Charizard.Fly.Height = 90
	c.Charizard.FireBreath.Speed = 450
	c.Charizard.FireBreath.Radius = 10
	c.Charizard.FireBreath.HP = 10
	c.Charizard.FireBreath.Damage = 2
	c.Charizard.FireBreath.Cooldown = 8
	c.Charizard.FireBreath.Mass = 20
	c.Charizard.FireBreath.Duration = 4
	c.Charizard.FireBreath.Quantity = 40
	c.Charizard.FireBreath.RandomOffsetAngle = 7.5
	c.Charizard.FireBreath.Length = 500
	c.Charizard.SeismicToss.Speed = 700
	c.Charizard.SeismicToss.Radius = 45
	c.Charizard.SeismicToss.Damage = 35
	c.Charizard.SeismicToss.Cooldown = 10
	c.Charizard.SeismicToss.Length = 300
	c.Charizard.SeismicToss.High = 180
	c.Chikorita.MaxHP = 110
	c.Chikorita.WalkSpeed = 170
	c.Chikorita.Radius = 18
	c.Chikorita.LeafSlash.Reach = groundReach
	c.Chikorita.LeafSlash.JumpDistance = 220
	c.Chikorita.LeafSlash.JumpDuration = 0.35
	c.Chikorita.LeafSlash.JumpHeight = 40
	c.Chikorita.LeafSlash.Slashes = 4
	c.Chikorita.LeafSlash.SlashInterval = 0.25
	c.Chikorita.LeafSlash.SlashStep = 40
	c.Chikorita.LeafSlash.SlashRadius = 90
	c.Chikorita.LeafSlash.LeafScale = 2.5
	c.Chikorita.LeafSlash.Damage = 12
	c.Chikorita.LeafSlash.Cooldown = 6
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
		{"court.mobile.width", c.Court.Mobile.Width, 0},
		{"court.web.width", c.Court.Web.Width, 0},
		{"pikachu.radius", c.Pikachu.Radius, 0},
		{"snorlax.radius", c.Snorlax.Radius, 0},
		{"pikachu.max_hp", float64(c.Pikachu.MaxHP), 0},
		{"pikachu.walk_speed", c.Pikachu.WalkSpeed, 0},
		{"pikachu.dash.distance", c.Pikachu.Dash.Distance, 0},
		{"pikachu.dash.duration", c.Pikachu.Dash.Duration, 0},
		{"pikachu.thunder.delay", c.Pikachu.Thunder.Delay, 0},
		{"pikachu.thunder.radius", c.Pikachu.Thunder.Radius, 0},
		{"pikachu.thunder.damage", float64(c.Pikachu.Thunder.Damage), 0},
		{"pikachu.thunder.strikes", float64(c.Pikachu.Thunder.Strikes), 0},
		{"pikachu.electro_ball.charge_time", c.Pikachu.ElectroBall.ChargeTime, 0},
		{"pikachu.electro_ball.speed", c.Pikachu.ElectroBall.Speed, 0},
		{"pikachu.electro_ball.radius", c.Pikachu.ElectroBall.Radius, 0},
		{"pikachu.electro_ball.hp", float64(c.Pikachu.ElectroBall.HP), 0},
		{"pikachu.electro_ball.explosion_radius", c.Pikachu.ElectroBall.ExplosionRadius, 0},
		{"pikachu.electro_ball.explosion_damage", float64(c.Pikachu.ElectroBall.ExplosionDamage), 0},
		{"snorlax.max_hp", float64(c.Snorlax.MaxHP), 0},
		{"snorlax.walk_speed", c.Snorlax.WalkSpeed, 0},
		{"snorlax.jump.height", c.Snorlax.Jump.Height, 0},
		{"snorlax.jump.duration", c.Snorlax.Jump.Duration, 0},
		{"snorlax.jump.slam_radius", c.Snorlax.Jump.SlamRadius, 0},
		{"snorlax.ice_punch.charge_duration", c.Snorlax.IcePunch.ChargeDuration, 0},
		{"snorlax.ice_punch.speed", c.Snorlax.IcePunch.Speed, 0},
		{"snorlax.ice_punch.distance", c.Snorlax.IcePunch.Distance, 0},
		{"snorlax.ice_punch.radius", c.Snorlax.IcePunch.Radius, 0},
		{"snorlax.ice_punch.freeze_duration", c.Snorlax.IcePunch.FreezeDuration, 0},
		{"snorlax.hyper_beam.charge_duration", c.Snorlax.HyperBeam.ChargeDuration, 0},
		{"snorlax.hyper_beam.sweep_duration", c.Snorlax.HyperBeam.SweepDuration, 0},
		{"snorlax.hyper_beam.damage", float64(c.Snorlax.HyperBeam.Damage), 0},
		{"snorlax.hyper_beam.length", c.Snorlax.HyperBeam.Length, 0},
		{"snorlax.hyper_beam.width", c.Snorlax.HyperBeam.Width, 0},
		{"charizard.max_hp", float64(c.Charizard.MaxHP), 0},
		{"charizard.walk_speed", c.Charizard.WalkSpeed, 0},
		{"charizard.radius", c.Charizard.Radius, 0},
		{"charizard.fly.duration", c.Charizard.Fly.Duration, 0},
		{"charizard.fly.speed", c.Charizard.Fly.Speed, 0},
		{"charizard.fly.height", c.Charizard.Fly.Height, 0},
		{"charizard.fire_breath.speed", c.Charizard.FireBreath.Speed, 0},
		{"charizard.fire_breath.radius", c.Charizard.FireBreath.Radius, 0},
		{"charizard.fire_breath.hp", float64(c.Charizard.FireBreath.HP), 0},
		{"charizard.fire_breath.duration", c.Charizard.FireBreath.Duration, 0},
		{"charizard.fire_breath.quantity", float64(c.Charizard.FireBreath.Quantity), 0},
		{"charizard.fire_breath.length", c.Charizard.FireBreath.Length, 0},
		{"charizard.seismic_toss.speed", c.Charizard.SeismicToss.Speed, 0},
		{"charizard.seismic_toss.radius", c.Charizard.SeismicToss.Radius, 0},
		{"charizard.seismic_toss.length", c.Charizard.SeismicToss.Length, 0},
		{"charizard.seismic_toss.high", c.Charizard.SeismicToss.High, 0},
		{"chikorita.max_hp", float64(c.Chikorita.MaxHP), 0},
		{"chikorita.walk_speed", c.Chikorita.WalkSpeed, 0},
		{"chikorita.radius", c.Chikorita.Radius, 0},
		{"chikorita.leaf_slash.jump_distance", c.Chikorita.LeafSlash.JumpDistance, 0},
		{"chikorita.leaf_slash.jump_duration", c.Chikorita.LeafSlash.JumpDuration, 0},
		{"chikorita.leaf_slash.slashes", float64(c.Chikorita.LeafSlash.Slashes), 0},
		{"chikorita.leaf_slash.slash_interval", c.Chikorita.LeafSlash.SlashInterval, 0},
		{"chikorita.leaf_slash.slash_radius", c.Chikorita.LeafSlash.SlashRadius, 0},
		{"chikorita.leaf_slash.leaf_scale", c.Chikorita.LeafSlash.LeafScale, 0},
	} {
		if !(f.v > f.min) { // also rejects NaN
			return fmt.Errorf("%s must be > %v, got %v", f.name, f.min, f.v)
		}
	}
	for name, l := range map[string]float64{"court.mobile.length": c.Court.Mobile.Length, "court.web.length": c.Court.Web.Length} {
		if !(l >= 2*spawnHalfGap) { // the players start spawnHalfGap either side of the middle
			return fmt.Errorf("%s must be >= %v so both players start inside it, got %v", name, 2*spawnHalfGap, l)
		}
	}
	for _, f := range []struct {
		name string
		v    float64
	}{
		{"pikachu.dash.cooldown", c.Pikachu.Dash.Cooldown},
		{"pikachu.thunder.interval", c.Pikachu.Thunder.Interval},
		{"pikachu.thunder.cooldown", c.Pikachu.Thunder.Cooldown},
		{"pikachu.thunder.follow_speed", c.Pikachu.Thunder.FollowSpeed},
		{"pikachu.electro_ball.contact_damage", float64(c.Pikachu.ElectroBall.ContactDamage)},
		{"pikachu.electro_ball.knockback", c.Pikachu.ElectroBall.Knockback},
		{"pikachu.electro_ball.cooldown", c.Pikachu.ElectroBall.Cooldown},
		{"pikachu.electro_ball.mass", c.Pikachu.ElectroBall.Mass},
		{"snorlax.hyper_beam.mass", c.Snorlax.HyperBeam.Mass},
		{"snorlax.jump.cooldown", c.Snorlax.Jump.Cooldown},
		{"snorlax.jump.air_speed_first_half", c.Snorlax.Jump.AirSpeedFirstHalf},
		{"snorlax.jump.air_speed_second_half", c.Snorlax.Jump.AirSpeedSecondHalf},
		{"snorlax.ice_punch.recharge_duration", c.Snorlax.IcePunch.RechargeDuration},
		{"snorlax.ice_punch.cooldown", c.Snorlax.IcePunch.Cooldown},
		{"snorlax.ice_punch.damage", float64(c.Snorlax.IcePunch.Damage)},
		{"snorlax.ice_punch.mass", c.Snorlax.IcePunch.Mass},
		{"snorlax.hyper_beam.cooldown", c.Snorlax.HyperBeam.Cooldown},
		{"snorlax.hyper_beam.knockback", c.Snorlax.HyperBeam.Knockback},
		{"snorlax.hyper_beam.hit_interval", c.Snorlax.HyperBeam.HitInterval},
		{"snorlax.hyper_beam.recharge_duration", c.Snorlax.HyperBeam.RechargeDuration},
		{"charizard.fly.cooldown", c.Charizard.Fly.Cooldown},
		{"charizard.fire_breath.damage", float64(c.Charizard.FireBreath.Damage)},
		{"charizard.fire_breath.cooldown", c.Charizard.FireBreath.Cooldown},
		{"charizard.fire_breath.mass", c.Charizard.FireBreath.Mass},
		{"charizard.seismic_toss.damage", float64(c.Charizard.SeismicToss.Damage)},
		{"charizard.seismic_toss.cooldown", c.Charizard.SeismicToss.Cooldown},
		{"chikorita.leaf_slash.jump_height", c.Chikorita.LeafSlash.JumpHeight},
		{"chikorita.leaf_slash.slash_step", c.Chikorita.LeafSlash.SlashStep},
		{"chikorita.leaf_slash.damage", float64(c.Chikorita.LeafSlash.Damage)},
		{"chikorita.leaf_slash.cooldown", c.Chikorita.LeafSlash.Cooldown},
	} {
		if !(f.v >= 0) {
			return fmt.Errorf("%s must be >= 0, got %v", f.name, f.v)
		}
	}
	for name, r := range map[string]Reach{
		"pikachu.thunder":        c.Pikachu.Thunder.Reach,
		"pikachu.electro_ball":   c.Pikachu.ElectroBall.Reach,
		"snorlax.jump":           c.Snorlax.Jump.Reach,
		"snorlax.ice_punch":      c.Snorlax.IcePunch.Reach,
		"snorlax.hyper_beam":     c.Snorlax.HyperBeam.Reach,
		"charizard.fire_breath":  c.Charizard.FireBreath.Reach,
		"charizard.seismic_toss": c.Charizard.SeismicToss.Reach,
		"chikorita.leaf_slash":   c.Chikorita.LeafSlash.Reach,
	} {
		if !(r.MinHeight >= 0 && r.MaxHeight >= r.MinHeight) { // also rejects NaN
			return fmt.Errorf("%s: need 0 <= min_height <= max_height, got %v and %v", name, r.MinHeight, r.MaxHeight)
		}
	}
	if a := c.Snorlax.HyperBeam.SweepAngle; !(a >= 0 && a <= 180) { // 0 = a fixed beam, 180 = a full circle
		return fmt.Errorf("snorlax.hyper_beam.sweep_angle must be between 0 and 180, got %v", a)
	}
	if a := c.Charizard.FireBreath.RandomOffsetAngle; !(a >= 0 && a <= 90) { // 0 = every fire ball flies straight at the target
		return fmt.Errorf("charizard.fire_breath.random_offset_angle must be between 0 and 90, got %v", a)
	}
	return nil
}

// maxHPOf, walkSpeedOf and radiusOf are a character's full HP, walking speed and body size.
func (c Config) maxHPOf(kind int) int {
	switch kind {
	case snorlax:
		return c.Snorlax.MaxHP
	case charizard:
		return c.Charizard.MaxHP
	case chikorita:
		return c.Chikorita.MaxHP
	}
	return c.Pikachu.MaxHP
}

func (c Config) walkSpeedOf(kind int) float64 {
	switch kind {
	case snorlax:
		return c.Snorlax.WalkSpeed
	case charizard:
		return c.Charizard.WalkSpeed
	case chikorita:
		return c.Chikorita.WalkSpeed
	}
	return c.Pikachu.WalkSpeed
}

func (c Config) radiusOf(kind int) float64 {
	switch kind {
	case snorlax:
		return c.Snorlax.Radius
	case charizard:
		return c.Charizard.Radius
	case chikorita:
		return c.Chikorita.Radius
	}
	return c.Pikachu.Radius
}

// A jump that peaks at h and lands after T seconds is a parabola with
// v0 = 4h/T and g = 8h/T^2.
func (c Config) jumpV() float64 { return 4 * c.Snorlax.Jump.Height / c.Snorlax.Jump.Duration }
func (c Config) gravity() float64 {
	return 8 * c.Snorlax.Jump.Height / (c.Snorlax.Jump.Duration * c.Snorlax.Jump.Duration)
}

// courtFor is the court of a room opened from the given view ("mobile" or "web").
func (c Config) courtFor(mode string) CourtSize {
	if mode == modeWeb {
		return c.Court.Web
	}
	return c.Court.Mobile
}
