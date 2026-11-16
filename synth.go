package main

import (
	"math"
	"sync"
)

const tau = 2 * math.Pi

type fmVoice struct {
	on               bool
	ph, mph, f, rat  float64
	idx, idxMul      float64
	env, envMul, rel float64
	amp, panL, panR  float64
	age, dur         int
	sendD, sendR     float64
	grp              int // 0 keys, 1 lead, 2 click chimes
}

type padBank struct {
	n      int
	f      [5]float64
	ph     [5][3]float64
	env    float64
	target float64
}

type comb struct {
	buf             []float64
	i               int
	fb, damp, store float64
}

func (c *comb) proc(x float64) float64 {
	y := c.buf[c.i]
	c.store = y*(1-c.damp) + c.store*c.damp
	c.buf[c.i] = x + c.store*c.fb
	c.i++
	if c.i == len(c.buf) {
		c.i = 0
	}
	return y
}

type allp struct {
	buf []float64
	i   int
}

func (a *allp) proc(x float64) float64 {
	b := a.buf[a.i]
	a.buf[a.i] = x + b*0.5
	a.i++
	if a.i == len(a.buf) {
		a.i = 0
	}
	return b - x
}

// metallic oscillator bank (ride / crash), 808-style inharmonic squares
var metalRatios = [6]float64{1, 1.4471, 1.617, 1.9265, 2.5028, 2.6637}

type Synth struct {
	s       *Song
	n       int64
	step    int
	stepAt  float64
	stepLen float64
	rng     uint32

	bank     [2]padBank
	cur      int
	padLP    [2][2]float64
	padA     float64
	lfo      float64
	duck     float64
	duckMul  float64
	padGate  float64
	padGateT float64

	// drums
	kT, sT, hT, rT, cT, clT int
	kV, sV, hV, rV, cV, clV float64
	kPh                     float64
	sLP, sLP2, hPrev        float64
	hOpen                   bool
	mPh                     [6]float64
	rHP, cHP, clLP, clHP    float64

	// bass
	bPh, bF, bEnv, bLP, bLP2 float64
	bGate, bAge              int

	// guitar (double-tracked power chords)
	gF    [3]float64
	gPh   [2][3]float64
	gEnv  float64
	gVel  float64
	gGate int
	gMute bool
	gLP   [2][2]float64
	gHP   [2]float64

	// mono lead
	lPh, lF, lT    float64
	lEnv, lVib     float64
	lGate, lAge    int
	lLP, lLP2, lBr float64

	fm       [24]fmVoice
	dlL, dlR []float64
	dpos     int
	dfL, dfR float64
	combs    [2][8]comb
	alls     [2][3]allp
	mLP      [2]float64
	mLP2     [2]float64
	pop      float64
	chime    int
}

func NewSynth(s *Song) *Synth {
	y := &Synth{s: s, rng: s.Seed | 1, kT: -1, sT: -1, hT: -1, rT: -1, cT: -1, clT: -1, padGate: 1, padGateT: 1}
	y.stepLen = s.SPB / 4
	y.duckMul = math.Exp(-1 / (0.22 * SR))
	dl := int(y.stepLen * 3)
	y.dlL = make([]float64, dl)
	y.dlR = make([]float64, dl)
	cl := []int{1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617}
	al := []int{556, 441, 341}
	for ch := 0; ch < 2; ch++ {
		for i, l := range cl {
			y.combs[ch][i] = comb{buf: make([]float64, l+ch*23), fb: 0.9, damp: 0.35}
		}
		for i, l := range al {
			y.alls[ch][i] = allp{buf: make([]float64, l+ch*23)}
		}
	}
	return y
}

func (y *Synth) noise() float64 {
	y.rng = y.rng*1664525 + 1013904223
	return float64(int32(y.rng)) / 2147483648.0
}

