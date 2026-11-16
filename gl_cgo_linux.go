//go:build linux && cgo

package vaporwave

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
typedef uintptr_t U;
typedef U (*F15)(U,U,U,U,U,U,U,U,U,U,U,U,U,U,U);
static U gl_call(U fn,U a0,U a1,U a2,U a3,U a4,U a5,U a6,U a7,U a8,U a9,U a10,U a11,U a12,U a13,U a14){
	return ((F15)fn)(a0,a1,a2,a3,a4,a5,a6,a7,a8,a9,a10,a11,a12,a13,a14);
}
typedef void* (*gpa_t)(const char*);
static gpa_t gpa;
static const char* egl_init(void){
	setenv("EGL_PLATFORM","surfaceless",0);
	void* h=dlopen("libEGL.so.1",RTLD_NOW);
	if(!h) return "cannot load libEGL.so.1";
	gpa=(gpa_t)dlsym(h,"eglGetProcAddress");
	typedef void* (*gd_t)(void*); typedef unsigned (*init_t)(void*,int*,int*);
	typedef unsigned (*bind_t)(unsigned); typedef void* (*cc_t)(void*,void*,void*,const int*);
	typedef unsigned (*mc_t)(void*,void*,void*,void*);
	void* d=((gd_t)dlsym(h,"eglGetDisplay"))((void*)0);
	int ma,mi;
	if(!((init_t)dlsym(h,"eglInitialize"))(d,&ma,&mi)) return "eglInitialize failed";
	if(!((bind_t)dlsym(h,"eglBindAPI"))(0x30A2)) return "eglBindAPI failed";
	int ctxa[]={0x3098,3,0x30FB,3,0x30FD,1,0x3038};
	void* c=((cc_t)dlsym(h,"eglCreateContext"))(d,0,0,ctxa);
	if(!c) return "eglCreateContext failed";
	if(!((mc_t)dlsym(h,"eglMakeCurrent"))(d,0,0,c)) return "eglMakeCurrent failed";
	return 0;
}
static U egl_proc(const char* n){ return (U)gpa(n); }
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

// GL contexts are bound to one OS thread.
func init() { runtime.LockOSThread() }

func glcall(fn uintptr, args ...uintptr) uintptr {
	var a [15]uintptr
	copy(a[:], args)
	return uintptr(C.gl_call(C.U(fn), C.U(a[0]), C.U(a[1]), C.U(a[2]), C.U(a[3]), C.U(a[4]), C.U(a[5]), C.U(a[6]),
		C.U(a[7]), C.U(a[8]), C.U(a[9]), C.U(a[10]), C.U(a[11]), C.U(a[12]), C.U(a[13]), C.U(a[14])))
}

// HeadlessGL creates an offscreen OpenGL 3.3 core context (Mesa) for tests.
func HeadlessGL() (*GLR, error) {
	if e := C.egl_init(); e != nil {
		return nil, errors.New(C.GoString(e))
	}
	return InitGL(func(n string) uintptr {
		cs := C.CString(n)
		defer C.free(unsafe.Pointer(cs))
		return uintptr(C.egl_proc(cs))
	})
}
