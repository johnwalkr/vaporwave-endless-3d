package main

import "math"

const nearZ = 0.3

type Cam struct {
	Pos     V3
	R, U, F V3
	f       float32
	cx, cy  float32
}

func (c *Cam) Set(pos V3, yaw, pitch, roll, vfov float32, W, H int) {
	c.Pos = pos
	sy, cy := sin32(yaw), cos32(yaw)
	sp, cp := sin32(pitch), cos32(pitch)
	c.F = V3{sy * cp, sp, cy * cp}
	r0 := V3{cy, 0, -sy}
	u0 := c.F.Cross(r0)
	sr, cr := sin32(roll), cos32(roll)
	c.R = r0.Mul(cr).Add(u0.Mul(sr))
	c.U = u0.Mul(cr).Sub(r0.Mul(sr))
	c.f = float32(H) * 0.5 / float32(math.Tan(float64(vfov)*0.5))
	c.cx, c.cy = float32(W)*0.5, float32(H)*0.5
}

func (c *Cam) View(p V3) V3 {
	d := p.Sub(c.Pos)
	return V3{d.Dot(c.R), d.Dot(c.U), d.Dot(c.F)}
}

// Dir transforms a direction (no translation).
func (c *Cam) Dir(d V3) V3 { return V3{d.Dot(c.R), d.Dot(c.U), d.Dot(c.F)} }

type FB struct {
	W, H   int
	C      []float32 // rgb, HDR
	Z      []float32 // 1/depth, 0 = infinitely far
	b0, b1 []float32
	bw, bh int
	cmds   []cmd
	gpu    bool
	GV     []float32 // GPU vertices: x y iz r g b a d h
	Runs   []GRun
	tris   []triCmd
	lines  []lineCmd
	vx2    []float32
	bxi    []int32
	bxf    []float32
}

func (fb *FB) Resize(w, h int) {
	fb.gpu = false
	if fb.W == w && fb.H == h {
		return
	}
	fb.W, fb.H = w, h
	fb.C = make([]float32, w*h*3)
	fb.Z = make([]float32, w*h)
	fb.bw, fb.bh = (w+3)/4, (h+3)/4
	fb.b0 = make([]float32, fb.bw*fb.bh*3)
	fb.b1 = make([]float32, fb.bw*fb.bh*3)
	fb.vx2 = nil
}

// GRun is a span of GPU vertices drawn with the same depth-write state.
type GRun struct {
	First, Count int32
	DepthWrite   bool
}

// SetGPU switches the framebuffer into display-list mode for the GPU path.
func (fb *FB) SetGPU(w, h int) {
	fb.gpu = true
	fb.W, fb.H = w, h
	fb.GV = fb.GV[:0]
	fb.Runs = fb.Runs[:0]
}

func (fb *FB) gbegin(dw bool) {
	n := int32(len(fb.GV) / 9)
	if k := len(fb.Runs); k > 0 && fb.Runs[k-1].DepthWrite == dw {
		return
	}
	fb.Runs = append(fb.Runs, GRun{First: n, DepthWrite: dw})
}

func (fb *FB) gvert(x, y, iz, r, g, b, a, d, h float32) {
	fb.GV = append(fb.GV, x, y, iz, r, g, b, a, d, h)
	fb.Runs[len(fb.Runs)-1].Count++
}

// gpuPix adds a single additive pixel (used for stars).
func (fb *FB) gpuPix(px, py int, r, g, b float32) {
	fb.gbegin(false)
	x0, y0 := float32(px)-0.5, float32(py)-0.5
	x1, y1 := x0+1, y0+1
	for _, p := range [6][2]float32{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y0}, {x1, y1}, {x0, y1}} {
		fb.gvert(p[0], p[1], 0, r, g, b, 0, 0, 1e4)
	}
}

type TV struct {
	P V3
	C Col
}

type sv struct{ x, y, iz, r, g, b float32 }

func lerpTV(a, b TV, t float32) TV {
	return TV{a.P.Add(b.P.Sub(a.P).Mul(t)), mixc(a.C, b.C, t)}
}

