package vaporwave

import (
	"fmt"
	"math"
	"math/rand"
)

// Game phases.
const (
	PhaseFlight = iota // fly through rings, tempo climbs towards 250 BPM
	PhaseBoom          // everything explodes
	PhaseChill         // 30 s of calm
	PhaseCreate        // every action makes music and changes the level
)

var PhaseNames = []string{"ＦＬＩＧＨＴ", "ＢＯＯＭ", "ＣＨＩＬＬ", "ＣＲＥＡＴＥ"}

const (
	BoomBPM      = 250
	ChillSecs    = 30
	shipAhead    = 5.5  // ship sits this far in front of the camera
	ringRadius   = 2.6  // generous: there is no penalty for missing
	ringBeats    = 8    // rings every two bars
	shipMaxX     = 12.0 //
	shipMinY     = 1.0
	shipMaxY     = 11.0
	maxBeacons   = 160
	maxParticles = 420
)

// Directions for GameInput.Pressed.
const (
	DirUp = iota
	DirDown
	DirLeft
	DirRight
)

// GameInput is everything the player did since the last update.
type GameInput struct {
	DX, DY  float32 // held direction, -1..1
	Pressed []int   // fresh key presses (Dir*)
	Click   bool
	ClickX  float32
	ClickZ  float64
	Space   bool
	Clear   bool
	Wheel   float32
}

type GRing struct {
	X, Y  float32
	Z     float64
	State int // 0 ahead, 1 hit, 2 missed
	T     float64
}

type Beacon struct {
	X, Y  float32
	Z     float64
	Pitch int
	T     float64
}

type Particle struct {
	X, Y       float32
	Z          float64
	VX, VY, VZ float32
	C          Col
}

// Game is the whole game state. It drives the audio engine and owns the scene.
type Game struct {
	Eng  *Engine
	Sc   *Scene
	Song *Song
	Opts SongOpts
	rng  *rand.Rand
	seed uint32

	Phase         int
	T, PhaseT     float64
	SceneStart    float64
	Dist          float64 // camera z
	X, Y, VX, VY  float32 // ship (Y is height above the ground)
	Base, Target  float64 // tempo
	drift         float64
	Rings         []GRing
	nextRingZ     float64
	ringX, ringY  float32
	Hits, Misses  int
	Beacons       []Beacon
	Particles     []Particle
	ClickRings    []Ring
	Toast         string
	ToastT        float64
	Flash, Glitch float32
	noteIdx       int
	createDrums   int
	drumsMuted    bool
	createHits    int
	createScene   int
	boomSwapped   bool
	Notes         int
}

// NewGame starts a fresh run. style < 0 picks a random style.
func NewGame(eng *Engine, seed uint32, style, mood int) *Game {
	g := &Game{Eng: eng, rng: rand.New(rand.NewSource(int64(seed))), seed: seed, Y: 4}
	if style < 0 {
		style = g.rng.Intn(NumStyles)
	}
	g.Opts = SongOpts{Style: style, Drums: 1, Tempo: 1, Mood: mood, Melody: 1}
	g.Sc = NewScene(seed)
	g.Song = NewSongOpts(seed, g.Opts)
	g.Sc.Song = g.Song
	g.Base = clampF(g.Song.BPM, 72, 104)
	g.Target = g.Base
	eng.ClearLoop()
	eng.NewSong(g.Song)
	eng.SetTempo(g.Base)
	g.ringX, g.ringY = 0, 4
	g.nextRingZ = 70
	g.say("ＦＬＩＧＨＴ — fly through the rings  ·  WASD")
	return g
}

func (g *Game) say(s string) { g.Toast, g.ToastT = s, g.T }

// ShipZ is the world z of the ship.
func (g *Game) ShipZ() float64 { return g.Dist + shipAhead }

func (g *Game) setSong() {
	g.Song = NewSongOpts(g.seed, g.Opts)
	g.Sc.Song = g.Song
	g.Eng.SwapSong(g.Song)
}

func (g *Game) setScene(sc *Scene) {
	sc.Song = g.Song
	g.Sc = sc
	g.SceneStart = g.T
}

