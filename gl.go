package vaporwave

// OpenGL 3.3 renderer. Sky, floor and post-processing run as fragment
// shaders; props, terrain and lines are projected on the CPU (shared with
// the software renderer) and rasterised by the GPU.
//
// Every GL entry point is called through glcall with integer/pointer
// arguments only (floats are always passed through *v pointer variants),
// which lets the same code run via syscall on Windows and via a small cgo
// trampoline in the Linux test harness.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

const (
	glTriangles         = 0x0004
	glDepthBufferBit    = 0x0100
	glLEqual            = 0x0203
	glOneMinusSrcAlpha  = 0x0303
	glCullFace          = 0x0B44
	glDepthTest         = 0x0B71
	glBlend             = 0x0BE2
	glScissorTest       = 0x0C11
	glUnpackAlignment   = 0x0CF5
	glPackAlignment     = 0x0D05
	glTexture2D         = 0x0DE1
	glUnsignedByte      = 0x1401
	glFloat             = 0x1406
	glHalfFloat         = 0x140B
	glDepth             = 0x1801
	glRGBA              = 0x1908
	glRenderer          = 0x1F01
	glVersion           = 0x1F02
	glNearest           = 0x2600
	glLinear            = 0x2601
	glTexMagFilter      = 0x2800
	glTexMinFilter      = 0x2801
	glTexWrapS          = 0x2802
	glTexWrapT          = 0x2803
	glRGBA8             = 0x8058
	glBGRA              = 0x80E1
	glClampToEdge       = 0x812F
	glDepthComponent24  = 0x81A6
	glTexture0          = 0x84C0
	glRGBA16F           = 0x881A
	glArrayBuffer       = 0x8892
	glStreamDraw        = 0x88E0
	glFragmentShader    = 0x8B30
	glVertexShader      = 0x8B31
	glCompileStatus     = 0x8B81
	glLinkStatus        = 0x8B82
	glFramebufferDone   = 0x8CD5
	glColorAttachment0  = 0x8CE0
	glDepthAttachment   = 0x8D00
	glFramebuffer       = 0x8D40
	glRenderbuffer      = 0x8D41
	glFramebufferSRGB   = 0x8DB9
	glMultisample       = 0x809D
	glDither            = 0x0BD0
	glOne               = 1
	glFalse, glTrueBool = 0, 1
)

type glFuncs struct {
	GetString, GetError, Viewport, Enable, Disable, BlendFunc, DepthFunc, DepthMask   uintptr
	ClearBufferfv, DrawArrays, GenTextures, BindTexture, TexImage2D, TexSubImage2D    uintptr
	TexParameteri, ActiveTexture, PixelStorei, ReadPixels, Finish, Flush              uintptr
	CreateShader, ShaderSource, CompileShader, GetShaderiv, GetShaderInfoLog          uintptr
	CreateProgram, AttachShader, LinkProgram, GetProgramiv, GetProgramInfoLog         uintptr
	UseProgram, GetUniformLocation, Uniform4fv, Uniform2fv, Uniform1i, Uniform2iv     uintptr
	Uniform4uiv, GenVertexArrays, BindVertexArray, GenBuffers, BindBuffer, BufferData uintptr
	VertexAttribPointer, EnableVertexAttribArray, GenFramebuffers, BindFramebuffer    uintptr
	FramebufferTexture2D, GenRenderbuffers, BindRenderbuffer, RenderbufferStorage     uintptr
	FramebufferRenderbuffer, CheckFramebufferStatus, DeleteTextures                   uintptr
	DeleteRenderbuffers, DeleteFramebuffers                                           uintptr
}

type glTarget struct {
	fbo, tex, depth uint32
	w, h            int
}

// GLR is the GPU renderer.
type GLR struct {
	f glFuncs

	progBG, progGeo, progDown, progBlur, progComp, progBlit uint32
	locU, locUI, locVP, locDirRad, locCompP, locCompF       int32
	locBlitP                                                int32

	vaoGeo, vboGeo, vaoEmpty uint32
	scene, comp, out         glTarget
	bloom                    [2]glTarget
	hudTex, cpuTex           uint32
	hudW, hudH, cpuW, cpuH   int

	// buffers handed to GL live here so they are always heap allocated
	params  [nParams * 4]float32
	ui      [4]uint32
	vec     [12]float32
	ivec    [4]int32
	scratch [8]uint32
	srcBuf  []byte
	srcPtr  [1]uintptr
	nameBuf []byte
	logBuf  []byte
	readBuf []uint32

	Renderer, Version string
}

func (g *GLR) c(fn uintptr, args ...uintptr) uintptr { return glcall(fn, args...) }

func p(ptr unsafe.Pointer) uintptr { return uintptr(ptr) }

