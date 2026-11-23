// Vaporwave Endless 3D for Meta Quest (WebXR) and desktop browsers.
// The Go core (ve3d.wasm) generates scenes, music and per-view geometry; this
// file draws it with WebGL 2 using the same shader passes as the desktop GPU
// renderer, runs the WebXR session and controllers, and plays the audio.
'use strict';

const $ = (id) => document.getElementById(id);
const params = new URLSearchParams(location.search);
const SR = 44100;
const canvas = $('c');
const gl = canvas.getContext('webgl2', {
  antialias: false, alpha: false, depth: false, xrCompatible: true, powerPreference: 'high-performance',
});
const go = new Go();
let ve3d = null;

function status(s) { $('status').textContent = s; }
function mem() { return go._inst.exports.mem.buffer; }

// ---------------------------------------------------------------------------
// WebGL 2 renderer (mirrors GLR.Frame in gl.go)

const R = {};

function compile(type, src, name) {
  const s = gl.createShader(type);
  gl.shaderSource(s, src);
  gl.compileShader(s);
  if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(name + ': ' + gl.getShaderInfoLog(s));
  return s;
}

function program(vs, fs, name) {
  const p = gl.createProgram();
  gl.attachShader(p, compile(gl.VERTEX_SHADER, vs, name + ' (vs)'));
  gl.attachShader(p, compile(gl.FRAGMENT_SHADER, fs, name));
  gl.linkProgram(p);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(name + ': ' + gl.getProgramInfoLog(p));
  return p;
}

function initRenderer() {
  const S = ve3d.shaders();
  R.nParams = S.nParams;
  R.bg = program(S.fsVS, S.bgFS, 'sky/floor');
  R.geo = program(S.geoVS, S.geoFS, 'geometry');
  R.down = program(S.fsVS, S.downFS, 'bloom down');
  R.blur = program(S.fsVS, S.blurFS, 'blur');
  R.comp = program(S.fsVS, S.compFS, 'composite');
  R.blit = program(S.fsVS, S.blitFS, 'blit');
  const loc = (p, n) => gl.getUniformLocation(p, n);
  R.bgU = loc(R.bg, 'U'); R.bgUI = loc(R.bg, 'UI');
  R.geoVP = loc(R.geo, 'vp');
  R.dirRad = loc(R.blur, 'dirRad');
  R.compP = loc(R.comp, 'P'); R.compF = loc(R.comp, 'F');
  R.blitP = loc(R.blit, 'P');
  const bind = (p, names) => { gl.useProgram(p); names.forEach((n, i) => gl.uniform1i(loc(p, n), i)); };
  bind(R.down, ['src']); bind(R.blur, ['src']); bind(R.comp, ['scene', 'bloomT', 'hud']); bind(R.blit, ['src']);

  R.vaoEmpty = gl.createVertexArray();
  R.vaoGeo = gl.createVertexArray();
  R.vbo = gl.createBuffer();
  gl.bindVertexArray(R.vaoGeo);
  gl.bindBuffer(gl.ARRAY_BUFFER, R.vbo);
  // 9 floats per vertex: x y invz | r g b a | edge distance, half width
  gl.enableVertexAttribArray(0); gl.vertexAttribPointer(0, 3, gl.FLOAT, false, 36, 0);
  gl.enableVertexAttribArray(1); gl.vertexAttribPointer(1, 4, gl.FLOAT, false, 36, 12);
  gl.enableVertexAttribArray(2); gl.vertexAttribPointer(2, 2, gl.FLOAT, false, 36, 28);
  gl.bindVertexArray(null);

  R.hdr = gl.getExtension('EXT_color_buffer_float')
    ? { i: gl.RGBA16F, f: gl.RGBA, t: gl.HALF_FLOAT }
    : { i: gl.RGBA8, f: gl.RGBA, t: gl.UNSIGNED_BYTE };
  R.ldr = { i: gl.RGBA8, f: gl.RGBA, t: gl.UNSIGNED_BYTE };
  R.w = 0; R.h = 0;
}

