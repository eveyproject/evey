import React from "react";
import { render, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { accessToken: string }) => string) =>
    selector({ accessToken: "test-token" }),
}));

const mockVTT = `WEBVTT

00:00:00.000 --> 00:00:10.000
/sprites/sprite1.jpg#xywh=0,0,160,90

00:00:10.000 --> 00:00:20.000
/sprites/sprite1.jpg#xywh=160,0,160,90

`;

describe("TrickplayPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // reset fetch mock
    vi.stubGlobal("fetch", vi.fn());
  });

  it("renders nothing when hoverTime is null", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => mockVTT,
    } as Response);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: null }),
    );

    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it("renders nothing when trickplay fetch fails", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 404,
      text: async () => "",
    } as Response);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: 5 }),
    );

    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it("renders preview div after successful VTT load with matching hoverTime", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => mockVTT,
    } as Response);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: 5 }),
    );

    await waitFor(() => {
      expect(container.firstChild).not.toBeNull();
    });

    const div = container.firstChild as HTMLElement;
    expect(div.style.width).toBe("160px");
    expect(div.style.height).toBe("90px");
  });

  it("renders nothing when hoverTime doesn't match any cue", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => mockVTT,
    } as Response);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: 999 }),
    );

    // Wait for fetch to complete
    await waitFor(() => {
      // Either null (no match) — cues loaded but hoverTime=999 is outside all cues
      expect(container.firstChild).toBeNull();
    });
  });

  it("uses correct background-position for the matched cue", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => mockVTT,
    } as Response);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      // hoverTime=15 matches the second cue: xywh=160,0,160,90
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: 15 }),
    );

    await waitFor(() => {
      expect(container.firstChild).not.toBeNull();
    });

    const inner = container.firstChild?.firstChild as HTMLElement;
    expect(inner.style.backgroundPosition).toBe("-160px 0px");
  });

  it("dispatches evey:unauthorized on 401 response", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 401,
      text: async () => "",
    } as Response);

    const dispatchSpy = vi.spyOn(window, "dispatchEvent");

    const { TrickplayPreview } = await import("./TrickplayPreview");
    render(
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: 5 }),
    );

    await waitFor(() => {
      expect(dispatchSpy).toHaveBeenCalledWith(
        expect.objectContaining({ type: "evey:unauthorized" }),
      );
    });
  });
});
