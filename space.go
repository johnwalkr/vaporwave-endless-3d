package vaporwave

import (
	"math"
	"math/rand"
)

const (
	FloorNone = iota + 10
	FloorRock
	FloorClouds
	FloorLava
)

const (
	PropRock = iota + 20
	PropPine
	PropTower
	PropSpike
)

// Body is a moon or planet hanging in the sky.
type Body struct {
	Dir          V3
	R            float32
	A, B         Col
	Bands        float32
	Haze         float32
	LX, LY       float32
	Rx, Rz, cosR float32
}

type Blob struct {
	Dir V3
	C   Col
}

type world struct {
	name, jp        string
	kind            int // 0 surface, 1 deep space, 2 black hole
	top, mid, hor   uint32
	g0, g1          uint32 // ground / cloud colours
	floor           int
	sunR            float32
	sunC            uint32
	stars           int
	fog, amp, rough float32
	props           []int
	dens            float32
	gloss, terrLine float32
	ring            int // 0 none, 1 broad, 2 thin
	bodies          []Body
	line, acc       uint32
	backdrop        int
}

func body(az, el, r float32, a, b uint32, bands, haze float32) Body {
	return Body{Dir: dirFrom(az, el), R: r, A: hexc(a), B: hexc(b), Bands: bands, Haze: haze}
}

