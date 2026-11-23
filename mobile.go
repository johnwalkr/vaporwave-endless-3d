package vaporwave

import (
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"time"
)

// Mobile is everything the iOS app needs from the Go core: scenes, music,
// touch input, the settings menu, and per-frame data for the Metal renderer.
// All methods except FillAudio are called from the main thread.
type Mobile struct {
	st           Settings
	fx           FX
	eng          *Engine
	rd           *Rd
	sc           *Scene
	pl           Player
	rng          *rand.Rand
	space        bool
	deck         []int
	t            float64
	sceneStart   float64
	posEst       float64
	frameN       uint32
	rateT        float64
	strafe       float32
	dw, dh       int
	iw, ih       int
	toast        string
	toastT       float64
	settingsPath string
	items        []mobileItem
	remixAt      float64
	params       [nParams * 4]float32
	fr           Frame
	ui           [4]uint32
	prepared     bool
}

type mobileItem struct {
	label  string
	header bool
	val    func() string
	adj    func(d int)
}

// MobileScaleSteps are the render-resolution choices offered on iPhone.
var MobileScaleSteps = []int{50, 67, 75, 100}

// NewMobile starts the app; dir is where settings are stored.
func NewMobile(dir string) *Mobile {
	m := &Mobile{st: DefaultSettings(), rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	m.st.Scale = 67
	if dir != "" {
		m.settingsPath = filepath.Join(dir, "settings.ini")
		_ = m.st.Load(m.settingsPath)
	}
	m.fx = m.st.FX()
	m.rd = &Rd{FX: &m.fx}
	m.eng = NewEngine()
	m.eng.SetMix(m.st.Mix())
	m.rateT = float64(m.st.Speed) / 100
	CurrentSongOpts = m.st.SongOpts()
	m.buildMenu()
	m.NewScene()
	return m
}

func (m *Mobile) say(s string) { m.toast, m.toastT = s, m.t }

// Save writes the settings file.
func (m *Mobile) Save() {
	if m.settingsPath == "" {
		return
	}
	m.st.Speed = int(math.Round(m.rateT * 100))
	_ = m.st.Save(m.settingsPath)
}

// Resize sets the drawable size in pixels.
func (m *Mobile) Resize(w, h int) { m.dw, m.dh = w, h }

// InternalSize is the resolution the scene is rendered at before scaling.
func (m *Mobile) InternalSize() (int, int) {
	s := float64(m.st.Scale) / 100
	w := int(clampF(float64(m.dw)*s, 16, 2400))
	h := int(clampF(float64(m.dh)*s, 16, 2400))
	return w, h
}

// NewScene starts a new scene and track.
func (m *Mobile) NewScene() {
	seed := m.rng.Uint32()
	CurrentSongOpts = m.st.SongOpts()
	m.sc = NewScene(seed)
	if m.space {
		if len(m.deck) == 0 {
			m.deck = m.rng.Perm(NumWorlds())
		}
		m.sc = NewSpaceScene(seed, m.deck[0])
		m.deck = m.deck[1:]
	}
	m.eng.NewSong(m.sc.Song)
	m.pl.X, m.pl.VX, m.pl.H = 0, 0, m.sc.CamH
	m.pl.Rings = nil
	m.sceneStart = m.t
	m.posEst = float64(m.eng.OutTotal())
}

// ToggleSpace switches between vaporwave and space travel.
func (m *Mobile) ToggleSpace() {
	m.space = !m.space
	m.NewScene()
}

// Recolour gives the current vaporwave scene a new palette.
func (m *Mobile) Recolour() {
	if m.sc.Space {
		return
	}
	sc := *m.sc
	sc.P = RandomPalette(m.rng)
	sc.setTerrCols()
	m.sc = &sc
}

func (m *Mobile) remix(seed uint32) {
	CurrentSongOpts = m.st.SongOpts()
	sc := *m.sc
	sc.Song = NewSong(seed)
	m.sc = &sc
	m.eng.NewSong(m.sc.Song)
	m.pl.Rings = nil
	m.sceneStart = m.t
	m.posEst = float64(m.eng.OutTotal())
	m.say(fmt.Sprintf("Remix: %s · %.0f BPM · %s", m.sc.Song.StyleTag(), m.sc.Song.BPM, m.sc.Song.KeyName()))
}

// ---- touch input ---------------------------------------------------------------

// Look turns the camera; dx, dy are drag distances as a fraction of the view.
func (m *Mobile) Look(dx, dy float32) {
	m.pl.YawT = clampf(m.pl.YawT+dx*2.2, -1.15, 1.15)
	m.pl.PitchT = clampf(m.pl.PitchT-dy*1.2, -0.5, 0.55)
}

// SetStrafe moves sideways while held (-1..1).
func (m *Mobile) SetStrafe(v float32) { m.strafe = clampf(v, -1, 1) }

// Speed changes the tape speed (music and travel) by dv.
func (m *Mobile) Speed(dv float32) {
	m.rateT = clampF(m.rateT+float64(dv), 0.4, 1.8)
}

// Altitude multiplies the camera height (pinch).
func (m *Mobile) Altitude(scale float32) {
	m.pl.H = clampf(m.pl.H*scale, 0.8, 14)
}

// Tap sends a shockwave across the floor and plays a chime; x, y are 0..1.
func (m *Mobile) Tap(x, y float32) {
	if m.iw == 0 {
		return
	}
	px, pz := m.rd.Pick(x*float32(m.iw), y*float32(m.ih))
	if len(m.pl.Rings) < 6 {
		m.pl.Rings = append(m.pl.Rings, Ring{px, pz, m.t})
	}
	m.eng.Chime()
}

// ---- frame ----------------------------------------------------------------------

// Frame advances by dt seconds and prepares the render data. latency is the
// audio output latency in seconds, so the visuals match what is heard.
func (m *Mobile) Frame(dt, latency float64) {
	m.Advance(dt, latency)
	m.iw, m.ih = m.InternalSize()
	m.PrepareView(nil, m.iw, m.ih)
}

// Advance moves time, the player and the music clock forward by dt seconds.
func (m *Mobile) Advance(dt, latency float64) {
	if dt > 0.1 {
		dt = 0.1
	}
	if dt < 0 {
		dt = 0
	}
	m.t += dt
	m.eng.SetRate(m.rateT)
	m.pl.Update(float32(dt), m.strafe, m.sc)
	for len(m.pl.Rings) > 0 && m.t-m.pl.Rings[0].T0 > 2.5 {
		m.pl.Rings = m.pl.Rings[1:]
	}
	if m.remixAt > 0 && m.t >= m.remixAt {
		m.remixAt = 0
		m.remix(m.sc.Seed)
	}
	// follow the audio clock: samples rendered minus what is still buffered
	m.posEst += dt * SR
	target := float64(m.eng.OutTotal()) - latency*SR
	if err := target - m.posEst; math.Abs(err) > 6000 {
		m.posEst = target
	} else {
		m.posEst += err * 0.05
	}
	m.frameN++
	m.fx = m.st.FX()
	m.fr = MakeFrame(m.sc, &m.pl, m.eng.BeatAt(m.posEst), m.t, m.t-m.sceneStart, m.frameN)
}

// PrepareView builds the render data for one view of the current frame: the
// normal camera when eye is nil, otherwise a tracked VR eye.
func (m *Mobile) PrepareView(eye *EyeView, w, h int) {
	m.fr.Eye = eye
	if eye != nil {
		m.fx.Scan, m.fx.Grain, m.fx.CA, m.fx.Track = 0, 0, 0, 0 // per-eye VHS noise is uncomfortable
	}
	m.iw, m.ih = w, h
	m.rd.Prepare(m.sc, &m.fr, w, h)
	m.ui = m.rd.PackParams(m.params[:])
	m.prepared = true
}

// SetScale sets the render scale in percent.
func (m *Mobile) SetScale(p int) { m.st.Scale = clampI(p, 25, 100) }

// Scale is the render scale in percent.
func (m *Mobile) Scale() int { return m.st.Scale }

// Settings text, for storing settings where there is no file system (browser).
func (m *Mobile) SettingsText() string {
	m.st.Speed = int(math.Round(m.rateT * 100))
	return m.st.Text()
}

// LoadSettingsText applies settings saved with SettingsText.
func (m *Mobile) LoadSettingsText(text string) {
	m.st.ParseText(text)
	m.fx = m.st.FX()
	m.eng.SetMix(m.st.Mix())
	m.rateT = float64(m.st.Speed) / 100
	CurrentSongOpts = m.st.SongOpts()
}

// Params copies the sky/floor parameters (NParams float4s) into dst.
func (m *Mobile) Params(dst []float32) int { return copy(dst, m.params[:]) }

// ParamsUI returns the integer sky/floor parameters.
func (m *Mobile) ParamsUI() [4]uint32 { return m.ui }

// GeometryFloats is the number of floats in this frame's vertex data (9 per vertex).
func (m *Mobile) GeometryFloats() int { return len(m.rd.fb.GV) }

// Geometry copies this frame's vertices into dst.
func (m *Mobile) Geometry(dst []float32) int { return copy(dst, m.rd.fb.GV) }

// Runs writes (first vertex, vertex count, depth write) triples; returns the run count.
func (m *Mobile) Runs(dst []int32) int {
	n := 0
	for _, r := range m.rd.fb.Runs {
		if r.Count == 0 || 3*n+2 >= len(dst) {
			continue
		}
		dw := int32(0)
		if r.DepthWrite {
			dw = 1
		}
		dst[3*n], dst[3*n+1], dst[3*n+2] = r.First, r.Count, dw
		n++
	}
	return n
}

// Comp returns the composite-pass parameters (3 float4s, 1 uint4).
func (m *Mobile) Comp() ([12]float32, [4]uint32) { return m.rd.CompParams(false) }

// FillAudio renders audio; called from the audio thread.
func (m *Mobile) FillAudio(left, right []float32) { m.eng.FillF32(left, right) }

// ---- HUD text -------------------------------------------------------------------

func (m *Mobile) Title() string { return m.sc.Title() }

func (m *Mobile) Info() string {
	return fmt.Sprintf("%s · %s · %.0f BPM · %s · TAPE x%.2f", m.sc.P.Name, m.sc.Song.StyleTag(), m.sc.Song.BPM,
		m.sc.Song.KeyName(), m.eng.Rate())
}

// Toast is a short status message, empty when there is none to show.
func (m *Mobile) Toast() string {
	if m.toast == "" || m.t-m.toastT > 5 {
		return ""
	}
	return m.toast
}

// SceneAge is the time since the current scene started (the HUD fades the title).
func (m *Mobile) SceneAge() float64 { return m.t - m.sceneStart }

func (m *Mobile) ShowFPS() bool { return m.st.ShowFPS }

// ---- settings menu ---------------------------------------------------------------

func (m *Mobile) MenuCount() int { return len(m.items) }

func (m *Mobile) MenuItem(i int) (label, value string, header bool) {
	if i < 0 || i >= len(m.items) {
		return "", "", true
	}
	it := &m.items[i]
	if it.header {
		return it.label, "", true
	}
	return it.label, it.val(), false
}

func (m *Mobile) MenuAdjust(i, d int) {
	if i >= 0 && i < len(m.items) && !m.items[i].header {
		m.items[i].adj(d)
	}
}

func (m *Mobile) buildMenu() {
	st := &m.st
	applyAudio := func() { m.eng.SetMix(st.Mix()) }
	level := func(label string, p *int, lo, hi, step int, mix bool) mobileItem {
		return mobileItem{label: label, val: func() string { return fmt.Sprintf("%d%%", *p) }, adj: func(d int) {
			*p = clampI(*p+d*step, lo, hi)
			if mix {
				st.Preset = PresetCustom
			}
			applyAudio()
		}}
	}
	music := func(label string, p *int, names []string) mobileItem {
		return mobileItem{label: label, val: func() string { return names[*p] }, adj: func(d int) {
			*p = (*p + d + len(names)) % len(names)
			CurrentSongOpts = st.SongOpts()
			m.remixAt = m.t + 0.6
		}}
	}
	onOff := func(b bool) string {
		if b {
			return "On"
		}
		return "Off"
	}
	m.items = []mobileItem{
		{label: "MUSIC", header: true},
		music("Style", &st.Style, append([]string{"Auto (random per scene)"}, StyleNames...)),
		music("Drums", &st.DrumLevel, DrumNames),
		music("Tempo", &st.Tempo, TempoNames),
		music("Mood", &st.Mood, MoodNames),
		music("Melody", &st.Melody, MelodyNames),
		{label: "New track for this scene", val: func() string { return "Go" }, adj: func(d int) { m.remix(m.rng.Uint32()) }},
		{label: "AUDIO", header: true},
		{label: "Preset", val: func() string { return PresetNames[st.Preset] }, adj: func(d int) {
			i := st.Preset
			if i == PresetCustom {
				i = map[bool]int{true: 0, false: PresetCustom - 1}[d > 0]
			} else {
				i = (i + d + PresetCustom) % PresetCustom
			}
			st.ApplyPreset(i)
			m.rateT = float64(st.Speed) / 100
			applyAudio()
		}},
		level("Master volume", &st.Master, 0, 100, 5, false),
		level("Drums", &st.Drums, 0, 200, 10, true),
		level("Bass", &st.Bass, 0, 200, 10, true),
		level("Pads", &st.Pads, 0, 200, 10, true),
		level("Keys & arpeggios", &st.Keys, 0, 200, 10, true),
		level("Lead melody", &st.Lead, 0, 200, 10, true),
		level("Tap chimes", &st.SFX, 0, 200, 10, false),
		level("Reverb", &st.Reverb, 0, 200, 10, true),
		level("Echo", &st.Echo, 0, 200, 10, true),
		level("Tape wobble", &st.Wobble, 0, 300, 20, true),
		level("Vinyl crackle & hiss", &st.Crackle, 0, 300, 20, true),
		level("Tone (dark - bright)", &st.Tone, 0, 100, 5, true),
		level("Stereo width", &st.Width, 0, 200, 10, true),
		{label: "Tape speed", val: func() string { return fmt.Sprintf("%d%%", int(math.Round(m.rateT*100))) }, adj: func(d int) {
			m.rateT = clampF(math.Round(m.rateT*20+float64(d))/20, 0.4, 1.8)
			st.Preset = PresetCustom
		}},
		{label: "VIDEO", header: true},
		{label: "Render scale", val: func() string { return fmt.Sprintf("%d%%", st.Scale) }, adj: func(d int) {
			i := 0
			for k, v := range MobileScaleSteps {
				if v <= st.Scale {
					i = k
				}
			}
			st.Scale = MobileScaleSteps[clampI(i+d, 0, len(MobileScaleSteps)-1)]
		}},
		level("Bloom", &st.Bloom, 0, 200, 10, false),
		{label: "VHS effects", val: func() string { return VHSNames[st.VHS] }, adj: func(d int) { st.VHS = (st.VHS + d + 3) % 3 }},
		{label: "FPS counter", val: func() string { return onOff(st.ShowFPS) }, adj: func(d int) { st.ShowFPS = !st.ShowFPS }},
		{label: "Reset all to defaults", val: func() string { return "Reset" }, adj: func(d int) {
			*st = DefaultSettings()
			st.Scale = 67
			m.rateT = 1
			applyAudio()
			CurrentSongOpts = st.SongOpts()
			m.remixAt = m.t + 0.6
		}},
	}
}