// InitGL loads entry points through getProc and builds all GPU resources.
// It must be called with a current OpenGL 3.3+ context.
func InitGL(getProc func(string) uintptr) (*GLR, error) {
	g := &GLR{}
	v := reflect.ValueOf(&g.f).Elem()
	var missing []string
	for i := 0; i < v.NumField(); i++ {
		name := "gl" + v.Type().Field(i).Name
		a := getProc(name)
		if a == 0 {
			missing = append(missing, name)
		}
		v.Field(i).SetUint(uint64(a))
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("OpenGL functions missing: %s", strings.Join(missing, ", "))
	}
	g.Renderer = g.str(glRenderer)
	g.Version = g.str(glVersion)
	var maj, min int
	fmt.Sscanf(g.Version, "%d.%d", &maj, &min)
	if maj < 3 || (maj == 3 && min < 3) {
		return nil, fmt.Errorf("OpenGL 3.3 required, driver offers %q (%s)", g.Version, g.Renderer)
	}
	var err error
	if g.progBG, err = g.program(fsVS, bgFS(), "sky/floor"); err != nil {
		return nil, err
	}
	if g.progGeo, err = g.program(geoVS, geoFS, "geometry"); err != nil {
		return nil, err
	}
	if g.progDown, err = g.program(fsVS, downFS, "bloom"); err != nil {
		return nil, err
	}
	if g.progBlur, err = g.program(fsVS, blurFS, "blur"); err != nil {
		return nil, err
	}
	if g.progComp, err = g.program(fsVS, compFS, "composite"); err != nil {
		return nil, err
	}
	if g.progBlit, err = g.program(fsVS, blitFS, "blit"); err != nil {
		return nil, err
	}
	g.locU = g.uniform(g.progBG, "U")
	g.locUI = g.uniform(g.progBG, "UI")
	g.locVP = g.uniform(g.progGeo, "vp")
	g.locDirRad = g.uniform(g.progBlur, "dirRad")
	g.locCompP = g.uniform(g.progComp, "P")
	g.locCompF = g.uniform(g.progComp, "F")
	g.locBlitP = g.uniform(g.progBlit, "P")
	g.c(g.f.UseProgram, uintptr(g.progComp))
	g.c(g.f.Uniform1i, uintptr(g.uniform(g.progComp, "scene")), 0)
	g.c(g.f.Uniform1i, uintptr(g.uniform(g.progComp, "bloomT")), 1)
	g.c(g.f.Uniform1i, uintptr(g.uniform(g.progComp, "hud")), 2)
	for _, pr := range []uint32{g.progDown, g.progBlur, g.progBlit} {
		g.c(g.f.UseProgram, uintptr(pr))
		g.c(g.f.Uniform1i, uintptr(g.uniform(pr, "src")), 0)
	}

	g.c(g.f.GenVertexArrays, 2, p(unsafe.Pointer(&g.scratch[0])))
	g.vaoGeo, g.vaoEmpty = g.scratch[0], g.scratch[1]
	g.c(g.f.GenBuffers, 1, p(unsafe.Pointer(&g.scratch[0])))
	g.vboGeo = g.scratch[0]
	g.c(g.f.BindVertexArray, uintptr(g.vaoGeo))
	g.c(g.f.BindBuffer, glArrayBuffer, uintptr(g.vboGeo))
	g.c(g.f.EnableVertexAttribArray, 0)
	g.c(g.f.EnableVertexAttribArray, 1)
	g.c(g.f.EnableVertexAttribArray, 2)
	g.c(g.f.VertexAttribPointer, 0, 3, glFloat, 0, 36, 0)
	g.c(g.f.VertexAttribPointer, 1, 4, glFloat, 0, 36, 12)
	g.c(g.f.VertexAttribPointer, 2, 2, glFloat, 0, 36, 28)
	g.c(g.f.BindVertexArray, 0)
	g.c(g.f.PixelStorei, glUnpackAlignment, 4)
	g.c(g.f.PixelStorei, glPackAlignment, 4)
	for _, cap := range []uintptr{glCullFace, glScissorTest, glFramebufferSRGB, glMultisample, glDither} {
		g.c(g.f.Disable, cap)
	}
	if e := uint32(g.c(g.f.GetError)); e != 0 {
		return nil, fmt.Errorf("OpenGL setup error 0x%X", e)
	}
	return g, nil
}

func (g *GLR) str(name uintptr) string {
	ptr := g.c(g.f.GetString, name)
	if ptr == 0 {
		return ""
	}
	var b []byte
	for i := uintptr(0); i < 512; i++ {
		a := ptr + i
		ch := **(**byte)(unsafe.Pointer(&a))
		if ch == 0 {
			break
		}
		b = append(b, ch)
	}
	return string(b)
}

func (g *GLR) uniform(prog uint32, name string) int32 {
	g.nameBuf = append(append(g.nameBuf[:0], name...), 0)
	return int32(g.c(g.f.GetUniformLocation, uintptr(prog), p(unsafe.Pointer(&g.nameBuf[0]))))
}

func (g *GLR) infoLog(fn uintptr, obj uint32) string {
	if g.logBuf == nil {
		g.logBuf = make([]byte, 8192)
	}
	g.scratch[0] = 0
	g.c(fn, uintptr(obj), uintptr(len(g.logBuf)), p(unsafe.Pointer(&g.scratch[0])), p(unsafe.Pointer(&g.logBuf[0])))
	n := int(g.scratch[0])
	if n > len(g.logBuf) {
		n = len(g.logBuf)
	}
	return strings.TrimSpace(string(g.logBuf[:n]))
}

func (g *GLR) shader(kind uintptr, src, what string) (uint32, error) {
	sh := uint32(g.c(g.f.CreateShader, kind))
	if sh == 0 {
		return 0, errors.New("glCreateShader failed")
	}
	g.srcBuf = append([]byte(src), 0)
	g.srcPtr[0] = p(unsafe.Pointer(&g.srcBuf[0]))
	g.c(g.f.ShaderSource, uintptr(sh), 1, p(unsafe.Pointer(&g.srcPtr[0])), 0)
	g.c(g.f.CompileShader, uintptr(sh))
	g.scratch[0] = 0
	g.c(g.f.GetShaderiv, uintptr(sh), glCompileStatus, p(unsafe.Pointer(&g.scratch[0])))
	if g.scratch[0] == 0 {
		return 0, fmt.Errorf("%s shader failed to compile:\n%s", what, g.infoLog(g.f.GetShaderInfoLog, sh))
	}
	return sh, nil
}

func (g *GLR) program(vs, fs, what string) (uint32, error) {
	v, err := g.shader(glVertexShader, vs, what+" vertex")
	if err != nil {
		return 0, err
	}
	f, err := g.shader(glFragmentShader, fs, what+" fragment")
	if err != nil {
		return 0, err
	}
	pr := uint32(g.c(g.f.CreateProgram))
	g.c(g.f.AttachShader, uintptr(pr), uintptr(v))
	g.c(g.f.AttachShader, uintptr(pr), uintptr(f))
	g.c(g.f.LinkProgram, uintptr(pr))
	g.scratch[0] = 0
	g.c(g.f.GetProgramiv, uintptr(pr), glLinkStatus, p(unsafe.Pointer(&g.scratch[0])))
	if g.scratch[0] == 0 {
		return 0, fmt.Errorf("%s program failed to link:\n%s", what, g.infoLog(g.f.GetProgramInfoLog, pr))
	}
	return pr, nil
}

func (g *GLR) newTex(w, h int, ifmt, fmt_, typ, filter uintptr) uint32 {
	g.c(g.f.GenTextures, 1, p(unsafe.Pointer(&g.scratch[0])))
	t := g.scratch[0]
	g.c(g.f.BindTexture, glTexture2D, uintptr(t))
	g.c(g.f.TexImage2D, glTexture2D, 0, ifmt, uintptr(w), uintptr(h), 0, fmt_, typ, 0)
	g.c(g.f.TexParameteri, glTexture2D, glTexMinFilter, filter)
	g.c(g.f.TexParameteri, glTexture2D, glTexMagFilter, filter)
	g.c(g.f.TexParameteri, glTexture2D, glTexWrapS, glClampToEdge)
	g.c(g.f.TexParameteri, glTexture2D, glTexWrapT, glClampToEdge)
	return t
}