var worlds = []world{
	{name: "DEEP SPACE", jp: "深宇宙", kind: 1, top: 0x000003, mid: 0x020108, hor: 0x04020c, floor: FloorNone, stars: 700, fog: 1, props: []int{PropRock}, dens: 0.12, line: 0x9fd0ff, acc: 0xff7ad9},
	{name: "NEBULA DRIFT", jp: "星雲", kind: 1, top: 0x020008, mid: 0x06021a, hor: 0x0a0424, floor: FloorNone, stars: 900, fog: 1, props: []int{PropRock, PropDiamond}, dens: 0.08, line: 0x7af0ff, acc: 0xffb36b},
	{name: "EVENT HORIZON", jp: "事象の地平線", kind: 2, top: 0x000002, mid: 0x020106, hor: 0x03020a, floor: FloorNone, stars: 800, fog: 1, line: 0xffb060, acc: 0x7a5cff},
	{name: "SINGULARITY", jp: "特異点", kind: 2, top: 0x000004, mid: 0x01030a, hor: 0x020510, floor: FloorNone, stars: 800, fog: 1, props: []int{PropRock}, dens: 0.05, line: 0x8ad8ff, acc: 0xff5ab0},

	{name: "MERCURY", jp: "水星", top: 0x000000, mid: 0x020202, hor: 0x060606, g0: 0x7a756f, g1: 0x3a3836, floor: FloorRock, sunR: 0.085, sunC: 0xfff6e0, stars: 500, fog: 1, amp: 6, rough: 0.8, props: []int{PropRock}, dens: 0.3},
	{name: "VENUS", jp: "金星", top: 0x8a5a18, mid: 0xc98a2a, hor: 0xe8b84a, g0: 0x6a4420, g1: 0x2e1c0c, floor: FloorRock, sunR: 0, fog: 2.6, amp: 9, rough: 0.4, props: []int{PropRock}, dens: 0.25},
	{name: "EARTH", jp: "地球", top: 0x1e5fbf, mid: 0x5aa0e8, hor: 0xcfe6ff, g0: 0x4f8d44, g1: 0x2a5a2a, floor: FloorRock, sunR: 0.035, sunC: 0xfffbe8, fog: 1.1, amp: 9, rough: 0.12, props: []int{PropPine}, dens: 0.3,
		bodies: []Body{body(-0.7, 0.42, 0.035, 0xe8e8e0, 0xb0b0b0, 0, 0.55)}},
	{name: "MARS", jp: "火星", top: 0x7a5c44, mid: 0xb98a5e, hor: 0xd9b38a, g0: 0xb05a34, g1: 0x6e3018, floor: FloorRock, sunR: 0.022, sunC: 0xe8f0ff, fog: 1.3, amp: 11, rough: 0.5, props: []int{PropRock}, dens: 0.3,
		bodies: []Body{body(0.8, 0.5, 0.012, 0xc0b0a0, 0x807060, 0, 0.3)}},
	{name: "JUPITER", jp: "木星", top: 0x02030a, mid: 0x1a1830, hor: 0x7a6a70, g0: 0xe3c9a0, g1: 0xa8703f, floor: FloorClouds, sunR: 0.011, sunC: 0xffffff, stars: 350, fog: 1,
		bodies: []Body{body(-0.6, 0.35, 0.05, 0xe8d060, 0xc08030, 5, 0.1), body(0.5, 0.5, 0.03, 0xf0f0f8, 0xb0a090, 3, 0.1), body(0.95, 0.25, 0.04, 0x9a8a7a, 0x5a5048, 4, 0.1)}},
	{name: "SATURN", jp: "土星", top: 0x05060f, mid: 0x2a2840, hor: 0x9a8a68, g0: 0xe8d9a8, g1: 0xc2a060, floor: FloorClouds, sunR: 0.008, sunC: 0xffffff, stars: 300, fog: 1, ring: 1,
		bodies: []Body{body(0.75, 0.3, 0.045, 0xe0a040, 0xb07020, 0, 0.15)}},
	{name: "URANUS", jp: "天王星", top: 0x02080f, mid: 0x103040, hor: 0x6fb0b8, g0: 0xa8e6e6, g1: 0x6fc4cc, floor: FloorClouds, sunR: 0.005, sunC: 0xffffff, stars: 350, fog: 1, ring: 2,
		bodies: []Body{body(-0.5, 0.4, 0.02, 0xc8c8d0, 0x808088, 0, 0.1)}},
	{name: "NEPTUNE", jp: "海王星", top: 0x00020a, mid: 0x0a1440, hor: 0x3050a0, g0: 0x3a5fd0, g1: 0x1e2f8a, floor: FloorClouds, sunR: 0.004, sunC: 0xffffff, stars: 400, fog: 1,
		bodies: []Body{body(0.6, 0.38, 0.04, 0xd8c8c0, 0xa89088, 2, 0.1)}},

	{name: "DUNE SEA KHARIS", jp: "砂の海", top: 0x3a6fb0, mid: 0xe0a060, hor: 0xffd9a0, g0: 0xd9a55a, g1: 0xa8702e, floor: FloorRock, sunR: 0.2, sunC: 0xff7a40, fog: 1.2, amp: 8, rough: 0, props: []int{PropRock}, dens: 0.06},
	{name: "GLACIER VESK", jp: "氷河の星", top: 0x4a7ab8, mid: 0xa8c8e8, hor: 0xeef6ff, g0: 0xdce8f2, g1: 0x8aa8c8, floor: FloorRock, sunR: 0.03, sunC: 0xffffff, fog: 1.7, amp: 10, rough: 0.6, props: []int{PropSpike}, dens: 0.3, gloss: 0.3,
		bodies: []Body{body(0.6, 0.4, 0.06, 0xb8d0e8, 0x7a98c0, 6, 0.6)}},
	{name: "EMBERFALL", jp: "溶岩の星", top: 0x100000, mid: 0x4a0a00, hor: 0xc83a0a, g0: 0x2a1410, g1: 0x0c0606, floor: FloorLava, sunR: 0, fog: 1.5, amp: 12, rough: 0.8, props: []int{PropSpike, PropRock}, dens: 0.25, terrLine: 0.5, line: 0xff5a10},
	{name: "THALASSA DEEP", jp: "海の星", top: 0x0a2a5a, mid: 0x2a7ab0, hor: 0xa0e0e8, g0: 0x0a3050, g1: 0x0a3050, floor: FloorWater, sunR: 0.05, sunC: 0xfff0d0, fog: 1, props: []int{PropRock}, dens: 0.05, gloss: 0.8,
		bodies: []Body{body(-0.5, 0.45, 0.07, 0xe8e0f0, 0xa8a0c0, 0, 0.5), body(-0.25, 0.3, 0.03, 0xf0c8a0, 0xc09060, 0, 0.5)}},
	{name: "VIRIDIA", jp: "森の月", top: 0x1a5a7a, mid: 0x7ab890, hor: 0xe0f0c0, g0: 0x2f5a2a, g1: 0x183818, floor: FloorRock, sunR: 0.03, sunC: 0xfff8d8, fog: 1.4, amp: 7, rough: 0.2, props: []int{PropPine}, dens: 0.75,
		bodies: []Body{body(0.55, 0.42, 0.28, 0xe8b890, 0xb07050, 9, 0.6)}},
	{name: "METROPOLIS PRIME", jp: "都市惑星", top: 0x0a0820, mid: 0x3a2a60, hor: 0xff9a6a, g0: 0x0c0a18, g1: 0x0c0a18, floor: FloorGrid, sunR: 0.06, sunC: 0xffd0a0, stars: 80, fog: 1, props: []int{PropTower}, dens: 0.7, gloss: 0.3, line: 0xffb040, acc: 0x40d0ff, backdrop: 2},
	{name: "CRYSTAL EXPANSE", jp: "水晶の原", top: 0x10002a, mid: 0x4a1a8a, hor: 0xc86aff, g0: 0x2a1a4a, g1: 0x120a2a, floor: FloorRock, sunR: 0.025, sunC: 0xd0f0ff, stars: 200, fog: 1.1, amp: 8, rough: 0.9, props: []int{PropDiamond, PropSpike}, dens: 0.45, gloss: 0.3, terrLine: 0.5, line: 0xc8a0ff, acc: 0x60ffe0},
	{name: "NIMBUS", jp: "雲の星", top: 0x3a4a9a, mid: 0xe88ab0, hor: 0xffe0b0, g0: 0xffd0b0, g1: 0xe88a8a, floor: FloorClouds, sunR: 0.06, sunC: 0xfff0c0, fog: 1, props: []int{PropRock}, dens: 0.08,
		bodies: []Body{body(-0.6, 0.4, 0.05, 0xf0e0f0, 0xc0a0c0, 0, 0.6)}},
}

