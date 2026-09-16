import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import { AdminTrickplayThumbnail } from "./AdminTrickplayThumbnail";

const cues = vi.hoisted(() => ({ list: [] as unknown[] }));
vi.mock("@/features/playback/trickplayCues", () => ({
  useTrickplayCues: () => cues.list,
  tileForTime: (list: unknown[], ms: number) => (list as { startMs: number; endMs: number }[]).find((c) => ms >= c.startMs && ms < c.endMs) ?? null,
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "t k" }) }));

beforeEach(() => {
  cues.list = [];
});

// A thumbnail PREFERS the pre-baked sprite tile (a plain crop); without one
// it falls back to the on-demand snapshot - a work order for ffmpeg - whose
// URL is quantised to 15 s so the 3 s dashboard poll cannot spawn a decode
// every tick; a failed snapshot hides rather than showing a broken image.
describe("AdminTrickplayThumbnail", () => {
  it("crops the sprite tile once its natural width is known", () => {
    cues.list = [{ startMs: 0, endMs: 10_000, url: "/sprite.jpg", x: 160, y: 90, w: 160, h: 90 }];
    const { container } = render(<AdminTrickplayThumbnail mediaId="m1" positionMs={5_000} mediaType="movie" />);
    const img = container.querySelector("img")!;
    expect(img.getAttribute("src")).toBe("/sprite.jpg");
    expect(img.style.opacity).toBe("0");
    Object.defineProperty(img, "naturalWidth", { value: 800 });
    fireEvent.load(img);
    expect(img.style.width).toBe("500%");
    expect(img.style.left).toBe("-100%");
    expect(img.style.top).toBe("-100%");
  });

  it("falls back to a quantised, tokenised snapshot URL and hides on error", () => {
    const { container, rerender } = render(<AdminTrickplayThumbnail mediaId="m1" positionMs={44_999} mediaType="episode" />);
    const img = container.querySelector("img")!;
    expect(img.getAttribute("src")).toBe("/api/v1/media/m1/snapshot?position_ms=30000&token=t%20k");
    rerender(<AdminTrickplayThumbnail mediaId="m1" positionMs={46_000} mediaType="episode" />);
    expect(container.querySelector("img")!.getAttribute("src")).toContain("position_ms=45000");
    fireEvent.error(container.querySelector("img")!);
    expect(container.querySelector("img")).toBeNull();
  });

  it("renders nothing for a type that has no frames, or without a media", () => {
    expect(render(<AdminTrickplayThumbnail mediaId="m1" positionMs={0} mediaType="MEDIA_ITEM_TYPE_TRACK" />).container.innerHTML).toBe("");
    expect(render(<AdminTrickplayThumbnail mediaId="" positionMs={0} />).container.innerHTML).toBe("");
  });
});
