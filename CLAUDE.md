# Working on Vaporwave Endless 3D: `quest` branch

This branch is the Meta Quest (WebXR) version. The Windows program is on `main`, the iPhone app on `ios`; they are maintained separately.

## Git workflow (the owner's standing instruction)

- **Always commit and push** every change when the work is done; don't wait to be asked.
- Quest work is committed and pushed **directly to the `quest` branch** (iOS → `ios`, Windows → `main`). No pull requests.
- Commit author email must be `johnwalkr@users.noreply.github.com`; GitHub rejects pushes that expose a private address.
- Pushing to `quest` triggers GitHub Actions, which builds `web/dist` and publishes it to the `gh-pages` branch (GitHub Pages).

## Layout

- Root `*.go`: shared core as `package vaporwave`. `mobile.go` is the app facade (used by iOS and the web), `render.go` `EyeView` is the VR camera, `webgl.go` converts the GL shaders to WebGL 2.
- `cmd/webxr`: WebAssembly entry point (`GOOS=js GOARCH=wasm`). `web/`: index.html, main.js (WebGL 2 renderer, WebXR, audio worklet), build.sh.
- `ios/`, `capi/`, `metalparams.go` come from the `ios` branch and are kept so merges stay simple.

## Building and checking

- `go vet ./...`, `GOOS=js GOARCH=wasm go vet ./cmd/webxr`, `go test -run 'TestMobileFacade|TestMetalShaders' .`
- `web/build.sh`, then serve `web/dist` and load it in headless Chrome (`google-chrome --headless=new --use-angle=swiftshader --enable-unsafe-swiftshader`) through the DevTools protocol to read console errors and take screenshots. `?autostart` skips the start page; `?eyetest` renders a fake stereo headset.
- WebXR itself can only be tested on the headset; ask the owner.
