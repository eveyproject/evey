import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { DownloadTrackButton } from "./DownloadTrackButton";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const startTokenDownload = vi.hoisted(() => vi.fn());
vi.mock("@/utils/downloadToken", () => ({ startTokenDownload: (...a: unknown[]) => startTokenDownload(...a) }));

beforeEach(() => {
  startTokenDownload.mockReset().mockResolvedValue(undefined);
  toast.error.mockClear();
});

// A track download mints a click-time token (the account JWT never rides
// in the URL) for the track's own route, and a refusal is a toast.
describe("DownloadTrackButton", () => {
  it("mints a click-time download for the track and toasts a refusal", async () => {
    render(<DownloadTrackButton trackID="t/1" />);
    const btn = screen.getByRole("button", { name: "media.music.downloadTrack" });
    fireEvent.click(btn);
    await waitFor(() => expect(startTokenDownload).toHaveBeenCalledWith("/api/v1/music/tracks/t%2F1/download", "t/1"));
    expect(toast.error).not.toHaveBeenCalled();
    startTokenDownload.mockRejectedValueOnce(new Error("403"));
    fireEvent.click(btn);
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("common.error"));
  });
});
