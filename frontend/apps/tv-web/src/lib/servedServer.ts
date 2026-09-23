/**
 * The server a served page belongs to. Opened from an Evey server under /tv,
 * this page IS that server's client, and asking the viewer to type its address
 * with a remote was friction for nothing. The origin is only trusted once it
 * answers as Evey (the shared health parser): a development host serves the
 * page from Vite, not from the API, and keeps the server screen.
 */
export async function servingServer(
  loc: { protocol: string; origin: string },
  probe: (url: string, timeoutMs?: number) => Promise<{ probe: string }>,
): Promise<string | null> {
  // The packaged application runs from file:, with no server behind it.
  if (loc.protocol !== "http:" && loc.protocol !== "https:") return null;
  try {
    const { probe: result } = await probe(loc.origin, 3000);
    return result === "ok" ? loc.origin : null;
  } catch {
    return null;
  }
}
