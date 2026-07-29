import { useEffect } from "react";
import { eveyFetch } from "@evey/api";
import { getAudioProbe, reportAudioProbeOnce } from "@evey/media-caps";

/**
 * Diagnostic only: reports, once per app launch, what this box's audio sink
 * actually declares (HDMI encodings, direct-playback support, the Android TV
 * surround setting, decoders). It changes NOTHING about what we declare to the
 * server.
 *
 * This is the app where it matters most: the TV baseline currently omits
 * ac3/eac3 because a decoder-only probe cannot tell a box with a Dolby decoder
 * from one that merely passes the bitstream through to a receiver - and the
 * second case is the common one on a TV. This beacon is how that gets settled
 * on evidence from real hardware instead of a guess.
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
