//go:build windows

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")

	pRegisterClassExW  = user32.NewProc("RegisterClassExW")
	pCreateWindowExW   = user32.NewProc("CreateWindowExW")
	pDefWindowProcW    = user32.NewProc("DefWindowProcW")
	pPeekMessageW      = user32.NewProc("PeekMessageW")
	pTranslateMessage  = user32.NewProc("TranslateMessage")
	pDispatchMessageW  = user32.NewProc("DispatchMessageW")
	pPostQuitMessage   = user32.NewProc("PostQuitMessage")
	pAdjustWindowRect  = user32.NewProc("AdjustWindowRectEx")
	pGetDC             = user32.NewProc("GetDC")
	pLoadCursorW       = user32.NewProc("LoadCursorW")
	pLoadIconW         = user32.NewProc("LoadIconW")
	pSetWindowTextW    = user32.NewProc("SetWindowTextW")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pGetWindowRect     = user32.NewProc("GetWindowRect")
	pSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	pGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	pSetWindowPos      = user32.NewProc("SetWindowPos")
	pGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	pSetTimer          = user32.NewProc("SetTimer")
	pKillTimer         = user32.NewProc("KillTimer")
	pDestroyWindow     = user32.NewProc("DestroyWindow")
	pMessageBoxW       = user32.NewProc("MessageBoxW")
	pSetFocus          = user32.NewProc("SetFocus")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pBitBlt             = gdi32.NewProc("BitBlt")
	pStretchBlt         = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
	pCreateFontW        = gdi32.NewProc("CreateFontW")
	pSetBkMode          = gdi32.NewProc("SetBkMode")
	pSetTextColor       = gdi32.NewProc("SetTextColor")
	pSetTextAlign       = gdi32.NewProc("SetTextAlign")
	pTextOutW           = gdi32.NewProc("TextOutW")
	pGdiFlush           = gdi32.NewProc("GdiFlush")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	pWaveOutOpen            = winmm.NewProc("waveOutOpen")
	pWaveOutClose           = winmm.NewProc("waveOutClose")
	pWaveOutPrepareHeader   = winmm.NewProc("waveOutPrepareHeader")
	pWaveOutUnprepareHeader = winmm.NewProc("waveOutUnprepareHeader")
	pWaveOutWrite           = winmm.NewProc("waveOutWrite")
	pWaveOutGetPosition     = winmm.NewProc("waveOutGetPosition")
	pWaveOutReset           = winmm.NewProc("waveOutReset")
	pWaveOutGetNumDevs      = winmm.NewProc("waveOutGetNumDevs")
	pWaveOutGetDevCapsW     = winmm.NewProc("waveOutGetDevCapsW")
	pTimeBeginPeriod        = winmm.NewProc("timeBeginPeriod")
)

const (
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmKillFocus     = 0x0008
	wmEraseBkgnd    = 0x0014
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmSysKeyDown    = 0x0104
	wmTimer         = 0x0113
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmRButtonDown   = 0x0204
	wmMouseWheel    = 0x020A
	wmEnterSizeMove = 0x0231
	wmExitSizeMove  = 0x0232
	wmQuit          = 0x0012

	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsPopup            = 0x80000000
	wsClipChildren     = 0x02000000
	waveMapper         = 0xFFFFFFFF
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	x, y    int32
	_       uint32
}

type rectT struct{ l, t, r, b int32 }

type bitmapInfo struct {
	size          uint32
	width, height int32
	planes, bpp   uint16
	compression   uint32
	sizeImage     uint32
	xppm, yppm    int32
	clrUsed       uint32
	clrImportant  uint32
	colors        [4]byte
}

type waveFormat struct {
	tag, channels uint16
	rate, bps     uint32
	align, bits   uint16
	cb            uint16
}

type waveHdr struct {
	data     *int16
	length   uint32
	recorded uint32
	user     uintptr
	flags    uint32
	loops    uint32
	next     uintptr
	reserved uintptr
}

const (
	maxBufs   = 8
	bufFrames = 1024
)

// audio buffers live in package-level memory so their addresses never move
var (
	hdrs    [maxBufs]waveHdr
	bufs    [maxBufs][bufFrames * 2]int16
	nullBuf [bufFrames * 2]int16
	quit    atomic.Bool
)

type menuItem struct {
	label  string
	header bool
	val    func() string
	adj    func(d int)
}

type rowHit struct {
	y0, y1 int
	idx    int
}

type App struct {
	hwnd, hdc, memDC, dib uintptr
	hinst, wndproc        uintptr
	pix                   []uint32
	iw, ih, cw, ch        int
	fBig, fMed, fSmall    uintptr

	rd  *Rd
	sc  *Scene
	pl  Player
	eng *Engine
	rng *rand.Rand
	st  Settings
	fx  FX

	keys       [256]bool
	mx, my     float32
	mouseSeen  bool
	start      time.Time
	last       float64
	sceneStart float64
	rateT      float64
	mute       bool
	help       bool
	helpAuto   bool
	space      bool
	deck       []int
	frameN     uint32
	sceneN     int
	fps        float64
	toast      string
	toastT     float64
	remixAt    float64
	game       *Game
	gameIn     GameInput

	// audio
	hwo        uintptr
	audioOK    bool
	audioStop  chan struct{}
	audioDone  chan struct{}
	wantBufs   atomic.Int32
	silentT0   time.Time
	posBase    float64
	lastRaw    uint32
	rawTotal   int64
	posEst     float64
	devices    []string
	deviceName string

	// GPU
	glw       *GLWindow
	gpuErr    string
	gpuFrames int
	vsyncOn   bool
	shotPath  string

	// menu
	menu     bool
	menuSel  int
	items    []menuItem
	rows     []rowHit
	menuX0   int
	menuValX int
	menuX1   int
	menuTop  int

	fullscreen bool
	savedStyle uintptr
	savedRect  rectT

	selftest bool
	noSave   bool
	log      *os.File
	shots    int
}

var app App

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func (a *App) logf(f string, args ...any) {
	if a.log != nil {
		fmt.Fprintf(a.log, "%7.2fs  "+f+"\n", append([]any{time.Since(a.start).Seconds()}, args...)...)
	}
}

// ---- settings files ------------------------------------------------------------

const appName = "Vaporwave Endless 3D"
const filePrefix = "vaporwave-endless-3d"
const settingsName = filePrefix + "_settings.ini"
const oldSettingsName = "fable3d_settings.ini"
const gpuMarkerName = filePrefix + "_gpu_starting.tmp"

func configDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, appName)
	}
	return exeDir()
}

func settingsPath() string {
	p := filepath.Join(exeDir(), settingsName)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.Close()
		os.Remove(p)
		return p
	}
	os.MkdirAll(configDir(), 0o755)
	return filepath.Join(configDir(), settingsName)
}