function makeTarget(w, h, fmt, filter, depth) {
  const t = { tex: gl.createTexture(), fbo: gl.createFramebuffer() };
  gl.bindTexture(gl.TEXTURE_2D, t.tex);
  gl.texImage2D(gl.TEXTURE_2D, 0, fmt.i, w, h, 0, fmt.f, fmt.t, null);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  gl.bindFramebuffer(gl.FRAMEBUFFER, t.fbo);
  gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.tex, 0);
  if (depth) {
    t.rb = gl.createRenderbuffer();
    gl.bindRenderbuffer(gl.RENDERBUFFER, t.rb);
    gl.renderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT24, w, h);
    gl.framebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, t.rb);
  }
  return t;
}

function freeTarget(t) {
  if (!t) return;
  gl.deleteTexture(t.tex); gl.deleteFramebuffer(t.fbo);
  if (t.rb) gl.deleteRenderbuffer(t.rb);
}

function ensureTargets(w, h) {
  if (w === R.w && h === R.h) return;
  [R.scene, R.b0, R.b1, R.compT].forEach(freeTarget);
  R.w = w; R.h = h;
  R.bw = (w + 3) >> 2; R.bh = (h + 3) >> 2;
  R.scene = makeTarget(w, h, R.hdr, gl.NEAREST, true);
  R.b0 = makeTarget(R.bw, R.bh, R.hdr, gl.LINEAR, false);
  R.b1 = makeTarget(R.bw, R.bh, R.hdr, gl.LINEAR, false);
  R.compT = makeTarget(w, h, R.ldr, gl.LINEAR, false);
}

function pass(fbo, w, h, prog) {
  gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
  gl.viewport(0, 0, w, h);
  gl.useProgram(prog);
}

function tex(unit, t) { gl.activeTexture(gl.TEXTURE0 + unit); gl.bindTexture(gl.TEXTURE_2D, t); }
function draw3() { gl.bindVertexArray(R.vaoEmpty); gl.drawArrays(gl.TRIANGLES, 0, 3); }

// renderView draws one prepared view (from ve3d.view) into a viewport of fbo.
function renderView(info, fbo, vx, vy, vw, vh) {
  const m = mem();
  const w = info.w, h = info.h;
  ensureTargets(w, h);

  // 1. sky + floor
  pass(R.scene.fbo, w, h, R.bg);
  gl.disable(gl.BLEND); gl.disable(gl.DEPTH_TEST); gl.depthMask(true);
  gl.clearBufferfv(gl.DEPTH, 0, [1]);
  gl.uniform4fv(R.bgU, new Float32Array(m, info.params, R.nParams * 4));
  gl.uniform4uiv(R.bgUI, new Uint32Array(info.ui));
  draw3();

  // 2. geometry
  if (info.geoFloats > 0 && info.nRuns > 0) {
    gl.useProgram(R.geo);
    gl.uniform2f(R.geoVP, w, h);
    gl.bindVertexArray(R.vaoGeo);
    gl.bindBuffer(gl.ARRAY_BUFFER, R.vbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(m, info.geo, info.geoFloats), gl.STREAM_DRAW);
    gl.enable(gl.DEPTH_TEST); gl.depthFunc(gl.LEQUAL);
    gl.enable(gl.BLEND); gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    const runs = new Int32Array(m, info.runs, info.nRuns * 3);
    const nv = info.geoFloats / 9;
    for (let i = 0; i < info.nRuns; i++) {
      const first = runs[3 * i], count = runs[3 * i + 1];
      if (count <= 0 || first + count > nv) continue;
      gl.depthMask(runs[3 * i + 2] !== 0);
      gl.drawArrays(gl.TRIANGLES, first, count);
    }
    gl.disable(gl.BLEND); gl.disable(gl.DEPTH_TEST); gl.depthMask(true);
  }

  // 3. bloom
  pass(R.b0.fbo, R.bw, R.bh, R.down);
  tex(0, R.scene.tex);
  draw3();
  gl.useProgram(R.blur);
  for (const [src, dst, dir, rad] of [[R.b0, R.b1, 0, 2], [R.b1, R.b0, 1, 2], [R.b0, R.b1, 0, 4], [R.b1, R.b0, 1, 4]]) {
    pass(dst.fbo, R.bw, R.bh, R.blur);
    gl.uniform2i(R.dirRad, dir, rad);
    tex(0, src.tex);
    draw3();
  }

  // 4. composite (tone curve, VHS effects)
  pass(R.compT.fbo, w, h, R.comp);
  tex(0, R.scene.tex); tex(1, R.b0.tex); tex(2, R.b0.tex);
  gl.uniform4fv(R.compP, new Float32Array(m, info.comp, 12));
  gl.uniform4uiv(R.compF, new Uint32Array([info.compF[0], 0, 0, 0]));
  draw3();

  // 5. scale into the target viewport
  gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
  gl.viewport(vx, vy, vw, vh);
  gl.useProgram(R.blit);
  tex(0, R.compT.tex);
  gl.uniform4f(R.blitP, vx, vy, vw, vh);
  draw3();
  tex(0, null); tex(1, null); tex(2, null);
}