func (g *GLR) delTex(t *uint32) {
	if *t != 0 {
		g.scratch[0] = *t
		g.c(g.f.DeleteTextures, 1, p(unsafe.Pointer(&g.scratch[0])))
		*t = 0
	}
}

// target (re)creates a framebuffer with one colour texture.
func (g *GLR) target(t *glTarget, w, h int, ifmt, typ, filter uintptr, depth bool) error {
	if t.w == w && t.h == h && t.fbo != 0 {
		return nil
	}
	g.delTex(&t.tex)
	if t.depth != 0 {
		g.scratch[0] = t.depth
		g.c(g.f.DeleteRenderbuffers, 1, p(unsafe.Pointer(&g.scratch[0])))
		t.depth = 0
	}
	if t.fbo == 0 {
		g.c(g.f.GenFramebuffers, 1, p(unsafe.Pointer(&g.scratch[0])))
		t.fbo = g.scratch[0]
	}
	t.w, t.h = w, h
	t.tex = g.newTex(w, h, ifmt, glRGBA, typ, filter)
	g.c(g.f.BindFramebuffer, glFramebuffer, uintptr(t.fbo))
	g.c(g.f.FramebufferTexture2D, glFramebuffer, glColorAttachment0, glTexture2D, uintptr(t.tex), 0)
	if depth {
		g.c(g.f.GenRenderbuffers, 1, p(unsafe.Pointer(&g.scratch[0])))
		t.depth = g.scratch[0]
		g.c(g.f.BindRenderbuffer, glRenderbuffer, uintptr(t.depth))
		g.c(g.f.RenderbufferStorage, glRenderbuffer, glDepthComponent24, uintptr(w), uintptr(h))
		g.c(g.f.FramebufferRenderbuffer, glFramebuffer, glDepthAttachment, glRenderbuffer, uintptr(t.depth))
	}
	if st := uint32(g.c(g.f.CheckFramebufferStatus, glFramebuffer)); st != glFramebufferDone {
		return fmt.Errorf("framebuffer %dx%d incomplete (0x%X)", w, h, st)
	}
	return nil
}

// Offscreen returns an RGBA8 framebuffer to render into (test harness).
func (g *GLR) Offscreen(w, h int) (uint32, error) {
	if err := g.target(&g.out, w, h, glRGBA8, glUnsignedByte, glNearest, false); err != nil {
		return 0, err
	}
	return g.out.fbo, nil
}

func (g *GLR) fullscreen(fbo uint32, w, h int, prog uint32) {
	g.c(g.f.BindFramebuffer, glFramebuffer, uintptr(fbo))
	g.c(g.f.Viewport, 0, 0, uintptr(w), uintptr(h))
	g.c(g.f.UseProgram, uintptr(prog))
}

func (g *GLR) draw3() {
	g.c(g.f.BindVertexArray, uintptr(g.vaoEmpty))
	g.c(g.f.DrawArrays, glTriangles, 0, 3)
}

func (g *GLR) bindTex(unit int, tex uint32) {
	g.c(g.f.ActiveTexture, uintptr(glTexture0+unit))
	g.c(g.f.BindTexture, glTexture2D, uintptr(tex))
}

// Frame renders a prepared Rd (see Rd.Prepare) plus an optional HUD overlay
// (iw*ih pixels, 0x00RRGGBB text or 0xAA000000 for a darkened panel) into
// the framebuffer fbo of size ow x oh.
func (g *GLR) Frame(rd *Rd, hud []uint32, ow, oh int, fbo uint32) error {
	fb := &rd.fb
	iw, ih := fb.W, fb.H
	if err := g.target(&g.scene, iw, ih, glRGBA16F, glHalfFloat, glNearest, true); err != nil {
		return err
	}
	bw, bh := (iw+3)/4, (ih+3)/4
	for i := range g.bloom {
		if err := g.target(&g.bloom[i], bw, bh, glRGBA16F, glHalfFloat, glLinear, false); err != nil {
			return err
		}
	}
	if err := g.target(&g.comp, iw, ih, glRGBA8, glUnsignedByte, glLinear, false); err != nil {
		return err
	}

	// 1. sky + floor
	g.fullscreen(g.scene.fbo, iw, ih, g.progBG)
	g.c(g.f.Disable, glBlend)
	g.c(g.f.Disable, glDepthTest)
	g.c(g.f.DepthMask, glTrueBool)
	g.vec[0] = 1
	g.c(g.f.ClearBufferfv, glDepth, 0, p(unsafe.Pointer(&g.vec[0])))
	g.pack(rd)
	g.c(g.f.Uniform4fv, uintptr(g.locU), nParams, p(unsafe.Pointer(&g.params[0])))
	g.c(g.f.Uniform4uiv, uintptr(g.locUI), 1, p(unsafe.Pointer(&g.ui[0])))
	g.draw3()

	// 2. geometry
	if len(fb.GV) > 0 {
		g.c(g.f.UseProgram, uintptr(g.progGeo))
		g.vec[0], g.vec[1] = float32(iw), float32(ih)
		g.c(g.f.Uniform2fv, uintptr(g.locVP), 1, p(unsafe.Pointer(&g.vec[0])))
		g.c(g.f.BindVertexArray, uintptr(g.vaoGeo))
		g.c(g.f.BindBuffer, glArrayBuffer, uintptr(g.vboGeo))
		g.c(g.f.BufferData, glArrayBuffer, uintptr(len(fb.GV)*4), p(unsafe.Pointer(&fb.GV[0])), glStreamDraw)
		g.c(g.f.Enable, glDepthTest)
		g.c(g.f.DepthFunc, glLEqual)
		g.c(g.f.Enable, glBlend)
		g.c(g.f.BlendFunc, glOne, glOneMinusSrcAlpha)
		for _, r := range fb.Runs {
			if r.Count == 0 {
				continue
			}
			dw := uintptr(0)
			if r.DepthWrite {
				dw = 1
			}
			g.c(g.f.DepthMask, dw)
			g.c(g.f.DrawArrays, glTriangles, uintptr(r.First), uintptr(r.Count))
		}
		g.c(g.f.Disable, glBlend)
		g.c(g.f.Disable, glDepthTest)
		g.c(g.f.DepthMask, glTrueBool)
	}

	// 3. bloom
	g.fullscreen(g.bloom[0].fbo, bw, bh, g.progDown)
	g.bindTex(0, g.scene.tex)
	g.draw3()
	pass := func(src, dst, dir, rad int) {
		g.fullscreen(g.bloom[dst].fbo, bw, bh, g.progBlur)
		g.ivec[0], g.ivec[1] = int32(dir), int32(rad)
		g.c(g.f.Uniform2iv, uintptr(g.locDirRad), 1, p(unsafe.Pointer(&g.ivec[0])))
		g.bindTex(0, g.bloom[src].tex)
		g.draw3()
	}
	pass(0, 1, 0, 2)
	pass(1, 0, 1, 2)
	pass(0, 1, 0, 4)
	pass(1, 0, 1, 4)

	// 4. composite (VHS effects, tone, HUD) at internal resolution
	hasHud := uint32(0)
	if len(hud) >= iw*ih {
		hasHud = 1
		if g.hudTex == 0 || g.hudW != iw || g.hudH != ih {
			g.delTex(&g.hudTex)
			g.hudTex = g.newTex(iw, ih, glRGBA8, glBGRA, glUnsignedByte, glNearest)
			g.hudW, g.hudH = iw, ih
		}
		g.bindTex(2, g.hudTex)
		g.c(g.f.TexSubImage2D, glTexture2D, 0, 0, 0, uintptr(iw), uintptr(ih), glBGRA, glUnsignedByte, p(unsafe.Pointer(&hud[0])))
	}
	g.fullscreen(g.comp.fbo, iw, ih, g.progComp)
	g.bindTex(0, g.scene.tex)
	g.bindTex(1, g.bloom[0].tex)
	if hasHud == 0 {
		g.bindTex(2, g.bloom[0].tex)
	}
	cp, cf := rd.CompParams(hasHud == 1)
	g.vec = cp
	g.c(g.f.Uniform4fv, uintptr(g.locCompP), 3, p(unsafe.Pointer(&g.vec[0])))
	g.scratch[0], g.scratch[1] = cf[0], cf[1]
	g.c(g.f.Uniform4uiv, uintptr(g.locCompF), 1, p(unsafe.Pointer(&g.scratch[0])))
	g.draw3()

	// 5. scale to the output
	g.blit(g.comp.tex, ow, oh, fbo)
	g.bindTex(0, 0)
	if e := uint32(g.c(g.f.GetError)); e != 0 {
		return fmt.Errorf("OpenGL error 0x%X", e)
	}
	return nil
}