func NumWorlds() int { return len(worlds) }

func NewSpaceScene(seed uint32, idx int) *Scene {
	r := rand.New(rand.NewSource(int64(seed)))
	rf := func() float32 { return float32(r.Float64()) }
	w := worlds[idx%len(worlds)]
	sc := &Scene{Seed: seed, Space: true, Kind: w.kind, Fog: w.fog, G: 4, U: 10, LineHalf: 0.04,
		ColSp: 10, Valley: 7, CamH: 2.4, Avenue: -1, GateBeats: 4, Density: w.dens, SunKind: 1,
		Name: w.name, NameJP: w.jp, FloorType: w.floor, Gloss: w.gloss, Amp: w.amp, Rough: w.rough,
		Outer: w.props, Backdrop: w.backdrop}
	sc.Song = NewSong(seed)
	p := Palette{Name: "SPACE", Top: hexc(w.top), Mid: hexc(w.mid), Hor: hexc(w.hor),
		SunT: hexc(w.sunC), SunB: hexc(w.sunC), Line: hexc(0xcfe8ff), Acc: hexc(0xffc890),
		Floor: hexc(w.g0), Tile: hexc(w.g1), Dark: true, Bloom: 0.9}
	if w.line != 0 {
		p.Line = hexc(w.line)
	}
	if w.acc != 0 {
		p.Acc = hexc(w.acc)
	}
	sc.P = p
	sc.TerrC0, sc.TerrC1, sc.TerrLine = p.Floor, p.Tile, w.terrLine
	sc.Hills = w.amp > 0
	sc.FullSky = w.floor == FloorNone
	sc.BH = w.kind == 2
	if w.floor == FloorClouds {
		sc.CamH = 7
	}
	if w.sunR == 0 {
		sc.SunKind = 2
	}
	sc.SunR = w.sunR
	sc.SunDir = dirFrom((rf()-0.5)*0.6, 0.12+rf()*0.2)
	if w.sunR > 0.15 {
		sc.SunDir = dirFrom((rf()-0.5)*0.3, 0.08)
	}
	sc.BackH = make([]float32, 240)
	if sc.Backdrop == 2 {
		for i := 0; i < len(sc.BackH); {
			n := 1 + r.Intn(3)
			h := 0.04 + rf()*rf()*0.3
			for j := 0; j < n && i < len(sc.BackH); j++ {
				sc.BackH[i] = h
				i++
			}
		}
	}
	for i := 0; i < w.stars; i++ {
		el := 0.03 + rf()*rf()*0.9
		if sc.FullSky {
			el = rf()*1.9 - 0.95
		}
		sc.Stars = append(sc.Stars, Star{dirFrom(rf()*2*math.Pi, el), 0.3 + rf()*rf()*1.2, rf() * 6.28})
	}
	for _, b := range w.bodies {
		l := sc.SunDir.Sub(b.Dir)
		n := sqrt32(l.X*l.X+l.Y*l.Y) + 1e-4
		b.LX, b.LY = l.X/n, l.Y/n
		h := sqrt32(b.Dir.X*b.Dir.X+b.Dir.Z*b.Dir.Z) + 1e-4
		b.Rx, b.Rz = b.Dir.Z/h, -b.Dir.X/h
		b.cosR = cos32(b.R*1.05 + 0.01)
		sc.Bodies = append(sc.Bodies, b)
	}
	switch w.ring {
	case 1:
		sc.RingN, sc.RingW, sc.RingC = V3{0.2, 0.75, -0.63}.Norm(), 0.2, hexc(0xf0e0b8)
	case 2:
		sc.RingN, sc.RingW, sc.RingC = V3{0.95, 0.1, -0.3}.Norm(), 0.035, hexc(0xa0d8e0)
	}
	if sc.FullSky {
		cols := []uint32{0x7a2a9a, 0x1a4aa8, 0xa83a5a, 0x1a8a8a, 0xa86a2a, 0x4a2ac8}
		n := 3 + r.Intn(3)
		if idx == 1 {
			n = 7
		}
		for i := 0; i < n; i++ {
			sc.Nebula = append(sc.Nebula, Blob{dirFrom(rf()*2*math.Pi, rf()*1.4-0.7), hexc(cols[r.Intn(len(cols))])})
		}
		sc.GalN = V3{rf() - 0.5, 1, rf() - 0.5}.Norm()
	}
	return sc
}

