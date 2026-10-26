// Deep screens (media/person/library detail, collections, playlists, music
// artist/album, settings sub-pages, history, notifications, manage-servers)
// render full-screen with NO sidebar - unlike the tab screens, whose sidebar
// already carries its own "Home" nav item, and unlike the player, which owns
// its own exit contract (see hooks/playerExit.ts) where a Home shortcut
// would be a trap mid-playback.
//
// This is the only place that decides "is the current screen deep enough to
// need an explicit way back to Home" - kept pure so it stays testable
// without mounting the layout or a router.

// Pathnames expo-router reports for the tab screens (group segments like
// "(auth)"/"(tabs)" are stripped at runtime) - each already exposes Home via
// the sidebar, so none of them need the shortcut.
const TAB_ROOT_PATHS = new Set([
  "/",
  "",
  "/favorites",
  "/library",
  "/music",
  "/requests",
  "/search",
  "/settings",
]);

export function isDeepScreenPath(pathname: string): boolean {
  if (!pathname) return false;
  if (pathname === "/player" || pathname.startsWith("/player/")) return false;
  return !TAB_ROOT_PATHS.has(pathname);
}

// A library's grid (/library/<id>): the one deep screen that lays itself out
// under the shortcut's band instead of being inset above it, so the row
// under the focused one stays in view on a television.
export function isLibraryGridPath(pathname: string): boolean {
  return /^\/library\/[^/]+\/?$/.test(pathname);
}

// Matches the href the sidebar itself uses for its Home nav item.
export const HOME_PATH = "/(auth)/(tabs)";