// ---------------------------------------------------------------------------
// Audio: the Go synth renders on the main thread; an AudioWorklet plays it.

const WORKLET = `
class Out extends AudioWorkletProcessor {
  constructor() { super(); this.q = []; this.off = 0; this.done = 0; this.n = 0;
    this.port.onmessage = (e) => this.q.push(e.data); }
  process(inputs, outputs) {
    const o = outputs[0], L = o[0], R = o[1] || o[0], N = L.length;
    let i = 0;
    while (i < N && this.q.length) {
      const [l, r] = this.q[0];
      const k = Math.min(N - i, l.length - this.off);
      L.set(l.subarray(this.off, this.off + k), i);
      R.set(r.subarray(this.off, this.off + k), i);
      i += k; this.off += k;
      if (this.off >= l.length) { this.q.shift(); this.off = 0; }
    }
    if (i < N) { L.fill(0, i); R.fill(0, i); }
    this.done += i;
    if (++this.n % 4 === 0) this.port.postMessage(this.done);
    return true;
  }
}
registerProcessor('ve3d-out', Out);
`;

const A = { ctx: null, node: null, sent: 0, played: 0 };

async function startAudio() {
  if (A.ctx) { await A.ctx.resume(); return; }
  try {
    A.ctx = new AudioContext({ sampleRate: SR, latencyHint: 'interactive' });
    const url = URL.createObjectURL(new Blob([WORKLET], { type: 'application/javascript' }));
    await A.ctx.audioWorklet.addModule(url);
    A.node = new AudioWorkletNode(A.ctx, 've3d-out', { numberOfInputs: 0, outputChannelCount: [2] });
    A.node.port.onmessage = (e) => { A.played = e.data; };
    A.node.connect(A.ctx.destination);
    await A.ctx.resume();
  } catch (err) {
    console.warn('audio unavailable', err);
    A.node = null;
  }
}

// Keep about 120 ms of audio queued; without audio output, still advance the
// music clock (the visuals follow it).
function pumpAudio(dt) {
  if (!A.node) {
    const n = Math.min(8192, Math.round(dt * SR));
    if (n > 0) ve3d.audio(n);
    return;
  }
  const ahead = Math.round(0.12 * SR);
  for (let guard = 0; A.sent - A.played < ahead && guard < 12; guard++) {
    const n = 1024;
    const [pl, pr] = ve3d.audio(n);
    const m = mem();
    const l = new Float32Array(m, pl, n).slice(), r = new Float32Array(m, pr, n).slice();
    A.node.port.postMessage([l, r], [l.buffer, r.buffer]);
    A.sent += n;
  }
}

function latency() {
  if (!A.node) return 0;
  return (A.sent - A.played) / SR + (A.ctx.outputLatency || 0) + (A.ctx.baseLatency || 0);
}

// ---------------------------------------------------------------------------
// WebXR

let xr = null; // { session, ref, floorY }
let last = 0;
const prevButtons = new Map();

