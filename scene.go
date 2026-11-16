package main

import (
	"math"
	"math/rand"
)

type Palette struct {
	Name                                 string
	Top, Mid, Hor, SunT, SunB, Line, Acc Col
	Floor, Tile                          Col
	Dark                                 bool
	Bloom                                float32
}

func pal(name string, top, mid, hor, sunT, sunB, line, acc, floor, tile uint32, dark bool, bloom float32) Palette {
	return Palette{name, hexc(top), hexc(mid), hexc(hor), hexc(sunT), hexc(sunB), hexc(line), hexc(acc), hexc(floor), hexc(tile), dark, bloom}
}

var palettes = []Palette{
	pal("CLASSIC", 0x12042b, 0x5b1a8c, 0xff5fa2, 0xffe96b, 0xff2e97, 0x00f0ff, 0xff3cf0, 0x0d0221, 0xf7a8d8, true, 1.0),
	pal("MIAMI", 0x2b1055, 0x7b2f9e, 0xffa46b, 0xfff3a0, 0xff5e62, 0xff71ce, 0x01cdfe, 0x1b0a2a, 0xffd1dc, true, 1.0),
	pal("MALLSOFT", 0x2a7fc4, 0x8f84ee, 0xf7a8d4, 0xfff3a0, 0xff6aa8, 0xeafcff, 0xff5ab8, 0x1f8f88, 0xf4d9ea, false, 0.5),
	pal("MIDNIGHT", 0x000010, 0x0b0b3b, 0x4a1c81, 0xe8f9ff, 0x7ad7f0, 0x8b71ff, 0x00ffc6, 0x020212, 0x9aa0ff, true, 1.1),
	pal("LAGOON", 0x0f2027, 0x2c5364, 0xf7797d, 0xfbd786, 0xf7797d, 0xc6ffdd, 0xfbd786, 0x0b1d26, 0xbfe9e0, true, 0.9),
	pal("HOTLINE", 0x23074d, 0xb5375a, 0xffc88b, 0xffffff, 0xff9a44, 0xff00aa, 0xffe600, 0x1a0520, 0xffb3de, true, 0.9),
	pal("LAVENDER", 0x33217f, 0x7a7fdc, 0xf2a6d8, 0xffe9a8, 0xff6f91, 0xe6e0ff, 0xc49cff, 0x241a52, 0xd9c8f5, false, 0.6),
	pal("DRAGON", 0x050014, 0x2d0036, 0xff0055, 0xffcc00, 0xff0055, 0xff2a6d, 0x00e5ff, 0x0a0010, 0xff8fb3, true, 1.1),
	pal("SEAFOAM", 0x123a52, 0x2f948e, 0xa9ecd9, 0xfff3b0, 0xff8fb0, 0xcffff4, 0xff8fc7, 0x0f2a38, 0xbfe8dc, false, 0.6),
	pal("ARCADE", 0x0a0026, 0x3b0a6b, 0xb517ff, 0xff9de2, 0xff2079, 0x39ff14, 0xff2079, 0x07001a, 0xd28cff, true, 1.0),
}

func (p Palette) rot(deg float32) Palette {
	for _, c := range []*Col{&p.Top, &p.Mid, &p.Hor, &p.SunT, &p.SunB, &p.Line, &p.Acc, &p.Floor, &p.Tile} {
		*c = hueRot(*c, deg)
	}
	return p
}

func RandomPalette(r *rand.Rand) Palette {
	p := palettes[r.Intn(len(palettes))]
	return p.rot(float32(r.Float64()*50 - 25))
}

const (
	FloorGrid = iota
	FloorChecker
	FloorWater
	FloorValley
)

const (
	PropPalm = iota
	PropColumn
	PropDiamond
	PropSphere
	PropCube
	PropPanel
	PropTri
	numProps
)

type Star struct {
	D   V3
	Mag float32
	Ph  float32
}

type Scene struct {
	Seed      uint32
	Name      string
	NameJP    string
	P         Palette
	FloorType int
	G         float32 // grid cell
	LineHalf  float32
	Gloss     float32
	SunDir    V3
	SunR      float32
	Stripes   float32
	SunKind   int // 0 striped, 1 plain, 2 none
	MoonKind  int // 0 none, 1 crescent, 2 full
	MoonDir   V3
	MoonR     float32
	Stars     []Star
	Backdrop  int // 0 none, 1 ridge, 2 city
	BackH     []float32
	Avenue    int // prop kind or -1
	Outer     []int
	Density   float32
	Gate      int // 0 none, 1 rect, 2 triangle, 3 circle, 4 hexagon
	GateBeats int
	CamH      float32
	U         float32 // world units travelled per beat
	ColSp     float32
	Valley    float32
	Amp       float32
	Song      *Song

	// space mode
	Space    bool
	Kind     int
	FullSky  bool
	BH       bool
	Hills    bool
	Fog      float32
	Rough    float32
	TerrC0   Col
	TerrC1   Col
	TerrLine float32
	Bodies   []Body
	Nebula   []Blob
	GalN     V3
	RingN    V3
	RingW    float32
	RingC    Col
}

