// Shared implementation lives in @evey/client (packages/client/src/hooks/useParentalGate.ts).
// This module keeps the path the screens and their tests import.
import { configureParentalUnlock } from "@evey/client";

// The web keeps the unlocked profiles for the browser session: a reload must
// not ask for the PIN again, closing the tab must.
const SESSION_KEY = "evey:parental_unlocked";
configureParentalUnlock({
  store: {
    load: () => {
      try {
        const raw = sessionStorage.getItem(SESSION_KEY);
        return new Set(raw ? (JSON.parse(raw) as string[]) : []);
      } catch {
        return new Set();
      }
    },
    save: (ids) => {
      try {
        sessionStorage.setItem(SESSION_KEY, JSON.stringify([...ids]));
      } catch {
        // Storage refused (private mode, quota): the gate simply asks again.
      }
    },
  },
});

export { isAgeExceeded, configureParentalUnlock, isProfileUnlocked, markUnlocked, clearUnlocked, pinRequiredFor, useParentalGate } from "@evey/client";
export type { ParentalUnlockStore, ParentalUnlockOptions, ParentalSettings } from "@evey/client";
