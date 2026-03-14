import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { DownloadButton } from "./DownloadButton";

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { accessToken: string | null }) => unknown) =>
    selector({ accessToken: "tok123" }),
}));

describe("DownloadButton", () => {
  it("renders download link with correct href", () => {
    render(<DownloadButton mediaId="m1" title="My Movie" />);
    const link = screen.getByTestId("download-btn");
    expect(link).toHaveAttribute("href", "/api/v1/media/m1/download?token=tok123");
    expect(link).toHaveAttribute("download", "My Movie");
  });

  it("renders 'Download' text", () => {
    render(<DownloadButton mediaId="m2" />);
    expect(screen.getByText("Download")).toBeInTheDocument();
  });
});

