import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

const fetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (opts: { url: string; method: string }) => fetchMock(opts),
  formatBytes: (b: number | string) => `${b} B`,
}));

import { qualityLine, restoreTrashed, useTrashConfirm, type TrashResult } from "./useTrashConfirm";

let api: ReturnType<typeof useTrashConfirm>;
function Host() {
  api = useTrashConfirm();
  return api.dialog;
}

// The server's record, NOT what the plugin sent: the dialog shows this path.
const record = {
  title: "Dune",
  year: 2021,
  filePath: "/media/Dune (2021)/Dune.mkv",
  fileSize: "8000",
  bitrate: "25000000",
  streams: [
    { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", height: 2160, colorTransfer: "smpte2084" },
    { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", channels: 6, isDefault: true },
  ],
};

describe("useTrashConfirm", () => {
  beforeEach(() => fetchMock.mockReset());

  it("reads the quality from the server's streams", () => {
    expect(qualityLine({ id: "m1", ...record })).toBe("2160p HEVC HDR · 25.0 Mb/s · EAC3 5.1");
  });

  it("shows the server's record and moves nothing on cancel", async () => {
    fetchMock.mockResolvedValueOnce(record);
    render(<Host />);
    let outcome: Promise<TrashResult> = Promise.resolve({ trashed: [], failed: [] });
    await act(async () => { outcome = api.request(["m1"]); });
    expect(await screen.findByText("/media/Dune (2021)/Dune.mkv")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/media/m1", method: "GET" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await expect(outcome).rejects.toThrow("cancelled");
    expect(fetchMock).toHaveBeenCalledTimes(1); // no trash call
  });

  it("on yes, trashes each item through the admin endpoint and reports each outcome", async () => {
    fetchMock
      .mockResolvedValueOnce(record)
      .mockResolvedValueOnce({ ...record, filePath: "/media/b.mkv" })
      .mockResolvedValueOnce({ trashedPath: "/media/.evey-trash/Dune (2021)/Dune.mkv", pending: true })
      .mockRejectedValueOnce(new Error("read-only file system"));
    render(<Host />);
    let outcome: Promise<TrashResult> = Promise.resolve({ trashed: [], failed: [] });
    await act(async () => { outcome = api.request(["m1", "m2"]); });
    fireEvent.click(await screen.findByTestId("trash-confirm-btn"));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/media/m2/trash", method: "POST" }));
    expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/media/m1/trash", method: "POST" });
    await expect(outcome).resolves.toEqual({
      trashed: [{ id: "m1", trashedPath: "/media/.evey-trash/Dune (2021)/Dune.mkv", pending: true }],
      failed: [{ id: "m2", message: "read-only file system" }],
    });
  });

  it("restores each trashed path through the admin endpoint and reports each outcome", async () => {
    fetchMock
      .mockResolvedValueOnce({ restoredPath: "/media/Dune.mkv" })
      .mockRejectedValueOnce(new Error("a file already exists where it was"));
    const res = await restoreTrashed(["/media/.evey-trash/Dune.mkv", "/media/.evey-trash/b.mkv"]);
    expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/media/trash/restore", method: "POST", data: { trashedPath: "/media/.evey-trash/Dune.mkv" } });
    expect(res).toEqual({
      restored: [{ trashedPath: "/media/.evey-trash/Dune.mkv", restoredPath: "/media/Dune.mkv" }],
      failed: [{ trashedPath: "/media/.evey-trash/b.mkv", message: "a file already exists where it was" }],
    });
  });
});
