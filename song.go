package vaporwave

import (
	"math"
	"math/rand"
)

const SR = 44100

type Chord struct {
	Root int   // semitones above key
	Iv   []int // intervals
}

type Note struct{ Midi, Len int8 }

// ---- styles and options -----------------------------------------------------

const (
	StyleVapor = iota
	StyleJazz
	StyleRock
	StyleMetal
	StylePop
	StyleFunk
	StyleLofi
	StyleSynth
	NumStyles
)

var StyleNames = []string{"Vaporwave", "Smooth jazz", "Soft rock", "Metal", "Pop", "Future funk", "Lo-fi hip hop", "Synthwave"}
var StyleTags = []string{"VAPORWAVE", "SMOOTH JAZZ", "SOFT ROCK", "METAL", "POP", "FUTURE FUNK", "LO-FI", "SYNTHWAVE"}

var DrumNames = []string{"Off", "Chill", "Normal", "Busy", "Intense"}
var TempoNames = []string{"Slower", "Normal", "Faster"}
var MoodNames = []string{"Auto", "Bright (major)", "Dark (minor)"}
var MelodyNames = []string{"Off", "Sparse", "Normal", "Busy"}

// SongOpts are the user's music-generation settings.
type SongOpts struct {
	Style  int // -1 random, else Style*
	Drums  int // 0 off .. 4 intense
	Tempo  int // 0 slower, 1 normal, 2 faster
	Mood   int // 0 auto, 1 major, 2 minor
	Melody int // 0 off .. 3 busy
}

var DefaultSongOpts = SongOpts{Style: -1, Drums: 2, Tempo: 1, Mood: 0, Melody: 2}

// CurrentSongOpts is used by NewSong; the app updates it from the settings.
var CurrentSongOpts = DefaultSongOpts

// instrument kinds
const (
	KickDeep = iota
	KickPunch
	KickTight
)
const (
	SnareClassic = iota
	SnareBrush
	SnareGated
	SnareTight
)
const (
	BassSub = iota
	BassUpright
	BassSaw
	BassSlap
)
const (
	CompNone = iota
	CompRhodes
	CompPluck
	CompGuitar
)
const (
	LeadBell = iota
	LeadSaw
	LeadSine
	LeadSax
	LeadGuitar
	LeadSquare
)

// Song is a deterministic description of a piece of music. Everything that
// sounds can also be queried by beat position so the visuals can follow it.
type Song struct {
	Seed      uint32
	Style     int
	Opts      SongOpts
	BPM       float64
	SPB       float64 // samples per beat
	Key       int
	Minor     bool
	Prog      []Chord
	ChordBars int

	Kick, Snare, Hat, Ride, Clap [16]float32
	OpenHat                      [16]bool
	KickKind, SnareKind          int
	Swing                        float64 // delay of swung steps, in steps
	SwingMask                    uint16
	CrashEvery, FillEvery        int
	Breakdowns                   bool

	Bass     [16]int8 // -1 none, else a bass code (see BassNote)
	BassLen  [16]float32
	BassKind int
	bassLow  int

	Comp     [16]float32
	CompLen  [16]float32
	CompMute [16]bool
	CompKind int
	GtrDrive float64
	gtrLow   int

	PadLevel float64
	PadChop  uint16
	PadCut   float64

	Arp     [16]int8
	ArpMode int // 0 alternate sections, 1 always, 2 never

	Motif    [2][32]Note
	LeadType int

	scale    []int
	leadBase int
}

var chordQ = [][]int{
	{0, 4, 7, 11},      // 0 maj7
	{0, 3, 7, 10},      // 1 m7
	{0, 4, 7, 10},      // 2 dom7
	{0, 5, 7, 10},      // 3 7sus4
	{0, 4, 7, 11, 14},  // 4 maj9
	{0, 3, 7, 10, 14},  // 5 m9
	{0, 4, 7, 14},      // 6 add9
	{0, 3, 7},          // 7 minor
	{0, 4, 7},          // 8 major
	{0, 4, 10, 14, 21}, // 9 13th
	{0, 3, 6, 10},      // 10 m7b5
	{0, 3, 7, 10, 17},  // 11 m11
	{0, 4, 9, 14},      // 12 6/9
	{0, 4, 7, 10, 13},  // 13 7b9
	{0, 3, 7, 14},      // 14 minor add9
}

type pc struct{ r, q int }

