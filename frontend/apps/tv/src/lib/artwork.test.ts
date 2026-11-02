import { describe, it, expect, vi } from "vitest";

vi.mock("react-native", () => ({ PixelRatio: { getPixelSizeForLayoutSize: (dp: number) => dp * 2 } }));
vi.mock("@evey/client", () => ({ artworkUrl: vi.fn(), coverArtUri: vi.fn() }));

import { tmdbProfileSized } from "./artwork";

describe("tmdbProfileSized", () => {
  it("asks TMDB for the smallest profile size that covers the picture", () => {
    expect(tmdbProfileSized("https://image.tmdb.org/t/p/original/abc.jpg", 160)).toBe("https://image.tmdb.org/t/p/w185/abc.jpg");
    expect(tmdbProfileSized("https://image.tmdb.org/t/p/original/abc.jpg", 280)).toBe("https://image.tmdb.org/t/p/h632/abc.jpg");
  });

  it("keeps the original past the largest width, and leaves other urls alone", () => {
    expect(tmdbProfileSized("https://image.tmdb.org/t/p/original/abc.jpg", 2000)).toBe("https://image.tmdb.org/t/p/original/abc.jpg");
    expect(tmdbProfileSized("https://image.tmdb.org/t/p/w500/abc.jpg", 160)).toBe("https://image.tmdb.org/t/p/w500/abc.jpg");
    expect(tmdbProfileSized("https://static.tvmaze.com/p/1.jpg", 160)).toBe("https://static.tvmaze.com/p/1.jpg");
  });
});
