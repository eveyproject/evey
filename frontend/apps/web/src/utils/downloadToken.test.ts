import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetch = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetch(...args) }));

import { mintDownloadToken, startTokenDownload } from "./downloadToken";

// The account JWT must never ride in a download URL: the anchor carries a
// one-shot token minted at click time, bound to the one resource.
describe("downloadToken", () => {
  beforeEach(() => {
    eveyFetch.mockReset();
    eveyFetch.mockResolvedValue({ token: "tok-1" });
  });

  it("mints against the resource id, POST, id URL-encoded", async () => {
    await expect(mintDownloadToken("a b/c")).resolves.toBe("tok-1");
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/download-token/a%20b%2Fc", method: "POST" });
  });

  it("appends the minted token to the URL, respecting an existing query string", async () => {
    const clicked: HTMLAnchorElement[] = [];
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      clicked.push(this);
    });
    try {
      await startTokenDownload("/api/v1/media/m1/download", "m1", "film.mkv");
      await startTokenDownload("/api/v1/media/m1/download?w=1", "m1");
    } finally {
      click.mockRestore();
    }
    expect(clicked).toHaveLength(2);
    expect(clicked[0].getAttribute("href")).toBe("/api/v1/media/m1/download?token=tok-1");
    expect(clicked[0].download).toBe("film.mkv");
    expect(clicked[1].getAttribute("href")).toBe("/api/v1/media/m1/download?w=1&token=tok-1");
    expect(clicked[1].download).toBe("");
    // The transient anchor does not linger in the DOM.
    expect(document.body.querySelector("a")).toBeNull();
  });

  it("does not trigger a download when the mint fails", async () => {
    eveyFetch.mockRejectedValue(new Error("401"));
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    try {
      await expect(startTokenDownload("/x", "m1")).rejects.toMatchObject({ message: "401" });
      expect(click).not.toHaveBeenCalled();
    } finally {
      click.mockRestore();
    }
  });
});
