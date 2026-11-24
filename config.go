package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"strconv"
	"strings"
)

// CourtSize is the playing field in game units.
type CourtSize struct {
	Width  float64 `json:"width"`
	Length float64 `json:"length"`
}

// CourtConfig is one view's court: the walls (`max_*`: what the fighters can walk on and where the spawns are; past them is out of
// bounds) and the window the camera shows. The island and the sea around it are drawn beyond the walls and the camera may look
// `sea_margin` units past them on every side. The camera shows `min_*` while the farthest two characters are close and widens to
// `max_*` plus the sea margin on both sides as they part: the zoom is all the way in at `zoom_in_distance` or less, all the way out at `zoom_out_distance`
// or more, and in between it is the distance's share of that span; it moves there by `zoom_speed` of the whole range per second. The window keeps the shape of `min_*`.
type CourtConfig struct {
	MinWidth        float64 `json:"min_width"`
	MinLength       float64 `json:"min_length"`
	MaxWidth        float64 `json:"max_width"`
	MaxLength       float64 `json:"max_length"`
	ZoomInDistance  float64 `json:"zoom_in_distance"`
	ZoomOutDistance float64 `json:"zoom_out_distance"`
	ZoomSpeed       float64 `json:"zoom_speed"`
	SeaMargin       float64 `json:"sea_margin"`
	// GigantamaxZoomOut: while any character on the field is Gigantamax the camera goes all the way out (at `zoom_speed`), whatever the distance between them is, since
	// such a body fills the screen; it goes back to following the distance when none is.
	GigantamaxZoomOut bool `json:"gigantamax_zoom_out"`
}

// size is the field itself.
func (c CourtConfig) size() CourtSize { return CourtSize{Width: c.MaxWidth, Length: c.MaxLength} }

// Reach is the band of heights in which a skill can hurt its target: a target whose height Z is between
// min_height and max_height (both included) is hit, anyone above or below it is not. Every attack has one;
// it is read as part of the skill's own block. Height 0 is the ground; the numbers are the same units as
// fly.height and jump.height.
type Reach struct {
	MinHeight float64 `json:"min_height"`
	MaxHeight float64 `json:"max_height"`
}

// minHeightFloor is how far below the ground a skill's min_height may be set: Gengar's Shadow Sneak puts it underground (a height below 0), and a skill whose
// band starts at -10 or lower is the one kind that can reach it.
const minHeightFloor = 1000

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

// StillTornado is the tornado Rowlet's Brave Bird leaves where it lands. It does not chase; it stands for `duration` seconds and takes whoever
// is within `radius` (at a height inside min_height..max_height) and spins them, like rowlet.tornado: carried up to `lift_height` by the middle of
// the spin, `spin_damage` every `spin_interval`, then dropped to take `landing_damage`. Unlike that one it also takes a body Brave Bird threw up
// in the air (it is lifted from where it hangs, up to lift_height if that is higher), so min_height..max_height must reach as high as launch_height.
// `scale` only changes how big it is drawn.
type StillTornado struct {
	Reach
	HP            int     `json:"hp"`
	Radius        float64 `json:"radius"`
	Scale         float64 `json:"scale"`
	Duration      float64 `json:"duration"` // 0 = none is left
	SpinInterval  float64 `json:"spin_interval"`
	SpinDamage    int     `json:"spin_damage"`
	LandingDamage int     `json:"landing_damage"`
	LiftHeight    float64 `json:"lift_height"`
	SpinDuration  float64 `json:"spin_duration"`
}

// spinParams is what a tornado does to whoever it holds: the numbers of rowlet.tornado, or of Brave Bird's still one.
type spinParams struct {
	Reach
	LiftHeight, SpinDuration, SpinInterval float64
	SpinDamage, LandingDamage              int
}

func (t StillTornado) spin() spinParams {
	return spinParams{t.Reach, t.LiftHeight, t.SpinDuration, t.SpinInterval, t.SpinDamage, t.LandingDamage}
}

// High Jump Kick (Machamp's and Ash's Kick): see BrawlerCfg.
type HighJumpKickCfg struct {
	Reach
	SlideDistance float64 `json:"slide_distance"`
	SlideDuration float64 `json:"slide_duration"`
	KickDelay     float64 `json:"kick_delay"` // seconds it stands still between the slide and the leap (the leap is aimed when it ends)
	KickDistance  float64 `json:"kick_distance"`
	KickSpeed     float64 `json:"kick_speed"`
	KickHeight    float64 `json:"kick_height"`
	Radius        float64 `json:"radius"`
	Damage        int     `json:"damage"`
	StunDuration  float64 `json:"stun_duration"`
	Knockback     float64 `json:"knockback"`
	Cooldown      float64 `json:"cooldown"`
	ShowWarning   bool    `json:"show_warning"` // true: the name of the move is shown over whoever is doing it
	SlideText     string  `json:"slide_text"`   // what is shown during the slide ("" = nothing)
	KickText      string  `json:"kick_text"`    // what is shown from the wait before the leap to the end of it ("" = nothing)
}

// Uppercut: see BrawlerCfg.
type UppercutCfg struct {
	Reach
	SlideDistance  float64 `json:"slide_distance"`
	SlideDuration  float64 `json:"slide_duration"`
	RushDelay      float64 `json:"rush_delay"`
	RushDistance   float64 `json:"rush_distance"`
	RushSpeed      float64 `json:"rush_speed"`
	Radius         float64 `json:"radius"`
	Damage         int     `json:"damage"`
	LaunchHeight   float64 `json:"launch_height"`
	LaunchDuration float64 `json:"launch_duration"`
	Cooldown       float64 `json:"cooldown"` // counted from the start of the slide
	ShowWarning    bool    `json:"show_warning"`
	SlideText      string  `json:"slide_text"`
	PunchText      string  `json:"punch_text"`
}

// Comet Punch: see BrawlerCfg.
type CometPunchCfg struct {
	Reach
	Radius             float64 `json:"radius"`
	HP                 int     `json:"hp"`
	Damage             int     `json:"damage"`
	Cooldown           float64 `json:"cooldown"` // counted from the start
	Mass               float64 `json:"mass"`
	Knockback          float64 `json:"knockback"`      // how far each fist that lands pushes the opponent, away from the fist
	BallKnockback      float64 `json:"ball_knockback"` // how far a fist shoves an opponent's projectile it meets: this much, times the share of the fist's mass the projectile lacks
	Duration           float64 `json:"duration"`
	Quantity           int     `json:"quantity"`
	RandomOffsetAngle  float64 `json:"random_offset_angle"`
	Distance           float64 `json:"distance"`
	ScatterMinDistance float64 `json:"scatter_min_distance"`
	ScatterMaxDistance float64 `json:"scatter_max_distance"`
	// How much the fists crowd towards the near end of that range: a fist's distance is min + (max - min) * u^bias, u an even random number from 0 to 1. 1 is even all along; 2 puts
	// half of them in the nearest quarter, so a foe close up is in front of more of them than behind them.
	ScatterNearBias float64 `json:"scatter_near_bias"`
	FistDuration    float64 `json:"fist_duration"`
}

// LightningPunchCfg is the Gigantamax Machamp's Lightning Punch (the K button in place of the High Jump Kick): it stands rooted for `charge_duration` seconds gathering
// the punch, aimed at the nearest foe all the while, and then throws a huge fist `distance` ahead, `radius` either side of the line. A foe it touches (at a height inside
// min_height..max_height) takes `damage`, is stunned `stun_duration` seconds and is knocked `knockback` away along the punch, and at the same moment a huge bolt of lightning
// comes down on it: every foe within `lightning_radius` of where it stood takes `lightning_damage`, once. `show_warning` draws the line of the punch on the ground while
// it gathers; `fist_scale` and `lightning_scale` only change how big the fist and the bolt are drawn (on top of the size of the body).
type LightningPunchCfg struct {
	Reach
	ChargeDuration  float64 `json:"charge_duration"`
	Distance        float64 `json:"distance"`
	Radius          float64 `json:"radius"`
	Damage          int     `json:"damage"`
	StunDuration    float64 `json:"stun_duration"`
	Knockback       float64 `json:"knockback"`
	LightningRadius float64 `json:"lightning_radius"`
	LightningDamage int     `json:"lightning_damage"`
	Cooldown        float64 `json:"cooldown"` // counted from the start of the gathering
	ShowWarning     bool    `json:"show_warning"`
	FistScale       float64 `json:"fist_scale"`
	LightningScale  float64 `json:"lightning_scale"`
}

// BurstPunchCfg is the Gigantamax Machamp's Burst Punch (its main button, in place of the Uppercut): it first slides `slide_distance` along the stick (facing, with none) in `slide_duration`
// seconds (0 for either: no slide), then stands rooted for `charge_duration` seconds gathering, its fist aimed at the
// nearest foe all the while (`show_warning` draws the cylinder it will burst in on the ground), and then the fist comes out `distance` ahead of it. Round that point, in a cylinder `radius` wide and
// `height` tall, `count` small explosions go off at random places spread over `duration` seconds, each higher than the one before: the cylinder is filled from the ground up. A foe an explosion (`explosion_radius`, a sphere) touches takes
// `explosion_damage` and is thrown up to `explosion_height`, helpless there for `launch_duration` seconds; every further explosion that touches it in the same cast throws it `height_gain`
// higher than the one before, up to `launch_cap`. `fist_scale` and `explosion_scale` only change how big the fist and the explosions are drawn, and `explosion_color_1` and
// `explosion_color_2` are the two colours an explosion flickers between (any CSS colour, so `rgba(...)` makes one see-through; the outline and the lightning inside are light on a dark colour
// and dark on a light one).
type BurstPunchCfg struct {
	SlideDistance   float64 `json:"slide_distance"`
	SlideDuration   float64 `json:"slide_duration"`
	ChargeDuration  float64 `json:"charge_duration"`
	Distance        float64 `json:"distance"`
	Radius          float64 `json:"radius"`
	Height          float64 `json:"height"`
	Count           int     `json:"count"`
	Duration        float64 `json:"duration"`
	ExplosionRadius float64 `json:"explosion_radius"`
	ExplosionDamage int     `json:"explosion_damage"`
	ExplosionHeight float64 `json:"explosion_height"`
	HeightGain      float64 `json:"height_gain"`
	LaunchCap       float64 `json:"launch_cap"`
	LaunchDuration  float64 `json:"launch_duration"`
	Cooldown        float64 `json:"cooldown"` // counted from the start of the gathering
	ShowWarning     bool    `json:"show_warning"`
	FistScale       float64 `json:"fist_scale"`
	ExplosionScale  float64 `json:"explosion_scale"`
	ExplosionColor1 string  `json:"explosion_color_1"`
	ExplosionColor2 string  `json:"explosion_color_2"`
}

// maxBursts is the most explosions one Burst Punch may let off.
const maxBursts = 200

// GigantamaxCfg is Machamp's ultimate (Z, once the charge is full): for `duration` seconds it is the Gigantamax Machamp, with its own `max_hp` (the HP it has is
// scaled to it, and back when it is over), `walk_speed` and body `radius`, and its own numbers for the skills it has: the Comet Punch (read exactly like
// machamp.comet_punch, which it uses the rest of the time), the Burst Punch, which takes the place of the Uppercut, and the Lightning Punch, which takes the place of the High Jump Kick.
type GigantamaxCfg struct {
	ChargeDuration float64           `json:"charge_duration"` // seconds it stands rooted gathering before it changes (0: at once); a stun, a freeze or a catch ends it and the charge is not given back
	MaxHP          int               `json:"max_hp"`
	WalkSpeed      float64           `json:"walk_speed"`
	Radius         float64           `json:"radius"`
	Duration       float64           `json:"duration"`
	CometPunch     CometPunchCfg     `json:"comet_punch"`
	BurstPunch     BurstPunchCfg     `json:"burst_punch"`
	LightningPunch LightningPunchCfg `json:"lightning_punch"`
}

// Ultimate is the charge a character's third skill (Z) needs. It starts the round empty and the skill only works once the charge
// reaches `max`; casting empties it. It fills in three independent ways, each in charge points: `gain_taken` for every HP of
// damage the character takes, `gain_dealt` for every HP of damage its foes take from the skills that are not an ultimate (in a 2v2 whoever hurt them, every foe of the
// victim's counts), `gain_ultimate_dealt` for every HP they take from an ultimate (Thousand Volt's bolts, the Giant Vines, Brave Bird and its tornado, the Megaton Punch,
// and everything the Gigantamax Machamp does while it lasts), and `gain_per_second` just for time passing.
type Ultimate struct {
	Max           float64 `json:"max"`
	GainTaken     float64 `json:"gain_taken"`
	GainDealt     float64 `json:"gain_dealt"`
	GainUltDealt  float64 `json:"gain_ultimate_dealt"` // like gain_dealt, but for the damage an ultimate does (so an ultimate that hits hard does not pay for itself)
	GainPerSecond float64 `json:"gain_per_second"`
}

// FlyCfg is Charizard's Fly (the main button): it lifts Charizard for `duration` seconds. It only moves faster. Whether a skill still hits it is up to that skill's
// min_height / max_height against `height`.
type FlyCfg struct {
	Duration float64 `json:"duration"`
	Cooldown float64 `json:"cooldown"` // starts when it has landed
	Speed    float64 `json:"speed"`    // walking speed while flying
	Height   float64 `json:"height"`
	// When the flight ends (Charizard is back on the ground) it breathes a ring of `ring_count` fire balls
	// that spread out from it, `ring_length` far; each is a fire ball of fire_breath (speed, size, HP, damage).
	// 0 balls = no ring.
	RingCount  int     `json:"ring_count"`
	RingLength float64 `json:"ring_length"`
}

// FireBreathCfg is Charizard's Fire Breath: for `duration` seconds Charizard spits `quantity` fire balls, evenly spaced, each at the
// opponent's position when it leaves the mouth, turned by a random angle within +-random_offset_angle.
// It can walk or fly while breathing. A fire ball hurts the first thing it touches and is gone; the
// opponent's skills wear its HP down, and it ends at `length` or at a wall.
type FireBreathCfg struct {
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
}

// SeismicTossCfg is Charizard's Seismic Toss: Charizard, on fire, runs `length` along the aim at `speed`. The body it touches (within
// `radius`) is caught and cannot move or act: Charizard flies up `high` with it, then slams it down for
// `damage`, with the same shock as Snorlax's landing (drawn `radius` wide). The rise and the fall go at `speed`.
type SeismicTossCfg struct {
	Reach            // who the run can catch, by height
	Speed    float64 `json:"speed"`
	Radius   float64 `json:"radius"`
	Damage   int     `json:"damage"`
	Cooldown float64 `json:"cooldown"` // counted from the start of the run
	Length   float64 `json:"length"`
	High     float64 `json:"high"`
	Mass     float64 `json:"mass"` // against another skill running at the same time, see pikachu.volt_tackle.mass
	// The slam sets the ground on fire where the opponent landed: a disc of `fire_radius` (plus the body of
	// whoever stands in it) that burns for `fire_duration` seconds. An opponent standing in it, no higher off
	// the ground than `fire_max_height`, takes `fire_damage` at once and again every `fire_interval` seconds.
	// A radius or duration of 0 = no fire.
	FireRadius    float64 `json:"fire_radius"`
	FireDuration  float64 `json:"fire_duration"`
	FireDamage    int     `json:"fire_damage"`
	FireInterval  float64 `json:"fire_interval"`
	FireMaxHeight float64 `json:"fire_max_height"`
}

