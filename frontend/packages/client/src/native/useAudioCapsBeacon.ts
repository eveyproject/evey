import { useEffect } from "react";
import { eveyFetch } from "@evey/api";
import { formatAbnormalExit, formatDecoderCheck, getAudioProbe, reportAudioProbeOnce, type DecoderCheck } from "@evey/media-caps";

let checkSent = false;

/**
 * Once per launch: the verdict on the app's own decoder (benchmark, what was
 * declared, and a previous run that died while it decoded - the one trace a
 * crash on a TV out of reach leaves).
 */
export async function reportDecoderCheckOnce(
  check: Promise<DecoderCheck | null>,
  post: (body: { type: string; details: string; fatal: boolean }) => Promise<unknown>,
): Promise<void> {
  if (checkSent) return;
  checkSent = true;
  const c = await check;
  if (!c) return;
  const exit = c.abnormalExit;
  try {
    if (exit) await post({ type: "ac3go_abnormal_exit", details: formatAbnormalExit(exit), fatal: exit.suspect === true });
    await post({ type: "ac3go_check", details: formatDecoderCheck(c), fatal: false });
  } catch {
    /* latched anyway: a refused beacon is refused again on retry */
  }
}

/** Test seam. */
export function resetDecoderCheckLatch(): void {
  checkSent = false;
}

/**
 * Diagnostic only: reports, once per app launch, what this box's audio sink
 * actually declares (HDMI encodings, direct-playback support, the Android TV
 * surround setting, decoders). It changes NOTHING about what we declare to the
 * server.
 *
 * Diagnostic does NOT mean the probe is unused: the declaration path reads the
 * same getAudioProbe(), through resolveAudioCodecs -> declarableAudioCodecs in
 * lib/deviceCodecs.ts, and that is what usePlaySession sends as
 * supportedAudioCodecs. The static ANDROID_TV_BASELINE is only the floor for a
 * box whose probe answered nothing.
 *
 * This comment used to say the baseline omitted ac3/eac3 because a
 * decoder-only probe could not tell a Dolby decoder from a passthrough box,
 * and that this beacon was how it would be settled. It WAS settled, on
 * 2026-08-31: the SINK decides, not the decoder. Read alone the old wording
 * suggested nothing was wired at all, which is how an already-closed item came
 * back onto a backlog.
 */
export function useAudioCapsBeacon(mediaId: string): void {
  useEffect(() => {
    if (!mediaId) return;
    const post = (body: { type: string; details: string; fatal?: boolean }) =>
      eveyFetch({
        url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
        method: "POST",
        data: { fatal: false, ...body, code: 0, variant: 0, segment: 0, posSec: 0 },
      });
    void reportAudioProbeOnce(getAudioProbe, post);
    // Lazy: deviceCodecs probes the device as it loads.
    void import("./deviceCodecs").then((m) => reportDecoderCheckOnce(m.getDecoderCheck(), post));
  }, [mediaId]);
}
