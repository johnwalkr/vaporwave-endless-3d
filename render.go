package main

import "math"

const farDist = 260

type ringC struct{ x, z, r, w, amp float32 }

// Rd renders a Scene + Frame into pixels.
type Rd struct {
	FX    *FX // post-processing settings; nil = defaults
	fb    FB
	cam   Cam
	sc    *Scene
	fr    *Frame
	zBase float64 // world z of render-space origin (keeps float32 precise forever)
	camZ  float64
	noFog bool

	invFar, invf    float32
	fogCol, glowCol Col
	glowAmt         float32
	sunR, sunCos    float32
	pxAng, scroll   float32
	moon2           V3
	rings           []ringC
	wave            float32
	T, bt           float32
	ph1, ph2, ph3   float32
	zCell           int32
	ws              float32
	halfBeat        uint32
	halfFrac        float32
	bhR, tun        float32
}

func (rd *Rd) rz(worldZ float64) float32 { return float32(worldZ - rd.zBase) }

// Effects returns the post-processing settings (defaults when unset).
func (rd *Rd) Effects() FX {
	if rd.FX == nil {
		return DefaultFX
	}
	return *rd.FX
}

func (rd *Rd) Render(sc *Scene, fr *Frame, W, H int, out []uint32) {
	rd.setup(sc, fr, W, H, false)
	parallel(H, 8, rd.background)
	rd.geometry()
	rd.fb.flush()
	fx := rd.Effects()
	rd.fb.post(out, sc.P.Bloom*fx.Bloom, fr.Glitch, fr.N, fr.Flash, fx)
}

// Prepare sets up camera and per-frame values and builds the GPU display
// list (sky, floor and post-processing are then done in shaders).
func (rd *Rd) Prepare(sc *Scene, fr *Frame, W, H int) {
	rd.setup(sc, fr, W, H, true)
	rd.geometry()
}

func (rd *Rd) geometry() {
	sc := rd.sc
	rd.stars()
	rd.backdrop()
	if sc.FloorType == FloorValley || sc.Hills {
		rd.terrain()
	}
	if sc.FullSky {
		rd.warp()
	}
	rd.props()
	if rd.fr.Game != nil {
		rd.drawGame(rd.fr.Game)
	}
}

