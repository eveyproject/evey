import { renderHook } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockHook = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useActivityServiceGetPopularMedia: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => {
    const raw = mockHook();
    const select = opts?.query?.select;
    return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
  },
}));

describe("useMostPopular", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns items from API", async () => {
    mockHook.mockReturnValue({
      data: {
        items: [
          { mediaItemId: "m-1", title: "Movie A", type: "movie", playCount: 42 },
          { mediaItemId: "m-2", title: "Movie B", type: "movie", playCount: 10 },
        ],
      },
      isSuccess: true,
    });
    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular());
    expect(result.current.data).toEqual([
      { mediaItemId: "m-1", title: "Movie A", type: "movie", playCount: 42 },
      { mediaItemId: "m-2", title: "Movie B", type: "movie", playCount: 10 },
    ]);
  });

  it("carries the release year, and drops an unknown one", async () => {
    mockHook.mockReturnValue({
      data: {
        items: [
          { mediaItemId: "m-1", title: "Movie A", type: "movie", playCount: 4, year: 1999 },
          { mediaItemId: "m-2", title: "Movie B", type: "movie", playCount: 3, year: 0 },
        ],
      },
      isSuccess: true,
    });
    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular());
    expect(result.current.data?.map((i) => i.year)).toEqual([1999, undefined]);
  });

  it("carries the poster placeholder and the audience score, and drops their proto zeros", async () => {
    mockHook.mockReturnValue({
      data: {
        items: [
          { mediaItemId: "m-1", title: "Movie A", type: "movie", playCount: 4, primaryBlurhash: "LKO2", communityRating: 8.1 },
          { mediaItemId: "m-2", title: "Movie B", type: "movie", playCount: 3, primaryBlurhash: "", communityRating: 0 },
        ],
      },
      isSuccess: true,
    });
    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular());
    expect(result.current.data?.map((i) => [i.primaryBlurhash, i.communityRating])).toEqual([
      ["LKO2", 8.1],
      [undefined, undefined],
    ]);
  });

  it("returns empty array when items is missing", async () => {
    mockHook.mockReturnValue({ data: {}, isSuccess: true });
    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular());
    expect(result.current.data).toEqual([]);
  });
});
