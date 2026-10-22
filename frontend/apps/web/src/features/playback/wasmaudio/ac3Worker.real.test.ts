// @vitest-environment node
//
// The REAL thing: the shipped ac3go.wasm, the shipped ac3Worker.js, a real
// browser (Chromium and Firefox), and CMAF audio segments cut from real films
// with the exact ffmpeg recipe the server uses (see hls_audio_only.go), for
// every layout a library actually holds: AC-3 stereo (44.1 kHz), AC-3 5.1,
// E-AC-3 5.1, E-AC-3 7.1.
//
// It answers two questions the synthetic tests cannot:
//   1. Do the worker-mixed reply, the main-thread planar path and the legacy
//      interleaved path produce IDENTICAL samples on real decoder output?
//   2. Is the channel MAPPING right? Each output channel is checked against an
//      independent decoder (ffmpeg's) on the same track, so a swapped centre,
//      a lost LFE or a wrong 7.1 fold cannot pass.
//
// Gated on EVEY_REAL_AUDIO_DIR because it needs the fixtures (cut from the
// library, never committed) and the Playwright browsers. Layout:
//   $EVEY_REAL_AUDIO_DIR/<case>/a_000.m4s a_001.m4s ... ref.f32 ref_drc.f32
// cut by scripts/real-audio-fixtures.sh: the references are ffmpeg's decode of
// the same span, float32 interleaved in ffmpeg's native channel order, from a
// session that starts at the film's first frame (that script says why).
import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { createRequire } from "node:module";
import { createServer, type Server } from "node:http";
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { resolve, join } from "node:path";
import { outputMix, FOLD_GAIN } from "./channelMix";
import { fillAudioBuffer, workerMixTable, type PcmDecoded } from "./WasmAudioEngine";

const REAL_DIR = process.env.EVEY_REAL_AUDIO_DIR;
const WASM_DIR = resolve(__dirname, "../../../../public/wasm");
const REPO_ROOT = resolve(__dirname, "../../../../../../..");

// One case per real layout. ffmpegOrder is what ffmpeg's decoder emits for the
// track (its native order), expected maps each Web Audio OUTPUT channel to
// the ffmpeg channels it must equal: a single index, or a weighted sum.
interface RealCase {
  name: string;
  channels: number;
  sampleRate: number;
  expected: Array<Array<[number, number]>>;
}
const CASES: RealCase[] = [
  { name: "ac3_2", channels: 2, sampleRate: 44100, expected: [[[0, 1]], [[1, 1]]] },
  // ffmpeg 5.1(side): FL FR FC LFE SL SR == Web Audio L R C LFE SL SR.
  { name: "ac3_6", channels: 6, sampleRate: 48000, expected: [[[0, 1]], [[1, 1]], [[2, 1]], [[3, 1]], [[4, 1]], [[5, 1]]] },
  { name: "eac3_6", channels: 6, sampleRate: 48000, expected: [[[0, 1]], [[1, 1]], [[2, 1]], [[3, 1]], [[4, 1]], [[5, 1]]] },
  // ffmpeg 7.1: FL FR FC LFE BL BR SL SR. Our fold: SL' = (SL + BL) * g.
  {
    name: "eac3_8",
    channels: 8,
    sampleRate: 48000,
    expected: [[[0, 1]], [[1, 1]], [[2, 1]], [[3, 1]], [[6, FOLD_GAIN], [4, FOLD_GAIN]], [[7, FOLD_GAIN], [5, FOLD_GAIN]]],
  },
];

interface DecodedReply {
  channels: number;
  sampleRate: number;
  frames: number;
  mixed: boolean;
  planar: string[]; // base64 of each Float32Array's bytes
}

function fromB64(s: string): Float32Array<ArrayBuffer> {
  const buf = Buffer.from(s, "base64");
  const out = new Float32Array(buf.byteLength / 4);
  new Uint8Array(out.buffer).set(buf);
  return out;
}

