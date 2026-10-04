// Tiny offline synth used by the flow-video soundtracks: a song buffer with reverb
// and delay sends, a generic subtractive voice, an FM electric piano, bells and
// a drum kit. Everything is deterministic (seeded noise) so builds are identical.

import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname } from 'node:path'

export const SR = 44100
export const midi = (m) => 440 * Math.pow(2, (m - 69) / 12)

const rng = (seed) => {
  let s = seed >>> 0
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return s / 4294967296
  }
}

// RBJ biquad; coefficients can be updated on the fly for sweeps.
export class Biquad {
  constructor(type, freq, q = 0.707) {
    this.type = type
    this.x1 = this.x2 = this.y1 = this.y2 = 0
    this.set(freq, q)
  }
  set(freq, q = this.q) {
    this.q = q
    const w = (2 * Math.PI * Math.max(20, Math.min(freq, SR * 0.45))) / SR
    const cos = Math.cos(w)
    const alpha = Math.sin(w) / (2 * q)
    let b0, b1, b2
    if (this.type === 'lp') {
      b0 = (1 - cos) / 2
      b1 = 1 - cos
      b2 = (1 - cos) / 2
    } else if (this.type === 'hp') {
      b0 = (1 + cos) / 2
      b1 = -(1 + cos)
      b2 = (1 + cos) / 2
    } else {
      b0 = alpha
      b1 = 0
      b2 = -alpha
    }
    const a0 = 1 + alpha
    this.b0 = b0 / a0
    this.b1 = b1 / a0
    this.b2 = b2 / a0
    this.a1 = (-2 * cos) / a0
    this.a2 = (1 - alpha) / a0
  }
  run(x) {
    const y = this.b0 * x + this.b1 * this.x1 + this.b2 * this.x2 - this.a1 * this.y1 - this.a2 * this.y2
    this.x2 = this.x1
    this.x1 = x
    this.y2 = this.y1
    this.y1 = y
    return y
  }
}

const blep = (t, dt) => {
  if (t < dt) {
    t /= dt
    return t + t - t * t - 1
  }
  if (t > 1 - dt) {
    t = (t - 1) / dt
    return t * t + t + t + 1
  }
  return 0
}

const wave = (type, ph, dt, pw = 0.5) => {
  switch (type) {
    case 'sine':
      return Math.sin(2 * Math.PI * ph)
    case 'tri':
      return 1 - 4 * Math.abs(ph - 0.5)
    case 'square': {
      let s = ph < pw ? 1 : -1
      s += blep(ph, dt)
      let p2 = ph - pw
      if (p2 < 0) p2 += 1
      s -= blep(p2, dt)
      return s
    }
    default:
      return 2 * ph - 1 - blep(ph, dt)
  }
}

class Comb {
  constructor(n) {
    this.buf = new Float32Array(n)
    this.i = 0
    this.store = 0
  }
  run(x, fb, damp) {
    const y = this.buf[this.i]
    this.store = y * (1 - damp) + this.store * damp
    this.buf[this.i] = x + this.store * fb
    if (++this.i >= this.buf.length) this.i = 0
    return y
  }
}

class Allpass {
  constructor(n) {
    this.buf = new Float32Array(n)
    this.i = 0
  }
  run(x) {
    const b = this.buf[this.i]
    this.buf[this.i] = x + b * 0.5
    if (++this.i >= this.buf.length) this.i = 0
    return b - x
  }
}

const COMBS = [1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617]
const ALLPASS = [556, 441, 341, 225]

export class Song {
  constructor({ bpm, beats, seed = 7 }) {
    this.bpm = bpm
    this.beat = 60 / bpm
    this.beats = beats
    this.N = Math.ceil(beats * this.beat * SR)
    const mk = () => new Float32Array(this.N)
    this.L = mk()
    this.R = mk()
    this.vL = mk()
    this.vR = mk()
    this.dL = mk()
    this.dR = mk()
    this.duck = mk().fill(1)
    this.rnd = rng(seed)
  }
  s(b) {
    return Math.round(b * this.beat * SR)
  }
  noise() {
    return this.rnd() * 2 - 1
  }
  add(i, l, r, verb = 0, dly = 0) {
    if (i < 0 || i >= this.N) return
    this.L[i] += l
    this.R[i] += r
    if (verb) {
      this.vL[i] += l * verb
      this.vR[i] += r * verb
    }
    if (dly) {
      this.dL[i] += l * dly
      this.dR[i] += r * dly
    }
  }
  /** Sidechain dip starting at beat b (used by voices with `duck`). */
  duckAt(b, depth = 0.65, rate = 9) {
    const s0 = this.s(b)
    for (let i = 0; i < SR * this.beat && s0 + i < this.N; i++) {
      const v = 1 - depth * Math.exp(-(i / SR) * rate)
      if (v < this.duck[s0 + i]) this.duck[s0 + i] = v
    }
  }