// InfernoCfg is the Gigantamax Charizard's Inferno (the main button, in place of Fly): it stands rooted for `summon_duration` seconds calling a bird of fire, which grows in the
// sky `sky_height` above it; then the bird dives at the nearest foe, `dive_speed` along the straight way, and it aims at where the foe stands until it is `lock_distance` (along
// the way) from it, then the way is fixed, so a foe that steps aside in the last stretch is missed. Where it lands it bursts: foes within `explosion_radius` (and at a height inside
// min_height..max_height) take `damage` and are thrown up `launch_height` and held helpless there for `launch_duration` seconds. The ground is left burning like the Seismic Toss's:
// a disc of `fire_radius` for `fire_duration` seconds, `fire_damage` at once and every `fire_interval` seconds to whoever stands in it no higher than `fire_max_height`.
// `bird_scale` only changes how big the bird is drawn. `cooldown` counts from the cast. The bird has `hp`: it is shot down (and nothing bursts) when that is worn away.
type InfernoCfg struct {
	Reach
	SummonDuration  float64 `json:"summon_duration"`
	SkyHeight       float64 `json:"sky_height"`
	DiveSpeed       float64 `json:"dive_speed"`
	LockDistance    float64 `json:"lock_distance"`
	ExplosionRadius float64 `json:"explosion_radius"`
	Damage          int     `json:"damage"`
	LaunchHeight    float64 `json:"launch_height"`
	LaunchDuration  float64 `json:"launch_duration"`
	Cooldown        float64 `json:"cooldown"`
	HP              int     `json:"hp"`          // the bird's HP: any skill that wears down projectiles (and Thousand Volt's bolts) can shoot it down before it lands; it cannot be hurt while it is still being called
	Radius          float64 `json:"radius"`      // how big the bird is to the projectiles and skills that hit it (the sky and the ground do not matter to them)
	BallDamage      int     `json:"ball_damage"` // what it takes off another projectile it runs into
	BirdScale       float64 `json:"bird_scale"`
	FireRadius      float64 `json:"fire_radius"`
	FireDuration    float64 `json:"fire_duration"`
	FireDamage      int     `json:"fire_damage"`
	FireInterval    float64 `json:"fire_interval"`
	FireMaxHeight   float64 `json:"fire_max_height"`
}

// CharizardGmaxCfg is Charizard's ultimate (Z, once the charge is full): for `duration` seconds it is the Gigantamax Charizard, with its own `max_hp` (the HP it has is
// scaled to it, and back when it is over), `walk_speed` and body `radius`, and its own numbers for its three skills (Inferno on the main button, in place of Fly, then Fire Breath and Seismic Toss),
// charizard.fire_breath and charizard.seismic_toss read exactly like the ones Charizard uses the rest of the time, and `inferno` takes the place of charizard.fly, which it uses the rest of the time. The fire balls and the burning ground it
// makes keep these numbers after it has changed back.
type CharizardGmaxCfg struct {
	ChargeDuration float64        `json:"charge_duration"` // seconds it stands rooted gathering before it changes (0: at once); a stun, a freeze or a catch ends it and the charge is not given back
	MaxHP          int            `json:"max_hp"`
	WalkSpeed      float64        `json:"walk_speed"`
	Radius         float64        `json:"radius"`
	Duration       float64        `json:"duration"`
	Inferno        InfernoCfg     `json:"inferno"` // takes the place of Fly on the main button
	FireBreath     FireBreathCfg  `json:"fire_breath"`
	SeismicToss    SeismicTossCfg `json:"seismic_toss"`
}