// mutate applies one small change to the world and returns its name.
func (g *Game) mutate(k int) string {
	sc := *g.Sc
	defer func() { g.Sc = &sc }()
	switch k % 8 {
	case 0:
		sc.P = sc.P.rot(18)
		sc.setTerrCols()
		return "hue shift"
	case 1:
		sc.SunR = float32(math.Min(float64(sc.SunR)*1.12, 0.4))
		sc.Stripes = float32(math.Min(float64(sc.Stripes)+1, 14))
		if sc.SunKind == 2 {
			sc.SunKind = 0
		}
		return "the sun grows"
	case 2:
		sc.Density = float32(math.Min(float64(sc.Density)+0.07, 0.65))
		if len(sc.Outer) == 0 {
			sc.Outer = []int{PropDiamond}
		}
		return "more things"
	case 3:
		sc.Gloss = float32(math.Min(float64(sc.Gloss)+0.15, 0.85))
		return "shinier floor"
	case 4:
		sc.Gate = sc.Gate%4 + 1
		sc.GateBeats = 4
		return "new gates"
	case 5:
		sc.P = sc.P.rot(-24)
		sc.P.Bloom *= 1.08
		sc.setTerrCols()
		return "colour drift"
	case 6:
		if !sc.Space {
			switch sc.FloorType {
			case FloorGrid:
				sc.FloorType = FloorChecker
			case FloorChecker:
				sc.FloorType = FloorWater
				sc.Gloss = 0.8
			default:
				sc.FloorType = FloorGrid
			}
			return "the floor changes"
		}
		sc.P = sc.P.rot(30)
		return "hue shift"
	default:
		if sc.MoonKind == 0 {
			sc.MoonKind = 1
			sc.MoonDir = dirFrom(0.7, 0.45)
			sc.MoonR = 0.06
			return "a moon rises"
		}
		sc.Avenue = []int{PropPalm, PropColumn, -1}[k/8%3]
		return "new avenue"
	}
}

// musicStep is the flight-phase music plan; it returns a description or "".
func (g *Game) musicStep(hit int) string {
	switch {
	case hit == 3 || hit == 9 || hit == 18:
		g.Opts.Drums = min(g.Opts.Drums+1, 4)
		g.setSong()
		return "drums " + DrumNames[g.Opts.Drums]
	case hit == 6 || hit == 15:
		g.Opts.Melody = min(g.Opts.Melody+1, 3)
		g.setSong()
		return "melody " + MelodyNames[g.Opts.Melody]
	case hit%12 == 0:
		g.Opts.Style = (g.Opts.Style + 1 + g.rng.Intn(NumStyles-1)) % NumStyles
		g.setSong()
		return StyleNames[g.Opts.Style]
	case hit >= 21 && hit%6 == 3:
		g.seed++
		g.setSong()
		return "new chords"
	}
	return ""
}

// Update advances the game by dt seconds.
func (g *Game) Update(dt float64, in GameInput) {
	if dt <= 0 {
		return
	}
	g.T += dt
	g.PhaseT += dt
	tempo := g.Eng.Tempo()
	if tempo <= 0 {
		tempo = g.Target
	}
	// sync the visual song tempo (used for the beat-pulse decay times)
	g.Song.BPM = tempo
	g.Sc.Song = g.Song

	// travel and ship
	speed := tempo / 60 * float64(g.Sc.U)
	if g.Phase == PhaseChill {
		speed *= 0.6
	}
	g.Dist += speed * dt
	fdt := float32(dt)
	k := clamp01(fdt * 6)
	g.VX += (in.DX*16 - g.VX) * k
	g.VY += (in.DY*10 - g.VY) * k
	g.X = clampf(g.X+g.VX*fdt, -shipMaxX, shipMaxX)
	g.Y = clampf(g.Y+g.VY*fdt, shipMinY, shipMaxY)

	g.Flash *= float32(math.Exp(-dt * 2.2))
	g.Glitch *= float32(math.Exp(-dt * 3))
	for i := range g.ClickRings {
		_ = i
	}
	for len(g.ClickRings) > 0 && g.T-g.ClickRings[0].T0 > 2.5 {
		g.ClickRings = g.ClickRings[1:]
	}
	g.updateParticles(dt)

	switch g.Phase {
	case PhaseFlight:
		g.drift += dt * (0.35 + 0.002*g.PhaseT)
		g.Target = math.Min(g.Base+2*float64(g.Hits)+g.drift, BoomBPM+2)
		g.Eng.SetTempo(g.Target)
		g.rings(true)
		if tempo >= BoomBPM-0.5 {
			g.startBoom()
		}
	case PhaseBoom:
		if !g.boomSwapped && g.PhaseT > 0.45 {
			g.boomSwapped = true
			g.Opts = SongOpts{Style: StyleVapor, Drums: 0, Tempo: 1, Mood: 2, Melody: 0}
			g.seed += 101
			g.setScene(NewSpaceScene(g.seed, 1))
			g.setSong()
			g.Eng.SetTempo(58)
		}
		if g.PhaseT > 3 {
			g.Phase, g.PhaseT = PhaseChill, 0
			g.say("ＣＨＩＬＬ — just float")
		}
	case PhaseChill:
		if g.PhaseT >= ChillSecs {
			g.startCreate()
		}
	case PhaseCreate:
		g.create(in)
		g.rings(false)
	}
	if in.Click {
		g.ClickRings = append(g.ClickRings, Ring{in.ClickX, in.ClickZ, g.T})
		g.Eng.Chime()
		if g.Phase == PhaseCreate {
			g.seed++
			g.setSong()
			sc := *g.Sc
			sc.P = sc.P.rot(30)
			sc.setTerrCols()
			g.Sc = &sc
			g.say("new chords · new colours")
		}
	}
}