  /**
   * Generic subtractive voice.
   * oscs: [{ type, det (cents), mult, gain, pan, pw }]
   * cut: Hz | { base, amt, dec } (filter envelope) | (tSec, beatAbs) => Hz
   */
  note(b, len, m, o = {}) {
    const {
      oscs = [{ type: 'saw' }],
      gain = 0.1,
      a = 0.005,
      d = 0.1,
      s = 1,
      r = 0.08,
      cut = null,
      q = 0.707,
      vib = null,
      wob = null,
      verb = 0,
      dly = 0,
      duck = 0,
      pan = 0.5,
      drive = 0,
    } = o
    const f0 = midi(m)
    const s0 = this.s(b)
    const lenS = len * this.beat
    const total = Math.ceil((lenS + r) * SR)
    const vs = oscs.map((v) => ({ ph: this.rnd(), ...v }))
    const fl = cut ? new Biquad('lp', 1000, q) : null
    const fr = cut ? new Biquad('lp', 1000, q) : null
    const cutAt = (t) =>
      typeof cut === 'number' ? cut : typeof cut === 'function' ? cut(t, b + t / this.beat) : cut.base + cut.amt * Math.exp(-t * cut.dec)
    let relFrom = null
    for (let i = 0; i < total; i++) {
      const idx = s0 + i
      if (idx >= this.N) break
      const t = i / SR
      let env
      if (t < a) env = t / a
      else if (t < a + d) env = 1 - ((1 - s) * (t - a)) / d
      else env = s
      if (t >= lenS) {
        if (relFrom === null) relFrom = env
        env = relFrom * Math.max(0, 1 - (t - lenS) / r)
      }
      let cents = 0
      if (vib) cents += vib.depth * Math.sin(2 * Math.PI * vib.rate * t) * Math.min(1, t / (vib.delay || 0.001))
      if (wob) cents += wob((idx) / SR)
      const f = f0 * Math.pow(2, cents / 1200)
      if (fl && i % 32 === 0) {
        const c = cutAt(t)
        fl.set(c)
        fr.set(c)
      }
      let l = 0
      let rr = 0
      for (const v of vs) {
        const fv = f * (v.mult || 1) * Math.pow(2, (v.det || 0) / 1200)
        const dt = fv / SR
        v.ph += dt
        v.ph -= Math.floor(v.ph)
        const x = wave(v.type || 'saw', v.ph, dt, v.pw) * (v.gain ?? 1)
        const p = v.pan ?? pan
        l += x * Math.cos((p * Math.PI) / 2)
        rr += x * Math.sin((p * Math.PI) / 2)
      }
      if (fl) {
        l = fl.run(l)
        rr = fr.run(rr)
      }
      if (drive) {
        l = Math.tanh(l * drive) / Math.tanh(drive)
        rr = Math.tanh(rr * drive) / Math.tanh(drive)
      }
      const dk = duck ? 1 - duck * (1 - this.duck[idx]) : 1
      const g = env * gain * dk * 1.41
      this.add(idx, l * g, rr * g, verb, dly)
    }
  }

