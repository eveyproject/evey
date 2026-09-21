// Shared implementation lives in @evey/client (packages/client/src/hooks/useParentalGate.ts).
// This module keeps the path the screens and their tests import.
import { configureParentalUnlock } from "@evey/client";

// A television's remote is shared by the whole living room: an unlock is
// forgotten after half an hour, and a profile switch clears it (clearUnlocked).
configureParentalUnlock({ ttlMs: 30 * 60 * 1000 });

export { isAgeExceeded, configureParentalUnlock, isProfileUnlocked, markUnlocked, clearUnlocked, pinRequiredFor, useParentalGate } from "@evey/client";
export type { ParentalUnlockStore, ParentalUnlockOptions, ParentalSettings } from "@evey/client";