const PAGE = `<!doctype html><meta charset="utf-8"><script>
window.decodeAll = (mixes, urls, dither) => new Promise((resolve, reject) => {
  const w = new Worker("/wasm/ac3Worker.js");
  const out = [];
  let id = 0;
  const bufs = [];
  const b64 = (buf) => {
    const u = new Uint8Array(buf); let s = "";
    for (let i = 0; i < u.length; i += 0x8000) s += String.fromCharCode.apply(null, u.subarray(i, i + 0x8000));
    return btoa(s);
  };
  w.onmessage = (e) => {
    const m = e.data;
    if (m.type === "ready") return next();
    if (m.type === "error") return reject(new Error(m.error));
    if (m.type === "decodeError") return reject(new Error(m.error));
    if (m.type === "decoded") {
      const planar = m.planar ? m.planar.map(b64) : null;
      out.push({ channels: m.channels, sampleRate: m.sampleRate, frames: m.frames, mixed: m.mixed === true, planar, pcm: m.pcm ? b64(m.pcm) : null });
      return next();
    }
  };
  w.onerror = (e) => reject(new Error(e.message));
  const next = async () => {
    if (id >= urls.length) { w.terminate(); return resolve(out); }
    const r = await fetch(urls[id++]);
    const bytes = await r.arrayBuffer();
    const msg = { type: "decode", id, bytes };
    if (typeof dither === "boolean") msg.dither = dither;
    w.postMessage(msg, [bytes]);
  };
  w.postMessage({ type: "init", wasmUrl: "/wasm/ac3go.wasm", wasmExecUrl: "/wasm/wasm_exec.js", mixes });
});
</script>`;

function contentType(path: string): string {
  if (path.endsWith(".wasm")) return "application/wasm";
  if (path.endsWith(".js")) return "text/javascript";
  return "application/octet-stream";
}

function startServer(): Promise<{ server: Server; url: string }> {
  const server = createServer((req, res) => {
    const url = req.url ?? "/";
    if (url === "/") {
      res.writeHead(200, { "content-type": "text/html" });
      res.end(PAGE);
      return;
    }
    const file = url.startsWith("/wasm/") ? join(WASM_DIR, url.slice(6)) : url.startsWith("/real/") ? join(REAL_DIR!, url.slice(6)) : null;
    if (!file || !existsSync(file)) {
      res.writeHead(404);
      res.end();
      return;
    }
    res.writeHead(200, { "content-type": contentType(file) });
    res.end(readFileSync(file));
  });
  return new Promise((ok) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      ok({ server, url: `http://127.0.0.1:${port}` });
    });
  });
}

// A capture buffer standing in for AudioBuffer: fillAudioBuffer only needs
// copyToChannel and getChannelData.
class CaptureBuffer {
  readonly data: Float32Array[];
  constructor(channels: number, frames: number) {
    this.data = Array.from({ length: channels }, () => new Float32Array(frames));
  }
  copyToChannel(src: Float32Array, ch: number): void {
    this.data[ch].set(src);
  }
  getChannelData(ch: number): Float32Array {
    return this.data[ch];
  }
}

function concat(parts: Float32Array[]): Float32Array {
  const n = parts.reduce((a, p) => a + p.length, 0);
  const out = new Float32Array(n);
  let o = 0;
  for (const p of parts) {
    out.set(p, o);
    o += p.length;
  }
  return out;
}

// A reference channel that never exceeds this is quiet: about 3 LSB of 16-bit
// audio (-80 dBFS), where two decoders' rounding and noise ARE the signal.
// Avatar: Fire and Ash's 7.1 LFE peaks at 3.2e-5 over its window, and read as
// a channel it scored corr 0.81 and -0.8 dB below 1 kHz against ffmpeg, which
// says nothing about the decoder (2026-10-22).
const QUIET_PEAK = 1e-4;

// Least-squares gain, peak residual and RMS error between two signals over
// their overlap. A wrong channel gives a near-zero correlation, and a wrong
// fold gain gives a gain that is not the same on every channel - those are the
// mapping checks. The RMS error is the two DECODERS disagreeing (ac3go vs
// ffmpeg's): measured and reported, guarded loosely, because it is not what
// this test is about and not something this code can change.
function compare(a: Float32Array, b: Float32Array, lag: number) {
  const n = Math.min(a.length, b.length) - Math.abs(lag) - 4096;
  let saa = 0, sbb = 0, sab = 0, refPeak = 0;
  for (let i = 0; i < n; i++) {
    const x = a[i + Math.max(0, lag)];
    const y = b[i + Math.max(0, -lag)];
    saa += x * x; sbb += y * y; sab += x * y;
    if (Math.abs(y) > refPeak) refPeak = Math.abs(y);
  }
  if (saa === 0 || sbb === 0 || refPeak < QUIET_PEAK) return { corr: saa === sbb ? 1 : 0, gain: 1, peak: 0, rms: 0, silent: true };
  const corr = sab / Math.sqrt(saa * sbb);
  const gain = sab / sbb; // a ~= gain * b
  let peak = 0, sdd = 0;
  for (let i = 0; i < n; i++) {
    const d = a[i + Math.max(0, lag)] - gain * b[i + Math.max(0, -lag)];
    if (Math.abs(d) > peak) peak = Math.abs(d);
    sdd += d * d;
  }
  return { corr, gain, peak, rms: Math.sqrt(sdd / (gain * gain * sbb)), silent: false };
}

