package main

import "math"

var lightDir = V3{-0.45, 0.65, -0.6}.Norm()

func shade(n V3) float32 {
	d := n.Dot(lightDir)
	if d < 0 {
		d = 0
	}
	return 0.5 + 0.5*d
}

func (rd *Rd) props() {
	sc := rd.sc
	rowSp := float64(sc.U) * 2
	nNear := int64(math.Floor(rd.camZ/rowSp)) - 1
	nFar := nNear + int64(farDist/rowSp) + 1
	kc := int32(math.Round(float64(rd.cam.Pos.X / sc.ColSp)))
	gateSp := float64(sc.U) * float64(sc.GateBeats)
	tanH := rd.cam.cx / rd.cam.f
	for n := nFar; n >= nNear; n-- {
		zw := float64(n) * rowSp
		// gates sit between this row and the next further one
		if sc.Gate != 0 {
			g0 := int64(math.Ceil(zw / gateSp))
			for g := g0; float64(g)*gateSp < zw+rowSp; g++ {
				rd.gate(g, rd.rz(float64(g)*gateSp))
			}
		}
		for kx := kc - 9; kx <= kc+9; kx++ {
			if kx == 0 {
				continue
			}
			inner := kx == 1 || kx == -1
			if sc.FloorType == FloorValley && !sc.Hills && !inner {
				continue
			}
			h := hashu(sc.Seed, uint32(n), uint32(kx)+1000)
			kind := -1
			x := float32(kx) * sc.ColSp
			z := zw
			if inner && sc.Avenue >= 0 {
				if hf(h, 1) < 0.93 {
					kind = sc.Avenue
				}
			} else if len(sc.Outer) > 0 && hf(h, 1) < sc.Density {
				kind = sc.Outer[int(h>>4)%len(sc.Outer)]
				if !inner {
					x += (hf(h, 2) - 0.5) * sc.ColSp * 0.6
					z += float64(hf(h, 3)-0.5) * rowSp * 0.6
				}
			}
			if kind < 0 {
				continue
			}
			base := V3{x, 0, rd.rz(z)}
			if sc.Hills {
				base.Y = rd.terrAt(x, z)
			}
			v := rd.cam.View(base.Add(V3{0, 4, 0}))
			if v.Z < -12 || v.Z > farDist+10 || abs32(v.X) > (v.Z+14)*tanH*1.25+14 {
				continue
			}
			switch kind {
			case PropPalm:
				rd.palm(base, h, v.Z)
			case PropColumn:
				rd.column(base, h, v.Z)
			case PropDiamond:
				rd.diamond(base, h)
			case PropSphere:
				rd.sphere(base, h, v.Z)
			case PropCube:
				rd.cube(base, h)
			case PropPanel:
				rd.panel(base, h)
			case PropTri:
				rd.trisign(base, h)
			case PropRock:
				rd.rock(base, h)
			case PropPine:
				rd.pine(base, h)
			case PropSpike:
				rd.spike(base, h)
			case PropTower:
				rd.tower(base, h, v.Z)
			}
		}
	}
}

func (rd *Rd) gate(n int64, z float32) {
	if z < rd.cam.Pos.Z-1 {
		return
	}
	sc := rd.sc
	p := &sc.P
	cols := [3]Col{p.Line, p.Acc, p.SunT}
	c := cols[((n%3)+3)%3]
	w := sc.ColSp * 0.6
	hgt := w * 1.25
	var pts []V3
	closed := false
	switch sc.Gate {
	case 1:
		pts = []V3{{-w, 0, z}, {-w, hgt, z}, {w, hgt, z}, {w, 0, z}}
	case 2:
		pts = []V3{{-w, 0, z}, {0, hgt * 1.3, z}, {w, 0, z}}
		closed = true
	case 3:
		for i := 0; i < 28; i++ {
			a := float32(i) / 28 * 2 * math.Pi
			pts = append(pts, V3{sin32(a) * w, w*0.98 + cos32(a)*w, z})
		}
		closed = true
	default:
		for i := 0; i < 6; i++ {
			a := (float32(i) + 0.5) / 6 * 2 * math.Pi
			pts = append(pts, V3{sin32(a) * w * 1.1, w*0.96 + cos32(a)*w*1.1, z})
		}
		closed = true
	}
	in := 1.0 + 0.9*rd.fr.Kick
	core := mixc(c, Col{1, 1, 1}, 0.65)
	m := len(pts)
	last := m - 1
	if closed {
		last = m
	}
	for i := 0; i < last; i++ {
		a, b := pts[i], pts[(i+1)%m]
		rd.line(a, b, c, in, 4.5)
		rd.line(a, b, core, in*0.7, 1.4)
	}
}

