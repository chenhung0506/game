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

// HyperBeamCfg is a Hyper Beam: Snorlax has one (snorlax.hyper_beam) and Gyarados has its own (gyarados.hyper_beam), the
// same skill with numbers of its own.
type HyperBeamCfg struct {
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
}

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
		// Volt Tackle (the main button): Pikachu, wrapped in lightning, dashes `distance` in `duration` along the
		// direction held. It can be used `max_uses` times in a row; if they are not all spent within `use_window`
		// seconds of the first, the cooldown starts anyway. Touching the opponent hurts and paralyzes it (no
		// walking, no movement skills) for `paralyze_duration`, once per dash. `mass` decides clashes with another
		// skill that is running at the same time: the heavier one wins and the lighter one is cut short.
		VoltTackle struct {
			Reach
			Radius           float64 `json:"radius"` // size of the lightning: it touches a body when the two overlap along the dash; drawn at this size
			Distance         float64 `json:"distance"`
			Duration         float64 `json:"duration"`
			MaxUses          int     `json:"max_uses"`
			UseWindow        float64 `json:"use_window"`
			Cooldown         float64 `json:"cooldown"` // starts when the last use is spent or the window runs out
			ParalyzeDuration float64 `json:"paralyze_duration"`
			Damage           int     `json:"damage"`
			Mass             float64 `json:"mass"`
		} `json:"volt_tackle"`
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
		HyperBeam HyperBeamCfg `json:"hyper_beam"`
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
			Mass     float64 `json:"mass"` // against another skill running at the same time, see pikachu.volt_tackle.mass
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
		// Razor Leaf: Chikorita scatters `count` leaves around itself, each at a random distance from its body out to
		// `scatter_radius`. After a random delay between `min_delay` and `max_delay` seconds a leaf shoots at where the
		// opponent stands at that moment, at `speed`, and hurts it (`damage`) if it touches it. A leaf has `leaf_hp`
		// that the opponent's skills can wear down, and goes at a wall or when the opponent is hit.
		RazorLeaf struct {
			Reach
			Count         int     `json:"count"`
			ScatterRadius float64 `json:"scatter_radius"`
			MinDelay      float64 `json:"min_delay"`
			MaxDelay      float64 `json:"max_delay"`
			LeafHP        int     `json:"leaf_hp"`
			Speed         float64 `json:"speed"`
			Radius        float64 `json:"radius"`     // size of a leaf: it touches the opponent when the bodies overlap
			LeafScale     float64 `json:"leaf_scale"` // how many times bigger a leaf is drawn than its radius suggests; looks only
			Damage        int     `json:"damage"`
			Mass          float64 `json:"mass"`     // see hyper_beam.mass
			Cooldown      float64 `json:"cooldown"` // counted from the cast
		} `json:"razor_leaf"`
		// Vine Whip: two vines swing out of Chikorita along the direction it aims. One starts `sweep_angle` degrees to
		// the left of that line, the other as far to the right, and both swing to it in `sweep_duration` seconds, meeting
		// on the line where the attack ends. A vine is `length` long and `width` wide. The first vine to touch the
		// opponent hurts it and binds it for `bind_duration` seconds: it cannot walk or use a movement skill
		// (dash, jump, fly, Ice Punch, Seismic Toss, Leaf Slash). Chikorita is rooted while it swings.
		VineWhip struct {
			Reach
			ChargeDuration   float64 `json:"charge_duration"`   // the vines unfurl and the aim follows the stick; rooted
			RechargeDuration float64 `json:"recharge_duration"` // rooted after the swing, while the vines pull back
			Length           float64 `json:"length"`
			Width            float64 `json:"width"`
			SweepAngle       float64 `json:"sweep_angle"`
			SweepDuration    float64 `json:"sweep_duration"`
			Damage           int     `json:"damage"`
			BindDuration     float64 `json:"bind_duration"`
			Cooldown         float64 `json:"cooldown"` // counted from the start of the swing
		} `json:"vine_whip"`
	} `json:"chikorita"`
	Vulpix struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		// Frost Field (the main button): Vulpix rides an iceberg for `spray_duration` seconds, sliding at
		// `slide_speed` along the stick (or the way it faces) and unable to stop. The iceberg is `radius` wide and
		// leaves a trail of frost as wide, which stays `duration` seconds. A body whose centre is on the frost (at a
		// height inside min_height..max_height) walks `boost` faster on its owner's frost and `slow` slower (a
		// fraction, 0..1) on the other player's. When the iceberg touches the opponent it bursts: `contact_damage`,
		// then `explosion_damage` to whoever is within `explosion_radius` of it (the same two blows as the Electro
		// Ball); when the ride runs out it bursts the same way without the contact blow. The ride is over once it bursts,
		// and the burst leaves a patch of frost `explosion_radius` wide (it slows like the trail).
		FrostField struct {
			Reach
			SprayDuration   float64 `json:"spray_duration"` // how long the ride lasts
			Duration        float64 `json:"duration"`
			Slow            float64 `json:"slow"`
			Boost           float64 `json:"boost"`
			Radius          float64 `json:"radius"`
			Scale           float64 `json:"scale"` // how big the iceberg under Vulpix is drawn, times radius; looks only, the reach is radius
			SlideSpeed      float64 `json:"slide_speed"`
			ContactDamage   int     `json:"contact_damage"`
			ExplosionRadius float64 `json:"explosion_radius"` // from the iceberg's centre, plus the body's radius
			ExplosionDamage int     `json:"explosion_damage"`
			Cooldown        float64 `json:"cooldown"` // counted from the start of the ride
		} `json:"frost_field"`
		// Ice Storm: a storm appears where Vulpix stands and chases the opponent at `follow_speed`, like Thunder's
		// shadow. Every `interval` seconds, `strikes` times, it hits anyone within `radius` (plus their body) for
		// `damage`; every `freeze_hits`-th hit of the cast freezes the target for `freeze_duration`.
		IceStorm struct {
			Reach
			Radius         float64 `json:"radius"`
			Interval       float64 `json:"interval"`
			Damage         int     `json:"damage"`
			Strikes        int     `json:"strikes"`
			FollowSpeed    float64 `json:"follow_speed"`
			Cooldown       float64 `json:"cooldown"` // counted from the cast
			FreezeHits     int     `json:"freeze_hits"`
			FreezeDuration float64 `json:"freeze_duration"`
		} `json:"ice_storm"`
		// Freeze Beam: for `control_duration` seconds Vulpix stands still and breathes a beam from its mouth to a
		// spot on the ground that the stick moves at `speed`, never farther than `length` from the mouth. The beam
		// hurts nobody; where the spot passes, ice pillars rise (`pillar_radius` wide, `pillar_hp`, gone after
		// `pillar_duration`). The other player cannot walk through a pillar while at a height inside
		// min_height..max_height. Pillars are hurt by what already hurts a projectile.
		FreezeBeam struct {
			Reach
			Speed           float64 `json:"speed"`
			Cooldown        float64 `json:"cooldown"` // counted from the start of the beam
			ControlDuration float64 `json:"control_duration"`
			Length          float64 `json:"length"`
			StartDistance   float64 `json:"start_distance"` // where the beam starts, from the body along the aim: 0 at Vulpix, `length` at the far end
			PillarRadius    float64 `json:"pillar_radius"`
			PillarHP        int     `json:"pillar_hp"`
			PillarDuration  float64 `json:"pillar_duration"`
		} `json:"freeze_beam"`
	} `json:"vulpix"`
	Rowlet struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"`   // body size: drawn at this size, and it widens what skills can hit
		FlyHigh   float64 `json:"fly_high"` // Rowlet is always this high in the air: only skills whose min_height..max_height take it in can hurt it
		// Tailwind (the main button): Rowlet runs `speed` for `duration` seconds along a circle of `radius`, starting
		// along the direction held and curving towards the opponent's side, like a car skidding round. It covers
		// speed x duration / radius of the circle. It hurts nobody.
		Tailwind struct {
			Radius   float64 `json:"radius"`
			Speed    float64 `json:"speed"`
			Duration float64 `json:"duration"`
			Cooldown float64 `json:"cooldown"` // counted from the start of the drift
		} `json:"tailwind"`
		// Arrows: `count` arrows fan out at once, `angle` degrees apart and centred on the opponent, at `speed`. An
		// arrow that touches the opponent hurts it (`damage`) and slows it by the fraction `slow` (0..1) for
		// `slow_duration` seconds, and is spent. An arrow has `hp` that the opponent's skills can wear down.
		Arrows struct {
			Reach
			Speed        float64 `json:"speed"`
			Radius       float64 `json:"radius"` // size of an arrow: it touches the opponent when the bodies overlap
			HP           int     `json:"hp"`
			Damage       int     `json:"damage"`
			Count        int     `json:"count"`
			Angle        float64 `json:"angle"`
			SlowDuration float64 `json:"slow_duration"`
			Slow         float64 `json:"slow"`
			Cooldown     float64 `json:"cooldown"` // counted from the cast
			Mass         float64 `json:"mass"`     // see hyper_beam.mass
			// ChargeDuration is how long the volley is aimed before it leaves: Rowlet may walk and drift meanwhile, and the
			// arrows go from where it is at the end, at where the opponent then stands. Scale is how many times bigger an
			// arrow is drawn than its radius suggests; looks only.
			ChargeDuration float64 `json:"charge_duration"`
			Scale          float64 `json:"scale"`
		} `json:"arrows"`
		// Tornado: Rowlet charges (`charge_duration`, rooted), the tornado appears in front of it and Rowlet recovers
		// (`recharge_duration`, rooted). The tornado chases the opponent at `speed` for at most `distance` of travel or
		// `duration` seconds, and has `hp` that the opponent's skills can wear down. A tornado that touches the opponent
		// (within `radius`, at a height inside min_height..max_height) holds it: the opponent cannot move or act, is
		// carried up to `lift_height` and spun for `spin_duration` seconds, taking `spin_damage` every `spin_interval`,
		// then falls and takes `landing_damage` with the same shock as Snorlax's landing. Breaking the tornado lets it
		// fall at once.
		Tornado struct {
			Reach
			HP               int     `json:"hp"`
			Speed            float64 `json:"speed"`
			Radius           float64 `json:"radius"`
			Duration         float64 `json:"duration"`
			Distance         float64 `json:"distance"`
			ChargeDuration   float64 `json:"charge_duration"`
			RechargeDuration float64 `json:"recharge_duration"`
			SpinInterval     float64 `json:"spin_interval"`
			SpinDamage       int     `json:"spin_damage"`
			LandingDamage    int     `json:"landing_damage"`
			LiftHeight       float64 `json:"lift_height"`
			SpinDuration     float64 `json:"spin_duration"`
			Cooldown         float64 `json:"cooldown"` // counted from the start of the charge
			Mass             float64 `json:"mass"`     // see hyper_beam.mass: a heavier beam or punch pushes the tornado (and whoever it holds) by the difference
			Scale            float64 `json:"scale"`    // how many times bigger the funnel is drawn than `radius` (what it hits within); looks only
		} `json:"tornado"`
	} `json:"rowlet"`
	// Magikarp evolves into Gyarados (the red one): each hop it lands from counts, and the `evolve_hops`-th landing
	// evolves it, keeping the share of its HP it has left. The two have their own HP, speed and size.
	Magikarp struct {
		MaxHP      int     `json:"max_hp"`
		WalkSpeed  float64 `json:"walk_speed"`
		Radius     float64 `json:"radius"`
		EvolveHops int     `json:"evolve_hops"`
		// Jump (the main button, its only skill): it goes up `height` and comes down in `duration` seconds in all, hanging at
		// the top for `highest_point_duration` of them, out of reach of whatever cannot hit that high; landing hurts
		// nobody. It can be steered in the air at `air_speed_first_half` for the first half of the jump and
		// `air_speed_second_half` for the second. `cooldown` counts from the take-off.
		Jump struct {
			Height               float64 `json:"height"`
			Duration             float64 `json:"duration"`
			HighestPointDuration float64 `json:"highest_point_duration"`
			Cooldown             float64 `json:"cooldown"`
			AirSpeedFirstHalf    float64 `json:"air_speed_first_half"`
			AirSpeedSecondHalf   float64 `json:"air_speed_second_half"`
		} `json:"jump"`
	} `json:"magikarp"`
	Gyarados struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"`
		// Wave (the main button): Gyarados hops `jump_height` for `jump_duration` seconds, rooted, and the landing shakes
		// the ground: a wave runs out from it in every direction at `speed` up to `radius`, hurting the opponent
		// once (`damage`) if the front reaches it at a height inside min_height..max_height.
		Wave struct {
			Reach
			Damage       int     `json:"damage"`
			Radius       float64 `json:"radius"`
			Speed        float64 `json:"speed"`
			JumpHeight   float64 `json:"jump_height"`
			JumpDuration float64 `json:"jump_duration"`
			Cooldown     float64 `json:"cooldown"` // counted from the start of the hop
		} `json:"wave"`
		// Tail whip (K): the opponent in a fan `angle` degrees wide (centred on it), up to `length` away, is knocked back
		// `knockback` and stunned for `stun_duration` seconds (it can neither move nor act).
		TailWhip struct {
			Reach
			Angle        float64 `json:"angle"`
			Length       float64 `json:"length"`
			Knockback    float64 `json:"knockback"`
			StunDuration float64 `json:"stun_duration"`
			Cooldown     float64 `json:"cooldown"`
		} `json:"tail_whip"`
		// Its Hyper Beam (J): the same skill as Snorlax's, with numbers of its own.
		HyperBeam HyperBeamCfg `json:"hyper_beam"`
	} `json:"gyarados"`
}