func (rd *Rd) setup(sc *Scene, fr *Frame, W, H int, gpu bool) {
	if gpu {
		rd.fb.SetGPU(W, H)
	} else {
		rd.fb.Resize(W, H)
	}
	rd.sc, rd.fr = sc, fr
	rd.camZ = fr.Beat * float64(sc.U)
	if fr.UseCamZ {
		rd.camZ = fr.CamZ
	}
	period := float64(sc.G) * 128
	rd.zBase = math.Floor(rd.camZ/period) * period
	rd.zCell = int32(int64(rd.zBase/float64(sc.G)) & 0xFFFFFF)
	twoPi := 2 * math.Pi
	rd.ph1 = float32(math.Mod(rd.zBase*0.35, twoPi))
	rd.ph2 = float32(math.Mod(rd.zBase*1.7, twoPi))
	rd.ph3 = float32(math.Mod(rd.zBase*2.3, twoPi))
	rd.T = float32(math.Mod(fr.Time, 4096))
	rd.bt = float32(math.Mod(fr.Beat, 4096))
	rd.ws = float32(H) / 600
	bob := 0.05 * sin32(rd.bt*math.Pi)
	fov := float32(1.22) - 0.02*fr.Kick
	camY := fr.H + bob
	if sc.Hills {
		camY += rd.terrAt(fr.X, rd.camZ)
	}
	rd.bhR, rd.tun = 1, 0
	if sc.BH {
		ph := float32(math.Mod(math.Max(fr.Beat, 0), 32) / 32)
		rd.bhR = 0.035 * float32(math.Exp(float64(ph)*5.2))
		if ph < 0.22 && fr.Beat > 4 {
			rd.tun = 1 - ph/0.22
		}
	}
	rd.cam.Set(V3{fr.X, camY, rd.rz(rd.camZ)}, fr.Yaw, fr.Pitch_, fr.Roll, fov, W, H)
	rd.invFar = 1.0 / farDist
	rd.invf = 1 / rd.cam.f
	rd.pxAng = rd.invf

	p := &sc.P
	tints := [4]Col{p.Acc, p.SunB, p.Line, p.SunT}
	n := len(sc.Song.Prog)
	prev := tints[(fr.Chord+n-1)%n%4]
	rd.glowCol = mixc(p.Hor, mixc(prev, tints[fr.Chord%4], smooth(fr.ChordT)), 0.4)
	rd.glowAmt = 0.25 + 0.3*fr.Kick
	if sc.Space {
		rd.glowAmt = 0.06 + 0.05*fr.Kick
		rd.glowCol = p.Hor
	} else if !p.Dark {
		rd.glowAmt = 0.08 + 0.12*fr.Kick
	}
	rd.fogCol = p.Hor.Add(rd.glowCol.Mul(rd.glowAmt))
	rd.sunR = sc.SunR * (1 + 0.035*fr.Kick)
	rd.sunCos = cos32(rd.sunR * 3.3)
	rd.scroll = float32(math.Mod(fr.Beat*0.125, 1))
	rd.moon2 = sc.MoonDir.Add(V3{0.55 * sc.MoonR, 0.25 * sc.MoonR, 0}).Norm()
	bf := math.Max(fr.Beat, 0)
	rd.wave = float32(bf-math.Floor(bf)) * 90
	hb := bf * 2
	rd.halfBeat = uint32(int64(hb))
	rd.halfFrac = float32(hb - math.Floor(hb))
	rd.rings = rd.rings[:0]
	for _, r := range fr.Rings {
		age := float32(fr.Time - r.T0)
		if age < 0 || age > 2.5 {
			continue
		}
		rd.rings = append(rd.rings, ringC{r.X, rd.rz(r.Z), age * 45, 2 + age*2.5, 1 - age/2.5})
	}

}

// Pick returns the floor point under a pixel (or a point ahead of the camera).
func (rd *Rd) Pick(mx, my float32) (float32, float64) {
	c := &rd.cam
	if c.f == 0 {
		return 0, rd.camZ + 15
	}
	d := c.F.Add(c.R.Mul((mx - c.cx) / c.f)).Add(c.U.Mul((c.cy - my) / c.f))
	t := float32(40)
	if d.Y < -0.01 {
		t = c.Pos.Y / -d.Y
		if t > 120 {
			t = 120
		}
	}
	return c.Pos.X + d.X*t, rd.zBase + float64(c.Pos.Z+d.Z*t)
}

func (rd *Rd) background(y0, y1 int) {
	fb, cam := &rd.fb, &rd.cam
	W := fb.W
	invf := rd.invf
	sx, sy, sz := cam.R.X*invf, cam.R.Y*invf, cam.R.Z*invf
	for y := y0; y < y1; y++ {
		b := (cam.cy - float32(y)) * invf
		a0 := -cam.cx * invf
		dx := cam.F.X + cam.U.X*b + cam.R.X*a0
		dy := cam.F.Y + cam.U.Y*b + cam.R.Y*a0
		dz := cam.F.Z + cam.U.Z*b + cam.R.Z*a0
		idx := y * W
		for x := 0; x < W; x++ {
			var c Col
			if dy >= 0 || rd.sc.FullSky {
				inv := 1 / sqrt32(dx*dx+dy*dy+dz*dz)
				c = rd.sky(dx*inv, dy*inv, dz*inv)
			} else {
				c = rd.floor(dx, dy, dz)
			}
			fb.C[idx*3], fb.C[idx*3+1], fb.C[idx*3+2] = c.R, c.G, c.B
			fb.Z[idx] = 0
			idx++
			dx += sx
			dy += sy
			dz += sz
		}
	}
}

