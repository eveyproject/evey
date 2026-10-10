import { describe, it, expect, vi } from "vitest";
import { fetchSubtitleTrack, retryAfterSeconds } from "./subtitleFetch";

function res(status: number, body: BodyInit | null = null, headers: Record<string, string> = {}): Response {
  return new Response(body, { status, headers });
}

const noSleep = async () => {};

describe("retryAfterSeconds", () => {
  it("reads the header, then the body, bounded to 2..30 s", () => {
    expect(retryAfterSeconds(res(202, null, { "Retry-After": "7" }))).toBe(7);
    expect(retryAfterSeconds(res(202), { retryAfterSec: 12 })).toBe(12);
    expect(retryAfterSeconds(res(202, null, { "Retry-After": "600" }))).toBe(30);
    expect(retryAfterSeconds(res(202, null, { "Retry-After": "0" }))).toBe(2);
    expect(retryAfterSeconds(res(202))).toBe(2);
  });
});

describe("fetchSubtitleTrack", () => {
  it("returns the bytes of a 200 at once", async () => {
    const r = await fetchSubtitleTrack({ doFetch: async () => res(200, new Uint8Array([1, 2, 3])), sleep: noSleep });
    expect(r.state).toBe("ready");
    expect(Array.from(r.bytes ?? [])).toEqual([1, 2, 3]);
  });

  it("follows 202s with the server's delay until the track is ready", async () => {
    const answers = [
      res(202, JSON.stringify({ state: "preparing", retryAfterSec: 5 }), { "Retry-After": "5" }),
      res(202, JSON.stringify({ state: "preparing", retryAfterSec: 3 }), { "Retry-After": "3" }),
      res(200, new Uint8Array([9])),
    ];
    const slept: number[] = [];
    const preparing: number[] = [];
    const r = await fetchSubtitleTrack({
      doFetch: async () => answers.shift() ?? res(500),
      onPreparing: (s) => preparing.push(s),
      sleep: async (ms) => { slept.push(ms); },
    });
    expect(r.state).toBe("ready");
    expect(preparing).toEqual([5, 3]);
    expect(slept).toEqual([5000, 3000]);
  });

  it("reports a 422 (or any other status) as failed, with the status", async () => {
    const r = await fetchSubtitleTrack({ doFetch: async () => res(422, "{}"), sleep: noSleep });
    expect(r).toEqual({ state: "failed", bytes: null, status: 422 });
  });

  it("never throws on a network error", async () => {
    const r = await fetchSubtitleTrack({ doFetch: async () => { throw new TypeError("offline"); }, sleep: noSleep });
    expect(r).toEqual({ state: "failed", bytes: null, status: 0 });
  });

  it("stops polling when aborted and says so", async () => {
    const ac = new AbortController();
    const doFetch = vi.fn(async () => {
      ac.abort();
      return res(202, null, { "Retry-After": "2" });
    });
    const r = await fetchSubtitleTrack({ doFetch, signal: ac.signal, sleep: noSleep });
    expect(r.state).toBe("cancelled");
    expect(doFetch).toHaveBeenCalledTimes(1);
  });

  it("gives up after the attempt cap on a server that never finishes", async () => {
    const doFetch = vi.fn(async () => res(202, null, { "Retry-After": "2" }));
    const r = await fetchSubtitleTrack({ doFetch, sleep: noSleep, maxAttempts: 4 });
    expect(r.state).toBe("failed");
    expect(doFetch).toHaveBeenCalledTimes(4);
  });
});