// ffmpeg's AC-3 decoder applies the bitstream's dynamic range control by
// default; the fixture directory carries the track decoded both ways (ref.f32
// without, ref_drc.f32 with). The comparison uses whichever ac3go is closer
// to and says which, so the report also tells what ac3go does with DRC.
function loadReferences(dir: string, channels: number): Array<{ label: string; ref: Float32Array; frames: number }> {
  const out = [];
  for (const [label, file] of [["drc=off", "ref.f32"], ["drc=on", "ref_drc.f32"]] as const) {
    const path = join(dir, file);
    if (!existsSync(path)) continue;
    const bytes = readFileSync(path);
    const ref = new Float32Array(bytes.buffer, bytes.byteOffset, bytes.byteLength / 4);
    out.push({ label, ref, frames: ref.length / channels });
  }
  return out;
}

describe.skipIf(!REAL_DIR)("ac3Worker on real films, in real browsers", () => {
  // Loaded in beforeAll, not at collection time: a skipped suite still runs
  // this factory, and the unit-test container has no Playwright to require.
  let pw: typeof import("@playwright/test");
  let server: Server;
  let base: string;

  beforeAll(async () => {
    const require = createRequire(join(REPO_ROOT, "tests/e2e/package.json"));
    pw = require("@playwright/test") as typeof import("@playwright/test");
    ({ server, url: base } = await startServer());
  });
  afterAll(() => {
    server?.close();
  });

  for (const browserName of ["chromium", "firefox"] as const) {
    describe(browserName, () => {
      for (const c of CASES) {
        const dir = REAL_DIR ? join(REAL_DIR, c.name) : "";
        const present = REAL_DIR ? existsSync(join(dir, "ref.f32")) : false;

        it.skipIf(!present)(`${c.name}: the three paths are sample-identical and the mapping matches ffmpeg`, async () => {
          const segs = readdirSync(dir).filter((f) => /^a_\d+\.m4s$/.test(f)).sort();
          const urls = segs.map((f) => `/real/${c.name}/${f}`);
          const mix = outputMix(c.channels);

          const browser = await pw[browserName].launch();
          try {
            const page = await browser.newPage();
            await page.goto(base + "/");
            const mixed = (await page.evaluate(([m, u]) => (window as unknown as { decodeAll: (m: unknown, u: string[]) => Promise<DecodedReply[]> }).decodeAll(m, u), [workerMixTable(), urls] as const)) as DecodedReply[];
            const raw = (await page.evaluate(([m, u]) => (window as unknown as { decodeAll: (m: unknown, u: string[]) => Promise<DecodedReply[]> }).decodeAll(m, u), [null, urls] as const)) as DecodedReply[];
            await page.close();

            expect(mixed).toHaveLength(segs.length);
            expect(raw).toHaveLength(segs.length);

            const perChannel: Float32Array[][] = Array.from({ length: mix.length }, () => []);
            for (let i = 0; i < segs.length; i++) {
              const m = mixed[i];
              const r = raw[i];
              expect(m.mixed).toBe(true);
              expect(r.mixed).toBe(false);
              expect(m.channels).toBe(c.channels);
              expect(m.sampleRate).toBe(c.sampleRate);
              expect(m.planar).toHaveLength(mix.length);
              expect(r.planar).toHaveLength(c.channels);

              const mixedPlanar = m.planar.map(fromB64);
              const srcPlanar = r.planar.map(fromB64);
              const frames = m.frames;

              // Path 2: main-thread fold of the source-planar reply.
              const viaPlanar = new CaptureBuffer(mix.length, frames);
              fillAudioBuffer(viaPlanar as unknown as AudioBuffer, { kind: "pcm", channels: c.channels, sampleRate: c.sampleRate, frames, planar: srcPlanar } satisfies PcmDecoded, mix);
              // Path 3: the legacy interleaved reply, rebuilt from the same samples.
              const inter = new Float32Array(frames * c.channels);
              for (let f = 0; f < frames; f++) for (let ch = 0; ch < c.channels; ch++) inter[f * c.channels + ch] = srcPlanar[ch][f];
              const viaInter = new CaptureBuffer(mix.length, frames);
              fillAudioBuffer(viaInter as unknown as AudioBuffer, { kind: "pcm", channels: c.channels, sampleRate: c.sampleRate, frames, pcm: inter }, mix);

              for (let co = 0; co < mix.length; co++) {
                expect(Buffer.from(mixedPlanar[co].buffer)).toEqual(Buffer.from(viaPlanar.data[co].buffer));
                expect(Buffer.from(mixedPlanar[co].buffer)).toEqual(Buffer.from(viaInter.data[co].buffer));
                perChannel[co].push(mixedPlanar[co]);
              }
            }

            // Against ffmpeg. Build what each output channel must be from the
            // reference's own channels, measure every channel first, and only
            // then judge, so a failure names the whole picture.
            const ours = perChannel.map(concat);
            type Row = { co: number; corr: number; gain: number; peak: number; rms: number; lag: number; silent: boolean };
            let best: { label: string; rows: Row[]; meanCorr: number } | null = null;
            for (const { label, ref, frames: refFrames } of loadReferences(dir, c.channels)) {
              const rows: Row[] = [];
              for (let co = 0; co < mix.length; co++) {
                const want = new Float32Array(refFrames);
                for (const [src, g] of c.expected[co]) for (let f = 0; f < refFrames; f++) want[f] += ref[f * c.channels + src] * g;
                // Decoders agree on frame boundaries; allow a whole-frame offset
                // either way in case the muxer started on a different packet.
                let b = { corr: -1, gain: 0, peak: Infinity, rms: Infinity, silent: false, lag: 0 };
                for (const lag of [0, 1536, -1536, 3072, -3072]) {
                  const r = compare(ours[co], want, lag);
                  if (r.corr > b.corr) b = { ...r, lag };
                }
                rows.push({ co, ...b });
              }
              const live = rows.filter((r) => !r.silent);
              const meanCorr = live.reduce((a, r) => a + r.corr, 0) / Math.max(1, live.length);
              if (!best || meanCorr > best.meanCorr) best = { label, rows, meanCorr };
            }
            expect(best, "no reference decode found in the fixture directory").not.toBeNull();
            const rows = best!.rows;
            const table = [`${c.name} (${browserName}) vs ffmpeg ${best!.label}:`]
              .concat(rows.map((r) => `  ch${r.co}: corr=${r.corr.toFixed(5)} gain=${r.gain.toFixed(4)} rms=${(r.rms * 100).toFixed(2)}% peak=${r.peak.toExponential(2)} lag=${r.lag}${r.silent ? " (silent or quiet)" : ""}`))
              .join("\n");
            // eslint-disable-next-line no-console
            console.log(table);
            for (const r of rows) {
              if (r.silent) continue; // a silent or quiet channel proves nothing either way
              // Mapping: the right source channel is on this output. A swapped or
              // dropped channel correlates near zero, not near one.
              expect(r.corr, `${c.name} channel ${r.co} does not carry the expected source\n${table}`).toBeGreaterThan(0.99);
              // Fidelity guard: the two decoders must stay close. Loose on
              // purpose (see compare); a gain or fold mistake shows up here too.
              expect(r.rms, `${c.name} channel ${r.co} strays from ffmpeg\n${table}`).toBeLessThan(0.1);
            }
            // Whatever constant the two decoders disagree by, it is the SAME on
            // every channel: a fold applied on the wrong channel would not be.
            const gains = rows.filter((r) => !r.silent).map((r) => r.gain);
            expect(Math.max(...gains) / Math.min(...gains), `${c.name} per-channel gains\n${table}`).toBeLessThan(1.02);
          } finally {
            await browser.close();
          }
        }, 120_000);
      }
    });
  }
});

