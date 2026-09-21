// Is the thing that answered actually an EVEY server?
//
// Accepting a bare HTTP 200 is not enough: a captive portal, an ISP's
// DNS-hijack page, a router's admin page or any unrelated host answers 200 to
// everything. The app then "connected" happily, the i18n fetch returned HTML
// (so every screen rendered raw keys like "auth.login.signIn") and the login
// POST vanished into a stranger - leaving NOTHING in the real server's logs.
// That is exactly what a beta tester hit on 2026-07-29 while the same APK
// worked for everyone whose DNS resolved honestly.
//
// Evey's /healthz answers a JSON body with a status and a version; requiring
// those two turns "something replied" into "Evey replied".

export interface EveyHealth {
  status: string;
  version: string;
  // The admin-chosen server name ("Server name" in the admin settings),
  // absent on servers that predate it.
  name?: string;
}

// parseEveyHealth returns the health payload when body is Evey's, else null.
// Tolerant about EXTRA fields (db, meilisearch, uptime_seconds - and whatever
// a future version adds), strict about the two that identify the product.
export function parseEveyHealth(body: unknown): EveyHealth | null {
  if (typeof body !== "object" || body === null) return null;
  const b = body as Record<string, unknown>;
  const status = typeof b.status === "string" ? b.status : "";
  const version = typeof b.version === "string" ? b.version : "";
  if (!status || !version) return null;
  const name = typeof b.name === "string" && b.name.trim() !== "" ? b.name.trim() : undefined;
  return name ? { status, version, name } : { status, version };
}
