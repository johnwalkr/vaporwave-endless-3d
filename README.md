# Vaporwave Endless 3D

A small Windows program that generates endless 3D vaporwave and space-travel scenes, each with its own procedurally synthesized soundtrack locked to the visuals. Press **Tab** for a new scene and track.

It is a single ~2.5 MB `.exe` with no installer, no runtime and no external assets: every image and every sound is generated in code while it runs.

---

## Controls

| Input | Effect |
|---|---|
| Tab | New scene + new track |
| Space | Switch between vaporwave and space travel |
| W / S (or ↑ / ↓) | Tape speed: music and travel slow down or speed up together |
| A / D (or ← / →) | Strafe |
| Mouse | Look around |
| Left click | Shockwave on the floor + a chime in key |
| Right click | Recolour the current vaporwave scene |
| Mouse wheel | Altitude |
| O or F1 | Settings menu (audio + video) |
| G | GPU renderer on/off |
| F / F11 / Alt+Enter | Fullscreen |
| F2 | FPS counter on/off (frame rate, frame time, render resolution) |
| R / M / P / H | Reset / mute / screenshot / help |
| Esc | Quit |

Command-line options: `-cpu` forces the software renderer, and `-selftest` runs a short automated pass that writes `vaporwave-endless-3d_selftest.txt` and screenshots next to the exe.

### Music settings (in the settings menu)

| Setting | Options |
|---|---|
| Style | Auto (random per scene), Vaporwave, Smooth jazz, Soft rock, Metal, Pop, Future funk, Lo-fi hip hop, Synthwave |
| Drums | Off, Chill, Normal, Busy (ghost notes, 16th hats, fills every 4 bars), Intense (double-time kicks, crashes, fills every 2 bars, no breakdowns) |
| Tempo | Slower (−18%), Normal, Faster (+18%), on top of each style's own tempo range |
| Mood | Auto, Bright (major), Dark (minor) |
| Melody | Off, Sparse, Normal, Busy |

Changing any of these remixes the current track immediately; "New track for this scene" rolls a fresh song with the same settings. Every style keeps the vaporwave core: detuned chorus pads, tape wobble, reverb and echo, vinyl crackle and slowed-down tempos. On top of that, each style has its own drum kit and groove, bass instrument, chord instrument and lead sound:

| Style | Groove | Bass | Chords | Lead |
|---|---|---|---|---|
| Vaporwave | laid-back, half-time options | sub bass | lush pads + arpeggios | FM bell / soft saw / sine |
| Smooth jazz | swung ride cymbal, brushes, feathered kick | walking upright bass | Rhodes comping (ii–V–I, 9ths, 13ths) | sax |
| Soft rock | backbeat, crashes | picked saw bass | distorted double-tracked power chords | overdriven guitar |
| Metal | gallops and double-kick, tight snare | low saw bass | heavily distorted palm-muted chugs | shred guitar (harmonic minor) |
| Pop | four-on-the-floor, claps | octave bounce | plucked chord stabs, add9 chords | square pluck |
| Future funk | four-on-the-floor, open-hat offbeats, light 16th swing | slap bass | chopped filter pads + stabs | bell / pluck |
| Lo-fi hip hop | swung boom-bap | long sub notes | Rhodes chords | soft sine / bell |
| Synthwave | gated-reverb snare | driving 8th-note synth bass | 16th arpeggiator + big pads | saw lead |

