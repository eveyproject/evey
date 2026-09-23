import { describe, expect, it } from "vitest";
import { decideHlsFatal } from "./hlsRecovery";

const none = { reload: 0, recoverMedia: 0 };

describe("a fatal hls error", () => {
  // Measured in prod on 2026-09-23: a browser without H.264 was handed an avc1
  // variant, hls.js raised this as a fatal media error, and the endless
  // recoverMediaError left the viewer on a frozen 0:00.
  it("fails at once when the set cannot decode the stream", () => {
    expect(decideHlsFatal({ type: "mediaError", details: "manifestIncompatibleCodecsError" }, none)).toBe("fail");
  });

  it("renegotiates an expired master", () => {
    expect(decideHlsFatal({ type: "networkError", details: "manifestLoadError", responseCode: 410 }, none)).toBe("renegotiate");
  });

  it("recovers a media error once, then gives up", () => {
    expect(decideHlsFatal({ type: "mediaError", details: "bufferStalledError" }, none)).toBe("recoverMedia");
    expect(decideHlsFatal({ type: "mediaError", details: "bufferStalledError" }, { reload: 0, recoverMedia: 1 })).toBe("fail");
  });

  it("reloads a network error three times, then gives up", () => {
    const err = { type: "networkError", details: "fragLoadError" };
    expect(decideHlsFatal(err, { reload: 2, recoverMedia: 0 })).toBe("reload");
    expect(decideHlsFatal(err, { reload: 3, recoverMedia: 0 })).toBe("fail");
  });

  it("fails on anything else", () => {
    expect(decideHlsFatal({ type: "otherError", details: "internalException" }, none)).toBe("fail");
  });
});