func (rd *Rd) sky(dx, dy, dz float32) Col {
	sc := rd.sc
	p := &sc.P
	e := dy
	if e < 0 {
		e = 0
		if sc.FullSky {
			e = -dy
		}
	}
	var c Col
	const midE = 0.17
	if e < midE {
		c = mixc(p.Hor, p.Mid, e*(1/midE))
	} else {
		t := (e - midE) * (1 / (1 - midE))
		c = mixc(p.Mid, p.Top, t*(2-t))
	}
	if sc.Space {
		c = rd.spaceSky(c, dx, dy, dz)
	}
	if g := 1 - e*4.5; g > 0 && !sc.FullSky {
		c = c.Add(rd.glowCol.Mul(g * g * g * rd.glowAmt))
	}
	if sc.SunKind != 2 {
		s := &sc.SunDir
		dot := dx*s.X + dy*s.Y + dz*s.Z
		if dot > rd.sunCos {
			R := rd.sunR
			rr := sqrt32(2 * (1 - dot))
			if h := 1 - (rr-R)/(2.3*R); h > 0 {
				if h > 1 {
					h = 1
				}
				c = c.Add(mixc(p.SunB, p.SunT, 0.4).Mul(h * h * (0.2 + 0.2*rd.fr.Kick) * rd.glowAmt * 3))
			}
			if rr < R+rd.pxAng {
				v := (dy - s.Y) / R
				a := clamp01((R-rr)/rd.pxAng + 0.5)
				if sc.SunKind == 0 && v < 0.3 {
					q := (0.3 - v) / 1.3
					ph := (v+2)*sc.Stripes*0.5 + rd.scroll
					fr := ph - floor32(ph)
					gap := 0.1 + 0.62*q
					w := rd.pxAng / R * sc.Stripes * 0.5
					a *= clamp01((fr-gap)/w+0.5) * clamp01((1-fr)/w+0.5)
				}
				sc2 := mixc(p.SunB, p.SunT, clamp01(v*0.5+0.5)).Mul(1.2)
				c = mixc(c, sc2, a)
			}
		}
	}
	if sc.MoonKind != 0 {
		m := &sc.MoonDir
		dot := dx*m.X + dy*m.Y + dz*m.Z
		if dot > 0.98 {
			rr := sqrt32(2 * (1 - dot))
			if rr < sc.MoonR+rd.pxAng {
				a := clamp01((sc.MoonR-rr)/rd.pxAng + 0.5)
				if sc.MoonKind == 1 {
					d2 := dx*rd.moon2.X + dy*rd.moon2.Y + dz*rd.moon2.Z
					r2 := sqrt32(2 * (1 - d2))
					a *= 1 - clamp01((sc.MoonR*0.92-r2)/rd.pxAng+0.5)
				}
				c = mixc(c, mixc(p.SunT, Col{1, 1, 1}, 0.6).Mul(1.15), a)
			}
		}
	}
	return c
}

func gridCov(h, invG, G, lineHalf, fp float32) float32 {
	g := h * invG
	d := abs32(g-floor32(g+0.5)) * G
	w := lineHalf
	if m := 0.6 * fp; m > w {
		w = m
	}
	cov := clamp01((w + 0.5*fp - d) / fp)
	k := clamp01(1.5 - 1.5*fp*invG)
	return cov*k + 0.3*(1-k)
}

