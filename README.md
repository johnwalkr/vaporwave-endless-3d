# Vaporwave Endless 3D for iPhone and iPad

> **This is the `ios` branch.** It holds the native iOS app. The Windows program lives on the `main` branch; the two are maintained separately (see "Branches" below).

A native iOS app that generates endless 3D vaporwave and space-travel scenes, each with its own procedurally synthesized soundtrack locked to the visuals. Everything is rendered on the GPU with Metal; every image and every sound is generated in code while it runs, with no assets.

---

## Controls

| Touch | Effect |
|---|---|
| Drag (one finger) | Look around |
| Tap | Shockwave across the floor + a chime in key |
| Double-tap | New scene + new track |
| Touch and hold | Recolour the current vaporwave scene |
| Two-finger drag left / right | Slide sideways |
| Two-finger drag up / down | Tape speed: music and travel speed up or slow down together |
| Pinch | Altitude |
| ⤨ button | New scene + new track |
| ✦ button | Switch between vaporwave and space travel |
| Sliders button | Settings (music, audio mix, render scale, bloom, VHS effects, FPS counter) |

The app runs in landscape. **Render scale** in settings trades sharpness for battery and frame rate: 67% is the default, 100% renders at the screen's full resolution.

### Music settings

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


Settings are saved in the app's Documents folder.

---

## How it was made

Vaporwave Endless 3D was written with Claude (Anthropic's AI model). The scene, music and camera code is shared Go (the same code as the Windows version); iOS gets it as a static library, and a small Swift app draws it with Metal and plays the audio.

### Architecture

```
            ┌──────────── Song (seeded) ────────────┐
            │ key, tempo, chords, drum/bass/arp/     │
            │ lead patterns, all queryable by beat   │
            └───────┬───────────────────────┬────────┘
                    │                       │
              Synth / Engine           Scene + Frame
        (audio thread, AVAudioEngine)   (kick/snare/melody envelopes
                    │               read at the *heard* beat)
                    │                       │
   playback position ──► beat ──► camera Z, pulses, flashes
                                            │
                           ┌────────────────┴───────────────┐
                     Go: geometry +                 Metal (Swift)
                 shader parameters ───────────► all shading on the GPU
```

**Audio is the clock.** The engine records which song sample went into each output buffer. Every frame, the app subtracts the audio output latency that iOS reports from the samples rendered so far, smooths that against the frame clock, converts it to a beat position, and places the camera there. That's why grid lines, gates and prop rows pass exactly on the beat, and why the tape-speed control (which resamples the audio) slows the visuals by the same amount.

**One renderer, two GPU APIs.** On Windows the GPU renderer is OpenGL; on iOS it is Metal. The Metal shaders (`ios/App/Shaders.metal`) are *generated* from the OpenGL ones in `gl.go` by `MetalShaders()` in `metalparams.go`, and a test fails if they drift apart, so the iPhone shades sky, floor, bloom and VHS effects exactly like the desktop GPU renderer.

### Source files

| File | What it does |
|---|---|
| `song.go`, `synth.go`, `settings.go` | Song generator (eight styles), synthesizer and engine, settings and mix model. Shared with Windows. |
| `scene.go`, `space.go`, `render.go`, `props.go`, `raster.go` | Vaporwave and space scenes, camera, props and the display list of geometry. Shared with Windows. |
| `gl.go` | The OpenGL renderer and its shaders: the source the Metal shaders are generated from. Also packs the shader parameters both renderers use. |
| `mobile.go` | The iOS-facing core: scenes, touch input, the settings menu, audio-synced timing and per-frame render data. |
| `metalparams.go` | Generates `ios/App/Shaders.metal` from the GLSL shaders. |
| `capi/main.go` | The C API (`ve3d_*` functions) that Swift calls, built with `-buildmode=c-archive`. |
| `ios/App/*.swift` | SwiftUI app and HUD, Metal renderer, AVAudioEngine output, touch gestures. |
| `ios/project.yml`, `ios/setup.sh`, `ios/build-go.sh` | Xcode project spec (XcodeGen) and build scripts. |
| `game.go`, `game_draw.go` | Game mode from the Windows version. Not used by the iOS app; kept so shared code merges cleanly. |
| `main_other.go`, `cmd/ve3d-test` | Linux test harness: renders scenes to PNG and audio to WAV, compares the GPU and CPU renderers. |

---

## Building the iOS app

You need a Mac with **Xcode 15 or newer**, **Go 1.24 or newer** and **Homebrew**.

```sh
brew install go xcodegen      # once
git clone -b ios https://github.com/johnwalkr/vaporwave-endless-3d.git
cd vaporwave-endless-3d/ios
./setup.sh                    # builds the Go core, generates and opens the Xcode project
```

Then in Xcode:

1. Select the **VaporwaveEndless3D** target → **Signing & Capabilities** → choose your **Team** (a free Apple ID works for your own devices). If the bundle identifier is taken, change it to something unique.
2. Pick your iPhone (or a simulator) as the run destination and press **Run** (⌘R).
3. The first time on a device, allow it under iPhone **Settings → General → VPN & Device Management**.

Xcode re-runs `build-go.sh` before every build, so edits to the Go code are picked up automatically. If you change `project.yml` or add Swift files, run `xcodegen generate` in `ios/` again. The `Generated/` folder, the `.xcodeproj` and `Info.plist` are build outputs and are not committed.

If you change any shader in `gl.go`, regenerate the Metal shaders with `UPDATE_SHADERS=1 go test -run TestMetalShaders .`

### Automatic builds

Every push builds the Go core for iOS and compiles the app for the simulator on a GitHub macOS runner (`.github/workflows/build.yml`), so compile errors show up in the repository's **Actions** tab.

---

## Branches

- `main`: the Windows program (with game mode).
- `ios`: this app. The shared Go files (`song.go`, `synth.go`, `scene.go`, `space.go`, `render.go`, `props.go`, `raster.go`, `settings.go`, `gl.go`) are the same code with `package vaporwave` instead of `package main`. To bring a music or scene improvement from `main` over, `git merge main` into `ios` (or cherry-pick the commit); conflicts are usually limited to the package line.

## Notes

- Scene and planet names are generated or written for this project. The eight non-solar-system worlds are original designs.