// --- extra sky / floor shading for space scenes -----------------------------

func (rd *Rd) spaceSky(c Col, dx, dy, dz float32) Col {
	sc := rd.sc
	for i := range sc.Nebula {
		b := &sc.Nebula[i]
		v := dx*b.Dir.X + dy*b.Dir.Y + dz*b.Dir.Z
		if v > 0.45 {
			t := (v - 0.45) * (1 / 0.55)
			t *= t
			m := 0.65 + 0.35*fsin(dx*11+dz*5+float32(i))*fsin(dy*9-dz*7)
			c = c.Add(b.C.Mul(t * t * m * 0.55))
		}
	}
	if sc.FullSky {
		s := abs32(dx*sc.GalN.X + dy*sc.GalN.Y + dz*sc.GalN.Z)
		if g := 1 - s*4; g > 0 {
			c = c.Add(Col{0.10, 0.09, 0.14}.Mul(g * g * (0.7 + 0.3*fsin(dx*17+dz*13))))
		}
	}
	for i := range sc.Bodies {
		b := &sc.Bodies[i]
		dot := dx*b.Dir.X + dy*b.Dir.Y + dz*b.Dir.Z
		if dot < b.cosR {
			continue
		}
		rr := sqrt32(2 * (1 - dot))
		a := clamp01((b.R-rr)/rd.pxAng + 0.5)
		if a <= 0 {
			continue
		}
		v := (dy - b.Dir.Y) / b.R
		u := ((dx-b.Dir.X)*b.Rx + (dz-b.Dir.Z)*b.Rz) / b.R
		col := b.A
		if b.Bands > 0 {
			col = mixc(b.A, b.B, 0.5+0.5*fsin(v*b.Bands+fsin(u*3)*0.5))
		}
		q := rr / b.R
		sh := clampf(0.55+0.7*(u*b.LX+v*b.LY), 0.2, 1) * (1 - 0.35*q*q)
		c = mixc(c, mixc(col.Mul(sh), c, b.Haze), a)
	}
	if sc.RingW > 0 && dy > 0 {
		s := abs32(dx*sc.RingN.X+dy*sc.RingN.Y+dz*sc.RingN.Z) / sc.RingW
		if s < 1 {
			b := (0.55 + 0.45*fsin(s*46)) * (1 - s*s)
			if s > 0.52 && s < 0.6 {
				b *= 0.15
			}
			c = c.Add(sc.RingC.Mul(b * 0.55 * clamp01(dy*6)))
		}
	}
	if sc.BH {
		R := rd.bhR
		u, v := dx/R, (dy-0.03)/R
		if dz > -0.2 {
			rr := sqrt32(u*u + v*v)
			hot := mixc(sc.P.Line, Col{1, 1, 1}, 0.35)
			dop := 1 + 0.55*clampf(-u/(rr+0.01), -1, 1)
			if rr > 1 {
				k := 1 / (1 + (rr-1)*7)
				c = c.Add(hot.Mul(k * k * 1.7 * dop * (0.8 + 0.2*fsin(rr*30-rd.T*4))))
			} else {
				c = c.Mul(clamp01((rr - 0.97) * 30))
			}
			if au := abs32(u); au < 3.4 {
				bow := -0.14 * sqrt32(1-(au/3.4)*(au/3.4))
				th := 0.05 + 0.03*au
				if w := 1 - abs32(v-bow)/th; w > 0 {
					in := (1 - (au/3.4)*(au/3.4)) * (0.75 + 0.25*fsin(u*13-rd.T*3)) * dop
					c = c.Add(mixc(sc.P.Line, Col{1, 1, 1}, 0.5).Mul(w * w * in * 2.2))
				}
			}
		}
		if rd.tun > 0 {
			rp := sqrt32(dx*dx + dy*dy)
			band := 0.5 + 0.5*fsin((0.9/(rp+0.05)+rd.bt*1.5)*6.283)
			t := mixc(sc.P.Acc, sc.P.Line, band).Mul(clamp01(rp*2.2) * 0.75 * (0.35 + 0.65*band*band))
			c = mixc(c, t, rd.tun)
		}
	}
	return c
}

