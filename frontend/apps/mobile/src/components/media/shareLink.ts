// Mirrors the web's ShareLinkButton: the share-link RPC returns a
// server-relative path (e.g. "/dl/abc123"), which only resolves against the
// server the app is currently pointed at - not the app's own origin (there
// is none, on native) - so it must be joined with serverUrl before it's
// usable outside the app (pasted in a browser, texted to someone).
export function buildPublicShareUrl(serverUrl: string, urlPath: string | undefined): string {
  if (!urlPath) return "";
  return `${serverUrl}${urlPath}`;
}
