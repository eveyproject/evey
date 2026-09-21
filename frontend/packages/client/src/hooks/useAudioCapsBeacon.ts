import { useEffect } from "react";
import { eveyFetch } from "@evey/api";
import { getAudioProbe, reportAudioProbeOnce } from "@evey/media-caps";

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
    void reportAudioProbeOnce(getAudioProbe, (body) =>
      eveyFetch({
        url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
        method: "POST",
        data: { ...body, fatal: false, code: 0, variant: 0, segment: 0, posSec: 0 },
      }),
    );
  }, [mediaId]);
}
