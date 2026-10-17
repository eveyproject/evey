// FilterMediaItems pages by offset. The server says where the next page starts
// (next_offset: the rows it read, BEFORE the parental pass removed any) and
// whether there is one. Counting the items received instead re-read rows and,
// on a kids profile, ended the scroll at the first page the pass shortened.
export interface FilterPage {
  items?: readonly unknown[];
  total?: number;
  nextOffset?: number;
  hasMore?: boolean;
}

// The offset of the next page, or undefined when this one was the last.
// requestOffset is the offset this page was asked at.
export function nextFilterOffset(page: FilterPage, requestOffset: number, limit: number): number | undefined {
  if (page.hasMore !== undefined || page.nextOffset !== undefined) {
    return page.hasMore && page.nextOffset !== undefined ? page.nextOffset : undefined;
  }
  // A server older than next_offset/has_more: best effort from the items.
  const got = page.items?.length ?? 0;
  const fetched = requestOffset + got;
  if (got < limit || fetched >= (page.total ?? 0)) return undefined;
  return fetched;
}
