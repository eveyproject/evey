// serverBrand.ts - the instance name under the idle splash.
//
// The receiver is generic (hosted apart from any instance), so before the
// first LOAD it cannot know which server it serves. From then on the playing
// URL names the origin, and the public setup-status endpoint names the
// instance: the idle screen reads "Salon" under the logo instead of nothing.
// Pure helpers; main.ts owns the fetch and the DOM.

// buildSetupStatusUrl derives the public setup-status endpoint from the
// playing master's URL. null for anything that is not an Evey media URL.
export function buildSetupStatusUrl(streamUrl: string): string | null {
  try {
    const u = new URL(streamUrl);
    if (!/\/api\/v1\/media\//.test(u.pathname)) return null;
    return `${u.origin}/api/v1/setup/status`;
  } catch {
    return null;
  }
}

// parseServerName returns the name worth showing, "" for the default "Evey"
// (the logo already says so) or a payload that is not ours.
export function parseServerName(data: unknown): string {
  if (typeof data !== "object" || data === null) return "";
  const raw = (data as { serverName?: unknown }).serverName;
  const name = typeof raw === "string" ? raw.trim() : "";
  return name.toLowerCase() === "evey" ? "" : name;
}
