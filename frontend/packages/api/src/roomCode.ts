// A syncplay room code is the ONLY secret standing between a stranger and an
// unauthenticated streaming token, so its shape is a security parameter, not a
// UI detail. It lives here because four surfaces read it (web join box, web
// login guest box, mobile, TV remote) and the server generates it: a client
// that disagrees with internal/domain/syncplay/model.go makes valid codes
// unenterable. roomCode.guard.test.ts pins the two together.

/** Crockford base32: no I, L, O or U, so nothing read off a screen is ambiguous. */
export const ROOM_CODE_ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";

export const ROOM_CODE_LENGTH = 6;

/**
 * Folds what someone actually types into the stored form: case, the separators
 * people add to a six-character code, and the three characters the alphabet
 * drops precisely because they are misread (I and L for 1, O for 0). Anything
 * else is discarded rather than mapped, so a wrong code stays wrong.
 */
export function normalizeRoomCode(input: string): string {
  let out = "";
  for (const ch of input.toUpperCase()) {
    if (ch === "I" || ch === "L") out += "1";
    else if (ch === "O") out += "0";
    else if (ROOM_CODE_ALPHABET.includes(ch)) out += ch;
  }
  return out;
}

/** True when the code is complete and could exist - not that it does. */
export function isValidRoomCode(code: string): boolean {
  const c = normalizeRoomCode(code);
  return c.length === ROOM_CODE_LENGTH;
}