func (y *Synth) fmOn(midi, vel, pan, rat, idx, idxSec, ampSec float64, dur int, sd, sr float64, grp int) {
	best, bestE := 0, math.MaxFloat64
	for i := range y.fm {
		v := &y.fm[i]
		if !v.on {
			best = i
			break
		}
		if v.env*v.amp < bestE {
			best, bestE = i, v.env*v.amp
		}
	}
	y.fm[best] = fmVoice{on: true, f: mtof(midi), rat: rat, idx: idx,
		idxMul: math.Exp(-1 / (idxSec * SR)), env: 1, envMul: math.Exp(-1 / (ampSec * SR)),
		rel: math.Exp(-1 / (0.12 * SR)), amp: vel, panL: math.Sqrt(1 - pan), panR: math.Sqrt(pan),
		dur: dur, sendD: sd, sendR: sr, grp: grp}
}

func (y *Synth) trigger(abs int) {
	s := y.s
	bar, st := abs/16, abs%16
	if st == 0 && (bar%s.ChordBars == 0) {
		y.bank[y.cur].target = 0
		y.cur ^= 1
		b := &y.bank[y.cur]
		var buf [6]int
		notes := s.ChordNotes(bar, buf[:])
		b.n = len(notes)
		for i, n := range notes {
			b.f[i] = mtof(float64(n))
		}
		b.target = 1
	}
	if s.PadChop != 0xFFFF {
		y.padGateT = 0.12
		if s.PadChop&(1<<uint(15-st)) != 0 {
			y.padGateT = 1
		}
	}
	if v := s.KickVel(bar, st); v > 0 {
		y.kT, y.kV, y.kPh = 0, float64(v), 0
		y.duck = []float64{1, 0.6, 0.35}[s.KickKind]
	}
	if v := s.SnareVel(bar, st); v > 0 {
		y.sT, y.sV = 0, float64(v)
	}
	if v := s.ClapVel(bar, st); v > 0 {
		y.clT, y.clV = 0, float64(v)
	}
	if v := s.HatVel(bar, st); v > 0 {
		y.hT, y.hV, y.hOpen = 0, float64(v), s.OpenHat[st]
	}
	if v := s.RideVel(bar, st); v > 0 {
		y.rT, y.rV = 0, float64(v)
	}
	if v := s.CrashVel(bar, st); v > 0 {
		y.cT, y.cV = 0, float64(v)
	}
	if n, l := s.BassNote(bar, st); n > 0 {
		y.bF = mtof(float64(n))
		y.bGate = int(y.stepLen * float64(l))
		y.bAge = 0
		if s.BassKind != BassSub {
			y.bEnv = 1
		}
	}
	if v, l, mute := s.CompHit(bar, st); v > 0 {
		dur := int(y.stepLen * float64(l))
		switch s.CompKind {
		case CompGuitar:
			r := float64(s.GuitarRoot(bar))
			y.gF = [3]float64{mtof(r), mtof(r + 7), mtof(r + 12)}
			y.gGate, y.gMute, y.gVel = dur, mute, float64(v)
			if mute {
				y.gEnv = 1
			}
		case CompRhodes:
			var buf [6]int
			for i, n := range s.ChordNotes(bar, buf[:]) {
				pan := 0.3 + 0.4*float64(i)/4
				y.fmOn(float64(n), float64(v)*0.21, pan, 1, 1.7, 0.35, 1.6, dur, 0.15, 0.45, 0)
			}
		case CompPluck:
			var buf [6]int
			for i, n := range s.ChordNotes(bar, buf[:]) {
				pan := 0.25 + 0.5*float64(i%3)/2
				y.fmOn(float64(n+12), float64(v)*0.16, pan, 2, 2.4, 0.07, 0.3, dur, 0.35, 0.35, 0)
			}
		}
	}
	if n := s.ArpNote(bar, st); n > 0 {
		pan := 0.3 + 0.4*float64(st%4)/3
		vel := 0.11
		if s.ArpMode == 1 {
			vel = 0.13
		}
		y.fmOn(float64(n), vel, pan, 1, 1.4, 0.25, 0.9, int(y.stepLen*1.5), 0.5, 0.5, 0)
	}
	if n, l := s.LeadNote(bar, st); n > 0 {
		dur := int(y.stepLen * float64(l) * 0.95)
		switch s.LeadType {
		case LeadBell:
			y.fmOn(float64(n), 0.2, 0.5, 1, 2.2, 0.4, 1.6, dur, 0.55, 0.55, 1)
			y.fmOn(float64(n)+0.07, 0.07, 0.3, 4, 0.8, 0.1, 0.5, dur, 0.3, 0.5, 1)
		default:
			y.lT = mtof(float64(n))
			if y.lEnv < 0.01 || y.lF == 0 {
				y.lF = y.lT
			}
			y.lGate, y.lAge = dur, 0
		}
	}
}