// spaceFloor shades rock, cloud-deck and lava surfaces.
func (rd *Rd) spaceFloor(hx, hz, fpz float32) Col {
	sc := rd.sc
	p := &sc.P
	fade := 1 / (1 + fpz*0.8)
	switch sc.FloorType {
	case FloorClouds:
		z := hz + rd.ph1*0 + rd.T*0.6
		n := fsin(hx*0.045+fsin(z*0.03+1.7)*2.2) * fsin(z*0.05+fsin(hx*0.035)*2.0)
		n2 := fsin(hx*0.21+fsin(z*0.17)*1.5) * fsin(z*0.19+hx*0.05)
		return mixc(p.Tile, p.Floor, clamp01(0.55+0.4*n+0.18*n2*fade))
	case FloorLava:
		n := fsin(hx*0.31+fsin(hz*0.23+rd.ph2)*2.1) * fsin(hz*0.27+rd.ph3+fsin(hx*0.19)*2.3)
		g := clamp01(1 - abs32(n)*7)
		g *= g
		rock := mixc(p.Tile, p.Floor, 0.5+0.5*fsin(hx*1.7+hz*1.3)*fade)
		return rock.Add(Col{1.9, 0.55, 0.08}.Mul(g * (0.8 + 0.6*rd.fr.Kick)))
	}
	n := fsin(hx*0.37+fsin(hz*0.29+rd.ph1)*1.9) * fsin(hz*0.41+rd.ph2+fsin(hx*0.23)*1.7)
	n2 := fsin(hx*2.9+fsin(hz*2.3+rd.ph3)*1.3) * fsin(hz*3.1+hx*0.7+rd.ph2)
	return mixc(p.Tile, p.Floor, clamp01(0.6+0.3*n+0.22*n2*fade))
}