// triV draws a view-space triangle with near clipping.
func (fb *FB) triV(cam *Cam, v *[3]TV, alpha, bias float32) {
	var in [4]TV
	n := 0
	for i := 0; i < 3; i++ {
		a, b := v[i], v[(i+1)%3]
		ain, bin := a.P.Z >= nearZ, b.P.Z >= nearZ
		if ain {
			in[n] = a
			n++
		}
		if ain != bin {
			in[n] = lerpTV(a, b, (nearZ-a.P.Z)/(b.P.Z-a.P.Z))
			n++
		}
	}
	if n < 3 {
		return
	}
	var s [4]sv
	for i := 0; i < n; i++ {
		iz := 1 / in[i].P.Z
		s[i] = sv{cam.cx + cam.f*in[i].P.X*iz, cam.cy - cam.f*in[i].P.Y*iz, iz, in[i].C.R, in[i].C.G, in[i].C.B}
	}
	fb.tri(&s[0], &s[1], &s[2], alpha, bias)
	if n == 4 {
		fb.tri(&s[0], &s[2], &s[3], alpha, bias)
	}
}

type triCmd struct {
	x0, y0, x1, y1                       int32
	wa0, wb0, dwadx, dwady, dwbdx, dwbdy float64
	iz, izx, izy                         float32
	r, rx, ry, g, gx, gy, b, bx, by      float32
	alpha                                float32
}

type lineCmd struct {
	x0, y0, iz0, dx, dy, diz float32
	c0, dc                   Col
	half, cosT               float32
	span, n                  int32
	ymin, ymax               int32
	steep, add               bool
}

type cmd struct {
	line bool
	idx  int32
}

// tri queues a screen-space triangle.
func (fb *FB) tri(a, b, c *sv, alpha, bias float32) {
	minx, maxx := a.x, a.x
	miny, maxy := a.y, a.y
	if b.x < minx {
		minx = b.x
	}
	if c.x < minx {
		minx = c.x
	}
	if b.x > maxx {
		maxx = b.x
	}
	if c.x > maxx {
		maxx = c.x
	}
	if b.y < miny {
		miny = b.y
	}
	if c.y < miny {
		miny = c.y
	}
	if b.y > maxy {
		maxy = b.y
	}
	if c.y > maxy {
		maxy = c.y
	}
	if maxx < 0 || maxy < 0 || minx > float32(fb.W-1) || miny > float32(fb.H-1) {
		return
	}
	x0, y0 := 0, 0
	if minx > 0 {
		x0 = int(minx)
	}
	if miny > 0 {
		y0 = int(miny)
	}
	x1, y1 := fb.W-1, fb.H-1
	if maxx < float32(x1) {
		x1 = int(maxx) + 1
	}
	if maxy < float32(y1) {
		y1 = int(maxy) + 1
	}
	ax, ay := float64(a.x), float64(a.y)
	bx, by := float64(b.x), float64(b.y)
	cx, cy := float64(c.x), float64(c.y)
	area := (bx-ax)*(cy-ay) - (cx-ax)*(by-ay)
	if area > -1e-9 && area < 1e-9 {
		return
	}
	if fb.gpu {
		zs := 1 + bias
		fb.gbegin(alpha >= 1)
		for _, v := range [3]*sv{a, b, c} {
			fb.gvert(v.x, v.y, v.iz*zs, v.r*alpha, v.g*alpha, v.b*alpha, alpha, 0, 1e4)
		}
		return
	}
	inv := 1 / area
	t := triCmd{x0: int32(x0), y0: int32(y0), x1: int32(x1), y1: int32(y1), alpha: alpha}
	t.dwadx, t.dwady = (by-cy)*inv, (cx-bx)*inv
	t.dwbdx, t.dwbdy = (cy-ay)*inv, (ax-cx)*inv
	px0, py0 := float64(x0), float64(y0)
	t.wa0 = ((bx-px0)*(cy-py0) - (cx-px0)*(by-py0)) * inv
	t.wb0 = ((cx-px0)*(ay-py0) - (ax-px0)*(cy-py0)) * inv
	plane := func(va, vb, vc float32) (float32, float32, float32) {
		da, db := float64(va-vc), float64(vb-vc)
		return float32(float64(vc) + da*t.wa0 + db*t.wb0), float32(da*t.dwadx + db*t.dwbdx), float32(da*t.dwady + db*t.dwbdy)
	}
	zs := 1 + bias
	t.iz, t.izx, t.izy = plane(a.iz*zs, b.iz*zs, c.iz*zs)
	t.r, t.rx, t.ry = plane(a.r, b.r, c.r)
	t.g, t.gx, t.gy = plane(a.g, b.g, c.g)
	t.b, t.bx, t.by = plane(a.b, b.b, c.b)
	fb.cmds = append(fb.cmds, cmd{false, int32(len(fb.tris))})
	fb.tris = append(fb.tris, t)
}

