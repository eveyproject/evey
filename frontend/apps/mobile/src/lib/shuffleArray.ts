// Fisher-Yates shuffle, mirroring the web app's shuffleTracks. Shared because
// both the tracks tab and the playlists tab build the same kind of ephemeral
// randomized queue - nothing is persisted server-side, so the two must not
// drift into two slightly different shuffles.
export function shuffleArray<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}
