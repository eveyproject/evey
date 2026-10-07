import { describe, it, expect } from "vitest";
import { sessionTrackLines } from "./sessionTrackLines";
import type { V1ActiveSession } from "./generated/admin_session.schemas";

describe("sessionTrackLines", () => {
  it("reads the engine and backend the server recorded, never a guess from the decision", () => {
    const session = {
      videoDecision: "copy",
      videoEngine: "ffmpeg",
      videoBackend: "nvenc",
      videoCodec: "h264",
      resolution: "1080p",
      audioDecision: "transcode",
      audioEngine: "mkvgo",
      audioCodec: "aac",
    } as V1ActiveSession;
    const [video, audio] = sessionTrackLines(session);
    expect(video).toMatchObject({ id: "video", decision: "copy", engine: "ffmpeg", backend: "", codec: "h264", res: "1080p" });
    expect(audio).toMatchObject({ id: "audio", decision: "transcode", engine: "mkvgo", backend: "" });
  });

  it("falls back to the old guess only when the server recorded nothing", () => {
    const session = { videoDecision: "transcode", videoBackend: "qsv", audioDecision: "wasm" } as V1ActiveSession;
    const [video, audio] = sessionTrackLines(session);
    expect(video).toMatchObject({ engine: "ffmpeg", backend: "qsv" });
    expect(audio).toMatchObject({ engine: "client" });
    expect(sessionTrackLines({ videoDecision: "copy" } as V1ActiveSession)[0].engine).toBe("mkvgo");
  });

  it("is empty for a session with no decision recorded", () => {
    expect(sessionTrackLines({ videoDecision: " " } as V1ActiveSession)).toEqual([]);
  });
});
