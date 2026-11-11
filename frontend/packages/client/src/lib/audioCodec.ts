// The server folds codec aliases before it compares a track with a client's
// declaration (normalizeAudioCodec): the clients fold the same way.
const AUDIO_ALIASES: Record<string, string> = {
  "ac-3": "ac3",
  "e-ac-3": "eac3",
  "ec-3": "eac3",
  ec3: "eac3",
  dca: "dts",
  mlp: "truehd",
};

/** The one name the server compares a codec under (dca is dts, ec-3 is eac3). */
export function canonicalAudioCodec(codec: string): string {
  const c = codec.trim().toLowerCase();
  return AUDIO_ALIASES[c] ?? c;
}

/** Whether a declaration names the codec, aliases folded. */
export function declaresAudioCodec(declared: readonly string[], codec: string): boolean {
  const want = canonicalAudioCodec(codec);
  return declared.some((c) => canonicalAudioCodec(c) === want);
}
