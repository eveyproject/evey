import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { DownloadButton } from "./DownloadButton";

describe("DownloadButton", () => {
  it("renders download link with stream token", () => {
    render(<DownloadButton mediaId="m1" title="My Movie" streamToken="st-123" />);
    const link = screen.getByTestId("download-btn");
    expect(link).toHaveAttribute("href", "/api/v1/media/m1/download?token=st-123");
    expect(link).toHaveAttribute("download", "My Movie");
  });

  it("renders download link without token when streamToken missing", () => {
    render(<DownloadButton mediaId="m2" />);
    const link = screen.getByTestId("download-btn");
    expect(link).toHaveAttribute("href", "/api/v1/media/m2/download");
  });

  it("renders 'Download' text", () => {
    render(<DownloadButton mediaId="m2" />);
    expect(screen.getByText("Download")).toBeInTheDocument();
  });
});