func polyblep(t, dt float64) float64 {
	if t < dt {
		t /= dt
		return t + t - t*t - 1
	}
	if t > 1-dt {
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

func saw(p, dt float64) float64 { return 2*p - 1 - polyblep(p, dt) }

func square(p, dt float64) float64 {
	q := p + 0.5
	if q >= 1 {
		q--
	}
	return saw(p, dt) - saw(q, dt)
}

func adv(p *float64, dt float64) float64 {
	*p += dt
	if *p >= 1 {
		*p -= 1
	}
	return *p
}

var padDet = [3]float64{0.9955, 1, 1.0045}
var padPL = [3]float64{0.85, 0.55, 0.2}
var padPR = [3]float64{0.2, 0.55, 0.85}

// Chime plays a bell on a chord tone (used for mouse clicks).
func (y *Synth) Chime() { y.chime++ }

func (y *Synth) metal(base float64) float64 {
	var m float64
	for i, r := range metalRatios {
		dt := base * r / SR
		p := adv(&y.mPh[i], dt)
		if p < 0.5 {
			m++
		} else {
			m--
		}
	}
	return m / 6
}

func (y *Synth) next(m *Mix) (float64, float64) {
	s := y.s
	for float64(y.n) >= y.stepAt {
		y.trigger(y.step)
		y.step++
		y.stepAt = (float64(y.step) + s.StepOffset(y.step%16)) * y.stepLen
	}
	if y.chime > 0 {
		y.chime--
		var buf [6]int
		notes := s.ChordNotes(y.step/16, buf[:])
		n := notes[int(y.rng>>8)%len(notes)] + 24
		y.fmOn(float64(n), 0.22, 0.2+0.6*float64(y.rng>>20&255)/255, 3.5, 2.5, 0.5, 1.8, SR/2, 0.6, 0.7, 2)
	}
	y.n++
	var L, R, dS, rL, rR float64
	nz := y.noise()
	dr := m.Drums

	// --- pad
	if y.n&31 == 1 {
		y.lfo += 32.0 / SR * 0.11
		fc := s.PadCut * (1 + 0.35*math.Sin(tau*y.lfo))
		y.padA = 1 - math.Exp(-tau*fc/SR)
	}
	var pL, pR float64
	for bi := range y.bank {
		b := &y.bank[bi]
		k := 1.0 / (0.30 * SR)
		if b.target < b.env {
			k = 1.0 / (0.45 * SR)
		}
		b.env += (b.target - b.env) * k
		if b.env < 1e-4 {
			continue
		}
		var sl, sr float64
		for i := 0; i < b.n; i++ {
			for d := 0; d < 3; d++ {
				dt := b.f[i] * padDet[d] / SR
				v := saw(adv(&b.ph[i][d], dt), dt)
				sl += v * padPL[d]
				sr += v * padPR[d]
			}
		}
		g := b.env / float64(b.n)
		pL += sl * g
		pR += sr * g
	}
	y.padLP[0][0] += (pL - y.padLP[0][0]) * y.padA
	y.padLP[0][1] += (y.padLP[0][0] - y.padLP[0][1]) * y.padA
	y.padLP[1][0] += (pR - y.padLP[1][0]) * y.padA
	y.padLP[1][1] += (y.padLP[1][0] - y.padLP[1][1]) * y.padA
	y.duck *= y.duckMul
	y.padGate += (y.padGateT - y.padGate) * 0.004
	pg := 0.45 * (1 - 0.55*y.duck) * m.Pads * s.PadLevel * y.padGate
	L += y.padLP[0][1] * pg
	R += y.padLP[1][1] * pg
	rL += y.padLP[0][1] * pg * 0.45
	rR += y.padLP[1][1] * pg * 0.45

	// --- kick
	if y.kT >= 0 {
		t := float64(y.kT) / SR
		var f, a, click float64
		clickN := 90
		switch s.KickKind {
		case KickPunch:
			f, a, click, clickN = 52+170*math.Exp(-t*38), math.Exp(-t*9), 0.4, 60
		case KickTight:
			f, a, click, clickN = 58+230*math.Exp(-t*55), math.Exp(-t*16), 0.6, 40
		default:
			f, a, click = 46+130*math.Exp(-t*26), math.Exp(-t*6.5), 0.25
		}
		y.kPh += f / SR
		k := math.Sin(tau*y.kPh) * a
		if y.kT < clickN {
			k += nz * click * (1 - float64(y.kT)/float64(clickN))
		}
		if s.KickKind != KickDeep {
			k = math.Tanh(k * 1.6)
		}
		k *= y.kV * dr * 0.8
		L += k
		R += k
		y.kT++
		if t > 0.6 {
			y.kT = -1
		}
	}
	// --- snare
	if y.sT >= 0 {
		t := float64(y.sT) / SR
		y.sLP += (nz - y.sLP) * 0.22
		hp := nz - y.sLP
		var v, send float64
		switch s.SnareKind {
		case SnareBrush:
			y.sLP2 += (nz - y.sLP2) * 0.3
			v = y.sLP2 * math.Min(1, t/0.012) * math.Exp(-t*10) * 0.55
			send = 0.5
		case SnareGated:
			v = (0.75*hp+0.25*nz)*math.Exp(-t*6) + math.Sin(tau*180*t)*math.Exp(-t*20)*0.6
			if t > 0.2 {
				v *= math.Max(0, 1-(t-0.2)/0.03)
			}
			v *= 0.42
			send = 1.1
		case SnareTight:
			v = (0.8*hp*math.Exp(-t*20) + math.Sin(tau*230*t)*math.Exp(-t*30)*0.7) * 0.4
			send = 0.4
		default:
			v = ((0.7*hp+0.3*nz)*math.Exp(-t*15) + math.Sin(tau*188*t)*math.Exp(-t*24)*0.6) * 0.38
			send = 0.55
		}
		v *= y.sV * dr
		L += v
		R += v
		rL += v * send
		rR += v * send
		y.sT++
		if t > 0.5 {
			y.sT = -1
		}
	}
	// --- clap: three quick bursts and a tail
	if y.clT >= 0 {
		t := float64(y.clT) / SR
		y.clLP += (nz - y.clLP) * 0.35
		y.clHP += (y.clLP - y.clHP) * 0.05
		bp := y.clLP - y.clHP
		e := 0.0
		for _, t0 := range []float64{0, 0.011, 0.022} {
			if t >= t0 {
				e = math.Max(e, math.Exp(-(t-t0)*180))
			}
		}
		if t > 0.022 {
			e = math.Max(e, 0.6*math.Exp(-(t-0.022)*16))
		}
		v := bp * e * y.clV * 0.9 * dr
		L += v * 0.9
		R += v
		rL += v * 0.6
		rR += v * 0.6
		y.clT++
		if t > 0.4 {
			y.clT = -1
		}
	}
	// --- hat
	if y.hT >= 0 {
		t := float64(y.hT) / SR
		d := 75.0
		if y.hOpen {
			d = 11
		}
		v := (nz - y.hPrev) * math.Exp(-t*d) * y.hV * 0.075 * dr
		L += v * 0.7
		R += v * 1.1
		rL += v * 0.15
		rR += v * 0.15
		y.hT++
		if t > 0.5 {
			y.hT = -1
		}
	}
	y.hPrev = nz
	// --- ride / crash (shared metallic bank)
	if y.rT >= 0 || y.cT >= 0 {
		mt := y.metal(520)
		if y.rT >= 0 {
			t := float64(y.rT) / SR
			y.rHP += (mt - y.rHP) * 0.12
			v := (mt - y.rHP) * (0.55*math.Exp(-t*2.8) + 0.6*math.Exp(-t*30)) * y.rV * 0.1 * dr
			L += v * 1.1
			R += v * 0.7
			rL += v * 0.3
			rR += v * 0.3
			y.rT++
			if t > 1.6 {
				y.rT = -1
			}
		}
		if y.cT >= 0 {
			t := float64(y.cT) / SR
			y.cHP += (nz - y.cHP) * 0.25
			x := 0.6*(nz-y.cHP) + 0.4*mt
			v := x * (math.Exp(-t*1.5) + 0.8*math.Exp(-t*14)) * y.cV * 0.09 * dr
			L += v * 0.8
			R += v * 1.1
			rL += v * 0.5
			rR += v * 0.5
			y.cT++
			if t > 2.5 {
				y.cT = -1
			}
		}
	}

	// --- bass
	y.bAge++
	switch s.BassKind {
	case BassSub:
		if y.bGate > 0 {
			y.bEnv += (1 - y.bEnv) * 0.004
		} else {
			y.bEnv *= 0.9992
		}
	case BassSaw:
		if y.bGate <= 0 {
			y.bEnv *= 0.998
		}
	default: // plucked
		k := 0.55
		if s.BassKind == BassSlap {
			k = 0.35
		}
		y.bEnv *= math.Exp(-1 / (k * SR))
		if y.bGate <= 0 {
			y.bEnv *= 0.9985
		}
	}
	if y.bGate > 0 {
		y.bGate--
	}
	if y.bEnv > 1e-4 && y.bF > 0 {
		age := float64(y.bAge) / SR
		var v float64
		switch s.BassKind {
		case BassUpright:
			p := adv(&y.bPh, y.bF*(1+0.02*math.Exp(-age*80))/SR)
			v = math.Sin(tau*p) + 0.3*math.Sin(2*tau*p) + 0.1*math.Sin(3*tau*p)
			v += nz * 0.4 * math.Exp(-age*250)
			y.bLP += (v - y.bLP) * 0.25
			v = math.Tanh(y.bLP*y.bEnv*1.4) * 0.26
		case BassSaw:
			dt := y.bF / SR
			x := saw(adv(&y.bPh, dt), dt)
			top := 1400.0
			if s.Style == StyleMetal || s.Style == StyleRock {
				top = 2200
			}
			fc := 160 + top*math.Exp(-age/0.09)
			a := 1 - math.Exp(-tau*fc/SR)
			y.bLP += (x - y.bLP) * a
			y.bLP2 += (y.bLP - y.bLP2) * a
			v = math.Tanh(y.bLP2*y.bEnv*1.8) * 0.3
		case BassSlap:
			p := adv(&y.bPh, y.bF/SR)
			v = math.Sin(tau*p) + 0.6*math.Sin(2*tau*p)*math.Exp(-age*20) + 0.35*math.Sin(3*tau*p)*math.Exp(-age*33)
			v += nz * 0.5 * math.Exp(-age*400)
			v = math.Tanh(v*y.bEnv*1.3) * 0.34
		default:
			p := adv(&y.bPh, y.bF/SR)
			v = math.Sin(tau*p) + 0.35*math.Sin(2*tau*p) + 0.12*math.Sin(3*tau*p)
			v = math.Tanh(v*y.bEnv*1.3) * 0.34
		}
		v *= m.Bass
		L += v
		R += v
	}

	// --- guitar
	if y.gGate > 0 {
		y.gGate--
		if y.gMute {
			y.gEnv *= 0.9993
		} else {
			y.gEnv += (1 - y.gEnv) * 0.01
			y.gEnv *= 0.99999
		}
	} else {
		y.gEnv *= 0.997
	}
	if y.gEnv > 1e-4 && y.gF[0] > 0 {
		cab := 0.3
		if y.gMute {
			cab = 0.19
		}
		for side := 0; side < 2; side++ {
			det := 1 + 0.004*float64(side)
			var x float64
			for i := 0; i < 3; i++ {
				dt := y.gF[i] * det / SR
				x += saw(adv(&y.gPh[side][i], dt), dt)
			}
			y.gHP[side] += (x - y.gHP[side]) * 0.01
			x = math.Tanh((x - y.gHP[side]) * y.gEnv * s.GtrDrive)
			y.gLP[side][0] += (x - y.gLP[side][0]) * cab
			y.gLP[side][1] += (y.gLP[side][0] - y.gLP[side][1]) * cab
			v := y.gLP[side][1] * 0.5 * y.gVel * m.Keys
			if side == 0 {
				L += v
				rL += v * 0.15
			} else {
				R += v
				rR += v * 0.15
			}
		}
	}

	// --- mono lead
	if y.lGate > 0 {
		y.lGate--
		y.lAge++
		y.lEnv += (1 - y.lEnv) * 0.0015
	} else {
		y.lEnv *= 0.9997
	}
	if y.lEnv > 1e-4 && y.lF > 0 {
		glide := 0.0012
		vibD := 0.006
		switch s.LeadType {
		case LeadSax:
			glide, vibD = 0.0025, 0.01
		case LeadGuitar:
			glide, vibD = 0.004, 0.012
		}
		y.lF += (y.lT - y.lF) * glide
		y.lVib += 5.3 / SR
		vd := math.Min(1, float64(y.lAge)/(0.35*SR)) * vibD
		f := y.lF * (1 + vd*math.Sin(tau*y.lVib))
		dt := f / SR
		p := adv(&y.lPh, dt)
		var v float64
		switch s.LeadType {
		case LeadSaw:
			v = saw(p, dt)*0.6 + math.Sin(tau*p)*0.5
			y.lLP += (v - y.lLP) * 0.22
			v = y.lLP * 0.22
		case LeadSax:
			x := saw(p, dt)*0.6 + square(p, dt)*0.3
			k := 0.1 + 0.12*y.lEnv
			y.lLP += (x - y.lLP) * k
			y.lLP2 += (y.lLP - y.lLP2) * k
			y.lBr += (y.lLP2 - y.lBr) * 0.01
			v = ((y.lLP2-y.lBr)*1.4 + nz*0.05*y.lEnv) * 0.12
		case LeadGuitar:
			x := math.Tanh(saw(p, dt) * 9)
			y.lLP += (x - y.lLP) * 0.3
			y.lLP2 += (y.lLP - y.lLP2) * 0.3
			v = y.lLP2 * 0.11
		case LeadSquare:
			x := square(p, dt) * 0.5
			y.lLP += (x - y.lLP) * 0.35
			v = y.lLP * 0.26 * math.Exp(-float64(y.lAge)/(0.3*SR))
		default:
			v = (math.Sin(tau*p) + 0.28*math.Sin(2*tau*p) + 0.08*math.Sin(3*tau*p) + nz*0.02) * 0.16
		}
		v *= y.lEnv * m.Lead
		L += v
		R += v
		dS += v * 0.6
		rL += v * 0.5
		rR += v * 0.5
	}

	// --- FM voices (e-piano, plucks, bells)
	for i := range y.fm {
		v := &y.fm[i]
		if !v.on {
			continue
		}
		v.age++
		v.env *= v.envMul
		if v.age > v.dur {
			v.env *= v.rel
			if v.env < 1e-4 {
				v.on = false
				continue
			}
		}
		v.idx *= v.idxMul
		a := v.env * v.amp
		if v.age < 160 {
			a *= float64(v.age) / 160
		}
		v.ph += v.f / SR
		v.mph += v.f * v.rat / SR
		if v.ph > 1e6 {
			v.ph -= 1e6
		}
		g := m.Keys
		if v.grp == 1 {
			g = m.Lead
		} else if v.grp == 2 {
			g = m.SFX
		}
		o := math.Sin(tau*v.ph+v.idx*math.Sin(tau*v.mph)) * a * g
		l, r := o*v.panL, o*v.panR
		L += l
		R += r
		dS += o * v.sendD
		rL += l * v.sendR
		rR += r * v.sendR
	}

	// --- ping-pong delay
	dl, drr := y.dlL[y.dpos], y.dlR[y.dpos]
	y.dfL += (drr - y.dfL) * 0.35
	y.dfR += (dl - y.dfR) * 0.35
	y.dlL[y.dpos] = dS*m.Echo + y.dfL*0.42
	y.dlR[y.dpos] = y.dfR * 0.42
	y.dpos++
	if y.dpos == len(y.dlL) {
		y.dpos = 0
	}
	L += dl * 0.5
	R += drr * 0.5
	rL += dl * 0.25
	rR += drr * 0.25

	// --- reverb
	var wl, wr float64
	il, ir := rL*0.05*m.Reverb, rR*0.05*m.Reverb
	for i := 0; i < 8; i++ {
		wl += y.combs[0][i].proc(il)
		wr += y.combs[1][i].proc(ir)
	}
	for i := 0; i < 3; i++ {
		wl = y.alls[0][i].proc(wl)
		wr = y.alls[1][i].proc(wr)
	}
	rg := 1.5 * (1 - 0.3*y.duck)
	L += wl * rg
	R += wr * rg

	// --- tape hiss + vinyl crackle
	L += nz * 0.0016 * m.Crackle
	R -= nz * 0.0016 * m.Crackle
	if y.rng>>8 < 1400 {
		y.pop = (float64(y.rng&255)/255 - 0.5) * 0.05 * m.Crackle
	}
	L += y.pop
	R += y.pop * 0.7
	y.pop *= 0.82

	// --- master
	L = math.Tanh(L * 1.25)
	R = math.Tanh(R * 1.25)
	a := m.ToneA
	y.mLP[0] += (L - y.mLP[0]) * a
	y.mLP[1] += (R - y.mLP[1]) * a
	y.mLP2[0] += (y.mLP[0] - y.mLP2[0]) * a
	y.mLP2[1] += (y.mLP[1] - y.mLP2[1]) * a
	mid := (y.mLP2[0] + y.mLP2[1]) * 0.5
	side := (y.mLP2[0] - y.mLP2[1]) * 0.5 * m.Width
	return mid + side, mid - side
}

// ---------------------------------------------------------------------------

type checkpoint struct {
	out0       int64
	src0, src1 float64
	n          int
	gen        int
}

// Engine plays songs at a variable tape speed and remembers which part of the
// song went into which output buffer, so that visuals can be locked to audio.
type Engine struct {
	mu        sync.Mutex
	cur, old  *Synth
	fade      float64
	gen       int
	rate      float64
	rateT     float64
	wow, flut float64
	frac      float64
	pl, pr    float64
	cl, cr    float64
	src       float64
	outTotal  int64
	cps       [64]checkpoint
	cpN       int
	mute      bool
	vol       float64
	mix       Mix
}

func NewEngine() *Engine { return &Engine{rate: 1, rateT: 1, vol: 1, mix: DefaultMix()} }

func (e *Engine) SetMix(m Mix) { e.mu.Lock(); e.mix = m; e.mu.Unlock() }

// Written returns the number of output frames generated so far.
func (e *Engine) Written() int64 { e.mu.Lock(); defer e.mu.Unlock(); return e.outTotal }

func (e *Engine) NewSong(s *Song) {
	e.mu.Lock()
	e.old, e.fade = e.cur, 1
	e.cur = NewSynth(s)
	e.gen++
	e.src, e.frac = 0, 0
	e.mu.Unlock()
}

func (e *Engine) SetRate(r float64) { e.mu.Lock(); e.rateT = r; e.mu.Unlock() }
func (e *Engine) Rate() float64     { e.mu.Lock(); defer e.mu.Unlock(); return e.rate }
func (e *Engine) SetMute(m bool)    { e.mu.Lock(); e.mute = m; e.mu.Unlock() }
func (e *Engine) Chime() {
	e.mu.Lock()
	if e.cur != nil {
		e.cur.Chime()
	}
	e.mu.Unlock()
}

// Fill renders interleaved stereo 16-bit frames.
func (e *Engine) Fill(out []int16) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(out) / 2
	cp := checkpoint{out0: e.outTotal, src0: e.src + e.frac, n: n, gen: e.gen}
	vt := e.mix.Master
	if e.mute {
		vt = 0
	}
	for i := 0; i < n; i++ {
		e.rate += (e.rateT - e.rate) * 0.00009
		e.wow += 0.45 / SR
		e.flut += 6.3 / SR
		r := e.rate * (1 + (0.0022*math.Sin(tau*e.wow)+0.0006*math.Sin(tau*e.flut))*e.mix.Wobble)
		e.frac += r
		for e.frac >= 1 {
			e.frac--
			e.src++
			e.pl, e.pr = e.cl, e.cr
			var l, rr float64
			if e.cur != nil {
				l, rr = e.cur.next(&e.mix)
			}
			if e.old != nil {
				ol, or := e.old.next(&e.mix)
				l += ol * e.fade
				rr += or * e.fade
				e.fade -= 1.0 / (0.3 * SR)
				if e.fade <= 0 {
					e.old = nil
				}
			}
			e.cl, e.cr = l, rr
		}
		e.vol += (vt - e.vol) * 0.002
		l := (e.pl + (e.cl-e.pl)*e.frac) * e.vol
		rr := (e.pr + (e.cr-e.pr)*e.frac) * e.vol
		out[2*i] = int16(clampF(l, -1, 1) * 30000)
		out[2*i+1] = int16(clampF(rr, -1, 1) * 30000)
	}
	cp.src1 = e.src + e.frac
	e.outTotal += int64(n)
	e.cps[e.cpN%len(e.cps)] = cp
	e.cpN++
}

// SrcAt maps an output frame position to a source-sample position of the
// current song (negative while the previous song is still draining).
func (e *Engine) SrcAt(outPos float64) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var first *checkpoint
	for k := 1; k <= len(e.cps) && k <= e.cpN; k++ {
		c := &e.cps[(e.cpN-k)%len(e.cps)]
		if c.gen != e.gen {
			break
		}
		first = c
		if outPos >= float64(c.out0) {
			return c.src0 + (outPos-float64(c.out0))*(c.src1-c.src0)/float64(c.n)
		}
	}
	if first != nil {
		return first.src0 - (float64(first.out0)-outPos)*e.rate
	}
	return -0.12 * SR
}

// OutTotal is the number of output frames generated so far.
func (e *Engine) OutTotal() int64 { e.mu.Lock(); defer e.mu.Unlock(); return e.outTotal }
