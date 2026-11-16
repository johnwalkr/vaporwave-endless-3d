package vaporwave

import (
	"os"
	"path/filepath"
	"testing"
)

const metalShadersPath = "ios/App/Shaders.metal"

// The Metal shaders are generated from the OpenGL ones and must stay in sync.
func TestMetalShaders(t *testing.T) {
	want := MetalShaders()
	if os.Getenv("UPDATE_SHADERS") != "" {
		if err := os.WriteFile(metalShadersPath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(metalShadersPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s is out of date; run UPDATE_SHADERS=1 go test -run TestMetalShaders .", metalShadersPath)
	}
}

// Exercise the iOS facade the way the app does and check the frame data.
func TestMobileFacade(t *testing.T) {
	dir := t.TempDir()
	m := NewMobile(dir)
	m.Resize(2556, 1179)
	l, r := make([]float32, 512), make([]float32, 512)
	params := make([]float32, NParams*4)
	runs := make([]int32, 3*8192)
	geo := make([]float32, 1<<21)
	var maxAbs float32
	for f := 0; f < 240; f++ {
		m.FillAudio(l, r)
		m.FillAudio(l, r)
		m.FillAudio(l, r)
		for _, v := range l {
			maxAbs = max(maxAbs, v, -v)
		}
		switch f {
		case 30:
			m.Look(0.2, -0.1)
		case 60:
			m.Tap(0.5, 0.8)
		case 90:
			m.SetStrafe(1)
		case 100:
			m.SetStrafe(0)
		case 120:
			m.ToggleSpace()
		case 150:
			m.Altitude(1.3)
			m.Speed(0.2)
		case 180:
			m.NewScene()
			m.Recolour()
		}
		m.Frame(1.0/60, 0.02)
		iw, ih := m.InternalSize()
		if iw != 2556*67/100 || ih != 1179*67/100 {
			t.Fatalf("internal size %dx%d", iw, ih)
		}
		if n := m.Params(params); n != NParams*4 {
			t.Fatalf("params %d", n)
		}
		gf := m.GeometryFloats()
		if gf%9 != 0 || gf > len(geo) {
			t.Fatalf("geometry floats %d", gf)
		}
		m.Geometry(geo)
		nr := m.Runs(runs)
		total := 0
		for i := 0; i < nr; i++ {
			if runs[3*i] != int32(total) && runs[3*i] < int32(total) {
				t.Fatalf("run %d starts at %d, before %d", i, runs[3*i], total)
			}
			total = int(runs[3*i] + runs[3*i+1])
		}
		if total > gf/9 {
			t.Fatalf("runs cover %d vertices, only %d exist", total, gf/9)
		}
		cp, _ := m.Comp()
		if cp[4] != float32(iw) || cp[5] != float32(ih) {
			t.Fatalf("comp size %v", cp)
		}
	}
	if maxAbs == 0 || maxAbs > 1 {
		t.Fatalf("audio peak %v", maxAbs)
	}
	// every settings row can be changed both ways, then saved and loaded
	for i := 0; i < m.MenuCount(); i++ {
		label, _, header := m.MenuItem(i)
		if header || label == "New track for this scene" || label == "Reset all to defaults" {
			continue
		}
		m.MenuAdjust(i, 1)
		m.MenuAdjust(i, -1)
	}
	m.Frame(1.0, 0.02) // let a pending remix happen
	m.Save()
	if _, err := os.Stat(filepath.Join(dir, "settings.ini")); err != nil {
		t.Fatal(err)
	}
	if m.Title() == "" || m.Info() == "" {
		t.Fatal("empty HUD text")
	}
	t.Logf("menu rows %d, last frame %d vertices in %d runs, audio peak %.2f", m.MenuCount(), m.GeometryFloats()/9, m.Runs(runs), maxAbs)
}
