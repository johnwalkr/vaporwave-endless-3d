package vaporwave

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Mix holds the audio parameters the synth and engine read while playing.
type Mix struct {
	Master, Drums, Bass, Pads, Keys, Lead, SFX float64
	Echo, Reverb, Crackle, Wobble, Width       float64
	ToneA                                      float64 // master low-pass coefficient (1 = bypass)
}

func DefaultMix() Mix { s := DefaultSettings(); return s.Mix() }

// Settings is everything that is saved between runs. Levels are percentages.
type Settings struct {
	// music generation (Style 0 = random per scene, else style index + 1)
	Style, DrumLevel, Tempo, Mood, Melody int

	Preset                                     int
	Master, Drums, Bass, Pads, Keys, Lead, SFX int
	Echo, Reverb, Crackle, Wobble, Tone, Width int
	Speed                                      int
	Device                                     string
	Latency                                    int // 0 low, 1 normal, 2 high
	GPU                                        bool
	Scale                                      int // percent of window resolution
	VSync                                      bool
	Bloom                                      int
	VHS                                        int // 0 off, 1 light, 2 full
	ShowFPS                                    bool
}

func DefaultSettings() Settings {
	return Settings{DrumLevel: 2, Tempo: 1, Melody: 2, Master: 90, Drums: 100, Bass: 100, Pads: 100, Keys: 100, Lead: 100, SFX: 100,
		Echo: 100, Reverb: 100, Crackle: 100, Wobble: 100, Tone: 67, Width: 100, Speed: 100,
		Latency: 1, Scale: 100, VSync: true, Bloom: 100, VHS: 2, ShowFPS: true}
}

func (s *Settings) Mix() Mix {
	f := func(v int) float64 { return float64(v) / 100 }
	return Mix{Master: f(s.Master), Drums: f(s.Drums), Bass: f(s.Bass), Pads: f(s.Pads), Keys: f(s.Keys),
		Lead: f(s.Lead), SFX: f(s.SFX), Echo: f(s.Echo), Reverb: f(s.Reverb), Crackle: f(s.Crackle),
		Wobble: f(s.Wobble), Width: f(s.Width), ToneA: 0.25 + 0.75*f(s.Tone)}
}

var PresetNames = []string{"Default", "Slowed + Reverb", "Mallsoft", "Lo-fi tape", "Clean", "Ambient", "Custom"}

const PresetCustom = 6

var ScaleSteps = []int{50, 67, 75, 100, 125, 150, 200}
var LatencyNames = []string{"Low", "Normal", "High"}
var LatencyBufs = []int{3, 5, 8}
var VHSNames = []string{"Off", "Light", "Full"}

// ApplyPreset overwrites the mix (but not master or click volume) and tape speed.
func (s *Settings) ApplyPreset(i int) {
	if i < 0 || i >= PresetCustom {
		s.Preset = PresetCustom
		return
	}
	d := DefaultSettings()
	s.Drums, s.Bass, s.Pads, s.Keys, s.Lead = d.Drums, d.Bass, d.Pads, d.Keys, d.Lead
	s.Echo, s.Reverb, s.Crackle, s.Wobble, s.Tone, s.Width, s.Speed = d.Echo, d.Reverb, d.Crackle, d.Wobble, d.Tone, d.Width, d.Speed
	switch i {
	case 1: // slowed + reverb
		s.Reverb, s.Echo, s.Tone, s.Wobble, s.Drums, s.Speed = 180, 130, 50, 120, 90, 80
	case 2: // mallsoft: the music drifting in from an empty food court
		s.Reverb, s.Echo, s.Tone, s.Drums, s.Lead, s.Keys, s.Pads, s.Width, s.Crackle, s.Speed = 200, 120, 35, 45, 80, 90, 120, 140, 50, 92
	case 3: // worn cassette
		s.Wobble, s.Crackle, s.Tone, s.Width, s.Reverb = 300, 300, 30, 70, 80
	case 4: // clean
		s.Wobble, s.Crackle, s.Tone, s.Reverb, s.Echo, s.Width = 0, 0, 100, 80, 80, 110
	case 5: // ambient: no drums
		s.Drums, s.Bass, s.Pads, s.Lead, s.Reverb, s.Echo, s.Speed = 0, 60, 140, 70, 180, 140, 85
	}
	s.Preset = i
}

type field struct {
	k string
	p any
}