func (rd *Rd) floor(dx, dy, dz float32) Col {
	sc, cam := rd.sc, &rd.cam
	p := &sc.P
	t := cam.Pos.Y / -dy
	fog := t * rd.invFar * sc.Fog
	if fog >= 1 {
		return rd.fogCol
	}
	fog *= fog
	hx := cam.Pos.X + dx*t
	hz := cam.Pos.Z + dz*t
	invf := rd.invf
	k := t / dy
	tpx := -k * cam.R.Y * invf
	tpy := k * cam.U.Y * invf
	fpx := abs32(cam.R.X*invf*t+dx*tpx) + abs32(-cam.U.X*invf*t+dx*tpy) + 1e-6
	fpz := abs32(cam.R.Z*invf*t+dz*tpx) + abs32(-cam.U.Z*invf*t+dz*tpy) + 1e-6
	inv := 1 / sqrt32(dx*dx+dy*dy+dz*dz)
	ny := -dy * inv
	G := sc.G
	invG := 1 / G
	var c Col
	switch sc.FloorType {
	case FloorRock, FloorClouds, FloorLava:
		c = rd.spaceFloor(hx, hz, fpz)
		if sc.Gloss > 0 {
			f := 1 - ny
			c = c.Add(rd.sky(dx*inv, ny, dz*inv).Mul(sc.Gloss * f * f))
		}
	case FloorChecker:
		ux := hx * invG * 0.5
		uz := hz * invG * 0.5
		tx := abs32(ux-floor32(ux)-0.5)*4 - 1
		tz := abs32(uz-floor32(uz)-0.5)*4 - 1
		s := clampf(tx*G*0.5/fpx, -1, 1) * clampf(tz*G*0.5/fpz, -1, 1)
		c = mixc(p.Floor, p.Tile.Mul(0.8), 0.5+0.5*s)
		if fpz < G {
			ix := ifloor(hx*invG + 0.5)
			iz := ifloor(hz*invG+0.5) + rd.zCell
			if hashu(uint32(ix), uint32(iz), rd.halfBeat+sc.Seed)&31 == 0 {
				c = c.Add(p.Acc.Mul(abs32(s) * (1 - rd.halfFrac) * (1 - fpz*invG) * 0.9))
			}
		}
	case FloorWater:
		att := 1 / (1 + fpz*1.2)
		w1 := fsin(hx*0.9 + hz*0.35 + rd.T*1.3 + rd.ph1)
		w2 := fsin(hz*1.7 - rd.T*1.9 + hx*0.2 + rd.ph2)
		w3 := fsin((hx+hz)*2.3 + rd.T*2.4 + rd.ph3)
		nx := (w1*0.04 + w3*0.02) * att
		nz := (w2*0.06 + w3*0.02) * att
		rx, ry, rzz := dx*inv+nx, ny+nz*0.6, dz*inv+nz*0.3
		if ry < 0.004 {
			ry = 0.004
		}
		ri := 1 / sqrt32(rx*rx+ry*ry+rzz*rzz)
		f := 1 - ny
		f = 0.3 + 0.7*f*f*f
		c = p.Floor.Mul(0.55).Add(rd.sky(rx*ri, ry*ri, rzz*ri).Mul(f * sc.Gloss))
		g := gridCov(hx, invG*0.25, G*4, sc.LineHalf, fpx)
		if gz := gridCov(hz, invG*0.25, G*4, sc.LineHalf, fpz); gz > g {
			g = gz
		}
		c = c.Add(p.Line.Mul(g * 0.3))
	default:
		gx := gridCov(hx, invG, G, sc.LineHalf, fpx)
		gz := gridCov(hz, invG, G, sc.LineHalf, fpz)
		boost := 1 + 0.9*rd.fr.Kick
		if wd := abs32(hz-cam.Pos.Z-rd.wave) * (1.0 / 7); wd < 1 {
			boost += (1 - wd) * 0.9
		}
		gz *= boost
		if gz > gx {
			gx = gz
		}
		c = p.Floor.Add(p.Line.Mul(gx * 1.45))
		if sc.Gloss > 0 {
			f := 1 - ny
			c = c.Add(rd.sky(dx*inv, ny, dz*inv).Mul(sc.Gloss * (0.12 + 0.88*f*f)))
		}
	}
	if sc.FloorType == FloorChecker && sc.Gloss > 0 {
		f := 1 - ny
		c = c.Add(rd.sky(dx*inv, ny, dz*inv).Mul(sc.Gloss * (0.05 + 0.6*f*f)))
	}
	for i := range rd.rings {
		r := &rd.rings[i]
		ex, ez := hx-r.x, hz-r.z
		u := (sqrt32(ex*ex+ez*ez) - r.r) / r.w
		if u > -1 && u < 1 {
			u = 1 - u*u
			c = c.Add(mixc(p.Acc, Col{1, 1, 1}, 0.4).Mul(u * u * r.amp * 1.6))
		}
	}
	return mixc(c, rd.fogCol, fog)
}

