import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
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

  it("always includes token in URL - never renders without auth", () => {
    render(<DownloadButton mediaId="m1" token="abc" />);
    const href = screen.getByTestId("download-btn").getAttribute("href");
    expect(href).toContain("?token=abc");
    expect(href).not.toBe("/api/v1/media/m1/download");
  });

  it("mintToken mode keeps any token out of the rendered href", () => {
    // The account JWT must never sit in the DOM/URL: the anchor is inert
    // until click, when a media-scoped stream token is minted.
    render(<DownloadButton mediaId="s1" mintToken={async () => "scoped-tok"} label="Download season" />);
    const link = screen.getByTestId("download-btn");
    expect(link.getAttribute("href")).toBe("#");
  });

  it("mintToken mode mints on click and triggers a scoped-token download", async () => {
    const mint = vi.fn(async () => "scoped-tok");
    const clicked: string[] = [];
    const origClick = HTMLAnchorElement.prototype.click;
    HTMLAnchorElement.prototype.click = function () {
      clicked.push(this.getAttribute("href") ?? "");
    };
    try {
      render(<DownloadButton mediaId="s1" title="s.zip" mintToken={mint} />);
      fireEvent.click(screen.getByTestId("download-btn"));
      await waitFor(() => expect(mint).toHaveBeenCalledTimes(1));
      await waitFor(() => expect(clicked).toContain("/api/v1/media/s1/download?token=scoped-tok"));
    } finally {
      HTMLAnchorElement.prototype.click = origClick;
    }
  });

  it("renders nothing with neither token nor mintToken", () => {
    const { container } = render(<DownloadButton mediaId="m3" />);
    expect(container.innerHTML).toBe("");
  });
});
