import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import { TrackArt } from "./TrackArt";

const auth = vi.hoisted(() => ({ accessToken: "tok en" as string | null }));
vi.mock("@evey/store", () => ({ useAuthStore: { getState: () => ({ accessToken: auth.accessToken }) } }));

beforeEach(() => {
  auth.accessToken = "tok en";
});

// The cover is requested display-sized at 2x with the token pinned at
// mount (a refresh must not reload every visible cover); no known cover or
// a broken one degrades to the note placeholder; the image is decoration.
describe("TrackArt (web)", () => {
  it("requests a 2x display-sized cover with the pinned token", () => {
    const { container } = render(<TrackArt trackID="t1" coverArtURL="/x" size={40} />);
    const img = container.querySelector("img") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe("/api/v1/music/tracks/t1/art?token=tok%20en&w=80");
    expect(img.getAttribute("aria-hidden")).toBe("true");
    expect(img.getAttribute("width")).toBe("40");
    fireEvent.error(img);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).toBeTruthy();
  });

  it("omits the token when signed out and shows the placeholder without a known cover", () => {
    auth.accessToken = null;
    const { container, rerender } = render(<TrackArt trackID="t2" coverArtURL="/x" />);
    expect(container.querySelector("img")?.getAttribute("src")).toBe("/api/v1/music/tracks/t2/art?w=72");
    rerender(<TrackArt trackID="t3" />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).toBeTruthy();
  });
});