// --- world-space drawing helpers (fogged) -----------------------------------

func (rd *Rd) fog(vz float32) float32 {
	if rd.noFog {
		return 0
	}
	f := vz * rd.invFar * rd.sc.Fog
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 1
	}
	return f * f
}

// line draws an additive neon line between world points.
func (rd *Rd) line(a, b V3, c Col, inten, w float32) {
	va, vb := rd.cam.View(a), rd.cam.View(b)
	rd.lineVV(va, vb, c, inten, w)
}

func (rd *Rd) lineVV(va, vb V3, c Col, inten, w float32) {
	if va.Z < nearZ && vb.Z < nearZ {
		return
	}
	fa, fb := (1-rd.fog(va.Z))*inten, (1-rd.fog(vb.Z))*inten
	if fa < 0.004 && fb < 0.004 {
		return
	}
	// thin and dim far-away lines so distant objects do not turn into glare
	zm := (va.Z + vb.Z) * 0.5
	if zm > 30 && w > 1 {
		k := 30 / zm
		if k < 0.3 {
			k = 0.3
		}
		w = 1 + (w-1)*k
		k = 0.45 + 0.55*k
		fa *= k
		fb *= k
	}
	rd.fb.lineV(&rd.cam, va, vb, c.Mul(fa), c.Mul(fb), w*rd.ws, true)
}

// lineO draws an opaque (blended) line.
func (rd *Rd) lineO(a, b V3, c Col, w float32) {
	va, vb := rd.cam.View(a), rd.cam.View(b)
	if va.Z < nearZ && vb.Z < nearZ {
		return
	}
	rd.fb.lineV(&rd.cam, va, vb, mixc(c, rd.fogCol, rd.fog(va.Z)), mixc(c, rd.fogCol, rd.fog(vb.Z)), w*rd.ws, false)
}

func (rd *Rd) tv(p V3, c Col) TV {
	v := rd.cam.View(p)
	return TV{v, mixc(c, rd.fogCol, rd.fog(v.Z))}
}

func (rd *Rd) tri(a, b, c V3, ca, cb, cc Col, alpha, bias float32) {
	v := [3]TV{rd.tv(a, ca), rd.tv(b, cb), rd.tv(c, cc)}
	if v[0].P.Z < nearZ && v[1].P.Z < nearZ && v[2].P.Z < nearZ {
		return
	}
	rd.fb.triV(&rd.cam, &v, alpha, bias)
}

func (rd *Rd) quad(a, b, c, d V3, col Col, alpha, bias float32) {
	rd.quadG(a, b, c, d, col, col, col, col, alpha, bias)
}

func (rd *Rd) quadG(a, b, c, d V3, ca, cb, cc, cd Col, alpha, bias float32) {
	va, vb, vc, vd := rd.tv(a, ca), rd.tv(b, cb), rd.tv(c, cc), rd.tv(d, cd)
	if va.P.Z < nearZ && vb.P.Z < nearZ && vc.P.Z < nearZ && vd.P.Z < nearZ {
		return
	}
	t := [3]TV{va, vb, vc}
	rd.fb.triV(&rd.cam, &t, alpha, bias)
	t = [3]TV{va, vc, vd}
	rd.fb.triV(&rd.cam, &t, alpha, bias)
}

// --- sky furniture -------------------------------------------------------------