Settings are saved to `vaporwave-endless-3d_settings.ini` next to the exe (or `%APPDATA%\Vaporwave Endless 3D\` if that folder isn't writable).

---

## How it was made

Vaporwave Endless 3D was written in Go with Claude (Anthropic's AI model) in a single conversation. It was built and tested on Linux and cross-compiled for Windows. It is written against the raw Win32 API through Go's `syscall` package, so it has **no dependencies outside the Go standard library** and needs no C compiler for Windows.

### Architecture

```
            ┌──────────── Song (seeded) ────────────┐
            │ key, tempo, chords, drum/bass/arp/     │
            │ lead patterns, all queryable by beat   │
            └───────┬───────────────────────┬────────┘
                    │                       │
              Synth / Engine           Scene + Frame
        (audio thread, waveOut)   (kick/snare/melody envelopes
                    │               read at the *heard* beat)
                    │                       │
   playback position ──► beat ──► camera Z, pulses, flashes
                                            │
                           ┌────────────────┴───────────────┐
                     CPU renderer                     GPU renderer
                 (software rasterizer)              (OpenGL 3.3)
```

**Audio is the clock.** The engine records which song sample went into each output buffer. Every frame, the program asks Windows how far playback has actually got (`waveOutGetPosition`), converts that to a beat position, and places the camera there. That's why grid lines, gates and prop rows pass exactly on the beat, and why the tape-speed control (which resamples the audio) slows the visuals by the same amount.

### Source files

| File | What it does |
|---|---|
| `song.go` | Deterministic, style-aware song generator: eight styles, each with its own tempo range, chord progressions (7ths through 13ths), drum grooves with swing, bass lines (including walking bass), chord comping and lead instrument; drum intensity, tempo, mood and melody-density options; fills, crashes and breakdowns. Exposes per-beat envelopes so visuals can react. |
| `synth.go` | The synthesizer: detuned PolyBLEP saw pads (optionally chopped), FM Rhodes, plucks and bells, four bass instruments (sub, upright, filtered saw, slap), double-tracked distorted guitar, mono leads (saw, sine, sax, overdriven guitar, square pluck), three kicks, four snares (incl. brushes and gated reverb), clap, hats, ride and crash from an inharmonic oscillator bank, ping-pong delay, Freeverb-style reverb, tape hiss and vinyl crackle, sidechain ducking. `Engine` handles variable-speed playback, wow/flutter, cross-fades between songs, and the sample→beat map. |
| `settings.go` | Saved settings, audio presets, the mix model and post-processing (FX) settings. |
| `scene.go` | Vaporwave scene generator: 10 base palettes with hue variation, 4 floor types, sun/moon/stars, skyline backdrops, prop avenues, neon gates. Also the player and per-frame state. |
| `space.go` | Space-travel mode: deep space, black holes (accretion disc, swallow, wormhole), all eight solar-system planets and eight original worlds, plus rocks, pines, ice spikes and towers. |
| `raster.go` | Software rasterizer: triangles and antialiased lines recorded as a display list, then drawn in parallel row bands; HDR framebuffer; bloom, chromatic aberration, scanlines, tracking bar, grain and VHS glitches. In GPU mode the same code records vertices instead. |
| `render.go` | Camera, per-pixel sky and floor shading (grid, checkerboard, water reflections, valley), backdrops, terrain. |
| `props.go` | Palms, marble columns, crystals, wireframe spheres and cubes, Windows-95 panels, neon gates. |
| `gl.go` | GPU renderer: sky/floor and all post-processing as GLSL shaders, with geometry from the shared display list. Every GL call goes through integer/pointer arguments only, so the same code runs via `syscall` on Windows and via a small cgo trampoline in Linux tests. |
| `gl_windows.go` | OpenGL context creation (WGL) in a child window, VSync, function loading. |
| `main_windows.go` | The Windows program: window, input, settings menu, HUD (GDI text), audio devices and latency, renderer switching, screenshots, self-test. |
| `main_other.go` | Non-Windows test harness: renders scenes to PNG and audio to WAV, prints timing and audio statistics, and compares the GPU renderer against the CPU renderer. |
| `gl_cgo_linux.go`, `gl_stub.go`, `hud_gl_test.go` | Headless OpenGL (Mesa/EGL) backend and tests for the harness. |

### How it was tested

Without a Windows machine, everything except the Win32 layer was tested on Linux:

- **Visuals:** the harness rendered still frames of every scene type, which were inspected for errors. The software renderer runs at about 11–25 ms per 800×600 frame on two cores.
- **GPU renderer:** run headlessly on Mesa's llvmpipe and compared pixel-by-pixel against the CPU renderer across 28 scenes. The average difference was about 2.5/255, mostly film grain, and at most 0.05% of pixels differed by more than 40/255. A unit test checks that the HUD/menu overlay is drawn the right way up and darkens correctly.
- **Audio:** each preset was rendered to WAV and measured for peak level, clipping, spectral brightness and stereo width. The synth uses about 2% of one core.

---

## Compiling for Windows

You need **Go 1.24 or newer** (<https://go.dev/dl/>). No C compiler, SDK or extra packages are required.

### On Windows

```bat
cd vaporwave-endless-3d
go build -ldflags="-s -w -H windowsgui" -o vaporwave-endless-3d.exe .
```

### From Linux or macOS (cross-compile)

```sh
cd vaporwave-endless-3d
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o vaporwave-endless-3d.exe .
```

For Windows on ARM, use `GOARCH=arm64`.

Build flags:

- `-H windowsgui` makes it a GUI program (no console window). Leave it out if you want a console for debugging output.
- `-s -w` strips debug symbols, roughly halving the file size.

Requirements at runtime: Windows 7 or newer, 64-bit. GPU mode needs an OpenGL 3.3 driver (any GPU from about 2010 onward); without one, the program falls back to the CPU renderer.

### Running the test harness (Linux)

```sh
go build -o ve3d_test .
./ve3d_test -seed 1 -n 4 -out /tmp/frames             # render 4 vaporwave scenes to PNG
./ve3d_test -space -seed 0 -n 20 -out /tmp/space      # all 20 space destinations
./ve3d_test -seed 5 -n 1 -audio 30 -preset 2          # 30 s of audio with the Mallsoft preset -> WAV
./ve3d_test -gpu -seed 1 -n 8                         # compare GPU vs CPU (needs cgo + Mesa EGL)
go test .                                              # GPU HUD overlay test
```

---

## Notes

- Scene and planet names are generated or written for this project. The eight non-solar-system worlds are original designs.
- Screenshots (P) are saved next to the exe as `vaporwave-endless-3d_<seed>_<n>.png`.
- If the program crashes, it writes `vaporwave-endless-3d_crash.txt` next to the exe.
