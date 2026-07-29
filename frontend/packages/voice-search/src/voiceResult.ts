// The three - and only three - outcomes a caller ever has to branch on. There
// is deliberately no error variant: a probe failure, a missing module, a
// declined system dialog and a user who says nothing are all "there is no
// text", not exceptions the search screen has to special-case.
export type VoiceSearchResult =
  | { status: "recognized"; text: string }
  // No native module in this build, no recognizer app on this device, or the
  // permission dialog the recognizer itself shows was refused.
  | { status: "unavailable" }
  // The recognizer ran but the user cancelled, or it heard nothing usable.
  | { status: "empty" };

/**
 * Turn raw native results into a VoiceSearchResult. Pure and native-free on
 * purpose - see index.ts for why the native call itself cannot be unit
 * tested the same way.
 */
export function toVoiceSearchResult(available: boolean, text: string | null | undefined): VoiceSearchResult {
  if (!available) return { status: "unavailable" };
  const trimmed = text?.trim();
  if (!trimmed) return { status: "empty" };
  return { status: "recognized", text: trimmed };
}