func DefaultConfig() Config {
	var c Config
	c.Court.Mobile = CourtSize{Width: arenaH, Length: arenaW}
	c.Court.Web = c.Court.Mobile
	c.Pikachu.MaxHP = maxHP
	c.Pikachu.WalkSpeed = speed
	c.Pikachu.Radius = pikachuRadius
	c.Pikachu.VoltTackle.Reach = groundReach
	c.Pikachu.VoltTackle.Radius = pikachuRadius
	c.Pikachu.VoltTackle.Distance = 220
	c.Pikachu.VoltTackle.Duration = 0.2
	c.Pikachu.VoltTackle.MaxUses = 3
	c.Pikachu.VoltTackle.UseWindow = 2
	c.Pikachu.VoltTackle.Cooldown = 0.6
	c.Pikachu.VoltTackle.ParalyzeDuration = 1.5
	c.Pikachu.VoltTackle.Damage = 5
	c.Pikachu.VoltTackle.Mass = 60
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
	c.Charizard.SeismicToss.Mass = 100
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
	c.Chikorita.RazorLeaf.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	c.Chikorita.RazorLeaf.Count = 8
	c.Chikorita.RazorLeaf.ScatterRadius = 120
	c.Chikorita.RazorLeaf.MinDelay = 0.2
	c.Chikorita.RazorLeaf.MaxDelay = 1.0
	c.Chikorita.RazorLeaf.LeafHP = 5
	c.Chikorita.RazorLeaf.Speed = 600
	c.Chikorita.RazorLeaf.Radius = 10
	c.Chikorita.RazorLeaf.LeafScale = 1
	c.Chikorita.RazorLeaf.Damage = 8
	c.Chikorita.RazorLeaf.Mass = 20
	c.Chikorita.RazorLeaf.Cooldown = 8
	c.Chikorita.VineWhip.Reach = groundReach
	c.Chikorita.VineWhip.ChargeDuration = 0.4
	c.Chikorita.VineWhip.RechargeDuration = 0.5
	c.Chikorita.VineWhip.Length = 160
	c.Chikorita.VineWhip.Width = 24
	c.Chikorita.VineWhip.SweepAngle = 70
	c.Chikorita.VineWhip.SweepDuration = 0.35
	c.Chikorita.VineWhip.Damage = 10
	c.Chikorita.VineWhip.BindDuration = 2
	c.Chikorita.VineWhip.Cooldown = 5
	c.Vulpix.MaxHP = 100
	c.Vulpix.WalkSpeed = 160
	c.Vulpix.Radius = 14
	c.Vulpix.FrostField.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	c.Vulpix.FrostField.SprayDuration = 2
	c.Vulpix.FrostField.Duration = 6
	c.Vulpix.FrostField.Slow = 0.5
	c.Vulpix.FrostField.Boost = 0.5
	c.Vulpix.FrostField.Radius = 40
	c.Vulpix.FrostField.Scale = 1
	c.Vulpix.FrostField.SlideSpeed = 450
	c.Vulpix.FrostField.ContactDamage = 10
	c.Vulpix.FrostField.ExplosionRadius = 120
	c.Vulpix.FrostField.ExplosionDamage = 15
	c.Vulpix.FrostField.Cooldown = 10
	c.Vulpix.IceStorm.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	c.Vulpix.IceStorm.Radius = 80
	c.Vulpix.IceStorm.Interval = 0.7
	c.Vulpix.IceStorm.Damage = 8
	c.Vulpix.IceStorm.Strikes = 6
	c.Vulpix.IceStorm.FollowSpeed = 70
	c.Vulpix.IceStorm.Cooldown = 9
	c.Vulpix.IceStorm.FreezeHits = 3
	c.Vulpix.IceStorm.FreezeDuration = 1.5
	c.Vulpix.FreezeBeam.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	c.Vulpix.FreezeBeam.Speed = 250
	c.Vulpix.FreezeBeam.Cooldown = 12
	c.Vulpix.FreezeBeam.ControlDuration = 3
	c.Vulpix.FreezeBeam.Length = 450
	c.Vulpix.FreezeBeam.StartDistance = 225 // half the length
	c.Vulpix.FreezeBeam.PillarRadius = 24
	c.Vulpix.FreezeBeam.PillarHP = 25
	c.Vulpix.FreezeBeam.PillarDuration = 8
	c.Rowlet.MaxHP = 100
	c.Rowlet.WalkSpeed = 150
	c.Rowlet.Radius = 13
	c.Rowlet.FlyHigh = 50
	c.Rowlet.Tailwind.Radius = 120
	c.Rowlet.Tailwind.Speed = 600
	c.Rowlet.Tailwind.Duration = 0.5
	c.Rowlet.Tailwind.Cooldown = 4
	c.Rowlet.Arrows.Reach = anyHeight
	c.Rowlet.Arrows.Speed = 500
	c.Rowlet.Arrows.Radius = 8
	c.Rowlet.Arrows.HP = 2
	c.Rowlet.Arrows.Damage = 6
	c.Rowlet.Arrows.Count = 5
	c.Rowlet.Arrows.Angle = 12
	c.Rowlet.Arrows.SlowDuration = 2
	c.Rowlet.Arrows.Slow = 0.4
	c.Rowlet.Arrows.Cooldown = 6
	c.Rowlet.Arrows.Mass = 20
	c.Rowlet.Arrows.ChargeDuration = 0.5
	c.Rowlet.Arrows.Scale = 1
	c.Rowlet.Tornado.Reach = anyHeight
	c.Rowlet.Tornado.HP = 40
	c.Rowlet.Tornado.Speed = 90
	c.Rowlet.Tornado.Radius = 50
	c.Rowlet.Tornado.Duration = 8
	c.Rowlet.Tornado.Distance = 500
	c.Rowlet.Tornado.ChargeDuration = 0.4
	c.Rowlet.Tornado.RechargeDuration = 0.4
	c.Rowlet.Tornado.SpinInterval = 0.5
	c.Rowlet.Tornado.SpinDamage = 4
	c.Rowlet.Tornado.LandingDamage = 20
	c.Rowlet.Tornado.LiftHeight = 200
	c.Rowlet.Tornado.SpinDuration = 3
	c.Rowlet.Tornado.Cooldown = 12
	c.Rowlet.Tornado.Mass = 100
	c.Rowlet.Tornado.Scale = 1

	c.Magikarp.MaxHP, c.Magikarp.WalkSpeed, c.Magikarp.Radius, c.Magikarp.EvolveHops = 70, 150, 10, 5
	c.Magikarp.Jump.Height, c.Magikarp.Jump.Duration, c.Magikarp.Jump.HighestPointDuration, c.Magikarp.Jump.Cooldown = 200, 2, 1, 3
	c.Magikarp.Jump.AirSpeedFirstHalf, c.Magikarp.Jump.AirSpeedSecondHalf = 200, 100
	c.Gyarados.MaxHP, c.Gyarados.WalkSpeed, c.Gyarados.Radius = 170, 130, 36
	c.Gyarados.HyperBeam = c.Snorlax.HyperBeam // to start from: Snorlax's, which can then be told apart
	c.Gyarados.Wave.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	c.Gyarados.Wave.Damage, c.Gyarados.Wave.Radius, c.Gyarados.Wave.Speed = 12, 260, 500
	c.Gyarados.Wave.JumpHeight, c.Gyarados.Wave.JumpDuration, c.Gyarados.Wave.Cooldown = 40, 0.4, 6
	c.Gyarados.TailWhip.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	c.Gyarados.TailWhip.Angle, c.Gyarados.TailWhip.Length, c.Gyarados.TailWhip.Knockback = 90, 180, 200
	c.Gyarados.TailWhip.StunDuration, c.Gyarados.TailWhip.Cooldown = 1.2, 6
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
		{"pikachu.volt_tackle.radius", c.Pikachu.VoltTackle.Radius, 0},
		{"pikachu.volt_tackle.distance", c.Pikachu.VoltTackle.Distance, 0},
		{"pikachu.volt_tackle.duration", c.Pikachu.VoltTackle.Duration, 0},
		{"pikachu.volt_tackle.max_uses", float64(c.Pikachu.VoltTackle.MaxUses), 0},
		{"pikachu.volt_tackle.use_window", c.Pikachu.VoltTackle.UseWindow, 0},
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
		{"gyarados.hyper_beam.charge_duration", c.Gyarados.HyperBeam.ChargeDuration, 0},
		{"snorlax.hyper_beam.sweep_duration", c.Snorlax.HyperBeam.SweepDuration, 0},
		{"gyarados.hyper_beam.sweep_duration", c.Gyarados.HyperBeam.SweepDuration, 0},
		{"snorlax.hyper_beam.damage", float64(c.Snorlax.HyperBeam.Damage), 0},
		{"gyarados.hyper_beam.damage", float64(c.Gyarados.HyperBeam.Damage), 0},
		{"snorlax.hyper_beam.length", c.Snorlax.HyperBeam.Length, 0},
		{"gyarados.hyper_beam.length", c.Gyarados.HyperBeam.Length, 0},
		{"snorlax.hyper_beam.width", c.Snorlax.HyperBeam.Width, 0},
		{"gyarados.hyper_beam.width", c.Gyarados.HyperBeam.Width, 0},
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
		{"vulpix.max_hp", float64(c.Vulpix.MaxHP), 0},
		{"vulpix.walk_speed", c.Vulpix.WalkSpeed, 0},
		{"vulpix.radius", c.Vulpix.Radius, 0},
		{"vulpix.frost_field.spray_duration", c.Vulpix.FrostField.SprayDuration, 0},
		{"vulpix.frost_field.duration", c.Vulpix.FrostField.Duration, 0},
		{"vulpix.frost_field.radius", c.Vulpix.FrostField.Radius, 0},
		{"vulpix.frost_field.slide_speed", c.Vulpix.FrostField.SlideSpeed, 0},
		{"vulpix.frost_field.scale", c.Vulpix.FrostField.Scale, 0},
		{"vulpix.ice_storm.radius", c.Vulpix.IceStorm.Radius, 0},
		{"vulpix.ice_storm.strikes", float64(c.Vulpix.IceStorm.Strikes), 0},
		{"vulpix.ice_storm.freeze_hits", float64(c.Vulpix.IceStorm.FreezeHits), 0},
		{"vulpix.freeze_beam.speed", c.Vulpix.FreezeBeam.Speed, 0},
		{"vulpix.freeze_beam.control_duration", c.Vulpix.FreezeBeam.ControlDuration, 0},
		{"vulpix.freeze_beam.length", c.Vulpix.FreezeBeam.Length, 0},
		{"vulpix.freeze_beam.pillar_radius", c.Vulpix.FreezeBeam.PillarRadius, 0},
		{"vulpix.freeze_beam.pillar_hp", float64(c.Vulpix.FreezeBeam.PillarHP), 0},
		{"vulpix.freeze_beam.pillar_duration", c.Vulpix.FreezeBeam.PillarDuration, 0},
		{"rowlet.max_hp", float64(c.Rowlet.MaxHP), 0},
		{"rowlet.walk_speed", c.Rowlet.WalkSpeed, 0},
		{"rowlet.radius", c.Rowlet.Radius, 0},
		{"magikarp.max_hp", float64(c.Magikarp.MaxHP), 0},
		{"magikarp.walk_speed", c.Magikarp.WalkSpeed, 0},
		{"magikarp.radius", c.Magikarp.Radius, 0},
		{"magikarp.evolve_hops", float64(c.Magikarp.EvolveHops), 0},
		{"magikarp.jump.height", c.Magikarp.Jump.Height, 0},
		{"magikarp.jump.duration", c.Magikarp.Jump.Duration, 0},
		{"magikarp.jump.air_speed_first_half", c.Magikarp.Jump.AirSpeedFirstHalf, 0},
		{"magikarp.jump.air_speed_second_half", c.Magikarp.Jump.AirSpeedSecondHalf, 0},
		{"gyarados.max_hp", float64(c.Gyarados.MaxHP), 0},
		{"gyarados.walk_speed", c.Gyarados.WalkSpeed, 0},
		{"gyarados.radius", c.Gyarados.Radius, 0},
		{"gyarados.wave.radius", c.Gyarados.Wave.Radius, 0},
		{"gyarados.wave.speed", c.Gyarados.Wave.Speed, 0},
		{"gyarados.wave.jump_duration", c.Gyarados.Wave.JumpDuration, 0},
		{"gyarados.tail_whip.length", c.Gyarados.TailWhip.Length, 0},
		{"rowlet.fly_high", c.Rowlet.FlyHigh, 0},
		{"rowlet.tailwind.radius", c.Rowlet.Tailwind.Radius, 0},
		{"rowlet.tailwind.speed", c.Rowlet.Tailwind.Speed, 0},
		{"rowlet.tailwind.duration", c.Rowlet.Tailwind.Duration, 0},
		{"rowlet.arrows.speed", c.Rowlet.Arrows.Speed, 0},
		{"rowlet.arrows.radius", c.Rowlet.Arrows.Radius, 0},
		{"rowlet.arrows.hp", float64(c.Rowlet.Arrows.HP), 0},
		{"rowlet.arrows.scale", c.Rowlet.Arrows.Scale, 0},
		{"rowlet.tornado.scale", c.Rowlet.Tornado.Scale, 0},
		{"rowlet.arrows.count", float64(c.Rowlet.Arrows.Count), 0},
		{"rowlet.tornado.hp", float64(c.Rowlet.Tornado.HP), 0},
		{"rowlet.tornado.speed", c.Rowlet.Tornado.Speed, 0},
		{"rowlet.tornado.radius", c.Rowlet.Tornado.Radius, 0},
		{"rowlet.tornado.duration", c.Rowlet.Tornado.Duration, 0},
		{"rowlet.tornado.distance", c.Rowlet.Tornado.Distance, 0},
		{"rowlet.tornado.spin_interval", c.Rowlet.Tornado.SpinInterval, 0}, // 0 would never end the spin's damage loop
		{"rowlet.tornado.lift_height", c.Rowlet.Tornado.LiftHeight, 0},
		{"rowlet.tornado.spin_duration", c.Rowlet.Tornado.SpinDuration, 0},
		{"chikorita.walk_speed", c.Chikorita.WalkSpeed, 0},
		{"chikorita.radius", c.Chikorita.Radius, 0},
		{"chikorita.leaf_slash.jump_distance", c.Chikorita.LeafSlash.JumpDistance, 0},
		{"chikorita.leaf_slash.jump_duration", c.Chikorita.LeafSlash.JumpDuration, 0},
		{"chikorita.leaf_slash.slashes", float64(c.Chikorita.LeafSlash.Slashes), 0},
		{"chikorita.leaf_slash.slash_interval", c.Chikorita.LeafSlash.SlashInterval, 0},
		{"chikorita.leaf_slash.slash_radius", c.Chikorita.LeafSlash.SlashRadius, 0},
		{"chikorita.leaf_slash.leaf_scale", c.Chikorita.LeafSlash.LeafScale, 0},
		{"chikorita.razor_leaf.count", float64(c.Chikorita.RazorLeaf.Count), 0},
		{"chikorita.razor_leaf.scatter_radius", c.Chikorita.RazorLeaf.ScatterRadius, 0},
		{"chikorita.razor_leaf.leaf_hp", float64(c.Chikorita.RazorLeaf.LeafHP), 0},
		{"chikorita.razor_leaf.speed", c.Chikorita.RazorLeaf.Speed, 0},
		{"chikorita.razor_leaf.radius", c.Chikorita.RazorLeaf.Radius, 0},
		{"chikorita.razor_leaf.leaf_scale", c.Chikorita.RazorLeaf.LeafScale, 0},
		{"chikorita.vine_whip.length", c.Chikorita.VineWhip.Length, 0},
		{"chikorita.vine_whip.width", c.Chikorita.VineWhip.Width, 0},
		{"chikorita.vine_whip.sweep_duration", c.Chikorita.VineWhip.SweepDuration, 0},
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
		{"pikachu.volt_tackle.cooldown", c.Pikachu.VoltTackle.Cooldown},
		{"pikachu.volt_tackle.paralyze_duration", c.Pikachu.VoltTackle.ParalyzeDuration},
		{"pikachu.volt_tackle.damage", float64(c.Pikachu.VoltTackle.Damage)},
		{"pikachu.volt_tackle.mass", c.Pikachu.VoltTackle.Mass},
		{"charizard.seismic_toss.mass", c.Charizard.SeismicToss.Mass},
		{"pikachu.thunder.interval", c.Pikachu.Thunder.Interval},
		{"pikachu.thunder.cooldown", c.Pikachu.Thunder.Cooldown},
		{"pikachu.thunder.follow_speed", c.Pikachu.Thunder.FollowSpeed},
		{"pikachu.electro_ball.contact_damage", float64(c.Pikachu.ElectroBall.ContactDamage)},
		{"pikachu.electro_ball.knockback", c.Pikachu.ElectroBall.Knockback},
		{"pikachu.electro_ball.cooldown", c.Pikachu.ElectroBall.Cooldown},
		{"pikachu.electro_ball.mass", c.Pikachu.ElectroBall.Mass},
		{"snorlax.hyper_beam.mass", c.Snorlax.HyperBeam.Mass},
		{"gyarados.hyper_beam.mass", c.Gyarados.HyperBeam.Mass},
		{"snorlax.jump.cooldown", c.Snorlax.Jump.Cooldown},
		{"snorlax.jump.air_speed_first_half", c.Snorlax.Jump.AirSpeedFirstHalf},
		{"snorlax.jump.air_speed_second_half", c.Snorlax.Jump.AirSpeedSecondHalf},
		{"snorlax.ice_punch.recharge_duration", c.Snorlax.IcePunch.RechargeDuration},
		{"snorlax.ice_punch.cooldown", c.Snorlax.IcePunch.Cooldown},
		{"snorlax.ice_punch.damage", float64(c.Snorlax.IcePunch.Damage)},
		{"snorlax.ice_punch.mass", c.Snorlax.IcePunch.Mass},
		{"snorlax.hyper_beam.cooldown", c.Snorlax.HyperBeam.Cooldown},
		{"gyarados.hyper_beam.cooldown", c.Gyarados.HyperBeam.Cooldown},
		{"snorlax.hyper_beam.knockback", c.Snorlax.HyperBeam.Knockback},
		{"gyarados.hyper_beam.knockback", c.Gyarados.HyperBeam.Knockback},
		{"snorlax.hyper_beam.hit_interval", c.Snorlax.HyperBeam.HitInterval},
		{"gyarados.hyper_beam.hit_interval", c.Gyarados.HyperBeam.HitInterval},
		{"snorlax.hyper_beam.recharge_duration", c.Snorlax.HyperBeam.RechargeDuration},
		{"gyarados.hyper_beam.recharge_duration", c.Gyarados.HyperBeam.RechargeDuration},
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
		{"chikorita.razor_leaf.min_delay", c.Chikorita.RazorLeaf.MinDelay},
		{"chikorita.razor_leaf.damage", float64(c.Chikorita.RazorLeaf.Damage)},
		{"chikorita.razor_leaf.mass", c.Chikorita.RazorLeaf.Mass},
		{"chikorita.razor_leaf.cooldown", c.Chikorita.RazorLeaf.Cooldown},
		{"chikorita.vine_whip.charge_duration", c.Chikorita.VineWhip.ChargeDuration},
		{"chikorita.vine_whip.recharge_duration", c.Chikorita.VineWhip.RechargeDuration},
		{"chikorita.vine_whip.damage", float64(c.Chikorita.VineWhip.Damage)},
		{"chikorita.vine_whip.bind_duration", c.Chikorita.VineWhip.BindDuration},
		{"chikorita.vine_whip.cooldown", c.Chikorita.VineWhip.Cooldown},
		{"vulpix.frost_field.boost", c.Vulpix.FrostField.Boost},
		{"vulpix.frost_field.cooldown", c.Vulpix.FrostField.Cooldown},
		{"vulpix.ice_storm.interval", c.Vulpix.IceStorm.Interval},
		{"vulpix.ice_storm.damage", float64(c.Vulpix.IceStorm.Damage)},
		{"vulpix.ice_storm.follow_speed", c.Vulpix.IceStorm.FollowSpeed},
		{"vulpix.ice_storm.cooldown", c.Vulpix.IceStorm.Cooldown},
		{"vulpix.ice_storm.freeze_duration", c.Vulpix.IceStorm.FreezeDuration},
		{"vulpix.freeze_beam.cooldown", c.Vulpix.FreezeBeam.Cooldown},
		{"rowlet.tailwind.cooldown", c.Rowlet.Tailwind.Cooldown},
		{"rowlet.arrows.damage", float64(c.Rowlet.Arrows.Damage)},
		{"rowlet.arrows.angle", c.Rowlet.Arrows.Angle},
		{"rowlet.arrows.slow_duration", c.Rowlet.Arrows.SlowDuration},
		{"rowlet.arrows.cooldown", c.Rowlet.Arrows.Cooldown},
		{"rowlet.arrows.mass", c.Rowlet.Arrows.Mass},
		{"rowlet.arrows.charge_duration", c.Rowlet.Arrows.ChargeDuration},
		{"rowlet.tornado.charge_duration", c.Rowlet.Tornado.ChargeDuration},
		{"rowlet.tornado.recharge_duration", c.Rowlet.Tornado.RechargeDuration},
		{"rowlet.tornado.spin_damage", float64(c.Rowlet.Tornado.SpinDamage)},
		{"rowlet.tornado.landing_damage", float64(c.Rowlet.Tornado.LandingDamage)},
		{"rowlet.tornado.cooldown", c.Rowlet.Tornado.Cooldown},
		{"rowlet.tornado.mass", c.Rowlet.Tornado.Mass},
		{"vulpix.freeze_beam.start_distance", c.Vulpix.FreezeBeam.StartDistance},
		{"vulpix.frost_field.contact_damage", float64(c.Vulpix.FrostField.ContactDamage)},
		{"magikarp.jump.highest_point_duration", c.Magikarp.Jump.HighestPointDuration},
		{"magikarp.jump.cooldown", c.Magikarp.Jump.Cooldown},
		{"gyarados.wave.damage", float64(c.Gyarados.Wave.Damage)},
		{"gyarados.wave.jump_height", c.Gyarados.Wave.JumpHeight},
		{"gyarados.wave.cooldown", c.Gyarados.Wave.Cooldown},
		{"gyarados.tail_whip.knockback", c.Gyarados.TailWhip.Knockback},
		{"gyarados.tail_whip.stun_duration", c.Gyarados.TailWhip.StunDuration},
		{"gyarados.tail_whip.cooldown", c.Gyarados.TailWhip.Cooldown},
		{"vulpix.frost_field.explosion_radius", c.Vulpix.FrostField.ExplosionRadius},
		{"vulpix.frost_field.explosion_damage", float64(c.Vulpix.FrostField.ExplosionDamage)},
	} {
		if !(f.v >= 0) {
			return fmt.Errorf("%s must be >= 0, got %v", f.name, f.v)
		}
	}
	for name, r := range map[string]Reach{
		"pikachu.volt_tackle":    c.Pikachu.VoltTackle.Reach,
		"pikachu.thunder":        c.Pikachu.Thunder.Reach,
		"pikachu.electro_ball":   c.Pikachu.ElectroBall.Reach,
		"snorlax.jump":           c.Snorlax.Jump.Reach,
		"snorlax.ice_punch":      c.Snorlax.IcePunch.Reach,
		"snorlax.hyper_beam":     c.Snorlax.HyperBeam.Reach,
		"gyarados.hyper_beam":    c.Gyarados.HyperBeam.Reach,
		"charizard.fire_breath":  c.Charizard.FireBreath.Reach,
		"charizard.seismic_toss": c.Charizard.SeismicToss.Reach,
		"chikorita.leaf_slash":   c.Chikorita.LeafSlash.Reach,
		"chikorita.razor_leaf":   c.Chikorita.RazorLeaf.Reach,
		"chikorita.vine_whip":    c.Chikorita.VineWhip.Reach,
		"vulpix.frost_field":     c.Vulpix.FrostField.Reach,
		"vulpix.ice_storm":       c.Vulpix.IceStorm.Reach,
		"vulpix.freeze_beam":     c.Vulpix.FreezeBeam.Reach,
		"rowlet.arrows":          c.Rowlet.Arrows.Reach,
		"rowlet.tornado":         c.Rowlet.Tornado.Reach,
		"gyarados.wave":          c.Gyarados.Wave.Reach,
		"gyarados.tail_whip":     c.Gyarados.TailWhip.Reach,
	} {
		if !(r.MinHeight >= 0 && r.MaxHeight >= r.MinHeight) { // also rejects NaN
			return fmt.Errorf("%s: need 0 <= min_height <= max_height, got %v and %v", name, r.MinHeight, r.MaxHeight)
		}
	}
	for name, a := range map[string]float64{"snorlax": c.Snorlax.HyperBeam.SweepAngle, "gyarados": c.Gyarados.HyperBeam.SweepAngle} {
		if !(a >= 0 && a <= 180) { // 0 = a fixed beam, 180 = a full circle
			return fmt.Errorf("%s.hyper_beam.sweep_angle must be between 0 and 180, got %v", name, a)
		}
	}
	if rl := c.Chikorita.RazorLeaf; rl.Count > 200 {
		return fmt.Errorf("chikorita.razor_leaf.count must be at most 200, got %d", rl.Count)
	} else if !(rl.MaxDelay >= rl.MinDelay) {
		return fmt.Errorf("chikorita.razor_leaf.max_delay must be >= min_delay, got %v and %v", rl.MaxDelay, rl.MinDelay)
	}
	if a := c.Chikorita.VineWhip.SweepAngle; !(a > 0 && a <= 180) {
		return fmt.Errorf("chikorita.vine_whip.sweep_angle must be between 0 (exclusive) and 180, got %v", a)
	}
	if n := c.Rowlet.Arrows.Count; n > 100 {
		return fmt.Errorf("rowlet.arrows.count must be at most 100, got %d", n)
	}
	if v := c.Rowlet.Arrows.Slow; !(v >= 0 && v <= 1) { // 1 = a slowed body cannot walk at all
		return fmt.Errorf("rowlet.arrows.slow must be between 0 and 1, got %v", v)
	}
	if v := c.Vulpix.FrostField.Slow; !(v >= 0 && v <= 1) { // 1 = a body on the frost cannot move at all
		return fmt.Errorf("vulpix.frost_field.slow must be between 0 and 1, got %v", v)
	}
	if fb := c.Vulpix.FreezeBeam; !(fb.StartDistance <= fb.Length) { // the end could not start farther out than it may go
		return fmt.Errorf("vulpix.freeze_beam.start_distance must be at most length, got %v and %v", fb.StartDistance, fb.Length)
	}
	if j := c.Magikarp.Jump; !(j.HighestPointDuration < j.Duration) { // the rise and the fall need time of their own
		return fmt.Errorf("magikarp.jump.highest_point_duration must be less than duration, got %v and %v", j.HighestPointDuration, j.Duration)
	}
	if a := c.Gyarados.TailWhip.Angle; !(a > 0 && a <= 360) {
		return fmt.Errorf("gyarados.tail_whip.angle must be between 0 (exclusive) and 360, got %v", a)
	}
	if a := c.Charizard.FireBreath.RandomOffsetAngle; !(a >= 0 && a <= 90) { // 0 = every fire ball flies straight at the target
		return fmt.Errorf("charizard.fire_breath.random_offset_angle must be between 0 and 90, got %v", a)
	}
	return nil
}