// WebXR is right-handed with -z forward; the scene uses x right, y up, z forward.
function eyeFrom(view, floorY) {
  const m = view.transform.matrix, p = view.projectionMatrix;
  return [
    m[0], m[1], -m[2], // right
    m[4], m[5], -m[6], // up
    -m[8], -m[9], m[10], // forward
    m[12], m[13] + floorY, -m[14], // position (metres)
    (1 - p[8]) / p[0], (1 + p[8]) / p[0], (1 + p[9]) / p[5], (1 - p[9]) / p[5], // tan left, right, up, down
  ];
}

function controllers(session, dt) {
  let sx = 0, sy = 0;
  for (const src of session.inputSources) {
    const gp = src.gamepad;
    if (!gp) continue;
    const key = src.handedness;
    const prev = prevButtons.get(key) || [];
    const now = gp.buttons.map((b) => b.pressed);
    const edge = (i) => now[i] && !prev[i];
    if (edge(0)) ve3d.newScene(); // trigger
    if (edge(1)) ve3d.tap(0.5, 0.72); // grip: shockwave
    if (edge(4)) ve3d.toggleSpace(); // A / X
    if (edge(5)) ve3d.recolour(); // B / Y
    prevButtons.set(key, now);
    if (gp.axes.length >= 4) {
      if (Math.abs(gp.axes[2]) > Math.abs(sx)) sx = gp.axes[2];
      if (Math.abs(gp.axes[3]) > Math.abs(sy)) sy = gp.axes[3];
    }
  }
  ve3d.strafe(Math.abs(sx) > 0.2 ? sx : 0);
  if (Math.abs(sy) > 0.25) ve3d.speed(-sy * dt * 0.6);
}

function xrFrame(t, frame) {
  const session = frame.session;
  session.requestAnimationFrame(xrFrame);
  const pose = frame.getViewerPose(xr.ref);
  const dt = last ? Math.min(0.1, (t - last) / 1000) : 1 / 72;
  last = t;
  if (!pose) return;
  controllers(session, dt);
  pumpAudio(dt);
  ve3d.frame(dt, latency());
  const layer = session.renderState.baseLayer;
  const scale = ve3d.scale() / 100;
  for (const view of pose.views) {
    const vp = layer.getViewport(view);
    const info = ve3d.view(eyeFrom(view, xr.floorY), Math.max(64, Math.round(vp.width * scale)), 0);
    renderView(info, layer.framebuffer, vp.x, vp.y, vp.width, vp.height);
  }
}

async function enterVR() {
  await startAudio();
  let session;
  try {
    session = await navigator.xr.requestSession('immersive-vr', { optionalFeatures: ['local-floor'] });
  } catch (err) {
    status('Could not start VR: ' + err.message);
    return;
  }
  await gl.makeXRCompatible();
  const layer = new XRWebGLLayer(session, gl, { antialias: false, depth: false, alpha: false, framebufferScaleFactor: 1.0 });
  if ('fixedFoveation' in layer) layer.fixedFoveation = 0.5;
  session.updateRenderState({ baseLayer: layer });
  let ref, floorY = 0;
  try { ref = await session.requestReferenceSpace('local-floor'); } catch {
    ref = await session.requestReferenceSpace('local');
    floorY = 1.6;
  }
  xr = { session, ref, floorY };
  last = 0;
  hideStart();
  session.addEventListener('end', () => {
    xr = null;
    last = 0;
    saveSettings();
    requestAnimationFrame(flatFrame);
  });
  session.requestAnimationFrame(xrFrame);
}

// ---------------------------------------------------------------------------
// Flat screen (desktop browser, or the Quest browser window before VR)

const keys = new Set();
let hudT = 0;
const eyeTest = params.has('eyetest');

function resizeCanvas() {
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const w = Math.max(1, Math.round(canvas.clientWidth * dpr)), h = Math.max(1, Math.round(canvas.clientHeight * dpr));
  if (canvas.width !== w || canvas.height !== h) { canvas.width = w; canvas.height = h; }
}