func (rd *Rd) palm(base V3, h uint32, dist float32) {
	p := &rd.sc.P
	hgt := 7 + 4*hf(h, 10)
	th := hf(h, 11) * 2 * math.Pi
	lean := 0.6 + 1.6*hf(h, 12)
	lx, lz := cos32(th)*lean, sin32(th)*lean
	const NT = 6
	var prev V3
	glow := 1 + 0.5*rd.fr.Kick
	for i := 0; i <= NT; i++ {
		t := float32(i) / NT
		q := base.Add(V3{lx * t * t, hgt * t, lz * t * t})
		if i > 0 {
			rd.line(prev, q, p.Acc, 0.75, 3.2-1.6*t)
			rd.line(prev, q, p.SunT, 0.35, 1)
		}
		prev = q
	}
	top := prev
	nf, S := 9, 6
	if dist > 110 {
		nf, S = 6, 3
	}
	sway := 0.25 * sin32(rd.T*1.1+hf(h, 13)*6)
	for k := 0; k < nf; k++ {
		ang := th + float32(k)/float32(nf)*2*math.Pi + hf(h, 20+uint32(k))*0.4
		d := V3{cos32(ang), 0, sin32(ang)}
		perp := V3{-d.Z, 0, d.X}
		L := 3.0 + 1.4*hf(h, 40+uint32(k))
		lift := 0.35 + 0.35*hf(h, 60+uint32(k))
		pq := top
		for s := 1; s <= S; s++ {
			u := float32(s) / float32(S)
			q := top.Add(d.Mul(L * u)).Add(V3{0, L * (lift*u - 0.95*u*u), 0}).Add(perp.Mul(sway * u * u))
			rd.line(pq, q, p.Line, 0.95*glow, 2.6-1.4*u)
			if dist < 110 {
				drop := V3{0, -0.55 * (1 - 0.4*u), 0}
				rd.line(q, q.Add(drop).Add(perp.Mul(0.35)).Add(d.Mul(0.2)), p.Line, 0.5*glow, 1)
				rd.line(q, q.Add(drop).Sub(perp.Mul(0.35)).Add(d.Mul(0.2)), p.Line, 0.5*glow, 1)
			}
			pq = q
		}
	}
}

func (rd *Rd) box(c V3, hx, hy, hz float32, col Col) {
	x0, x1, y0, y1, z0, z1 := c.X-hx, c.X+hx, c.Y-hy, c.Y+hy, c.Z-hz, c.Z+hz
	rd.quad(V3{x0, y0, z0}, V3{x1, y0, z0}, V3{x1, y1, z0}, V3{x0, y1, z0}, col.Mul(shade(V3{0, 0, -1})), 1, 0)
	rd.quad(V3{x0, y0, z1}, V3{x1, y0, z1}, V3{x1, y1, z1}, V3{x0, y1, z1}, col.Mul(shade(V3{0, 0, 1})), 1, 0)
	rd.quad(V3{x0, y0, z0}, V3{x0, y0, z1}, V3{x0, y1, z1}, V3{x0, y1, z0}, col.Mul(shade(V3{-1, 0, 0})), 1, 0)
	rd.quad(V3{x1, y0, z0}, V3{x1, y0, z1}, V3{x1, y1, z1}, V3{x1, y1, z0}, col.Mul(shade(V3{1, 0, 0})), 1, 0)
	rd.quad(V3{x0, y1, z0}, V3{x1, y1, z0}, V3{x1, y1, z1}, V3{x0, y1, z1}, col.Mul(shade(V3{0, 1, 0})), 1, 0)
}

func (rd *Rd) column(base V3, h uint32, dist float32) {
	p := &rd.sc.P
	marble := mixc(Col{0.97, 0.95, 0.98}, p.Hor, 0.22)
	hgt := 6 + 3*hf(h, 10)
	broken := hf(h, 11) < 0.25
	if broken {
		hgt *= 0.3 + 0.4*hf(h, 12)
	}
	const r = 0.55
	rd.box(base.Add(V3{0, 0.2, 0}), 0.9, 0.2, 0.9, marble)
	const N = 8
	for i := 0; i < N; i++ {
		a0 := float32(i) / N * 2 * math.Pi
		a1 := float32(i+1) / N * 2 * math.Pi
		am := (a0 + a1) / 2
		p0 := base.Add(V3{cos32(a0) * r, 0.4, sin32(a0) * r})
		p1 := base.Add(V3{cos32(a1) * r, 0.4, sin32(a1) * r})
		t0, t1 := p0, p1
		t0.Y, t1.Y = hgt, hgt
		if broken {
			t0.Y -= hf(h, 30+uint32(i)) * 0.8
			t1.Y -= hf(h, 30+uint32(i+1)%N) * 0.8
		}
		col := marble.Mul(shade(V3{cos32(am), 0, sin32(am)}))
		rd.quadG(p0, p1, t1, t0, col.Mul(0.85), col.Mul(0.85), col, col, 1, 0)
		if dist < 90 {
			rd.lineO(p0, t0, marble.Mul(0.55), 1)
		}
	}
	if !broken {
		rd.box(base.Add(V3{0, hgt + 0.12, 0}), 0.7, 0.12, 0.7, marble)
		rd.box(base.Add(V3{0, hgt + 0.4, 0}), 0.95, 0.16, 0.95, marble)
	}
}

