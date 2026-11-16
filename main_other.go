//go:build !windows

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"time"
)

// Offline test harness: renders frames to PNG and audio to WAV.
func main() {
	seed := flag.Uint("seed", 1, "first seed")
	count := flag.Int("n", 4, "number of scenes")
	out := flag.String("out", "/tmp/ve3d", "output dir")
	audio := flag.Float64("audio", 0, "seconds of audio to render per scene")
	beat := flag.Float64("beat", 9.1, "beat position to render")
	yaw := flag.Float64("yaw", 0, "yaw")
	pitch := flag.Float64("pitch", 0, "pitch")
	strafe := flag.Float64("x", 0, "camera x")
	W := flag.Int("w", 800, "width")
	H := flag.Int("h", 600, "height")
	age := flag.Float64("age", 5, "scene age (glitch when <0.45)")
	space := flag.Bool("space", false, "space mode: seed = world index")
	preset := flag.Int("preset", 1, "audio preset index")
	gpu := flag.Bool("gpu", false, "also render with the OpenGL renderer and compare")
	mstyle := flag.Int("style", -1, "music style (-1 random, 0-7)")
	mdrums := flag.Int("drums", 2, "drum intensity 0-4")
	mtempo := flag.Int("tempo", 1, "tempo 0 slower, 1 normal, 2 faster")
	mmood := flag.Int("mood", 0, "mood 0 auto, 1 major, 2 minor")
	mmel := flag.Int("melody", 2, "melody density 0-3")
	vhs := flag.Int("vhs", 2, "VHS effects level 0-2")
	bloom := flag.Int("bloom", 100, "bloom percent")
	flag.Parse()
	CurrentSongOpts = SongOpts{Style: *mstyle, Drums: *mdrums, Tempo: *mtempo, Mood: *mmood, Melody: *mmel}
	os.MkdirAll(*out, 0o755)
	rd := &Rd{}
	var gl *GLR
	if *gpu {
		var err error
		if gl, err = HeadlessGL(); err != nil {
			fmt.Println("GL:", err)
			os.Exit(1)
		}
		fmt.Println("GL:", gl.Renderer, "|", gl.Version)
	}
	rdg := &Rd{}
	st := DefaultSettings()
	st.VHS, st.Bloom = *vhs, *bloom
	fx := st.FX()
	rd.FX, rdg.FX = &fx, &fx
	pix := make([]uint32, *W**H)
	for i := 0; i < *count; i++ {
		s := uint32(*seed) + uint32(i)
		sc := NewScene(s)
		if *space {
			sc = NewSpaceScene(s+100, int(s))
		}
		pl := &Player{H: sc.CamH, X: float32(*strafe), Yaw: float32(*yaw), Pitch: float32(*pitch)}
		pl.Rings = []Ring{{0, float64(sc.U)**beat + 25, 4.5}}
		fr := MakeFrame(sc, pl, *beat, 5, *age, 77)
		rd.Render(sc, &fr, *W, *H, pix)
		t0 := time.Now()
		const reps = 20
		for k := 0; k < reps; k++ {
			fr2 := MakeFrame(sc, pl, *beat+float64(k)*0.02, 5, *age, uint32(k))
			rd.Render(sc, &fr2, *W, *H, pix)
		}
		ms := float64(time.Since(t0).Microseconds()) / 1000 / reps
		rd.Render(sc, &fr, *W, *H, pix)
		cols := 1
		var gpix []uint32
		if gl != nil {
			cols = 2
			rdg.Prepare(sc, &fr, *W, *H)
			fbo, err := gl.Offscreen(*W, *H)
			if err == nil {
				err = gl.Frame(rdg, nil, *W, *H, fbo)
			}
			if err != nil {
				fmt.Println("GL frame:", err)
				os.Exit(1)
			}
			gpix = gl.Read(fbo, *W, *H)
			t1 := time.Now()
			for k := 0; k < 5; k++ {
				fr2 := MakeFrame(sc, pl, *beat+float64(k)*0.02, 5, *age, uint32(k))
				rdg.Prepare(sc, &fr2, *W, *H)
				gl.Frame(rdg, nil, *W, *H, fbo)
				gl.Read(fbo, 1, 1)
			}
			gms := float64(time.Since(t1).Microseconds()) / 1000 / 5
			var sum float64
			big := 0
			for j := range pix {
				a, b := pix[j], gpix[j]
				d := 0
				for sh := 0; sh <= 16; sh += 8 {
					x := int(a>>sh&255) - int(b>>sh&255)
					if x < 0 {
						x = -x
					}
					d += x
				}
				sum += float64(d) / 3
				if d/3 > 40 {
					big++
				}
			}
			fmt.Printf("   gpu vs cpu: mean |diff| %.2f /255, pixels off by >40: %.2f%% | verts %d runs %d | llvmpipe %.0f ms\n",
				sum/float64(len(pix)), 100*float64(big)/float64(len(pix)), len(rdg.fb.GV)/9, len(rdg.fb.Runs), gms)
		}
		img := image.NewRGBA(image.Rect(0, 0, *W*cols, *H))
		for j, p := range pix {
			img.Set(j%*W, j / *W, color.RGBA{uint8(p >> 16), uint8(p >> 8), uint8(p), 255})
		}
		for j, p := range gpix {
			img.Set(*W+j%*W, j / *W, color.RGBA{uint8(p >> 16), uint8(p >> 8), uint8(p), 255})
		}
		f, _ := os.Create(fmt.Sprintf("%s/s%d.png", *out, s))
		png.Encode(f, img)
		f.Close()
		fmt.Printf("seed %d: %s | %s | pal %s floor %d sun %d moon %d back %d avenue %d outer %v gate %d | %.0f bpm %s lead %d | %.1f ms/frame (%d workers)\n",
			s, sc.Name, sc.NameJP, sc.P.Name, sc.FloorType, sc.SunKind, sc.MoonKind, sc.Backdrop, sc.Avenue, sc.Outer, sc.Gate,
			sc.Song.BPM, sc.Song.StyleTag()+" "+sc.Song.KeyName(), sc.Song.LeadType, ms, nWorkers)
		if *audio > 0 {
			e := NewEngine()
			st := DefaultSettings()
			st.Master = 100
			st.ApplyPreset(*preset)
			e.SetMix(st.Mix())
			e.SetRate(float64(st.Speed) / 100)
			e.rate = e.rateT
			e.NewSong(sc.Song)
			n := int(*audio * SR)
			buf := make([]int16, n*2)
			t0 := time.Now()
			for o := 0; o < n; o += 1024 {
				e.Fill(buf[o*2 : min(o+1024, n)*2])
			}
			el := time.Since(t0).Seconds()
			var peak, sum float64
			clip := 0
			secs := make([]float64, int(*audio)+1)
			for j, v := range buf {
				x := float64(v) / 32768
				if math.Abs(x) > peak {
					peak = math.Abs(x)
				}
				if math.Abs(x) > 0.9 {
					clip++
				}
				sum += x * x
				secs[j/2/SR] += x * x
			}
			fmt.Printf("   audio: peak %.2f rms %.1f dB, >0.9: %.3f%%, cpu %.1f%% | per-2s rms:", peak, 10*math.Log10(sum/float64(len(buf))), 100*float64(clip)/float64(len(buf)), 100*el / *audio)
			for j := 0; j+1 < len(secs); j += 2 {
				fmt.Printf(" %.0f", 10*math.Log10((secs[j]+secs[j+1])/(4*SR)+1e-12))
			}
			fmt.Println()
			wf, _ := os.Create(fmt.Sprintf("%s/s%d_p%d.wav", *out, s, *preset))
			hdr := make([]byte, 44)
			copy(hdr, "RIFF")
			binary.LittleEndian.PutUint32(hdr[4:], uint32(36+len(buf)*2))
			copy(hdr[8:], "WAVEfmt ")
			binary.LittleEndian.PutUint32(hdr[16:], 16)
			binary.LittleEndian.PutUint16(hdr[20:], 1)
			binary.LittleEndian.PutUint16(hdr[22:], 2)
			binary.LittleEndian.PutUint32(hdr[24:], SR)
			binary.LittleEndian.PutUint32(hdr[28:], SR*4)
			binary.LittleEndian.PutUint16(hdr[32:], 4)
			binary.LittleEndian.PutUint16(hdr[34:], 16)
			copy(hdr[36:], "data")
			binary.LittleEndian.PutUint32(hdr[40:], uint32(len(buf)*2))
			wf.Write(hdr)
			binary.Write(wf, binary.LittleEndian, buf)
			wf.Close()
		}
	}
}