// maxHPOf, walkSpeedOf and radiusOf are a character's full HP, walking speed and body size.
// beamOf is the Hyper Beam of whoever is playing kind (Snorlax or Gyarados).
func (c Config) beamOf(kind int) HyperBeamCfg {
	if kind == gyarados {
		return c.Gyarados.HyperBeam
	}
	return c.Snorlax.HyperBeam
}

func (c Config) maxHPOf(kind int) int {
	switch kind {
	case snorlax:
		return c.Snorlax.MaxHP
	case charizard:
		return c.Charizard.MaxHP
	case chikorita:
		return c.Chikorita.MaxHP
	case vulpix:
		return c.Vulpix.MaxHP
	case rowlet:
		return c.Rowlet.MaxHP
	case magikarp:
		return c.Magikarp.MaxHP
	case gyarados:
		return c.Gyarados.MaxHP
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
	case vulpix:
		return c.Vulpix.WalkSpeed
	case rowlet:
		return c.Rowlet.WalkSpeed
	case magikarp:
		return c.Magikarp.WalkSpeed
	case gyarados:
		return c.Gyarados.WalkSpeed
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
	case vulpix:
		return c.Vulpix.Radius
	case rowlet:
		return c.Rowlet.Radius
	case magikarp:
		return c.Magikarp.Radius
	case gyarados:
		return c.Gyarados.Radius
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