func (rd *Rd) stars() {
	fb, cam := &rd.fb, &rd.cam
	fr := rd.fr
	for i := range rd.sc.Stars {
		s := &rd.sc.Stars[i]
		v := cam.Dir(s.D)
		if v.Z < 0.05 {
			continue
		}
		if rd.sc.BH && s.D.Z > 0 {
			if q := rd.bhR * 1.1; s.D.X*s.D.X+(s.D.Y-0.03)*(s.D.Y-0.03) < q*q {
				continue
			}
		}
		x := int(cam.cx + cam.f*v.X/v.Z)
		y := int(cam.cy - cam.f*v.Y/v.Z)
		if x < 1 || y < 1 || x >= fb.W-1 || y >= fb.H-1 {
			continue
		}
		b := s.Mag * (0.65 + 0.35*sin32(rd.T*2.2+s.Ph))
		if fr.Pitch > 0 && i%5 == fr.Pitch%5 {
			b *= 1 + 2.5*fr.Melody
		}
		add := func(px, py int, k float32) {
			if fb.gpu {
				fb.gpuPix(px, py, k, k, k*1.1)
				return
			}
			o := (py*fb.W + px) * 3
			fb.C[o] += k
			fb.C[o+1] += k
			fb.C[o+2] += k * 1.1
		}
		add(x, y, b)
		if b > 0.9 {
			k := (b - 0.9) * 0.6
			add(x-1, y, k)
			add(x+1, y, k)
			add(x, y-1, k)
			add(x, y+1, k)
		}
	}
}

func (rd *Rd) backdrop() {
	sc := rd.sc
	if sc.Backdrop == 0 {
		return
	}
	rd.noFog = true
	defer func() { rd.noFog = false }()
	p := &sc.P
	const D = 3000
	N := len(sc.BackH)
	pos := rd.cam.Pos
	sil := mixc(p.Floor, p.Hor, 0.28)
	if sc.Backdrop == 2 {
		sil = mixc(p.Floor, p.Top, 0.5).Mul(0.75)
	}
	haze := mixc(sil, rd.fogCol, 0.75)
	pt := func(i int, h float32) V3 {
		a := float32(i) / float32(N) * 2 * math.Pi
		return pos.Add(V3{sin32(a) * D, h * D, cos32(a) * D})
	}
	for i := 0; i < N; i++ {
		a := (float32(i) + 0.5) / float32(N) * 2 * math.Pi
		if sin32(a)*rd.cam.F.X+cos32(a)*rd.cam.F.Z < -0.1 {
			continue
		}
		h0, h1 := sc.BackH[i], sc.BackH[(i+1)%N]
		if sc.Backdrop == 2 {
			h1 = h0
			if h0 == 0 {
				continue
			}
		}
		b0, b1 := pt(i, -0.03), pt(i+1, -0.03)
		t0, t1 := pt(i, h0), pt(i+1, h1)
		rd.quadG(b0, b1, t1, t0, haze, haze, sil, sil, 1, 0)
		if sc.Backdrop == 1 {
			rd.line(t0, t1, p.Line, 0.55, 1.2)
		} else {
			rd.line(t0, t1, p.Acc, 0.35, 1)
			rows := int(h0 / 0.011)
			for r := 1; r < rows; r++ {
				hh := hashu(sc.Seed, uint32(i), uint32(r))
				if hh&3 != 0 {
					continue
				}
				y := float32(r) * 0.011
				f0 := 0.1 + float32(hh>>4&3)*0.15
				a0 := pt(i, y)
				a1 := pt(i+1, y)
				s0 := a0.Add(a1.Sub(a0).Mul(f0))
				s1 := a0.Add(a1.Sub(a0).Mul(f0 + 0.3))
				col := p.SunT
				if hh&8 != 0 {
					col = p.Acc
				}
				rd.line(s0, s1, col, 0.55, 1.4)
			}
		}
	}
}

// --- valley terrain ---------------------------------------------------------

