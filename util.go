package main

import (
	"math"
	"runtime"
	"sync"
	"sync/atomic"
)

type V3 struct{ X, Y, Z float32 }

func (a V3) Add(b V3) V3      { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V3) Sub(b V3) V3      { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V3) Mul(s float32) V3 { return V3{a.X * s, a.Y * s, a.Z * s} }
func (a V3) Dot(b V3) float32 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a V3) Cross(b V3) V3 {
	return V3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func (a V3) Norm() V3 {
	l := sqrt32(a.Dot(a))
	if l < 1e-9 {
		return V3{0, 1, 0}
	}
	return a.Mul(1 / l)
}

type Col struct{ R, G, B float32 }

func hexc(h uint32) Col {
	return Col{float32(h>>16&255) / 255, float32(h>>8&255) / 255, float32(h&255) / 255}
}
func (c Col) Mul(s float32) Col { return Col{c.R * s, c.G * s, c.B * s} }
func (c Col) Add(d Col) Col     { return Col{c.R + d.R, c.G + d.G, c.B + d.B} }
func mixc(a, b Col, t float32) Col {
	return Col{a.R + (b.R-a.R)*t, a.G + (b.G-a.G)*t, a.B + (b.B-a.B)*t}
}

func hueRot(c Col, deg float32) Col {
	a := float64(deg) * math.Pi / 180
	cs, sn := float32(math.Cos(a)), float32(math.Sin(a))
	k := (1 - cs) / 3
	q := sn * 0.57735027
	r := c.R*(cs+k) + c.G*(k-q) + c.B*(k+q)
	g := c.R*(k+q) + c.G*(cs+k) + c.B*(k-q)
	b := c.R*(k-q) + c.G*(k+q) + c.B*(cs+k)
	return Col{clamp01(r), clamp01(g), clamp01(b)}
}

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
func clampf(x, a, b float32) float32 {
	if x < a {
		return a
	}
	if x > b {
		return b
	}
	return x
}
func sqrt32(x float32) float32 { return float32(math.Sqrt(float64(x))) }
func sin32(x float32) float32  { return float32(math.Sin(float64(x))) }
func cos32(x float32) float32  { return float32(math.Cos(float64(x))) }
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
func floor32(x float32) float32 {
	i := float32(int32(x))
	if i > x {
		i--
	}
	return i
}
func ifloor(x float32) int32 {
	i := int32(x)
	if float32(i) > x {
		i--
	}
	return i
}
func smooth(x float32) float32 { x = clamp01(x); return x * x * (3 - 2*x) }

// fsin is a fast sine approximation, good to ~0.1%.
func fsin(x float32) float32 {
	k := x * 0.15915494
	k -= floor32(k + 0.5)
	y := k * 6.2831853
	y = 1.27323954*y - 0.405284735*y*abs32(y)
	return 0.225*(y*abs32(y)-y) + y
}

func hashu(a, b, c uint32) uint32 {
	h := a*0x9E3779B1 ^ (b+0x7F4A7C15)*0x85EBCA77 ^ (c+0x165667B1)*0xC2B2AE3D
	h ^= h >> 15
	h *= 0x2C1B3C6D
	h ^= h >> 12
	h *= 0x297A2D39
	h ^= h >> 15
	return h
}

// hf returns a float in [0,1) derived from h and a salt.
func hf(h uint32, salt uint32) float32 {
	return float32(hashu(h, salt, 0x51)>>8) / 16777216.0
}

var nWorkers = func() int {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > 12 {
		n = 12
	}
	return n
}()

// parallel runs fn over [0,n) in chunks across worker goroutines.
func parallel(n, chunk int, fn func(a, b int)) {
	if nWorkers == 1 || n <= chunk {
		fn(0, n)
		return
	}
	var next int64
	var wg sync.WaitGroup
	for w := 0; w < nWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				a := int(atomic.AddInt64(&next, int64(chunk))) - chunk
				if a >= n {
					return
				}
				b := a + chunk
				if b > n {
					b = n
				}
				fn(a, b)
			}
		}()
	}
	wg.Wait()
}