func (g *GLR) blit(tex uint32, ow, oh int, fbo uint32) {
	g.fullscreen(fbo, ow, oh, g.progBlit)
	g.bindTex(0, tex)
	g.vec[0], g.vec[1], g.vec[2], g.vec[3] = float32(ow), float32(oh), 0, 0
	g.c(g.f.Uniform4fv, uintptr(g.locBlitP), 1, p(unsafe.Pointer(&g.vec[0])))
	g.draw3()
}

// Blit presents a CPU-rendered frame (iw x ih, 0x00RRGGBB) through GL.
func (g *GLR) Blit(pix []uint32, iw, ih, ow, oh int, fbo uint32) {
	if g.cpuTex == 0 || g.cpuW != iw || g.cpuH != ih {
		g.delTex(&g.cpuTex)
		g.cpuTex = g.newTex(iw, ih, glRGBA8, glBGRA, glUnsignedByte, glLinear)
		g.cpuW, g.cpuH = iw, ih
	}
	g.bindTex(0, g.cpuTex)
	g.c(g.f.TexSubImage2D, glTexture2D, 0, 0, 0, uintptr(iw), uintptr(ih), glBGRA, glUnsignedByte, p(unsafe.Pointer(&pix[0])))
	g.blit(g.cpuTex, ow, oh, fbo)
}

// Read returns the w x h image in fbo, top row first, as 0x00RRGGBB.
func (g *GLR) Read(fbo uint32, w, h int) []uint32 {
	if len(g.readBuf) < w*h {
		g.readBuf = make([]uint32, w*h)
	}
	g.c(g.f.BindFramebuffer, glFramebuffer, uintptr(fbo))
	g.c(g.f.ReadPixels, 0, 0, uintptr(w), uintptr(h), glBGRA, glUnsignedByte, p(unsafe.Pointer(&g.readBuf[0])))
	out := make([]uint32, w*h)
	for y := 0; y < h; y++ {
		copy(out[y*w:(y+1)*w], g.readBuf[(h-1-y)*w:(h-y)*w])
	}
	for i := range out {
		out[i] &= 0xFFFFFF
	}
	return out
}

// ---------------------------------------------------------------------------
// shader parameters

const (
	pTop = iota
	pMid
	pHor
	pSunT
	pSunB
	pLine
	pAcc
	pFloor
	pTile
	pGlow
	pFogC
	pSun
	pSunP
	pMoon
	pMoon2
	pCamP
	pCamR
	pCamU
	pCamF
	pMisc
	pFlr
	pPh
	pFlags
	pBH
	pRingN
	pRingC
	pGalN
	pRings
	pRAmp   = pRings + 6
	pBody   = pRAmp + 2
	pNeb    = pBody + 12
	nParams = pNeb + 16
)

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (g *GLR) pack(rd *Rd) { g.ui = rd.PackParams(g.params[:]) }

// NParams is the number of float4 shader parameters PackParams writes.
const NParams = nParams