func rotY(v V3, a float32) V3 {
	s, c := sin32(a), cos32(a)
	return V3{v.X*c + v.Z*s, v.Y, -v.X*s + v.Z*c}
}
func rotX(v V3, a float32) V3 {
	s, c := sin32(a), cos32(a)
	return V3{v.X, v.Y*c - v.Z*s, v.Y*s + v.Z*c}
}

func (rd *Rd) diamond(base V3, h uint32) {
	p := &rd.sc.P
	s := 1.1 + 1.3*hf(h, 10)
	ph := hf(h, 11) * 6.28
	c := base.Add(V3{0, 3 + 4*hf(h, 12) + 0.45*sin32(rd.bt*math.Pi*0.5+ph), 0})
	rot := rd.bt*0.4 + ph
	var eq [4]V3
	for i := range eq {
		eq[i] = c.Add(rotY(V3{s, 0, 0}, rot+float32(i)*math.Pi/2))
	}
	up, dn := c.Add(V3{0, s * 1.5, 0}), c.Add(V3{0, -s * 1.5, 0})
	col := mixc(p.Acc, p.Line, hf(h, 13))
	for i := 0; i < 4; i++ {
		a, b := eq[i], eq[(i+1)%4]
		k := 0.35 + 0.2*float32(i)
		rd.tri(a, b, up, col.Mul(k), col.Mul(k), Col{1, 1, 1}, 0.4, 0)
		rd.tri(a, b, dn, col.Mul(k*0.7), col.Mul(k*0.7), col.Mul(0.2), 0.4, 0)
	}
	in := 1.1 + 0.8*rd.fr.Snare
	for i := 0; i < 4; i++ {
		a, b := eq[i], eq[(i+1)%4]
		rd.line(a, b, col, in, 1.6)
		rd.line(a, up, col, in, 1.6)
		rd.line(a, dn, col, in, 1.6)
	}
}

func (rd *Rd) sphere(base V3, h uint32, dist float32) {
	p := &rd.sc.P
	r := (1.2 + 1.5*hf(h, 10)) * (1 + 0.12*rd.fr.Snare)
	c := base.Add(V3{0, 2.5 + r + 4*hf(h, 12), 0})
	rot := rd.bt*0.25 + hf(h, 11)*6
	col := mixc(p.Line, p.SunT, hf(h, 13))
	seg := 14
	if dist > 100 {
		seg = 8
	}
	pt := func(lat, lon float32) V3 {
		return c.Add(rotY(V3{cos32(lat) * cos32(lon) * r, sin32(lat) * r, cos32(lat) * sin32(lon) * r}, rot))
	}
	for i := 1; i < 6; i++ {
		lat := (float32(i)/6 - 0.5) * math.Pi
		for s := 0; s < seg; s++ {
			rd.line(pt(lat, float32(s)/float32(seg)*2*math.Pi), pt(lat, float32(s+1)/float32(seg)*2*math.Pi), col, 0.8, 1.2)
		}
	}
	for i := 0; i < 8; i++ {
		lon := float32(i) / 8 * 2 * math.Pi
		for s := 0; s < seg/2+1; s++ {
			l0 := (float32(s)/float32(seg/2+1) - 0.5) * math.Pi
			l1 := (float32(s+1)/float32(seg/2+1) - 0.5) * math.Pi
			rd.line(pt(l0, lon), pt(l1, lon), col, 0.8, 1.2)
		}
	}
}

var cubeE = [12][2]int{{0, 1}, {1, 3}, {3, 2}, {2, 0}, {4, 5}, {5, 7}, {7, 6}, {6, 4}, {0, 4}, {1, 5}, {2, 6}, {3, 7}}

