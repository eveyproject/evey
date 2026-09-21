// Shared implementation lives in @evey/client (packages/client/src/hooks/useParentalGate.ts).
// This module keeps the path the screens and their tests import.
export { isAgeExceeded, configureParentalUnlock, isProfileUnlocked, markUnlocked, clearUnlocked, pinRequiredFor, useParentalGate } from "@evey/client";
export type { ParentalUnlockStore, ParentalUnlockOptions, ParentalSettings } from "@evey/client";