// rings spawns, passes and retires rings.
func (g *Game) rings(flight bool) {
	spacing := ringBeats * float64(g.Sc.U)
	for g.nextRingZ < g.Dist+300 {
		g.ringX = clampf(g.ringX+(g.rng.Float32()-0.5)*14, -10, 10)
		g.ringY = clampf(g.ringY+(g.rng.Float32()-0.5)*7, 1.8, 9)
		g.Rings = append(g.Rings, GRing{X: g.ringX, Y: g.ringY, Z: g.nextRingZ})
		g.nextRingZ += spacing
	}
	sz := g.ShipZ()
	for i := range g.Rings {
		r := &g.Rings[i]
		if r.State != 0 || sz < r.Z {
			continue
		}
		dx, dy := g.X-r.X, g.Y-r.Y
		r.T = g.T
		if dx*dx+dy*dy < ringRadius*ringRadius {
			r.State = 1
			g.hit(flight)
		} else {
			r.State = 2
			g.Misses++
		}
	}
	for len(g.Rings) > 0 && g.Rings[0].Z < g.Dist-30 {
		g.Rings = g.Rings[1:]
	}
}

func (g *Game) hit(flight bool) {
	g.Hits++
	g.Flash = max(g.Flash, 0.12)
	g.Eng.Chime()
	if flight {
		msg := g.musicStep(g.Hits)
		w := g.mutate(g.Hits - 1)
		if msg == "" {
			msg = "+2 BPM"
		}
		g.say(fmt.Sprintf("RING %d  ·  %s  ·  %s", g.Hits, msg, w))
		return
	}
	// create phase: rings remix everything
	g.createHits++
	var msg string
	switch {
	case g.createHits%3 == 0:
		g.Opts.Style = (g.Opts.Style + 1 + g.rng.Intn(NumStyles-1)) % NumStyles
		msg = StyleNames[g.Opts.Style]
	default:
		g.createDrums = (g.createDrums + 1) % 5
		g.drumsMuted = false
		g.Opts.Drums = g.createDrums
		msg = "drums " + DrumNames[g.createDrums]
	}
	g.setSong()
	w := g.mutate(g.createHits + 3)
	if g.createHits%4 == 0 {
		g.createScene++
		var sc *Scene
		if g.createScene%2 == 1 {
			sc = NewSpaceScene(g.seed+uint32(g.createScene), g.rng.Intn(NumWorlds()))
		} else {
			sc = NewScene(g.seed + uint32(g.createScene)*7)
		}
		g.setScene(sc)
		w = "new world: " + sc.Name
	}
	g.say(msg + "  ·  " + w)
}

func (g *Game) startBoom() {
	g.Phase, g.PhaseT = PhaseBoom, 0
	g.Eng.Boom()
	g.Flash, g.Glitch = 1.6, 1
	g.Rings = nil
	g.say("２５０ ＢＰＭ")
	// the world flies apart: debris from the ship and the scenery
	g.Particles = g.Particles[:0]
	cols := []Col{g.Sc.P.Line, g.Sc.P.Acc, g.Sc.P.SunT, {1, 1, 1}}
	for i := 0; i < maxParticles; i++ {
		th := g.rng.Float64() * 2 * math.Pi
		ph := math.Acos(2*g.rng.Float64() - 1)
		sp := float32(8 + 45*g.rng.Float64())
		p := Particle{X: g.X, Y: g.Y, Z: g.ShipZ(), C: cols[i%len(cols)]}
		if i%3 == 0 { // scenery debris further out
			p.X += (g.rng.Float32() - 0.5) * 60
			p.Y = g.rng.Float32() * 14
			p.Z += g.rng.Float64() * 120
		}
		p.VX = sp * float32(math.Sin(ph)*math.Cos(th))
		p.VY = sp * float32(math.Cos(ph))
		p.VZ = sp * float32(math.Sin(ph)*math.Sin(th))
		g.Particles = append(g.Particles, p)
	}
}