// PackParams writes the sky/floor shader parameters (NParams float4s) into P
// and returns the integer parameters. Shared by the OpenGL and Metal renderers.
func (rd *Rd) PackParams(P []float32) [4]uint32 {
	sc, fr := rd.sc, rd.fr
	pl := &sc.P
	P = P[:nParams*4]
	for i := range P {
		P[i] = 0
	}
	set := func(i int, a, b, c, d float32) { P[i*4], P[i*4+1], P[i*4+2], P[i*4+3] = a, b, c, d }
	setc := func(i int, c Col, w float32) { set(i, c.R, c.G, c.B, w) }
	setv := func(i int, v V3, w float32) { set(i, v.X, v.Y, v.Z, w) }
	setc(pTop, pl.Top, 0)
	setc(pMid, pl.Mid, 0)
	setc(pHor, pl.Hor, 0)
	setc(pSunT, pl.SunT, 0)
	setc(pSunB, pl.SunB, 0)
	setc(pLine, pl.Line, 0)
	setc(pAcc, pl.Acc, 0)
	setc(pFloor, pl.Floor, 0)
	setc(pTile, pl.Tile, 0)
	setc(pGlow, rd.glowCol, rd.glowAmt)
	setc(pFogC, rd.fogCol, 0)
	setv(pSun, sc.SunDir, rd.sunR)
	set(pSunP, rd.sunCos, sc.Stripes, rd.scroll, float32(sc.SunKind))
	setv(pMoon, sc.MoonDir, sc.MoonR)
	setv(pMoon2, rd.moon2, float32(sc.MoonKind))
	c := &rd.cam
	setv(pCamP, c.Pos, rd.invf)
	setv(pCamR, c.R, c.cx)
	setv(pCamU, c.U, c.cy)
	setv(pCamF, c.F, rd.pxAng)
	set(pMisc, fr.Kick, rd.T, rd.bt, rd.wave)
	set(pFlr, float32(sc.FloorType), sc.G, sc.LineHalf, sc.Gloss)
	set(pPh, rd.ph1, rd.ph2, rd.ph3, rd.halfFrac)
	set(pFlags, b2f(sc.Space), b2f(sc.FullSky), b2f(sc.BH), sc.Fog)
	nr := min(len(rd.rings), 6)
	set(pBH, rd.bhR, rd.tun, sc.RingW, float32(nr))
	nb := min(len(sc.Bodies), 3)
	setv(pRingN, sc.RingN, float32(nb))
	nn := min(len(sc.Nebula), 8)
	setc(pRingC, sc.RingC, float32(nn))
	setv(pGalN, sc.GalN, 0)
	for i := 0; i < nr; i++ {
		r := &rd.rings[i]
		set(pRings+i, r.x, r.z, r.r, r.w)
		P[(pRAmp+i/4)*4+i%4] = r.amp
	}
	for i := 0; i < nb; i++ {
		b := &sc.Bodies[i]
		k := pBody + i*4
		setv(k, b.Dir, b.R)
		setc(k+1, b.A, b.Haze)
		setc(k+2, b.B, b.Bands)
		set(k+3, b.LX, b.LY, b.Rx, b.Rz)
	}
	for i := 0; i < nn; i++ {
		setv(pNeb+i*2, sc.Nebula[i].Dir, 0)
		setc(pNeb+i*2+1, sc.Nebula[i].C, 0)
	}
	return [4]uint32{sc.Seed, rd.halfBeat, uint32(rd.zCell), fr.N}
}

// CompParams returns the composite (VHS/tone) pass parameters.
func (rd *Rd) CompParams(hasHud bool) ([12]float32, [4]uint32) {
	fx := rd.Effects()
	fr, sc := rd.fr, rd.sc
	h := uint32(0)
	if hasHud {
		h = 1
	}
	return [12]float32{sc.P.Bloom * fx.Bloom, fr.Glitch, fr.Flash, 0, float32(rd.fb.W), float32(rd.fb.H), 0, 0, fx.Scan, fx.Grain, fx.CA, fx.Track},
		[4]uint32{fr.N, h, 0, 0}
}

// ---------------------------------------------------------------------------
// shaders

const fsVS = `#version 330 core
void main(){ vec2 q=vec2(float((gl_VertexID<<1)&2), float(gl_VertexID&2)); gl_Position=vec4(q*2.0-1.0,0.0,1.0); }
`

const glslHash = `
uint hashu(uint a,uint b,uint c){
	uint h=a*0x9E3779B1u ^ (b+0x7F4A7C15u)*0x85EBCA77u ^ (c+0x165667B1u)*0xC2B2AE3Du;
	h^=h>>15u; h*=0x2C1B3C6Du; h^=h>>12u; h*=0x297A2D39u; h^=h>>15u; return h;
}
`

// bgDefs names every sky/floor shader parameter (shared with the Metal header).
var bgDefs = []struct {
	n string
	i int
	s string
}{
	{"TOP", pTop, "rgb"}, {"MIDC", pMid, "rgb"}, {"HOR", pHor, "rgb"}, {"SUNT", pSunT, "rgb"}, {"SUNB", pSunB, "rgb"},
	{"LINEC", pLine, "rgb"}, {"ACC", pAcc, "rgb"}, {"FLOORC", pFloor, "rgb"}, {"TILE", pTile, "rgb"},
	{"GLOWC", pGlow, "rgb"}, {"GLOWAMT", pGlow, "w"}, {"FOGC", pFogC, "rgb"},
	{"SUNDIR", pSun, "xyz"}, {"SUNR", pSun, "w"}, {"SUNCOS", pSunP, "x"}, {"STRIPES", pSunP, "y"}, {"SCROLL", pSunP, "z"}, {"SUNKIND", pSunP, "w"},
	{"MOONDIR", pMoon, "xyz"}, {"MOONR", pMoon, "w"}, {"MOON2", pMoon2, "xyz"}, {"MOONKIND", pMoon2, "w"},
	{"CAMP", pCamP, "xyz"}, {"INVF", pCamP, "w"}, {"CAMR", pCamR, "xyz"}, {"CX", pCamR, "w"},
	{"CAMU", pCamU, "xyz"}, {"CY", pCamU, "w"}, {"CAMF", pCamF, "xyz"}, {"PXANG", pCamF, "w"},
	{"KICK", pMisc, "x"}, {"TIME", pMisc, "y"}, {"BT", pMisc, "z"}, {"WAVE", pMisc, "w"},
	{"FLOORTYPE", pFlr, "x"}, {"GSZ", pFlr, "y"}, {"LINEHALF", pFlr, "z"}, {"GLOSS", pFlr, "w"},
	{"PH1", pPh, "x"}, {"PH2", pPh, "y"}, {"PH3", pPh, "z"}, {"HALFFRAC", pPh, "w"},
	{"FSPACE", pFlags, "x"}, {"FFULL", pFlags, "y"}, {"FBH", pFlags, "z"}, {"FOGMUL", pFlags, "w"},
	{"BHR", pBH, "x"}, {"TUN", pBH, "y"}, {"RINGW", pBH, "z"}, {"NRINGSF", pBH, "w"},
	{"RINGN", pRingN, "xyz"}, {"NBODIESF", pRingN, "w"}, {"RINGC", pRingC, "rgb"}, {"NNEBF", pRingC, "w"},
	{"GALN", pGalN, "xyz"},
}