const terrG = 4

func (rd *Rd) terrH(i int32, j int64) float32 {
	sc := rd.sc
	ax := abs32(float32(i) * terrG)
	m := smooth((ax - sc.Valley) / 18)
	if m == 0 {
		return 0
	}
	n := hf(hashu(sc.Seed, uint32(i), uint32(j)), 3)
	if sc.Hills {
		fi, fj := float32(i), float32(j&0xFFFF)
		sm := 0.5 + 0.25*fsin(fi*0.55+fsin(fj*0.31)*1.3) + 0.25*fsin(fj*0.47+fi*0.21)
		n = sm + (n-sm)*sc.Rough
		return m * n * sc.Amp * (1 + (ax-sc.Valley)/90)
	}
	return m * (0.3 + 0.7*n) * sc.Amp * (1 + (ax-sc.Valley)/70) * (1 + 0.1*rd.fr.Kick)
}

func (rd *Rd) terrain() {
	sc := rd.sc
	p := &sc.P
	const nI = 30
	nJ := farDist/terrG + 2
	j0 := int64(math.Floor(rd.camZ/terrG)) - 1
	cols := 2*nI + 1
	type tvx struct {
		w V3
		v V3
		h float32
	}
	vs := make([]tvx, cols*(nJ+1))
	for j := 0; j <= nJ; j++ {
		z := rd.rz(float64(j0+int64(j)) * terrG)
		for i := 0; i < cols; i++ {
			h := rd.terrH(int32(i-nI), j0+int64(j))
			w := V3{float32(i-nI) * terrG, h, z}
			vs[j*cols+i] = tvx{w, rd.cam.View(w), h}
		}
	}
	fill0, fill1 := sc.TerrC0, sc.TerrC1
	invA := 1 / (sc.Amp * 1.6)
	for j := 0; j < nJ; j++ {
		for i := 0; i < cols-1; i++ {
			a, b := &vs[j*cols+i], &vs[j*cols+i+1]
			c, d := &vs[(j+1)*cols+i+1], &vs[(j+1)*cols+i]
			if a.h == 0 && b.h == 0 && c.h == 0 && d.h == 0 {
				continue
			}
			if a.v.Z < nearZ && b.v.Z < nearZ && c.v.Z < nearZ && d.v.Z < nearZ {
				continue
			}
			sh := 0.75 + 0.35*clampf((d.h-a.h+c.h-b.h)*0.5/terrG, -1, 1)
			if sc.Hills {
				sh = 0.8 - 0.45*clampf((d.h-a.h+c.h-b.h)*0.5/terrG*sc.SunDir.Z+(b.h-a.h+c.h-d.h)*0.5/terrG*sc.SunDir.X, -1, 1)
			}
			col := func(t *tvx) Col {
				return mixc(mixc(fill0, fill1, clamp01(t.h*invA)).Mul(sh), rd.fogCol, rd.fog(t.v.Z))
			}
			ta, tb, tc, td := TV{a.v, col(a)}, TV{b.v, col(b)}, TV{c.v, col(c)}, TV{d.v, col(d)}
			t := [3]TV{ta, tb, tc}
			rd.fb.triV(&rd.cam, &t, 1, 0)
			t = [3]TV{ta, tc, td}
			rd.fb.triV(&rd.cam, &t, 1, 0)
		}
	}
	for j := nJ - 1; j >= 0 && sc.TerrLine > 0; j-- {
		for i := 0; i < cols-1; i++ {
			a, b, d := &vs[j*cols+i], &vs[j*cols+i+1], &vs[(j+1)*cols+i]
			if a.h == 0 && b.h == 0 && d.h == 0 {
				continue
			}
			in := sc.TerrLine * (1 + 0.45*rd.fr.Kick)
			rd.lineVV(a.v, b.v, p.Line, in, 1.3)
			rd.lineVV(a.v, d.v, p.Line, in, 1.3)
		}
	}
}