func (fb *FB) triBand(t *triCmd, yLo, yHi int) {
	ya, yb := int(t.y0), int(t.y1)
	if ya < yLo {
		ya = yLo
	}
	if yb > yHi-1 {
		yb = yHi - 1
	}
	dwcdx := -(t.dwadx + t.dwbdx)
	W := fb.W
	for y := ya; y <= yb; y++ {
		dyf := float64(y - int(t.y0))
		wa := t.wa0 + t.dwady*dyf
		wb := t.wb0 + t.dwbdy*dyf
		wc := 1 - wa - wb
		xs, xe := 0.0, float64(t.x1-t.x0)
		if t.dwadx > 1e-12 {
			if v := -wa / t.dwadx; v > xs {
				xs = v
			}
		} else if t.dwadx < -1e-12 {
			if v := -wa / t.dwadx; v < xe {
				xe = v
			}
		} else if wa < 0 {
			continue
		}
		if t.dwbdx > 1e-12 {
			if v := -wb / t.dwbdx; v > xs {
				xs = v
			}
		} else if t.dwbdx < -1e-12 {
			if v := -wb / t.dwbdx; v < xe {
				xe = v
			}
		} else if wb < 0 {
			continue
		}
		if dwcdx > 1e-12 {
			if v := -wc / dwcdx; v > xs {
				xs = v
			}
		} else if dwcdx < -1e-12 {
			if v := -wc / dwcdx; v < xe {
				xe = v
			}
		} else if wc < 0 {
			continue
		}
		ixs := int(math.Ceil(xs - 1e-5))
		ixe := int(math.Floor(xe + 1e-5))
		if ixs > ixe {
			continue
		}
		fx, fy := float32(ixs), float32(dyf)
		iz := t.iz + t.izx*fx + t.izy*fy
		r := t.r + t.rx*fx + t.ry*fy
		g := t.g + t.gx*fx + t.gy*fy
		b := t.b + t.bx*fx + t.by*fy
		idx := y*W + int(t.x0) + ixs
		zrow := fb.Z[idx : idx+ixe-ixs+1]
		crow := fb.C[idx*3 : (idx+ixe-ixs+1)*3]
		if t.alpha >= 1 {
			for i := range zrow {
				if iz > zrow[i] {
					zrow[i] = iz
					crow[i*3], crow[i*3+1], crow[i*3+2] = r, g, b
				}
				iz += t.izx
				r += t.rx
				g += t.gx
				b += t.bx
			}
		} else {
			al := t.alpha
			for i := range zrow {
				if iz > zrow[i] {
					crow[i*3] += (r - crow[i*3]) * al
					crow[i*3+1] += (g - crow[i*3+1]) * al
					crow[i*3+2] += (b - crow[i*3+2]) * al
				}
				iz += t.izx
				r += t.rx
				g += t.gx
				b += t.bx
			}
		}
	}
}

