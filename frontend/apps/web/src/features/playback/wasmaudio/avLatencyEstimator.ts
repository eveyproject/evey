// Estimator for the audio-lead compensation MEASURED, instead of a
// wet-finger constant: the real gap between what the <video> displays and
// what WebAudio outputs varies from one pipeline to another (~100ms on a
// desktop, 500-600ms measured on a Mi TV as a cast receiver, 2026-07-25).
//
// The sample provided by the caller = (video display latency) - (audio
// output latency), where the video latency comes from requestVideoFrameCallback
// (playhead - mediaTime of the presented frame + compositor-to-panel delay) and
// the audio latency from AudioContext.outputLatency. The estimator smooths
// via EMA, discards outlier samples, and only declares itself usable after a
// minimum number of samples - before that, the caller keeps its fallback constant.

// Plausibility bounds for a sample. Negative = the audio sink is slower than
// the picture (a Bluetooth speaker or a TV over HDMI reports 0.2-1 s of
// outputLatency) and the audio must be scheduled EARLY by that much: clamping
// it to 0 left the sound that late (prod 2026-09-11, -900 ms by ear). Beyond
// +2 s = implausible pipeline, more likely a stall.
const SAMPLE_MIN_SEC = -2;
const SAMPLE_MAX_SEC = 2;
const ESTIMATE_MIN_SEC = -1.5;
const ESTIMATE_MAX_SEC = 1.5;
const EMA_ALPHA = 0.05;
const MIN_SAMPLES = 10;

export class AVLatencyEstimator {
  private ema: number | null = null;
  private samples = 0;

  update(sampleSec: number): void {
    if (!Number.isFinite(sampleSec)) return;
    if (sampleSec < SAMPLE_MIN_SEC || sampleSec > SAMPLE_MAX_SEC) return;
    this.samples++;
    this.ema = this.ema === null ? sampleSec : this.ema * (1 - EMA_ALPHA) + sampleSec * EMA_ALPHA;
  }

  /** Smoothed, bounded estimate, or null until the measurement is mature enough. */
  get estimate(): number | null {
    if (this.ema === null || this.samples < MIN_SAMPLES) return null;
    return Math.min(ESTIMATE_MAX_SEC, Math.max(ESTIMATE_MIN_SEC, this.ema));
  }
}
