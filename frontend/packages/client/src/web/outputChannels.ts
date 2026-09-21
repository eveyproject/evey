// The channel ceiling of the RESTITUTION chain - what the speakers/headphones
// behind this browser actually reproduce. It is NOT a decode capability: every
// browser decodes 5.1 AAC happily and then folds it to the stereo output with
// the "speakers" matrix (L=1, R=1, C=.7071, LFE=0, SL=.7071, SR=.7071), whose
// worst case sums to 2.41 (+7.66 dB) with no normalisation whatsoever. Measured
// on Chromium and Firefox: a 5.1 source peaking at -1 dBFS clips 1.66% of its
// samples on an action scene while the dialogue (centre, attenuated, alone)
// stays clean - exactly the "voices fine, music and effects saturate" report.
// Declaring the real ceiling to the server moves the down-mix to ffmpeg, which
// normalises it.
//
// AudioContext.destination.maxChannelCount is the one web API that answers the
// restitution question. navigator.mediaCapabilities.decodingInfo does not: it
// answers "can this device DECODE 6 channels", which is yes on a laptop with
// two speakers.

// The server clamps the query to 1..8 (maxAudioChannelsQuery); mirror it here
// so a JACK/pro output reporting 32 channels does not produce a rejected value.
const MAX_DECLARABLE = 8;

// clampOutputChannels turns a raw platform reading into a value worth
// declaring, or undefined when the platform said nothing usable. Undefined is
// a real answer: we omit the parameter rather than invent a ceiling.
export function clampOutputChannels(raw: unknown): number | undefined {
  if (typeof raw !== "number" || !Number.isFinite(raw) || raw < 1) return undefined;
  return Math.min(Math.floor(raw), MAX_DECLARABLE);
}

let cached: number | undefined;
let probed = false;
let watching = false;

function probeDestination(): number | undefined {
  if (typeof window === "undefined") return undefined;
  const Ctor: typeof AudioContext | undefined =
    window.AudioContext ??
    (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!Ctor) return undefined;
  let ctx: AudioContext | undefined;
  try {
    ctx = new Ctor();
    return clampOutputChannels(ctx.destination?.maxChannelCount);
  } catch {
    return undefined;
  } finally {
    // A context left open counts against the browser's per-page limit; this one
    // exists only to read a number.
    void ctx?.close?.().catch(() => {});
  }
}

// The output can change mid-session (headphones plugged, a TV connected). The
// probe is cached because building a stream URL happens during a render, but
// the cache is dropped on devicechange so the NEXT negotiation re-probes. A
// playback already in flight keeps the layout it was negotiated with: changing
// it would mean tearing the session down and reloading the manifest mid-scene,
// which costs more than the 3 dB it would win back. Track switch, quality
// change, next episode and a fresh play all re-negotiate, so the new output is
// picked up at the first natural boundary.
function watchOutputChanges(): void {
  if (watching || typeof navigator === "undefined") return;
  const md = navigator.mediaDevices as MediaDevices | undefined;
  if (typeof md?.addEventListener !== "function") return;
  watching = true;
  md.addEventListener("devicechange", resetOutputChannelsProbe);
}

export function resetOutputChannelsProbe(): void {
  probed = false;
  cached = undefined;
}

/**
 * Channels the output device reproduces, or undefined when this browser has no
 * AudioContext to ask. Memoised until a devicechange invalidates it.
 */
export function getMaxOutputChannels(): number | undefined {
  if (probed) return cached;
  probed = true;
  watchOutputChanges();
  cached = probeDestination();
  return cached;
}