func (s *Settings) fields() []field {
	return []field{
		{"style", &s.Style}, {"drumlevel", &s.DrumLevel}, {"tempo", &s.Tempo}, {"mood", &s.Mood}, {"melody", &s.Melody},
		{"preset", &s.Preset}, {"master", &s.Master}, {"drums", &s.Drums}, {"bass", &s.Bass}, {"pads", &s.Pads},
		{"keys", &s.Keys}, {"lead", &s.Lead}, {"sfx", &s.SFX}, {"echo", &s.Echo}, {"reverb", &s.Reverb},
		{"crackle", &s.Crackle}, {"wobble", &s.Wobble}, {"tone", &s.Tone}, {"width", &s.Width}, {"speed", &s.Speed},
		{"device", &s.Device}, {"latency", &s.Latency}, {"gpu", &s.GPU}, {"scale", &s.Scale}, {"vsync", &s.VSync},
		{"bloom", &s.Bloom}, {"vhs", &s.VHS}, {"fpscounter", &s.ShowFPS},
	}
}

func (s *Settings) Save(path string) error {
	return os.WriteFile(path, []byte(s.Text()), 0o644)
}

// Text is the settings file contents.
func (s *Settings) Text() string {
	var b strings.Builder
	b.WriteString("# Vaporwave Endless 3D settings (levels in percent)\n")
	for _, f := range s.fields() {
		switch p := f.p.(type) {
		case *int:
			fmt.Fprintf(&b, "%s=%d\n", f.k, *p)
		case *bool:
			fmt.Fprintf(&b, "%s=%v\n", f.k, *p)
		case *string:
			fmt.Fprintf(&b, "%s=%s\n", f.k, *p)
		}
	}
	return b.String()
}

func (s *Settings) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s.ParseText(string(data))
	return nil
}

// ParseText reads settings in the format Text writes.
func (s *Settings) ParseText(text string) {
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || l[0] == '#' {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	for _, fl := range s.fields() {
		v, ok := m[fl.k]
		if !ok {
			continue
		}
		switch p := fl.p.(type) {
		case *int:
			if x, err := strconv.Atoi(v); err == nil {
				*p = x
			}
		case *bool:
			*p = v == "true"
		case *string:
			*p = v
		}
	}
	s.Clamp()
}

func clampF(x, a, b float64) float64 { return max(a, min(b, x)) }
func clampI(x, a, b int) int         { return max(a, min(b, x)) }

func (s *Settings) Clamp() {
	for _, p := range []*int{&s.Drums, &s.Bass, &s.Pads, &s.Keys, &s.Lead, &s.SFX, &s.Echo, &s.Reverb, &s.Width, &s.Bloom} {
		*p = clampI(*p, 0, 200)
	}
	s.Style = clampI(s.Style, 0, NumStyles)
	s.DrumLevel = clampI(s.DrumLevel, 0, 4)
	s.Tempo = clampI(s.Tempo, 0, 2)
	s.Mood = clampI(s.Mood, 0, 2)
	s.Melody = clampI(s.Melody, 0, 3)
	s.Master = clampI(s.Master, 0, 100)
	s.Tone = clampI(s.Tone, 0, 100)
	s.Wobble = clampI(s.Wobble, 0, 300)
	s.Crackle = clampI(s.Crackle, 0, 300)
	s.Speed = clampI(s.Speed, 40, 180)
	s.Preset = clampI(s.Preset, 0, PresetCustom)
	s.Latency = clampI(s.Latency, 0, 2)
	s.VHS = clampI(s.VHS, 0, 2)
	ok := false
	for _, v := range ScaleSteps {
		ok = ok || v == s.Scale
	}
	if !ok {
		s.Scale = 100
	}
}

// SongOpts converts the music settings for the generator.
func (s *Settings) SongOpts() SongOpts {
	return SongOpts{Style: s.Style - 1, Drums: s.DrumLevel, Tempo: s.Tempo, Mood: s.Mood, Melody: s.Melody}
}

// FX controls the post-processing look on both renderers.
type FX struct {
	Bloom, Scan, Grain, CA, Track float32
}

func (s *Settings) FX() FX {
	f := FX{Bloom: float32(s.Bloom) / 100}
	switch s.VHS {
	case 1:
		f.Scan, f.Grain, f.CA, f.Track = 0.5, 0.5, 1, 0.5
	case 2:
		f.Scan, f.Grain, f.CA, f.Track = 1, 1, 1, 1
	}
	return f
}

var DefaultFX = FX{Bloom: 1, Scan: 1, Grain: 1, CA: 1, Track: 1}
