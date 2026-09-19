// Which HDR formats the attached DISPLAY renders, and what to declare from it.
//
// Third member of the same family as the audio half of this package: the codec
// declaration asks the SINK, not the decoder (declarableAudioCodecs), the
// channel ceiling asks the OUTPUT, not the decoder (resolveMaxOutputChannels),
// and HDR asks the PANEL, not the decoder. A box whose HEVC decoder eats Main10
// all day still needs the server's tone-map when the screen behind the HDMI
// cable is SDR.

/** evey names for Display.HdrCapabilities types (Android). */
export type DisplayHdrType = "hdr10" | "hlg" | "hdr10plus" | "dovi";

// The server's ClientProfile carries ONE bool, and treats it as "HDR10-capable
// panel" (decision.go: a Dolby Vision source tone-maps server-side whatever the
// client says, because a single bool cannot tell a DoVi panel from an HDR10-only
// one). So a display that reports Dolby Vision ALONE must not set it: we would
// be claiming an HDR10 capability nothing has evidenced.
const HDR10_FAMILY: readonly string[] = ["hdr10", "hlg", "hdr10plus"];

/**
 * Whether to declare `?hdr=1` for this display. No evidence (empty list, null,
 * probe absent) means NO: a PQ source re-encoded without tone-mapping renders
 * washed out, so guessing HDR is the expensive mistake and guessing SDR only
 * costs an encode the server was already doing.
 */
export function displaySupportsHdr(types: readonly string[] | null | undefined): boolean {
  if (!types) return false;
  return types.some((t) => HDR10_FAMILY.includes(t.toLowerCase()));
}