function flatFrame(t) {
  if (xr) return;
  requestAnimationFrame(flatFrame);
  const dt = last ? Math.min(0.1, (t - last) / 1000) : 1 / 60;
  last = t;
  ve3d.strafe((keys.has('KeyD') || keys.has('ArrowRight') ? 1 : 0) - (keys.has('KeyA') || keys.has('ArrowLeft') ? 1 : 0));
  const sp = (keys.has('KeyW') || keys.has('ArrowUp') ? 1 : 0) - (keys.has('KeyS') || keys.has('ArrowDown') ? 1 : 0);
  if (sp) ve3d.speed(sp * dt * 0.6);
  pumpAudio(dt);
  ve3d.frame(dt, latency());
  resizeCanvas();
  const cw = canvas.width, ch = canvas.height, scale = ve3d.scale() / 100;
  if (eyeTest) {
    // a fake headset: two eyes side by side with asymmetric fields of view, looking around
    const yaw = params.has('yaw') ? +params.get('yaw') : Math.sin(t / 3000) * 1.2, pitch = Math.sin(t / 4100) * 0.3;
    const roll = +(params.get('roll') || 0);
    const F = [Math.sin(yaw) * Math.cos(pitch), Math.sin(pitch), Math.cos(yaw) * Math.cos(pitch)];
    const R0 = [Math.cos(yaw), 0, -Math.sin(yaw)];
    const U0 = [F[1] * R0[2] - F[2] * R0[1], F[2] * R0[0] - F[0] * R0[2], F[0] * R0[1] - F[1] * R0[0]];
    const cr = Math.cos(roll), sr = Math.sin(roll);
    const Rv = R0.map((v, i) => v * cr + U0[i] * sr), U = U0.map((v, i) => v * cr - R0[i] * sr);
    for (const side of [0, 1]) {
      const s = side ? 0.032 : -0.032;
      const e = [...Rv, ...U, ...F, Rv[0] * s, 1.6 + Rv[1] * s, Rv[2] * s, side ? 0.85 : 1.0, side ? 1.0 : 0.85, 0.95, 1.05];
      const info = ve3d.view(e, Math.max(64, Math.round(cw / 2 * scale)), 0);
      const vh = Math.round(cw / 2 * info.h / info.w);
      renderView(info, null, side * (cw >> 1), (ch - vh) >> 1, cw >> 1, vh);
    }
  } else {
    const info = ve3d.view(null, Math.max(16, Math.round(cw * scale)), Math.max(16, Math.round(ch * scale)));
    renderView(info, null, 0, 0, cw, ch);
  }
  if (t - hudT > 100) { hudT = t; updateHUD(); }
}

function updateHUD() {
  const [title, info, toast, age, showFPS] = ve3d.text();
  $('title').textContent = title;
  $('title').style.opacity = age < 6 ? 1 : Math.max(0, 1 - (age - 6) / 2);
  $('info').textContent = info;
  $('toast').textContent = toast;
  $('fps').classList.toggle('hidden', !showFPS);
  if (showFPS) $('fps').textContent = fps.value + ' FPS · WebGL 2 · ' + R.w + 'x' + R.h;
}

const fps = { value: 0, n: 0, t0: 0 };
function countFPS(t) {
  fps.n++;
  if (t - fps.t0 > 500) { fps.value = Math.round(fps.n * 1000 / (t - fps.t0)); fps.n = 0; fps.t0 = t; }
  requestAnimationFrame(countFPS);
}

function hideStart() {
  $('start').classList.add('hidden');
  ['hud', 'toast', 'bar'].forEach((id) => $(id).classList.remove('hidden'));
}

function startFlat() {
  hideStart();
  last = 0;
  requestAnimationFrame(flatFrame);
}

// ---------------------------------------------------------------------------
// Settings (rows come from the Go core)

function saveSettings() {
  try { localStorage.setItem('ve3d-settings', ve3d.settings()); } catch { /* private mode */ }
}