var jpNames = []string{"夢の島", "仮想プラザ", "永遠の夏", "真夜中のモール", "水晶の海", "電子の夢", "東京ネオン", "失われた未来",
	"サンセット通り", "デジタル天国", "パームリゾート", "新世界", "蒸気波", "無限回廊", "静かな夜", "ピンクの空", "深夜放送", "楽園の記憶", "月の光", "大理石の庭"}
var enA = []string{"VIRTUAL", "ETERNAL", "MIDNIGHT", "CRYSTAL", "LOST", "NEON", "PASTEL", "INFINITE", "DIGITAL", "VELVET", "MARBLE", "SILICON", "LUXURY", "PARADISE", "LUNAR", "SOFT", "PACIFIC", "ELECTRIC"}
var enB = []string{"PLAZA", "SUNSET", "MALL", "LAGOON", "HIGHWAY", "RESORT", "DREAM", "ARCADE", "BOULEVARD", "ATRIUM", "TERMINAL", "GARDEN", "OASIS", "SKYLINE", "BROADCAST", "HORIZON", "FOUNTAIN", "TOWER"}

// Fullwidth converts ASCII to the fullwidth forms beloved of the genre.
func Fullwidth(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		switch {
		case c == ' ':
			out = append(out, 0x3000)
		case c > 0x20 && c < 0x7f:
			out = append(out, c-0x21+0xFF01)
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

func dirFrom(az, sinEl float32) V3 {
	ce := sqrt32(1 - sinEl*sinEl)
	return V3{sin32(az) * ce, sinEl, cos32(az) * ce}
}

func NewScene(seed uint32) *Scene {
	r := rand.New(rand.NewSource(int64(seed)))
	rf := func() float32 { return float32(r.Float64()) }
	sc := &Scene{Seed: seed, Fog: 1, TerrLine: 1.15}
	sc.Song = NewSong(seed)
	sc.P = RandomPalette(r)
	sc.Name = enA[r.Intn(len(enA))] + " " + enB[r.Intn(len(enB))]
	sc.NameJP = jpNames[r.Intn(len(jpNames))]

	switch k := r.Intn(10); {
	case k < 3:
		sc.FloorType = FloorGrid
	case k < 6:
		sc.FloorType = FloorValley
	case k < 8:
		sc.FloorType = FloorChecker
	default:
		sc.FloorType = FloorWater
	}
	sc.G = float32(2 + r.Intn(3))
	if sc.FloorType == FloorChecker && sc.G < 3 {
		sc.G = 3
	}
	sc.U = map[float32]float32{2: 8, 3: 9, 4: 12}[sc.G]
	sc.LineHalf = 0.035 + rf()*0.03
	switch sc.FloorType {
	case FloorChecker:
		sc.Gloss = 0.25 + rf()*0.3
	case FloorWater:
		sc.Gloss = 0.8
	default:
		if r.Intn(2) == 0 {
			sc.Gloss = 0.2 + rf()*0.3
		}
	}
	sc.ColSp = 8 + rf()*3
	sc.Valley = sc.ColSp + 5 + rf()*4
	sc.Amp = 9 + rf()*14
	sc.CamH = 1.9 + rf()*1.4

	sc.SunDir = dirFrom((rf()-0.5)*0.5, 0.05+rf()*0.2)
	sc.SunR = 0.16 + rf()*0.13
	sc.Stripes = float32(5 + r.Intn(6))
	switch k := r.Intn(10); {
	case k < 7:
		sc.SunKind = 0
	case k < 9:
		sc.SunKind = 1
	default:
		sc.SunKind = 2
	}
	if r.Intn(100) < 45 || sc.SunKind == 2 {
		sc.MoonKind = 1 + r.Intn(2)
		az := 0.45 + rf()*0.5
		if r.Intn(2) == 0 {
			az = -az
		}
		sc.MoonDir = dirFrom(az, 0.3+rf()*0.25)
		sc.MoonR = 0.04 + rf()*0.05
	}
	ns := r.Intn(40)
	if sc.P.Dark {
		ns = 150 + r.Intn(250)
	}
	for i := 0; i < ns; i++ {
		d := dirFrom(rf()*2*math.Pi, 0.03+rf()*rf()*0.9)
		if d.Dot(sc.SunDir) > cos32(sc.SunR*1.15) && sc.SunKind != 2 {
			continue
		}
		sc.Stars = append(sc.Stars, Star{d, 0.3 + rf()*rf()*1.2, rf() * 6.28})
	}

	k := r.Intn(100)
	if sc.FloorType == FloorValley {
		if k < 30 {
			sc.Backdrop = 1
		}
	} else if k < 35 {
		sc.Backdrop = 1
	} else if k < 65 {
		sc.Backdrop = 2
	}
	// backdrop skyline profile (tan of elevation), 240 samples around the horizon
	sc.BackH = make([]float32, 240)
	if sc.Backdrop == 1 {
		for i := range sc.BackH {
			a := float64(i) / 240 * 2 * math.Pi
			h := 0.07 + 0.045*math.Sin(a*3+float64(seed%7)) + 0.03*math.Sin(a*7+1.3) + 0.02*math.Sin(a*17+2.1)
			sc.BackH[i] = float32(h) + rf()*0.035
		}
	} else if sc.Backdrop == 2 {
		for i := 0; i < len(sc.BackH); {
			w := 1 + r.Intn(4)
			h := 0.02 + rf()*rf()*0.2
			if r.Intn(6) == 0 {
				h = 0
			}
			for j := 0; j < w && i < len(sc.BackH); j++ {
				sc.BackH[i] = h
				i++
			}
		}
	}

	switch r.Intn(5) {
	case 0, 1:
		sc.Avenue = PropPalm
	case 2, 3:
		sc.Avenue = PropColumn
	default:
		sc.Avenue = -1
	}
	perm := r.Perm(numProps)
	sc.Outer = perm[:2+r.Intn(2)]
	sc.Density = 0.16 + rf()*0.22
	if r.Intn(100) < 55 || sc.Avenue < 0 {
		sc.Gate = 1 + r.Intn(4)
		sc.GateBeats = 4
		if r.Intn(3) == 0 {
			sc.GateBeats = 8
		}
	}
	sc.setTerrCols()
	return sc
}

func (sc *Scene) setTerrCols() {
	sc.TerrC0, sc.TerrC1 = sc.P.Floor.Mul(1.15), mixc(sc.P.Floor, sc.P.Mid, 0.45)
}

func (sc *Scene) Title() string { return Fullwidth(sc.Name) + "　" + sc.NameJP }

// ---------------------------------------------------------------------------

type Ring struct {
	X  float32
	Z  float64
	T0 float64
}

// Player holds everything the user controls.
type Player struct {
	X, VX        float32
	H            float32
	Yaw, Pitch   float32
	YawT, PitchT float32
	Roll         float32
	Rings        []Ring
}

func (p *Player) Update(dt float32, dir float32, sc *Scene) {
	p.VX += (dir*14 - p.VX) * clamp01(dt*4)
	p.X += p.VX * dt
	if sc.FloorType == FloorValley {
		lim := sc.Valley - 3
		if p.X > lim {
			p.X, p.VX = lim, 0
		}
		if p.X < -lim {
			p.X, p.VX = -lim, 0
		}
	}
	k := clamp01(dt * 6)
	p.Yaw += (p.YawT - p.Yaw) * k
	p.Pitch += (p.PitchT - p.Pitch) * k
	p.Roll += (-p.VX*0.012 - p.Roll) * k
	if p.H == 0 {
		p.H = sc.CamH
	}
}

// Frame is the full description of one rendered image.
type Frame struct {
	Time, Beat        float64
	Kick, Snare, Hat  float32
	Melody            float32
	Pitch             int
	Chord             int
	ChordT            float32 // 0..1 blend from previous chord
	Glitch, Flash     float32
	X, H              float32
	Yaw, Pitch_, Roll float32
	Rings             []Ring
	N                 uint32
}

func MakeFrame(sc *Scene, p *Player, beat, t, sceneAge float64, n uint32) Frame {
	s := sc.Song
	f := Frame{Time: t, Beat: beat, N: n}
	f.Kick, f.Snare, f.Hat = s.KickEnv(beat), s.SnareEnv(beat), s.HatEnv(beat)
	f.Melody, f.Pitch = s.MelodyEnv(beat)
	bar := int(math.Floor(beat / 4))
	f.Chord = s.ChordIndex(bar)
	cb := float64(s.ChordBars * 4)
	f.ChordT = clamp01(float32(math.Mod(math.Max(beat, 0), cb)))
	if sceneAge < 0.45 {
		f.Glitch = float32(1 - sceneAge/0.45)
		f.Flash = f.Glitch * f.Glitch * 0.25
	}
	if sc.BH && beat > 0 {
		if p := math.Mod(beat, 32) / 32; p < 0.04 && beat > 4 {
			f.Flash += float32(1-p/0.04) * 0.7
		}
	}
	f.X, f.H, f.Yaw, f.Pitch_, f.Roll = p.X, p.H, p.Yaw, p.Pitch, p.Roll
	f.Rings = p.Rings
	return f
}
