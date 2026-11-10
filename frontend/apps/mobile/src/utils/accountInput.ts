// A bare address the server accepts: one "@", no space, no display name or
// list separator, no dot at either end of a part or doubled. The server stays
// the judge (net/mail); this only names the field before the round trip.
const PLAIN_EMAIL = /^[^\s@<>()[\],;:"\\]+@[^\s@<>()[\],;:"\\]+$/;

export function isPlainEmail(value: string): boolean {
  if (!PLAIN_EMAIL.test(value)) return false;
  return value.split("@").every((part) => !part.startsWith(".") && !part.endsWith(".") && !part.includes(".."));
}

// bcrypt's input limit, in UTF-8 bytes, as the server counts it after trimming.
export const PASSWORD_MAX_BYTES = 72;

export function passwordTooLong(value: string): boolean {
  let bytes = 0;
  for (const ch of value.trim()) {
    const cp = ch.codePointAt(0) ?? 0;
    bytes += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
  }
  return bytes > PASSWORD_MAX_BYTES;
}
