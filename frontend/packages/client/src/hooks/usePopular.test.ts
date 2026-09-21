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

  it("returns empty array when items is missing", async () => {
    mockHook.mockReturnValue({ data: {}, isSuccess: true });
    const { useMostPopular } = await import("./usePopular");
    const { result } = renderHook(() => useMostPopular());
    expect(result.current.data).toEqual([]);
  });
});
