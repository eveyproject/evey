import { useCallback, useEffect, useRef, useState } from "react";
import {
  getProfileSchedule,
  isProfileClosedError,
  onProfileClosed,
  unlockProfileSchedule,
  type ProfileClosedInfo,
} from "@evey/api";
import { useAuthStore } from "@evey/store";

// While a kids profile is closed the lock screen re-reads its state on this
// beat, and once more the moment the announced opening passes, so it lifts by
// itself at 18:00 without anyone touching the device.
const RECHECK_MS = 60_000;

// Every answer is tied to the profile it was asked for: a request still in
// flight when the device switched to another profile must not lock (or
// unlock) the new one.
function isActive(id: string | null | undefined): boolean {
  return !!id && useAuthStore.getState().profileId === id;
}

export interface ProfileLock {
  closed: boolean;
  /** RFC 3339, when the profile opens again; undefined when not within a week. */
  nextOpen?: string;
  /** Lift the lock with a parent's PIN. Resolves to an error code, or null on success. */
  unlock: (pin: string) => Promise<"wrong_pin" | "no_parent_pin" | "rate_limited" | "failed" | null>;
  /** Re-read the state now (after a profile switch, an app resume). */
  recheck: () => void;
}

/**
 * The lock state of the active profile, shared by the four apps. It turns on
 * when any request is refused as profile_closed (the fetcher signals it), and
 * off when the server says the profile is open again. onReopen lets the app
 * refetch what failed while it was closed.
 */
export function useProfileLock(onReopen?: () => void): ProfileLock {
  const profileId = useAuthStore((s) => s.profileId);
  const [state, setState] = useState<{ closed: boolean; nextOpen?: string }>({ closed: false });
  const reopenRef = useRef(onReopen);
  reopenRef.current = onReopen;

  const closedRef = useRef(false);
  const apply = useCallback((closed: boolean, nextOpen?: string) => {
    const wasClosed = closedRef.current;
    closedRef.current = closed;
    setState((prev) => (prev.closed === closed && prev.nextOpen === nextOpen ? prev : { closed, nextOpen }));
    if (wasClosed && !closed) reopenRef.current?.();
  }, []);

  const recheck = useCallback(() => {
    if (!profileId) return;
    const asked = profileId;
    getProfileSchedule(asked)
      .then((s) => {
        if (isActive(asked)) apply(!s.access.open, s.access.nextOpen);
      })
      .catch(() => {
        // A failed read leaves the lock as it is: closing again is the
        // server's job, on the next refused request.
      });
  }, [profileId, apply]);

  useEffect(
    () =>
      onProfileClosed((info: ProfileClosedInfo) => {
        // A refusal naming another profile is a late answer for one the
        // device has left; one naming none predates the field, so it counts.
        if (info.profileId && !isActive(info.profileId)) return;
        apply(true, info.nextOpen);
      }),
    [apply],
  );

  // A different profile starts unlocked; the server says otherwise on its
  // first refused request.
  useEffect(() => {
    closedRef.current = false;
    setState({ closed: false });
  }, [profileId]);

  useEffect(() => {
    if (!state.closed) return;
    const timer = setInterval(recheck, RECHECK_MS);
    let atOpening: ReturnType<typeof setTimeout> | undefined;
    const until = state.nextOpen ? Date.parse(state.nextOpen) - Date.now() : NaN;
    if (Number.isFinite(until) && until > 0 && until < 24 * 3600_000) {
      atOpening = setTimeout(recheck, until + 1_000);
    }
    return () => {
      clearInterval(timer);
      if (atOpening) clearTimeout(atOpening);
    };
  }, [state.closed, state.nextOpen, recheck]);

  const unlock = useCallback(
    async (pin: string) => {
      if (!profileId) return "failed" as const;
      const asked = profileId;
      try {
        const s = await unlockProfileSchedule(asked, pin);
        if (!isActive(asked)) return "failed" as const;
        apply(!s.access.open, s.access.nextOpen);
        return s.access.open ? null : ("failed" as const);
      } catch (err) {
        const e = err as { httpStatus?: number };
        if (isProfileClosedError(err)) return "failed" as const;
        if (e.httpStatus === 403) return "wrong_pin" as const;
        if (e.httpStatus === 409) return "no_parent_pin" as const;
        if (e.httpStatus === 429) return "rate_limited" as const;
        return "failed" as const;
      }
    },
    [profileId, apply],
  );

  return { closed: state.closed, nextOpen: state.nextOpen, unlock, recheck };
}