func bgFS() string {
	var b strings.Builder
	fmt.Fprintf(&b, "#version 330 core\nuniform vec4 U[%d];\nuniform uvec4 UI;\nout vec4 o;\n", nParams)
	for _, d := range bgDefs {
		fmt.Fprintf(&b, "#define %s U[%d].%s\n", d.n, d.i, d.s)
	}
	fmt.Fprintf(&b, "#define RINGS %d\n#define RAMP %d\n#define BODY %d\n#define NEB %d\n#define FARD %d.0\n", pRings, pRAmp, pBody, pNeb, farDist)
	b.WriteString(glslHash)
	b.WriteString(bgBody)
	return b.String()
}

const bgBody = `
#define SPACE (FSPACE>0.5)
#define FULLSKY (FFULL>0.5)
#define BHOLE (FBH>0.5)
float sat(float x){ return clamp(x,0.0,1.0); }

vec3 spaceSky(vec3 c, vec3 d){
	int nn=int(NNEBF);
	for(int i=0;i<nn;i++){
		vec3 bd=U[NEB+2*i].xyz; vec3 bc=U[NEB+2*i+1].rgb;
		float v=dot(d,bd);
		if(v>0.45){
			float t=(v-0.45)/0.55; t*=t;
			float m=0.65+0.35*sin(d.x*11.0+d.z*5.0+float(i))*sin(d.y*9.0-d.z*7.0);
			c+=bc*(t*t*m*0.55);
		}
	}
	if(FULLSKY){
		float s=abs(dot(d,GALN)); float g=1.0-s*4.0;
		if(g>0.0) c+=vec3(0.10,0.09,0.14)*(g*g*(0.7+0.3*sin(d.x*17.0+d.z*13.0)));
	}
	int nb=int(NBODIESF);
	for(int i=0;i<nb;i++){
		vec4 D=U[BODY+i*4]; vec4 A=U[BODY+i*4+1]; vec4 B=U[BODY+i*4+2]; vec4 E=U[BODY+i*4+3];
		float R=D.w; float dt=dot(d,D.xyz);
		if(dt<cos(R*1.05+0.01)) continue;
		float rr=sqrt(max(2.0*(1.0-dt),0.0));
		float a=sat((R-rr)/PXANG+0.5);
		if(a<=0.0) continue;
		float v=(d.y-D.y)/R;
		float u=((d.x-D.x)*E.z+(d.z-D.z)*E.w)/R;
		vec3 col=A.rgb;
		if(B.w>0.0) col=mix(A.rgb,B.rgb,0.5+0.5*sin(v*B.w+sin(u*3.0)*0.5));
		float q=rr/R;
		float sh=clamp(0.55+0.7*(u*E.x+v*E.y),0.2,1.0)*(1.0-0.35*q*q);
		c=mix(c,mix(col*sh,c,A.w),a);
	}
	if(RINGW>0.0 && d.y>0.0){
		float s=abs(dot(d,RINGN))/RINGW;
		if(s<1.0){
			float b=(0.55+0.45*sin(s*46.0))*(1.0-s*s);
			if(s>0.52 && s<0.6) b*=0.15;
			c+=RINGC*(b*0.55*sat(d.y*6.0));
		}
	}
	if(BHOLE){
		float R=BHR; float u=d.x/R, v=(d.y-0.03)/R;
		if(d.z>-0.2){
			float rr=sqrt(u*u+v*v);
			vec3 hot=mix(LINEC,vec3(1.0),0.35);
			float dop=1.0+0.55*clamp(-u/(rr+0.01),-1.0,1.0);
			if(rr>1.0){
				float k=1.0/(1.0+(rr-1.0)*7.0);
				c+=hot*(k*k*1.7*dop*(0.8+0.2*sin(rr*30.0-TIME*4.0)));
			} else {
				c*=sat((rr-0.97)*30.0);
			}
			float au=abs(u);
			if(au<3.4){
				float bow=-0.14*sqrt(1.0-(au/3.4)*(au/3.4));
				float th=0.05+0.03*au;
				float w=1.0-abs(v-bow)/th;
				if(w>0.0){
					float inn=(1.0-(au/3.4)*(au/3.4))*(0.75+0.25*sin(u*13.0-TIME*3.0))*dop;
					c+=mix(LINEC,vec3(1.0),0.5)*(w*w*inn*2.2);
				}
			}
		}
		if(TUN>0.0){
			float rp=sqrt(d.x*d.x+d.y*d.y);
			float band=0.5+0.5*sin((0.9/(rp+0.05)+BT*1.5)*6.283);
			vec3 t=mix(ACC,LINEC,band)*(sat(rp*2.2)*0.75*(0.35+0.65*band*band));
			c=mix(c,t,TUN);
		}
	}
	return c;
}

vec3 sky(vec3 d){
	float e=d.y;
	if(e<0.0) e = FULLSKY ? -d.y : 0.0;
	vec3 c;
	const float midE=0.17;
	if(e<midE) c=mix(HOR,MIDC,e/midE);
	else { float t=(e-midE)/(1.0-midE); c=mix(MIDC,TOP,t*(2.0-t)); }
	if(SPACE) c=spaceSky(c,d);
	if(!FULLSKY){ float g=1.0-e*4.5; if(g>0.0) c+=GLOWC*(g*g*g*GLOWAMT); }
	if(SUNKIND<1.5 || SUNKIND>2.5){
		float dt=dot(d,SUNDIR);
		if(dt>SUNCOS){
			float R=SUNR;
			float rr=sqrt(max(2.0*(1.0-dt),0.0));
			float h=1.0-(rr-R)/(2.3*R);
			if(h>0.0){ h=min(h,1.0); c+=mix(SUNB,SUNT,0.4)*(h*h*(0.2+0.2*KICK)*GLOWAMT*3.0); }
			if(rr<R+PXANG){
				float v=(d.y-SUNDIR.y)/R;
				float a=sat((R-rr)/PXANG+0.5);
				if(SUNKIND<0.5 && v<0.3){
					float q=(0.3-v)/1.3;
					float ph=(v+2.0)*STRIPES*0.5+SCROLL;
					float fr=ph-floor(ph);
					float gap=0.1+0.62*q;
					float w=PXANG/R*STRIPES*0.5;
					a*=sat((fr-gap)/w+0.5)*sat((1.0-fr)/w+0.5);
				}
				vec3 sc2=mix(SUNB,SUNT,sat(v*0.5+0.5))*1.2;
				c=mix(c,sc2,a);
			}
		}
	}
	if(MOONKIND>0.5){
		float dt=dot(d,MOONDIR);
		if(dt>0.98){
			float rr=sqrt(max(2.0*(1.0-dt),0.0));
			if(rr<MOONR+PXANG){
				float a=sat((MOONR-rr)/PXANG+0.5);
				if(MOONKIND<1.5){
					float d2=dot(d,MOON2);
					float r2=sqrt(max(2.0*(1.0-d2),0.0));
					a*=1.0-sat((MOONR*0.92-r2)/PXANG+0.5);
				}
				c=mix(c,mix(SUNT,vec3(1.0),0.6)*1.15,a);
			}
		}
	}
	return c;
}

float gridCov(float h,float invG,float G,float lh,float fp){
	float g=h*invG;
	float d=abs(g-floor(g+0.5))*G;
	float w=max(lh,0.6*fp);
	float cov=sat((w+0.5*fp-d)/fp);
	float k=sat(1.5-1.5*fp*invG);
	return cov*k+0.3*(1.0-k);
}

vec3 spaceFloor(float hx,float hz,float fpz){
	float fade=1.0/(1.0+fpz*0.8);
	int ft=int(FLOORTYPE+0.5);
	if(ft==12){
		float z=hz+TIME*0.6;
		float n=sin(hx*0.045+sin(z*0.03+1.7)*2.2)*sin(z*0.05+sin(hx*0.035)*2.0);
		float n2=sin(hx*0.21+sin(z*0.17)*1.5)*sin(z*0.19+hx*0.05);
		return mix(TILE,FLOORC,sat(0.55+0.4*n+0.18*n2*fade));
	}
	if(ft==13){
		float n=sin(hx*0.31+sin(hz*0.23+PH2)*2.1)*sin(hz*0.27+PH3+sin(hx*0.19)*2.3);
		float g=sat(1.0-abs(n)*7.0); g*=g;
		vec3 rock=mix(TILE,FLOORC,0.5+0.5*sin(hx*1.7+hz*1.3)*fade);
		return rock+vec3(1.9,0.55,0.08)*(g*(0.8+0.6*KICK));
	}
	float n=sin(hx*0.37+sin(hz*0.29+PH1)*1.9)*sin(hz*0.41+PH2+sin(hx*0.23)*1.7);
	float n2=sin(hx*2.9+sin(hz*2.3+PH3)*1.3)*sin(hz*3.1+hx*0.7+PH2);
	return mix(TILE,FLOORC,sat(0.6+0.3*n+0.22*n2*fade));
}

vec3 floorC(vec3 d){
	float t=CAMP.y/-d.y;
	float fog=t*(1.0/FARD)*FOGMUL;
	if(fog>=1.0) return FOGC;
	fog*=fog;
	float hx=CAMP.x+d.x*t, hz=CAMP.z+d.z*t;
	float k=t/d.y;
	float tpx=-k*CAMR.y*INVF, tpy=k*CAMU.y*INVF;
	float fpx=abs(CAMR.x*INVF*t+d.x*tpx)+abs(-CAMU.x*INVF*t+d.x*tpy)+1e-6;
	float fpz=abs(CAMR.z*INVF*t+d.z*tpx)+abs(-CAMU.z*INVF*t+d.z*tpy)+1e-6;
	float inv=1.0/length(d);
	float ny=-d.y*inv;
	float G=GSZ, invG=1.0/G;
	int ft=int(FLOORTYPE+0.5);
	vec3 c;
	vec3 refl=vec3(d.x*inv,ny,d.z*inv);
	if(ft>=11){
		c=spaceFloor(hx,hz,fpz);
		if(GLOSS>0.0){ float f=1.0-ny; c+=sky(refl)*(GLOSS*f*f); }
	} else if(ft==1){
		float ux=hx*invG*0.5, uz=hz*invG*0.5;
		float tx=abs(fract(ux)-0.5)*4.0-1.0;
		float tz=abs(fract(uz)-0.5)*4.0-1.0;
		float s=clamp(tx*G*0.5/fpx,-1.0,1.0)*clamp(tz*G*0.5/fpz,-1.0,1.0);
		c=mix(FLOORC,TILE*0.8,0.5+0.5*s);
		if(fpz<G){
			int ix=int(floor(hx*invG+0.5));
			int iz=int(floor(hz*invG+0.5))+int(UI.z);
			if((hashu(uint(ix),uint(iz),UI.y+UI.x)&31u)==0u)
				c+=ACC*(abs(s)*(1.0-HALFFRAC)*(1.0-fpz*invG)*0.9);
		}
		if(GLOSS>0.0){ float f=1.0-ny; c+=sky(refl)*(GLOSS*(0.05+0.6*f*f)); }
	} else if(ft==2){
		float att=1.0/(1.0+fpz*1.2);
		float w1=sin(hx*0.9+hz*0.35+TIME*1.3+PH1);
		float w2=sin(hz*1.7-TIME*1.9+hx*0.2+PH2);
		float w3=sin((hx+hz)*2.3+TIME*2.4+PH3);
		float nx=(w1*0.04+w3*0.02)*att;
		float nz=(w2*0.06+w3*0.02)*att;
		vec3 r=vec3(d.x*inv+nx, max(ny+nz*0.6,0.004), d.z*inv+nz*0.3);
		float f=1.0-ny; f=0.3+0.7*f*f*f;
		c=FLOORC*0.55+sky(normalize(r))*(f*GLOSS);
		float g=max(gridCov(hx,invG*0.25,G*4.0,LINEHALF,fpx), gridCov(hz,invG*0.25,G*4.0,LINEHALF,fpz));
		c+=LINEC*(g*0.3);
	} else {
		float gx=gridCov(hx,invG,G,LINEHALF,fpx);
		float gz=gridCov(hz,invG,G,LINEHALF,fpz);
		float boost=1.0+0.9*KICK;
		float wd=abs(hz-CAMP.z-WAVE)*(1.0/7.0);
		if(wd<1.0) boost+=(1.0-wd)*0.9;
		gz*=boost;
		gx=max(gx,gz);
		c=FLOORC+LINEC*(gx*1.45);
		if(GLOSS>0.0){ float f=1.0-ny; c+=sky(refl)*(GLOSS*(0.12+0.88*f*f)); }
	}
	int nr=int(NRINGSF);
	for(int i=0;i<nr;i++){
		vec4 r=U[RINGS+i];
		float amp=U[RAMP+i/4][i%4];
		float ex=hx-r.x, ez=hz-r.y;
		float u=(sqrt(ex*ex+ez*ez)-r.z)/r.w;
		if(u>-1.0 && u<1.0){ u=1.0-u*u; c+=mix(ACC,vec3(1.0),0.4)*(u*u*amp*1.6); }
	}
	return mix(c,FOGC,fog);
}

void main(){
	float px=gl_FragCoord.x-0.5, py=gl_FragCoord.y-0.5;
	float a=(px-CX)*INVF, b=(CY-py)*INVF;
	vec3 d=CAMF+CAMU*b+CAMR*a;
	vec3 c;
	if(d.y>=0.0 || FULLSKY) c=sky(normalize(d)); else c=floorC(d);
	o=vec4(c,1.0);
}
`

