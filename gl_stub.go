//go:build !windows && !(linux && cgo)

package vaporwave

import "errors"

func glcall(fn uintptr, args ...uintptr) uintptr { panic("no OpenGL backend") }

func HeadlessGL() (*GLR, error) { return nil, errors.New("built without cgo: no OpenGL test backend") }