// progressions per style: [style][0 major / 1 minor]
var progs = [NumStyles][2][][]pc{
	StyleVapor: {
		{{{5, 0}, {4, 1}, {2, 1}, {7, 3}}, {{0, 4}, {9, 1}, {5, 0}, {7, 3}}, {{5, 0}, {7, 2}, {4, 1}, {9, 5}},
			{{2, 5}, {7, 3}, {0, 4}, {9, 1}}, {{0, 0}, {5, 0}}, {{9, 5}, {5, 0}, {0, 4}, {7, 3}}, {{5, 4}, {4, 1}, {9, 5}, {0, 0}}},
		{{{0, 5}, {8, 0}, {3, 0}, {10, 2}}, {{0, 1}, {5, 1}, {8, 0}, {7, 2}}, {{0, 5}, {10, 3}, {8, 4}, {7, 2}}, {{8, 0}, {10, 2}, {0, 5}, {0, 5}}},
	},
	StyleJazz: {
		{{{2, 5}, {7, 9}, {0, 4}, {0, 4}}, {{0, 4}, {9, 5}, {2, 5}, {7, 9}}, {{4, 1}, {9, 13}, {2, 5}, {7, 9}}, {{5, 4}, {4, 1}, {2, 5}, {0, 12}}},
		{{{2, 10}, {7, 13}, {0, 11}, {0, 11}}, {{0, 5}, {5, 5}, {10, 9}, {3, 4}}, {{0, 11}, {8, 4}, {2, 10}, {7, 13}}},
	},
	StyleRock: {
		{{{0, 0}, {7, 3}, {9, 1}, {5, 0}}, {{0, 0}, {10, 8}, {5, 0}, {0, 0}}, {{9, 1}, {5, 0}, {0, 0}, {7, 3}}, {{0, 6}, {5, 6}, {9, 14}, {7, 8}}},
		{{{0, 1}, {8, 0}, {3, 0}, {10, 2}}, {{0, 5}, {10, 8}, {8, 0}, {10, 8}}, {{0, 14}, {5, 1}, {8, 0}, {7, 8}}},
	},
	StyleMetal: {
		{{{0, 8}, {10, 8}, {5, 8}, {0, 8}}, {{0, 6}, {7, 8}, {9, 7}, {5, 8}}},
		{{{0, 7}, {1, 8}, {0, 7}, {10, 8}}, {{0, 7}, {8, 8}, {10, 8}, {0, 7}}, {{0, 7}, {5, 7}, {8, 8}, {7, 8}}, {{0, 14}, {3, 8}, {8, 8}, {10, 8}}},
	},
	StylePop: {
		{{{0, 6}, {7, 6}, {9, 14}, {5, 6}}, {{9, 14}, {5, 6}, {0, 6}, {7, 6}}, {{0, 6}, {9, 14}, {5, 6}, {7, 3}}, {{5, 0}, {7, 3}, {4, 1}, {9, 1}}},
		{{{0, 14}, {8, 6}, {3, 6}, {10, 6}}, {{0, 14}, {10, 6}, {8, 6}, {10, 6}}},
	},
	StyleFunk: {
		{{{2, 5}, {7, 9}}, {{0, 4}, {9, 5}, {2, 5}, {7, 9}}, {{5, 4}, {4, 5}, {2, 5}, {0, 4}}},
		{{{0, 5}, {5, 9}}, {{0, 11}, {10, 4}, {8, 4}, {7, 13}}},
	},
	StyleLofi: {
		{{{5, 4}, {4, 5}, {2, 5}, {0, 4}}, {{0, 4}, {9, 5}, {2, 5}, {7, 3}}, {{2, 5}, {7, 9}, {0, 4}, {9, 5}}},
		{{{0, 5}, {5, 5}, {10, 9}, {3, 4}}, {{2, 10}, {7, 13}, {0, 11}, {0, 5}}, {{0, 11}, {8, 0}, {5, 5}, {7, 13}}},
	},
	StyleSynth: {
		{{{0, 0}, {9, 1}, {5, 0}, {7, 3}}, {{5, 0}, {7, 3}, {9, 1}, {9, 1}}},
		{{{0, 1}, {8, 0}, {3, 0}, {10, 2}}, {{0, 5}, {8, 4}, {10, 3}, {10, 2}}, {{0, 1}, {3, 0}, {10, 2}, {5, 1}}},
	},
}