// Config holds the tuning values. Distances are in game units and times are in seconds.
type Config struct {
	// Length runs between the two players (left-right on the web page, up-down on a phone), width
	// across. A room takes the court of the view its first player opened it in.
	Court struct {
		Mobile CourtConfig `json:"mobile"`
		Web    CourtConfig `json:"web"`
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
			Scale            float64 `json:"scale"` // how big the ball of lightning Pikachu is wrapped in is drawn, as a multiple of `radius`; looks only
		} `json:"volt_tackle"`
		// Thunder: every bolt also takes `damage` off every projectile (fire balls, stones, pillars...) of the other player inside `radius`.
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
			Scale           float64 `json:"scale"`     // how big the ball and its blast are drawn, times radius and explosion_radius; looks only, the reach is unchanged
		} `json:"electro_ball"`
		// Thousand Volt (Z, the ultimate; the button is lit once `ultimate` is full, and the cast empties the charge): Pikachu first
		// gathers, rooted, for `ring_duration` + `ball_duration` seconds (like the Megaton Punch: rings closing in on the hand, then a
		// black and white ball swelling; being frozen, stunned or held breaks it, and the charge is gone). Only the look is set by
		// `bolt_start`/`bolt_count`/`bolt_interval`/`bolt_offsets` (the bolts from the sky onto the hand, as the Megaton Punch's) and
		// `ring_scale`/`ball_scale`. Then `count` bolts of lightning
		// leave Pikachu all round it, fly out `ring_radius` at `spread_speed`, then each heads for the nearest foe at `follow_speed`
		// wherever it goes. A bolt is a projectile (`radius` big, `hp` and `mass`: the foe's skills and projectiles wear it down; at 0 it
		// is gone). Touching a foe it hurts it for `contact_damage` and pushes it `knockback` straight away from the bolt, and again
		// every `rehit_interval` seconds it still overlaps (0: once). `terminal_time` seconds after the cast every bolt left blows up (sooner for a bolt that has touched a foe `hits_to_explode` times: it blows up at that touch):
		// within `explosion_radius` a foe is hurt `explosion_damage`, once for the whole cast however many bolts reach it.
		ThousandVolt struct {
			Reach
			// Once a bolt has flown out it chases the nearest foe; with hunt_hp above 0 it goes first for the nearest projectile of a foe that has at least that much HP (a big attack, like the Inferno's
			// bird; not a standing pillar or stone) within hunt_range (0: anywhere), and only when there is none does it chase the foe.
			HuntHP           int       `json:"hunt_hp"`
			HuntRange        float64   `json:"hunt_range"`
			RingDuration     float64   `json:"ring_duration"`
			BallDuration     float64   `json:"ball_duration"`
			BoltStart        float64   `json:"bolt_start"`
			BoltCount        int       `json:"bolt_count"` // 0..20
			BoltInterval     float64   `json:"bolt_interval"`
			BoltOffsets      []float64 `json:"bolt_offsets"`
			RingScale        float64   `json:"ring_scale"`
			BallScale        float64   `json:"ball_scale"`
			BoltColors       []string  `json:"bolt_colors"` // "#rrggbb": bolt n of a cast gets the (n mod how many there are)-th
			SkyDark          float64   `json:"sky_dark"`    // 0..1: how dark the screen goes while a Thousand Volt is on (looks only; fades in and out)
			FlashEvery       int       `json:"flash_every"` // the screen flashes white, clear, black, clear, each for this many frames, on each Thousand Volt explosion and Thunder strike; 0 = no flashing (looks only)
			FlashAlpha       float64   `json:"flash_alpha"` // 0..1: how strong the flash is
			RingRadius       float64   `json:"ring_radius"`
			SpreadSpeed      float64   `json:"spread_speed"`
			FollowSpeed      float64   `json:"follow_speed"`
			ContactDamage    int       `json:"contact_damage"`
			Count            int       `json:"count"`
			Radius           float64   `json:"radius"`
			RehitInterval    float64   `json:"rehit_interval"`
			HP               int       `json:"hp"`
			Mass             float64   `json:"mass"`
			Knockback        float64   `json:"knockback"`
			TerminalTime     float64   `json:"terminal_time"`
			TipBallScale     float64   `json:"tip_ball_scale"`     // how big the ball of electricity a bolt is drawn as is, as a multiple of `radius` (looks only; 0: no ball drawn)
			LinkWidth        float64   `json:"link_width"`         // how thick the line of lightning joining a bolt to its Pikachu is drawn (looks only)
			JumpHeight       float64   `json:"jump_height"`        // how high Pikachu flips up while it gathers and stays until its bolts are done (it rises and comes down over ring_duration)
			RootedWhileBolts bool      `json:"rooted_while_bolts"` // Pikachu cannot walk or dash from the cast until none of its bolts is left (gone or exploded)
			HitsToExplode    int       `json:"hits_to_explode"`    // a bolt blows up at once on its Nth touch of a foe (0: never early)
			ExplosionDamage  int       `json:"explosion_damage"`
			ExplosionRadius  float64   `json:"explosion_radius"`
		} `json:"thousand_volt"`
		Ultimate Ultimate `json:"ultimate"` // how fast the charge for Thousand Volt fills
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
			SlamRadius float64 `json:"slam_radius"` // reach of the landing shock; the shock effect is drawn at this size
			Damage     int     `json:"damage"`      // what the landing shock does to whoever it reaches (and to the projectiles it reaches)
			// The earthquake the landing leaves round the spot: `earthquake_radius` wide (0: none; it must not be smaller than slam_radius), for `earthquake_duration` seconds. Every
			// `earthquake_interval` seconds it does `earthquake_damage` to every foe in it (by the jump's min/max height), and all the while a foe is in it, it is slowed:
			// `earthquake_slow` is the share of its walking speed it loses (0..1).
			EarthquakeRadius   float64 `json:"earthquake_radius"`
			EarthquakeDuration float64 `json:"earthquake_duration"`
			EarthquakeDamage   int     `json:"earthquake_damage"`
			EarthquakeInterval float64 `json:"earthquake_interval"`
			EarthquakeSlow     float64 `json:"earthquake_slow"`
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
		MaxHP       int            `json:"max_hp"`
		WalkSpeed   float64        `json:"walk_speed"`
		Radius      float64        `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		Fly         FlyCfg         `json:"fly"`
		FireBreath  FireBreathCfg  `json:"fire_breath"`
		SeismicToss SeismicTossCfg `json:"seismic_toss"`
		// Ultimate (Z) is the charge for the Gigantamax Charizard below; Gigantamax is what it turns into (see CharizardGmaxCfg).
		Ultimate   Ultimate         `json:"ultimate"`
		Gigantamax CharizardGmaxCfg `json:"gigantamax"`
	} `json:"charizard"`
	Chikorita struct {
		MaxHP     int     `json:"max_hp"`
		WalkSpeed float64 `json:"walk_speed"`
		Radius    float64 `json:"radius"` // body size: drawn at this size, and it widens what skills can hit
		// Leaf Slash (the main button): Chikorita hops `jump_distance` the way it is walking, `jump_duration` seconds
		// in the air at the top `jump_height` (no damage on landing), then whips the leaf on its head `slashes` times,
		// `slash_interval` seconds each: the first from the upper left to the lower right, the second from the upper
		// right to the lower left, and so on. Each swing carries it `slash_step` the way the stick is held (not at the opponent by itself) and hits once,
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
		Ultimate Ultimate `json:"ultimate"` // the charge Giant Vines (Z) needs
		// Giant Vines (the ultimate): a very thick vine runs underground at `follow_speed` after the nearest foe and, when it touches it, or
		// `terminal_time` seconds after it set off (wherever it has got to by then), surfaces there. `count` giant vines burst out of the ground
		// on a ring of `ring_radius` round that spot, one every `interval` seconds. Each stands up for `charge_duration` seconds, locked on the foe
		// as it was when it came out (the shape it will hit is drawn on the ground, like the Tree Slam's trunk, `trunk_length` long and
		// `trunk_width` wide from where it stands), then comes down in `swing_duration` seconds and hits whoever is in that shape when it lands:
		// `damage` and a slow (`slow_duration` seconds, taking the share `slow` of its walking speed; 0 seconds = no slow) each. It lies there for `linger` seconds and is gone. Chikorita cannot move until the last
		// one has landed. `head_radius` is how near the head must get to count as touching; `head_width` and `trunk_scale` only change how
		// big the head and the vines are drawn.
		GiantVines struct {
			Reach
			FollowSpeed    float64 `json:"follow_speed"`
			TerminalTime   float64 `json:"terminal_time"`
			HeadRadius     float64 `json:"head_radius"`
			HeadWidth      float64 `json:"head_width"`
			WormOut        float64 `json:"worm_out_length"` // looks only: the head runs under the ground like a worm: it is out of the ground for this far...
			WormIn         float64 `json:"worm_in_length"`  // ...then under it for this far, and so on (a stretch that came out stays until the skill is over)
			Count          int     `json:"count"`           // 1..maxGiantVines
			RingRadius     float64 `json:"ring_radius"`
			SpreadTime     float64 `json:"spread_time"` // seconds the first vine takes to be spread out from where the head surfaced to its place on the ring (each later one takes `interval` longer, so they spread out slower and slower: it is also when each comes out)
			Interval       float64 `json:"interval"`
			ChargeDuration float64 `json:"charge_duration"`
			SwingDuration  float64 `json:"swing_duration"`
			TrunkLength    float64 `json:"trunk_length"`
			TrunkWidth     float64 `json:"trunk_width"` // how wide it is where it comes out of the ground...
			TipWidth       float64 `json:"tip_width"`   // ...and at its tip: what it hits is that root-shaped taper
			TrunkScale     float64 `json:"trunk_scale"`
			Damage         int     `json:"damage"`
			SlowDuration   float64 `json:"slow_duration"`
			Slow           float64 `json:"slow"`          // 0..1
			StunHits       int     `json:"stun_hits"`     // a foe that has been hit by this many vines of one cast (and again every this many more) is stunned for stun_duration seconds; 0 = never
			StunDuration   float64 `json:"stun_duration"` // 0 = no stun
			RingScale      float64 `json:"ring_scale"`    // looks only: the size of the rings that close in on a vine as it stands up (the Thousand Volt's ring_duration look)
			Linger         float64 `json:"linger"`
			ShowWarning    bool    `json:"show_warning"` // false: the shape each vine will hit is not drawn on the ground (the rings, the vine rising and the hit are; the bot still reads where it will hit)
		} `json:"giant_vines"`
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
		// is `width` wide and stops at the first opponent in its way (inside the height band): it hurts that opponent
		// `damage` every `interval` seconds, and every `freeze_hits`-th hit of the cast freezes it for `freeze_duration`
		// (0 hits = never). Where the spot passes, ice pillars rise (`pillar_radius` wide, `pillar_hp`, gone after
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
			Width           float64 `json:"width"`
			Damage          int     `json:"damage"`
			Interval        float64 `json:"interval"`
			FreezeHits      int     `json:"freeze_hits"`
			FreezeDuration  float64 `json:"freeze_duration"`
		} `json:"freeze_beam"`
		Ultimate Ultimate `json:"ultimate"` // the charge Glaciate (Z) needs
		// Glaciate (Z): after `ring_duration` seconds of gathering (rooted) Vulpix calls `count` icicles down over `duration` seconds, at random
		// places in a circle of `radius` whose middle is on the nearest foe as it was when the gathering ended, but no further than `distance` from Vulpix. Each
		// icicle shows where it will come down for `fall_duration` seconds, then lands: foes within `damage_radius` take `damage` and are
		// slowed, and it stays standing like a pillar (the other player cannot walk through it) with its own `hp`, `radius` and `scale`, for
		// `duration` seconds. min_height/max_height are the heights a landing hurts and a standing icicle blocks.
		Glaciate struct {
			Reach
			RingDuration float64 `json:"ring_duration"`
			Duration     float64 `json:"duration"`
			Count        int     `json:"count"`
			Radius       float64 `json:"radius"`
			Distance     float64 `json:"distance"`   // the furthest from Vulpix the middle of the circle may be (0: on Vulpix)
			RingScale    float64 `json:"ring_scale"` // looks only: how big the gathering rings are drawn
			Icicle       struct {
				FallDuration float64 `json:"fall_duration"`
				Damage       int     `json:"damage"`
				DamageRadius float64 `json:"damage_radius"`
				Slow         float64 `json:"slow"` // the share of the walking speed taken off, 0..1
				SlowDuration float64 `json:"slow_duration"`
				HP           int     `json:"hp"`       // what a standing icicle takes before it breaks, worn down like a pillar
				Radius       float64 `json:"radius"`   // the body it blocks the foe with
				Scale        float64 `json:"scale"`    // looks only: how many times bigger than its radius it is drawn
				Duration     float64 `json:"duration"` // seconds it stands before it melts
			} `json:"icicle"`
		} `json:"glaciate"`
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
			// Slash (the page calls the skill Air Slash): while it drifts Rowlet throws `count` air slashes, evenly spread along the run
			// (the last one as it ends), each flying at `speed` straight at the nearest opponent as it stood when thrown. A slash hurts
			// (`damage`) what its `radius` touches at the heights of `min_height`..`max_height`, and is spent; it has `hp` that the
			// opponent's skills can wear down and `mass` (see hyper_beam.mass). `scale` is how many times bigger it is drawn than its
			// radius suggests; looks only. A count of 0 throws none.
			Slash struct {
				Reach
				Count  int     `json:"count"`
				Speed  float64 `json:"speed"`
				Radius float64 `json:"radius"`
				HP     int     `json:"hp"`
				Damage int     `json:"damage"`
				Mass   float64 `json:"mass"`
				Scale  float64 `json:"scale"`
			} `json:"slash"`
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
		// Ultimate (Z) and Brave Bird, its skill: Rowlet is rooted for `ring_duration` (the rings of the Megaton Punch close in on it), then
		// for `charge_duration` it climbs to `sky_height` while a light in the shape of a bird grows out of its middle; then it dives at
		// where the opponent stands at that moment, `dive_speed` along the way down, and bursts on landing: foes within
		// `explosion_radius` (and `min_height`..`max_height`) take `damage` and are not pushed but thrown up to `launch_height` and kept
		// helpless in the air for `launch_duration` seconds. Stuns and freezes cannot stop it from the charge to the landing.
		Ultimate  Ultimate `json:"ultimate"`
		BraveBird struct {
			Reach
			RingDuration    float64      `json:"ring_duration"`
			ChargeDuration  float64      `json:"charge_duration"`
			SkyHeight       float64      `json:"sky_height"`
			DiveSpeed       float64      `json:"dive_speed"`
			LockDistance    float64      `json:"lock_distance"` // the dive follows the foe until it is this far (along the way) from it, then the way is fixed; 0 = it follows it all the way down
			ExplosionRadius float64      `json:"explosion_radius"`
			Damage          int          `json:"damage"`
			LaunchHeight    float64      `json:"launch_height"`
			LaunchDuration  float64      `json:"launch_duration"`
			RingScale       float64      `json:"ring_scale"`     // looks only: the size of the rings that close in
			LightScale      float64      `json:"light_scale"`    // looks only: how big the bird of light grows
			LeafScale       float64      `json:"leaf_scale"`     // looks only: how many times bigger the leaves that burst out where it lands are drawn
			GravityRadius   float64      `json:"gravity_radius"` // after the burst, foes within this far of where it landed (plus their body) are dragged toward that spot (0 = no pull)
			Gravity         float64      `json:"gravity"`        // pixels a second they are dragged, never past the middle (like shadow_ball.gravity; 0 = no pull)
			PullDuration    float64      `json:"pull_duration"`  // seconds the pull lasts (0 = no pull)
			Tornado         StillTornado `json:"tornado"`        // the tornado left where it lands (see StillTornado)
			SkyDark         float64      `json:"sky_dark"`       // looks only: how dark the overcast half of the flicker is (0..1) while the bird is gathered and dives
			FlashEvery      float64      `json:"flash_every"`    // looks only: how many frames the sky stays overcast, then bright (0 = no flicker, just overcast)
			FlashAlpha      float64      `json:"flash_alpha"`    // looks only: how bright the bright half is (0..1)
		} `json:"brave_bird"`
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
		// Tail whip (K): Gyarados winds up for `charge_duration` seconds, rooted, with the fan it will hit drawn on the
		// ground for the opponent to see (the aim is fixed when K is pressed; 0 = no wind-up, it hits at once). Then
		// the opponent in the fan, `angle` degrees wide and up to `length` away, is knocked back `knockback` and
		// stunned for `stun_duration` seconds (it can neither move nor act).
		TailWhip struct {
			Reach
			ChargeDuration float64 `json:"charge_duration"`
			Angle          float64 `json:"angle"`
			Length         float64 `json:"length"`
			Knockback      float64 `json:"knockback"`
			StunDuration   float64 `json:"stun_duration"`
			Cooldown       float64 `json:"cooldown"`
		} `json:"tail_whip"`
		// Its Hyper Beam (J): the same skill as Snorlax's, with numbers of its own.
		HyperBeam HyperBeamCfg `json:"hyper_beam"`
	} `json:"gyarados"`
	// Ash (Pallet Town) and Machamp use the same skills, each with its own numbers (Machamp's a little lower).
	Ash     BrawlerCfg `json:"ash"`
	Machamp BrawlerCfg `json:"machamp"`
	// Gengar: Shadow Sneak (the main button) slips into the ground for `duration` seconds at `height` (negative: below it), so
	// that no skill whose band (min_height..max_height) does not reach down that far can touch it; it steers at
	// `shadow_speed_first_half` then `shadow_speed_second_half`, and a black shadow `shadow_radius` wide marks where it is: an opponent
	// touching the shadow (at a height inside this skill's own min_height..max_height) cannot start a skill.
	// Ghost Fire (J) lets out `count` purple flames all round it that fly out `ring_radius` at `spread_speed`, then come
	// back to it at `convergence_speed`, following it wherever it goes; each hurts the opponent at most once (`damage`) if it touches it (`radius`).
	// A flame is a projectile in Game.Balls with `hp` and `mass`: the opponent's balls and skills wear it down and a flame
	// wears them down by its `damage`, as fire balls do.
	// Shadow Ball (K, aimed like the Vine Whip) lobs a ball `height` high that takes `duration` seconds to come down, at most
	// `speed` x `duration` away, on the opponent unless an aim was chosen; where it lands it hurts the opponent in
	// `slam_radius` (`damage`), and sends `ghost_fire_count` purple flames out `ghost_fire_spread_distance` at `ghost_fire_spread_speed`
	// that do not come back (`ghost_fire_radius` big). They have no HP, and the whole burst hurts the opponent at most once (`ghost_fire_damage`),
	// however many of them touch it. `ball_scale` only makes the drawn ball bigger.
	Gengar struct {
		MaxHP       int     `json:"max_hp"`
		WalkSpeed   float64 `json:"walk_speed"`
		Radius      float64 `json:"radius"`
		ShadowSneak struct {
			Reach
			Height                float64 `json:"height"`
			Duration              float64 `json:"duration"`
			Cooldown              float64 `json:"cooldown"` // counted from the start
			ShadowSpeedFirstHalf  float64 `json:"shadow_speed_first_half"`
			ShadowSpeedSecondHalf float64 `json:"shadow_speed_second_half"`
			ShadowRadius          float64 `json:"shadow_radius"`
		} `json:"shadow_sneak"`
		GhostFire struct {
			Reach
			Cooldown         float64 `json:"cooldown"`
			RingRadius       float64 `json:"ring_radius"`
			SpreadSpeed      float64 `json:"spread_speed"`
			ConvergenceSpeed float64 `json:"convergence_speed"`
			Damage           int     `json:"damage"`
			Count            int     `json:"count"`
			Radius           float64 `json:"radius"`
			RehitInterval    float64 `json:"rehit_interval"` // 0: a flame hurts the opponent once; above 0: again every that many seconds it still overlaps
			HP               int     `json:"hp"`             // a flame is a projectile: the opponent's balls and skills wear it down
			Mass             float64 `json:"mass"`           // what a beam or a fist has to beat to push it
		} `json:"ghost_fire"`
		// Shadow Ball (K): rooted for `charge_time` seconds, then a ball flies straight along the aim (the way the Electro Ball does) at
		// `speed`, `radius` big, with `hp` and `mass` (the opponent's skills and projectiles wear it down; at 0 it bursts where it
		// is). It bursts on the opponent, a wall or when shot down: `slam_radius` round it the opponent takes `damage` once and is then
		// dragged to the middle at `gravity` pixels a second for `pull_duration` seconds. `ball_scale` only makes the drawn ball bigger.
		ShadowBall struct {
			Reach
			BallScale    float64 `json:"ball_scale"`
			Damage       int     `json:"damage"`
			Cooldown     float64 `json:"cooldown"`
			SlamRadius   float64 `json:"slam_radius"` // the reach of the blast that hurts
			Speed        float64 `json:"speed"`
			ChargeTime   float64 `json:"charge_time"`
			Radius       float64 `json:"radius"`
			HP           int     `json:"hp"`
			Mass         float64 `json:"mass"`
			Gravity      float64 `json:"gravity"`
			PullDuration float64 `json:"pull_duration"`
			// The reach of the pull that drags the opponent in, from the middle of the burst (plus its body), on its own: it may be wider than the blast. 0: the same as slam_radius.
			GravityRadius float64 `json:"gravity_radius"`
		} `json:"shadow_ball"`
		// Night Shade: the lobbed ball Shadow Ball used to be, kept for a skill to come (no key or button is bound to it yet; the
		// server runs it when an input says `shade`, which a person's never does). A ball is lobbed `speed` x `duration` out at most,
		// flies `duration` seconds in an arc `height` high and where it comes down hurts what is within `slam_radius` of it for
		// `damage`, once, and sends `ghost_fire_count` flames out that do not come back (`ghost_fire_spread_speed` fast, `ghost_fire_spread_distance`
		// far, `ghost_fire_radius` wide, `ghost_fire_damage` between them for the opponent, once). `ball_scale` only makes the drawn ball bigger.
		NightShade struct {
			Reach
			BallScale               float64 `json:"ball_scale"`
			Damage                  int     `json:"damage"`
			Height                  float64 `json:"height"`
			Duration                float64 `json:"duration"`
			Cooldown                float64 `json:"cooldown"`
			SlamRadius              float64 `json:"slam_radius"`
			Speed                   float64 `json:"speed"`
			GhostFireSpreadSpeed    float64 `json:"ghost_fire_spread_speed"`
			GhostFireRadius         float64 `json:"ghost_fire_radius"`
			GhostFireSpreadDistance float64 `json:"ghost_fire_spread_distance"`
			GhostFireCount          int     `json:"ghost_fire_count"`
			GhostFireDamage         int     `json:"ghost_fire_damage"`
		} `json:"night_shade"`
	} `json:"gengar"`
	// The practice dummy (the mode "打木樁"): a stump that stands still and never hits back (a blow can still knock it back, and it stays where it lands), with `max_hp` so high that a practice
	// session does not end it, `radius` wide. It is only ever the computer's, in a room opened for practice.
	Dummy struct {
		MaxHP  int     `json:"max_hp"`
		Radius float64 `json:"radius"`
	} `json:"dummy"`
}

// BrawlerCfg is the numbers of one of the two brawlers (Ash and Machamp): the tree slam, the comet punch and the rock tomb.
type BrawlerCfg struct {
	MaxHP     int      `json:"max_hp"`
	WalkSpeed float64  `json:"walk_speed"`
	Radius    float64  `json:"radius"`
	Ultimate  Ultimate `json:"ultimate"` // Ash only (Machamp has none): the charge the Megaton Punch, which is his ultimate (Z), needs
	// Tree slam (the main button): Machamp jumps `jump_distance` along the direction held (else the way it faces) in
	// `jump_duration` seconds, `jump_height` up; on landing it pulls a tree out of the ground and holds it up for
	// `charge_duration` seconds (rooted, with the shape it will smash down drawn on the ground for the opponent), then
	// smashes it down along the jump. The tree is a trunk, `trunk_length` long and `trunk_width` wide from Machamp's
	// feet, and a canopy at its tip: a sector `leaf_radius` out, `leaf_angle` degrees wide (180 = a half circle facing
	// away from Machamp). The opponent touching either, at a height inside min_height..max_height, takes `damage`
	// once. `trunk_scale` and `leaf_scale` only make the drawn tree bigger; what it hits is the lengths and radius.
	TreeSlam struct {
		Reach
		JumpDistance   float64 `json:"jump_distance"`
		JumpDuration   float64 `json:"jump_duration"`
		JumpHeight     float64 `json:"jump_height"`
		ChargeDuration float64 `json:"charge_duration"`
		SwingDuration  float64 `json:"swing_duration"`
		TrunkScale     float64 `json:"trunk_scale"`
		TrunkLength    float64 `json:"trunk_length"`
		TrunkWidth     float64 `json:"trunk_width"`
		LeafScale      float64 `json:"leaf_scale"`
		LeafRadius     float64 `json:"leaf_radius"`
		LeafAngle      float64 `json:"leaf_angle"`
		Damage         int     `json:"damage"`
		StunDuration   float64 `json:"stun_duration"` // seconds the opponent it lands on is stunned (can neither move nor act); 0 = none
		Cooldown       float64 `json:"cooldown"`
	} `json:"tree_slam"`
	// High Jump Kick (Machamp's main skill; a person's Ash has the tree instead, the computer's Ash has both, with `ash.high_jump_kick`): Machamp slides `slide_distance` along the stick (the way it faces
	// without one) in `slide_duration` seconds, then leaps at the opponent, aimed as the slide ends, flying `kick_speed` per second
	// for up to `kick_distance` and rising `kick_height` at the top of the leap. The opponent (at a height inside min_height..max_height)
	// whose body comes within `radius` of its path takes `damage`, is stunned `stun_duration` seconds and is kicked `knockback` away;
	// the kick stops where it lands. A kick that finds nobody ends at `kick_distance`.
	HighJumpKick HighJumpKickCfg `json:"high_jump_kick"`
	// Uppercut (Machamp's main skill; the computer's Ash does not have it): like the High Jump Kick it is in two parts. First it slides `slide_distance` along the stick
	// (the way it faces without one) in `slide_duration` seconds, stands still `rush_delay` seconds, then rushes at where the opponent stands, `rush_speed` per second for at
	// most `rush_distance`, and stops at the first body it meets (`radius` more than the bodies) to throw an uppercut: that foe, at a height inside min_height..max_height, takes
	// `damage` and is not knocked back: it is thrown up to `launch_height` and held helpless in the air for `launch_duration` seconds (as Brave Bird does). `show_warning`
	// shows `slide_text` over it during the slide and `punch_text` from the wait to the end of the rush ("" = nothing).
	Uppercut UppercutCfg `json:"uppercut"`
	// Comet Punch (J): for `duration` seconds Machamp runs at the opponent (`distance` in all, stopping at its body)
	// throwing `quantity` fists, evenly spaced in time. Each fist appears ahead of it, `scatter_min_distance` to
	// `scatter_max_distance` beyond its body and up to `random_offset_angle` degrees either side of its heading, hangs
	// there for `fist_duration` seconds and hurts the opponent it touches once (`damage`) and pushes it `knockback` away from the fist. A fist has `hp` and is worn
	// down like a fire ball (`mass`: see hyper_beam.mass). A fist also shoves the opponent's projectiles (electro balls, leaves, fire balls,
	// arrows) back, away from itself, by `ball_knockback` x (mass - the projectile's mass) / mass: none for one as heavy as the fist, nearly the whole
	// of `ball_knockback` for a light one.
	CometPunch CometPunchCfg `json:"comet_punch"`
	// Gigantamax (Machamp's ultimate; Ash has none): see GigantamaxCfg. Its charge is `ultimate`, like the other ultimates.
	Gigantamax GigantamaxCfg `json:"gigantamax"`
	// Megaton Punch (Ash only: Machamp never throws it): Ash stands rooted for `ring_duration` + `ball_duration` seconds, the direction locked
	// on the opponent as it begins, with a warning on the ground (the line the fist will fly along) and circles closing in on
	// its hand; then it throws a fist `distance` ahead, `radius` either side of the line. The opponent it touches (at a height
	// inside min_height..max_height) takes `damage` once, is stunned `stun_duration` seconds and is knocked `knockback` away
	// along the punch. Ash is carried `lunge_distance` forward with the fist as it is thrown (it stops at the opponent's body); 0 =
	// it stays where it is. `show_warning` false leaves out the line of the fist drawn on the ground (the rings and the charge stay);
	// the computer still reads the danger from the state. `show_text` false leaves out the "轟！" shown where it lands. The fist is only drawn flying from `fist_start` to `fist_end`, measured along the punch from
	// where Ash stood as it threw (negative = behind it); what it hits is `distance`. `ring_scale` (the closing rings), `ball_scale` (the black and white ball), `fist_scale` and `impact_scale` only make the drawn rings, ball, fist and impact; `ball_follow_scale`/`ball_follow_duration` the ball that goes forward with Ash as it is thrown, `ball_hit_scale`/`ball_hit_duration` the black ball that bursts on the opponent it hits (its size is `ball_scale` times `ball_hit_scale`; seconds, drawn only)
	// bigger; what it hits is `distance` and `radius`.
	MegatonPunch struct {
		Reach
		RingDuration       float64   `json:"ring_duration"` // the charge is two parts: the rings closing in on the hand for this long, then...
		BallDuration       float64   `json:"ball_duration"` // ...the black and white ball swelling for this long; the fist is thrown when both are over
		BoltStart          float64   `json:"bolt_start"`    // seconds from the start of the charge to the first bolt from the sky onto the fist
		BoltCount          int       `json:"bolt_count"`    // how many bolts (0..20); each is drawn for 0.22 s
		BoltInterval       float64   `json:"bolt_interval"` // seconds between the starts of two bolts
		BoltOffsets        []float64 `json:"bolt_offsets"`  // where bolt n starts in the sky, sideways of the fist in pixels (the n-th, repeated if there are fewer); every bolt ends on the fist
		Distance           float64   `json:"distance"`
		Radius             float64   `json:"radius"`
		Damage             int       `json:"damage"`
		StunDuration       float64   `json:"stun_duration"`
		Knockback          float64   `json:"knockback"`
		Cooldown           float64   `json:"cooldown"` // counted from the start
		LungeDistance      float64   `json:"lunge_distance"`
		JumpDistance       float64   `json:"jump_distance"` // the leap along the stick before it gathers (0 = none: it gathers where it stands)
		JumpDuration       float64   `json:"jump_duration"`
		JumpHeight         float64   `json:"jump_height"`
		ShowWarning        bool      `json:"show_warning"`
		ShowText           bool      `json:"show_text"`
		FistStart          float64   `json:"fist_start"`
		FistEnd            float64   `json:"fist_end"`
		RingScale          float64   `json:"ring_scale"`
		BallScale          float64   `json:"ball_scale"`
		FistScale          float64   `json:"fist_scale"`
		ImpactScale        float64   `json:"impact_scale"`
		BallFollowScale    float64   `json:"ball_follow_scale"`    // the ball that goes forward with Ash as the fist is thrown: its size...
		BallFollowDuration float64   `json:"ball_follow_duration"` // ...and how long it stays
		BallHitScale       float64   `json:"ball_hit_scale"`       // the black ball that bursts in lightning on the opponent it hits: its size (times ball_scale)...
		BallHitDuration    float64   `json:"ball_hit_duration"`    // ...and how long the burst lasts
		FlashEvery         int       `json:"flash_every"`          // Ash only: while the ball swells (`ball_duration`) the screen flashes white, clear, black, clear, each for this many frames; 0 = no flashing (looks only)
	} `json:"megaton_punch"`
	// Rock Tomb (K): Machamp jumps `height` up for `duration` seconds, steered in the air at `air_speed_first_half`
	// then `air_speed_second_half`, and the landing raises `count` stones in a ring `ring_radius` from where it lands
	// (those that would be outside the court are left out). A stone is a solid disc of `stone_radius` for the
	// opponent (not for Machamp) at heights inside min_height..max_height, has `stone_hp`, and is worn down like a
	// projectile; it crumbles after `stone_duration` seconds.
	RockTomb struct {
		Reach
		Enabled            bool    `json:"enabled"` // false: nobody uses it (a person has no key for it and the computer never starts it)
		RingRadius         float64 `json:"ring_radius"`
		Height             float64 `json:"height"`
		Duration           float64 `json:"duration"`
		Cooldown           float64 `json:"cooldown"` // counted from the take-off
		AirSpeedFirstHalf  float64 `json:"air_speed_first_half"`
		AirSpeedSecondHalf float64 `json:"air_speed_second_half"`
		Count              int     `json:"count"`
		StoneRadius        float64 `json:"stone_radius"`
		StoneHP            int     `json:"stone_hp"`
		StoneDuration      float64 `json:"stone_duration"`
	} `json:"rock_tomb"`
}

func DefaultConfig() Config {
	var c Config
	c.Court.Mobile = CourtConfig{MinWidth: arenaH, MinLength: arenaW, MaxWidth: arenaH, MaxLength: arenaW, ZoomInDistance: 500, ZoomOutDistance: 800, ZoomSpeed: 0.5} // no zoom: the window is the whole court
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
	c.Pikachu.VoltTackle.Scale = 1
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
	c.Pikachu.ElectroBall.Scale = 1
	tv := &c.Pikachu.ThousandVolt
	tv.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	tv.RingDuration, tv.BallDuration, tv.BoltStart, tv.BoltCount, tv.BoltInterval = 0.6, 1.8, 1.2, 6, 0.2
	tv.LinkWidth, tv.TipBallScale, tv.JumpHeight = 9, 0.6, 80
	tv.SkyDark, tv.FlashEvery, tv.FlashAlpha = 0.4, 15, 0.25
	tv.BoltColors = []string{"#a855f7", "#ff8c1a", "#15151f"}
	tv.BoltOffsets, tv.RingScale, tv.BallScale = []float64{60, -60, -120, 120, 60, -60}, 2.2, 2
	tv.RingRadius, tv.SpreadSpeed, tv.FollowSpeed, tv.ContactDamage, tv.Count, tv.Radius = 220, 400, 260, 4, 10, 16
	tv.RehitInterval, tv.HP, tv.Mass, tv.Knockback, tv.TerminalTime, tv.ExplosionDamage, tv.ExplosionRadius = 0.5, 10, 20, 40, 5, 20, 110
	c.Pikachu.Ultimate = Ultimate{Max: 100, GainTaken: 1, GainDealt: 1, GainUltDealt: 1, GainPerSecond: 1}
	c.Ash.Ultimate = Ultimate{Max: 100, GainTaken: 1, GainDealt: 1, GainUltDealt: 1, GainPerSecond: 1}
	c.Snorlax.MaxHP = maxHP
	c.Snorlax.WalkSpeed = speed
	c.Snorlax.Radius = snorlaxRadius
	c.Snorlax.Jump.Height = 72
	c.Snorlax.Jump.Duration = 0.8
	c.Snorlax.Jump.Cooldown = 1.5
	c.Snorlax.Jump.SlamRadius = slamRadius
	c.Snorlax.Jump.Damage = slamDamage
	c.Snorlax.Jump.EarthquakeDuration, c.Snorlax.Jump.EarthquakeDamage, c.Snorlax.Jump.EarthquakeInterval, c.Snorlax.Jump.EarthquakeSlow = 3, 5, 0.5, 0.3 // no earthquake (radius 0) unless the file asks for one
	c.Snorlax.Jump.AirSpeedFirstHalf = speed                                                                                                              // the same as walking, i.e. no change until it is tuned
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
	c.Charizard.Fly.RingCount, c.Charizard.Fly.RingLength = 16, 350
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
	c.Charizard.SeismicToss.FireRadius, c.Charizard.SeismicToss.FireDuration, c.Charizard.SeismicToss.FireDamage = 130, 6, 2
	c.Charizard.SeismicToss.FireInterval, c.Charizard.SeismicToss.FireMaxHeight = 0.5, groundZ
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
	c.Chikorita.Ultimate = Ultimate{Max: 100, GainTaken: 1, GainDealt: 1, GainUltDealt: 1, GainPerSecond: 1}
	gv := &c.Chikorita.GiantVines
	gv.Reach = groundReach
	gv.FollowSpeed, gv.TerminalTime, gv.HeadRadius, gv.HeadWidth, gv.WormOut, gv.WormIn = 260, 4, 40, 50, 100, 100
	gv.Count, gv.RingRadius, gv.SpreadTime, gv.Interval, gv.ChargeDuration, gv.SwingDuration = 10, 200, 0.4, 0.25, 0.5, 0.2
	gv.TrunkLength, gv.TrunkWidth, gv.TipWidth, gv.TrunkScale, gv.Damage, gv.SlowDuration, gv.Slow, gv.Linger = 320, 80, 20, 1, 8, 3, 0.5, 0.7
	gv.ShowWarning = true
	gv.StunHits, gv.StunDuration, gv.RingScale = 5, 1, 0.8
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
	c.Vulpix.Ultimate = Ultimate{Max: 100, GainTaken: 1, GainDealt: 1, GainUltDealt: 1, GainPerSecond: 1}
	gl := &c.Vulpix.Glaciate
	gl.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	gl.RingDuration, gl.Duration, gl.Count, gl.Radius, gl.Distance, gl.RingScale = 0.8, 4, 12, 300, 300, 1
	gl.Icicle.FallDuration, gl.Icicle.Damage, gl.Icicle.DamageRadius, gl.Icicle.Slow, gl.Icicle.SlowDuration = 0.8, 10, 100, 0.4, 2
	gl.Icicle.HP, gl.Icicle.Radius, gl.Icicle.Scale, gl.Icicle.Duration = 30, 30, 1, 8
	c.Vulpix.FreezeBeam.Interval = 0.5 // damage, freeze_hits and width stay 0: the beam only blocks and hurts when configured
	c.Rowlet.MaxHP = 100
	c.Rowlet.WalkSpeed = 150
	c.Rowlet.Radius = 13
	c.Rowlet.FlyHigh = 50
	c.Rowlet.Tailwind.Radius = 120
	c.Rowlet.Tailwind.Speed = 600
	c.Rowlet.Tailwind.Duration = 0.5
	c.Rowlet.Tailwind.Cooldown = 4
	c.Rowlet.Tailwind.Slash.Reach = Reach{MinHeight: 0, MaxHeight: 15}
	c.Rowlet.Tailwind.Slash.Speed = 650
	c.Rowlet.Tailwind.Slash.Radius = 8
	c.Rowlet.Tailwind.Slash.HP = 1
	c.Rowlet.Tailwind.Slash.Damage = 2
	c.Rowlet.Tailwind.Slash.Mass = 20
	c.Rowlet.Tailwind.Slash.Scale = 0.5 // Count stays 0: no slashes unless config.json asks for them
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
	c.Rowlet.Ultimate = Ultimate{Max: 100, GainTaken: 1, GainDealt: 1, GainUltDealt: 1, GainPerSecond: 1}
	bb := &c.Rowlet.BraveBird
	bb.Reach = Reach{MinHeight: 0, MaxHeight: 1000}
	bb.RingDuration, bb.ChargeDuration, bb.SkyHeight, bb.DiveSpeed, bb.LockDistance = 0.5, 1.5, 400, 1200, 400
	bb.ExplosionRadius, bb.Damage, bb.LaunchHeight, bb.LaunchDuration = 160, 30, 200, 1.5
	bb.RingScale, bb.LightScale, bb.LeafScale = 1, 1, 1
	bb.SkyDark, bb.FlashEvery, bb.FlashAlpha = 0.45, 6, 0.3
	bb.Tornado = StillTornado{Reach: Reach{MinHeight: 0, MaxHeight: 1000}, HP: 40, Radius: 90, Scale: 1, Duration: 4, SpinInterval: 0.5, SpinDamage: 4, LandingDamage: 20, LiftHeight: 300, SpinDuration: 3}

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
	c.Gyarados.TailWhip.ChargeDuration = 0.5
	c.Ash.MaxHP, c.Ash.WalkSpeed, c.Ash.Radius = 140, 140, 30
	ts := &c.Ash.TreeSlam
	ts.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	ts.JumpDistance, ts.JumpDuration, ts.JumpHeight, ts.ChargeDuration, ts.SwingDuration = 250, 0.5, 50, 0.6, 0.25
	ts.TrunkScale, ts.TrunkLength, ts.TrunkWidth, ts.LeafScale, ts.LeafRadius, ts.LeafAngle = 1, 300, 70, 1, 180, 180
	ts.Damage, ts.Cooldown, ts.StunDuration = 35, 8, 1
	mp := &c.Ash.MegatonPunch
	mp.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	mp.RingDuration, mp.BallDuration, mp.BoltStart, mp.BoltCount, mp.BoltOffsets = 0.5, 0.5, 0.5, 1, []float64{0}
	mp.Distance, mp.Radius, mp.Damage, mp.StunDuration, mp.Knockback, mp.Cooldown = 420, 45, 40, 1.5, 250, 14
	mp.RingScale, mp.BallScale, mp.FistScale, mp.ImpactScale = 1, 1, 1, 1
	mp.BallFollowScale, mp.BallFollowDuration, mp.BallHitScale, mp.BallHitDuration = 1, 0.3, 1, 0.6
	mp.FlashEvery = 15
	mp.LungeDistance, mp.ShowWarning, mp.ShowText = 120, true, true
	mp.JumpDistance, mp.JumpDuration, mp.JumpHeight = 200, 0.4, 50
	mp.FistStart, mp.FistEnd = 24, mp.Distance
	c.Dummy.MaxHP, c.Dummy.Radius = 100000, 28
	cp := &c.Ash.CometPunch
	cp.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	cp.Radius, cp.HP, cp.Damage, cp.Cooldown, cp.Mass = 22, 20, 2, 8, 10
	cp.BallKnockback, cp.Knockback = 120, 10
	cp.Duration, cp.Quantity, cp.RandomOffsetAngle, cp.Distance = 1.5, 24, 40, 300
	cp.ScatterMinDistance, cp.ScatterMaxDistance, cp.FistDuration, cp.ScatterNearBias = 20, 90, 0.2, 1
	rt := &c.Ash.RockTomb
	rt.Enabled = true
	rt.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	rt.RingRadius, rt.Height, rt.Duration, rt.Cooldown = 500, 80, 0.8, 12
	rt.AirSpeedFirstHalf, rt.AirSpeedSecondHalf = 150, 100
	rt.Count, rt.StoneRadius, rt.StoneHP, rt.StoneDuration = 24, 60, 100, 8
	g := &c.Gengar
	g.MaxHP, g.WalkSpeed, g.Radius = 110, 155, 18
	ss := &g.ShadowSneak
	ss.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	ss.Height, ss.Duration, ss.Cooldown, ss.ShadowSpeedFirstHalf, ss.ShadowSpeedSecondHalf, ss.ShadowRadius = -10, 2.5, 8, 260, 180, 90
	gf := &g.GhostFire
	gf.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	gf.Cooldown, gf.RingRadius, gf.SpreadSpeed, gf.ConvergenceSpeed, gf.Damage, gf.Count, gf.Radius = 7, 260, 300, 200, 4, 12, 22
	gf.HP, gf.Mass = 10, 20
	sb := &g.ShadowBall
	sb.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	sb.BallScale, sb.Damage, sb.Cooldown, sb.SlamRadius, sb.Speed = 1, 25, 6, 110, 500
	sb.ChargeTime, sb.Radius, sb.HP, sb.Mass, sb.Gravity, sb.PullDuration = 0.8, 30, 30, 50, 250, 1
	ns := &g.NightShade
	ns.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	ns.BallScale, ns.Damage, ns.Height, ns.Duration, ns.Cooldown, ns.SlamRadius, ns.Speed = 1, 25, 150, 0.9, 6, 110, 500
	ns.GhostFireSpreadSpeed, ns.GhostFireRadius, ns.GhostFireSpreadDistance, ns.GhostFireCount, ns.GhostFireDamage = 260, 20, 220, 10, 5
	c.Machamp = c.Ash // the same moves a notch weaker: less health and reach, fewer and frailer fists and stones, longer waits
	c.Machamp.MaxHP, c.Machamp.WalkSpeed = 125, 130
	c.Machamp.TreeSlam.TrunkLength, c.Machamp.TreeSlam.LeafRadius, c.Machamp.TreeSlam.Damage, c.Machamp.TreeSlam.Cooldown = 270, 160, 30, 9
	c.Machamp.TreeSlam.StunDuration = 0.8
	hk := &c.Machamp.HighJumpKick
	hk.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	hk.SlideDistance, hk.SlideDuration, hk.KickDistance, hk.KickSpeed, hk.KickHeight = 180, 0.25, 450, 1000, 40
	hk.Radius, hk.Damage, hk.StunDuration, hk.Knockback, hk.Cooldown = 10, 35, 1.2, 300, 9
	hk.SlideText, hk.KickText = "閃身步", "浪子踢球"
	c.Ash.HighJumpKick = *hk // the computer's Ash kicks as well, a notch harder
	c.Ash.HighJumpKick.ShowWarning = true
	c.Machamp.HighJumpKick.KickText = "飛膝踢" // Machamp's is the Flying Knee Kick
	c.Ash.HighJumpKick.SlideDistance, c.Ash.HighJumpKick.KickDistance, c.Ash.HighJumpKick.Damage, c.Ash.HighJumpKick.Cooldown = 200, 500, 40, 8
	uc := &c.Machamp.Uppercut
	uc.Reach = Reach{MinHeight: 0, MaxHeight: 60}
	uc.SlideDistance, uc.SlideDuration, uc.RushDistance, uc.RushSpeed, uc.Radius = 120, 0.2, 380, 900, 14
	uc.Damage, uc.LaunchHeight, uc.LaunchDuration, uc.Cooldown = 25, 200, 1.2, 7
	uc.SlideText, uc.PunchText = "閃身步", "上鉤拳"
	c.Ash.Uppercut = *uc // not used by Ash: filled so the file's check is the same
	c.Machamp.CometPunch.Quantity, c.Machamp.CometPunch.HP, c.Machamp.CometPunch.Cooldown, c.Machamp.CometPunch.BallKnockback = 20, 16, 9, 100
	c.Machamp.CometPunch.Knockback = 8
	c.Machamp.RockTomb.RingRadius, c.Machamp.RockTomb.Count, c.Machamp.RockTomb.StoneHP, c.Machamp.RockTomb.StoneDuration, c.Machamp.RockTomb.Cooldown = 450, 20, 80, 7, 14
	gm := &c.Machamp.Gigantamax // the ultimate: bigger, tougher and harder-hitting than Machamp
	gm.MaxHP, gm.WalkSpeed, gm.Radius, gm.Duration = 190, 125, 40, 15
	gm.CometPunch = c.Machamp.CometPunch
	gm.BurstPunch = BurstPunchCfg{SlideDistance: 150, SlideDuration: 0.25, ChargeDuration: 0.8, Distance: 350, Radius: 220, Height: 300, Count: 24, Duration: 1.2, ExplosionRadius: 70, ExplosionDamage: 8, ExplosionHeight: 90, HeightGain: 60, LaunchCap: 420, LaunchDuration: 0.6, Cooldown: 10, ShowWarning: true, FistScale: 1, ExplosionScale: 1, ExplosionColor1: "rgba(8,8,13,0.55)", ExplosionColor2: "#fafafa"}
	gm.LightningPunch = LightningPunchCfg{Reach: Reach{MinHeight: 0, MaxHeight: 80}, ChargeDuration: 1.2, Distance: 500, Radius: 70, Damage: 40, StunDuration: 1, Knockback: 200, LightningRadius: 160, LightningDamage: 30, Cooldown: 9, FistScale: 1, LightningScale: 1}
	gm.CometPunch.Damage, gm.CometPunch.Quantity, gm.CometPunch.Radius = 3, 28, 26
	c.Charizard.Ultimate = Ultimate{Max: 100, GainTaken: 1, GainDealt: 1, GainUltDealt: 1, GainPerSecond: 1}
	cg := &c.Charizard.Gigantamax // the ultimate: bigger and tougher than Charizard, with bigger fire balls and a bigger toss
	cg.MaxHP, cg.WalkSpeed, cg.Radius, cg.Duration = 600, 200, 60, 8
	cg.FireBreath, cg.SeismicToss = c.Charizard.FireBreath, c.Charizard.SeismicToss
	cg.Inferno = InfernoCfg{Reach: groundReach, SummonDuration: 1, SkyHeight: 300, DiveSpeed: 700, LockDistance: 250, ExplosionRadius: 220, Damage: 50, LaunchHeight: 150, LaunchDuration: 0.8, Cooldown: 14, HP: 60, Radius: 50, BallDamage: 4, BirdScale: 1.5,
		FireRadius: 380, FireDuration: 8, FireDamage: 6, FireInterval: 0.5, FireMaxHeight: 30}
	cg.FireBreath.Radius, cg.FireBreath.Damage, cg.FireBreath.HP, cg.FireBreath.Quantity = 28, 6, 12, 30
	cg.SeismicToss.Radius, cg.SeismicToss.Damage, cg.SeismicToss.FireRadius = 160, 60, 320
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

// validCSSColor is whether s is short and made only of the characters a CSS colour is (a name, #hex, rgb()/rgba()/hsl()): it is only ever given to the canvas as a colour.
func validCSSColor(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("#(),.% /-", r)) {
			return false
		}
	}
	return true
}

// validate checks the Burst Punch's numbers.
func (b BurstPunchCfg) validate() error {
	if b.Count < 1 || b.Count > maxBursts {
		return fmt.Errorf("count must be 1..%d, got %d", maxBursts, b.Count)
	}
	for _, f := range []struct {
		name string
		v    float64
		min  float64 // exclusive lower bound
	}{{"radius", b.Radius, 0}, {"explosion_radius", b.ExplosionRadius, 0}, {"explosion_height", b.ExplosionHeight, 0}, {"fist_scale", b.FistScale, 0}, {"explosion_scale", b.ExplosionScale, 0}} {
		if !(f.v > f.min) {
			return fmt.Errorf("%s must be above %v, got %v", f.name, f.min, f.v)
		}
	}
	for _, f := range []struct {
		name string
		v    float64
	}{{"slide_distance", b.SlideDistance}, {"slide_duration", b.SlideDuration}, {"charge_duration", b.ChargeDuration}, {"distance", b.Distance}, {"height", b.Height}, {"duration", b.Duration}, {"height_gain", b.HeightGain}, {"launch_duration", b.LaunchDuration}, {"cooldown", b.Cooldown}, {"explosion_damage", float64(b.ExplosionDamage)}} {
		if !(f.v >= 0) || math.IsInf(f.v, 0) {
			return fmt.Errorf("%s must not be negative, got %v", f.name, f.v)
		}
	}
	for name, col := range map[string]string{"explosion_color_1": b.ExplosionColor1, "explosion_color_2": b.ExplosionColor2} {
		if !validCSSColor(col) {
			return fmt.Errorf("%s must be a CSS colour such as #fafafa or rgba(8,8,13,0.55), got %q", name, col)
		}
	}
	if b.LaunchCap < b.ExplosionHeight {
		return fmt.Errorf("launch_cap (%v) must not be below explosion_height (%v)", b.LaunchCap, b.ExplosionHeight)
	}
	return nil
}

// validate checks the whole config: the numbers of every character, and then the Gigantamax Machamp's own (its three skills are checked just like Machamp's).
func (c Config) validate() error {
	if err := c.validateSkills(); err != nil {
		return err
	}
	gm := c.Machamp.Gigantamax
	if gm.MaxHP <= 0 || !(gm.WalkSpeed > 0) || !(gm.Radius > 0) || !(gm.Duration > 0) {
		return fmt.Errorf("machamp.gigantamax: max_hp (%d), walk_speed (%v), radius (%v) and duration (%v) must be above 0", gm.MaxHP, gm.WalkSpeed, gm.Radius, gm.Duration)
	}
	tmp := c
	tmp.Machamp.CometPunch = gm.CometPunch
	if err := tmp.validateSkills(); err != nil {
		return fmt.Errorf("machamp.gigantamax's skills are checked like Machamp's, and: %w", err)
	}
	if err := gm.BurstPunch.validate(); err != nil {
		return fmt.Errorf("machamp.gigantamax.burst_punch: %w", err)
	}
	cg := c.Charizard.Gigantamax
	if cg.MaxHP <= 0 || !(cg.WalkSpeed > 0) || !(cg.Radius > 0) || !(cg.Duration > 0) {
		return fmt.Errorf("charizard.gigantamax: max_hp (%d), walk_speed (%v), radius (%v) and duration (%v) must be above 0", cg.MaxHP, cg.WalkSpeed, cg.Radius, cg.Duration)
	}
	tmp = c
	tmp.Charizard.FireBreath, tmp.Charizard.SeismicToss = cg.FireBreath, cg.SeismicToss
	if err := tmp.validateSkills(); err != nil {
		return fmt.Errorf("charizard.gigantamax's skills are checked like Charizard's, and: %w", err)
	}
	return nil
}

func (c Config) validateSkills() error {
	for _, f := range []struct {
		name string
		v    float64
		min  float64 // exclusive lower bound
	}{
		{"court.mobile.min_width", c.Court.Mobile.MinWidth, 0},
		{"court.web.min_width", c.Court.Web.MinWidth, 0},
		{"court.mobile.max_width", c.Court.Mobile.MaxWidth, 0},
		{"court.web.max_width", c.Court.Web.MaxWidth, 0},
		{"court.mobile.zoom_speed", c.Court.Mobile.ZoomSpeed, 0},
		{"court.web.zoom_speed", c.Court.Web.ZoomSpeed, 0},
		{"pikachu.radius", c.Pikachu.Radius, 0},
		{"snorlax.radius", c.Snorlax.Radius, 0},
		{"pikachu.max_hp", float64(c.Pikachu.MaxHP), 0},
		{"pikachu.walk_speed", c.Pikachu.WalkSpeed, 0},
		{"pikachu.volt_tackle.radius", c.Pikachu.VoltTackle.Radius, 0},
		{"pikachu.volt_tackle.distance", c.Pikachu.VoltTackle.Distance, 0},
		{"pikachu.volt_tackle.duration", c.Pikachu.VoltTackle.Duration, 0},
		{"pikachu.volt_tackle.max_uses", float64(c.Pikachu.VoltTackle.MaxUses), 0},
		{"pikachu.volt_tackle.use_window", c.Pikachu.VoltTackle.UseWindow, 0},
		{"pikachu.volt_tackle.scale", c.Pikachu.VoltTackle.Scale, 0},
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
		{"pikachu.electro_ball.scale", c.Pikachu.ElectroBall.Scale, 0},
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
		{"charizard.fly.ring_length", c.Charizard.Fly.RingLength, 0},
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
		{"charizard.gigantamax.inferno.sky_height", c.Charizard.Gigantamax.Inferno.SkyHeight, 0},
		{"charizard.gigantamax.inferno.dive_speed", c.Charizard.Gigantamax.Inferno.DiveSpeed, 0},
		{"charizard.gigantamax.inferno.explosion_radius", c.Charizard.Gigantamax.Inferno.ExplosionRadius, 0},
		{"charizard.gigantamax.inferno.bird_scale", c.Charizard.Gigantamax.Inferno.BirdScale, 0},
		{"charizard.gigantamax.inferno.hp", float64(c.Charizard.Gigantamax.Inferno.HP), 0},
		{"charizard.gigantamax.inferno.radius", c.Charizard.Gigantamax.Inferno.Radius, 0},
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
		{"vulpix.freeze_beam.interval", c.Vulpix.FreezeBeam.Interval, 0},
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
		{"pikachu.thousand_volt.ring_radius", c.Pikachu.ThousandVolt.RingRadius, 0},
		{"pikachu.thousand_volt.spread_speed", c.Pikachu.ThousandVolt.SpreadSpeed, 0},
		{"pikachu.thousand_volt.count", float64(c.Pikachu.ThousandVolt.Count), 0},
		{"pikachu.thousand_volt.radius", c.Pikachu.ThousandVolt.Radius, 0},
		{"pikachu.thousand_volt.hp", float64(c.Pikachu.ThousandVolt.HP), 0},
		{"pikachu.thousand_volt.mass", c.Pikachu.ThousandVolt.Mass, 0},
		{"pikachu.thousand_volt.terminal_time", c.Pikachu.ThousandVolt.TerminalTime, 0},
		{"pikachu.ultimate.max", c.Pikachu.Ultimate.Max, 0},
		{"ash.ultimate.max", c.Ash.Ultimate.Max, 0},
		{"chikorita.ultimate.max", c.Chikorita.Ultimate.Max, 0},
		{"rowlet.ultimate.max", c.Rowlet.Ultimate.Max, 0},
		{"machamp.ultimate.max", c.Machamp.Ultimate.Max, 0},
		{"charizard.ultimate.max", c.Charizard.Ultimate.Max, 0},
		{"vulpix.ultimate.max", c.Vulpix.Ultimate.Max, 0},
		{"vulpix.glaciate.duration", c.Vulpix.Glaciate.Duration, 0},
		{"vulpix.glaciate.radius", c.Vulpix.Glaciate.Radius, 0},
		{"vulpix.glaciate.ring_scale", c.Vulpix.Glaciate.RingScale, 0},
		{"vulpix.glaciate.icicle.fall_duration", c.Vulpix.Glaciate.Icicle.FallDuration, 0},
		{"vulpix.glaciate.icicle.damage_radius", c.Vulpix.Glaciate.Icicle.DamageRadius, 0},
		{"vulpix.glaciate.icicle.hp", float64(c.Vulpix.Glaciate.Icicle.HP), 0},
		{"vulpix.glaciate.icicle.radius", c.Vulpix.Glaciate.Icicle.Radius, 0},
		{"vulpix.glaciate.icicle.scale", c.Vulpix.Glaciate.Icicle.Scale, 0},
		{"vulpix.glaciate.icicle.duration", c.Vulpix.Glaciate.Icicle.Duration, 0},
		{"chikorita.giant_vines.follow_speed", c.Chikorita.GiantVines.FollowSpeed, 0},
		{"chikorita.giant_vines.terminal_time", c.Chikorita.GiantVines.TerminalTime, 0},
		{"chikorita.giant_vines.head_radius", c.Chikorita.GiantVines.HeadRadius, 0},
		{"chikorita.giant_vines.worm_out_length", c.Chikorita.GiantVines.WormOut, 0},
		{"chikorita.giant_vines.worm_in_length", c.Chikorita.GiantVines.WormIn, 0},
		{"chikorita.giant_vines.ring_radius", c.Chikorita.GiantVines.RingRadius, 0},
		{"chikorita.giant_vines.trunk_length", c.Chikorita.GiantVines.TrunkLength, 0},
		{"chikorita.giant_vines.trunk_width", c.Chikorita.GiantVines.TrunkWidth, 0},
		{"chikorita.giant_vines.trunk_scale", c.Chikorita.GiantVines.TrunkScale, 0},
		{"gyarados.max_hp", float64(c.Gyarados.MaxHP), 0},
		{"gyarados.walk_speed", c.Gyarados.WalkSpeed, 0},
		{"gyarados.radius", c.Gyarados.Radius, 0},
		{"gyarados.wave.radius", c.Gyarados.Wave.Radius, 0},
		{"gyarados.wave.speed", c.Gyarados.Wave.Speed, 0},
		{"gyarados.wave.jump_duration", c.Gyarados.Wave.JumpDuration, 0},
		{"gyarados.tail_whip.length", c.Gyarados.TailWhip.Length, 0},
		{"machamp.max_hp", float64(c.Machamp.MaxHP), 0},
		{"ash.max_hp", float64(c.Ash.MaxHP), 0},
		{"machamp.walk_speed", c.Machamp.WalkSpeed, 0},
		{"ash.walk_speed", c.Ash.WalkSpeed, 0},
		{"machamp.radius", c.Machamp.Radius, 0},
		{"gengar.max_hp", float64(c.Gengar.MaxHP), 0},
		{"gengar.walk_speed", c.Gengar.WalkSpeed, 0},
		{"gengar.radius", c.Gengar.Radius, 0},
		{"gengar.shadow_sneak.duration", c.Gengar.ShadowSneak.Duration, 0},
		{"gengar.shadow_sneak.shadow_radius", c.Gengar.ShadowSneak.ShadowRadius, 0},
		{"gengar.ghost_fire.ring_radius", c.Gengar.GhostFire.RingRadius, 0},
		{"gengar.ghost_fire.spread_speed", c.Gengar.GhostFire.SpreadSpeed, 0},
		{"gengar.ghost_fire.convergence_speed", c.Gengar.GhostFire.ConvergenceSpeed, 0},
		{"gengar.ghost_fire.count", float64(c.Gengar.GhostFire.Count), 0},
		{"gengar.ghost_fire.radius", c.Gengar.GhostFire.Radius, 0},
		{"gengar.ghost_fire.hp", float64(c.Gengar.GhostFire.HP), 0},
		{"gengar.shadow_ball.ball_scale", c.Gengar.ShadowBall.BallScale, 0},
		{"gengar.shadow_ball.slam_radius", c.Gengar.ShadowBall.SlamRadius, 0},
		{"gengar.shadow_ball.speed", c.Gengar.ShadowBall.Speed, 0},
		{"gengar.night_shade.ball_scale", c.Gengar.NightShade.BallScale, 0},
		{"gengar.night_shade.duration", c.Gengar.NightShade.Duration, 0},
		{"gengar.night_shade.slam_radius", c.Gengar.NightShade.SlamRadius, 0},
		{"gengar.night_shade.speed", c.Gengar.NightShade.Speed, 0},
		{"gengar.night_shade.ghost_fire_spread_speed", c.Gengar.NightShade.GhostFireSpreadSpeed, 0},
		{"gengar.night_shade.ghost_fire_radius", c.Gengar.NightShade.GhostFireRadius, 0},
		{"gengar.night_shade.ghost_fire_spread_distance", c.Gengar.NightShade.GhostFireSpreadDistance, 0},
		{"gengar.shadow_ball.radius", c.Gengar.ShadowBall.Radius, 0},
		{"gengar.shadow_ball.hp", float64(c.Gengar.ShadowBall.HP), 0},
		{"ash.radius", c.Ash.Radius, 0},
		{"machamp.tree_slam.jump_distance", c.Machamp.TreeSlam.JumpDistance, 0},
		{"machamp.high_jump_kick.kick_distance", c.Machamp.HighJumpKick.KickDistance, 0},
		{"machamp.high_jump_kick.kick_speed", c.Machamp.HighJumpKick.KickSpeed, 0},
		{"ash.high_jump_kick.kick_distance", c.Ash.HighJumpKick.KickDistance, 0},
		{"ash.high_jump_kick.kick_speed", c.Ash.HighJumpKick.KickSpeed, 0},
		{"ash.tree_slam.jump_distance", c.Ash.TreeSlam.JumpDistance, 0},
		{"ash.megaton_punch.distance", c.Ash.MegatonPunch.Distance, 0},
		{"dummy.max_hp", float64(c.Dummy.MaxHP), 0},
		{"dummy.radius", c.Dummy.Radius, 0},
		{"ash.megaton_punch.radius", c.Ash.MegatonPunch.Radius, 0},
		{"ash.megaton_punch.ring_scale", c.Ash.MegatonPunch.RingScale, 0},
		{"ash.megaton_punch.ball_scale", c.Ash.MegatonPunch.BallScale, 0},
		{"ash.megaton_punch.ball_follow_scale", c.Ash.MegatonPunch.BallFollowScale, 0},
		{"ash.megaton_punch.ball_hit_scale", c.Ash.MegatonPunch.BallHitScale, 0},
		{"ash.megaton_punch.fist_scale", c.Ash.MegatonPunch.FistScale, 0},
		{"ash.megaton_punch.impact_scale", c.Ash.MegatonPunch.ImpactScale, 0},
		{"machamp.tree_slam.jump_duration", c.Machamp.TreeSlam.JumpDuration, 0},
		{"ash.tree_slam.jump_duration", c.Ash.TreeSlam.JumpDuration, 0},
		{"machamp.tree_slam.trunk_scale", c.Machamp.TreeSlam.TrunkScale, 0},
		{"ash.tree_slam.trunk_scale", c.Ash.TreeSlam.TrunkScale, 0},
		{"machamp.tree_slam.trunk_length", c.Machamp.TreeSlam.TrunkLength, 0},
		{"ash.tree_slam.trunk_length", c.Ash.TreeSlam.TrunkLength, 0},
		{"machamp.tree_slam.trunk_width", c.Machamp.TreeSlam.TrunkWidth, 0},
		{"ash.tree_slam.trunk_width", c.Ash.TreeSlam.TrunkWidth, 0},
		{"machamp.tree_slam.leaf_scale", c.Machamp.TreeSlam.LeafScale, 0},
		{"ash.tree_slam.leaf_scale", c.Ash.TreeSlam.LeafScale, 0},
		{"machamp.tree_slam.leaf_radius", c.Machamp.TreeSlam.LeafRadius, 0},
		{"ash.tree_slam.leaf_radius", c.Ash.TreeSlam.LeafRadius, 0},
		{"machamp.comet_punch.radius", c.Machamp.CometPunch.Radius, 0},
		{"ash.comet_punch.radius", c.Ash.CometPunch.Radius, 0},
		{"machamp.comet_punch.hp", float64(c.Machamp.CometPunch.HP), 0},
		{"ash.comet_punch.hp", float64(c.Ash.CometPunch.HP), 0},
		{"machamp.comet_punch.duration", c.Machamp.CometPunch.Duration, 0},
		{"ash.comet_punch.duration", c.Ash.CometPunch.Duration, 0},
		{"machamp.comet_punch.quantity", float64(c.Machamp.CometPunch.Quantity), 0},
		{"ash.comet_punch.quantity", float64(c.Ash.CometPunch.Quantity), 0},
		{"machamp.comet_punch.fist_duration", c.Machamp.CometPunch.FistDuration, 0},
		{"ash.comet_punch.fist_duration", c.Ash.CometPunch.FistDuration, 0},
		{"machamp.rock_tomb.duration", c.Machamp.RockTomb.Duration, 0},
		{"ash.rock_tomb.duration", c.Ash.RockTomb.Duration, 0},
		{"machamp.rock_tomb.count", float64(c.Machamp.RockTomb.Count), 0},
		{"ash.rock_tomb.count", float64(c.Ash.RockTomb.Count), 0},
		{"machamp.rock_tomb.stone_radius", c.Machamp.RockTomb.StoneRadius, 0},
		{"ash.rock_tomb.stone_radius", c.Ash.RockTomb.StoneRadius, 0},
		{"machamp.rock_tomb.stone_hp", float64(c.Machamp.RockTomb.StoneHP), 0},
		{"ash.rock_tomb.stone_hp", float64(c.Ash.RockTomb.StoneHP), 0},
		{"machamp.rock_tomb.stone_duration", c.Machamp.RockTomb.StoneDuration, 0},
		{"ash.rock_tomb.stone_duration", c.Ash.RockTomb.StoneDuration, 0},
		{"rowlet.fly_high", c.Rowlet.FlyHigh, 0},
		{"machamp.uppercut.rush_distance", c.Machamp.Uppercut.RushDistance, 0},
		{"machamp.gigantamax.lightning_punch.distance", c.Machamp.Gigantamax.LightningPunch.Distance, 0},
		{"machamp.gigantamax.lightning_punch.radius", c.Machamp.Gigantamax.LightningPunch.Radius, 0},
		{"machamp.gigantamax.lightning_punch.lightning_radius", c.Machamp.Gigantamax.LightningPunch.LightningRadius, 0},
		{"machamp.gigantamax.lightning_punch.fist_scale", c.Machamp.Gigantamax.LightningPunch.FistScale, 0},
		{"machamp.gigantamax.lightning_punch.lightning_scale", c.Machamp.Gigantamax.LightningPunch.LightningScale, 0},
		{"machamp.uppercut.rush_speed", c.Machamp.Uppercut.RushSpeed, 0},
		{"rowlet.brave_bird.dive_speed", c.Rowlet.BraveBird.DiveSpeed, 0},
		{"rowlet.brave_bird.explosion_radius", c.Rowlet.BraveBird.ExplosionRadius, 0},
		{"rowlet.brave_bird.sky_height", c.Rowlet.BraveBird.SkyHeight, 0},
		{"rowlet.brave_bird.ring_scale", c.Rowlet.BraveBird.RingScale, 0},
		{"rowlet.brave_bird.light_scale", c.Rowlet.BraveBird.LightScale, 0},
		{"rowlet.brave_bird.leaf_scale", c.Rowlet.BraveBird.LeafScale, 0},
		{"rowlet.brave_bird.tornado.hp", float64(c.Rowlet.BraveBird.Tornado.HP), 0},
		{"rowlet.brave_bird.tornado.radius", c.Rowlet.BraveBird.Tornado.Radius, 0},
		{"rowlet.brave_bird.tornado.scale", c.Rowlet.BraveBird.Tornado.Scale, 0},
		{"rowlet.brave_bird.tornado.spin_interval", c.Rowlet.BraveBird.Tornado.SpinInterval, 0},
		{"rowlet.brave_bird.tornado.lift_height", c.Rowlet.BraveBird.Tornado.LiftHeight, 0},
		{"rowlet.brave_bird.tornado.spin_duration", c.Rowlet.BraveBird.Tornado.SpinDuration, 0},
		{"rowlet.tailwind.radius", c.Rowlet.Tailwind.Radius, 0},
		{"rowlet.tailwind.speed", c.Rowlet.Tailwind.Speed, 0},
		{"rowlet.tailwind.duration", c.Rowlet.Tailwind.Duration, 0},
		{"rowlet.tailwind.slash.speed", c.Rowlet.Tailwind.Slash.Speed, 0},
		{"rowlet.tailwind.slash.radius", c.Rowlet.Tailwind.Slash.Radius, 0},
		{"rowlet.tailwind.slash.hp", float64(c.Rowlet.Tailwind.Slash.HP), 0},
		{"rowlet.tailwind.slash.scale", c.Rowlet.Tailwind.Slash.Scale, 0},
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
	for name, l := range map[string]float64{
		"court.mobile.min_length": c.Court.Mobile.MinLength, "court.web.min_length": c.Court.Web.MinLength,
		"court.mobile.max_length": c.Court.Mobile.MaxLength, "court.web.max_length": c.Court.Web.MaxLength,
	} {
		if !(l >= 2*spawnHalfGap) { // the players start spawnHalfGap either side of the middle
			return fmt.Errorf("%s must be >= %v so both players start inside it, got %v", name, 2*spawnHalfGap, l)
		}
	}
	for name, ct := range map[string]CourtConfig{"court.mobile": c.Court.Mobile, "court.web": c.Court.Web} {
		if ct.MinWidth > ct.MaxWidth || ct.MinLength > ct.MaxLength {
			return fmt.Errorf("%s: min_width x min_length (%v x %v) must not be bigger than max_width x max_length (%v x %v)", name, ct.MinWidth, ct.MinLength, ct.MaxWidth, ct.MaxLength)
		}
		if !(ct.SeaMargin >= 0) {
			return fmt.Errorf("%s: sea_margin must be >= 0, got %v", name, ct.SeaMargin)
		}
		if !(ct.ZoomInDistance >= 0) || !(ct.ZoomOutDistance >= ct.ZoomInDistance) {
			return fmt.Errorf("%s: zoom_in_distance (%v) must be >= 0 and not above zoom_out_distance (%v)", name, ct.ZoomInDistance, ct.ZoomOutDistance)
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
		{"charizard.fly.ring_count", float64(c.Charizard.Fly.RingCount)},
		{"charizard.seismic_toss.fire_radius", c.Charizard.SeismicToss.FireRadius},
		{"charizard.seismic_toss.fire_duration", c.Charizard.SeismicToss.FireDuration},
		{"charizard.seismic_toss.fire_damage", float64(c.Charizard.SeismicToss.FireDamage)},
		{"charizard.seismic_toss.fire_interval", c.Charizard.SeismicToss.FireInterval},
		{"charizard.seismic_toss.fire_max_height", c.Charizard.SeismicToss.FireMaxHeight},
		{"charizard.gigantamax.inferno.summon_duration", c.Charizard.Gigantamax.Inferno.SummonDuration},
		{"charizard.gigantamax.inferno.lock_distance", c.Charizard.Gigantamax.Inferno.LockDistance},
		{"charizard.gigantamax.inferno.damage", float64(c.Charizard.Gigantamax.Inferno.Damage)},
		{"charizard.gigantamax.inferno.launch_height", c.Charizard.Gigantamax.Inferno.LaunchHeight},
		{"charizard.gigantamax.inferno.launch_duration", c.Charizard.Gigantamax.Inferno.LaunchDuration},
		{"charizard.gigantamax.inferno.cooldown", c.Charizard.Gigantamax.Inferno.Cooldown},
		{"charizard.gigantamax.inferno.ball_damage", float64(c.Charizard.Gigantamax.Inferno.BallDamage)},
		{"charizard.gigantamax.charge_duration", c.Charizard.Gigantamax.ChargeDuration},
		{"machamp.gigantamax.charge_duration", c.Machamp.Gigantamax.ChargeDuration},
		{"pikachu.thousand_volt.hunt_hp", float64(c.Pikachu.ThousandVolt.HuntHP)},
		{"pikachu.thousand_volt.hunt_range", c.Pikachu.ThousandVolt.HuntRange},
		{"charizard.gigantamax.inferno.fire_radius", c.Charizard.Gigantamax.Inferno.FireRadius},
		{"charizard.gigantamax.inferno.fire_duration", c.Charizard.Gigantamax.Inferno.FireDuration},
		{"charizard.gigantamax.inferno.fire_damage", float64(c.Charizard.Gigantamax.Inferno.FireDamage)},
		{"charizard.gigantamax.inferno.fire_interval", c.Charizard.Gigantamax.Inferno.FireInterval},
		{"charizard.gigantamax.inferno.fire_max_height", c.Charizard.Gigantamax.Inferno.FireMaxHeight},
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
		{"rowlet.tailwind.slash.count", float64(c.Rowlet.Tailwind.Slash.Count)},
		{"rowlet.tailwind.slash.damage", float64(c.Rowlet.Tailwind.Slash.Damage)},
		{"rowlet.tailwind.slash.mass", c.Rowlet.Tailwind.Slash.Mass},
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
		{"vulpix.freeze_beam.width", c.Vulpix.FreezeBeam.Width},
		{"vulpix.freeze_beam.damage", float64(c.Vulpix.FreezeBeam.Damage)},
		{"vulpix.freeze_beam.freeze_hits", float64(c.Vulpix.FreezeBeam.FreezeHits)},
		{"vulpix.freeze_beam.freeze_duration", c.Vulpix.FreezeBeam.FreezeDuration},
		{"vulpix.frost_field.contact_damage", float64(c.Vulpix.FrostField.ContactDamage)},
		{"magikarp.jump.highest_point_duration", c.Magikarp.Jump.HighestPointDuration},
		{"magikarp.jump.cooldown", c.Magikarp.Jump.Cooldown},
		{"gyarados.wave.damage", float64(c.Gyarados.Wave.Damage)},
		{"gyarados.wave.jump_height", c.Gyarados.Wave.JumpHeight},
		{"gyarados.wave.cooldown", c.Gyarados.Wave.Cooldown},
		{"gyarados.tail_whip.knockback", c.Gyarados.TailWhip.Knockback},
		{"gyarados.tail_whip.stun_duration", c.Gyarados.TailWhip.StunDuration},
		{"gyarados.tail_whip.cooldown", c.Gyarados.TailWhip.Cooldown},
		{"gyarados.tail_whip.charge_duration", c.Gyarados.TailWhip.ChargeDuration},
		{"machamp.tree_slam.jump_height", c.Machamp.TreeSlam.JumpHeight},
		{"ash.tree_slam.jump_height", c.Ash.TreeSlam.JumpHeight},
		{"machamp.tree_slam.charge_duration", c.Machamp.TreeSlam.ChargeDuration},
		{"ash.tree_slam.charge_duration", c.Ash.TreeSlam.ChargeDuration},
		{"machamp.tree_slam.swing_duration", c.Machamp.TreeSlam.SwingDuration},
		{"ash.tree_slam.swing_duration", c.Ash.TreeSlam.SwingDuration},
		{"machamp.tree_slam.damage", float64(c.Machamp.TreeSlam.Damage)},
		{"ash.tree_slam.damage", float64(c.Ash.TreeSlam.Damage)},
		{"machamp.tree_slam.stun_duration", c.Machamp.TreeSlam.StunDuration},
		{"ash.tree_slam.stun_duration", c.Ash.TreeSlam.StunDuration},
		{"ash.megaton_punch.ring_duration", c.Ash.MegatonPunch.RingDuration},
		{"ash.megaton_punch.ball_duration", c.Ash.MegatonPunch.BallDuration},
		{"ash.megaton_punch.bolt_start", c.Ash.MegatonPunch.BoltStart},
		{"ash.megaton_punch.bolt_interval", c.Ash.MegatonPunch.BoltInterval},
		{"ash.megaton_punch.ball_follow_duration", c.Ash.MegatonPunch.BallFollowDuration},
		{"ash.megaton_punch.ball_hit_duration", c.Ash.MegatonPunch.BallHitDuration},
		{"ash.megaton_punch.flash_every", float64(c.Ash.MegatonPunch.FlashEvery)},
		{"ash.megaton_punch.damage", float64(c.Ash.MegatonPunch.Damage)},
		{"ash.megaton_punch.stun_duration", c.Ash.MegatonPunch.StunDuration},
		{"ash.megaton_punch.knockback", c.Ash.MegatonPunch.Knockback},
		{"ash.megaton_punch.lunge_distance", c.Ash.MegatonPunch.LungeDistance},
		{"machamp.uppercut.slide_distance", c.Machamp.Uppercut.SlideDistance},
		{"machamp.gigantamax.lightning_punch.charge_duration", c.Machamp.Gigantamax.LightningPunch.ChargeDuration},
		{"machamp.gigantamax.lightning_punch.damage", float64(c.Machamp.Gigantamax.LightningPunch.Damage)},
		{"machamp.gigantamax.lightning_punch.stun_duration", c.Machamp.Gigantamax.LightningPunch.StunDuration},
		{"machamp.gigantamax.lightning_punch.knockback", c.Machamp.Gigantamax.LightningPunch.Knockback},
		{"machamp.gigantamax.lightning_punch.lightning_damage", float64(c.Machamp.Gigantamax.LightningPunch.LightningDamage)},
		{"machamp.gigantamax.lightning_punch.cooldown", c.Machamp.Gigantamax.LightningPunch.Cooldown},
		{"machamp.uppercut.slide_duration", c.Machamp.Uppercut.SlideDuration},
		{"machamp.uppercut.rush_delay", c.Machamp.Uppercut.RushDelay},
		{"machamp.uppercut.radius", c.Machamp.Uppercut.Radius},
		{"machamp.uppercut.damage", float64(c.Machamp.Uppercut.Damage)},
		{"machamp.uppercut.launch_height", c.Machamp.Uppercut.LaunchHeight},
		{"machamp.uppercut.launch_duration", c.Machamp.Uppercut.LaunchDuration},
		{"machamp.uppercut.cooldown", c.Machamp.Uppercut.Cooldown},
		{"machamp.high_jump_kick.slide_distance", c.Machamp.HighJumpKick.SlideDistance},
		{"machamp.high_jump_kick.slide_duration", c.Machamp.HighJumpKick.SlideDuration},
		{"machamp.high_jump_kick.kick_delay", c.Machamp.HighJumpKick.KickDelay},
		{"machamp.high_jump_kick.kick_height", c.Machamp.HighJumpKick.KickHeight},
		{"machamp.high_jump_kick.radius", c.Machamp.HighJumpKick.Radius},
		{"machamp.high_jump_kick.damage", float64(c.Machamp.HighJumpKick.Damage)},
		{"machamp.high_jump_kick.stun_duration", c.Machamp.HighJumpKick.StunDuration},
		{"machamp.high_jump_kick.knockback", c.Machamp.HighJumpKick.Knockback},
		{"machamp.high_jump_kick.cooldown", c.Machamp.HighJumpKick.Cooldown},
		{"ash.high_jump_kick.slide_distance", c.Ash.HighJumpKick.SlideDistance},
		{"ash.high_jump_kick.slide_duration", c.Ash.HighJumpKick.SlideDuration},
		{"ash.high_jump_kick.kick_delay", c.Ash.HighJumpKick.KickDelay},
		{"ash.high_jump_kick.kick_height", c.Ash.HighJumpKick.KickHeight},
		{"ash.high_jump_kick.radius", c.Ash.HighJumpKick.Radius},
		{"ash.high_jump_kick.damage", float64(c.Ash.HighJumpKick.Damage)},
		{"ash.high_jump_kick.stun_duration", c.Ash.HighJumpKick.StunDuration},
		{"ash.high_jump_kick.knockback", c.Ash.HighJumpKick.Knockback},
		{"ash.high_jump_kick.cooldown", c.Ash.HighJumpKick.Cooldown},
		{"ash.megaton_punch.jump_distance", c.Ash.MegatonPunch.JumpDistance},
		{"ash.megaton_punch.jump_duration", c.Ash.MegatonPunch.JumpDuration},
		{"ash.megaton_punch.jump_height", c.Ash.MegatonPunch.JumpHeight},
		{"ash.megaton_punch.cooldown", c.Ash.MegatonPunch.Cooldown},
		{"machamp.tree_slam.cooldown", c.Machamp.TreeSlam.Cooldown},
		{"ash.tree_slam.cooldown", c.Ash.TreeSlam.Cooldown},
		{"machamp.comet_punch.damage", float64(c.Machamp.CometPunch.Damage)},
		{"ash.comet_punch.damage", float64(c.Ash.CometPunch.Damage)},
		{"machamp.comet_punch.cooldown", c.Machamp.CometPunch.Cooldown},
		{"ash.comet_punch.cooldown", c.Ash.CometPunch.Cooldown},
		{"machamp.comet_punch.mass", c.Machamp.CometPunch.Mass},
		{"gengar.shadow_sneak.cooldown", c.Gengar.ShadowSneak.Cooldown},
		{"gengar.shadow_sneak.shadow_speed_first_half", c.Gengar.ShadowSneak.ShadowSpeedFirstHalf},
		{"gengar.shadow_sneak.shadow_speed_second_half", c.Gengar.ShadowSneak.ShadowSpeedSecondHalf},
		{"pikachu.thousand_volt.ring_duration", c.Pikachu.ThousandVolt.RingDuration},
		{"pikachu.thousand_volt.ball_duration", c.Pikachu.ThousandVolt.BallDuration},
		{"pikachu.thousand_volt.bolt_start", c.Pikachu.ThousandVolt.BoltStart},
		{"pikachu.thousand_volt.bolt_interval", c.Pikachu.ThousandVolt.BoltInterval},
		{"pikachu.thousand_volt.link_width", c.Pikachu.ThousandVolt.LinkWidth},
		{"pikachu.thousand_volt.jump_height", c.Pikachu.ThousandVolt.JumpHeight},
		{"pikachu.thousand_volt.tip_ball_scale", c.Pikachu.ThousandVolt.TipBallScale},
		{"pikachu.thousand_volt.ring_scale", c.Pikachu.ThousandVolt.RingScale},
		{"pikachu.thousand_volt.ball_scale", c.Pikachu.ThousandVolt.BallScale},
		{"pikachu.thousand_volt.follow_speed", c.Pikachu.ThousandVolt.FollowSpeed},
		{"pikachu.thousand_volt.contact_damage", float64(c.Pikachu.ThousandVolt.ContactDamage)},
		{"pikachu.thousand_volt.rehit_interval", c.Pikachu.ThousandVolt.RehitInterval},
		{"pikachu.thousand_volt.knockback", c.Pikachu.ThousandVolt.Knockback},
		{"pikachu.thousand_volt.explosion_damage", float64(c.Pikachu.ThousandVolt.ExplosionDamage)},
		{"pikachu.thousand_volt.explosion_radius", c.Pikachu.ThousandVolt.ExplosionRadius},
		{"pikachu.thousand_volt.hits_to_explode", float64(c.Pikachu.ThousandVolt.HitsToExplode)},
		{"pikachu.ultimate.gain_taken", c.Pikachu.Ultimate.GainTaken},
		{"pikachu.ultimate.gain_dealt", c.Pikachu.Ultimate.GainDealt},
		{"pikachu.ultimate.gain_per_second", c.Pikachu.Ultimate.GainPerSecond},
		{"ash.ultimate.gain_taken", c.Ash.Ultimate.GainTaken},
		{"ash.ultimate.gain_dealt", c.Ash.Ultimate.GainDealt},
		{"ash.ultimate.gain_per_second", c.Ash.Ultimate.GainPerSecond},
		{"vulpix.ultimate.gain_taken", c.Vulpix.Ultimate.GainTaken},
		{"vulpix.ultimate.gain_dealt", c.Vulpix.Ultimate.GainDealt},
		{"vulpix.ultimate.gain_per_second", c.Vulpix.Ultimate.GainPerSecond},
		{"vulpix.ultimate.gain_ultimate_dealt", c.Vulpix.Ultimate.GainUltDealt},
		{"vulpix.glaciate.ring_duration", c.Vulpix.Glaciate.RingDuration},
		{"vulpix.glaciate.distance", c.Vulpix.Glaciate.Distance},
		{"vulpix.glaciate.icicle.damage", float64(c.Vulpix.Glaciate.Icicle.Damage)},
		{"vulpix.glaciate.icicle.slow_duration", c.Vulpix.Glaciate.Icicle.SlowDuration},
		{"chikorita.ultimate.gain_taken", c.Chikorita.Ultimate.GainTaken},
		{"chikorita.ultimate.gain_dealt", c.Chikorita.Ultimate.GainDealt},
		{"chikorita.ultimate.gain_per_second", c.Chikorita.Ultimate.GainPerSecond},
		{"chikorita.giant_vines.head_width", c.Chikorita.GiantVines.HeadWidth},
		{"chikorita.giant_vines.spread_time", c.Chikorita.GiantVines.SpreadTime},
		{"chikorita.giant_vines.interval", c.Chikorita.GiantVines.Interval},
		{"chikorita.giant_vines.charge_duration", c.Chikorita.GiantVines.ChargeDuration},
		{"rowlet.ultimate.gain_taken", c.Rowlet.Ultimate.GainTaken},
		{"rowlet.ultimate.gain_dealt", c.Rowlet.Ultimate.GainDealt},
		{"pikachu.ultimate.gain_ultimate_dealt", c.Pikachu.Ultimate.GainUltDealt},
		{"ash.ultimate.gain_ultimate_dealt", c.Ash.Ultimate.GainUltDealt},
		{"chikorita.ultimate.gain_ultimate_dealt", c.Chikorita.Ultimate.GainUltDealt},
		{"rowlet.ultimate.gain_ultimate_dealt", c.Rowlet.Ultimate.GainUltDealt},
		{"machamp.ultimate.gain_taken", c.Machamp.Ultimate.GainTaken},
		{"machamp.ultimate.gain_dealt", c.Machamp.Ultimate.GainDealt},
		{"machamp.ultimate.gain_ultimate_dealt", c.Machamp.Ultimate.GainUltDealt},
		{"machamp.ultimate.gain_per_second", c.Machamp.Ultimate.GainPerSecond},
		{"charizard.ultimate.gain_taken", c.Charizard.Ultimate.GainTaken},
		{"charizard.ultimate.gain_dealt", c.Charizard.Ultimate.GainDealt},
		{"charizard.ultimate.gain_ultimate_dealt", c.Charizard.Ultimate.GainUltDealt},
		{"charizard.ultimate.gain_per_second", c.Charizard.Ultimate.GainPerSecond},
		{"rowlet.ultimate.gain_per_second", c.Rowlet.Ultimate.GainPerSecond},
		{"rowlet.brave_bird.ring_duration", c.Rowlet.BraveBird.RingDuration},
		{"rowlet.brave_bird.charge_duration", c.Rowlet.BraveBird.ChargeDuration},
		{"rowlet.brave_bird.damage", float64(c.Rowlet.BraveBird.Damage)},
		{"rowlet.brave_bird.launch_height", c.Rowlet.BraveBird.LaunchHeight},
		{"rowlet.brave_bird.lock_distance", c.Rowlet.BraveBird.LockDistance},
		{"rowlet.brave_bird.launch_duration", c.Rowlet.BraveBird.LaunchDuration},
		{"rowlet.brave_bird.tornado.duration", c.Rowlet.BraveBird.Tornado.Duration},
		{"rowlet.brave_bird.tornado.spin_damage", float64(c.Rowlet.BraveBird.Tornado.SpinDamage)},
		{"rowlet.brave_bird.tornado.landing_damage", float64(c.Rowlet.BraveBird.Tornado.LandingDamage)},
		{"rowlet.brave_bird.sky_dark", c.Rowlet.BraveBird.SkyDark},
		{"rowlet.brave_bird.flash_every", c.Rowlet.BraveBird.FlashEvery},
		{"rowlet.brave_bird.flash_alpha", c.Rowlet.BraveBird.FlashAlpha},
		{"chikorita.giant_vines.swing_duration", c.Chikorita.GiantVines.SwingDuration},
		{"chikorita.giant_vines.damage", float64(c.Chikorita.GiantVines.Damage)},
		{"chikorita.giant_vines.slow_duration", c.Chikorita.GiantVines.SlowDuration},
		{"chikorita.giant_vines.stun_hits", float64(c.Chikorita.GiantVines.StunHits)},
		{"chikorita.giant_vines.stun_duration", c.Chikorita.GiantVines.StunDuration},
		{"chikorita.giant_vines.ring_scale", c.Chikorita.GiantVines.RingScale},
		{"chikorita.giant_vines.tip_width", c.Chikorita.GiantVines.TipWidth},
		{"chikorita.giant_vines.linger", c.Chikorita.GiantVines.Linger},
		{"gengar.ghost_fire.cooldown", c.Gengar.GhostFire.Cooldown},
		{"gengar.ghost_fire.rehit_interval", c.Gengar.GhostFire.RehitInterval},
		{"gengar.ghost_fire.damage", float64(c.Gengar.GhostFire.Damage)},
		{"gengar.ghost_fire.mass", c.Gengar.GhostFire.Mass},
		{"gengar.shadow_ball.damage", float64(c.Gengar.ShadowBall.Damage)},
		{"gengar.night_shade.damage", float64(c.Gengar.NightShade.Damage)},
		{"gengar.night_shade.height", c.Gengar.NightShade.Height},
		{"gengar.night_shade.cooldown", c.Gengar.NightShade.Cooldown},
		{"gengar.night_shade.ghost_fire_count", float64(c.Gengar.NightShade.GhostFireCount)}, // none is allowed: the blast alone
		{"gengar.night_shade.ghost_fire_damage", float64(c.Gengar.NightShade.GhostFireDamage)},
		{"gengar.shadow_ball.cooldown", c.Gengar.ShadowBall.Cooldown},
		{"gengar.shadow_ball.charge_time", c.Gengar.ShadowBall.ChargeTime},
		{"gengar.shadow_ball.mass", c.Gengar.ShadowBall.Mass},
		{"rowlet.brave_bird.gravity_radius", c.Rowlet.BraveBird.GravityRadius},
		{"rowlet.brave_bird.gravity", c.Rowlet.BraveBird.Gravity},
		{"rowlet.brave_bird.pull_duration", c.Rowlet.BraveBird.PullDuration},
		{"gengar.shadow_ball.gravity", c.Gengar.ShadowBall.Gravity},
		{"gengar.shadow_ball.pull_duration", c.Gengar.ShadowBall.PullDuration},
		{"ash.comet_punch.mass", c.Ash.CometPunch.Mass},
		{"machamp.comet_punch.ball_knockback", c.Machamp.CometPunch.BallKnockback},
		{"ash.comet_punch.ball_knockback", c.Ash.CometPunch.BallKnockback},
		{"machamp.comet_punch.knockback", c.Machamp.CometPunch.Knockback},
		{"ash.comet_punch.knockback", c.Ash.CometPunch.Knockback},
		{"machamp.comet_punch.distance", c.Machamp.CometPunch.Distance},
		{"ash.comet_punch.distance", c.Ash.CometPunch.Distance},
		{"machamp.comet_punch.scatter_min_distance", c.Machamp.CometPunch.ScatterMinDistance},
		{"ash.comet_punch.scatter_min_distance", c.Ash.CometPunch.ScatterMinDistance},
		{"machamp.rock_tomb.ring_radius", c.Machamp.RockTomb.RingRadius},
		{"ash.rock_tomb.ring_radius", c.Ash.RockTomb.RingRadius},
		{"machamp.rock_tomb.height", c.Machamp.RockTomb.Height},
		{"ash.rock_tomb.height", c.Ash.RockTomb.Height},
		{"machamp.rock_tomb.cooldown", c.Machamp.RockTomb.Cooldown},
		{"ash.rock_tomb.cooldown", c.Ash.RockTomb.Cooldown},
		{"machamp.rock_tomb.air_speed_first_half", c.Machamp.RockTomb.AirSpeedFirstHalf},
		{"ash.rock_tomb.air_speed_first_half", c.Ash.RockTomb.AirSpeedFirstHalf},
		{"machamp.rock_tomb.air_speed_second_half", c.Machamp.RockTomb.AirSpeedSecondHalf},
		{"ash.rock_tomb.air_speed_second_half", c.Ash.RockTomb.AirSpeedSecondHalf},
		{"vulpix.frost_field.explosion_radius", c.Vulpix.FrostField.ExplosionRadius},
		{"vulpix.frost_field.explosion_damage", float64(c.Vulpix.FrostField.ExplosionDamage)},
	} {
		if !(f.v >= 0) {
			return fmt.Errorf("%s must be >= 0, got %v", f.name, f.v)
		}
	}
	if n := c.Pikachu.ThousandVolt.BoltCount; n < 0 || n > 20 {
		return fmt.Errorf("pikachu.thousand_volt.bolt_count must be 0..20, got %d", n)
	}
	if tv := c.Pikachu.ThousandVolt; tv.SkyDark < 0 || tv.SkyDark > 1 || tv.FlashAlpha < 0 || tv.FlashAlpha > 1 || tv.FlashEvery < 0 {
		return fmt.Errorf("pikachu.thousand_volt: sky_dark and flash_alpha must be 0..1 and flash_every >= 0, got %v, %v, %d", tv.SkyDark, tv.FlashAlpha, tv.FlashEvery)
	}
	if len(c.Pikachu.ThousandVolt.BoltColors) == 0 {
		return errors.New("pikachu.thousand_volt.bolt_colors needs at least one colour")
	}
	for _, s := range c.Pikachu.ThousandVolt.BoltColors {
		if _, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32); err != nil || len(s) != 7 || s[0] != '#' {
			return fmt.Errorf("pikachu.thousand_volt.bolt_colors: %q is not #rrggbb", s)
		}
	}
	if n := c.Chikorita.GiantVines.Count; n < 1 || n > maxGiantVines {
		return fmt.Errorf("chikorita.giant_vines.count must be 1..%d, got %d", maxGiantVines, n)
	}
	if n := c.Pikachu.ThousandVolt.Count; n > maxVoltBolts {
		return fmt.Errorf("pikachu.thousand_volt.count must be at most %d, got %d", maxVoltBolts, n)
	}
	if n := c.Ash.MegatonPunch.BoltCount; n < 0 || n > 20 {
		return fmt.Errorf("ash.megaton_punch.bolt_count must be 0..20, got %d", n)
	}
	for name, r := range map[string]Reach{
		"machamp.high_jump_kick":             c.Machamp.HighJumpKick.Reach,
		"ash.high_jump_kick":                 c.Ash.HighJumpKick.Reach,
		"pikachu.volt_tackle":                c.Pikachu.VoltTackle.Reach,
		"pikachu.thunder":                    c.Pikachu.Thunder.Reach,
		"pikachu.electro_ball":               c.Pikachu.ElectroBall.Reach,
		"pikachu.thousand_volt":              c.Pikachu.ThousandVolt.Reach,
		"snorlax.jump":                       c.Snorlax.Jump.Reach,
		"snorlax.ice_punch":                  c.Snorlax.IcePunch.Reach,
		"snorlax.hyper_beam":                 c.Snorlax.HyperBeam.Reach,
		"gyarados.hyper_beam":                c.Gyarados.HyperBeam.Reach,
		"charizard.fire_breath":              c.Charizard.FireBreath.Reach,
		"charizard.seismic_toss":             c.Charizard.SeismicToss.Reach,
		"charizard.gigantamax.inferno":       c.Charizard.Gigantamax.Inferno.Reach,
		"chikorita.leaf_slash":               c.Chikorita.LeafSlash.Reach,
		"chikorita.razor_leaf":               c.Chikorita.RazorLeaf.Reach,
		"chikorita.vine_whip":                c.Chikorita.VineWhip.Reach,
		"chikorita.giant_vines":              c.Chikorita.GiantVines.Reach,
		"rowlet.brave_bird":                  c.Rowlet.BraveBird.Reach,
		"vulpix.frost_field":                 c.Vulpix.FrostField.Reach,
		"vulpix.ice_storm":                   c.Vulpix.IceStorm.Reach,
		"vulpix.freeze_beam":                 c.Vulpix.FreezeBeam.Reach,
		"vulpix.glaciate":                    c.Vulpix.Glaciate.Reach,
		"rowlet.tailwind.slash":              c.Rowlet.Tailwind.Slash.Reach,
		"rowlet.arrows":                      c.Rowlet.Arrows.Reach,
		"rowlet.tornado":                     c.Rowlet.Tornado.Reach,
		"rowlet.brave_bird.tornado":          c.Rowlet.BraveBird.Tornado.Reach,
		"gyarados.wave":                      c.Gyarados.Wave.Reach,
		"gyarados.tail_whip":                 c.Gyarados.TailWhip.Reach,
		"machamp.tree_slam":                  c.Machamp.TreeSlam.Reach,
		"ash.tree_slam":                      c.Ash.TreeSlam.Reach,
		"machamp.comet_punch":                c.Machamp.CometPunch.Reach,
		"ash.comet_punch":                    c.Ash.CometPunch.Reach,
		"ash.megaton_punch":                  c.Ash.MegatonPunch.Reach,
		"machamp.rock_tomb":                  c.Machamp.RockTomb.Reach,
		"machamp.uppercut":                   c.Machamp.Uppercut.Reach,
		"machamp.gigantamax.lightning_punch": c.Machamp.Gigantamax.LightningPunch.Reach,
		"gengar.shadow_sneak":                c.Gengar.ShadowSneak.Reach,
		"gengar.ghost_fire":                  c.Gengar.GhostFire.Reach,
		"gengar.shadow_ball":                 c.Gengar.ShadowBall.Reach,
		"gengar.night_shade":                 c.Gengar.NightShade.Reach,
		"ash.rock_tomb":                      c.Ash.RockTomb.Reach,
	} {
		if !(r.MinHeight >= -minHeightFloor && r.MaxHeight >= r.MinHeight) { // also rejects NaN
			return fmt.Errorf("%s: need %v <= min_height <= max_height, got %v and %v", name, -minHeightFloor, r.MinHeight, r.MaxHeight)
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
	if n := c.Rowlet.Tailwind.Slash.Count; n > 100 {
		return fmt.Errorf("rowlet.tailwind.slash.count must be at most 100, got %d", n)
	}
	if n := c.Rowlet.Arrows.Count; n > 100 {
		return fmt.Errorf("rowlet.arrows.count must be at most 100, got %d", n)
	}
	if gr := c.Gengar.ShadowBall.GravityRadius; !(gr >= 0) || math.IsInf(gr, 0) {
		return fmt.Errorf("gengar.shadow_ball.gravity_radius must be 0 (the same as slam_radius) or more, got %v", gr)
	}
	if jp := c.Snorlax.Jump; jp.Damage < 0 || !(jp.EarthquakeRadius >= 0) || (jp.EarthquakeRadius > 0 &&
		(jp.EarthquakeRadius < jp.SlamRadius || !(jp.EarthquakeDuration > 0) || !(jp.EarthquakeInterval > 0) || jp.EarthquakeDamage < 0 || !(jp.EarthquakeSlow >= 0 && jp.EarthquakeSlow < 1))) {
		return fmt.Errorf("snorlax.jump: damage (%d) must not be negative; an earthquake_radius (%v) above 0 needs to be at least slam_radius (%v), earthquake_duration (%v) and earthquake_interval (%v) above 0, earthquake_damage (%d) not negative and earthquake_slow (%v) from 0 up to (not including) 1", jp.Damage, jp.EarthquakeRadius, jp.SlamRadius, jp.EarthquakeDuration, jp.EarthquakeInterval, jp.EarthquakeDamage, jp.EarthquakeSlow)
	}
	if n := c.Vulpix.Glaciate.Count; n < 1 || n > maxIcicles {
		return fmt.Errorf("vulpix.glaciate.count must be 1..%d, got %d", maxIcicles, n)
	}
	if v := c.Vulpix.Glaciate.Icicle.Slow; !(v >= 0 && v <= 1) {
		return fmt.Errorf("vulpix.glaciate.icicle.slow must be between 0 and 1, got %v", v)
	}
	if v := c.Chikorita.GiantVines.Slow; !(v >= 0 && v <= 1) {
		return fmt.Errorf("chikorita.giant_vines.slow must be between 0 and 1, got %v", v)
	}
	if c.Chikorita.GiantVines.TipWidth > c.Chikorita.GiantVines.TrunkWidth {
		return fmt.Errorf("chikorita.giant_vines.tip_width (%v) must not be wider than trunk_width (%v)", c.Chikorita.GiantVines.TipWidth, c.Chikorita.GiantVines.TrunkWidth)
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
	if err := c.Ash.validate("ash"); err != nil {
		return err
	}
	if err := c.Machamp.validate("machamp"); err != nil {
		return err
	}
	if h := c.Gengar.ShadowSneak.Height; !(h <= 0 && h >= -minHeightFloor) { // below the ground (0 = just on it)
		return fmt.Errorf("gengar.shadow_sneak.height must be between %v and 0, got %v", -minHeightFloor, h)
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
	case ash:
		return c.Ash.MaxHP
	case machamp:
		return c.Machamp.MaxHP
	case gengar:
		return c.Gengar.MaxHP
	case gyarados:
		return c.Gyarados.MaxHP
	case dummy:
		return c.Dummy.MaxHP
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
	case ash:
		return c.Ash.WalkSpeed
	case machamp:
		return c.Machamp.WalkSpeed
	case gengar:
		return c.Gengar.WalkSpeed
	case gyarados:
		return c.Gyarados.WalkSpeed
	case dummy:
		return 0.0
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
	case ash:
		return c.Ash.Radius
	case machamp:
		return c.Machamp.Radius
	case gengar:
		return c.Gengar.Radius
	case gyarados:
		return c.Gyarados.Radius
	case dummy:
		return c.Dummy.Radius
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
func (c Config) courtFor(mode string) CourtSize { return c.courtConfigFor(mode).size() }

// courtConfigFor is the whole court section (field, camera window and zoom) of that view.
func (c Config) courtConfigFor(mode string) CourtConfig {
	if mode == modeWeb {
		return c.Court.Web
	}
	return c.Court.Mobile
}

// validate checks the values that have a range of their own; name is the section's name in the file.
func (b *BrawlerCfg) validate(name string) error {
	if a := b.TreeSlam.LeafAngle; !(a > 0 && a <= 360) {
		return fmt.Errorf("%s.tree_slam.leaf_angle must be between 0 (exclusive) and 360, got %v", name, a)
	}
	if cp := b.CometPunch; cp.Quantity > 200 {
		return fmt.Errorf("%s.comet_punch.quantity must be at most 200, got %d", name, cp.Quantity)
	} else if !(cp.ScatterMaxDistance >= cp.ScatterMinDistance) {
		return fmt.Errorf("%s.comet_punch.scatter_max_distance must be >= scatter_min_distance, got %v and %v", name, cp.ScatterMaxDistance, cp.ScatterMinDistance)
	} else if !(cp.ScatterNearBias >= 1 && cp.ScatterNearBias <= 10) {
		return fmt.Errorf("%s.comet_punch.scatter_near_bias must be between 1 and 10, got %v", name, cp.ScatterNearBias)
	} else if !(cp.RandomOffsetAngle >= 0 && cp.RandomOffsetAngle <= 90) {
		return fmt.Errorf("%s.comet_punch.random_offset_angle must be between 0 and 90, got %v", name, cp.RandomOffsetAngle)
	}
	if n := b.RockTomb.Count; n > 100 {
		return fmt.Errorf("%s.rock_tomb.count must be at most 100, got %d", name, n)
	}
	return nil
}

// brawlerOf is the numbers of the brawler of this kind (Machamp's own, Ash's for anyone else).
func (c *Config) brawlerOf(kind int) *BrawlerCfg {
	if kind == machamp {
		return &c.Machamp
	}
	return &c.Ash
}
