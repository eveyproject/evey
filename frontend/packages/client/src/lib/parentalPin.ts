import { parentalServiceVerifyPin } from "@evey/api";

/**
 * Checking a parental PIN, once, for every client.
 *
 * Two rules, each learned the hard way and each previously held by only one
 * client:
 *
 *  - The profile being LEFT is tried first. The server lets a restricted
 *    profile be left on its own PIN or on the target's, but verify-pin
 *    against a profile that carries NO PIN answers "valid" to any digits and
 *    mints a proof the switch then refuses (measured on the dev server,
 *    2026-09-22: 9999 accepted by a PIN-less target, the prompt closed, the
 *    switch failed with nothing said). Asking the profile known to be
 *    restricted first removes that hole; the target is still tried after, for
 *    a target that holds its own PIN.
 *  - A real network failure is NOT a wrong PIN. Only a refusal carrying an
 *    HTTP status falls through; anything else is rethrown so the caller can
 *    say "the server did not answer" instead of blaming the viewer.
 */
export interface VerifyParentalPinInput {
  /** The profile being opened. */
  targetProfileId: string | undefined;
  /** The profile being left, when this is a profile switch. */
  leavingProfileId?: string;
  pin: string;
}

/**
 * The short-lived proof to hand to the switch, "" when the server said valid
 * without minting one (legacy contract), or null when the PIN was refused.
 */
export async function verifyParentalPin({ targetProfileId, leavingProfileId, pin }: VerifyParentalPinInput): Promise<string | null> {
  const order = [
    ...(leavingProfileId && leavingProfileId !== targetProfileId ? [leavingProfileId] : []),
    ...(targetProfileId ? [targetProfileId] : []),
  ];
  for (const profileId of order) {
    const token = await verifyOne(profileId, pin);
    if (token !== null) return token;
  }
  return null;
}

async function verifyOne(profileId: string, pin: string): Promise<string | null> {
  try {
    const resp = await parentalServiceVerifyPin(profileId, { pin });
    if (!resp.valid) return null;
    return resp.pinToken ?? "";
  } catch (err) {
    // A refusal the server spelled out: this profile says no, try the next.
    if ((err as { httpStatus?: number }).httpStatus !== undefined) return null;
    throw err;
  }
}
