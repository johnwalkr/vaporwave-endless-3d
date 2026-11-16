//go:build linux && cgo

package vaporwave

import (
	"runtime"
	"testing"
)

// The game's ship, rings and debris must look the same on the GPU path.
func TestGameGPUMatchesCPU(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gl, err := HeadlessGL()
	if err != nil {
		t.Skip(err)
	}
	const W, H = 400, 300
	eng := NewEngine()
	g := NewGame(eng, 7, 0, 0)
	buf := make([]int16, 2*735)
	for i := 0; i < 400; i++ { // ~6.7 s of flight
		eng.Fill(buf)
		g.Update(1.0/60, GameInput{DX: 0.3, DY: 0.2})
	}
	fr := g.MakeFrame(eng.BeatAt(float64(eng.OutTotal())), g.T, 1)
	fx := FX{Bloom: 1}
	cpu, gpu := &Rd{FX: &fx}, &Rd{FX: &fx}
	pix := make([]uint32, W*H)
	cpu.Render(g.Sc, &fr, W, H, pix)
	fbo, err := gl.Offscreen(W, H)
	if err != nil {
		t.Fatal(err)
	}
	gpu.Prepare(g.Sc, &fr, W, H)
	if err := gl.Frame(gpu, nil, W, H, fbo); err != nil {
		t.Fatal(err)
	}
	gp := gl.Read(fbo, W, H)
	var sum float64
	big := 0
	for i := range pix {
		d := 0
		for sh := 0; sh <= 16; sh += 8 {
			x := int(pix[i]>>sh&255) - int(gp[i]>>sh&255)
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
	mean := sum / float64(len(pix))
	t.Logf("game frame GPU vs CPU: mean diff %.2f/255, %.2f%% pixels off by >40", mean, 100*float64(big)/float64(len(pix)))
	if mean > 6 || float64(big)/float64(len(pix)) > 0.01 {
		t.Fail()
	}
}
