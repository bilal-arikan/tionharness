// Synthesises the three flow-video soundtracks (original, license-free) into
// public/flows/*.wav. Each song's beat is a whole number of video frames at 30 fps
// so the scene lists in src/flows/*.tsx cut on the beat:
//   neon.wav   120 BPM, 72 beats (15 frames/beat)  -> FlowsOverview
//   dusty.wav   90 BPM, 56 beats (20 frames/beat)  -> FlowsEvolution
//   signal.wav 100 BPM, 64 beats (18 frames/beat)  -> FlowsJev
//
//   node scripts/make-flow-music.mjs [neon|dusty|signal ...]

import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { Song } from './lib/synth.mjs'

const OUT = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'flows')

// ---------------------------------------------------------------------------------
// 1. NEON -- synthwave, E minor (Em C G D). Scenes: 0 8 20 32 46 58 66 | 72
// ---------------------------------------------------------------------------------
function neon() {
  const S = new Song({ bpm: 120, beats: 72, seed: 11 })
  const CH = {
    Em: { bass: 40, pad: [55, 59, 64, 67, 71] },
    C: { bass: 36, pad: [55, 60, 64, 67, 72] },
    G: { bass: 43, pad: [55, 59, 62, 67, 71] },
    D: { bass: 38, pad: [54, 57, 62, 66, 69] },
  }
  const PROG = ['Em', 'C', 'G', 'D']
  const chordAt = (b) => {
    if (b < 4) return CH.C
    if (b < 8) return CH.D
    if (b >= 66) return CH.Em
    return CH[PROG[Math.floor((b - 8) / 4) % 4]]
  }
  const groove = (b) => (b >= 8 && b < 56) || (b >= 58 && b < 66)

  const kicks = []
  for (let b = 8; b < 66; b++) if (groove(b)) kicks.push(b)
  for (const b of kicks) S.duckAt(b, 0.7, 9)
  S.duckAt(66, 0.7, 4)

  // pad
  const padCut = (_t, b) => {
    if (b < 8) return 300 + Math.pow(b / 8, 2.2) * 3000
    if (b < 56) return 2400 + 700 * Math.sin((b / 16) * Math.PI)
    if (b < 58) return 1200 + (b - 56) * 1400
    if (b < 66) return 3200
    return 3400 * Math.exp(-(b - 66) * 0.35) + 500
  }
  for (let bar = 0; bar < 18; bar++) {
    const b = bar * 4
    // bar 16 is cut short by the final Em hit at beat 66
    const at = bar === 17 ? 66 : b
    const len = bar === 16 ? 2 : bar === 17 ? 5.2 : 4
    for (const m of chordAt(at).pad)
      S.note(at, len, m, {
        oscs: [
          { type: 'saw', det: -9, pan: 0.15 },
          { type: 'saw', det: 0, pan: 0.5 },
          { type: 'saw', det: 10, pan: 0.85 },
        ],
        gain: b < 8 ? 0.012 + 0.008 * (b / 8) : 0.02,
        a: 0.04,
        r: at === 66 ? 0.8 : 0.25,
        cut: padCut,
        q: 0.9,
        verb: 0.35,
        duck: 0.85,
      })
  }

  // bass: rolling 8ths, octave pop on the last 8th of each beat pair
  for (let e = 16; e < 132; e++) {
    const b = e / 2
    if (!groove(Math.floor(b))) continue
    const ch = chordAt(b)
    const m = ch.bass + (e % 4 === 3 ? 12 : 0)
    S.note(b, 0.42, m, {
      oscs: [{ type: 'saw', gain: 0.8 }, { type: 'square', mult: 0.5, gain: 0.35 }, { type: 'sine', gain: 0.6 }],
      gain: e % 2 ? 0.11 : 0.085,
      a: 0.003,
      r: 0.05,
      cut: { base: 260, amt: 1500, dec: 14 },
      q: 1.2,
      duck: 0.9,
    })
  }

  // arp 16ths
  const pat = [0, 2, 1, 3, 2, 4, 3, 1]
  for (let s16 = 0; s16 < 66 * 4; s16++) {
    const b = s16 / 4
    if (b >= 56 && b < 58) continue
    let m = chordAt(b).pad[pat[s16 % 8]] + 12
    if (b >= 46 && b < 56 && s16 % 4 === 2) m += 12
    let vol = 0.045
    if (b < 8) vol *= 0.3 + 0.7 * (b / 8)
    S.note(b, 0.2, m, {
      oscs: [{ type: 'square', pw: 0.3 }],
      gain: vol,
      a: 0.002,
      d: 0.08,
      s: 0.2,
      r: 0.06,
      cut: { base: 700, amt: b < 8 ? 1200 + b * 300 : 3400, dec: 22 },
      q: 2,
      verb: 0.15,
      dly: 0.4,
      duck: 0.5,
    })
  }

  // lead melody 32-56 (8ths; '.' holds, '-' rests)
  const MEL = {
    Em: 'E5 . G5 . B5 A5 G5 .',
    C: 'E5 . G5 . A5 G5 E5 D5',
    G: 'D5 . G5 . B5 . A5 G5',
    D: 'F#5 . A5 . D6 . B5 A5',
  }
  const NOTE = { C: 0, 'C#': 1, D: 2, 'D#': 3, E: 4, F: 5, 'F#': 6, G: 7, 'G#': 8, A: 9, 'A#': 10, Bb: 10, B: 11 }
  const toMidi = (n) => {
    const m = n.match(/^([A-G][#b]?)(\d)$/)
    return NOTE[m[1]] + (Number(m[2]) + 1) * 12
  }
  const playLine = (b0, line, opts) => {
    const toks = line.split(/\s+/)
    toks.forEach((tok, i) => {
      if (tok === '.' || tok === '-') return
      let len = 0.5
      for (let k = i + 1; k < toks.length && toks[k] === '.'; k++) len += 0.5
      S.note(b0 + i * 0.5, len - 0.05, toMidi(tok), opts)
    })
  }
  const leadOpts = {
    oscs: [
      { type: 'saw', det: -7, pan: 0.35 },
      { type: 'saw', det: 7, pan: 0.65 },
      { type: 'square', mult: 0.5, gain: 0.4 },
    ],
    gain: 0.04,
    a: 0.01,
    d: 0.2,
    s: 0.8,
    r: 0.12,
    cut: 3200,
    q: 1,
    vib: { rate: 5.5, depth: 14, delay: 0.18 },
    verb: 0.25,
    dly: 0.3,
    duck: 0.4,
  }
  for (let bar = 0; bar < 6; bar++) {
    const b = 32 + bar * 4
    playLine(b, MEL[PROG[Math.floor((b - 8) / 4) % 4]], leadOpts)
  }
  for (let bar = 0; bar < 2; bar++) {
    const b = 58 + bar * 4
    playLine(b, MEL[PROG[bar]], { ...leadOpts, gain: 0.03 })
  }

  // drums
  for (const b of kicks) S.kick(b)
  S.kick(66, { gain: 1.3 })
  for (let b = 8; b < 66; b++) {
    if (!groove(b)) continue
    if (b % 2 === 1) S.clap(b)
    S.hat(b + 0.5, { open: b % 4 === 3, pan: 0.6 })
    if (b >= 20) {
      S.hat(b + 0.25, { gain: 0.4, pan: 0.3 })
      S.hat(b + 0.75, { gain: 0.4, pan: 0.7 })
    }
  }
  for (let s = 0; s < 8; s++) {
    const b = 56 + s / 4
    S.snare(b, { gain: 0.35 + (s / 8) * 0.6, tone: 190 + s * 8, verb: 0.2 })
    if (s >= 4) S.snare(b + 0.125, { gain: 0.35 + (s / 8) * 0.5, tone: 220 + s * 8 })
  }
  for (let s = 0; s < 8; s++) S.snare(6 + s / 4, { gain: 0.15 + (s / 8) * 0.4, tone: 200 + s * 6, verb: 0.3 })
  S.riser(0, 8, { gain: 1 })
  S.riser(54, 58, { gain: 1 })
  for (const b of [8, 58]) {
    S.crash(b, { gain: 1.1 })
    S.boom(b)
  }
  for (const b of [20, 32, 46]) S.crash(b, { gain: 0.6, len: 1.5 })
  S.crash(66, { gain: 1.3, len: 3.5 })
  S.boom(66, { gain: 1.2 })

  return { S, mix: { delayBeats: 0.75, drive: 1.25, fadeOutBeats: 3 } }
}

// ---------------------------------------------------------------------------------
// 2. DUSTY -- lo-fi hip hop, F major 9ths. Scenes: 0 8 16 28 40 48 | 56
// ---------------------------------------------------------------------------------
function dusty() {
  const S = new Song({ bpm: 90, beats: 56, seed: 23 })
  const CH = [
    { bass: 41, v: [57, 60, 64, 67] }, // Fmaj9
    { bass: 40, v: [55, 59, 62, 66] }, // Em9
    { bass: 38, v: [53, 57, 60, 64] }, // Dm9
    { bass: 36, v: [52, 55, 59, 62] }, // Cmaj9
  ]
  const chordAt = (b) => (b >= 48 ? CH[0] : CH[Math.floor(b / 4) % 4])
  const wob = (t) => 9 * Math.sin(2 * Math.PI * 0.45 * t) + 3 * Math.sin(2 * Math.PI * 1.7 * t)
  const drums = (b) => b >= 8 && b < 48

  const kickPat = [0, 1.75, 2.5]
  for (let bar = 2; bar < 12; bar++) for (const k of kickPat) S.duckAt(bar * 4 + k, 0.3, 7)

  // e-piano comping: chord on 1 and on the "and" of 2, gently strummed
  for (let bar = 0; bar < 12; bar++) {
    const b = bar * 4
    const ch = chordAt(b)
    const lp = b < 8 ? (bb) => 700 + Math.pow(bb / 8, 2) * 3500 : 5200
    const hits = [
      [0, 2.3, 0.9],
      [2.5, 1.4, 0.6],
    ]
    for (const [off, len, vel] of hits)
      ch.v.forEach((m, k) =>
        S.epiano(b + off, len, m, { gain: 0.07, vel: vel * (0.85 + 0.3 * S.rnd()), offset: k * 0.014, wob, lp, verb: 0.3, pan: 0.3 + k * 0.13, duck: 1 }),
      )
  }
  // outro chord rings
  for (const [k, m] of [53, 57, 60, 64, 67].entries()) S.epiano(48, 6, m, { gain: 0.08, vel: 0.85, offset: k * 0.03, wob, verb: 0.45, pan: 0.25 + k * 0.12 })
  S.bell(48.02, 84, { gain: 0.035, verb: 0.6, len: 4 })
  S.bell(50, 79, { gain: 0.025, verb: 0.6, len: 4, pan: 0.7 })

  // bass
  for (let bar = 2; bar < 12; bar++) {
    const b = bar * 4
    const root = chordAt(b).bass
    const nxt = chordAt(b + 4).bass
    const line = [
      [0, 1.4, root],
      [1.75, 0.6, root],
      [2.5, 1, root + 7],
      [3.5, 0.45, nxt + (nxt > root ? -1 : 1)],
    ]
    for (const [off, len, m] of line)
      S.note(b + off, len, m, {
        oscs: [{ type: 'sine' }, { type: 'tri', gain: 0.35 }],
        gain: 0.15,
        a: 0.01,
        r: 0.08,
        cut: 650,
        wob,
      })
  }

  // drums: boom-bap with swung 8th hats, all a bit dusty
  const swing = 0.08
  for (let b = 8; b < 48; b++) {
    const inBar = b % 4
    if (inBar === 1 || inBar === 3) S.snare(b, { gain: 0.9, tone: 170, dec: 11, lp: 4200, hp: 600, verb: 0.22 })
    S.hat(b, { gain: 0.55 + 0.2 * S.rnd(), hp: 6500, pan: 0.6 })
    S.hat(b + 0.5 + swing, { gain: 0.35 + 0.2 * S.rnd(), hp: 6500, pan: 0.65 })
    if (inBar === 3 && Math.floor(b / 4) % 2 === 1) S.hat(b + 0.75 + swing / 2, { gain: 0.3, hp: 7000, pan: 0.4 })
  }
  for (let bar = 2; bar < 12; bar++)
    for (const k of kickPat) S.kick(bar * 4 + k, { f0: 120, f1: 48, pd: 22, dec: 9, click: 0.08, drive: 1.2, gain: k === 0 ? 0.95 : 0.75 })
  // ghost snare before the melody enters + soft rim at the outro
  S.snare(27.75, { gain: 0.3, tone: 200, lp: 4000, hp: 800 })
  S.snare(47.5, { gain: 0.35, tone: 200, lp: 4000, hp: 800, verb: 0.4 })

  // melody 28-48: soft triangle lead with tape wobble and dotted delay
  const NOTE = { C: 0, D: 2, E: 4, F: 5, G: 7, A: 9, B: 11 }
  const toMidi = (n) => NOTE[n[0]] + (Number(n.slice(1)) + 1) * 12
  const MEL = ['A4 . C5 . E5 . D5 C5', 'B4 . . G4 . A4 B4 .', 'A4 . F4 . A4 C5 . E5', 'D5 . . C5 B4 . G4 .', 'A4 . . . . . - -']
  MEL.forEach((line, bar) => {
    const b0 = 28 + bar * 4
    const toks = line.split(/\s+/)
    toks.forEach((tok, i) => {
      if (tok === '.' || tok === '-') return
      let len = 0.5
      for (let k = i + 1; k < toks.length && toks[k] === '.'; k++) len += 0.5
      const at = b0 + i * 0.5 + (i % 2 ? swing : 0)
      S.note(at, len - 0.08, toMidi(tok) + 12, {
        oscs: [{ type: 'tri' }, { type: 'sine', mult: 2, gain: 0.15 }],
        gain: 0.05,
        a: 0.02,
        d: 0.3,
        s: 0.7,
        r: 0.2,
        cut: 2600,
        vib: { rate: 4.5, depth: 10, delay: 0.25 },
        wob,
        verb: 0.35,
        dly: 0.35,
      })
    })
  })

  S.vinyl(0, 56, { gain: 1 })
  return { S, mix: { delayBeats: 0.75, delayFb: 0.38, delayLp: 2400, room: 0.8, damp: 0.5, lp: 9000, drive: 1.1, fadeOutBeats: 5 } }
}

// ---------------------------------------------------------------------------------
// 3. SIGNAL -- cinematic pulse, D minor. Scenes: 0 8 20 28 36 48 56 | 64
// ---------------------------------------------------------------------------------
function signal() {
  const S = new Song({ bpm: 100, beats: 64, seed: 37 })
  const CH = {
    Dm: { root: 38, tones: [50, 53, 57] },
    Bb: { root: 34, tones: [46, 50, 53] },
    F: { root: 41, tones: [48, 53, 57] },
    C: { root: 36, tones: [48, 52, 55] },
    Gm: { root: 43, tones: [50, 55, 58] },
    A: { root: 45, tones: [49, 52, 57] },
  }
  const P1 = ['Dm', 'Bb', 'F', 'C', 'Dm', 'Bb', 'Gm', 'A']
  const P2 = ['Dm', 'Bb', 'Gm', 'A']
  const chordAt = (b) => {
    if (b < 8 || b >= 56) return CH.Dm
    if (b < 40) return CH[P1[Math.floor((b - 8) / 4)]]
    return CH[P2[Math.floor((b - 40) / 4)]]
  }
  const HITS = [8, 20, 28, 36, 48]

  // strings pad (slow attack), one event per bar
  for (let bar = 0; bar < 16; bar++) {
    const b = bar * 4
    const ch = chordAt(b)
    const notes = b < 8 ? [38, 50, 57] : [ch.root, ...ch.tones, ch.tones[0] + 12]
    const len = b >= 56 ? 6.5 : 4
    if (b === 60) continue
    for (const m of notes)
      S.note(b, len, m, {
        oscs: [
          { type: 'saw', det: -8, pan: 0.2 },
          { type: 'saw', det: 6, pan: 0.8 },
          { type: 'saw', det: 0, mult: 2, gain: 0.25, pan: 0.5 },
        ],
        gain: m < 45 ? 0.024 : 0.013,
        a: b < 8 ? 1.5 : 0.35,
        r: b >= 56 ? 1.5 : 0.4,
        cut: (_t, bb) => (bb < 8 ? 500 + bb * 150 : bb < 56 ? 1400 + (bb > 36 ? 900 : 0) : 2200 * Math.exp(-(bb - 56) * 0.3) + 400),
        q: 0.8,
        vib: { rate: 4.8, depth: 6, delay: 0.5 },
        verb: 0.5,
      })
  }

  // ticking clock in the intro (decisions being made)
  for (let e = 0; e < 16; e++) S.tick(e / 2, { gain: e % 2 ? 0.35 : 0.6, freq: e % 2 ? 4200 : 3000, pan: e % 2 ? 0.7 : 0.3 })
  S.boom(0, { gain: 0.8, len: 2.4 })
  S.bell(0.02, 74, { gain: 0.05, len: 4 })
  S.bell(4, 77, { gain: 0.04, len: 4, pan: 0.7 })
  S.riser(4, 8, { gain: 1 })

  // ostinato 16ths (spiccato strings) 8-56
  const shape = [0, 12, 7, 12, 0, 12, 7, 15]
  for (let s16 = 32; s16 < 56 * 4; s16++) {
    const b = s16 / 4
    const ch = chordAt(b)
    const third = ch.tones[1] - ch.tones[0]
    const iv = shape[s16 % 8] === 15 ? 12 + third : shape[s16 % 8]
    const m = ch.root + 12 + iv
    const acc = s16 % 4 === 0 ? 1 : s16 % 2 === 0 ? 0.7 : 0.55
    const open = b < 20 ? 1400 : b < 36 ? 1900 : 2600
    S.note(b, 0.16, m, {
      oscs: [
        { type: 'saw', det: -5, pan: 0.3 },
        { type: 'saw', det: 5, pan: 0.7 },
      ],
      gain: 0.045 * acc,
      a: 0.004,
      d: 0.06,
      s: 0.3,
      r: 0.05,
      cut: { base: 350, amt: open, dec: 18 },
      q: 1.1,
      verb: 0.25,
    })
    if (b >= 20 && s16 % 2 === 0)
      S.note(b, 0.12, m + 12, {
        oscs: [{ type: 'square', pw: 0.25 }],
        gain: 0.018 * acc,
        a: 0.003,
        d: 0.05,
        s: 0.2,
        r: 0.04,
        cut: { base: 900, amt: 2400, dec: 25 },
        verb: 0.3,
        dly: 0.25,
        pan: s16 % 4 === 0 ? 0.25 : 0.75,
      })
  }

  // toms
  const tomPat = [
    [0, 1, 70],
    [0.75, 0.55, 95],
    [1.5, 0.6, 70],
    [2, 0.85, 110],
    [2.5, 0.5, 70],
    [3, 0.8, 95],
    [3.5, 0.45, 130],
  ]
  for (let bar = 2; bar < 14; bar++) {
    const b = bar * 4
    for (const [off, g, f] of tomPat) S.tom(b + off, { gain: g * (b >= 36 ? 1 : 0.8), f, pan: f > 100 ? 0.65 : f < 80 ? 0.4 : 0.5 })
    if (b >= 36 && bar % 2 === 1) for (let k = 0; k < 4; k++) S.tom(b + 3 + k * 0.25, { gain: 0.35 + k * 0.12, f: 140 - k * 15 })
  }
  for (let k = 0; k < 16; k++) S.tom(52 + k / 4 + 2, { gain: 0.3 + (k / 16) * 0.8, f: 80 + k * 4 })
  S.riser(52, 56, { gain: 1.1 })

  // braam on the big hits
  const braam = (b, len, gain) => {
    for (const m of [26, 33, 38, 41])
      S.note(b, len, m, {
        oscs: [
          { type: 'saw', det: -12, pan: 0.2 },
          { type: 'saw', det: 0 },
          { type: 'saw', det: 12, pan: 0.8 },
        ],
        gain: gain * (m < 30 ? 1 : 0.6),
        a: 0.01,
        r: 1.2,
        cut: (t) => 180 + 2200 * Math.exp(-t * 2.2) * Math.min(1, t * 20),
        q: 1.1,
        drive: 2,
        verb: 0.4,
      })
  }
  braam(8, 3, 0.05)
  braam(56, 5, 0.06)
  for (const b of HITS) {
    S.boom(b, { gain: b === 8 ? 1.1 : 0.8 })
    S.crash(b, { gain: b === 8 ? 1 : 0.55, len: b === 8 ? 2.5 : 1.6, verb: 0.35 })
  }
  S.boom(56, { gain: 1.3, len: 2.6 })
  S.crash(56, { gain: 1.2, len: 4, verb: 0.4 })
  for (const b of [8, 20, 28, 36, 48, 56]) S.kick(b, { gain: 1, f0: 140, f1: 40, dec: 5, len: 0.7 })
  // bells: sparse motif over the second half
  ;[
    [28, 74],
    [30, 77],
    [32, 81],
    [36, 79],
    [40, 77],
    [42, 74],
    [44, 79],
    [46, 76],
    [56.02, 86],
    [58, 81],
  ].forEach(([b, m], k) => S.bell(b, m, { gain: 0.035, len: 3, pan: k % 2 ? 0.7 : 0.3, dly: 0.2 }))

  return { S, mix: { delayBeats: 0.75, delayFb: 0.35, room: 0.88, damp: 0.3, verbWet: 1.1, drive: 1.3, fadeOutBeats: 4 } }
}

const SONGS = { neon, dusty, signal }
const pick = process.argv.slice(2)
for (const name of pick.length ? pick : Object.keys(SONGS)) {
  const t0 = Date.now()
  const { S, mix } = SONGS[name]()
  const stats = S.render(mix)
  const out = join(OUT, `${name}.wav`)
  S.write(out)
  console.log(
    `${name}.wav  ${(S.N / 44100).toFixed(2)} s  pre-norm peak ${stats.peak.toFixed(3)}  rms ${stats.rmsDb.toFixed(1)} dBFS  (${((Date.now() - t0) / 1000).toFixed(1)} s)`,
  )
}
