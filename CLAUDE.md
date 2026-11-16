# Working on Vaporwave Endless 3D: `ios` branch

This branch is the native iOS app. The Windows program is on `main`; they are maintained separately.

## Git workflow (the owner's standing instruction)

- **Always commit and push** every change when the work is done; don't wait to be asked.
- iOS work is committed and pushed **directly to the `ios` branch**; Windows work goes directly to `main`. No pull requests.
- Commit author email must be `johnwalkr@users.noreply.github.com`; GitHub rejects pushes that expose a private address.

## Layout

- Root `*.go`: the shared core as `package vaporwave` (on `main` it is `package main`). `mobile.go` is the iOS facade, `capi/` the C API built with `-buildmode=c-archive`.
- `ios/`: Swift app, generated `Shaders.metal`, XcodeGen `project.yml`, `build-go.sh`, `setup.sh`.

## Building and checking

- `go vet ./...` and `go test -run 'TestMetalShaders|TestMobileFacade' .` (fast). `go test .` runs everything (about 2 minutes).
- `Shaders.metal` is generated from the GLSL in `gl.go`: after changing shaders run `UPDATE_SHADERS=1 go test -run TestMetalShaders .`
- C API check on Linux: `go build -buildmode=c-archive -o /tmp/libve3d.a ./capi`.
- Swift and Metal can't be compiled here; the GitHub Actions workflow builds the app for the simulator on macOS. Ask the owner to build in Xcode for anything that needs a device.
- Linux harness: `go build -o ve3d_test ./cmd/ve3d-test && ./ve3d_test -h`; `-gpu` compares OpenGL with the CPU renderer.
