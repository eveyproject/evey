import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const fetchRaw = vi.fn();
const applyMutate = vi.fn();
let applyState: Record<string, unknown> = { mutate: applyMutate, isPending: false };
const getMediaItem = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetchRaw: (...args: unknown[]) => fetchRaw(...args),
  useAdminMediaServiceApplyAudioShift: () => applyState,
  useLibraryServiceGetMediaItem: (...args: unknown[]) => getMediaItem(...args),
}));

import { SyncPreviewPanel, suggestedShifts } from "./SyncPreviewPanel";

const ITEM = {
  mediaId: "m-1",
  title: "A film",
  path: "/media/films/a.mkv",
  audioShortfallSec: 19,
  audioTracksLagSec: 0,
  audioDelaySec: 0,
};

const MEDIA = {
  durationMs: "7200000",
  streams: [
    { type: "MEDIA_STREAM_TYPE_VIDEO", frameRate: 24 },
    { type: "MEDIA_STREAM_TYPE_AUDIO", language: "fra", codec: "ac3", channels: 6 },
    { type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng", codec: "dts", channels: 6 },
  ],
};

function blobResponse() {
  return { blob: async () => new Blob(["clip"], { type: "video/mp4" }) };
}

function renderPanel(item: Partial<typeof ITEM> = {}) {
  return render(
    React.createElement(SyncPreviewPanel, {
      item: { ...ITEM, ...item },
      title: "A film",
      onClose: () => {},
    }),
  );
}

describe("SyncPreviewPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    applyState = { mutate: applyMutate, isPending: false };
    getMediaItem.mockReturnValue({ data: MEDIA });
    globalThis.URL.createObjectURL = vi.fn(() => "blob:clip");
    globalThis.URL.revokeObjectURL = vi.fn();
  });

  // The bug this panel was rebuilt around: the clip used to be handed to
  // <video src="/api/v1/admin/..."> as a raw URL. That route is behind the
  // admin bearer and a <video> attaches no Authorization header, so every
  // render came back 401 and the browser reported "no video with a supported
  // format or MIME type". The clip must travel through the app's own fetcher.
  it("fetches the clip through the authenticated fetcher, not as a raw <video src>", async () => {
    fetchRaw.mockResolvedValue(blobResponse());
    renderPanel();
    fireEvent.click(screen.getByTestId("sync-preview-render"));
    await waitFor(() => expect(screen.getByTestId("sync-preview-video")).toBeInTheDocument());
    expect(fetchRaw).toHaveBeenCalledTimes(1);
    const req = fetchRaw.mock.calls[0][0] as { url: string; method: string };
    expect(req.method).toBe("GET");
    expect(req.url).toContain("/api/v1/admin/media/m-1/sync-preview");
    expect(screen.getByTestId("sync-preview-video")).toHaveAttribute("src", "blob:clip");
  });

  it("shows the server's own refusal instead of a silent broken player", async () => {
    fetchRaw.mockRejectedValue(Object.assign(new Error("sync preview: nothing to show at 600s"), { httpStatus: 422 }));
    renderPanel();
    fireEvent.click(screen.getByTestId("sync-preview-render"));
    await waitFor(() => expect(screen.getByTestId("sync-preview-error")).toBeInTheDocument());
    expect(screen.getByTestId("sync-preview-error")).toHaveTextContent("nothing to show at 600s");
    expect(screen.queryByTestId("sync-preview-video")).not.toBeInTheDocument();
  });

  // The fixed ladder stopped at 12s on a page that reports twenty: the offset
  // the operator was told about was not offerable.
  it("offers the offsets this file was measured at, on both signs", () => {
    expect(suggestedShifts({ audioShortfallSec: 19, audioTracksLagSec: 0, audioDelaySec: 0 })).toEqual([-19, 19]);
    expect(suggestedShifts({ audioShortfallSec: 0, audioTracksLagSec: -2.5, audioDelaySec: 0.08 })).toEqual([
      -2.5, -0.08, 0.08, 2.5,
    ]);
    // Past what the server accepts, and below what an ear resolves: neither is
    // a candidate.
    expect(suggestedShifts({ audioShortfallSec: 900, audioTracksLagSec: 0.01, audioDelaySec: 0 })).toEqual([]);
  });

  it("puts a measured offset one click away and sends it with the render", async () => {
    fetchRaw.mockResolvedValue(blobResponse());
    renderPanel();
    fireEvent.click(screen.getByTestId("sync-preview-suggest--19"));
    expect(screen.getByTestId("sync-preview-shift")).toHaveValue(-19);
    fireEvent.click(screen.getByTestId("sync-preview-render"));
    await waitFor(() => expect(fetchRaw).toHaveBeenCalled());
    expect((fetchRaw.mock.calls[0][0] as { url: string }).url).toContain("shift=-19");
  });

  it("scrubs over the whole film once its duration is known", () => {
    renderPanel();
    const at = screen.getByTestId("sync-preview-at");
    expect(at).toHaveAttribute("type", "range");
    expect(at).toHaveAttribute("max", "7200");
    fireEvent.change(at, { target: { value: "5400" } });
    expect(screen.getByTestId("sync-preview-at-label")).toHaveTextContent("1:30:00 / 2:00:00");
  });

  it("says on screen when the controls have moved past the clip being shown", async () => {
    fetchRaw.mockResolvedValue(blobResponse());
    renderPanel();
    fireEvent.click(screen.getByTestId("sync-preview-render"));
    await waitFor(() => expect(screen.getByTestId("sync-preview-video")).toBeInTheDocument());
    expect(screen.queryByTestId("sync-preview-stale")).not.toBeInTheDocument();
    fireEvent.change(screen.getByTestId("sync-preview-shift"), { target: { value: "-3" } });
    expect(screen.getByTestId("sync-preview-stale")).toBeInTheDocument();
  });

  it("applies the offset the ear settled on to the chosen track", () => {
    renderPanel();
    fireEvent.change(screen.getByTestId("sync-preview-shift"), { target: { value: "-2.4" } });
    fireEvent.change(screen.getByTestId("sync-preview-track"), { target: { value: "1" } });
    fireEvent.click(screen.getByTestId("sync-preview-apply"));
    expect(applyMutate).toHaveBeenCalledWith({ mediaId: "m-1", data: { track: "2", shiftSec: -2.4 } });
  });

  it("falls back to a number field and numbered tracks when the file is not described", () => {
    getMediaItem.mockReturnValue({ data: undefined });
    renderPanel();
    expect(screen.getByTestId("sync-preview-at")).toHaveAttribute("type", "number");
    expect(screen.getByTestId("sync-preview-track")).toBeInTheDocument();
  });
});

// A refusal must be readable. On a file whose timebase is not the 1 ms default,
// only shifts that are a whole number of its own ticks are representable, so
// most values an operator picks come back refused - and before this the button
// simply stopped spinning with nothing on screen, the same silent failure the
// render path was rebuilt to remove.
describe("SyncPreviewPanel apply outcome", () => {
  it("shows the server's refusal instead of failing silently", () => {
    applyState = {
      mutate: applyMutate,
      isPending: false,
      isError: true,
      error: { message: "retime: shift -2400000000ns is not a whole number of timecode ticks (20832ns per tick)" },
    };
    renderPanel();
    expect(screen.getByTestId("sync-preview-apply-error")).toHaveTextContent(/not a whole number of timecode ticks/);
  });

  it("confirms when the shift went through", () => {
    applyState = { mutate: applyMutate, isPending: false, isSuccess: true };
    renderPanel();
    expect(screen.getByTestId("sync-preview-apply-done")).toBeInTheDocument();
  });
});
