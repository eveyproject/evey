// The profile cap typed in an invite form: empty or 0 means no cap (the
// server's "unset"), a whole number above 0 is the cap. Anything else is
// treated as no cap rather than sending a value the server would refuse.
export function parseInviteProfileCap(raw: string): number | undefined {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) return undefined;
  const n = Number.parseInt(trimmed, 10);
  return n > 0 ? n : undefined;
}