// terrAt samples terrain height at a world position.
func (rd *Rd) terrAt(x float32, zw float64) float32 {
	gx := x / terrG
	gz := zw / terrG
	i := ifloor(gx)
	j := int64(math.Floor(gz))
	fx, fz := gx-float32(i), float32(gz-float64(j))
	a, b := rd.terrH(i, j), rd.terrH(i+1, j)
	c, d := rd.terrH(i, j+1), rd.terrH(i+1, j+1)
	return (a+(b-a)*fx)*(1-fz) + (c+(d-c)*fx)*fz
}

// warp draws streaking stars rushing past the camera.
func (rd *Rd) warp() {
	sc := rd.sc
	const step = 2.0
	n0 := int64(math.Floor(rd.camZ / step))
	streak := float32(1.2 + 3*rd.fr.Kick)
	for n := n0; n < n0+110; n++ {
		for k := uint32(0); k < 4; k++ {
			h := hashu(sc.Seed, uint32(n), k+77)
			x := (hf(h, 1) - 0.5) * 90
			y := (hf(h, 2) - 0.5) * 70
			if x*x+y*y < 6 {
				continue
			}
			z := rd.rz(float64(n)*step) + hf(h, 3)*step
			a := V3{x + rd.cam.Pos.X*0.0, y + rd.cam.Pos.Y, z}
			if sc.BH {
				d := a.Sub(rd.cam.Pos).Norm()
				if d.X*d.X+(d.Y-0.03)*(d.Y-0.03) < rd.bhR*rd.bhR*1.1 {
					continue
				}
			}
			col := mixc(Col{1, 1, 1}, sc.P.Line, hf(h, 4))
			rd.line(a, a.Add(V3{0, 0, streak}), col, 0.9+0.6*hf(h, 5), 1.5)
		}
	}
}

// --- props --------------------------------------------------------------------

func (rd *Rd) sunShade(n V3) float32 {
	d := n.Dot(rd.sc.SunDir)*0.6 + n.Y*0.3
	return clampf(0.45+0.6*d, 0.18, 1.1)
}

func (rd *Rd) rock(base V3, h uint32) {
	sc := rd.sc
	s := 0.5 + 1.6*hf(h, 10)*hf(h, 11)
	c := base.Add(V3{0, s * 0.35, 0})
	if sc.FloorType == FloorNone || sc.FloorType == FloorClouds {
		s *= 2.5
		c = base.Add(V3{0, (hf(h, 12) - 0.5) * 44, 0})
		if sc.FloorType == FloorClouds {
			c.Y = 8 + hf(h, 12)*20
		}
	}
	rot := hf(h, 13)*6 + rd.bt*0.05*(hf(h, 14)-0.5)*4
	var v [6]V3
	dirs := [6]V3{{1, 0, 0}, {0, 0, 1}, {-1, 0, 0}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}}
	for i := range v {
		v[i] = c.Add(rotY(dirs[i].Mul(s*(0.6+0.7*hf(h, 20+uint32(i)))), rot))
	}
	col := mixc(sc.P.Tile, sc.P.Floor, hf(h, 15))
	if sc.FloorType == FloorNone || sc.FloorType == FloorClouds {
		col = mixc(Col{0.42, 0.38, 0.36}, Col{0.25, 0.24, 0.28}, hf(h, 15))
	}
	for i := 0; i < 4; i++ {
		a, b := v[i], v[(i+1)%4]
		for _, t := range []V3{v[4], v[5]} {
			n := b.Sub(a).Cross(t.Sub(a)).Norm()
			if n.Dot(a.Sub(c)) < 0 {
				n = n.Mul(-1)
			}
			cc := col.Mul(rd.sunShade(n))
			rd.tri(a, b, t, cc, cc, cc, 1, 0)
		}
	}
}

