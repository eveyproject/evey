// Turning a remote-access refusal into something a human can act on.
//
// A gated server answers every route with 403 remote_access_disabled to any
// client outside the LAN. Left unhandled, that looked like "wrong password"
// or "server unreachable" - a beta tester stayed locked out for two days
// before anyone thought to check the toggle (2026-07-29). The server tags the
// refusal with a stable code (eveyFetch surfaces it as `errorCode`) and an
// `X-Evey-Remote-Access: disabled` header; this names it.
//
// Shared by web, mobile and TV: the fix only ever landed on mobile, so the
// other two clients kept blaming the password.

// isRemoteAccessDisabled recognizes the refusal from whatever eveyFetch (or a
// raw fetch, for the pre-auth server probe) threw or returned. errorCode is
// the primary signal; the message check is a fallback for the rare path
// where the error crosses a layer that drops custom properties.
export function isRemoteAccessDisabled(err: unknown): boolean {
  if (err && typeof err === "object") {
    const code = (err as { errorCode?: unknown; code?: unknown }).errorCode
      ?? (err as { code?: unknown }).code;
    if (code === "remote_access_disabled") return true;
  }
  const message = err instanceof Error ? err.message : typeof err === "string" ? err : "";
  return /remote access disabled/i.test(message);
}

// remoteAccessDisabledMessage renders the title/body pair, whether the client
// shows it in a native alert or inline under the form.
export function remoteAccessDisabledMessage(
  t: (key: string, fallback: string) => string,
): { title: string; body: string } {
  return {
    title: t("auth.remoteAccess.title", "Remote access disabled"),
    body: t(
      "auth.remoteAccess.body",
      "Remote access is disabled on this server. The administrator must enable it in Admin > Remote access to allow connections from outside the local network.",
    ),
  };
}
