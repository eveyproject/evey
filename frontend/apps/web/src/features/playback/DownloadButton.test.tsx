import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { DownloadButton } from "./DownloadButton";

describe("DownloadButton", () => {
  it("renders download link with token", () => {
    render(<DownloadButton mediaId="m1" title="My Movie" token="st-123" />);
    const link = screen.getByTestId("download-btn");
    expect(link).toHaveAttribute("href", "/api/v1/media/m1/download?token=st-123");
    expect(link).toHaveAttribute("download", "My Movie");
  });

  it("returns null when token is missing", () => {
    const { container } = render(<DownloadButton mediaId="m2" />);
    expect(container.innerHTML).toBe("");
  });

  it("returns null when token is empty", () => {
    const { container } = render(<DownloadButton mediaId="m2" token="" />);
    expect(container.innerHTML).toBe("");
  });

  it("renders a label when token provided", () => {
    render(<DownloadButton mediaId="m2" token="tok" />);
    expect(screen.getByTestId("download-btn").textContent).toBeTruthy();
  });

  it("uses the label override when provided", () => {
    render(<DownloadButton mediaId="m2" token="tok" label="Download season" />);
    expect(screen.getByTestId("download-btn")).toHaveTextContent("Download season");
  });

  it("always includes token in URL — never renders without auth", () => {
    render(<DownloadButton mediaId="m1" token="abc" />);
    const href = screen.getByTestId("download-btn").getAttribute("href");
    expect(href).toContain("?token=abc");
    expect(href).not.toBe("/api/v1/media/m1/download");
  });
});