func markerPath() string { return filepath.Join(filepath.Dir(settingsPath()), gpuMarkerName) }

func (a *App) save() {
	if a.noSave {
		return
	}
	a.st.Speed = int(math.Round(a.rateT * 100))
	if err := a.st.Save(settingsPath()); err != nil {
		a.logf("save settings: %v", err)
	}
}

// ---- main -------------------------------------------------------------------------

func main() {
	runtime.LockOSThread()
	a := &app
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf(appName+" crashed: %v\n\n%s", r, debug.Stack())
			os.WriteFile(filepath.Join(exeDir(), filePrefix+"_crash.txt"), []byte(msg), 0o644)
			pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(msg))), uintptr(unsafe.Pointer(u16(appName))), 0x10)
		}
	}()
	a.start = time.Now()
	forceCPU := false
	for _, arg := range os.Args[1:] {
		switch strings.TrimLeft(arg, "-/") {
		case "selftest":
			a.selftest = true
		case "cpu":
			forceCPU = true
		}
	}
	a.st = DefaultSettings()
	if a.selftest {
		a.noSave = true
		a.log, _ = os.Create(filepath.Join(exeDir(), filePrefix+"_selftest.txt"))
		defer a.log.Close()
		a.logf("selftest start, %s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	} else if err := a.st.Load(settingsPath()); err == nil {
		a.st.Clamp()
	} else if err := a.st.Load(filepath.Join(exeDir(), oldSettingsName)); err == nil {
		a.st.Clamp()
	}
	if _, err := os.Stat(markerPath()); err == nil {
		os.Remove(markerPath())
		if a.st.GPU {
			a.st.GPU = false
			a.setToast("GPU mode did not start cleanly last time, so this run uses the CPU. Re-enable it in Settings (O).")
		}
	}
	if forceCPU {
		a.st.GPU = false
	}
	a.fx = a.st.FX()

	pTimeBeginPeriod.Call(1)
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.rd = &Rd{FX: &a.fx}
	a.eng = NewEngine()
	a.eng.SetMix(a.st.Mix())
	CurrentSongOpts = a.st.SongOpts()
	a.rateT = float64(a.st.Speed) / 100
	a.help, a.helpAuto = true, true
	a.createWindow()
	a.openAudio()
	a.newScene()
	if a.st.GPU {
		a.enableGPU()
	}

	var m msgT
	for !quit.Load() {
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			if m.message == wmQuit {
				quit.Store(true)
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if quit.Load() {
			break
		}
		t0 := time.Now()
		a.frame()
		if a.selftest {
			a.selftestStep()
		}
		target := 16600 * time.Microsecond
		if a.glw != nil {
			if a.vsyncOn {
				target = 0
			} else {
				target = 4 * time.Millisecond
			}
		}
		if d := target - time.Since(t0); d > 0 {
			time.Sleep(d)
		}
	}
	a.save()
	a.disableGPU()
	a.closeAudio()
}

func (a *App) setToast(s string) { a.toast, a.toastT = s, a.now() }

func (a *App) createWindow() {
	a.hinst, _, _ = pGetModuleHandleW.Call(0)
	a.wndproc = syscall.NewCallback(wndProc)
	cls := u16("VaporwaveEndless3DWindow")
	cur, _, _ := pLoadCursorW.Call(0, 32515) // IDC_CROSS
	ico, _, _ := pLoadIconW.Call(0, 32512)
	wc := wndClassEx{style: 0x0020 | 0x0003, lpfnWndProc: a.wndproc, hInstance: a.hinst, hCursor: cur, hIcon: ico, lpszClassName: cls}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		panic(fmt.Sprint("RegisterClassEx: ", err))
	}
	rc := rectT{0, 0, 800, 600}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), wsOverlappedWindow, 0, 0)
	style := uintptr(wsOverlappedWindow | wsVisible | wsClipChildren)
	const useDefault = 0x80000000
	h, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(u16(appName))),
		style, useDefault, useDefault, uintptr(rc.r-rc.l), uintptr(rc.b-rc.t), 0, 0, a.hinst, 0)
	if h == 0 {
		panic(fmt.Sprint("CreateWindowEx: ", err))
	}
	a.hwnd = h
	a.hdc, _, _ = pGetDC.Call(h)
	a.memDC, _, _ = pCreateCompatibleDC.Call(a.hdc)
	pSetStretchBltMode.Call(a.hdc, 4) // HALFTONE
	var cr rectT
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	a.cw, a.ch = int(cr.r), int(cr.b)
	a.logf("window ok, client %dx%d", a.cw, a.ch)
}

// renderSize is the internal resolution for the current renderer and scale.
func (a *App) renderSize() (int, int) {
	s := float64(a.st.Scale) / 100
	var w, h float64
	if a.glw != nil {
		w, h = float64(a.cw)*s, float64(a.ch)*s
	} else {
		k := math.Ceil(math.Sqrt(float64(a.cw*a.ch) / 1.1e6))
		if k < 1 {
			k = 1
		}
		w, h = math.Floor(float64(a.cw)/k)*s, math.Floor(float64(a.ch)/k)*s
	}
	return int(clampF(w, 16, 4096)), int(clampF(h, 16, 4096))
}

