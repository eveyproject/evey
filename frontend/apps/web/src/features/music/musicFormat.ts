// shuffleTracks returns a new array in random order (Fisher-Yates). Used by the
// ephemeral "shuffle play" - builds a randomized queue without persisting it.
export function shuffleTracks<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}
