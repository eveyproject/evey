// How to print a server version next to a discovered server.
//
// The screen used to hardcode a "v" prefix, which reads fine for "1.2.3" but
// produces "vdev" on a development build and "vbeta-0.0.1" in prod - the "v"
// only belongs in front of a NUMBER (user report 2026-07-29).
export function versionLabel(version: string | null | undefined): string {
  const v = (version ?? "").trim();
  if (!v) return "";
  return /^\d/.test(v) ? `v${v}` : v;
}