// ensureSurface (re)creates the DIB the scene (CPU) or HUD (GPU) is drawn into.
func (a *App) ensureSurface(w, h int) {
	if w == a.iw && h == a.ih && a.dib != 0 {
		return
	}
	bi := bitmapInfo{width: int32(w), height: -int32(h), planes: 1, bpp: 32}
	bi.size = 40
	var bits unsafe.Pointer
	dib, _, err := pCreateDIBSection.Call(a.memDC, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == nil {
		panic(fmt.Sprintf("CreateDIBSection %dx%d: %v", w, h, err))
	}
	pSelectObject.Call(a.memDC, dib)
	if a.dib != 0 {
		pDeleteObject.Call(a.dib)
	}
	a.dib = dib
	a.pix = unsafe.Slice((*uint32)(bits), w*h)
	a.iw, a.ih = w, h
	for _, f := range []uintptr{a.fBig, a.fMed, a.fSmall} {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
	mk := func(px int, weight uintptr, face string) uintptr {
		f, _, _ := pCreateFontW.Call(uintptr(int32(-max(px, 6))), 0, 0, 0, weight, 0, 0, 0, 1, 0, 0, 3, 0, uintptr(unsafe.Pointer(u16(face))))
		return f
	}
	s := float64(h) / 600
	a.fBig = mk(int(30*s), 700, "MS Gothic")
	a.fMed = mk(int(20*s), 700, "MS Gothic")
	a.fSmall = mk(int(14*s), 400, "MS Gothic")
	pSetBkMode.Call(a.memDC, 1)
}

func (a *App) text(font uintptr, x, y int, col uint32, align uintptr, s string) {
	u, _ := syscall.UTF16FromString(s)
	n := len(u) - 1
	if n <= 0 {
		return
	}
	pSelectObject.Call(a.memDC, font)
	pSetTextAlign.Call(a.memDC, align)
	pSetTextColor.Call(a.memDC, uintptr(col))
	pTextOutW.Call(a.memDC, uintptr(int32(x)), uintptr(int32(y)), uintptr(unsafe.Pointer(&u[0])), uintptr(n))
}

// shadowed text; col is 0x00BBGGRR
func (a *App) textS(font uintptr, x, y int, col uint32, align uintptr, s string) {
	a.text(font, x+2, y+2, 0x200010, align, s)
	a.text(font, x, y, col, align, s)
}

// darken a rectangle behind HUD text (CPU: halve pixels, GPU: alpha mask)
func (a *App) darkRect(x0, y0, x1, y1 int, strength uint32) {
	pGdiFlush.Call()
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, a.iw), min(y1, a.ih)
	for y := y0; y < y1; y++ {
		row := a.pix[y*a.iw+x0 : y*a.iw+x1]
		if a.glw != nil {
			for i := range row {
				row[i] = strength << 24
			}
		} else {
			k := 255 - strength
			for i, p := range row {
				r, g, b := (p>>16&255)*k/255, (p>>8&255)*k/255, (p&255)*k/255
				row[i] = r<<16 | g<<8 | b
			}
		}
	}
}

func (a *App) setTitle() {
	if a.sc == nil || a.hwnd == 0 {
		return
	}
	mode := "CPU"
	if a.glw != nil {
		mode = "GPU"
	}
	t := fmt.Sprintf(appName+" [%s] — %s — [Tab] new scene · [Space] space travel · [O] settings · [G] GPU/CPU", mode, a.sc.Title())
	pSetWindowTextW.Call(a.hwnd, uintptr(unsafe.Pointer(u16(t))))
}

func (a *App) newScene() {
	seed := a.rng.Uint32()
	if a.selftest {
		seed = uint32(4 + a.sceneN*8)
	}
	a.sc = NewScene(seed)
	if a.space {
		if len(a.deck) == 0 {
			a.deck = a.rng.Perm(NumWorlds())
		}
		a.sc = NewSpaceScene(seed, a.deck[0])
		a.deck = a.deck[1:]
	}
	a.sceneN++
	a.eng.NewSong(a.sc.Song)
	a.pl.X, a.pl.VX, a.pl.H = 0, 0, a.sc.CamH
	a.pl.Rings = nil
	a.sceneStart = a.now()
	a.setTitle()
}

func (a *App) now() float64 { return time.Since(a.start).Seconds() }

// remix replaces the current scene's track using the music settings.
func (a *App) remix(seed uint32) {
	CurrentSongOpts = a.st.SongOpts()
	sc := *a.sc
	sc.Song = NewSong(seed)
	a.sc = &sc
	a.eng.NewSong(a.sc.Song)
	a.pl.Rings = nil
	a.sceneStart = a.now()
	a.setToast(fmt.Sprintf("Remix: %s · %.0f BPM · %s", a.sc.Song.StyleTag(), a.sc.Song.BPM, a.sc.Song.KeyName()))
}

// ---- audio ------------------------------------------------------------------

func waveDevices() []string {
	n, _, _ := pWaveOutGetNumDevs.Call()
	var out []string
	for i := uintptr(0); i < n && i < 64; i++ {
		var caps [84]byte
		if r, _, _ := pWaveOutGetDevCapsW.Call(i, uintptr(unsafe.Pointer(&caps[0])), 84); r != 0 {
			out = append(out, fmt.Sprintf("Device %d", i))
			continue
		}
		name := unsafe.Slice((*uint16)(unsafe.Pointer(&caps[8])), 32)
		out = append(out, strings.TrimSpace(syscall.UTF16ToString(name)))
	}
	return out
}

func (a *App) openAudio() {
	a.devices = waveDevices()
	dev := uintptr(waveMapper)
	a.deviceName = "System default"
	for i, n := range a.devices {
		if a.st.Device != "" && n == a.st.Device {
			dev, a.deviceName = uintptr(i), n
		}
	}
	a.wantBufs.Store(int32(LatencyBufs[a.st.Latency]))
	a.posBase = float64(a.eng.OutTotal())
	a.rawTotal, a.lastRaw, a.posEst = 0, 0, a.posBase
	wf := waveFormat{tag: 1, channels: 2, rate: SR, bps: SR * 4, align: 4, bits: 16}
	r, _, _ := pWaveOutOpen.Call(uintptr(unsafe.Pointer(&a.hwo)), dev, uintptr(unsafe.Pointer(&wf)), 0, 0, 0)
	if r != 0 && dev != waveMapper {
		a.setToast("Could not open " + a.deviceName + ", using the system default.")
		a.deviceName = "System default"
		r, _, _ = pWaveOutOpen.Call(uintptr(unsafe.Pointer(&a.hwo)), waveMapper, uintptr(unsafe.Pointer(&wf)), 0, 0, 0)
	}
	a.audioOK = r == 0
	a.logf("waveOutOpen %q result %d, %d devices %q", a.deviceName, r, len(a.devices), a.devices)
	stop, done := make(chan struct{}), make(chan struct{})
	a.audioStop, a.audioDone = stop, done
	if !a.audioOK {
		// no audio device: keep the song running silently against the wall clock
		a.silentT0 = time.Now()
		t0, base := a.silentT0, a.posBase
		go func() {
			defer close(done)
			written := base
			for {
				select {
				case <-stop:
					return
				default:
				}
				for written < base+time.Since(t0).Seconds()*SR+2048 {
					a.eng.Fill(nullBuf[:])
					written += bufFrames
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
		return
	}
	for i := range hdrs {
		hdrs[i] = waveHdr{data: &bufs[i][0], length: bufFrames * 4}
		pWaveOutPrepareHeader.Call(a.hwo, uintptr(unsafe.Pointer(&hdrs[i])), unsafe.Sizeof(hdrs[i]))
	}
	hwo := a.hwo
	go func() {
		defer close(done)
		head, queued := 0, 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			for queued > 0 && atomic.LoadUint32(&hdrs[head].flags)&1 != 0 { // WHDR_DONE
				head = (head + 1) % maxBufs
				queued--
			}
			want := int(a.wantBufs.Load())
			for queued < want {
				i := (head + queued) % maxBufs
				a.eng.Fill(bufs[i][:])
				atomic.StoreUint32(&hdrs[i].flags, atomic.LoadUint32(&hdrs[i].flags)&^1)
				pWaveOutWrite.Call(hwo, uintptr(unsafe.Pointer(&hdrs[i])), unsafe.Sizeof(hdrs[i]))
				queued++
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
}

func (a *App) closeAudio() {
	if a.audioStop != nil {
		close(a.audioStop)
		<-a.audioDone
		a.audioStop = nil
	}
	if a.audioOK {
		pWaveOutReset.Call(a.hwo)
		for i := range hdrs {
			pWaveOutUnprepareHeader.Call(a.hwo, uintptr(unsafe.Pointer(&hdrs[i])), unsafe.Sizeof(hdrs[i]))
		}
		pWaveOutClose.Call(a.hwo)
		a.audioOK = false
	}
}

// playPos returns the output frame the listener is hearing right now.
func (a *App) playPos(dt float64) float64 {
	if !a.audioOK {
		return a.posBase + time.Since(a.silentT0).Seconds()*SR
	}
	var mm [12]byte
	*(*uint32)(unsafe.Pointer(&mm[0])) = 2 // TIME_SAMPLES
	r, _, _ := pWaveOutGetPosition.Call(a.hwo, uintptr(unsafe.Pointer(&mm[0])), 12)
	typ := *(*uint32)(unsafe.Pointer(&mm[0]))
	val := *(*uint32)(unsafe.Pointer(&mm[4]))
	a.posEst += dt * SR
	if r == 0 && (typ == 2 || typ == 4 || typ == 1) {
		switch typ {
		case 4:
			val /= 4
		case 1:
			val = uint32(float64(val) * SR / 1000)
		}
		d := val - a.lastRaw
		if d < 1<<31 {
			a.rawTotal += int64(d)
		}
		a.lastRaw = val
		err := a.posBase + float64(a.rawTotal) - a.posEst
		if math.Abs(err) > 4000 {
			a.posEst += err
		} else {
			a.posEst += err * 0.08
		}
	}
	return a.posEst
}

func (a *App) applyAudio() {
	a.eng.SetMix(a.st.Mix())
	a.wantBufs.Store(int32(LatencyBufs[a.st.Latency]))
}

// ---- GPU --------------------------------------------------------------------

func (a *App) enableGPU() bool {
	if a.glw != nil {
		return true
	}
	if !a.noSave {
		os.WriteFile(markerPath(), []byte("GPU start in progress\n"), 0o644)
	}
	gw, err := newGLWindow(a.hwnd, a.hinst, a.wndproc, a.cw, a.ch)
	if err != nil {
		os.Remove(markerPath())
		a.gpuErr = err.Error()
		a.st.GPU = false
		a.logf("GPU init failed: %v", err)
		a.setToast("GPU mode unavailable (" + err.Error() + "). Using the CPU renderer.")
		return false
	}
	a.glw = gw
	a.gpuErr = ""
	a.gpuFrames = 0
	a.vsyncOn = gw.SetVSync(a.st.VSync) && a.st.VSync
	a.st.GPU = true
	a.logf("GPU on: %s | %s | vsync %v", gw.R.Renderer, gw.R.Version, a.vsyncOn)
	a.setToast("Switched to GPU rendering (OpenGL) on " + gw.R.Renderer)
	a.setTitle()
	pSetFocus.Call(a.hwnd)
	return true
}

func (a *App) disableGPU() {
	if a.glw == nil {
		return
	}
	a.glw.Close()
	a.glw = nil
	os.Remove(markerPath())
}

func (a *App) setGPU(on bool) {
	if on {
		a.enableGPU()
	} else {
		a.disableGPU()
		a.st.GPU = false
		a.setToast("Switched to CPU rendering (software)")
	}
	a.setTitle()
	a.save()
}

// ---- frame ------------------------------------------------------------------

func (a *App) frame() {
	now := a.now()
	dt := now - a.last
	a.last = now
	if dt > 0.1 {
		dt = 0.1
	}
	if dt > 0 {
		a.fps += (1/dt - a.fps) * 0.05
	}
	// input
	var dir float32
	if a.keys['A'] || (!a.menu && a.keys[0x25]) {
		dir--
	}
	if a.keys['D'] || (!a.menu && a.keys[0x27]) {
		dir++
	}
	if a.keys['W'] || (!a.menu && a.keys[0x26]) {
		a.rateT += dt * 0.5
	}
	if a.keys['S'] || (!a.menu && a.keys[0x28]) {
		a.rateT -= dt * 0.5
	}
	a.rateT = math.Min(1.8, math.Max(0.4, a.rateT))
	if a.game != nil {
		a.rateT = 1
		in := a.gameIn
		a.gameIn = GameInput{}
		if !a.menu {
			if a.keys['A'] || a.keys[0x25] {
				in.DX--
			}
			if a.keys['D'] || a.keys[0x27] {
				in.DX++
			}
			if a.keys['W'] || a.keys[0x26] {
				in.DY++
			}
			if a.keys['S'] || a.keys[0x28] {
				in.DY--
			}
		}
		a.game.Update(dt, in)
		a.sc = a.game.Sc
		dir = 0
	}
	a.eng.SetRate(a.rateT)
	if a.game == nil && a.mouseSeen && !a.menu && a.cw > 0 && a.ch > 0 {
		a.pl.YawT = (a.mx/float32(a.cw) - 0.5) * 2 * 1.15
		a.pl.PitchT = (0.5-a.my/float32(a.ch))*2*0.5 + 0.02
	}
	a.pl.Update(float32(dt), dir, a.sc)
	for len(a.pl.Rings) > 0 && now-a.pl.Rings[0].T0 > 2.5 {
		a.pl.Rings = a.pl.Rings[1:]
	}
	if a.remixAt > 0 && now >= a.remixAt && a.game == nil {
		a.remixAt = 0
		a.remix(a.sc.Seed)
	}
	if a.cw < 16 || a.ch < 16 {
		return
	}
	pos := a.playPos(dt)
	beat := a.eng.BeatAt(pos)
	a.frameN++
	fr := MakeFrame(a.sc, &a.pl, beat, now, now-a.sceneStart, a.frameN)
	if a.game != nil {
		fr = a.game.MakeFrame(beat, now, a.frameN)
	}
	w, h := a.renderSize()
	a.ensureSurface(w, h)
	a.fx = a.st.FX()

	if a.glw != nil {
		a.glw.Resize(a.cw, a.ch)
		pGdiFlush.Call()
		clear(a.pix)
		a.hud(now, beat)
		pGdiFlush.Call()
		a.rd.Prepare(a.sc, &fr, w, h)
		err := a.glw.R.Frame(a.rd, a.pix, a.cw, a.ch, 0)
		if err == nil && a.shotPath != "" {
			out := a.glw.R.Read(0, a.cw, a.ch)
			a.logf("screenshot %s: %v", a.shotPath, savePNG(a.shotPath, out, a.cw, a.ch))
			a.shotPath = ""
		}
		if err != nil {
			a.logf("GPU frame error: %v", err)
			a.gpuErr = err.Error()
			a.disableGPU()
			a.st.GPU = false
			a.setToast("GPU error (" + err.Error() + "). Switched to the CPU renderer.")
			return
		}
		a.glw.Swap()
		a.gpuFrames++
		if a.gpuFrames == 120 {
			os.Remove(markerPath())
		}
		return
	}

	a.rd.Render(a.sc, &fr, w, h, a.pix)
	a.hud(now, beat)
	pGdiFlush.Call()
	if a.shotPath != "" {
		a.logf("screenshot %s: %v", a.shotPath, savePNG(a.shotPath, a.pix, a.iw, a.ih))
		a.shotPath = ""
	}
	if a.iw == a.cw && a.ih == a.ch {
		pBitBlt.Call(a.hdc, 0, 0, uintptr(a.cw), uintptr(a.ch), a.memDC, 0, 0, 0x00CC0020)
	} else {
		pStretchBlt.Call(a.hdc, 0, 0, uintptr(a.cw), uintptr(a.ch), a.memDC, 0, 0, uintptr(a.iw), uintptr(a.ih), 0x00CC0020)
	}
}

func (a *App) hud(now, beat float64) {
	W, H := a.iw, a.ih
	s := func(v int) int { return v * H / 600 }
	age := now - a.sceneStart
	if a.game != nil {
		a.gameHUD()
		age = 1e9
	} else if age < 3.5 && (age < 2 || int(age*4)%2 == 0) {
		a.textS(a.fBig, s(28), s(22), 0xFFFFFF, 0, "▶ PLAY")
	}
	t := int(math.Max(beat, 0) * 60 / a.sc.Song.BPM)
	if a.game != nil {
		t = int(a.game.T)
	}
	a.textS(a.fMed, W-s(28), s(26), 0xFFFFFF, 2, fmt.Sprintf("SP  %d:%02d:%02d", t/3600, t/60%60, t%60))
	y := s(52)
	if a.mute {
		a.textS(a.fMed, W-s(28), y, 0x6060FF, 2, "MUTE")
		y += s(24)
	}
	badge, bcol := "CPU RENDER", uint32(0x40B0FF) // orange (0x00BBGGRR)
	if a.glw != nil {
		badge, bcol = "GPU RENDER", 0x60FF60 // green
	}
	if a.st.ShowFPS {
		fcol := uint32(0x60FF60) // green / yellow / red by frame rate (0x00BBGGRR)
		if a.fps < 50 {
			fcol = 0x40E0FF
		}
		if a.fps < 30 {
			fcol = 0x5050FF
		}
		ms := 0.0
		if a.fps > 0 {
			ms = 1000 / a.fps
		}
		a.textS(a.fMed, W-s(28), y, fcol, 2, fmt.Sprintf("%.0f FPS", a.fps))
		y += s(24)
		a.textS(a.fSmall, W-s(28), y, 0xD0D0D0, 2, fmt.Sprintf("%.1f ms · %dx%d", ms, a.iw, a.ih))
		y += s(18)
	}
	a.textS(a.fSmall, W-s(28), y, bcol, 2, "● "+badge)
	if !a.menu && (age < 6 || a.help) {
		ti := a.sc.Title()
		y := H - s(84)
		a.text(a.fBig, s(28)-2, y, 0xFFFF00, 0, ti)
		a.text(a.fBig, s(28)+2, y, 0xFF00FF, 0, ti)
		a.text(a.fBig, s(28), y, 0xFFFFFF, 0, ti)
		a.textS(a.fSmall, s(30), H-s(46), 0xFFE0FF, 0,
			fmt.Sprintf("SCENE %08X · %s · %s · %.0f BPM · %s · TAPE x%.2f", a.sc.Seed, a.sc.P.Name, a.sc.Song.StyleTag(), a.sc.Song.BPM, a.sc.Song.KeyName(), a.eng.Rate()))
	}
	if a.help && a.helpAuto && now > 14 {
		a.help = false
	}
	if a.help && !a.menu && a.game == nil {
		lines := []string{
			"TAB  new scene + new track     SPACE  vaporwave <-> space travel",
			"W/S  tape speed (music + travel)     A/D  strafe",
			"MOUSE  look     CLICK  pulse + chime     WHEEL  altitude",
			"RIGHT CLICK  recolour    R  reset    M  mute    P  screenshot",
			"ENTER  game mode: fly a ship through rings with WASD",
			"O  settings    G  GPU/CPU    F2  FPS counter    F  fullscreen",
			"H  hide this     ESC  quit",
		}
		y := s(70)
		for _, l := range lines {
			a.textS(a.fSmall, s(30), y, 0xFFFFC0, 0, l)
			y += s(19)
		}
	}
	if a.toast != "" && now-a.toastT < 7 {
		a.textS(a.fSmall, W/2, H-s(120), 0x80E0FF, 6, a.toast) // TA_CENTER
	}
	if a.menu {
		a.drawMenu()
	}
}

// ---- game mode ------------------------------------------------------------------

func (a *App) toggleGame() {
	if a.game != nil {
		a.game = nil
		a.eng.ClearLoop()
		a.newScene()
		a.setToast("Game over — back to the endless drive")
		return
	}
	a.space = false
	a.help = false
	if a.menu {
		a.toggleMenu()
	}
	a.game = NewGame(a.eng, a.rng.Uint32(), a.st.Style-1, a.st.Mood)
	a.sc = a.game.Sc
	a.sceneStart = a.now()
	a.setTitle()
}

func (a *App) gameHUD() {
	g := a.game
	W, H := a.iw, a.ih
	s := func(v int) int { return v * H / 600 }
	label := PhaseNames[g.Phase]
	if g.Phase == PhaseChill {
		label += fmt.Sprintf("  %d", g.ChillLeft())
	}
	a.textS(a.fBig, s(28), s(20), 0xFFFFFF, 0, label)
	bpm := a.eng.Tempo()
	y := s(58)
	if g.Phase == PhaseFlight {
		n := int(clampF((bpm-60)/(BoomBPM-60), 0, 1) * 20)
		bar := strings.Repeat("■", n) + strings.Repeat("□", 20-n)
		col := uint32(0xFFFFC0)
		if bpm > 220 {
			col = 0x6060FF
		}
		a.textS(a.fMed, s(28), y, col, 0, fmt.Sprintf("%3.0f BPM  %s  %d", bpm, bar, BoomBPM))
		y += s(24)
		a.textS(a.fSmall, s(30), y, 0xFFE0FF, 0, fmt.Sprintf("RINGS %d   ·   %s", g.Hits, StyleNames[g.Opts.Style]))
	} else {
		a.textS(a.fMed, s(28), y, 0xFFFFC0, 0, fmt.Sprintf("%3.0f BPM", bpm))
		y += s(24)
		if g.Phase == PhaseCreate {
			a.textS(a.fSmall, s(30), y, 0xFFE0FF, 0, fmt.Sprintf("NOTES %d   ·   RINGS %d   ·   %s", g.Notes, g.createHits, StyleNames[g.Opts.Style]))
		}
	}
	if g.Toast != "" && g.T-g.ToastT < 4 {
		a.textS(a.fMed, W/2, H-s(110), 0xFFFFFF, 6, g.Toast)
	}
	if g.PhaseT < 9 {
		var hint string
		switch g.Phase {
		case PhaseFlight:
			hint = "WASD fly  ·  every ring changes the world and the music  ·  ENTER quit game"
		case PhaseChill:
			hint = "breathe"
		case PhaseCreate:
			hint = "WASD play notes  ·  SPACE drums in/out  ·  CLICK new chords  ·  WHEEL tempo  ·  BACKSPACE clear"
		}
		a.textS(a.fSmall, W/2, H-s(70), 0xD0F0FF, 6, hint)
	}
}

// ---- settings menu ------------------------------------------------------------

func pct(v int) string { return fmt.Sprintf("%d%%", v) }
func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

func (a *App) buildMenu() {
	st := &a.st
	custom := func() { st.Preset = PresetCustom }
	level := func(label string, p *int, lo, hi, step int, mix bool) menuItem {
		return menuItem{label: label, val: func() string { return pct(*p) }, adj: func(d int) {
			*p = clampI(*p+d*step, lo, hi)
			if mix {
				custom()
			}
			a.applyAudio()
		}}
	}
	music := func(label string, p *int, names []string) menuItem {
		return menuItem{label: label, val: func() string { return names[*p] }, adj: func(d int) {
			*p = (*p + d + len(names)) % len(names)
			CurrentSongOpts = st.SongOpts()
			a.remixAt = a.now() + 0.6
		}}
	}
	a.items = []menuItem{
		{label: "GAME", header: true},
		{label: "Game mode (Enter)", val: func() string { return onOff(a.game != nil) }, adj: func(d int) { a.toggleGame() }},
		{label: "MUSIC  (changes remix the current track)", header: true},
		music("Style", &st.Style, append([]string{"Auto (random per scene)"}, StyleNames...)),
		music("Drums", &st.DrumLevel, DrumNames),
		music("Tempo", &st.Tempo, TempoNames),
		music("Mood", &st.Mood, MoodNames),
		music("Melody", &st.Melody, MelodyNames),
		{label: "New track for this scene", val: func() string { return "Enter" }, adj: func(d int) {
			a.remix(a.rng.Uint32())
		}},
		{label: "AUDIO", header: true},
		{label: "Preset", val: func() string { return PresetNames[st.Preset] }, adj: func(d int) {
			i := st.Preset
			if i == PresetCustom {
				i = map[bool]int{true: 0, false: PresetCustom - 1}[d > 0]
			} else {
				i = (i + d + PresetCustom) % PresetCustom
			}
			st.ApplyPreset(i)
			a.rateT = float64(st.Speed) / 100
			a.applyAudio()
		}},
		level("Master volume", &st.Master, 0, 100, 5, false),
		level("Drums", &st.Drums, 0, 200, 10, true),
		level("Bass", &st.Bass, 0, 200, 10, true),
		level("Pads", &st.Pads, 0, 200, 10, true),
		level("Keys & arpeggios", &st.Keys, 0, 200, 10, true),
		level("Lead melody", &st.Lead, 0, 200, 10, true),
		level("Click chimes", &st.SFX, 0, 200, 10, false),
		level("Reverb", &st.Reverb, 0, 200, 10, true),
		level("Echo", &st.Echo, 0, 200, 10, true),
		level("Tape wobble", &st.Wobble, 0, 300, 20, true),
		level("Vinyl crackle & hiss", &st.Crackle, 0, 300, 20, true),
		{label: "Tone (dark - bright)", val: func() string { return pct(st.Tone) }, adj: func(d int) {
			st.Tone = clampI(st.Tone+d*5, 0, 100)
			custom()
			a.applyAudio()
		}},
		level("Stereo width", &st.Width, 0, 200, 10, true),
		{label: "Tape speed", val: func() string { return pct(int(math.Round(a.rateT * 100))) }, adj: func(d int) {
			a.rateT = clampF(math.Round(a.rateT*20+float64(d))/20, 0.4, 1.8)
			custom()
		}},
		{label: "Output device", val: func() string { return a.deviceName }, adj: func(d int) {
			names := append([]string{""}, waveDevices()...)
			cur := 0
			for i, n := range names {
				if n == st.Device {
					cur = i
				}
			}
			st.Device = names[(cur+d+len(names))%len(names)]
			a.closeAudio()
			a.openAudio()
		}},
		{label: "Latency", val: func() string {
			return fmt.Sprintf("%s (%d ms)", LatencyNames[st.Latency], LatencyBufs[st.Latency]*bufFrames*1000/SR)
		}, adj: func(d int) {
			st.Latency = clampI(st.Latency+d, 0, 2)
			a.applyAudio()
		}},
		{label: "VIDEO", header: true},
		{label: "Renderer", val: func() string {
			if a.glw != nil {
				return "GPU (OpenGL)"
			}
			if a.gpuErr != "" {
				return "CPU (GPU failed)"
			}
			return "CPU (software)"
		}, adj: func(d int) { a.setGPU(a.glw == nil) }},
		{label: "Render scale", val: func() string { return pct(st.Scale) }, adj: func(d int) {
			i := 0
			for k, v := range ScaleSteps {
				if v == st.Scale {
					i = k
				}
			}
			st.Scale = ScaleSteps[clampI(i+d, 0, len(ScaleSteps)-1)]
		}},
		{label: "VSync (GPU)", val: func() string { return onOff(st.VSync) }, adj: func(d int) {
			st.VSync = !st.VSync
			if a.glw != nil {
				a.vsyncOn = a.glw.SetVSync(st.VSync) && st.VSync
			}
		}},
		level("Bloom", &st.Bloom, 0, 200, 10, false),
		{label: "VHS effects", val: func() string { return VHSNames[st.VHS] }, adj: func(d int) {
			st.VHS = (st.VHS + d + 3) % 3
		}},
		{label: "FPS counter (F2)", val: func() string { return onOff(st.ShowFPS) }, adj: func(d int) { st.ShowFPS = !st.ShowFPS }},
		{label: "Reset all to defaults", val: func() string { return "Enter" }, adj: func(d int) {
			gpu, dev := st.GPU, st.Device
			*st = DefaultSettings()
			st.GPU, st.Device = gpu, dev
			a.rateT = 1
			a.applyAudio()
			CurrentSongOpts = st.SongOpts()
			a.remixAt = a.now() + 0.6
		}},
	}
	if a.menuSel <= 0 || a.menuSel >= len(a.items) {
		a.menuSel = 1
	}
}

func (a *App) toggleMenu() {
	a.menu = !a.menu
	if a.menu {
		a.buildMenu()
		a.help = false
	} else {
		a.save()
	}
}

func (a *App) moveSel(d int) {
	for i := 0; i < len(a.items); i++ {
		a.menuSel = (a.menuSel + d + len(a.items)) % len(a.items)
		if !a.items[a.menuSel].header {
			return
		}
	}
}

func (a *App) adjSel(d int) {
	if a.menuSel > 0 && a.menuSel < len(a.items) && !a.items[a.menuSel].header {
		a.items[a.menuSel].adj(d)
	}
}

func (a *App) drawMenu() {
	W, H := a.iw, a.ih
	s := func(v int) int { return v * H / 600 }
	rowH := s(18)
	n := len(a.items)
	vis := max(min(n, (H-s(110))/rowH), 4)
	if a.menuSel-1 < a.menuTop {
		a.menuTop = max(a.menuSel-1, 0)
	}
	if a.menuSel >= a.menuTop+vis {
		a.menuTop = a.menuSel - vis + 1
	}
	a.menuTop = clampI(a.menuTop, 0, max(n-vis, 0))
	panelH := (vis+4)*rowH + s(10)
	x0, x1 := W/2-s(280), W/2+s(280)
	y0 := max((H-panelH)/2, 0)
	a.darkRect(x0, y0, x1, y0+panelH, 170)
	a.menuX0, a.menuX1, a.menuValX = x0, x1, x0+s(300)
	a.textS(a.fMed, W/2, y0+s(8), 0xFFC0FF, 6, "ＳＥＴＴＩＮＧＳ")
	y := y0 + s(8) + rowH*2
	a.rows = a.rows[:0]
	if a.menuTop > 0 {
		a.text(a.fSmall, x1-s(22), y0+s(8)+rowH, 0xB0B0B0, 2, "^ more")
	}
	for i := a.menuTop; i < a.menuTop+vis && i < n; i++ {
		it := a.items[i]
		if it.header {
			a.text(a.fSmall, x0+s(18), y+s(2), 0xFFFF60, 0, "── "+it.label+" "+strings.Repeat("─", 30))
		} else {
			col := uint32(0xE8E8E8)
			mark := "  "
			if i == a.menuSel {
				col, mark = 0x80FFFF, "> "
				a.darkRect(x0+s(10), y, x1-s(10), y+rowH, 230)
			}
			a.textS(a.fSmall, x0+s(18), y+s(2), col, 0, mark+it.label)
			a.textS(a.fSmall, x1-s(22), y+s(2), col, 2, "< "+it.val()+" >")
			a.rows = append(a.rows, rowHit{y, y + rowH, i})
		}
		y += rowH
	}
	if a.menuTop+vis < n {
		a.text(a.fSmall, x1-s(22), y-s(2), 0xB0B0B0, 2, "v more")
	}
	help := "↑↓ select   ←→ change   click / right-click on a value   O or Esc close"
	a.text(a.fSmall, W/2, y+s(6), 0xB0B0B0, 6, help)
	if a.gpuErr != "" && a.glw == nil {
		e := a.gpuErr
		if len(e) > 90 {
			e = e[:90] + "…"
		}
		a.text(a.fSmall, W/2, y+s(6)+rowH, 0x8080FF, 6, "GPU: "+e)
	}
}

// menuClick handles a click inside the menu; returns true if it was consumed.
func (a *App) menuClick(cx, cy int, dir int) bool {
	if a.cw == 0 {
		return false
	}
	x, y := cx*a.iw/a.cw, cy*a.ih/a.ch
	if x < a.menuX0 || x > a.menuX1 {
		return true
	}
	for _, r := range a.rows {
		if y >= r.y0 && y < r.y1 {
			a.menuSel = r.idx
			if x >= a.menuValX {
				a.adjSel(dir)
			}
		}
	}
	return true
}

// ---- misc ------------------------------------------------------------------

func savePNG(path string, pix []uint32, w, h int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, p := range pix[:w*h] {
		img.SetRGBA(i%w, i/w, color.RGBA{uint8(p >> 16), uint8(p >> 8), uint8(p), 255})
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func (a *App) toggleFullscreen() {
	const gwlStyle = ^uintptr(15) // -16
	if !a.fullscreen {
		a.savedStyle, _, _ = pGetWindowLongPtrW.Call(a.hwnd, gwlStyle)
		pGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&a.savedRect)))
		w, _, _ := pGetSystemMetrics.Call(0)
		h, _, _ := pGetSystemMetrics.Call(1)
		pSetWindowLongPtrW.Call(a.hwnd, gwlStyle, wsPopup|wsVisible|wsClipChildren)
		pSetWindowPos.Call(a.hwnd, 0, 0, 0, w, h, 0x0020|0x0004)
	} else {
		r := a.savedRect
		pSetWindowLongPtrW.Call(a.hwnd, gwlStyle, a.savedStyle|wsVisible)
		pSetWindowPos.Call(a.hwnd, 0, uintptr(r.l), uintptr(r.t), uintptr(r.r-r.l), uintptr(r.b-r.t), 0x0020|0x0004)
	}
	a.fullscreen = !a.fullscreen
}

func (a *App) keyPress(vk uintptr) {
	if a.menu {
		switch vk {
		case 0x26:
			a.moveSel(-1)
			return
		case 0x28:
			a.moveSel(1)
			return
		case 0x25:
			a.adjSel(-1)
			return
		case 0x27, 0x0D:
			a.adjSel(1)
			return
		case 0x1B, 'O', 0x70:
			a.toggleMenu()
			return
		}
	}
	if vk == 0x0D { // Enter: start / stop the game
		a.toggleGame()
		return
	}
	if a.game != nil {
		switch vk {
		case 'W', 0x26:
			a.gameIn.Pressed = append(a.gameIn.Pressed, DirUp)
			return
		case 'S', 0x28:
			a.gameIn.Pressed = append(a.gameIn.Pressed, DirDown)
			return
		case 'A', 0x25:
			a.gameIn.Pressed = append(a.gameIn.Pressed, DirLeft)
			return
		case 'D', 0x27:
			a.gameIn.Pressed = append(a.gameIn.Pressed, DirRight)
			return
		case 0x20:
			a.gameIn.Space = true
			return
		case 0x08:
			a.gameIn.Clear = true
			return
		case 0x09, 'R':
			return
		}
	}
	switch vk {
	case 0x09: // Tab
		a.newScene()
	case 0x20: // Space
		a.space = !a.space
		a.newScene()
	case 0x1B: // Esc
		pDestroyWindow.Call(a.hwnd)
	case 'O', 0x70: // O or F1
		a.toggleMenu()
	case 'G':
		a.setGPU(a.glw == nil)
	case 'H':
		a.help, a.helpAuto = !a.help, false
	case 0x71: // F2: FPS counter
		a.st.ShowFPS = !a.st.ShowFPS
		a.save()
	case 'M':
		a.mute = !a.mute
		a.eng.SetMute(a.mute)
	case 'R':
		a.rateT = 1
		a.pl.X, a.pl.H = 0, a.sc.CamH
	case 'F', 0x7A: // F11
		a.toggleFullscreen()
	case 'P':
		a.shots++
		a.shotPath = filepath.Join(exeDir(), fmt.Sprintf(filePrefix+"_%08X_%02d.png", a.sc.Seed, a.shots))
	}
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	a := &app
	isMain := hwnd == a.hwnd
	switch uint32(msg) {
	case wmDestroy:
		if isMain {
			quit.Store(true)
			pPostQuitMessage.Call(0)
		}
		return 0
	case wmSize:
		if isMain {
			a.cw, a.ch = int(lp&0xFFFF), int(lp>>16&0xFFFF)
		}
		return 0
	case wmEraseBkgnd:
		return 1
	case wmKeyDown:
		if lp&(1<<30) == 0 && a.sc != nil {
			a.keyPress(wp)
		}
		a.keys[wp&255] = true
		return 0
	case wmKeyUp:
		a.keys[wp&255] = false
		return 0
	case wmSysKeyDown:
		if wp == 0x0D && lp&(1<<30) == 0 { // Alt+Enter
			a.toggleFullscreen()
			return 0
		}
	case wmKillFocus:
		if isMain {
			a.keys = [256]bool{}
		}
	case wmMouseMove:
		a.mx, a.my = float32(int16(lp)), float32(int16(lp>>16))
		a.mouseSeen = true
		return 0
	case wmLButtonDown, wmRButtonDown:
		pSetFocus.Call(a.hwnd)
		x, y := int(int16(lp)), int(int16(lp>>16))
		if a.sc == nil || a.cw == 0 {
			return 0
		}
		if a.menu {
			d := 1
			if uint32(msg) == wmRButtonDown {
				d = -1
			}
			a.menuClick(x, y, d)
			return 0
		}
		if uint32(msg) == wmRButtonDown {
			if !a.sc.Space {
				sc := *a.sc
				sc.P = RandomPalette(a.rng)
				sc.setTerrCols()
				a.sc = &sc
			}
			return 0
		}
		k := float32(a.iw) / float32(a.cw)
		px, pz := a.rd.Pick(float32(x)*k, float32(y)*k)
		if a.game != nil {
			a.gameIn.Click, a.gameIn.ClickX, a.gameIn.ClickZ = true, px, pz
			return 0
		}
		if len(a.pl.Rings) < 6 {
			a.pl.Rings = append(a.pl.Rings, Ring{px, pz, a.now()})
		}
		a.eng.Chime()
		return 0
	case wmMouseWheel:
		d := float32(int16(wp>>16)) / 120
		if a.menu {
			if d > 0 {
				a.moveSel(-1)
			} else {
				a.moveSel(1)
			}
			return 0
		}
		if a.game != nil {
			a.gameIn.Wheel += d
			return 0
		}
		a.pl.H = clampf(a.pl.H+d*0.4, 0.8, 14)
		return 0
	case wmEnterSizeMove:
		if isMain {
			pSetTimer.Call(hwnd, 1, 16, 0)
		}
	case wmExitSizeMove:
		if isMain {
			pKillTimer.Call(hwnd, 1)
		}
	case wmTimer:
		if a.sc != nil && isMain {
			a.frame()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// selftestStep drives a short hidden run that exercises everything and
// writes a log + screenshots next to the exe (run: vaporwave-endless-3d.exe -selftest).
func (a *App) selftestStep() {
	n := a.frameN
	if n%20 == 1 {
		r := "CPU"
		if a.glw != nil {
			r = "GPU"
		}
		a.logf("frame %d: %s %dx%d fps %.0f playpos %.0f beat %.2f scene %s", n, r, a.iw, a.ih, a.fps, a.posEst,
			a.eng.BeatAt(a.posEst), a.sc.Name)
	}
	shot := func(name string) { a.shotPath = filepath.Join(exeDir(), name) }
	switch n {
	case 30:
		shot(filePrefix + "_selftest1_cpu.png")
	case 32:
		a.keyPress(0x20)
	case 40:
		wndProc(a.hwnd, wmLButtonDown, 0, uintptr(400|450<<16))
	case 50:
		a.logf("enable GPU: %v (%s)", a.enableGPU(), a.gpuErr)
		a.noSave = true
	case 90:
		shot(filePrefix + "_selftest2_gpu.png")
	case 100:
		a.toggleMenu()
		a.menuSel = 1
		a.adjSel(1) // preset -> slowed + reverb
		a.st.ShowFPS = true
		a.st.Latency = 2
		a.applyAudio()
		a.logf("menu: preset %s, mix %+v", PresetNames[a.st.Preset], a.st.Mix())
	case 110:
		a.logf("reopen audio")
		a.closeAudio()
		a.openAudio()
	case 130:
		shot(filePrefix + "_selftest3_menu.png")
	case 140:
		a.toggleMenu()
		a.setGPU(false)
	case 170:
		shot(filePrefix + "_selftest4_cpu.png")
	case 180:
		a.toggleGame()
		a.logf("game started: %s, %.0f BPM", a.game.Sc.Name, a.eng.Tempo())
	case 185, 190, 195, 200:
		a.gameIn.Pressed = append(a.gameIn.Pressed, DirUp)
	case 230:
		shot(filePrefix + "_selftest5_game.png")
		a.logf("game: phase %d dist %.0f rings %d hits %d", a.game.Phase, a.game.Dist, len(a.game.Rings), a.game.Hits)
	case 240:
		a.toggleGame()
		a.logf("done")
		pDestroyWindow.Call(a.hwnd)
	}
}