func (g *Game) updateParticles(dt float64) {
	f := float32(math.Exp(-dt * 0.6))
	fdt := float32(dt)
	n := 0
	for _, p := range g.Particles {
		p.X += p.VX * fdt
		p.Y += p.VY * fdt
		p.Z += float64(p.VZ * fdt)
		p.VX, p.VY, p.VZ = p.VX*f, p.VY*f, p.VZ*f
		if p.Z > g.Dist-20 {
			g.Particles[n] = p
			n++
		}
	}
	g.Particles = g.Particles[:n]
}

func (g *Game) startCreate() {
	g.Phase, g.PhaseT = PhaseCreate, 0
	g.Particles = nil
	g.seed += 7
	g.Opts = SongOpts{Style: g.rng.Intn(NumStyles), Drums: 0, Tempo: 1, Mood: 0, Melody: 0}
	g.setScene(NewScene(g.seed))
	g.setSong()
	g.Target = 92
	g.Eng.SetTempo(g.Target)
	g.nextRingZ = g.Dist + 90
	g.ringX, g.ringY = g.X, g.Y
	g.noteIdx = len(g.Song.scale) + 2
	g.say("ＣＲＥＡＴＥ — every move makes music now")
}

// create handles the free-play phase.
func (g *Game) create(in GameInput) {
	s := g.Song
	L := len(s.scale)
	for _, d := range in.Pressed {
		switch d {
		case DirUp:
			g.noteIdx += 2
		case DirDown:
			g.noteIdx -= 2
		case DirLeft:
			g.noteIdx--
		case DirRight:
			g.noteIdx++
		}
		g.noteIdx = clampI(g.noteIdx, 0, 2*L+2)
		midi := s.leadBase - 12 + 12*(g.noteIdx/L) + s.scale[g.noteIdx%L]
		g.Eng.PlayNote(midi)
		g.Notes++
		if len(g.Beacons) >= maxBeacons {
			g.Beacons = g.Beacons[1:]
		}
		g.Beacons = append(g.Beacons, Beacon{X: g.X, Y: g.Y, Z: g.ShipZ() + 4, Pitch: midi, T: g.T})
		sc := *g.Sc
		sc.P = sc.P.rot(float32(3 * (d%2*2 - 1)))
		g.Sc = &sc
	}
	if in.Space {
		g.drumsMuted = !g.drumsMuted
		if g.drumsMuted {
			g.Opts.Drums = 0
			g.say("drums out")
		} else {
			g.createDrums = max(g.createDrums, 2)
			g.Opts.Drums = g.createDrums
			g.say("drums in")
		}
		g.setSong()
		g.Flash = max(g.Flash, 0.2)
	}
	if in.Wheel != 0 {
		g.Target = clampF(g.Target+float64(in.Wheel)*4, 60, 200)
		g.Eng.SetTempo(g.Target)
		g.say(fmt.Sprintf("%.0f BPM", g.Target))
	}
	if in.Clear {
		g.Eng.ClearLoop()
		g.Beacons = nil
		g.say("loop cleared")
	}
}

// MakeFrame builds the render frame: a chase camera behind the ship.
func (g *Game) MakeFrame(beat, now float64, n uint32) Frame {
	camX := g.X * 0.75
	camH := g.Y*0.75 + 1.55
	dx := float64(g.X - camX)
	dy := float64(g.Y + 0.4 - camH)
	dz := float64(shipAhead + 9)
	p := Player{X: camX, H: camH, Yaw: float32(math.Atan2(dx, dz)), Pitch: float32(math.Atan2(dy, math.Hypot(dx, dz))),
		Roll: -g.VX * 0.012, Rings: g.ClickRings}
	fr := MakeFrame(g.Sc, &p, beat, now, g.T-g.SceneStart+5, n)
	if g.T-g.SceneStart < 0.45 {
		a := float32(1 - (g.T-g.SceneStart)/0.45)
		fr.Glitch = max(fr.Glitch, a)
	}
	fr.UseCamZ, fr.CamZ, fr.Game = true, g.Dist, g
	fr.Flash += g.Flash * g.Flash * 0.6
	fr.Glitch = max(fr.Glitch, g.Glitch)
	if g.Phase == PhaseFlight {
		// a little VHS jitter as the tempo nears the limit
		fr.Glitch = max(fr.Glitch, float32(clampF((g.Eng.Tempo()-215)/35, 0, 1))*0.12)
	}
	return fr
}

// ChillLeft is the seconds remaining in the chill phase.
func (g *Game) ChillLeft() int { return int(math.Ceil(ChillSecs - g.PhaseT)) }