const geoVS = `#version 330 core
layout(location=0) in vec3 pos;
layout(location=1) in vec4 col;
layout(location=2) in vec2 dh;
uniform vec2 vp;
out vec4 vcol;
out vec2 vdh;
void main(){
	gl_Position=vec4((pos.x+0.5)/vp.x*2.0-1.0, (pos.y+0.5)/vp.y*2.0-1.0, 1.0-2.0*min(pos.z*0.3,1.0), 1.0);
	vcol=col; vdh=dh;
}
`

const geoFS = `#version 330 core
in vec4 vcol;
in vec2 vdh;
out vec4 o;
void main(){
	float cov=clamp(vdh.y+0.5-abs(vdh.x),0.0,1.0);
	o=vec4(vcol.rgb*cov, vcol.a*cov);
}
`

const downFS = `#version 330 core
uniform sampler2D src;
out vec4 o;
void main(){
	ivec2 b=ivec2(gl_FragCoord.xy);
	ivec2 sz=textureSize(src,0);
	vec3 s=vec3(0.0); int n=0;
	for(int y=0;y<4;y++) for(int x=0;x<4;x++){
		ivec2 q=b*4+ivec2(x,y);
		if(q.x<sz.x && q.y<sz.y){ s+=max(texelFetch(src,q,0).rgb-0.62,vec3(0.0)); n++; }
	}
	o=vec4(s/float(max(n,1)),1.0);
}
`

