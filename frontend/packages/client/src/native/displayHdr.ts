import { displaySupportsHdr, getDisplayHdrTypes } from "@evey/media-caps";

// Twin of apps/mobile/src/lib/displayHdr.ts - keep the SEMANTICS in sync when
// touching either side, like deviceCodecs.ts and outputChannels.ts already are.
//
// HDR capability of the RESTITUTION chain (what the panel renders), not of the
// decoder. The TV declared nothing at all until 2026-09-19, so the server saw
// an SDR client on every title and tone-mapped every HDR source - including the
// ones a HDR panel would have direct-played. Same shape as the two audio
// probes next door: the sink decides, not the decoder.

/**
 * Whether to declare `?hdr=1` for the display currently attached. False when
 * the box cannot answer (module absent, probe failure): no evidence means the
 * server keeps its tone-map, which is the safe direction - a PQ source encoded
 * without tone-mapping renders washed out.
 *
 * Probed at every stream negotiation, never memoised at startup: the display
 * changes mid-session (HDMI re-plugged, AV receiver powered on, mode switch).
 * A playback already running keeps the capability it negotiated - re-negotiating
 * mid-scene would reload the manifest - so a new screen takes effect at the
 * next natural boundary (track switch, quality change, next episode, replay).
 */
export async function probeDisplayHdr(): Promise<boolean> {
  return displaySupportsHdr(await getDisplayHdrTypes());
}
