import { toVoiceSearchResult, type VoiceSearchResult } from "./voiceResult";

export type { VoiceSearchResult } from "./voiceResult";

interface EveyVoiceSearchNative {
  isAvailable(): Promise<boolean>;
  recognize(): Promise<string | null>;
}

/**
 * Launch the system voice recognizer and return whatever it heard.
 *
 * Never throws: a device with no recognizer, a build without this native
 * module, and a user who cancels the system dialog all degrade to a
 * VoiceSearchResult the caller can render without a special case - the
 * keyboard search box this sits next to keeps working exactly as before.
 *
 * expo-modules-core is imported LAZILY, not at module load: its top-level
 * code reads globalThis.expo, which only exists inside a real RN runtime. A
 * static import would make this package unimportable from a plain node test
 * - see @evey/media-caps/src/index.ts, which established the pattern.
 */
export async function startVoiceSearch(): Promise<VoiceSearchResult> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyVoiceSearchNative>("EveyVoiceSearch");
    if (!native) return toVoiceSearchResult(false, null);
    const available = await native.isAvailable();
    if (!available) return toVoiceSearchResult(false, null);
    const text = await native.recognize();
    return toVoiceSearchResult(true, text);
  } catch {
    return toVoiceSearchResult(false, null);
  }
}