const blurFS = `#version 330 core
uniform sampler2D src;
uniform ivec2 dirRad;
out vec4 o;
void main(){
	ivec2 q=ivec2(gl_FragCoord.xy);
	ivec2 sz=textureSize(src,0);
	ivec2 dd=dirRad.x==0 ? ivec2(1,0) : ivec2(0,1);
	int r=dirRad.y;
	vec3 s=vec3(0.0);
	for(int k=-r;k<=r;k++) s+=texelFetch(src,clamp(q+dd*k,ivec2(0),sz-1),0).rgb;
	o=vec4(s/float(2*r+1),1.0);
}
`

const compFS = `#version 330 core
uniform sampler2D scene;
uniform sampler2D bloomT;
uniform sampler2D hud;
uniform vec4 P[3];
uniform uvec4 F;
out vec4 o;
` + glslHash + `
float tone(float x){
	if(x<=0.0) return 0.0;
	if(x>0.75) x=0.75+0.25*(1.0-1.0/(1.0+(x-0.75)*4.0));
	return min(x,1.0);
}
void main(){
	int W=int(P[1].x), H=int(P[1].y);
	int x=int(gl_FragCoord.x), y=int(gl_FragCoord.y);
	uint frame=F.x;
	float glitch=P[0].y;
	int off=0;
	if(glitch>0.0){
		uint band=hashu(uint(y/(6+int(frame%5u)*3)),frame/2u,5u);
		if(float(band&1023u)/1023.0<glitch*0.8)
			off=int((float((band>>10u)&1023u)/1023.0-0.5)*glitch*float(W)*0.35);
	}
	int ca=int(P[2].z)+int(glitch*6.0);
	float sl=1.0;
	if(y%3==2) sl=1.0-0.16*P[2].x;
	float tb=float((int(frame)*2+H-y)%(H*2))/float(H);
	if(tb<0.06) sl*=1.0+0.08*P[2].w;
	int sx=x+off;
	if(sx<0) sx+=W; else if(sx>=W) sx-=W;
	int xr=max(sx-ca,0), xb=min(sx+ca,W-1);
	vec3 c=vec3(texelFetch(scene,ivec2(xr,y),0).r, texelFetch(scene,ivec2(sx,y),0).g, texelFetch(scene,ivec2(xb,y),0).b);
	vec2 buv=vec2((float(sx)+0.5)*0.25,(float(y)+0.5)*0.25)/vec2(textureSize(bloomT,0));
	c+=texture(bloomT,buv).rgb*P[0].x;
	float vx=float(x)/float(W)-0.5, vy=float(y)/float(H)-0.5;
	float v=(1.0-0.55*(vx*vx+vy*vy))*sl;
	uint rs=hashu(uint(x),uint(y),frame*7u+99u);
	float gr=(float(rs>>24u)/255.0-0.5)*(0.035*P[2].y+glitch*0.25)+P[0].z;
	c=c*v+gr;
	c=vec3(tone(c.r),tone(c.g),tone(c.b));
	if(F.y!=0u){
		vec4 h=texelFetch(hud,ivec2(x,y),0);
		if(h.r+h.g+h.b>0.0) c=h.rgb; else c*=1.0-h.a;
	}
	o=vec4(c,1.0);
}
`

const blitFS = `#version 330 core
uniform sampler2D src;
uniform vec4 P;
out vec4 o;
void main(){
	vec2 uv=vec2(gl_FragCoord.x/P.x, 1.0-gl_FragCoord.y/P.y);
	o=vec4(texture(src,uv).rgb,1.0);
}
`
