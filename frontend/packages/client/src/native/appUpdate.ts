// The Play Store auto-update is not something to count on: its jobs wait for
// an idle device, which a TV never is (27-09, a TV a week on 0.1.4) and a
// phone rarely is on Wi-Fi (10-10, a phone a month on beta.74 with a known
// silent-DTS bug). The server advertises on /healthz the build of each app
// released with it, under clients.tv and clients.mobile; each app compares
// its own versionCode and sends the viewer to its store page.

export type ClientApp = "tv" | "mobile";

export interface ClientRelease {
  version: string;
  versionCode: number;
}

// The newer release of `app` a /healthz body advertises, or null: an older
// server, an unstamped build or a malformed field all mean "nothing to offer".
// Each app reads its own key only - the two share one versionCode sequence
// under com.evey.app, so the TV's code says nothing about the phone's.
export function newerClientRelease(health: unknown, app: ClientApp, installedCode: number): ClientRelease | null {
  const clients = (health as { clients?: Partial<Record<ClientApp, { version?: unknown; version_code?: unknown }>> } | null)?.clients;
  const release = clients?.[app];
  const code = release?.version_code;
  if (typeof code !== "number" || !Number.isInteger(code) || code <= installedCode) return null;
  return { version: typeof release?.version === "string" ? release.version : String(code), versionCode: code };
}
