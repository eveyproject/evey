// A bare address the server accepts: one "@", no space, no display name or
// list separator, no dot at either end of a part or doubled. The server stays
// the judge (net/mail); this only names the field before the round trip.
const PLAIN_EMAIL = /^[^\s@<>()[\],;:"\\]+@[^\s@<>()[\],;:"\\]+$/;

export function isPlainEmail(value: string): boolean {
  if (!PLAIN_EMAIL.test(value)) return false;
  return value.split("@").every((part) => !part.startsWith(".") && !part.endsWith(".") && !part.includes(".."));
}
