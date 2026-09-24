import { useCallback, useEffect, useRef, useState } from "react";

type SleepTimerKind = "minutes" | "endOfEpisode";

export interface SleepTimerState {
  kind: SleepTimerKind;
  minutesLeft: number | null;
}

interface UseSleepTimerArgs {
  onMinutesFire: () => void;
}

/**
 * The sleep timer every player shares. Two modes:
 *   - "minutes": tick every minute, fire `onMinutesFire` when the countdown
 *     reaches zero (caller pauses the player).
 *   - "endOfEpisode": no countdown - the host watches the player's "ended"
 *     event and reads `state.kind === "endOfEpisode"` to decide whether to
 *     stop after the current item.
 */
// The presets every player offers, and which one reads as chosen: the one the
// countdown still falls within. Four copies until 2026-09-24.
export const SLEEP_TIMER_PRESETS_MIN = [15, 30, 45, 60, 90] as const;

export function isSleepPresetActive(state: SleepTimerState | null | undefined, minutes: number): boolean {
  return state?.kind === "minutes" && state.minutesLeft != null && state.minutesLeft <= minutes && state.minutesLeft > minutes - 1;
}

export function useSleepTimer({ onMinutesFire }: UseSleepTimerArgs) {
  const [state, setState] = useState<SleepTimerState | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const fireRef = useRef(onMinutesFire);
  fireRef.current = onMinutesFire;

  const clearInternal = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const cancel = useCallback(() => {
    clearInternal();
    setState(null);
  }, [clearInternal]);

  const setMinutes = useCallback(
    (minutes: number) => {
      clearInternal();
      if (minutes <= 0) {
        setState(null);
        return;
      }
      setState({ kind: "minutes", minutesLeft: minutes });
      intervalRef.current = setInterval(() => {
        setState((prev) => {
          if (!prev || prev.kind !== "minutes" || prev.minutesLeft === null) return prev;
          const next = prev.minutesLeft - 1;
          if (next <= 0) {
            clearInternal();
            fireRef.current();
            return null;
          }
          return { kind: "minutes", minutesLeft: next };
        });
      }, 60_000);
    },
    [clearInternal],
  );

  const setEndOfEpisode = useCallback(() => {
    clearInternal();
    setState({ kind: "endOfEpisode", minutesLeft: null });
  }, [clearInternal]);

  useEffect(() => {
    return () => clearInternal();
  }, [clearInternal]);

  return { state, setMinutes, setEndOfEpisode, cancel };
}
