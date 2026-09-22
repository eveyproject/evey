// A playlist's type as the server actually sends it. The TV's own type
// declared "manual" | "smart" while the API answers "PLAYLIST_TYPE_MANUAL",
// and the screen printed the value raw - the enum was on the television, in
// every language (2026-09-19). Both spellings are accepted here so neither
// side has to be right.
export function isSmartPlaylist(type?: string): boolean {
  return type === "smart" || type === "PLAYLIST_TYPE_SMART";
}
