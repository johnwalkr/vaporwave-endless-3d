//go:build linux && cgo

package vaporwave

import (
	"runtime"
	"testing"
)

// The HUD/menu overlay must land the right way up and darken where alpha is set.
func TestGPUHudOverlay(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	g, err := HeadlessGL()
	if err != nil {
		t.Skip(err)
	}
	const W, H = 320, 240
	sc := NewScene(4)
	pl := &Player{H: sc.CamH}
	fr := MakeFrame(sc, pl, 9, 5, 5, 1)
	none := FX{Bloom: 1}
	rd := &Rd{FX: &none}
	fbo, err := g.Offscreen(W, H)
	if err != nil {
		t.Fatal(err)
	}
	rd.Prepare(sc, &fr, W, H)
	if err := g.Frame(rd, nil, W, H, fbo); err != nil {
		t.Fatal(err)
	}
	base := g.Read(fbo, W, H)
	hud := make([]uint32, W*H)
	for y := 10; y < 30; y++ { // white block, top-left
		for x := 10; x < 60; x++ {
			hud[y*W+x] = 0xFFFFFF
		}
	}
	for y := 100; y < 200; y++ { // dark panel
		for x := 100; x < 300; x++ {
			hud[y*W+x] = 200 << 24
		}
	}
	rd.Prepare(sc, &fr, W, H)
	if err := g.Frame(rd, hud, W, H, fbo); err != nil {
		t.Fatal(err)
	}
	got := g.Read(fbo, W, H)
	lum := func(p uint32) int { return int(p>>16&255) + int(p>>8&255) + int(p&255) }
	if l := lum(got[20*W+30]); l < 700 {
		t.Errorf("HUD text not at top-left (lum %d)", l)
	}
	if l := lum(got[(H-20)*W+30]); l > 700 && lum(base[(H-20)*W+30]) < 700 {
		t.Errorf("HUD drawn upside down")
	}
	var a, b int
	for y := 110; y < 190; y++ {
		for x := 110; x < 290; x++ {
			a += lum(base[y*W+x])
			b += lum(got[y*W+x])
		}
	}
	if r := float64(b) / float64(a); r > 0.35 || r < 0.1 {
		t.Errorf("panel darkening ratio %.2f, want ~0.22", r)
	}
	t.Logf("panel ratio %.2f, text lum %d", float64(b)/float64(a), lum(got[20*W+30]))
}
