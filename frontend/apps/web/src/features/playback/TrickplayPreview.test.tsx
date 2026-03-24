import React from "react";
import { render, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockEveyFetchRaw = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetchRaw: (...args: unknown[]) => mockEveyFetchRaw(...args),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (selector: (s: { accessToken: string }) => string) =>
      selector({ accessToken: "test-token" }),
    { getState: () => ({ accessToken: "test-token" }) },
  ),
}));

const mockVTT = `WEBVTT

00:00:00.000 --> 00:00:10.000
/sprites/sprite1.jpg#xywh=0,0,160,90

00:00:10.000 --> 00:00:20.000
/sprites/sprite1.jpg#xywh=160,0,160,90

`;

function mockFetchRawOk(text: string) {
  mockEveyFetchRaw.mockResolvedValue({ text: async () => text } as Response);
}

function mockFetchRawError() {
  mockEveyFetchRaw.mockRejectedValue(new Error("fetch error"));
}

describe("TrickplayPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when hoverTime is null", async () => {
    mockFetchRawOk(mockVTT);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m1", hoverTime: null }),
    );

    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it("renders nothing when trickplay fetch fails", async () => {
    mockFetchRawError();

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m-fail", hoverTime: 5 }),
    );

    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it("renders preview div after successful VTT load with matching hoverTime", async () => {
    mockFetchRawOk(mockVTT);

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
    mockFetchRawOk(mockVTT);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m-nomatch", hoverTime: 999 }),
    );

    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it("uses correct background-position for the matched cue", async () => {
    mockFetchRawOk(mockVTT);

    const { TrickplayPreview } = await import("./TrickplayPreview");
    const { container } = render(
      React.createElement(TrickplayPreview, { mediaId: "m-bgpos", hoverTime: 15 }),
    );

    await waitFor(() => {
      expect(container.firstChild).not.toBeNull();
    });

    const inner = container.firstChild?.firstChild as HTMLElement;
    expect(inner.style.backgroundPosition).toBe("-160px 0px");
  });

  it("rejects on 401 via eveyFetchRaw (handled by fetcher)", async () => {
    mockFetchRawError();

    const { TrickplayPreview } = await import("./TrickplayPreview");
    render(
      React.createElement(TrickplayPreview, { mediaId: "m-401", hoverTime: 5 }),
    );

    await waitFor(() => {
      expect(mockEveyFetchRaw).toHaveBeenCalled();
    });
  });
});
