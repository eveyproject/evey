import { formatAudioProbe, type AudioProbe } from "./audioProbe";

export type ProbePoster = (body: { type: string; details: string }) => Promise<unknown>;

export type ReportOutcome = "sent" | "skipped-already-sent" | "skipped-no-probe" | "post-failed";

// One beacon per app launch. The capability picture only changes when the user
// re-plugs an AVR or flips the surround setting, neither of which happens
// mid-session often enough to be worth a beacon on every play - and a per-play
// beacon would bury the admin log the way the cast liveness beats once did.
let alreadySent = false;

/** Test seam: the module-level latch would otherwise leak between test cases. */
export function resetAudioProbeLatch(): void {
  alreadySent = false;
}

/**
 * Fire the diagnostic beacon once, if the native probe is present.
 *
 * Deliberately reports RAW evidence and changes NOTHING about what the app
 * declares to the server: the point is to learn what real boxes say before
 * touching the declaration. Never throws - telemetry must not break playback.
 */
export async function reportAudioProbeOnce(
  probeFn: () => Promise<AudioProbe | null>,
  post: ProbePoster,
): Promise<ReportOutcome> {
  if (alreadySent) return "skipped-already-sent";
  let probe: AudioProbe | null = null;
  try {
    probe = await probeFn();
  } catch {
    probe = null;
  }
  // No native module (iOS/tvOS, or a build predating it) is not a failure -
  // there is simply nothing to report, and we must not latch so a later build
  // still gets its chance.
  if (!probe) return "skipped-no-probe";
  alreadySent = true;
  try {
    await post({ type: "audio_caps_probe", details: formatAudioProbe(probe) });
    return "sent";
  } catch {
    // Latch stays set: a server that refused this beacon will refuse the
    // retry too, and retrying on every play is how you flood a log.
    return "post-failed";
  }
}
