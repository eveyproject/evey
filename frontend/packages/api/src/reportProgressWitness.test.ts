import { beforeEach, describe, expect, it, vi } from "vitest";

const send = vi.fn();
vi.mock("./generated/playback-service/playback-service", () => ({ playbackServiceReportProgress: (b: unknown) => send(b) }));

import { reportProgressWithWitness, resetPlayedRefusal } from "./reportProgressWitness";

// Measured against a 0.1.13 server: the gateway refused the whole tick with
// `proto: (line 1:135): unknown field "played"`, so the resume point was lost.
describe("reportProgressWithWitness", () => {
  beforeEach(() => {
    send.mockReset();
    resetPlayedRefusal();
  });

  it("sends the played report to a server that takes it", async () => {
    send.mockResolvedValue({ servedAudio: { track: 1 } });
    const res = await reportProgressWithWitness({ sessionId: "s", played: { source: "hls" } });
    expect(res).toEqual({ servedAudio: { track: 1 } });
    expect(send).toHaveBeenCalledTimes(1);
    expect(send.mock.calls[0][0].played).toEqual({ source: "hls" });
  });

  it("sends the tick again without it to an older server, and stops sending it", async () => {
    send.mockRejectedValueOnce(new Error('proto: (line 1:135): unknown field "played"')).mockResolvedValue({});
    await reportProgressWithWitness({ sessionId: "s", positionMs: "5000", played: { source: "hls" } });
    expect(send).toHaveBeenCalledTimes(2);
    expect(send.mock.calls[1][0]).toEqual({ sessionId: "s", positionMs: "5000", played: undefined });

    await reportProgressWithWitness({ sessionId: "s", positionMs: "8000", played: { source: "hls" } });
    expect(send).toHaveBeenCalledTimes(3);
    expect(send.mock.calls[2][0].played).toBeUndefined();
  });

  it("lets any other failure through", async () => {
    send.mockRejectedValue(new Error("503 Service Unavailable"));
    await expect(reportProgressWithWitness({ sessionId: "s", played: { source: "hls" } })).rejects.toThrow("503");
    expect(send).toHaveBeenCalledTimes(1);
  });
});
