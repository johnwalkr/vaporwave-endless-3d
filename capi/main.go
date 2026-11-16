// Package main is the C API of the Go core for the iOS app. It is built with
// -buildmode=c-archive (see ios/build-go.sh), which also writes libve3d.h.
// Go never hands pointers to Go memory to C: callers pass buffers and Go
// copies into them.
package main

/*
#include <stdint.h>
*/
import "C"

import (
	"unsafe"

	vaporwave "vaporwave-endless-3d"
)

var app *vaporwave.Mobile

func floats(p *C.float, n C.int) []float32 {
	if p == nil || n <= 0 {
		return nil
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(p)), int(n))
}

func putString(s string, buf *C.char, n C.int) C.int {
	if buf == nil || n <= 0 {
		return 0
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(n))
	k := copy(dst[:n-1], s)
	dst[k] = 0
	return C.int(k)
}

//export ve3d_init
func ve3d_init(dir *C.char) { app = vaporwave.NewMobile(C.GoString(dir)) }

//export ve3d_save
func ve3d_save() { app.Save() }

//export ve3d_resize
func ve3d_resize(w, h C.int) { app.Resize(int(w), int(h)) }

//export ve3d_frame
func ve3d_frame(dt, latency C.double) { app.Frame(float64(dt), float64(latency)) }

//export ve3d_internal_size
func ve3d_internal_size(w, h *C.int) {
	iw, ih := app.InternalSize()
	*w, *h = C.int(iw), C.int(ih)
}

// ve3d_params copies the sky/floor parameters; returns the float count (NParams*4).
//
//export ve3d_params
func ve3d_params(dst *C.float, n C.int, ui *C.uint32_t) C.int {
	u := app.ParamsUI()
	if ui != nil {
		copy(unsafe.Slice((*uint32)(unsafe.Pointer(ui)), 4), u[:])
	}
	return C.int(app.Params(floats(dst, n)))
}

//export ve3d_param_count
func ve3d_param_count() C.int { return C.int(vaporwave.NParams * 4) }

//export ve3d_geometry_floats
func ve3d_geometry_floats() C.int { return C.int(app.GeometryFloats()) }

//export ve3d_geometry
func ve3d_geometry(dst *C.float, n C.int) C.int { return C.int(app.Geometry(floats(dst, n))) }

// ve3d_runs writes (first, count, depthWrite) triples and returns the run count.
//
//export ve3d_runs
func ve3d_runs(dst *C.int32_t, n C.int) C.int {
	if dst == nil || n <= 0 {
		return 0
	}
	return C.int(app.Runs(unsafe.Slice((*int32)(unsafe.Pointer(dst)), int(n))))
}

//export ve3d_comp
func ve3d_comp(p *C.float, f *C.uint32_t) {
	cp, cf := app.Comp()
	copy(floats(p, 12), cp[:])
	copy(unsafe.Slice((*uint32)(unsafe.Pointer(f)), 4), cf[:])
}

//export ve3d_audio
func ve3d_audio(l, r *C.float, n C.int) { app.FillAudio(floats(l, n), floats(r, n)) }

//export ve3d_look
func ve3d_look(dx, dy C.float) { app.Look(float32(dx), float32(dy)) }

//export ve3d_strafe
func ve3d_strafe(v C.float) { app.SetStrafe(float32(v)) }

//export ve3d_speed
func ve3d_speed(dv C.float) { app.Speed(float32(dv)) }

//export ve3d_altitude
func ve3d_altitude(s C.float) { app.Altitude(float32(s)) }

//export ve3d_tap
func ve3d_tap(x, y C.float) { app.Tap(float32(x), float32(y)) }

//export ve3d_new_scene
func ve3d_new_scene() { app.NewScene() }

//export ve3d_toggle_space
func ve3d_toggle_space() { app.ToggleSpace() }

//export ve3d_recolour
func ve3d_recolour() { app.Recolour() }

// ve3d_text: which 0 = title, 1 = info line, 2 = toast.
//
//export ve3d_text
func ve3d_text(which C.int, buf *C.char, n C.int) C.int {
	switch which {
	case 0:
		return putString(app.Title(), buf, n)
	case 1:
		return putString(app.Info(), buf, n)
	default:
		return putString(app.Toast(), buf, n)
	}
}

//export ve3d_scene_age
func ve3d_scene_age() C.double { return C.double(app.SceneAge()) }

//export ve3d_show_fps
func ve3d_show_fps() C.int {
	if app.ShowFPS() {
		return 1
	}
	return 0
}

//export ve3d_menu_count
func ve3d_menu_count() C.int { return C.int(app.MenuCount()) }

// ve3d_menu_item writes the label and value; returns 1 for a section header.
//
//export ve3d_menu_item
func ve3d_menu_item(i C.int, label *C.char, ln C.int, value *C.char, vn C.int) C.int {
	l, v, h := app.MenuItem(int(i))
	putString(l, label, ln)
	putString(v, value, vn)
	if h {
		return 1
	}
	return 0
}

//export ve3d_menu_adjust
func ve3d_menu_adjust(i, d C.int) { app.MenuAdjust(int(i), int(d)) }

func main() {}