// Where the residual against ffmpeg comes from. AC-3 fills the mantissas it
// gave no bits to with noise the DECODER draws locally (bap 0 dither, clause
// 7.3.4): two conforming decoders disagree there by design, and ffmpeg has no
// switch for it. ac3go has. So: decode with the noise off, and the remaining
// disagreement with ffmpeg must be ffmpeg's own noise, of the same size as
// ours - the energies have to add up. If they do, nothing is wrong with the
// decoder: the residual is the format's, not ours. If they do not, something
// else is off and this names the channel.
describe.skipIf(!REAL_DIR)("ac3Worker vs ffmpeg: level per band, and waveform below the noise bands", () => {
  let pw: typeof import("@playwright/test");
  let server: Server;
  let base: string;

  beforeAll(async () => {
    const require = createRequire(join(REPO_ROOT, "tests/e2e/package.json"));
    pw = require("@playwright/test") as typeof import("@playwright/test");
    ({ server, url: base } = await startServer());
  });
  afterAll(() => {
    server?.close();
  });

  // Where in the spectrum the disagreement lives. Coupling (clause 7.4) only
  // touches the bands above the coupling start frequency, a few kHz up;
  // dynamic range control and exponents touch everything. A 2048-point DFT
  // per hop, energies summed into coarse bands, is enough to tell them apart.
  function bandEnergies(x: Float32Array, sampleRate: number, edges: number[]): number[] {
    const N = 2048;
    const out: number[] = Array.from({ length: edges.length - 1 }, () => 0);
    const cosT = new Float32Array(N), sinT = new Float32Array(N);
    for (let i = 0; i < N; i++) { cosT[i] = Math.cos((2 * Math.PI * i) / N); sinT[i] = Math.sin((2 * Math.PI * i) / N); }
    for (let start = 0; start + N <= x.length; start += N * 8) {
      for (let k = 1; k < N / 2; k++) {
        let re = 0, im = 0;
        for (let n = 0; n < N; n++) {
          const idx = (k * n) % N;
          const w = 0.5 - 0.5 * cosT[n]; // Hann
          re += x[start + n] * w * cosT[idx];
          im -= x[start + n] * w * sinT[idx];
        }
        const hz = (k * sampleRate) / N;
        for (let b = 0; b < edges.length - 1; b++) if (hz >= edges[b] && hz < edges[b + 1]) { out[b] += re * re + im * im; break; }
      }
    }
    return out;
  }

  // Per-band waveform coherence between two signals: the normalised real
  // cross-spectrum. Two decoders that reconstruct the same coefficients score
  // near one even where each adds its own noise on top; a band rebuilt
  // differently (a phase flag, a shifted coordinate band) scores near zero at
  // the same level.
  function bandCoherence(x: Float32Array, y: Float32Array, sampleRate: number, edges: number[]): number[] {
    const N = 2048;
    const zeros = () => Array.from({ length: edges.length - 1 }, () => 0);
    const xy = zeros(), xx = zeros(), yy = zeros();
    const cosT = new Float32Array(N), sinT = new Float32Array(N);
    for (let i = 0; i < N; i++) { cosT[i] = Math.cos((2 * Math.PI * i) / N); sinT[i] = Math.sin((2 * Math.PI * i) / N); }
    const len = Math.min(x.length, y.length);
    for (let start = 0; start + N <= len; start += N * 8) {
      for (let k = 1; k < N / 2; k++) {
        let xr = 0, xi = 0, yr = 0, yi = 0;
        for (let n = 0; n < N; n++) {
          const idx = (k * n) % N;
          const w = 0.5 - 0.5 * cosT[n];
          const c = cosT[idx], sn = sinT[idx];
          xr += x[start + n] * w * c; xi -= x[start + n] * w * sn;
          yr += y[start + n] * w * c; yi -= y[start + n] * w * sn;
        }
        const hz = (k * sampleRate) / N;
        for (let b = 0; b < edges.length - 1; b++) if (hz >= edges[b] && hz < edges[b + 1]) { xy[b] += xr * yr + xi * yi; xx[b] += xr * xr + xi * xi; yy[b] += yr * yr + yi * yi; break; }
      }
    }
    return xy.map((v, b) => (xx[b] > 0 && yy[b] > 0 ? v / Math.sqrt(xx[b] * yy[b]) : 0));
  }

  function rmsRel(a: Float32Array, b: Float32Array): number {
    const n = Math.min(a.length, b.length);
    let sdd = 0, sbb = 0;
    for (let i = 0; i < n; i++) {
      const d = a[i] - b[i];
      sdd += d * d;
      sbb += b[i] * b[i];
    }
    return sbb === 0 ? 0 : Math.sqrt(sdd / sbb);
  }

  for (const c of CASES) {
    const dir = REAL_DIR ? join(REAL_DIR, c.name) : "";
    const present = REAL_DIR ? existsSync(join(dir, "ref_drc.f32")) : false;

    it.skipIf(!present)(`${c.name}: every band at the ffmpeg level, every coded band the same waveform`, async () => {
      const segs = readdirSync(dir).filter((f) => /^a_\d+\.m4s$/.test(f)).sort();
      const urls = segs.map((f) => `/real/${c.name}/${f}`);
      const mix = outputMix(c.channels);
      type Decode = (m: unknown, u: string[], d: boolean) => Promise<DecodedReply[]>;

      const browser = await pw.chromium.launch();
      try {
        const page = await browser.newPage();
        await page.goto(base + "/");
        const on = (await page.evaluate(([m, u]) => (window as unknown as { decodeAll: Decode }).decodeAll(m, u, true), [workerMixTable(), urls] as const)) as DecodedReply[];
        const off = (await page.evaluate(([m, u]) => (window as unknown as { decodeAll: Decode }).decodeAll(m, u, false), [workerMixTable(), urls] as const)) as DecodedReply[];
        await page.close();

        const ours = (replies: DecodedReply[]) =>
          Array.from({ length: mix.length }, (_, co) => concat(replies.map((r) => fromB64(r.planar[co]))));
        const withDither = ours(on);
        const noDither = ours(off);

        const refBytes = readFileSync(join(dir, "ref_drc.f32"));
        const ref = new Float32Array(refBytes.buffer, refBytes.byteOffset, refBytes.byteLength / 4);
        const refFrames = ref.length / c.channels;

        const rows: string[] = [`${c.name}: residual budget (relative RMS)`];
        for (let co = 0; co < mix.length; co++) {
          const want = new Float32Array(refFrames);
          for (const [src, g] of c.expected[co]) for (let f = 0; f < refFrames; f++) want[f] += ref[f * c.channels + src] * g;
          const n = Math.min(want.length, withDither[co].length);
          const w = want.subarray(0, n);
          const ourNoise = rmsRel(withDither[co].subarray(0, n), noDither[co].subarray(0, n)); // D1: what we add
          const errOn = rmsRel(withDither[co].subarray(0, n), w);   // D1 - D2
          const errOff = rmsRel(noDither[co].subarray(0, n), w);    // - D2 (+ any real defect)
          const budget = Math.sqrt(ourNoise * ourNoise + errOff * errOff); // what errOn must be if the noises are independent
          rows.push(`  ch${co}: ours=${(ourNoise * 100).toFixed(2)}% errOn=${(errOn * 100).toFixed(2)}% errOff=${(errOff * 100).toFixed(2)}% budget=${(budget * 100).toFixed(2)}%`);
        }
        // Measured 2026-09-08 on four library tracks, and what it means:
        //   - below 12 kHz every channel is waveform-coherent with ffmpeg to
        //     1.00, dither or not: mantissas, exponents, allocation, coupling,
        //     the transform and the dynamic range control all agree;
        //   - above 12 kHz the coded part still matches (coherence 0.93-0.98
        //     with our dither off) and each decoder adds its own noise there:
        //     AC-3's bap-0 dither, and in E-AC-3 the spectral-extension noise
        //     (the 7.1 centre loses 49 dB in that band without it);
        //   - with dither on, every band sits at ffmpeg's level to 0.0 dB.
        // So the 1-6% residual is noise against noise, by the format's design,
        // at the same level in the same bands. Nothing is missing, nothing is
        // added, nothing is misplaced. The assertions below hold that.
        // Spectral split of the dither-free disagreement, per channel, against
        // both references: printed, not asserted - this is the diagnosis.
        const edges = [0, 500, 1000, 2000, 3000, 4000, 6000, 8000, 12000, 24000];
        for (const [label, file] of [["drc=on", "ref_drc.f32"], ["drc=off", "ref.f32"]] as const) {
          const rb = readFileSync(join(dir, file));
          const rf = new Float32Array(rb.buffer, rb.byteOffset, rb.byteLength / 4);
          const frames = rf.length / c.channels;
          rows.push(`  spectrum of (ours-without-dither minus ffmpeg ${label}), share of the ERROR energy per band, and error/signal per band:`);
          rows.push(`    bands(Hz): ${edges.slice(1).join(" ")}`);
          for (let co = 0; co < mix.length; co++) {
            const want = new Float32Array(frames);
            for (const [src, g] of c.expected[co]) for (let f = 0; f < frames; f++) want[f] += rf[f * c.channels + src] * g;
            const n = Math.min(want.length, noDither[co].length);
            const diff = new Float32Array(n);
            for (let i = 0; i < n; i++) diff[i] = noDither[co][i] - want[i];
            const eErr = bandEnergies(diff, c.sampleRate, edges);
            const eSig = bandEnergies(want.subarray(0, n), c.sampleRate, edges);
            const tot = eErr.reduce((a, v) => a + v, 0) || 1;
            rows.push(`    ch${co} share: ${eErr.map((v) => ((100 * v) / tot).toFixed(0).padStart(3)).join(" ")} | err/sig%: ${eErr.map((v, i) => (eSig[i] > 0 ? (100 * Math.sqrt(v / eSig[i])).toFixed(1) : "-").padStart(5)).join(" ")}`);
          }
        }
        // Band energies of the three signals themselves, relative to ffmpeg's:
        // whether a band is merely DIFFERENT (uncorrelated noise, same level)
        // or MISSING (ours quieter) is the audible question.
        {
          const rb = readFileSync(join(dir, "ref_drc.f32"));
          const rf = new Float32Array(rb.buffer, rb.byteOffset, rb.byteLength / 4);
          const frames = rf.length / c.channels;
          rows.push(`  band level vs ffmpeg drc=on, dB (ours with dither / ours without dither):`);
          rows.push(`    bands(Hz): ${edges.slice(1).join(" ")}`);
          const db = (a: number, b: number) => (a > 0 && b > 0 ? (10 * Math.log10(a / b)).toFixed(1) : "-");
          for (let co = 0; co < mix.length; co++) {
            const want = new Float32Array(frames);
            for (const [src, g] of c.expected[co]) for (let f = 0; f < frames; f++) want[f] += rf[f * c.channels + src] * g;
            const n = Math.min(want.length, withDither[co].length);
            const eRef = bandEnergies(want.subarray(0, n), c.sampleRate, edges);
            const eOn = bandEnergies(withDither[co].subarray(0, n), c.sampleRate, edges);
            const eOff = bandEnergies(noDither[co].subarray(0, n), c.sampleRate, edges);
            rows.push(`    ch${co}: ${eOn.map((v, i) => `${db(v, eRef[i]).padStart(5)}/${db(eOff[i], eRef[i]).padStart(5)}`).join(" ")}`);
          }
        }
        {
          const rb = readFileSync(join(dir, "ref_drc.f32"));
          const rf = new Float32Array(rb.buffer, rb.byteOffset, rb.byteLength / 4);
          const frames = rf.length / c.channels;
          rows.push(`  waveform coherence per band (ours-without-dither vs ffmpeg / ours-with-dither vs ffmpeg / ours-with vs ours-without):`);
          rows.push(`    bands(Hz): ${edges.slice(1).join(" ")}`);
          for (let co = 0; co < mix.length; co++) {
            const want = new Float32Array(frames);
            for (const [src, g] of c.expected[co]) for (let f = 0; f < frames; f++) want[f] += rf[f * c.channels + src] * g;
            const n = Math.min(want.length, withDither[co].length);
            const cOff = bandCoherence(noDither[co].subarray(0, n), want.subarray(0, n), c.sampleRate, edges);
            const cOn = bandCoherence(withDither[co].subarray(0, n), want.subarray(0, n), c.sampleRate, edges);
            const cSelf = bandCoherence(withDither[co].subarray(0, n), noDither[co].subarray(0, n), c.sampleRate, edges);
            rows.push(`    ch${co}: ${cOff.map((v, i) => `${v.toFixed(2)}/${cOn[i].toFixed(2)}/${cSelf[i].toFixed(2)}`).join(" ")}`);
          }
        }
        // The fold hides a question: is each RAW channel at ffmpeg's level, and
        // is the synthesized noise correlated between channels the same way?
        // Two decoders that draw independent noise per channel sum quieter in
        // a fold than one whose channels share noise. Decode without the fold
        // and look at both.
        if (c.channels > mix.length) {
          const identity = { ...workerMixTable(), [c.channels]: Array.from({ length: c.channels }, (_, i) => [[i, 1]] as Array<[number, number]>) };
          const page2 = await browser.newPage();
          await page2.goto(base + "/");
          const rawOn = (await page2.evaluate(([m, u]) => (window as unknown as { decodeAll: Decode }).decodeAll(m, u, true), [identity, urls] as const)) as DecodedReply[];
          await page2.close();
          const raw = Array.from({ length: c.channels }, (_, ch) => concat(rawOn.map((r) => fromB64(r.planar[ch]))));
          const rb = readFileSync(join(dir, "ref_drc.f32"));
          const rf = new Float32Array(rb.buffer, rb.byteOffset, rb.byteLength / 4);
          const frames = rf.length / c.channels;
          const db = (a: number, b: number) => (a > 0 && b > 0 ? (10 * Math.log10(a / b)).toFixed(1) : "-");
          rows.push(`  RAW channels (no fold), level vs ffmpeg drc=on, dB:`);
          rows.push(`    bands(Hz): ${edges.slice(1).join(" ")}`);
          const refCh = (ch: number) => { const w = new Float32Array(frames); for (let f = 0; f < frames; f++) w[f] = rf[f * c.channels + ch]; return w; };
          for (let ch = 0; ch < c.channels; ch++) {
            const want = refCh(ch);
            const n = Math.min(want.length, raw[ch].length);
            const eRef = bandEnergies(want.subarray(0, n), c.sampleRate, edges);
            const eOn = bandEnergies(raw[ch].subarray(0, n), c.sampleRate, edges);
            rows.push(`    raw${ch}: ${eOn.map((v, i) => db(v, eRef[i]).padStart(5)).join(" ")}`);
          }
          // Coherence BETWEEN the two channels each fold sums, per band, in
          // ffmpeg's decode and in ours: how alike the two noises are.
          for (const [a, b] of [[4, 6], [5, 7]] as const) {
            const n = Math.min(raw[a].length, raw[b].length, frames);
            const cF = bandCoherence(refCh(a).subarray(0, n), refCh(b).subarray(0, n), c.sampleRate, edges);
            const cO = bandCoherence(raw[a].subarray(0, n), raw[b].subarray(0, n), c.sampleRate, edges);
            rows.push(`    coherence raw${a}~raw${b} (ffmpeg / ours): ${cF.map((v, i) => `${v.toFixed(2)}/${cO[i].toFixed(2)}`).join(" ")}`);
          }
        }

        // eslint-disable-next-line no-console
        console.log(rows.join("\n"));

        // The guards. Where a band carries none of our own synthesized noise
        // (switching the dither off changes nothing there), the waveform must
        // be ffmpeg's; where it does, only the level can be compared, and it
        // must match. The split is read off the decode itself rather than a
        // fixed frequency: where the noise starts depends on the stream (a
        // 44.1 kHz stereo track at a low rate dithers from 8 kHz up, a 7.1
        // E-AC-3 track starts its spectral extension near 10 kHz).
        {
          const rb = readFileSync(join(dir, "ref_drc.f32"));
          const rf = new Float32Array(rb.buffer, rb.byteOffset, rb.byteLength / 4);
          const frames = rf.length / c.channels;
          for (let co = 0; co < mix.length; co++) {
            const want = new Float32Array(frames);
            for (const [src, g] of c.expected[co]) for (let f = 0; f < frames; f++) want[f] += rf[f * c.channels + src] * g;
            const n = Math.min(want.length, withDither[co].length);
            const eRef = bandEnergies(want.subarray(0, n), c.sampleRate, edges);
            const eOn = bandEnergies(withDither[co].subarray(0, n), c.sampleRate, edges);
            const cOff = bandCoherence(noDither[co].subarray(0, n), want.subarray(0, n), c.sampleRate, edges);
            const cSelf = bandCoherence(withDither[co].subarray(0, n), noDither[co].subarray(0, n), c.sampleRate, edges);
            const total = eRef.reduce((a, v) => a + v, 0);
            if (total === 0 || want.subarray(0, n).every((v) => Math.abs(v) < QUIET_PEAK)) continue;
            for (let b = 0; b < edges.length - 1; b++) {
              // Bands 60 dB under the channel's total carry nothing to judge.
              if (eRef[b] < total * 1e-6) continue;
              const levelDb = 10 * Math.log10(eOn[b] / eRef[b]);
              // A folded output sums two channels. In a band both fill with
              // synthesized noise, the sum's level depends on how alike the two
              // noises are, and that is each decoder's own business (ours are
              // drawn independently so the noise stays diffuse). Allow the
              // fold that difference, and nothing else.
              const foldedNoise = c.expected[co].length > 1 && cSelf[b] < 0.999;
              expect(Math.abs(levelDb), `${c.name} ch${co} band <${edges[b + 1]} Hz sits ${levelDb.toFixed(1)} dB from ffmpeg\n${rows.join("\n")}`).toBeLessThan(foldedNoise ? 1.5 : 0.5);
              if (cSelf[b] > 0.999) {
                expect(cOff[b], `${c.name} ch${co} band <${edges[b + 1]} Hz carries no noise of ours yet is not the same waveform as ffmpeg\n${rows.join("\n")}`).toBeGreaterThan(0.995);
              }
            }
          }
        }
      } finally {
        await browser.close();
      }
    }, 120_000);
  }
});