func (rd *Rd) cube(base V3, h uint32) {
	p := &rd.sc.P
	s := 0.9 + 1.0*hf(h, 10)
	ph := hf(h, 11) * 6
	c := base.Add(V3{0, 2.5 + s + 4*hf(h, 12), 0})
	for pass := 0; pass < 2; pass++ {
		sz := s
		col := p.Line
		if pass == 1 {
			sz = s * (0.45 + 0.2*rd.fr.Kick)
			col = p.Acc
		}
		var v [8]V3
		for i := range v {
			q := V3{sz, sz, sz}
			if i&1 == 0 {
				q.X = -sz
			}
			if i&2 == 0 {
				q.Y = -sz
			}
			if i&4 == 0 {
				q.Z = -sz
			}
			v[i] = c.Add(rotY(rotX(q, rd.bt*0.3+ph), rd.bt*0.45+ph))
		}
		for _, e := range cubeE {
			rd.line(v[e[0]], v[e[1]], col, 1.1, 1.6)
		}
	}
}

func (rd *Rd) trisign(base V3, h uint32) {
	p := &rd.sc.P
	s := 1.5 + 1.5*hf(h, 10)
	c := base.Add(V3{0, 2 + s + 3*hf(h, 12), 0})
	a := (hf(h, 11) - 0.5) * 0.8
	flick := 0.9 + 0.6*rd.fr.Hat
	for pass := 0; pass < 2; pass++ {
		k := float32(1)
		col := p.Acc
		if pass == 1 {
			k, col = 0.72, p.Line
		}
		var v [3]V3
		for i := range v {
			an := float32(i) / 3 * 2 * math.Pi
			v[i] = c.Add(rotY(V3{sin32(an) * s * k, cos32(an) * s * k, 0}, a))
		}
		for i := range v {
			rd.line(v[i], v[(i+1)%3], col, flick, 2.6-float32(pass))
		}
	}
}

// panel draws a floating Windows-95 style window.
func (rd *Rd) panel(base V3, h uint32) {
	p := &rd.sc.P
	w := 2.2 + 1.2*hf(h, 10)
	ht := w * 0.72
	yaw := (hf(h, 11) - 0.5) * 0.9
	c := base.Add(V3{0, 2.2 + ht + 3.5*hf(h, 12) + 0.25*sin32(rd.bt*math.Pi*0.5+hf(h, 13)*6), 0})
	rx := rotY(V3{1, 0, 0}, yaw)
	up := V3{0, 1, 0}
	// pt maps panel coordinates (u,v in -1..1) to world
	pt := func(u, v float32) V3 { return c.Add(rx.Mul(u * w)).Add(up.Mul(v * ht)) }
	rect := func(u0, v0, u1, v1 float32, c0, c1 Col, bias float32) {
		rd.quadG(pt(u0, v0), pt(u1, v0), pt(u1, v1), pt(u0, v1), c0, c0, c1, c1, 1, bias)
	}
	grey := Col{0.76, 0.76, 0.78}
	bar := mixc(Col{0, 0, 0.5}, p.Acc, 0.45)
	rect(-1, -1, 1, 1, grey, grey, 0)
	rect(-0.95, 0.72, 0.95, 0.93, bar, mixc(bar, p.Line, 0.5), 0.004)
	rect(0.8, 0.76, 0.92, 0.89, grey, grey, 0.008)
	rect(-0.93, -0.55, 0.93, 0.66, p.Hor, p.Mid, 0.004)
	// little sun inside the window
	sc := pt(0, -0.2)
	for i := 0; i < 10; i++ {
		a0 := float32(i) / 10 * math.Pi
		a1 := float32(i+1) / 10 * math.Pi
		q0 := sc.Add(rx.Mul(cos32(a0) * w * 0.3)).Add(up.Mul(sin32(a0)*w*0.3 - ht*0.35))
		q1 := sc.Add(rx.Mul(cos32(a1) * w * 0.3)).Add(up.Mul(sin32(a1)*w*0.3 - ht*0.35))
		rd.tri(sc.Add(up.Mul(-ht*0.35)), q0, q1, p.SunB, p.SunT, p.SunT, 1, 0.008)
	}
	rect(-0.5, -0.9, -0.05, -0.66, grey.Mul(1.1), grey.Mul(1.1), 0.004)
	rect(0.05, -0.9, 0.5, -0.66, grey.Mul(1.1), grey.Mul(1.1), 0.004)
	white, dark := Col{1, 1, 1}, Col{0.25, 0.25, 0.3}
	rd.lineO(pt(-1, 1), pt(1, 1), white, 1.5)
	rd.lineO(pt(-1, -1), pt(-1, 1), white, 1.5)
	rd.lineO(pt(-1, -1), pt(1, -1), dark, 1.5)
	rd.lineO(pt(1, -1), pt(1, 1), dark, 1.5)
	for _, u := range []float32{-0.5, 0.05} {
		rd.lineO(pt(u, -0.9), pt(u+0.45, -0.9), dark, 1)
		rd.lineO(pt(u+0.45, -0.9), pt(u+0.45, -0.66), dark, 1)
	}
}
