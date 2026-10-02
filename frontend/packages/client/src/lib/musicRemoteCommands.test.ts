import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const fetchRaw = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetchRaw: (...args: unknown[]) => fetchRaw(...args) }));

import { MUSIC_COMMAND_RETRY_MS, pollMusicRemoteCommands } from "./musicRemoteCommands";

function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status });
}

describe("pollMusicRemoteCommands", () => {
  beforeEach(() => {
    fetchRaw.mockReset();
    vi.useFakeTimers();
  });
  afterEach(() => vi.useRealTimers());

  it("hands each command over and re-polls at once after a held empty answer", async () => {
    const ctrl = new AbortController();
    const seen: string[] = [];
    fetchRaw
      .mockImplementationOnce(async () => {
        vi.setSystemTime(Date.now() + 25_000); // the server held it for its window
        return answer(200, { remoteCommand: "" });
      })
      .mockResolvedValueOnce(answer(200, { remoteCommand: "pause" }))
      .mockImplementationOnce(() => {
        ctrl.abort();
        return Promise.resolve(answer(200, { remoteCommand: "play" }));
      });

    await pollMusicRemoteCommands("music-1", ctrl.signal, (c) => seen.push(c));

    expect(seen).toEqual(["pause"]);
    expect(fetchRaw).toHaveBeenCalledTimes(3);
    expect(fetchRaw.mock.calls[0][0]).toMatchObject({
      url: "/api/v1/music/nowplaying/music-1/command",
      method: "GET",
      raw: true,
    });
  });

  it("backs off on a 404 (no live session yet) instead of hammering the server", async () => {
    const ctrl = new AbortController();
    fetchRaw.mockResolvedValue(answer(404, {}));

    const loop = pollMusicRemoteCommands("music-1", ctrl.signal, () => {});
    await vi.advanceTimersByTimeAsync(MUSIC_COMMAND_RETRY_MS - 1);
    expect(fetchRaw).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchRaw).toHaveBeenCalledTimes(2);

    ctrl.abort();
    await loop;
  });

  it("backs off when an empty answer comes back instantly instead of spinning", async () => {
    const ctrl = new AbortController();
    fetchRaw.mockResolvedValue(new Response(null, { status: 204 }));

    const loop = pollMusicRemoteCommands("music-1", ctrl.signal, () => {});
    await vi.advanceTimersByTimeAsync(MUSIC_COMMAND_RETRY_MS - 1);
    expect(fetchRaw).toHaveBeenCalledTimes(1);

    ctrl.abort();
    await loop;
  });

  it("stops as soon as the signal aborts, even mid back-off", async () => {
    const ctrl = new AbortController();
    fetchRaw.mockRejectedValue(new Error("offline"));

    const loop = pollMusicRemoteCommands("music-1", ctrl.signal, () => {});
    await vi.advanceTimersByTimeAsync(0);
    ctrl.abort();
    await loop;
    expect(fetchRaw).toHaveBeenCalledTimes(1);
  });
});
