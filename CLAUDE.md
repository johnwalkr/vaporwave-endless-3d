# Working on Vaporwave Endless 3D

## Git workflow (the owner's standing instruction)

- **Always commit and push** every change when the work is done; don't wait to be asked.
- Commit and push **directly to `main`**: no feature branches or pull requests.
- Commit author email must be `johnwalkr@users.noreply.github.com`; GitHub rejects pushes that expose a private address.

## Building and checking

- Windows build (no C compiler needed): `GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o vaporwave-endless-3d.exe .`
- Vet both targets before committing: `go vet .` and `GOOS=windows GOARCH=amd64 go vet .`
- The Linux test harness renders frames/audio (`go build -o ve3d_test . && ./ve3d_test -h`); `-game` plays a full game with an autopilot. `-gpu` compares the OpenGL renderer with the CPU renderer (needs cgo + Mesa).
- `go test .` runs the audio, GPU and game tests (about 2 minutes).
- The Windows window/input layer can't be run here; ask the owner to try `vaporwave-endless-3d.exe -selftest` if something needs checking on Windows.