function renderMenu() {
  const el = $('menu');
  el.innerHTML = '';
  ve3d.menu().forEach(([label, value, header], i) => {
    if (header) {
      const h = document.createElement('h3');
      h.textContent = label;
      el.appendChild(h);
      return;
    }
    const row = document.createElement('div');
    row.className = 'row';
    const l = document.createElement('span');
    l.className = 'l';
    l.textContent = label;
    row.appendChild(l);
    const btn = (text, d) => {
      const b = document.createElement('button');
      b.textContent = text;
      b.onclick = () => { ve3d.menuAdjust(i, d); saveSettings(); renderMenu(); };
      row.appendChild(b);
    };
    if (value === 'Go' || value === 'Reset') {
      btn(value, 1);
    } else {
      btn('−', -1);
      const v = document.createElement('span');
      v.className = 'v';
      v.textContent = value;
      row.appendChild(v);
      btn('+', 1);
    }
    el.appendChild(row);
  });
}

function toggleMenu() {
  const el = $('menu');
  const show = el.style.display !== 'block';
  el.style.display = show ? 'block' : 'none';
  if (show) renderMenu();
}

// ---------------------------------------------------------------------------
// Input and start-up

function setupInput() {
  let down = null;
  canvas.addEventListener('pointerdown', (e) => { down = { x: e.clientX, y: e.clientY, lx: e.clientX, ly: e.clientY, moved: false }; canvas.setPointerCapture(e.pointerId); });
  canvas.addEventListener('pointermove', (e) => {
    if (!down) return;
    const dx = e.clientX - down.lx, dy = e.clientY - down.ly;
    down.lx = e.clientX; down.ly = e.clientY;
    if (Math.abs(e.clientX - down.x) + Math.abs(e.clientY - down.y) > 6) down.moved = true;
    ve3d.look(dx / canvas.clientWidth, dy / canvas.clientHeight);
  });
  canvas.addEventListener('pointerup', (e) => {
    if (down && !down.moved) ve3d.tap(e.clientX / canvas.clientWidth, e.clientY / canvas.clientHeight);
    down = null;
  });
  canvas.addEventListener('dblclick', () => ve3d.newScene());
  window.addEventListener('keydown', (e) => {
    if ($('start').classList.contains('hidden') === false) return;
    keys.add(e.code);
    if (e.code === 'Tab') { e.preventDefault(); ve3d.newScene(); }
    if (e.code === 'Space') { e.preventDefault(); ve3d.toggleSpace(); }
    if (e.code === 'KeyC') ve3d.recolour();
    if (e.code === 'KeyO' || e.code === 'Escape') toggleMenu();
  });
  window.addEventListener('keyup', (e) => keys.delete(e.code));
  window.addEventListener('pagehide', saveSettings);
  $('bNew').onclick = () => ve3d.newScene();
  $('bSpace').onclick = () => ve3d.toggleSpace();
  $('bMenu').onclick = toggleMenu;
  $('bVR').onclick = enterVR;
  $('enterVR').onclick = enterVR;
  $('enterFlat').onclick = async () => { await startAudio(); startFlat(); };
}

async function main() {
  if (!gl) { status('This browser does not support WebGL 2.'); return; }
  try {
    globalThis.ve3dReady = () => { ve3d = globalThis.ve3d; };
    const bytes = await (await fetch('ve3d.wasm')).arrayBuffer();
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    go.run(instance);
    let saved = '';
    try { saved = localStorage.getItem('ve3d-settings') || ''; } catch { /* private mode */ }
    ve3d.init(saved);
    initRenderer();
  } catch (err) {
    console.error(err);
    status('Failed to start: ' + err.message);
    return;
  }
  setupInput();
  requestAnimationFrame(countFPS);
  if (navigator.xr && await navigator.xr.isSessionSupported('immersive-vr').catch(() => false)) {
    $('enterVR').classList.remove('hidden');
    $('bVR').classList.remove('hidden');
    status('Ready. Use the controllers: trigger = new scene, A/X = space, B/Y = new colours, grip = pulse, stick = slide & tape speed.');
  } else {
    status('Ready. No VR headset found, so it will play on this screen.');
  }
  if (params.has('autostart') || eyeTest) startFlat();
}

main();