// lineV queues a view-space line. add=true is additive (neon), otherwise
// it is alpha-blended. Depth tested but never written.
func (fb *FB) lineV(cam *Cam, a, b V3, ca, cb Col, width float32, add bool) {
	if a.Z < nearZ && b.Z < nearZ {
		return
	}
	if a.Z < nearZ {
		t := (nearZ - a.Z) / (b.Z - a.Z)
		a = a.Add(b.Sub(a).Mul(t))
		ca = mixc(ca, cb, t)
	} else if b.Z < nearZ {
		t := (nearZ - b.Z) / (a.Z - b.Z)
		b = b.Add(a.Sub(b).Mul(t))
		cb = mixc(cb, ca, t)
	}
	iz0, iz1 := 1/a.Z, 1/b.Z
	x0, y0 := cam.cx+cam.f*a.X*iz0, cam.cy-cam.f*a.Y*iz0
	x1, y1 := cam.cx+cam.f*b.X*iz1, cam.cy-cam.f*b.Y*iz1
	// Liang-Barsky clip
	m := width + 2
	xmin, ymin, xmax, ymax := -m, -m, float32(fb.W)+m, float32(fb.H)+m
	dx, dy := x1-x0, y1-y0
	t0, t1 := float32(0), float32(1)
	clip := func(p, q float32) bool {
		if p == 0 {
			return q >= 0
		}
		r := q / p
		if p < 0 {
			if r > t1 {
				return false
			}
			if r > t0 {
				t0 = r
			}
		} else {
			if r < t0 {
				return false
			}
			if r < t1 {
				t1 = r
			}
		}
		return true
	}
	if !clip(-dx, x0-xmin) || !clip(dx, xmax-x0) || !clip(-dy, y0-ymin) || !clip(dy, ymax-y0) {
		return
	}
	if t0 > 0 || t1 < 1 {
		nx0, ny0, nz0, nc0 := x0+dx*t0, y0+dy*t0, iz0+(iz1-iz0)*t0, mixc(ca, cb, t0)
		x1, y1, iz1, cb = x0+dx*t1, y0+dy*t1, iz0+(iz1-iz0)*t1, mixc(ca, cb, t1)
		x0, y0, iz0, ca = nx0, ny0, nz0, nc0
		dx, dy = x1-x0, y1-y0
	}
	if fb.gpu {
		fb.gpuLine(x0, y0, iz0*1.03, x1, y1, iz1*1.03, ca, cb, width*0.5, add)
		return
	}
	adx, ady := abs32(dx), abs32(dy)
	ln := sqrt32(dx*dx + dy*dy)
	l := lineCmd{x0: x0, y0: y0, iz0: iz0 * 1.03, dx: dx, dy: dy, diz: (iz1 - iz0) * 1.03,
		c0: ca, dc: Col{cb.R - ca.R, cb.G - ca.G, cb.B - ca.B}, add: add, cosT: 1}
	l.steep = ady > adx
	major := adx
	if l.steep {
		major = ady
	}
	if ln > 1e-3 {
		l.cosT = major / ln
	}
	l.half = width * 0.5
	l.span = int32(l.half/l.cosT+1.5) + 1
	l.n = int32(major) + 1
	lo, hi := y0, y1
	if lo > hi {
		lo, hi = hi, lo
	}
	l.ymin, l.ymax = ifloor(lo)-l.span-1, ifloor(hi)+l.span+1
	fb.cmds = append(fb.cmds, cmd{true, int32(len(fb.lines))})
	fb.lines = append(fb.lines, l)
}

// gpuLine expands an antialiased line into a quad carrying the signed
// distance from the centre line, so the shader can compute coverage.
func (fb *FB) gpuLine(x0, y0, iz0, x1, y1, iz1 float32, ca, cb Col, half float32, add bool) {
	dx, dy := x1-x0, y1-y0
	ln := sqrt32(dx*dx + dy*dy)
	ux, uy := float32(1), float32(0)
	if ln > 1e-4 {
		ux, uy = dx/ln, dy/ln
	}
	nx, ny := -uy, ux
	ext := half + 1
	const e = 0.5
	a := float32(1)
	if add {
		a = 0
	}
	type q struct {
		x, y, iz, d float32
		c           Col
	}
	p := [4]q{
		{x0 - ux*e + nx*ext, y0 - uy*e + ny*ext, iz0, ext, ca},
		{x0 - ux*e - nx*ext, y0 - uy*e - ny*ext, iz0, -ext, ca},
		{x1 + ux*e - nx*ext, y1 + uy*e - ny*ext, iz1, -ext, cb},
		{x1 + ux*e + nx*ext, y1 + uy*e + ny*ext, iz1, ext, cb},
	}
	fb.gbegin(false)
	for _, i := range [6]int{0, 1, 2, 0, 2, 3} {
		v := &p[i]
		fb.gvert(v.x, v.y, v.iz, v.c.R, v.c.G, v.c.B, a, v.d, half)
	}
}

