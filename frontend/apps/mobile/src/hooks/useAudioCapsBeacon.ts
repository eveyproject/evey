import { useEffect } from "react";
import { eveyFetch } from "@evey/api";
import { getAudioProbe, reportAudioProbeOnce } from "@evey/media-caps";

/**
 * Diagnostic only: reports, once per app launch, what this device's audio sink
 * actually declares (HDMI encodings, direct-playback support, the Android TV
 * surround setting, decoders). It changes NOTHING about what we declare to the
 * server - the point is to learn what real boxes say before touching the
 * declaration, since a decoder-only probe answers the wrong question on a box
 * that passes Dolby through to a receiver.
 *
 * Rides the existing player-event route, so there is no server change and the
 * line lands in the admin log next to the rest of that playback's story.
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
