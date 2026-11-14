import { useCallback, useRef } from "react";
import { DEFAULT_SUBTITLE_STYLE, type SubtitleStyle } from "../lib/subtitleStyle";
import { usePlayerProfilePrefs } from "./usePlayerProfilePrefs";

/**
 * The subtitle appearance as a profile setting, for the settings screens: the
 * same value and the same save as the players' own menu, so a change made
 * here is the one the next title opens with, on every client. A profile that
 * never chose shows the defaults.
 */
export function useSubtitleStyleSetting(): {
  style: SubtitleStyle;
  update: (patch: Partial<SubtitleStyle>) => void;
  ready: boolean;
} {
  const { profile, subtitleStyle, saveSubtitleStyle } = usePlayerProfilePrefs();
  const style = subtitleStyle ?? DEFAULT_SUBTITLE_STYLE;
  // Two controls moved in the same tick must both land: the second patch
  // builds on the first, not on the value of the last render.
  const latest = useRef(style);
  latest.current = style;
  const update = useCallback((patch: Partial<SubtitleStyle>) => {
    const next = { ...latest.current, ...patch };
    latest.current = next;
    saveSubtitleStyle(next);
  }, [saveSubtitleStyle]);
  return { style, update, ready: !!profile?.id };
}
