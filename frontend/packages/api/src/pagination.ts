// Pagination contract helpers, shared by web/mobile/TV so the rule lives in
// ONE place. It was duplicated per app before, and the same defect shipped
// twice (mobile and TV both froze every library at its first page).

// Whether a keyset-paginated list response has another page.
//
// The server emits `next_cursor` only when a FULL page came back
// (nextMediaItemsCursor, library_handler_mediaitems.go), so its presence is
// the complete answer - no count or total arithmetic needed.
//
// Do NOT reintroduce a `page < totalPages` test. The server builds
// PaginationResponse with TotalItems/Page/PageSize/NextCursor and NEVER sets
// total_pages, so the field arrives present-and-zero on every response: a
// `totalPages ?? 1` default keeps the 0, `1 < 0` is false, and the list stops
// after one page. Observed in production on 2026-07-28 with
// `{totalItems: 8076, totalPages: 0, nextCursor: "..."}`.
//
// The same trap exists for ListPhotos: its keyset path (a non-empty
// request cursor) reports `total: 0` by design - recomputing an exact total
// there would pay back exactly the COUNT(*) cost the cursor path exists to
// avoid (photo_handler.go). `total` is therefore never a valid "more pages"
// signal for any cursor-paginated list; nextCursor's presence is.
export function hasMoreFromCursor(
  pagination: { nextCursor?: string } | undefined,
): boolean {
  return !!pagination?.nextCursor;
}
