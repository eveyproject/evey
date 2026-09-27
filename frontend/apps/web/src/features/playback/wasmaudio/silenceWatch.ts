// Decoded-silence watch for WasmAudioEngine: a wasm leg that decodes without
// error but hands back pure digital silence. The stall gate, the starvation
// guard and the audio-health checks all see a healthy leg then - segments
// decode, the clock runs, sources are scheduled - and the viewer hears nothing.
// That is how ac3go's silent E-AC-3 stereo downmix reached two phones unseen
// (2026-09-27); the cast receiver and the web player run the same decoder.
// Pure judgement: the engine feeds it each wasm segment in decode order.

import type { PcmDecoded } from "./WasmAudioEngine";

/**
 * Seconds of consecutive exact-zero wasm audio before the alarm. ac3go
 * dithers the bands a quiet frame leaves empty, so a quiet scene is never
 * bit-for-bit zero for this long; a lost track is.
 */
export const SILENT_RUN_ALARM_SEC = 10;

/** True when every sample of a wasm decode is exactly zero. */
export function pcmIsSilent(d: PcmDecoded): boolean {
  const planes = "planar" in d ? d.planar : [d.pcm];
  for (const p of planes) {
    for (let i = 0; i < p.length; i++) if (p[i] !== 0) return false;
  }
  return true;
}

/**
 * SilenceRun accumulates the silent seconds of CONSECUTIVE segments (by
 * index: a seek or a gap starts a new run) and says once per engine when the
 * run crosses the alarm.
 */
export class SilenceRun {
  private lastIdx = -2;
  private runSec = 0;
  private alarmed = false;
  /** Silent seconds and segments seen in total, for the report. */
  silentSec = 0;
  segments = 0;

  constructor(private readonly alarmSec = SILENT_RUN_ALARM_SEC) {}

  /** Record one decoded wasm segment; true exactly once, when the run first crosses the alarm. */
  record(idx: number, durSec: number, silent: boolean): boolean {
    this.segments++;
    if (idx !== this.lastIdx + 1) this.runSec = 0;
    this.lastIdx = idx;
    if (!silent) {
      this.runSec = 0;
      return false;
    }
    this.silentSec += durSec;
    this.runSec += durSec;
    if (this.alarmed || this.runSec < this.alarmSec) return false;
    this.alarmed = true;
    return true;
  }

  get currentRunSec(): number {
    return this.runSec;
  }
}