var noteNames = []string{"C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}

func set16(a *[16]float32, v float32, steps ...int) {
	for _, s := range steps {
		a[s] = v
	}
}

func NewSong(seed uint32) *Song { return NewSongOpts(seed, CurrentSongOpts) }

func NewSongOpts(seed uint32, o SongOpts) *Song {
	r := rand.New(rand.NewSource(int64(seed)*7919 + 17))
	rf := func() float64 { return r.Float64() }
	pick := func(n int) int { return r.Intn(n) }
	s := &Song{Seed: seed, Opts: o, ArpMode: 2, PadLevel: 1, PadChop: 0xFFFF, Breakdowns: true}
	style := o.Style
	styleRoll := pick(NumStyles)
	if style < 0 || style >= NumStyles {
		style = styleRoll
	}
	s.Style = style

	// tempo
	lo, hi := map[int][2]float64{
		StyleVapor: {62, 90}, StyleJazz: {70, 96}, StyleRock: {84, 112}, StyleMetal: {96, 140},
		StylePop: {90, 116}, StyleFunk: {108, 124}, StyleLofi: {70, 88}, StyleSynth: {84, 108},
	}[style][0], map[int][2]float64{
		StyleVapor: {62, 90}, StyleJazz: {70, 96}, StyleRock: {84, 112}, StyleMetal: {96, 140},
		StylePop: {90, 116}, StyleFunk: {108, 124}, StyleLofi: {70, 88}, StyleSynth: {84, 108},
	}[style][1]
	s.BPM = math.Round(lo + rf()*(hi-lo))
	s.BPM *= []float64{0.82, 1, 1.18}[clampI(o.Tempo, 0, 2)]
	s.BPM = math.Round(s.BPM)
	s.SPB = SR * 60 / s.BPM

	// key and mood
	s.Key = pick(12)
	minorP := []float64{0.33, 0.45, 0.4, 0.9, 0.2, 0.3, 0.6, 0.65}[style]
	s.Minor = rf() < minorP
	switch o.Mood {
	case 1:
		s.Minor = false
	case 2:
		s.Minor = true
	}
	mi := 0
	if s.Minor {
		mi = 1
	}
	set := progs[style][mi]
	p := set[pick(len(set))]
	for _, c := range p {
		s.Prog = append(s.Prog, Chord{c.r, chordQ[c.q]})
	}
	s.ChordBars = 1
	if len(p) == 2 || rf() < 0.33 {
		s.ChordBars = 2
	}

	// melody scale
	switch {
	case style == StyleJazz && s.Minor:
		s.scale = []int{0, 2, 3, 5, 7, 9, 10}
	case style == StyleJazz:
		s.scale = []int{0, 2, 4, 7, 9, 11}
	case style == StyleMetal && s.Minor:
		s.scale = []int{0, 2, 3, 5, 7, 8, 11}
	case style == StyleRock && s.Minor:
		s.scale = []int{0, 3, 5, 6, 7, 10}
	case s.Minor:
		s.scale = []int{0, 3, 5, 7, 10}
	default:
		s.scale = []int{0, 2, 4, 7, 9}
	}
	s.leadBase = 60 + s.Key
	if s.Key >= 6 {
		s.leadBase -= 12
	}
	s.bassLow = 31
	s.gtrLow = 40
	for i := range s.Bass {
		s.Bass[i] = -1
	}
	for i := range s.Arp {
		s.Arp[i] = -1
	}
	bass := func(code int8, length float32, steps ...int) {
		for _, st := range steps {
			s.Bass[st], s.BassLen[st] = code, length
		}
	}
	comp := func(v, length float32, mute bool, steps ...int) {
		for _, st := range steps {
			s.Comp[st], s.CompLen[st], s.CompMute[st] = v, length, mute
		}
	}

	// ---- style templates
	switch style {
	case StyleVapor:
		s.KickKind, s.SnareKind, s.BassKind = KickDeep, SnareClassic, BassSub
		kicks := [][]int{{0, 10}, {0, 6, 10}, {0, 8}, {0, 7, 10}, {0, 3, 10}}
		set16(&s.Kick, 1, kicks[pick(len(kicks))]...)
		if pick(4) == 0 {
			s.Snare[8] = 1
		} else {
			set16(&s.Snare, 1, 4, 12)
		}
		switch pick(3) {
		case 0:
			set16(&s.Hat, 0.7, 0, 2, 4, 6, 8, 10, 12, 14)
		case 1:
			for i := range s.Hat {
				s.Hat[i] = 0.35
			}
			set16(&s.Hat, 0.8, 2, 6, 10, 14)
		default:
			set16(&s.Hat, 0.8, 2, 6, 10, 14)
		}
		if pick(2) == 0 {
			s.OpenHat[14], s.Hat[14] = true, 0.7
		}
		bp := [][]int{{0, 6, 8, 14}, {0, 3, 8, 11}, {0, 8, 10}, {0, 6, 10, 12}, {0, 7, 8}}[pick(5)]
		for i, st := range bp {
			code := int8(0)
			if i == 3 {
				code = int8(1 + pick(2))
			}
			bass(code, 1.8, st)
		}
		s.ArpMode = 0
		s.LeadType = []int{LeadBell, LeadSaw, LeadSine}[pick(3)]
		s.PadCut = 750 + rf()*900

	case StyleJazz:
		s.KickKind, s.SnareKind, s.BassKind = KickDeep, SnareBrush, BassUpright
		s.Swing, s.SwingMask = 0.62, 1<<2|1<<6|1<<10|1<<14
		set16(&s.Ride, 0.75, 0, 4, 8, 12)
		set16(&s.Ride, 0.55, 6, 14)
		set16(&s.Hat, 0.5, 4, 12) // foot hi-hat on 2 and 4
		set16(&s.Kick, 0.22, 0, 4, 8, 12)
		set16(&s.Snare, 0.35, []int{6, 10, 14}[pick(3)])
		s.Snare[2+4*pick(3)] = 0.25
		bass(0, 3.6, 0)
		bass(6, 3.6, 4)
		bass(7, 3.6, 8)
		bass(5, 3.6, 12)
		s.CompKind = CompRhodes
		hits := []int{2, 3, 6, 7, 10, 11, 14}
		for k := 0; k < 2+pick(2); k++ {
			comp(float32(0.55+0.3*rf()), 3, false, hits[pick(len(hits))])
		}
		s.PadLevel, s.PadCut = 0.65, 900+rf()*500
		s.LeadType = []int{LeadSax, LeadSax, LeadBell}[pick(3)]
		s.CrashEvery = 16
		s.leadBase -= 5

	case StyleRock:
		s.KickKind, s.SnareKind, s.BassKind = KickPunch, SnareClassic, BassSaw
		set16(&s.Kick, 1, 0, 8)
		s.Kick[[]int{10, 7, 14}[pick(3)]] = 0.85
		set16(&s.Snare, 1, 4, 12)
		set16(&s.Hat, 0.65, 0, 2, 4, 6, 8, 10, 12, 14)
		s.CrashEvery = 8
		s.CompKind, s.GtrDrive = CompGuitar, 7
		switch pick(3) {
		case 0: // driving palm-muted eighths with open accents
			comp(0.75, 1.6, true, 2, 4, 6, 10, 12, 14)
			comp(1, 2, false, 0, 8)
			for st := 0; st < 16; st += 2 {
				bass(0, 1.7, st)
			}
		case 1: // big open strums
			comp(1, 5.5, false, 0)
			comp(0.9, 3.5, false, 6, 10)
			bass(0, 5.5, 0)
			bass(0, 3.5, 6)
			bass(0, 3.5, 10)
		default: // syncopated stabs
			comp(1, 2.6, false, 0, 3, 8, 11)
			comp(0.7, 1, true, 6, 14)
			bass(0, 2.6, 0)
			bass(0, 2.6, 3)
			bass(0, 2.6, 8)
			bass(2, 2.6, 11)
		}
		s.PadLevel, s.PadCut = 0.75, 900+rf()*400
		s.LeadType = []int{LeadGuitar, LeadGuitar, LeadSaw}[pick(3)]

	case StyleMetal:
		s.KickKind, s.SnareKind, s.BassKind = KickTight, SnareTight, BassSaw
		s.gtrLow, s.bassLow = 38, 26
		if pick(2) == 0 { // half-time
			s.Snare[8] = 1
		} else {
			set16(&s.Snare, 1, 4, 12)
		}
		set16(&s.Hat, 0.7, 0, 2, 4, 6, 8, 10, 12, 14)
		s.CrashEvery = 4
		s.CompKind, s.GtrDrive = CompGuitar, 16
		var chug []int
		if pick(2) == 0 { // gallop
			chug = []int{0, 2, 3, 4, 6, 7, 8, 10, 11, 12, 14, 15}
		} else {
			chug = []int{0, 1, 2, 3, 6, 7, 8, 9, 10, 11, 14, 15}
		}
		for _, st := range chug {
			comp(1.2, 0.8, true, st)
			s.Kick[st] = 0.8
			bass(0, 0.9, st)
		}
		comp(1.3, 3, false, 0)
		if pick(2) == 0 {
			comp(1.3, 3, false, 8)
		}
		s.PadLevel, s.PadCut = 0.6, 650+rf()*300
		s.LeadType = LeadGuitar

	case StylePop:
		s.KickKind, s.SnareKind, s.BassKind = KickPunch, SnareClassic, BassSub
		set16(&s.Kick, 1, 0, 4, 8, 12)
		set16(&s.Clap, 1, 4, 12)
		set16(&s.Snare, 0.5, 4, 12)
		for i := range s.Hat {
			s.Hat[i] = 0.3
		}
		set16(&s.Hat, 0.6, 2, 6, 10, 14)
		s.CrashEvery = 8
		for st := 0; st < 16; st += 2 {
			code := int8(0)
			if st%4 == 2 {
				code = 2
			}
			bass(code, 1.4, st)
		}
		s.CompKind = CompPluck
		if pick(2) == 0 {
			comp(0.8, 1.2, false, 2, 6, 10, 14)
		} else {
			comp(0.8, 1.2, false, 0, 3, 6, 8, 11, 14)
		}
		if pick(2) == 0 {
			s.ArpMode = 0
			for i := 0; i < 16; i += 2 {
				s.Arp[i] = int8((i / 2) % 4)
			}
		}
		s.PadLevel, s.PadCut = 0.85, 1400+rf()*600
		s.LeadType = []int{LeadSquare, LeadSquare, LeadBell}[pick(3)]

	case StyleFunk:
		s.KickKind, s.SnareKind, s.BassKind = KickPunch, SnareClassic, BassSlap
		s.Swing, s.SwingMask = 0.14, 0xAAAA
		set16(&s.Kick, 0.8, 0, 4, 8, 12)
		set16(&s.Snare, 0.55, 4, 12)
		set16(&s.Clap, 0.7, 4, 12)
		set16(&s.Hat, 0.28, 0, 1, 3, 4, 5, 7, 8, 9, 11, 12, 13, 15)
		for _, st := range []int{2, 6, 10, 14} {
			s.Hat[st], s.OpenHat[st] = 0.55, true
		}
		s.CrashEvery = 16
		bass(0, 0.9, 0)
		bass(2, 0.6, 3)
		bass(0, 0.9, 6)
		bass(2, 0.6, 7)
		bass(0, 0.9, 10)
		bass(2, 0.6, 11)
		bass(1, 0.9, 14)
		s.CompKind = CompPluck
		comp(1.3, 1.4, false, 3, 6, 11)
		if pick(2) == 0 {
			comp(0.6, 1, false, 14)
		}
		s.PadChop = []uint16{0b1011011010110110, 0b1101101101101101, 0b1001101110011011}[pick(3)]
		s.PadLevel, s.PadCut = 0.9, 1600+rf()*700
		s.LeadType = []int{LeadBell, LeadSquare, LeadSaw}[pick(3)]

	case StyleLofi:
		s.KickKind, s.SnareKind, s.BassKind = KickDeep, SnareClassic, BassSub
		s.Swing, s.SwingMask = 0.3, 0xAAAA
		bk := [][]int{{0, 7, 10}, {0, 3, 10}, {0, 10, 11}}[pick(3)]
		set16(&s.Kick, 0.9, bk...)
		set16(&s.Snare, 0.7, 4, 12)
		set16(&s.Hat, 0.5, 0, 2, 4, 6, 8, 10, 12, 14)
		set16(&s.Hat, 0.22, 3, 11)
		bass(0, 6, 0)
		bass([]int8{0, 1, 2}[pick(3)], 4, 10)
		s.CompKind = CompRhodes
		comp(0.8, 9, false, 0)
		comp(0.5, 2, false, []int{10, 14, 7}[pick(3)])
		s.PadLevel, s.PadCut = 0.75, 600+rf()*300
		s.LeadType = []int{LeadSine, LeadBell}[pick(2)]

	case StyleSynth:
		s.KickKind, s.SnareKind, s.BassKind = KickPunch, SnareGated, BassSaw
		if pick(2) == 0 {
			set16(&s.Kick, 1, 0, 4, 8, 12)
		} else {
			set16(&s.Kick, 1, 0, 8, 10)
		}
		set16(&s.Snare, 1, 4, 12)
		for i := range s.Hat {
			s.Hat[i] = 0.28
		}
		set16(&s.Hat, 0.5, 2, 6, 10, 14)
		s.CrashEvery = 8
		for st := 0; st < 16; st += 2 {
			code := int8(0)
			if pick(5) == 0 {
				code = 2
			}
			bass(code, 1.6, st)
		}
		s.ArpMode = 1
		pat := [][]int8{{0, 1, 2, 3}, {0, 2, 1, 3}, {3, 2, 1, 0}, {0, 1, 2, 1}}[pick(4)]
		for i := range s.Arp {
			s.Arp[i] = pat[i%4]
		}
		s.PadLevel, s.PadCut = 1, 1200+rf()*700
		s.LeadType = []int{LeadSaw, LeadSaw, LeadSquare}[pick(3)]
	}

	// ---- drum intensity
	switch o.Drums {
	case 0:
		s.Kick, s.Snare, s.Hat, s.Ride, s.Clap = [16]float32{}, [16]float32{}, [16]float32{}, [16]float32{}, [16]float32{}
		s.CrashEvery = 0
	case 1: // chill: sparse and soft
		for i := range s.Kick {
			if i != 0 && i != 8 && i != 10 {
				s.Kick[i] = 0
			}
			s.Kick[i] *= 0.8
			s.Snare[i] *= 0.6
			s.Clap[i] *= 0.6
			if i%4 != 2 && s.Ride[i] == 0 {
				s.Hat[i] = 0
			}
			s.Hat[i] *= 0.7
			s.OpenHat[i] = false
		}
		s.CrashEvery = 0
	case 3: // busy: ghost notes, 16th hats, extra kicks
		for k := 0; k < 3; k++ {
			st := 1 + 2*pick(8)
			if s.Snare[st] == 0 && s.Clap[st] == 0 {
				s.Snare[st] = 0.22
			}
		}
		if style != StyleJazz {
			for i := range s.Hat {
				if s.Hat[i] == 0 {
					s.Hat[i] = 0.3
				}
			}
			if s.Kick[14] == 0 {
				s.Kick[14] = 0.7
			}
		} else {
			set16(&s.Ride, 0.45, 2, 10)
		}
		s.FillEvery = 4
		if s.CrashEvery == 0 || s.CrashEvery > 4 {
			s.CrashEvery = 4
		}
	case 4: // intense
		for i := range s.Hat {
			s.Hat[i] = max(s.Hat[i], 0.55)
			s.Kick[i] *= 1.1
			s.Snare[i] *= 1.1
		}
		switch style {
		case StyleMetal:
			for i := range s.Kick {
				s.Kick[i] = 0.95 // double bass
			}
		case StyleRock, StyleSynth, StylePop, StyleVapor:
			for i := 0; i < 16; i += 2 {
				s.Kick[i] = max(s.Kick[i], 0.85)
			}
		case StyleJazz:
			set16(&s.Ride, 0.7, 2, 10)
			set16(&s.Snare, 0.4, 3, 7, 11, 15)
		default:
			set16(&s.Kick, 0.8, 6, 14)
		}
		for st := 1; st < 16; st += 4 {
			if s.Snare[st] == 0 {
				s.Snare[st] = 0.25
			}
		}
		s.CrashEvery, s.FillEvery, s.Breakdowns = 1, 2, false
		if style != StyleRock && style != StyleMetal {
			s.CrashEvery = 4
		}
	}
	if o.Drums == 2 {
		s.FillEvery = 8
	}
	if style == StyleVapor || style == StyleLofi {
		if o.Drums < 4 {
			s.CrashEvery = 0
		}
	}

	// ---- lead motif: a random walk on the scale
	density := []float64{0, 0.5, 1, 1.6}[clampI(o.Melody, 0, 3)]
	L := len(s.scale)
	idx := L/2 + pick(2)
	maxCap := 6
	if o.Melody == 3 {
		maxCap = 4
	}
	jump := 2
	if style == StyleMetal || style == StyleJazz {
		jump = 3
	}
	gen := func(m *[32]Note, from int) {
		last := -1
		for st := from; st < 32; st++ {
			m[st] = Note{}
			p := 0.0
			switch {
			case st%8 == 0:
				p = 0.8
			case st%4 == 0:
				p = 0.55
			case st%2 == 0:
				p = 0.4
			default:
				p = 0.08
				if style == StyleJazz {
					p = 0.02
				}
				if style == StyleMetal && o.Melody == 3 {
					p = 0.5
				}
			}
			p = math.Min(0.95, p*density)
			if st == from || rf() < p {
				idx += pick(2*jump+1) - jump
				idx = clampI(idx, 0, L+L/2)
				m[st].Midi = int8(s.leadBase + 12*(idx/L) + s.scale[idx%L])
				if last >= 0 {
					m[last].Len = int8(min(st-last, maxCap))
				}
				last = st
			}
		}
		if last >= 0 {
			m[last].Len = int8(min(32-last, maxCap))
		}
	}
	gen(&s.Motif[0], 0)
	s.Motif[1] = s.Motif[0]
	gen(&s.Motif[1], 16)
	return s
}

func (s *Song) KeyName() string {
	m := "maj"
	if s.Minor {
		m = "min"
	}
	return noteNames[s.Key] + " " + m
}

func (s *Song) StyleTag() string { return StyleTags[s.Style] }

func (s *Song) breakdown(bar int) bool {
	return s.Breakdowns && (bar/8)%4 == 3 && bar%8 < 4
}

func (s *Song) fill(bar int) bool {
	return s.FillEvery > 0 && bar >= 2 && bar%s.FillEvery == s.FillEvery-1 && !s.breakdown(bar)
}

// StepOffset is the swing delay of a 16th step, in steps.
func (s *Song) StepOffset(st int) float64 {
	if s.SwingMask&(1<<uint(st&15)) != 0 {
		return s.Swing
	}
	return 0
}

func (s *Song) ChordIndex(bar int) int {
	if bar < 0 {
		bar = 0
	}
	return (bar / s.ChordBars) % len(s.Prog)
}

// ChordNotes returns pad voicing (MIDI notes) of the chord for a bar.
func (s *Song) ChordNotes(bar int, out []int) []int {
	c := s.Prog[s.ChordIndex(bar)]
	out = out[:0]
	for _, iv := range c.Iv {
		n := 48 + s.Key + c.Root + iv
		for n >= 67 {
			n -= 12
		}
		for n < 55 {
			n += 12
		}
		out = append(out, n)
	}
	return out
}

func wrapInto(n, lo int) int {
	for n >= lo+12 {
		n -= 12
	}
	for n < lo {
		n += 12
	}
	return n
}

// GuitarRoot is the low power-chord root for a bar.
func (s *Song) GuitarRoot(bar int) int {
	return wrapInto(s.Key+s.Prog[s.ChordIndex(bar)].Root, s.gtrLow)
}

func (s *Song) hum(bar, st, salt int) float32 {
	return 0.88 + 0.24*hf(hashu(s.Seed, uint32(bar*16+st), uint32(salt)), 1)
}

func (s *Song) KickVel(bar, st int) float32 {
	if bar < 0 || s.breakdown(bar) {
		return 0
	}
	v := s.Kick[st]
	if s.fill(bar) && st == 12 && s.Opts.Drums > 0 {
		v = max(v, 0.9)
	}
	return v
}

func (s *Song) SnareVel(bar, st int) float32 {
	if bar < 2 || s.breakdown(bar) || s.Opts.Drums == 0 {
		return 0
	}
	if s.fill(bar) {
		from := 12
		if s.Opts.Drums >= 4 {
			from = 8
		}
		if st >= from {
			if s.Style == StyleJazz {
				if st%2 == 0 {
					return 0.5 + 0.1*float32(st-from)
				}
				return 0
			}
			return 0.4 + 0.6*float32(st-from)/float32(15-from)
		}
	}
	v := s.Snare[st]
	if v > 0 && v < 0.5 {
		v *= s.hum(bar, st, 3)
	}
	return v
}

func (s *Song) ClapVel(bar, st int) float32 {
	if bar < 2 || s.breakdown(bar) {
		return 0
	}
	return s.Clap[st]
}

func (s *Song) HatVel(bar, st int) float32 {
	if bar < 1 {
		return 0
	}
	if v := s.Hat[st]; v > 0 {
		return v * s.hum(bar, st, 5)
	}
	return 0
}

func (s *Song) RideVel(bar, st int) float32 {
	if bar < 1 || (s.breakdown(bar) && s.Opts.Drums < 3) {
		return 0
	}
	if v := s.Ride[st]; v > 0 {
		return v * s.hum(bar, st, 7)
	}
	return 0
}

func (s *Song) CrashVel(bar, st int) float32 {
	if st != 0 || bar < 2 || s.CrashEvery == 0 || s.breakdown(bar) {
		return 0
	}
	if bar%s.CrashEvery == 0 || s.fill(bar-1) || (s.Breakdowns && s.breakdown(bar-1)) {
		return 0.9
	}
	return 0
}

// BassNote returns (midi, length in steps) for a step, or 0.
func (s *Song) BassNote(bar, st int) (int, float32) {
	code := s.Bass[st]
	if bar < 2 || code < 0 {
		return 0, 0
	}
	if s.breakdown(bar) && s.Style != StyleJazz {
		return 0, 0
	}
	c := s.Prog[s.ChordIndex(bar)]
	root := wrapInto(s.Key+c.Root, s.bassLow)
	h := hashu(s.Seed, uint32(bar), uint32(st))
	n := root
	switch code {
	case 1:
		n = root + 7
	case 2:
		n = root + 12
	case 4:
		n = root + c.Iv[1]
	case 5: // chromatic approach to the next bar's root
		nx := s.Prog[s.ChordIndex(bar+1)]
		next := wrapInto(s.Key+nx.Root, s.bassLow)
		if h&1 == 0 {
			n = next - 1
		} else {
			n = next + 1
		}
	case 6:
		n = root + []int{c.Iv[1], 2, c.Iv[len(c.Iv)-1] % 12}[h%3]
	case 7:
		n = root + []int{7, c.Iv[1] + 12 - 12, 9, 7}[h%4]
	}
	return n, s.BassLen[st]
}

// CompHit returns velocity, length (steps) and mute for chord comping.
func (s *Song) CompHit(bar, st int) (float32, float32, bool) {
	if bar < 1 || s.CompKind == CompNone {
		return 0, 0, false
	}
	if s.breakdown(bar) && s.CompKind == CompGuitar && bar%8 < 2 {
		return 0, 0, false
	}
	v := s.Comp[st]
	if v == 0 {
		return 0, 0, false
	}
	if s.CompKind == CompRhodes && s.Style == StyleJazz && bar%2 == 1 {
		// vary the comping rhythm on alternate bars
		alt := (st + 4) % 16
		if s.Comp[alt] == 0 && hashu(s.Seed, uint32(bar), 9)&1 == 0 {
			return 0, 0, false
		}
	}
	return v * s.hum(bar, st, 11), s.CompLen[st], s.CompMute[st]
}

func (s *Song) ArpNote(bar, st int) int {
	if bar < 0 || s.Arp[st] < 0 || s.ArpMode == 2 {
		return 0
	}
	if s.ArpMode == 0 && (bar/8)%2 == 1 && !s.breakdown(bar) {
		return 0
	}
	var buf [6]int
	n := s.ChordNotes(bar, buf[:])
	return n[int(s.Arp[st])%len(n)] + 12
}

func (s *Song) LeadNote(bar, st int) (midi, length int) {
	if bar < 4 {
		return 0, 0
	}
	switch s.Opts.Melody {
	case 0:
		return 0, 0
	case 1:
		if ((bar-4)/8)%2 == 1 || bar%4 >= 2 {
			return 0, 0
		}
	case 2:
		if ((bar-4)/8)%2 == 1 {
			return 0, 0
		}
	}
	m := &s.Motif[0]
	if (bar/2)%4 == 3 {
		m = &s.Motif[1]
	}
	n := m[(bar%2)*16+st]
	return int(n.Midi), int(n.Len)
}

// env returns a decaying envelope following the latest event at or before beat.
func (s *Song) env(beat float64, vel func(bar, st int) float32, decay float64) float32 {
	if beat < 0 {
		return 0
	}
	pos := beat * 4
	cur := int(pos)
	for k := -1; k < 16; k++ {
		a := cur - k
		if a < 0 {
			break
		}
		t := float64(a) + s.StepOffset(a%16)
		if t > pos {
			continue
		}
		if v := vel(a/16, a%16); v > 0 {
			dt := (pos - t) / 4 * 60 / s.BPM
			return v * float32(math.Exp(-dt*decay))
		}
	}
	return 0
}

func (s *Song) KickEnv(beat float64) float32 { return s.env(beat, s.KickVel, 6) }
func (s *Song) SnareEnv(beat float64) float32 {
	return s.env(beat, func(b, st int) float32 { return max(s.SnareVel(b, st), s.ClapVel(b, st)) }, 5)
}
func (s *Song) HatEnv(beat float64) float32 {
	return s.env(beat, func(b, st int) float32 { return max(s.HatVel(b, st), s.RideVel(b, st)) }, 14)
}

// MelodyEnv follows lead notes (or the arpeggio when the lead rests).
func (s *Song) MelodyEnv(beat float64) (float32, int) {
	pitch := 0
	e := s.env(beat, func(bar, st int) float32 {
		if m, _ := s.LeadNote(bar, st); m > 0 {
			pitch = m
			return 1
		}
		if m := s.ArpNote(bar, st); m > 0 {
			pitch = m
			return 0.6
		}
		return 0
	}, 4)
	return e, pitch
}

func mtof(m float64) float64 { return 440 * math.Pow(2, (m-69)/12) }
