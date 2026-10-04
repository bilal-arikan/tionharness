// Synthesises the promo soundtrack (original, license-free) into public/music.wav.
//
// 120 BPM, 78 beats (39 s) -- one beat is exactly 15 video frames at 30 fps, so the
// scene cuts in src/timeline.ts land on kicks. Song map (in beats):
//   0-8   intro: filtered pad + arp opening up, noise riser
//   8     drop: impact + full groove (Am F C G)
//   40-64 groove B: extra 16th hats + lead arp octave
//   64-68 snare roll + riser (theme swatches)
//   68    final hit (logo), drums out, pad rings and fades to 78

import { writeFileSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const SR = 44100
const BPM = 120
const BEAT = 60 / BPM
const TOTAL_BEATS = 78
const N = Math.ceil(TOTAL_BEATS * BEAT * SR)
const L = new Float32Array(N)
const R = new Float32Array(N)

const midi = (m) => 440 * Math.pow(2, (m - 69) / 12)
const beatToSample = (b) => Math.round(b * BEAT * SR)

// Deterministic noise so every build of the track is identical.
let seed = 1337
const rnd = () => {
  seed = (seed * 1664525 + 1013904223) >>> 0
  return seed / 4294967296
}
const noise = () => rnd() * 2 - 1

// RBJ biquad; coefficients can be updated on the fly for sweeps.
class Biquad {
  constructor(type, freq, q = 0.707) {
    this.type = type
    this.x1 = this.x2 = this.y1 = this.y2 = 0
    this.set(freq, q)
  }
  set(freq, q = this.q) {
    this.q = q
    const w = (2 * Math.PI * Math.min(freq, SR * 0.45)) / SR
    const cos = Math.cos(w)
    const alpha = Math.sin(w) / (2 * q)
    let b0, b1, b2
    const a0 = 1 + alpha
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

// --- Harmony -----------------------------------------------------------------
// Intro bars play C, G so the drop resolves onto Am.
const CHORDS = {
  Am: { bass: 45, pad: [57, 60, 64, 69, 72] },
  F: { bass: 41, pad: [53, 57, 60, 65, 69] },
  C: { bass: 48, pad: [55, 60, 64, 67, 72] },
  G: { bass: 43, pad: [55, 59, 62, 67, 71] },
}
const PROG = ['Am', 'F', 'C', 'G']
const chordAtBeat = (b) => {
  const bar = Math.floor(b / 4)
  if (bar >= 17) return CHORDS.Am
  return CHORDS[PROG[(((bar - 2) % 4) + 4) % 4]]
}

// --- Sidechain (ducking from the kick) ----------------------------------------
const kickBeats = []
for (let b = 8; b < 68; b++) kickBeats.push(b)
kickBeats.push(68)
const duck = new Float32Array(N).fill(1)
for (const b of kickBeats) {
  const s0 = beatToSample(b)
  for (let i = 0; i < SR * BEAT && s0 + i < N; i++) {
    const t = i / SR
    duck[s0 + i] = Math.min(duck[s0 + i], 1 - 0.7 * Math.exp(-t * 9))
  }
}

const add = (i, l, r = l) => {
  if (i >= 0 && i < N) {
    L[i] += l
    R[i] += r
  }
}

// --- Pad: detuned saws through a sweeping lowpass ------------------------------
{
  const voices = []
  for (let v = 0; v < 5; v++)
    for (const det of [-0.11, 0, 0.12]) voices.push({ v, det, ph: rnd(), pan: det === 0 ? 0.5 : det < 0 ? 0.15 : 0.85 })
  const lpL = new Biquad('lp', 400, 0.9)
  const lpR = new Biquad('lp', 400, 0.9)
  for (let i = 0; i < N; i++) {
    const b = i / SR / BEAT
    if (i % 64 === 0) {
      let cut
      if (b < 8) cut = 300 + Math.pow(b / 8, 2.2) * 3200
      else if (b < 64) cut = 2600 + 600 * Math.sin((b / 16) * Math.PI)
      else if (b < 68) cut = 1500 + ((b - 64) / 4) * 3000
      else cut = 3200 * Math.exp(-(b - 68) * 0.18) + 500
      lpL.set(cut)
      lpR.set(cut)
    }
    const chord = chordAtBeat(b)
    let sl = 0
    let sr = 0
    for (const vo of voices) {
      const f = midi(chord.pad[vo.v] + vo.det)
      vo.ph += f / SR
      vo.ph -= Math.floor(vo.ph)
      const s = 2 * vo.ph - 1
      sl += s * (1 - vo.pan)
      sr += s * vo.pan
    }
    let amp = 0.05
    if (b < 8) amp *= 0.4 + 0.6 * (b / 8)
    if (b > 68) amp *= Math.max(0, 1 - (b - 68) / 10)
    const d = b >= 8 && b < 68 ? duck[i] : 1
    add(i, lpL.run(sl) * amp * d, lpR.run(sr) * amp * d)
  }
}

// --- Bass: saw + sine sub on offbeat 8ths ------------------------------------
{
  const lp = new Biquad('lp', 700, 1.2)
  let ph = 0
  let sub = 0
  for (let i = beatToSample(8); i < beatToSample(68); i++) {
    const b = i / SR / BEAT
    const eighth = b * 2
    const inEighth = eighth - Math.floor(eighth)
    const isOff = Math.floor(eighth) % 2 === 1
    const env = isOff ? Math.exp(-inEighth * 3.5) : 0.15 * Math.exp(-inEighth * 6)
    const f = midi(chordAtBeat(b).bass)
    ph += f / SR
    ph -= Math.floor(ph)
    sub += (f / 2) / SR
    sub -= Math.floor(sub)
    const s = lp.run(2 * ph - 1) * 0.6 + Math.sin(2 * Math.PI * sub) * 0.5
    add(i, s * env * 0.32 * duck[i])
  }
}

// --- Arp: plucked pulse on 16ths with ping-pong delay ---------------------------
{
  const dryL = new Float32Array(N)
  const dryR = new Float32Array(N)
  const pattern = [0, 2, 1, 3, 2, 4, 3, 1]
  const lp = new Biquad('lp', 1000, 2)
  for (let s16 = 0; s16 < 74 * 4; s16++) {
    const b = s16 / 4
    const chord = chordAtBeat(b)
    let note = chord.pad[pattern[s16 % 8]] + 12
    if (b >= 40 && b < 64 && s16 % 4 === 2) note += 12
    const f = midi(note)
    const s0 = beatToSample(b)
    const len = Math.round(SR * 0.16)
    let vol = 0.11
    if (b < 8) vol *= 0.35 + 0.65 * (b / 8)
    if (b >= 68) vol *= Math.max(0, 1 - (b - 68) / 6)
    let ph = 0
    for (let i = 0; i < len && s0 + i < N; i++) {
      const t = i / SR
      if (i % 32 === 0) {
        const open = b < 8 ? 700 + (b / 8) * 2600 : 3800
        lp.set(open * (0.35 + 0.65 * Math.exp(-t * 25)), 2)
      }
      ph += f / SR
      ph -= Math.floor(ph)
      const sq = ph < 0.3 ? 1 : -1
      const v = lp.run(sq) * Math.exp(-t * 18) * vol
      dryL[s0 + i] += v
      dryR[s0 + i] += v
    }
  }
  // ping-pong delay at 3/16
  const dly = beatToSample(0.75)
  const bufL = new Float32Array(N)
  const bufR = new Float32Array(N)
  for (let i = 0; i < N; i++) {
    const inL = i >= dly ? bufR[i - dly] * 0.45 : 0
    const inR = i >= dly ? bufL[i - dly] * 0.45 : 0
    bufL[i] = dryL[i] + inL
    bufR[i] = inR
    const d = i / SR / BEAT >= 8 && i / SR / BEAT < 68 ? 0.6 + 0.4 * duck[i] : 1
    add(i, (dryL[i] + inL * 0.6) * d, (dryR[i] + inR * 0.6) * d)
  }
}

// --- Drums ----------------------------------------------------------------------
const kick = (b, gain = 1) => {
  const s0 = beatToSample(b)
  let ph = 0
  for (let i = 0; i < SR * 0.4; i++) {
    const t = i / SR
    const f = 45 + 120 * Math.exp(-t * 28)
    ph += f / SR
    const click = i < 60 ? noise() * 0.3 * (1 - i / 60) : 0
    const s = Math.tanh(Math.sin(2 * Math.PI * ph) * 1.6) * Math.exp(-t * 7) + click
    add(s0 + i, s * 0.55 * gain)
  }
}
const clap = (b, gain = 1, pitch = 1) => {
  const s0 = beatToSample(b)
  const bp = new Biquad('bp', 1400 * pitch, 1.1)
  const hp = new Biquad('hp', 300, 0.7)
  let tone = 0
  for (let i = 0; i < SR * 0.25; i++) {
    const t = i / SR
    // three quick bursts then a tail, like a layered clap
    const burst = t < 0.03 ? (Math.floor(t / 0.01) % 2 === 0 ? 1 : 0.4) : 1
    tone += (190 * pitch) / SR
    const body = Math.sin(2 * Math.PI * tone) * Math.exp(-t * 30) * 0.4
    const s = hp.run(bp.run(noise())) * Math.exp(-t * 16) * burst * 1.4 + body
    add(s0 + i, s * 0.32 * gain, s * 0.3 * gain)
  }
}
const hat = (b, gain = 1, open = false, pan = 0.5) => {
  const s0 = beatToSample(b)
  const hp = new Biquad('hp', 7500, 0.8)
  const dec = open ? 9 : 45
  for (let i = 0; i < SR * (open ? 0.3 : 0.06); i++) {
    const t = i / SR
    const s = hp.run(noise()) * Math.exp(-t * dec) * 0.16 * gain
    add(s0 + i, s * (1 - pan) * 2, s * pan * 2)
  }
}
const crash = (b, gain = 1, len = 2.2) => {
  const s0 = beatToSample(b)
  const hpL = new Biquad('hp', 4000, 0.6)
  const hpR = new Biquad('hp', 4200, 0.6)
  for (let i = 0; i < SR * len; i++) {
    const t = i / SR
    const e = Math.exp(-t * 2.2) * 0.22 * gain
    add(s0 + i, hpL.run(noise()) * e, hpR.run(noise()) * e)
  }
}
const boom = (b, gain = 1) => {
  const s0 = beatToSample(b)
  let ph = 0
  for (let i = 0; i < SR * 1.6; i++) {
    const t = i / SR
    const f = 32 + 60 * Math.exp(-t * 6)
    ph += f / SR
    add(s0 + i, Math.sin(2 * Math.PI * ph) * Math.exp(-t * 2.4) * 0.55 * gain)
  }
}
const riser = (b0, b1, gain = 1) => {
  const s0 = beatToSample(b0)
  const s1 = beatToSample(b1)
  const bp = new Biquad('bp', 300, 1.5)
  let ph = 0
  for (let i = s0; i < s1; i++) {
    const p = (i - s0) / (s1 - s0)
    if (i % 32 === 0) bp.set(250 * Math.pow(40, p), 1.8)
    ph += (180 + 900 * p * p) / SR
    const s = bp.run(noise()) * 0.9 + Math.sin(2 * Math.PI * ph) * 0.06
    const e = Math.pow(p, 2) * 0.28 * gain
    add(i, s * e * (0.7 + 0.3 * Math.sin(p * 40)), s * e * (0.7 - 0.3 * Math.sin(p * 40)))
  }
}

riser(0, 8, 1)
for (const b of kickBeats) kick(b, b === 68 ? 1.3 : 1)
for (let b = 8; b < 64; b++) {
  if (b % 2 === 1) clap(b)
  hat(b + 0.5, 1, b % 4 === 3, 0.6)
  if (b >= 40) {
    hat(b + 0.25, 0.45, false, 0.3)
    hat(b + 0.75, 0.45, false, 0.7)
  }
}
// roll into the logo hit
for (let s = 0; s < 16; s++) {
  const b = 64 + s / 4
  clap(b, 0.45 + (s / 16) * 0.75, 1 + s / 30)
  if (s >= 8) clap(b + 0.125, 0.4 + (s / 16) * 0.6, 1 + s / 30)
}
riser(62, 68, 1.1)
crash(8, 1.1)
boom(8, 1)
crash(40, 0.6, 1.5)
crash(56, 0.7, 1.6)
crash(68, 1.3, 3.5)
boom(68, 1.2)

// --- Master: soft clip, fades, normalise --------------------------------------
let peak = 0
for (let i = 0; i < N; i++) {
  const b = i / SR / BEAT
  const fadeIn = Math.min(1, i / (SR * 0.05))
  const fadeOut = b > 74 ? Math.max(0, 1 - (b - 74) / 4) : 1
  L[i] = Math.tanh(L[i] * 1.25) * fadeIn * fadeOut
  R[i] = Math.tanh(R[i] * 1.25) * fadeIn * fadeOut
  peak = Math.max(peak, Math.abs(L[i]), Math.abs(R[i]))
}
const norm = 0.89 / peak

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
  buf.writeInt16LE(Math.round(Math.max(-1, Math.min(1, L[i] * norm)) * 32767), 44 + i * 4)
  buf.writeInt16LE(Math.round(Math.max(-1, Math.min(1, R[i] * norm)) * 32767), 46 + i * 4)
}
const out = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'music.wav')
mkdirSync(dirname(out), { recursive: true })
writeFileSync(out, buf)
console.log(`wrote ${out} (${(N / SR).toFixed(2)} s, peak ${peak.toFixed(3)})`)
