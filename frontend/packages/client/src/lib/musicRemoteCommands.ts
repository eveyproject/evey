import { eveyFetchRaw } from "@evey/api";

// How long to wait before polling again when the server has no live session
// for us yet (the first heartbeat has not landed) or the request failed.
export const MUSIC_COMMAND_RETRY_MS = 5_000;

// An empty answer faster than this did not come from a held poll (an old
// server, a proxy answering for it): back off instead of spinning.
const MIN_HELD_POLL_MS = 1_000;

// pollMusicRemoteCommands holds a long poll open on the server so an admin's
// play/pause/stop reaches the music player at once instead of on the next
// ~10s heartbeat. Each answer is either a command or an empty "nothing yet",
// and the loop re-polls straight away; it ends when the signal aborts.
export async function pollMusicRemoteCommands(
  sessionId: string,
  signal: AbortSignal,
  onCommand: (command: string) => void,
): Promise<void> {
  const url = `/api/v1/music/nowplaying/${encodeURIComponent(sessionId)}/command`;
  while (!signal.aborted) {
    let retry = false;
    const started = Date.now();
    try {
      const res = await eveyFetchRaw({ url, method: "GET", signal, raw: true });
      if (!res.ok) {
        retry = true;
      } else {
        const data = (await res.json().catch(() => null)) as { remoteCommand?: string } | null;
        if (!signal.aborted && data?.remoteCommand) onCommand(data.remoteCommand);
        else retry = Date.now() - started < MIN_HELD_POLL_MS;
      }
    } catch {
      retry = true;
    }
    if (retry) await abortableDelay(MUSIC_COMMAND_RETRY_MS, signal);
  }
}

function abortableDelay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) return resolve();
    const done = () => {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    };
    const timer = setTimeout(done, ms);
    signal.addEventListener("abort", done);
  });
}