  /** FM electric piano (Rhodes-ish): sine carrier, decaying modulator + tine. */
  epiano(b, len, m, o = {}) {
    const { gain = 0.1, verb = 0.2, dly = 0, vel = 1, pan = 0.5, wob = null, lp = null, offset = 0, duck = 0 } = o
    const f0 = midi(m)
    const s0 = this.s(b) + Math.round(offset * SR)
    const lenS = len * this.beat
    const rel = 0.35
    const total = Math.ceil((lenS + rel) * SR)
    const fl = lp ? new Biquad('lp', 1000, 0.8) : null
    const fr = lp ? new Biquad('lp', 1000, 0.8) : null
    let pc = 0
    let pm = 0
    let pt = 0
    for (let i = 0; i < total; i++) {
      const idx = s0 + i
      if (idx >= this.N || idx < 0) break
      const t = i / SR
      if (fl && i % 64 === 0) {
        const c = typeof lp === 'function' ? lp(b + t / this.beat) : lp
        fl.set(c)
        fr.set(c)
      }
      const cents = wob ? wob(idx / SR) : 0
      const f = f0 * Math.pow(2, cents / 1200)
      pm += f / SR
      pt += (f * 14) / SR
      pc += f / SR
      const idxMod = (0.9 + vel * 1.4) * Math.exp(-t * 3.5) + 0.25
      const tine = Math.sin(2 * Math.PI * pt) * Math.exp(-t * 40) * 0.35 * vel
      const x = Math.sin(2 * Math.PI * pc + idxMod * Math.sin(2 * Math.PI * pm) + tine)
      let env = Math.exp(-t * (0.9 + f0 / 900)) * Math.min(1, t / 0.002)
      if (t > lenS) env *= Math.max(0, 1 - (t - lenS) / rel)
      const trem = 1 + 0.12 * Math.sin(2 * Math.PI * 4.2 * (idx / SR))
      let l = x * env * gain * vel * trem
      let r = x * env * gain * vel * (2 - trem)
      if (fl) {
        l = fl.run(l)
        r = fr.run(r)
      }
      const dk = duck ? 1 - duck * (1 - this.duck[idx]) : 1
      this.add(idx, l * (1.2 - pan * 0.4) * dk, r * (0.8 + pan * 0.4) * dk, verb, dly)
    }
  }

  /** Inharmonic bell / glass ping. */
  bell(b, m, o = {}) {
    const { gain = 0.08, verb = 0.5, dly = 0, len = 3, pan = 0.5 } = o
    const f0 = midi(m)
    const s0 = this.s(b)
    const parts = [
      [1, 1, 1.4],
      [2.76, 0.45, 2.4],
      [5.4, 0.25, 4],
      [8.93, 0.12, 6],
    ]
    const ph = parts.map(() => 0)
    for (let i = 0; i < len * SR; i++) {
      const idx = s0 + i
      if (idx >= this.N) break
      const t = i / SR
      let x = 0
      parts.forEach(([mul, g, dec], k) => {
        ph[k] += (f0 * mul) / SR
        x += Math.sin(2 * Math.PI * ph[k]) * g * Math.exp(-t * dec)
      })
      x *= gain * Math.min(1, t / 0.003)
      this.add(idx, x * (1.2 - pan * 0.4), x * (0.8 + pan * 0.4), verb, dly)
    }
  }

