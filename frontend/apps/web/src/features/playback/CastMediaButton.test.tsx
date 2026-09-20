import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const casting = vi.hoisted(() => ({
  castSdkReady: true,
  connected: false,
  deviceName: null as string | null,
  startCast: vi.fn().mockResolvedValue("connected"),
  stopCast: vi.fn(),
}));
const streamInfo = vi.hoisted(() => ({ get: vi.fn().mockResolvedValue({ playMethod: "TRANSCODE" }) }));
const token = vi.hoisted(() => ({ fetch: vi.fn().mockResolvedValue("cast-token") }));

vi.mock("@/hooks/useCasting", () => ({ useCasting: () => casting }));
vi.mock("@/hooks/useStreamToken", () => ({ useStreamToken: () => ({ fetchStreamToken: token.fetch }) }));
vi.mock("@evey/api", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  playbackServiceGetStreamInfo: (...a: unknown[]) => streamInfo.get(...a),
}));

import { CastMediaButton } from "./CastMediaButton";

describe("CastMediaButton", () => {
  beforeEach(() => {
    casting.castSdkReady = true;
    casting.connected = false;
    casting.startCast.mockClear();
    casting.stopCast.mockClear();
    token.fetch.mockClear();
  });

  it("stays out of the way where the browser has no Cast path", () => {
    casting.castSdkReady = false;
    render(<CastMediaButton mediaId="m1" title="Film" />);
    expect(screen.queryByTestId("cast-media-button")).not.toBeInTheDocument();
  });

  it("negotiates the stream for the RECEIVER, with a cast token and the page's resume point", async () => {
    render(<CastMediaButton mediaId="m1" title="Film" resumeMs={90_000} audioTrack={2} />);
    fireEvent.click(screen.getByTestId("cast-media-button"));
    await waitFor(() => expect(casting.startCast).toHaveBeenCalled());

    const [resolver, title] = casting.startCast.mock.calls[0];
    expect(title).toBe("Film");
    const plan = await resolver({ kind: "sdk", caps: null, userAgent: null });
    expect(token.fetch).toHaveBeenCalledWith("m1", { cast: true });
    expect(plan.url).toContain("token=cast-token");
    expect(plan.url).toContain("audioTrack=2");
    expect(plan.castResumeSec).toBe(90);
  });

  it("asks the parental PIN before anything leaves for a television", () => {
    const gate = vi.fn().mockReturnValue(true);
    render(<CastMediaButton mediaId="m1" gate={gate} />);
    fireEvent.click(screen.getByTestId("cast-media-button"));
    expect(gate).toHaveBeenCalled();
    expect(casting.startCast).not.toHaveBeenCalled();
  });

  it("stops the session instead of opening a second one when already casting", () => {
    casting.connected = true;
    render(<CastMediaButton mediaId="m1" />);
    fireEvent.click(screen.getByTestId("cast-media-button"));
    expect(casting.stopCast).toHaveBeenCalled();
    expect(casting.startCast).not.toHaveBeenCalled();
  });
});
