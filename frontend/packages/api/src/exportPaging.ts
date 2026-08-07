// Collecting a whole server-side list for an export, shared by web and mobile
// so the rule lives in ONE place.
//
// The admin exports used to ask for a single page of 10000 rows. No endpoint
// has ever honoured that: GetPlayHistory clamps to maxPageSize
// (clampPagination, grpcapi/helpers.go) and the audit log clamps in its repo -
// both at 500. So "Export" handed back a file holding the first 500 rows,
// named as if it were the whole log, with nothing on screen or in the file to
// say otherwise. An export that silently drops data is worse than one that
// refuses: nobody re-checks a file they already downloaded.

// The largest page any of these endpoints serves. Asking for more is not an
// error, it is silently reduced - which is exactly how this went unnoticed.
export const EXPORT_PAGE_SIZE = 500;

// Hard ceiling on a single export. The target machine is a weak NAS: an
// unbounded walk over years of history would hold the whole result in memory
// on both ends, and a bug in a stop condition would loop forever. Reaching it
// is reported, never silent.
export const EXPORT_MAX_ROWS = 50_000;

export interface ExportPage<T> {
  rows: T[];
  /** Total matching rows server-side, when the endpoint reports one. */
  total?: number;
}

export interface CollectedExport<T> {
  rows: T[];
  /** What the server said exists, or the number collected if it never said. */
  total: number;
  /** EXPORT_MAX_ROWS was reached with rows still left behind. */
  truncated: boolean;
}

/**
 * Walks every page of a list endpoint and returns the rows as one array.
 *
 * `fetchPage` receives a 0-based page index and must request exactly
 * `pageSize` rows at the matching offset - the two endpoints spell that
 * differently (`pagination.page` is 1-based, the audit log takes a raw
 * `offset`), so building the params stays with the caller.
 *
 * Stops on the first of: a short page, the server's own total being reached,
 * or the row ceiling. The ceiling is what makes this loop provably finite -
 * a server that kept returning full pages forever would otherwise never end
 * it.
 */
export async function collectAllPages<T>(
  fetchPage: (pageIndex: number) => Promise<ExportPage<T>>,
  opts?: { pageSize?: number; maxRows?: number },
): Promise<CollectedExport<T>> {
  const pageSize = opts?.pageSize ?? EXPORT_PAGE_SIZE;
  const maxRows = opts?.maxRows ?? EXPORT_MAX_ROWS;

  const rows: T[] = [];
  let reportedTotal: number | undefined;

  for (let pageIndex = 0; rows.length < maxRows; pageIndex++) {
    const page = await fetchPage(pageIndex);
    const got = page.rows ?? [];
    if (page.total !== undefined) reportedTotal = page.total;
    rows.push(...got);

    // A page shorter than asked for is the last one. Checked before the total
    // so an endpoint that reports no total still terminates.
    if (got.length < pageSize) break;
    if (reportedTotal !== undefined && rows.length >= reportedTotal) break;
  }

  const capped = rows.length > maxRows ? rows.slice(0, maxRows) : rows;
  const total = reportedTotal ?? capped.length;
  return { rows: capped, total, truncated: capped.length < total };
}