func (fb *FB) lineBand(l *lineCmd, yLo, yHi int) {
	if int(l.ymax) < yLo || int(l.ymin) >= yHi {
		return
	}
	n := int(l.n)
	i0, i1 := 0, n
	if abs32(l.dy) > 1e-4 {
		m := float32(l.span + 1)
		ta := (float32(yLo) - m - l.y0) / l.dy
		tb := (float32(yHi) + m - l.y0) / l.dy
		if ta > tb {
			ta, tb = tb, ta
		}
		if a := int(ta*float32(n)) - 1; a > i0 {
			i0 = a
		}
		if b := int(tb*float32(n)) + 2; b < i1 {
			i1 = b
		}
	}
	inv := 1 / float32(n)
	W := fb.W
	span := int(l.span)
	for i := i0; i <= i1; i++ {
		t := float32(i) * inv
		x, y := l.x0+l.dx*t, l.y0+l.dy*t
		iz := l.iz0 + l.diz*t
		r, g, bl := l.c0.R+l.dc.R*t, l.c0.G+l.dc.G*t, l.c0.B+l.dc.B*t
		var mi int
		var mf float32
		if l.steep {
			mi = int(ifloor(x))
			mf = x
		} else {
			mi = int(ifloor(y))
			mf = y
		}
		for k := mi - span + 1; k <= mi+span; k++ {
			d := abs32(float32(k)-mf) * l.cosT
			cov := l.half + 0.5 - d
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			var px, py int
			if l.steep {
				px, py = k, int(ifloor(y+0.5))
			} else {
				px, py = int(ifloor(x+0.5)), k
			}
			if px < 0 || py < yLo || px >= W || py >= yHi {
				continue
			}
			idx := py*W + px
			if iz < fb.Z[idx] {
				continue
			}
			p := fb.C[idx*3 : idx*3+3 : idx*3+3]
			if l.add {
				p[0] += r * cov
				p[1] += g * cov
				p[2] += bl * cov
			} else {
				p[0] += (r - p[0]) * cov
				p[1] += (g - p[1]) * cov
				p[2] += (bl - p[2]) * cov
			}
		}
	}
}

// flush rasterises all queued primitives, in order, across row bands.
func (fb *FB) flush() {
	band := fb.H / (nWorkers * 3)
	if band < 8 {
		band = 8
	}
	parallel(fb.H, band, func(a, b int) {
		for _, c := range fb.cmds {
			if c.line {
				fb.lineBand(&fb.lines[c.idx], a, b)
			} else {
				t := &fb.tris[c.idx]
				if int(t.y1) >= a && int(t.y0) < b {
					fb.triBand(t, a, b)
				}
			}
		}
	})
	fb.cmds, fb.tris, fb.lines = fb.cmds[:0], fb.tris[:0], fb.lines[:0]
}

