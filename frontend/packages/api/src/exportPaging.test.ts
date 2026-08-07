import { describe, it, expect, vi } from "vitest";
import { collectAllPages, EXPORT_PAGE_SIZE } from "./exportPaging";

function rowsOf(n: number, offset = 0): number[] {
  return Array.from({ length: n }, (_, i) => offset + i);
}

describe("collectAllPages", () => {
  // The bug this exists for: one request for 10000 rows came back holding 500,
  // and the export was written to disk as if it were everything.
  it("walks past the server's page cap instead of stopping at the first page", async () => {
    const fetchPage = vi.fn(async (pageIndex: number) => ({
      rows: rowsOf(pageIndex < 2 ? EXPORT_PAGE_SIZE : 40, pageIndex * EXPORT_PAGE_SIZE),
      total: 1040,
    }));

    const out = await collectAllPages(fetchPage);

    expect(fetchPage).toHaveBeenCalledTimes(3);
    expect(out.rows).toHaveLength(1040);
    expect(out.total).toBe(1040);
    expect(out.truncated).toBe(false);
  });

  it("asks for each page in order, starting at index 0", async () => {
    const seen: number[] = [];
    await collectAllPages(async (pageIndex) => {
      seen.push(pageIndex);
      return { rows: rowsOf(pageIndex === 0 ? EXPORT_PAGE_SIZE : 3), total: EXPORT_PAGE_SIZE + 3 };
    });
    expect(seen).toEqual([0, 1]);
  });

  it("stops on a short page even when the endpoint reports no total", async () => {
    const fetchPage = vi.fn(async () => ({ rows: rowsOf(10) }));
    const out = await collectAllPages(fetchPage);

    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(out.total).toBe(10);
    expect(out.truncated).toBe(false);
  });

  it("stops once the server's own total is reached, without a wasted request", async () => {
    const fetchPage = vi.fn(async (pageIndex: number) => ({
      rows: rowsOf(EXPORT_PAGE_SIZE, pageIndex * EXPORT_PAGE_SIZE),
      total: EXPORT_PAGE_SIZE * 2,
    }));

    const out = await collectAllPages(fetchPage);

    expect(fetchPage).toHaveBeenCalledTimes(2);
    expect(out.rows).toHaveLength(EXPORT_PAGE_SIZE * 2);
  });

  // A server that keeps handing back full pages must not loop forever - the
  // ceiling is the only thing that makes this terminate, so it is asserted
  // rather than assumed.
  it("stops at the row ceiling and says so, on an endpoint that never ends", async () => {
    let calls = 0;
    const out = await collectAllPages(
      async (pageIndex) => {
        calls++;
        return { rows: rowsOf(10, pageIndex * 10), total: 1_000_000 };
      },
      { pageSize: 10, maxRows: 35 },
    );

    expect(calls).toBe(4);
    expect(out.rows).toHaveLength(35);
    expect(out.truncated).toBe(true);
    expect(out.total).toBe(1_000_000);
  });

  it("reports truncated=false when the ceiling lands exactly on the last row", async () => {
    const out = await collectAllPages(
      async (pageIndex) => ({ rows: rowsOf(10, pageIndex * 10), total: 20 }),
      { pageSize: 10, maxRows: 20 },
    );

    expect(out.rows).toHaveLength(20);
    expect(out.truncated).toBe(false);
  });

  it("handles an empty first page without a second request", async () => {
    const fetchPage = vi.fn(async () => ({ rows: [], total: 0 }));
    const out = await collectAllPages(fetchPage);

    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(out.rows).toEqual([]);
    expect(out.truncated).toBe(false);
  });
});
