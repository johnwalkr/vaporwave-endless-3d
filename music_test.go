package vaporwave

import (
	"fmt"
	"math"
	"testing"
)

func renderMix(o SongOpts, seed uint32, mix Mix, secs float64) []float64 {
	e := NewEngine()
	e.SetMix(mix)
	e.vol = mix.Master
	e.NewSong(NewSongOpts(seed, o))
	n := int(secs * SR)
	buf := make([]int16, n*2)
	for i := 0; i < n; i += 1024 {
		e.Fill(buf[i*2 : min(i+1024, n)*2])
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = (float64(buf[2*i]) + float64(buf[2*i+1])) / 65536
	}
	return out[int(10*SR):]
}

func rmsDB(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v * v
	}
	return 10 * math.Log10(s/float64(len(x))+1e-12)
}

// TestInstrumentShare reports how loud each instrument group is per style
// (level drop when that group is muted).
func TestInstrumentShare(t *testing.T) {
	for st := 0; st < NumStyles; st++ {
		o := SongOpts{Style: st, Drums: 2, Tempo: 1, Melody: 2}
		base := DefaultMix()
		base.Master = 1
		full := rmsDB(renderMix(o, 11, base, 26))
		line := fmt.Sprintf("%-14s full %5.1f dB |", StyleNames[st], full)
		for _, g := range []struct {
			name string
			f    func(*Mix)
		}{{"drums", func(m *Mix) { m.Drums = 0 }}, {"bass", func(m *Mix) { m.Bass = 0 }}, {"pads", func(m *Mix) { m.Pads = 0 }},
			{"keys/gtr", func(m *Mix) { m.Keys = 0 }}, {"lead", func(m *Mix) { m.Lead = 0 }}} {
			mm := base
			g.f(&mm)
			only := rmsDB(renderMix(o, 11, mm, 26))
			// energy of the muted group ~ full - rest
			share := 1 - math.Pow(10, (only-full)/10)
			line += fmt.Sprintf(" %s %3.0f%%", g.name, 100*math.Max(share, 0))
		}
		t.Log(line)
	}
}

func onsetRate(x []float64) float64 {
	n := len(x) / 441
	var prev float64
	c := 0
	for i := 1; i < n; i++ {
		var e float64
		for j := i * 441; j < (i+1)*441; j++ {
			d := x[j] - x[j-1]
			e += d * d
		}
		le := math.Log(math.Sqrt(e/441) + 1e-6)
		if le-prev > 0.5 {
			c++
		}
		prev = le
	}
	return float64(c) / (float64(len(x)) / SR)
}

func TestAllCombos(t *testing.T) {
	for st := 0; st < NumStyles; st++ {
		line := fmt.Sprintf("%-14s onsets/s by drum level:", StyleNames[st])
		for d := 0; d <= 4; d++ {
			mix := DefaultMix()
			x := renderMix(SongOpts{Style: st, Drums: d, Tempo: 1, Melody: 2}, uint32(20+d), mix, 30)
			for _, v := range x {
				if math.IsNaN(v) || math.Abs(v) > 1 {
					t.Fatalf("bad sample style %d drums %d", st, d)
				}
			}
			line += fmt.Sprintf(" %4.1f", onsetRate(x))
		}
		for mel := 0; mel <= 3; mel++ {
			for mood := 0; mood <= 2; mood++ {
				for tempo := 0; tempo <= 2; tempo++ {
					s := NewSongOpts(uint32(st*100+mel*10+mood*3+tempo), SongOpts{Style: st, Drums: 2, Tempo: tempo, Mood: mood, Melody: mel})
					for bar := 0; bar < 64; bar++ {
						for k := 0; k < 16; k++ {
							s.KickVel(bar, k)
							s.SnareVel(bar, k)
							s.BassNote(bar, k)
							s.CompHit(bar, k)
							s.LeadNote(bar, k)
							s.ArpNote(bar, k)
							s.CrashVel(bar, k)
						}
					}
					s.MelodyEnv(37.3)
					s.SnareEnv(37.3)
				}
			}
		}
		t.Log(line)
	}
}