  // --- Drums -------------------------------------------------------------------
  kick(b, o = {}) {
    const { gain = 1, f0 = 160, f1 = 45, pd = 28, dec = 7, click = 0.3, drive = 1.6, len = 0.45 } = o
    const s0 = this.s(b)
    let ph = 0
    for (let i = 0; i < SR * len; i++) {
      const t = i / SR
      ph += (f1 + (f0 - f1) * Math.exp(-t * pd)) / SR
      const c = i < 60 ? this.noise() * click * (1 - i / 60) : 0
      const x = (Math.tanh(Math.sin(2 * Math.PI * ph) * drive) * Math.exp(-t * dec) + c) * 0.55 * gain
      this.add(s0 + i, x, x)
    }
  }
  snare(b, o = {}) {
    const { gain = 1, tone = 185, dec = 14, lp = 7000, hp = 900, verb = 0.15, len = 0.32, pan = 0.5 } = o
    const s0 = this.s(b)
    const fh = new Biquad('hp', hp, 0.7)
    const fl = new Biquad('lp', lp, 0.7)
    let ph = 0
    for (let i = 0; i < SR * len; i++) {
      const t = i / SR
      ph += tone * (1 + 0.5 * Math.exp(-t * 40)) / SR
      const body = Math.sin(2 * Math.PI * ph) * Math.exp(-t * 28) * 0.5
      const nz = fl.run(fh.run(this.noise())) * Math.exp(-t * dec)
      const x = (body + nz) * 0.33 * gain
      this.add(s0 + i, x * (1.2 - pan * 0.4), x * (0.8 + pan * 0.4), verb)
    }
  }
  clap(b, o = {}) {
    const { gain = 1, pitch = 1, verb = 0.18 } = o
    const s0 = this.s(b)
    const bp = new Biquad('bp', 1400 * pitch, 1.1)
    const hp = new Biquad('hp', 300, 0.7)
    for (let i = 0; i < SR * 0.25; i++) {
      const t = i / SR
      const burst = t < 0.03 ? (Math.floor(t / 0.01) % 2 === 0 ? 1 : 0.4) : 1
      const x = hp.run(bp.run(this.noise())) * Math.exp(-t * 16) * burst * 0.45 * gain
      this.add(s0 + i, x, x * 0.94, verb)
    }
  }
  hat(b, o = {}) {
    const { gain = 1, open = false, pan = 0.5, hp = 7500, verb = 0 } = o
    const s0 = this.s(b)
    const f = new Biquad('hp', hp, 0.8)
    const dec = open ? 9 : 45
    for (let i = 0; i < SR * (open ? 0.3 : 0.06); i++) {
      const t = i / SR
      const x = f.run(this.noise()) * Math.exp(-t * dec) * 0.16 * gain
      this.add(s0 + i, x * (1 - pan) * 2, x * pan * 2, verb)
    }
  }
  tom(b, o = {}) {
    const { gain = 1, f = 90, dec = 6, verb = 0.25, pan = 0.5 } = o
    const s0 = this.s(b)
    const fl = new Biquad('lp', 1200, 0.7)
    let ph = 0
    for (let i = 0; i < SR * 0.7; i++) {
      const t = i / SR
      ph += (f * (1 + 0.6 * Math.exp(-t * 18))) / SR
      const x = (Math.tanh(Math.sin(2 * Math.PI * ph) * 1.8) * Math.exp(-t * dec) + fl.run(this.noise()) * 0.25 * Math.exp(-t * 25)) * 0.45 * gain
      this.add(s0 + i, x * (1.2 - pan * 0.4), x * (0.8 + pan * 0.4), verb)
    }
  }
  tick(b, o = {}) {
    const { gain = 1, freq = 3500, pan = 0.5, verb = 0.2 } = o
    const s0 = this.s(b)
    const f = new Biquad('bp', freq, 4)
    for (let i = 0; i < SR * 0.04; i++) {
      const t = i / SR
      const x = f.run(this.noise()) * Math.exp(-t * 120) * 0.5 * gain
      this.add(s0 + i, x * (1 - pan) * 2, x * pan * 2, verb)
    }
  }
  crash(b, o = {}) {
    const { gain = 1, len = 2.2, verb = 0.25 } = o
    const s0 = this.s(b)
    const hl = new Biquad('hp', 4000, 0.6)
    const hr = new Biquad('hp', 4200, 0.6)
    for (let i = 0; i < SR * len; i++) {
      const t = i / SR
      const e = Math.exp(-t * 2.2) * 0.2 * gain
      this.add(s0 + i, hl.run(this.noise()) * e, hr.run(this.noise()) * e, verb)
    }
  }
  boom(b, o = {}) {
    const { gain = 1, len = 1.8, f0 = 92, f1 = 32, verb = 0.1 } = o
    const s0 = this.s(b)
    let ph = 0
    for (let i = 0; i < SR * len; i++) {
      const t = i / SR
      ph += (f1 + (f0 - f1) * Math.exp(-t * 6)) / SR
      const x = Math.sin(2 * Math.PI * ph) * Math.exp(-t * 2.4) * 0.55 * gain
      this.add(s0 + i, x, x, verb)
    }
  }
  riser(b0, b1, o = {}) {
    const { gain = 1, verb = 0.3 } = o
    const s0 = this.s(b0)
    const s1 = this.s(b1)
    const bp = new Biquad('bp', 300, 1.5)
    let ph = 0
    for (let i = s0; i < s1; i++) {
      const p = (i - s0) / (s1 - s0)
      if (i % 32 === 0) bp.set(250 * Math.pow(40, p), 1.8)
      ph += (180 + 900 * p * p) / SR
      const x = bp.run(this.noise()) * 0.9 + Math.sin(2 * Math.PI * ph) * 0.05
      const e = p * p * 0.26 * gain
      this.add(i, x * e * (0.7 + 0.3 * Math.sin(p * 40)), x * e * (0.7 - 0.3 * Math.sin(p * 40)), verb)
    }
  }
  /** Dust: low hiss plus random pops. */
  vinyl(b0, b1, o = {}) {
    const { gain = 1 } = o
    const fl = new Biquad('lp', 2800, 0.7)
    const fh = new Biquad('hp', 400, 0.7)
    let pop = 0
    for (let i = this.s(b0); i < Math.min(this.N, this.s(b1)); i++) {
      const hiss = fh.run(fl.run(this.noise())) * 0.012
      if (this.rnd() < 9 / SR) pop = (this.rnd() * 0.5 + 0.2) * (this.rnd() < 0.5 ? -1 : 1)
      const x = (hiss + pop) * gain
      pop *= 0.82
      this.add(i, x, x * (0.9 + 0.2 * this.rnd()))
    }
  }