func (rd *Rd) cone(c V3, r, h float32, col Col, sides int) {
	top := c.Add(V3{0, h, 0})
	for i := 0; i < sides; i++ {
		a0 := float32(i) / float32(sides) * 2 * math.Pi
		a1 := float32(i+1) / float32(sides) * 2 * math.Pi
		am := (a0 + a1) / 2
		cc := col.Mul(rd.sunShade(V3{cos32(am), 0.4, sin32(am)}.Norm()))
		rd.tri(c.Add(V3{cos32(a0) * r, 0, sin32(a0) * r}), c.Add(V3{cos32(a1) * r, 0, sin32(a1) * r}), top, cc, cc, cc.Mul(1.15), 1, 0)
	}
}

func (rd *Rd) pine(base V3, h uint32) {
	s := 0.8 + 0.9*hf(h, 10)
	rd.cone(base, 0.22*s, 2*s, Col{0.3, 0.2, 0.12}, 5)
	g := mixc(Col{0.1, 0.36, 0.16}, Col{0.2, 0.5, 0.2}, hf(h, 11))
	for i := 0; i < 3; i++ {
		f := float32(i)
		rd.cone(base.Add(V3{0, (1.2 + f*1.3) * s, 0}), (1.7-f*0.42)*s, 2.3*s, g.Mul(1+f*0.08), 6)
	}
}

func (rd *Rd) spike(base V3, h uint32) {
	p := &rd.sc.P
	s := 0.5 + 1.2*hf(h, 10)
	col := mixc(p.Floor, Col{1, 1, 1}, 0.35)
	top := base.Add(V3{(hf(h, 11) - 0.5) * 2, 3 + 7*hf(h, 12), (hf(h, 13) - 0.5) * 2})
	rot := hf(h, 14) * 6
	var q [4]V3
	for i := range q {
		q[i] = base.Add(rotY(V3{s, -0.3, 0}, rot+float32(i)*math.Pi/2))
	}
	for i := range q {
		a, b := q[i], q[(i+1)%4]
		n := b.Sub(a).Cross(top.Sub(a)).Norm()
		if n.Dot(a.Sub(base)) < 0 {
			n = n.Mul(-1)
		}
		cc := col.Mul(rd.sunShade(n))
		rd.tri(a, b, top, cc, cc, cc.Mul(1.3), 1, 0)
		if rd.sc.TerrLine > 0 {
			rd.line(a, top, p.Line, 0.5, 1.2)
		}
	}
}

func (rd *Rd) tower(base V3, h uint32, dist float32) {
	p := &rd.sc.P
	w := 1.2 + 1.8*hf(h, 10)
	ht := 6 + 34*hf(h, 11)*hf(h, 12)
	col := mixc(Col{0.10, 0.10, 0.16}, p.Mid, 0.25)
	rd.box(base.Add(V3{0, ht / 2, 0}), w, ht/2, w, col)
	if dist > 150 {
		return
	}
	rows := int(ht / 1.6)
	for r := 1; r < rows; r++ {
		hh := hashu(h, uint32(r), 3)
		if hh&1 == 0 {
			continue
		}
		c := p.Line
		if hh&2 != 0 {
			c = p.Acc
		}
		y := float32(r) * 1.6
		x0 := -w + 2*w*float32(hh>>4&3)*0.2
		rd.line(base.Add(V3{x0, y, -w - 0.02}), base.Add(V3{x0 + w*0.9, y, -w - 0.02}), c, 0.8, 1.6)
	}
	if hf(h, 13) < 0.3 {
		rd.line(base.Add(V3{0, ht, 0}), base.Add(V3{0, ht + 4, 0}), p.Acc, 0.6+rd.fr.Hat, 1.2)
	}
}
