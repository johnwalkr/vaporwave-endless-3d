package vaporwave

import "math"

// groundAt is the floor height under a world position.
func (rd *Rd) groundAt(x float32, z float64) float32 {
	if rd.sc.Hills {
		return rd.terrAt(x, z)
	}
	return 0
}

func (rd *Rd) drawGame(g *Game) {
	fr := rd.fr
	p := &rd.sc.P
	// rings
	next := -1
	for i := range g.Rings {
		if g.Rings[i].State == 0 {
			next = i
			break
		}
	}
	for i := range g.Rings {
		r := &g.Rings[i]
		if r.Z < rd.camZ-5 || r.Z > rd.camZ+farDist {
			continue
		}
		c := V3{r.X, rd.groundAt(r.X, r.Z) + r.Y, rd.rz(r.Z)}
		R := float32(ringRadius)
		in := float32(1.0 + 0.6*fr.Kick)
		col := p.Acc
		switch r.State {
		case 1: // hit: burst outward and fade
			a := float32(g.T - r.T)
			if a > 0.35 {
				continue
			}
			R *= 1 + a*0.8
			in *= 1.3 * (1 - a/0.35)
			col = Col{1, 1, 1}
		case 2:
			in *= 0.25
			col = p.Line
		default:
			if i == next {
				in *= 1.4
				col = mixc(p.Acc, Col{1, 1, 1}, 0.25)
			}
		}
		rd.ringShape(c, R, col, in)
	}
	// note crystals left by the player in create mode
	for i := range g.Beacons {
		b := &g.Beacons[i]
		if b.Z < rd.camZ-5 || b.Z > rd.camZ+farDist {
			continue
		}
		c := V3{b.X, rd.groundAt(b.X, b.Z) + b.Y, rd.rz(b.Z)}
		col := hueRot(p.Acc, float32((b.Pitch%12)*30))
		age := float32(g.T - b.T)
		s := 0.3 + 0.15*fr.Melody + 0.35*float32(math.Exp(-float64(age)*3))
		rd.crystal(c, s, col, 1+fr.Melody, age)
	}
	// explosion debris
	for i := range g.Particles {
		q := &g.Particles[i]
		a := V3{q.X, q.Y, rd.rz(q.Z)}
		b := a.Sub(V3{q.VX, q.VY, q.VZ}.Mul(0.06))
		rd.line(a, b, q.C, 1.4, 2)
	}
	// the ship (hidden while it is in pieces)
	if g.Phase == PhaseBoom && g.PhaseT < 2 {
		return
	}
	sz := g.ShipZ()
	pos := V3{g.X, rd.groundAt(g.X, sz) + g.Y, rd.rz(sz)}
	rd.ship(pos, g.VX*0.045, -g.VY*0.035, fr.Kick)
}

func (rd *Rd) ringShape(c V3, R float32, col Col, in float32) {
	const N = 28
	var prev V3
	core := mixc(col, Col{1, 1, 1}, 0.6)
	for i := 0; i <= N; i++ {
		a := float32(i) / N * 2 * math.Pi
		q := c.Add(V3{cos32(a) * R, sin32(a) * R, 0})
		if i > 0 {
			rd.line(prev, q, col, in, 5)
			rd.line(prev, q, core, in*0.7, 1.6)
		}
		prev = q
	}
}

func (rd *Rd) crystal(c V3, s float32, col Col, in, rot float32) {
	var eq [4]V3
	for i := range eq {
		eq[i] = c.Add(rotY(V3{s, 0, 0}, rot*1.5+float32(i)*math.Pi/2))
	}
	up, dn := c.Add(V3{0, s * 1.6, 0}), c.Add(V3{0, -s * 1.6, 0})
	for i := 0; i < 4; i++ {
		a, b := eq[i], eq[(i+1)%4]
		rd.tri(a, b, up, col.Mul(0.5), col.Mul(0.5), Col{1, 1, 1}, 0.5, 0)
		rd.line(a, b, col, in, 1.5)
		rd.line(a, up, col, in, 1.5)
		rd.line(a, dn, col, in, 1.5)
	}
}

// ship draws the player's craft: a low-poly delta-wing glider with a tail fin.
func (rd *Rd) ship(pos V3, roll, pitch, kick float32) {
	p := &rd.sc.P
	tr := func(v V3) V3 { return pos.Add(rotX(rotZ(v.Mul(1.3), roll), pitch)) }
	nose := tr(V3{0, 0.05, 1.7})
	lw := tr(V3{-1.35, -0.12, -0.55})
	rw := tr(V3{1.35, -0.12, -0.55})
	lt := tr(V3{-0.35, 0, -0.75})
	rt := tr(V3{0.35, 0, -0.75})
	fin := tr(V3{0, 0.6, -0.8})
	top := tr(V3{0, 0.22, -0.1})
	belly := tr(V3{0, -0.2, -0.2})

	hull := mixc(Col{0.85, 0.85, 0.95}, p.Top, 0.35)
	dark := hull.Mul(0.45)
	rd.tri(nose, lw, top, hull, dark, hull.Mul(1.1), 1, 0)
	rd.tri(nose, top, rw, hull, hull.Mul(1.1), dark, 1, 0)
	rd.tri(lw, lt, top, dark, hull.Mul(0.7), hull, 1, 0)
	rd.tri(rw, top, rt, dark, hull, hull.Mul(0.7), 1, 0)
	rd.tri(lt, rt, top, hull.Mul(0.6), hull.Mul(0.6), hull, 1, 0)
	rd.tri(nose, rw, belly, dark, dark, dark.Mul(0.6), 1, 0)
	rd.tri(nose, belly, lw, dark, dark.Mul(0.6), dark, 1, 0)
	rd.tri(top, fin, lt, hull, p.Acc, hull.Mul(0.8), 1, 0.002)
	rd.tri(top, rt, fin, hull, hull.Mul(0.8), p.Acc, 1, 0.002)

	glow := 1 + 0.8*kick
	edge := mixc(p.Line, Col{1, 1, 1}, 0.3)
	for _, e := range [][2]V3{{nose, lw}, {nose, rw}, {lw, lt}, {rw, rt}, {lt, rt}, {top, fin}, {nose, top}} {
		rd.line(e[0], e[1], edge, 0.9*glow, 1.6)
	}
	// engine trails
	for _, x := range []float32{-0.28, 0.28} {
		a := tr(V3{x, 0.02, -0.78})
		b := tr(V3{x, 0.02, -2.2 - 1.6*kick})
		rd.line(a, b, p.Acc, 1.4*glow, 4)
		rd.line(a, b, Col{1, 1, 1}, 0.8*glow, 1.4)
	}
}

func rotZ(v V3, a float32) V3 {
	s, c := sin32(a), cos32(a)
	return V3{v.X*c - v.Y*s, v.X*s + v.Y*c, v.Z}
}
