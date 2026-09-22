import * as SecureStore from "expo-secure-store";
import { DEFAULT_SUBTITLE_STYLE, type SubtitleStyle } from "../lib/subtitleStyle";

// The model lives in ../lib/subtitleStyle; this file only keeps the choice.
export * from "../lib/subtitleStyle";

const STYLE_KEY = "evey-subtitle-style";

/** null when nothing is stored, the keystore read fails, or the value is corrupt. */
export async function getSubtitleStylePref(): Promise<SubtitleStyle | null> {
  try {
    const raw = await SecureStore.getItemAsync(STYLE_KEY);
    if (!raw) return null;
    return { ...DEFAULT_SUBTITLE_STYLE, ...(JSON.parse(raw) as Partial<SubtitleStyle>) };
  } catch {
    return null;
  }
}

export async function saveSubtitleStylePref(style: SubtitleStyle): Promise<void> {
  try {
    await SecureStore.setItemAsync(STYLE_KEY, JSON.stringify(style));
  } catch {
    /* no keystore available: the preference just won't stick */
  }
}
