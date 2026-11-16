//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	opengl32 = syscall.NewLazyDLL("opengl32.dll")

	pWglCreateContext  = opengl32.NewProc("wglCreateContext")
	pWglDeleteContext  = opengl32.NewProc("wglDeleteContext")
	pWglMakeCurrent    = opengl32.NewProc("wglMakeCurrent")
	pWglGetProcAddress = opengl32.NewProc("wglGetProcAddress")

	pChoosePixelFormat = gdi32.NewProc("ChoosePixelFormat")
	pSetPixelFormat    = gdi32.NewProc("SetPixelFormat")
	pSwapBuffers       = gdi32.NewProc("SwapBuffers")
	pReleaseDC         = user32.NewProc("ReleaseDC")
	pMoveWindow        = user32.NewProc("MoveWindow")
)

func glcall(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

func wglProc(name string) uintptr {
	b := append([]byte(name), 0)
	a, _, _ := pWglGetProcAddress.Call(uintptr(unsafe.Pointer(&b[0])))
	// some drivers return small sentinel values instead of NULL
	if a <= 3 || a == ^uintptr(0) {
		p := opengl32.NewProc(name)
		if p.Find() != nil {
			return 0
		}
		return p.Addr()
	}
	return a
}

// GLWindow is a child window that fills the main window and owns the GL context.
type GLWindow struct {
	hwnd, hdc, ctx uintptr
	swapInterval   uintptr
	w, h           int
	R              *GLR
}

var glClassDone bool

func newGLWindow(parent, hinst, wndproc uintptr, w, h int) (gw *GLWindow, err error) {
	gw = &GLWindow{w: w, h: h}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("OpenGL driver crashed during setup: %v", r)
		}
		if err != nil {
			gw.Close()
			gw = nil
		}
	}()
	cls := u16("VaporwaveEndless3DGL")
	if !glClassDone {
		wc := wndClassEx{style: 0x0020, lpfnWndProc: wndproc, hInstance: hinst, lpszClassName: cls}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			return nil, fmt.Errorf("RegisterClassEx: %v", e)
		}
		glClassDone = true
	}
	const wsChild, wsVisible, wsClipSiblings = 0x40000000, 0x10000000, 0x04000000
	hwnd, _, e := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, wsChild|wsVisible|wsClipSiblings,
		0, 0, uintptr(w), uintptr(h), parent, 0, hinst, 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx (GL): %v", e)
	}
	gw.hwnd = hwnd
	gw.hdc, _, _ = pGetDC.Call(hwnd)
	// PIXELFORMATDESCRIPTOR
	var pfd [40]byte
	*(*uint16)(unsafe.Pointer(&pfd[0])) = 40
	*(*uint16)(unsafe.Pointer(&pfd[2])) = 1
	*(*uint32)(unsafe.Pointer(&pfd[4])) = 0x4 | 0x20 | 0x1 // DRAW_TO_WINDOW | SUPPORT_OPENGL | DOUBLEBUFFER
	pfd[8] = 0                                             // RGBA
	pfd[9] = 32                                            // colour bits
	pfd[23] = 24                                           // depth bits
	pfd[24] = 8                                            // stencil bits
	pf, _, _ := pChoosePixelFormat.Call(gw.hdc, uintptr(unsafe.Pointer(&pfd[0])))
	if pf == 0 {
		return nil, errors.New("no suitable OpenGL pixel format")
	}
	if r, _, e := pSetPixelFormat.Call(gw.hdc, pf, uintptr(unsafe.Pointer(&pfd[0]))); r == 0 {
		return nil, fmt.Errorf("SetPixelFormat: %v", e)
	}
	gw.ctx, _, e = pWglCreateContext.Call(gw.hdc)
	if gw.ctx == 0 {
		return nil, fmt.Errorf("wglCreateContext: %v", e)
	}
	if r, _, e := pWglMakeCurrent.Call(gw.hdc, gw.ctx); r == 0 {
		return nil, fmt.Errorf("wglMakeCurrent: %v", e)
	}
	gw.swapInterval = wglProc("wglSwapIntervalEXT")
	gw.R, err = InitGL(wglProc)
	return gw, err
}

func (gw *GLWindow) SetVSync(on bool) bool {
	if gw.swapInterval == 0 {
		return false
	}
	v := uintptr(0)
	if on {
		v = 1
	}
	glcall(gw.swapInterval, v)
	return true
}

func (gw *GLWindow) Resize(w, h int) {
	if w != gw.w || h != gw.h {
		pMoveWindow.Call(gw.hwnd, 0, 0, uintptr(w), uintptr(h), 1)
		gw.w, gw.h = w, h
	}
}

func (gw *GLWindow) Swap() { pSwapBuffers.Call(gw.hdc) }

func (gw *GLWindow) Close() {
	if gw == nil {
		return
	}
	if gw.ctx != 0 {
		pWglMakeCurrent.Call(0, 0)
		pWglDeleteContext.Call(gw.ctx)
		gw.ctx = 0
	}
	if gw.hdc != 0 {
		pReleaseDC.Call(gw.hwnd, gw.hdc)
		gw.hdc = 0
	}
	if gw.hwnd != 0 {
		pDestroyWindow.Call(gw.hwnd)
		gw.hwnd = 0
	}
}
