// Estimateur de la compensation d'avance audio MESURÉE, au lieu d'une
// constante au doigt mouillé : l'écart réel entre ce que la <video> affiche et
// ce que WebAudio sort varie d'un pipeline à l'autre (~100 ms sur un desktop,
// 500-600 ms mesurés sur une Mi TV en receiver cast, 2026-07-25).
//
// L'échantillon fourni par l'appelant = (latence d'affichage vidéo) - (latence
// de sortie audio), où la latence vidéo vient de requestVideoFrameCallback
// (playhead - mediaTime de la frame présentée + délai compositeur→dalle) et la
// latence audio d'AudioContext.outputLatency. L'estimateur lisse en EMA, jette
// les échantillons aberrants, et ne se déclare utilisable qu'après un minimum
// d'échantillons - avant ça, l'appelant garde sa constante de repli.

// Bornes de plausibilité d'un échantillon : négatif au-delà du jitter = mesure
// cassée ; au-delà de 2 s = pipeline invraisemblable, plus sûrement un stall.
const SAMPLE_MIN_SEC = -0.2;
const SAMPLE_MAX_SEC = 2;
// L'estimation finale reste dans une plage sûre : jamais d'audio avancé (< 0),
// jamais plus de 1,5 s de retard imposé.
const ESTIMATE_MIN_SEC = 0;
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

  /** Estimation lissée et bornée, ou null tant que la mesure n'est pas mûre. */
  get estimate(): number | null {
    if (this.ema === null || this.samples < MIN_SAMPLES) return null;
    return Math.min(ESTIMATE_MAX_SEC, Math.max(ESTIMATE_MIN_SEC, this.ema));
  }
}
