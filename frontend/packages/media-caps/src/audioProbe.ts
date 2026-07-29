// Raw evidence returned by the native probe. Every platform integer is kept
// raw on purpose - see the module's Kotlin doc comment: this beacon exists to
// correct our assumptions about real boxes, so it must not pre-digest them.
export interface AudioProbe {
  apiLevel: number;
  isTv: boolean;
  deviceModel: string;
  /** evey codec names this device has a DECODER for. */
  decoders: string[];
  /** Can the current sink take this compressed format directly (API 29+). */
  directPlayback: Record<string, boolean>;
  /**
   * API 33+ replacement for the above: the profiles the sink accepts without
   * decoding. Carries channel masks, so it also says whether 5.1 survives or
   * gets downmixed. Empty below API 33.
   */
  directProfiles: { encoding: number; channelMasks: number[] }[];
  hdmi: {
    available: boolean;
    plugState?: number;
    /** Raw AudioFormat.ENCODING_* values advertised by the HDMI sink. */
    encodings?: number[];
    maxChannelCount?: number;
  };
  /** Raw Settings.Global "encoded_surround_output"; -1 when unreadable. */
  surroundOutputRaw: number;
  /** Raw AudioDeviceInfo.TYPE_* of the current output devices. */
  outputDeviceTypes: number[];
}

// The server clamps a player-event `details` string to 200 characters
// (player_event_handler.go). Going over doesn't error - it TRUNCATES silently,
// which would quietly drop whichever field we put last. So the formatter emits
// fields in decreasing evidential value and stops before the limit rather than
// letting the server cut mid-token.
export const MAX_DETAIL_LEN = 200;

// The codecs the whole question is about. Listing every decoder would spend the
// budget on aac/mp3/flac, which were never in doubt.
const DOLBY_FAMILY = ["ac3", "eac3", "dts", "truehd"];

function truthyKeys(map: Record<string, boolean>): string[] {
  return Object.keys(map).filter((k) => map[k]);
}

/**
 * Compact one-line rendering of the probe for the player-event beacon.
 *
 * Field order is the priority order: direct-playback support and the HDMI
 * encodings are the two that actually answer "does this box declare itself
 * capable"; the model name is last because the server already logs the
 * User-Agent alongside it.
 */
export function formatAudioProbe(probe: AudioProbe): string {
  const dp = truthyKeys(probe.directPlayback);
  const fields: string[] = [
    `dp=${dp.length ? dp.join("+") : "none"}`,
    `enc=${probe.hdmi.encodings?.length ? probe.hdmi.encodings.join("+") : "none"}`,
    `prof=${probe.directProfiles.length ? probe.directProfiles.map((p) => p.encoding).join("+") : "none"}`,
    `sur=${probe.surroundOutputRaw}`,
    `plug=${probe.hdmi.available ? (probe.hdmi.plugState ?? -1) : "na"}`,
    `ch=${probe.hdmi.maxChannelCount ?? -1}`,
    `dec=${probe.decoders.filter((c) => DOLBY_FAMILY.includes(c)).join("+") || "none"}`,
    `api=${probe.apiLevel}`,
    `tv=${probe.isTv ? 1 : 0}`,
    `out=${probe.outputDeviceTypes.join("+") || "none"}`,
    `model=${probe.deviceModel}`,
  ];

  let out = "";
  for (const field of fields) {
    const next = out ? `${out} ${field}` : field;
    // Drop a field that would overflow rather than emitting a half token the
    // server would truncate anyway; keep scanning, a later short field may fit.
    if (next.length > MAX_DETAIL_LEN) continue;
    out = next;
  }
  return out;
}
