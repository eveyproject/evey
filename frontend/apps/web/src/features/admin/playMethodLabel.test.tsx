import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useSessionPlayMethodLabel } from "./playMethodLabel";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// One label per served path, the same rule as the server's, and an empty
// string before any byte was served so the caller drops the badge.
describe("useSessionPlayMethodLabel", () => {
  it("labels what the bytes were", () => {
    const { result } = renderHook(() => useSessionPlayMethodLabel());
    const label = result.current;
    expect(label({ servedVideo: { engine: "file", copy: true, track: -1 } })).toBe("admin.sessions.directPlay");
    expect(label({ servedVideo: { engine: "mkvgo", copy: true }, servedAudio: { engine: "mkvgo", copy: true } })).toBe("admin.sessions.directStream");
    // A copied AC-3 the browser decodes in wasm is not a server transcode.
    expect(label({ servedVideo: { engine: "mkvgo", copy: true }, servedAudio: { engine: "ffmpeg", copy: true, codec: "ac3" }, played: { source: "hls", audioDecoder: "wasm" } })).toBe("admin.sessions.directStream");
    expect(label({ servedVideo: { engine: "mkvgo", copy: true }, servedAudio: { engine: "ffmpeg", codec: "aac", sourceCodec: "mp3" } })).toBe("admin.sessions.transcode");
    expect(label({})).toBe("");
  });
});