  // --- Mixdown -------------------------------------------------------------------
  render({ delayBeats = 0.75, delayFb = 0.42, delayLp = 3500, room = 0.84, damp = 0.35, verbWet = 1, lp = null, drive = 1.2, fadeIn = 0.03, fadeOutBeats = 4 } = {}) {
    const { N, L, R } = this
    // ping-pong delay -> mix (+ a little into the reverb)
    const dn = this.s(delayBeats)
    const bl = new Float32Array(N)
    const br = new Float32Array(N)
    const fbl = new Biquad('lp', delayLp, 0.7)
    const fbr = new Biquad('lp', delayLp, 0.7)
    for (let i = 0; i < N; i++) {
      const inL = i >= dn ? fbl.run(br[i - dn]) * delayFb : 0
      const inR = i >= dn ? fbr.run(bl[i - dn]) * delayFb : 0
      bl[i] = this.dL[i] + inL
      br[i] = this.dR[i] * 0.3 + inR
      const ol = i >= dn ? bl[i - dn] : 0
      const or = i >= dn ? br[i - dn] : 0
      L[i] += ol * 0.7
      R[i] += or * 0.7
      this.vL[i] += ol * 0.25
      this.vR[i] += or * 0.25
    }
    // freeverb
    const cl = COMBS.map((n) => new Comb(n))
    const cr = COMBS.map((n) => new Comb(n + 23))
    const al = ALLPASS.map((n) => new Allpass(n))
    const ar = ALLPASS.map((n) => new Allpass(n + 23))
    const pre = new Biquad('hp', 250, 0.7)
    for (let i = 0; i < N; i++) {
      const x = pre.run(this.vL[i] + this.vR[i]) * 0.015
      let ol = 0
      let or = 0
      for (let k = 0; k < 8; k++) {
        ol += cl[k].run(x, room, damp)
        or += cr[k].run(x, room, damp)
      }
      for (let k = 0; k < 4; k++) {
        ol = al[k].run(ol)
        or = ar[k].run(or)
      }
      L[i] += ol * 3 * verbWet
      R[i] += or * 3 * verbWet
    }
    // master
    const ml = lp ? new Biquad('lp', lp, 0.7) : null
    const mr = lp ? new Biquad('lp', lp, 0.7) : null
    const fadeOutS = this.s(this.beats - fadeOutBeats)
    // gain-stage so the soft clipper sees the same level on every song
    let raw = 0
    for (let i = 0; i < N; i++) raw = Math.max(raw, Math.abs(L[i]), Math.abs(R[i]))
    const stage = 1.15 / raw
    let peak = 0
    let sum = 0
    for (let i = 0; i < N; i++) {
      let l = L[i] * stage
      let r = R[i] * stage
      if (ml) {
        l = ml.run(l)
        r = mr.run(r)
      }
      const fi = Math.min(1, i / (SR * fadeIn))
      const fo = i > fadeOutS ? Math.max(0, 1 - (i - fadeOutS) / (N - fadeOutS)) : 1
      L[i] = Math.tanh(l * drive) * fi * fo
      R[i] = Math.tanh(r * drive) * fi * fo
      peak = Math.max(peak, Math.abs(L[i]), Math.abs(R[i]))
      sum += L[i] * L[i] + R[i] * R[i]
    }
    const norm = 0.89 / peak
    for (let i = 0; i < N; i++) {
      L[i] *= norm
      R[i] *= norm
    }
    const rms = Math.sqrt(sum / (2 * N)) * norm
    return { peak, rmsDb: 20 * Math.log10(rms) }
  }

  write(path) {
    const { N, L, R } = this
    const buf = Buffer.alloc(44 + N * 4)
    buf.write('RIFF', 0)
    buf.writeUInt32LE(36 + N * 4, 4)
    buf.write('WAVE', 8)
    buf.write('fmt ', 12)
    buf.writeUInt32LE(16, 16)
    buf.writeUInt16LE(1, 20)
    buf.writeUInt16LE(2, 22)
    buf.writeUInt32LE(SR, 24)
    buf.writeUInt32LE(SR * 4, 28)
    buf.writeUInt16LE(4, 32)
    buf.writeUInt16LE(16, 34)
    buf.write('data', 36)
    buf.writeUInt32LE(N * 4, 40)
    for (let i = 0; i < N; i++) {
      buf.writeInt16LE(Math.round(Math.max(-1, Math.min(1, L[i])) * 32767), 44 + i * 4)
      buf.writeInt16LE(Math.round(Math.max(-1, Math.min(1, R[i])) * 32767), 46 + i * 4)
    }
    mkdirSync(dirname(path), { recursive: true })
    writeFileSync(path, buf)
  }
}
