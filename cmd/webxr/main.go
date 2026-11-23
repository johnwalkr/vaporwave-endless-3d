//go:build js && wasm

// Command webxr is the Go core of the Quest (WebXR) version, compiled to
// WebAssembly. It exposes a global `ve3d` object to web/main.js. Bulk data
// (vertices, shader parameters, audio) stays in Go memory; JS reads it
// through views on the WebAssembly memory using the returned addresses.
package main

import (
	"syscall/js"
	"unsafe"

	vaporwave "vaporwave-endless-3d"
)

var (
	app        *vaporwave.Mobile
	runs       = make([]int32, 3*16384)
	comp       = make([]float32, 16)
	audioL     []float32
	audioR     []float32
	eye        vaporwave.EyeView
	paramsBuf  = make([]float32, vaporwave.NParams*4)
	geoScratch []float32
)

func addr[T any](s []T) int {
	if len(s) == 0 {
		return 0
	}
	return int(uintptr(unsafe.Pointer(&s[0])))
}

func f32(v js.Value) float32 { return float32(v.Float()) }

func fn(f func(args []js.Value) any) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any { return f(args) })
}

func main() {
	api := js.Global().Get("Object").New()
	set := func(name string, f func(args []js.Value) any) { api.Set(name, fn(f)) }

	set("init", func(a []js.Value) any {
		app = vaporwave.NewMobile("")
		if len(a) > 0 && a[0].Type() == js.TypeString && a[0].String() != "" {
			app.LoadSettingsText(a[0].String())
		} else {
			app.SetScale(50)
		}
		return nil
	})
	set("shaders", func(a []js.Value) any {
		o := js.Global().Get("Object").New()
		for k, v := range vaporwave.WebGLShaders() {
			o.Set(k, v)
		}
		o.Set("nParams", vaporwave.NParams)
		return o
	})
	set("frame", func(a []js.Value) any { app.Advance(a[0].Float(), a[1].Float()); return nil })
	set("scale", func(a []js.Value) any { return app.Scale() })
	// view(eye|null, w, h): prepares one view; with an eye, h is derived from its field of view.
	set("view", func(a []js.Value) any {
		w, h := a[1].Int(), a[2].Int()
		var e *vaporwave.EyeView
		if v := a[0]; !v.IsNull() && !v.IsUndefined() {
			g := func(i int) float32 { return f32(v.Index(i)) }
			eye = vaporwave.EyeView{
				R: vaporwave.V3{X: g(0), Y: g(1), Z: g(2)}, U: vaporwave.V3{X: g(3), Y: g(4), Z: g(5)},
				F: vaporwave.V3{X: g(6), Y: g(7), Z: g(8)}, Pos: vaporwave.V3{X: g(9), Y: g(10), Z: g(11)},
				TanL: g(12), TanR: g(13), TanU: g(14), TanD: g(15),
			}
			e = &eye
			w, h = vaporwave.EyeSize(e, w)
		}
		app.PrepareView(e, w, h)
		gf := app.GeometryFloats()
		if cap(geoScratch) < gf {
			geoScratch = make([]float32, gf+gf/2)
		}
		geoScratch = geoScratch[:gf]
		app.Geometry(geoScratch)
		nr := app.Runs(runs)
		app.Params(paramsBuf)
		ui := app.ParamsUI()
		cp, cf := app.Comp()
		copy(comp, cp[:])
		o := js.Global().Get("Object").New()
		o.Set("w", w)
		o.Set("h", h)
		o.Set("geo", addr(geoScratch))
		o.Set("geoFloats", gf)
		o.Set("runs", addr(runs))
		o.Set("nRuns", nr)
		o.Set("params", addr(paramsBuf))
		o.Set("ui", js.ValueOf([]any{ui[0], ui[1], ui[2], ui[3]}))
		o.Set("comp", addr(comp))
		o.Set("compF", js.ValueOf([]any{cf[0], cf[1], cf[2], cf[3]}))
		return o
	})
	// audio(n): renders n stereo samples; returns [addrL, addrR].
	set("audio", func(a []js.Value) any {
		n := a[0].Int()
		if cap(audioL) < n {
			audioL, audioR = make([]float32, n), make([]float32, n)
		}
		audioL, audioR = audioL[:n], audioR[:n]
		app.FillAudio(audioL, audioR)
		return js.ValueOf([]any{addr(audioL), addr(audioR)})
	})
	set("look", func(a []js.Value) any { app.Look(f32(a[0]), f32(a[1])); return nil })
	set("strafe", func(a []js.Value) any { app.SetStrafe(f32(a[0])); return nil })
	set("speed", func(a []js.Value) any { app.Speed(f32(a[0])); return nil })
	set("tap", func(a []js.Value) any { app.Tap(f32(a[0]), f32(a[1])); return nil })
	set("newScene", func(a []js.Value) any { app.NewScene(); return nil })
	set("toggleSpace", func(a []js.Value) any { app.ToggleSpace(); return nil })
	set("recolour", func(a []js.Value) any { app.Recolour(); return nil })
	set("text", func(a []js.Value) any {
		return js.ValueOf([]any{app.Title(), app.Info(), app.Toast(), app.SceneAge(), app.ShowFPS()})
	})
	set("menu", func(a []js.Value) any {
		rows := make([]any, app.MenuCount())
		for i := range rows {
			l, v, h := app.MenuItem(i)
			rows[i] = []any{l, v, h}
		}
		return js.ValueOf(rows)
	})
	set("menuAdjust", func(a []js.Value) any { app.MenuAdjust(a[0].Int(), a[1].Int()); return nil })
	set("settings", func(a []js.Value) any { return app.SettingsText() })

	js.Global().Set("ve3d", api)
	if cb := js.Global().Get("ve3dReady"); cb.Type() == js.TypeFunction {
		cb.Invoke()
	}
	select {}
}
