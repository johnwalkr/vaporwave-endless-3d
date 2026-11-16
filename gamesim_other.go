//go:build !windows && !ios

package vaporwave

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
)

// runGameSim plays a whole game with an autopilot, saving frames and audio.
func runGameSim(out string, seed uint32, style, W, H int, gpu *GLR) {
	os.MkdirAll(out, 0o755)
	eng := NewEngine()
	mix := DefaultMix()
	mix.Master = 1
	eng.SetMix(mix)
	eng.vol = 1
	g := NewGame(eng, seed, style, 0)
	rd := &Rd{}
	pix := make([]uint32, W*H)
	const dt = 1.0 / 60
	audio := make([]int16, 0, 44100*2*420)
	buf := make([]int16, 2*int(SR*dt+2))
	rng := rand.New(rand.NewSource(int64(seed)))
	var t, boomAt, createAt float64 = 0, -1, -1
	shots := map[string]bool{}
	lastToast := ""
	beatPrev := -1.0
	monotonic := true
	var nextKey, nextClick float64
	save := func(name string, fr *Frame) {
		if shots[name] {
			return
		}
		shots[name] = true
		rd.Render(g.Sc, fr, W, H, pix)
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		for j, p := range pix {
			img.Set(j%W, j/W, color.RGBA{uint8(p >> 16), uint8(p >> 8), uint8(p), 255})
		}
		f, _ := os.Create(fmt.Sprintf("%s/%s.png", out, name))
		png.Encode(f, img)
		f.Close()
		fmt.Printf("  [frame %-12s t=%6.1fs  phase %d  %.0f BPM  hits %d]\n", name, t, g.Phase, eng.Tempo(), g.Hits)
	}
	for t < 900 {
		// audio for this tick
		n := int(SR*(t+dt)) - int(SR*t)
		eng.Fill(buf[:2*n])
		audio = append(audio, buf[:2*n]...)
		beat := eng.BeatAt(float64(eng.OutTotal()) - 1)
		if beat < beatPrev-1e-6 && g.Phase != PhaseBoom {
			monotonic = false
		}
		beatPrev = beat
		// autopilot
		in := GameInput{}
		for i := range g.Rings {
			r := &g.Rings[i]
			if r.State == 0 && r.Z > g.ShipZ() {
				in.DX = clampf((r.X-g.X)*0.5, -1, 1)
				in.DY = clampf((r.Y-g.Y)*0.5, -1, 1)
				break
			}
		}
		if g.Phase == PhaseCreate {
			if t >= nextKey {
				in.Pressed = []int{rng.Intn(4)}
				nextKey = t + 0.25 + rng.Float64()*0.6
			}
			if t >= nextClick {
				in.Click, in.ClickX, in.ClickZ = true, g.X, g.ShipZ()+20
				nextClick = t + 11
			}
			if t-createAt > 14 && t-createAt < 14+dt {
				in.Space = true
			}
		}
		g.Update(dt, in)
		t += dt
		if g.Toast != lastToast {
			lastToast = g.Toast
			fmt.Printf("%6.1fs  %-7.0f %s\n", t, eng.Tempo(), g.Toast)
		}
		fr := g.MakeFrame(beat, t, uint32(t*60))
		switch {
		case t > 4 && g.Phase == PhaseFlight:
			save("1_flight", &fr)
		}
		if g.Phase == PhaseFlight && eng.Tempo() > 170 {
			save("2_flight170", &fr)
		}
		if g.Phase == PhaseFlight && eng.Tempo() > 240 {
			save("3_flight240", &fr)
		}
		if g.Phase == PhaseBoom {
			if boomAt < 0 {
				boomAt = t
			}
			if t-boomAt > 0.15 {
				save("4_boom", &fr)
			}
			if t-boomAt > 1.6 {
				save("5_boom_after", &fr)
			}
		}
		if g.Phase == PhaseChill && g.PhaseT > 12 {
			save("6_chill", &fr)
		}
		if g.Phase == PhaseCreate {
			if createAt < 0 {
				createAt = t
			}
			if t-createAt > 9 {
				save("7_create", &fr)
			}
			if t-createAt > 32 {
				save("8_create_late", &fr)
			}
			if t-createAt > 45 {
				break
			}
		}
	}
	fmt.Printf("game sim: %.0fs, boom at %.0fs, create at %.0fs, hits %d misses %d notes %d, beat monotonic %v\n",
		t, boomAt, createAt, g.Hits, g.Misses, g.Notes, monotonic)
	var peak, sum float64
	for _, v := range audio {
		x := math.Abs(float64(v) / 32768)
		peak = math.Max(peak, x)
		sum += x * x
	}
	fmt.Printf("audio: %.0fs peak %.2f rms %.1f dB\n", float64(len(audio))/2/SR, peak, 10*math.Log10(sum/float64(len(audio))))
	wf, _ := os.Create(out + "/game.wav")
	hdr := make([]byte, 44)
	copy(hdr, "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+len(audio)*2))
	copy(hdr[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1)
	binary.LittleEndian.PutUint16(hdr[22:], 2)
	binary.LittleEndian.PutUint32(hdr[24:], SR)
	binary.LittleEndian.PutUint32(hdr[28:], SR*4)
	binary.LittleEndian.PutUint16(hdr[32:], 4)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(len(audio)*2))
	wf.Write(hdr)
	binary.Write(wf, binary.LittleEndian, audio)
	wf.Close()
}
