import { describe, it, expect, afterEach } from "vitest";
import { EveyBridge } from "../src/adapters/evey/EveyBridge";

function setEvey(evey: unknown): void {
  (globalThis as unknown as { window: { evey?: unknown } }).window = { evey } as never;
}

describe("EveyBridge.readPoster", () => {
  afterEach(() => {
    delete (globalThis as unknown as { window?: unknown }).window;
  });

  it("returns the host-relayed data URL for a media id", async () => {
    let askedId = "";
    setEvey({ media: { poster: async (id: string) => { askedId = id; return "data:image/jpeg;base64,AAAA"; } } });
    const url = await new EveyBridge().readPoster("mi-42");
    expect(askedId).toBe("mi-42");
    expect(url).toBe("data:image/jpeg;base64,AAAA");
  });

  it("degrades to an empty string (placeholder kept) on any failure or missing bridge", async () => {
    setEvey({ media: { poster: async () => { throw new Error("no artwork"); } } });
    expect(await new EveyBridge().readPoster("mi-1")).toBe("");

    setEvey({ media: {} }); // host without the poster method (older host)
    expect(await new EveyBridge().readPoster("mi-1")).toBe("");

    setEvey({ media: { poster: async () => "data:x" } });
    expect(await new EveyBridge().readPoster("")).toBe(""); // no id
  });
});