// post applies bloom, chromatic aberration, scanlines, vignette, grain and
// VHS glitches, and converts to 0x00RRGGBB pixels.
func (fb *FB) post(out []uint32, bloom, glitch float32, frame uint32, flash float32, fx FX) {
	W, H, bw, bh := fb.W, fb.H, fb.bw, fb.bh
	const thr = 0.62
	parallel(bh, 8, func(a, b int) {
		for by := a; by < b; by++ {
			for bx := 0; bx < bw; bx++ {
				var r, g, bl float32
				cnt := 0
				for yy := by * 4; yy < by*4+4 && yy < H; yy++ {
					p := (yy*W + bx*4) * 3
					for xx := bx * 4; xx < bx*4+4 && xx < W; xx++ {
						if v := fb.C[p] - thr; v > 0 {
							r += v
						}
						if v := fb.C[p+1] - thr; v > 0 {
							g += v
						}
						if v := fb.C[p+2] - thr; v > 0 {
							bl += v
						}
						p += 3
						cnt++
					}
				}
				k := 1 / float32(cnt)
				o := (by*bw + bx) * 3
				fb.b0[o], fb.b0[o+1], fb.b0[o+2] = r*k, g*k, bl*k
			}
		}
	})
	// separable box blurs
	blurH := func(src, dst []float32, rad int) {
		k := 1 / float32(2*rad+1)
		for y := 0; y < bh; y++ {
			for c := 0; c < 3; c++ {
				var s float32
				row := y * bw * 3
				for x := -rad; x <= rad; x++ {
					xx := min(max(x, 0), bw-1)
					s += src[row+xx*3+c]
				}
				for x := 0; x < bw; x++ {
					dst[row+x*3+c] = s * k
					s += src[row+min(x+rad+1, bw-1)*3+c] - src[row+max(x-rad, 0)*3+c]
				}
			}
		}
	}
	blurV := func(src, dst []float32, rad int) {
		k := 1 / float32(2*rad+1)
		for x := 0; x < bw; x++ {
			for c := 0; c < 3; c++ {
				var s float32
				for y := -rad; y <= rad; y++ {
					yy := min(max(y, 0), bh-1)
					s += src[(yy*bw+x)*3+c]
				}
				for y := 0; y < bh; y++ {
					dst[(y*bw+x)*3+c] = s * k
					s += src[(min(y+rad+1, bh-1)*bw+x)*3+c] - src[(max(y-rad, 0)*bw+x)*3+c]
				}
			}
		}
	}
	blurH(fb.b0, fb.b1, 2)
	blurV(fb.b1, fb.b0, 2)
	blurH(fb.b0, fb.b1, 4)
	blurV(fb.b1, fb.b0, 4)

	invW, invH := 1/float32(W), 1/float32(H)
	// per-column tables
	if len(fb.vx2) != W {
		fb.vx2 = make([]float32, W)
		fb.bxi = make([]int32, W)
		fb.bxf = make([]float32, W)
		for x := 0; x < W; x++ {
			vx := float32(x)*invW - 0.5
			fb.vx2[x] = vx * vx
			f := (float32(x)+0.5)*0.25 - 0.5
			if f < 0 {
				f = 0
			}
			i := int(f)
			if i > bw-2 {
				i = max(bw-2, 0)
			}
			fb.bxi[x] = int32(i * 3)
			fb.bxf[x] = clamp01(f - float32(i))
		}
	}
	parallel(H, 16, func(ya, yb int) {
		brow := make([]float32, (bw+1)*3)
		for y := ya; y < yb; y++ {
			rs := hashu(uint32(y), frame, 99)
			// horizontal glitch displacement for this row
			off := 0
			if glitch > 0 {
				band := hashu(uint32(y/(6+int(frame%5)*3)), frame/2, 5)
				if float32(band&1023)/1023 < glitch*0.8 {
					off = int((float32(band>>10&1023)/1023 - 0.5) * glitch * float32(W) * 0.35)
				}
			}
			ca := int(fx.CA) + int(glitch*6)
			sl := float32(1)
			if y%3 == 2 {
				sl = 1 - 0.16*fx.Scan
			}
			// slow rolling tracking bar
			tb := float32((int(frame)*2+H-y)%(H*2)) * invH
			if tb < 0.06 {
				sl *= 1 + 0.08*fx.Track
			}
			vy := float32(y)*invH - 0.5
			vy2 := vy * vy
			by := (float32(y)+0.5)*0.25 - 0.5
			if by < 0 {
				by = 0
			}
			by0 := int(by)
			by1 := min(by0+1, bh-1)
			fy := by - float32(by0)
			r0, r1 := fb.b0[by0*bw*3:(by0+1)*bw*3], fb.b0[by1*bw*3:(by1+1)*bw*3]
			for i := range r0 {
				brow[i] = (r0[i] + (r1[i]-r0[i])*fy) * bloom
			}
			gamp := 0.035*fx.Grain + glitch*0.25
			orow := y * W
			crow := fb.C[orow*3 : (orow+W)*3]
			orow2 := out[orow : orow+W]
			for x := 0; x < W; x++ {
				sx := x + off
				if sx < 0 {
					sx += W
				} else if sx >= W {
					sx -= W
				}
				xr, xb := sx-ca, sx+ca
				if xr < 0 {
					xr = 0
				}
				if xb >= W {
					xb = W - 1
				}
				bi := fb.bxi[sx]
				fx := fb.bxf[sx]
				bp := brow[bi : bi+6 : bi+6]
				v := (1 - 0.55*(fb.vx2[x]+vy2)) * sl
				rs = rs*1664525 + 1013904223
				gr := (float32(rs>>24)*(1.0/255)-0.5)*gamp + flash
				r := (crow[xr*3]+bp[0]+(bp[3]-bp[0])*fx)*v + gr
				g := (crow[sx*3+1]+bp[1]+(bp[4]-bp[1])*fx)*v + gr
				bl := (crow[xb*3+2]+bp[2]+(bp[5]-bp[2])*fx)*v + gr
				orow2[x] = tone(r)<<16 | tone(g)<<8 | tone(bl)
			}
		}
	})
}

var toneLUT = func() (t [1024]uint8) {
	for i := range t {
		x := float32(i) / 256
		if x > 0.75 {
			x = 0.75 + 0.25*(1-1/(1+(x-0.75)*4))
		}
		t[i] = uint8(x*255 + 0.5)
	}
	return
}()

func tone(x float32) uint32 {
	i := int32(x * 256)
	if i < 0 {
		i = 0
	}
	if i > 1023 {
		i = 1023
	}
	return uint32(toneLUT[i])
}
