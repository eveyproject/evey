import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { libraryServiceListMediaItems, type LibraryServiceListMediaItemsParams, type V1ListMediaItemsResponse, type V1MediaItem } from "@evey/api";

/**
 * A home row that shows its first page at once and fetches the rest as the
 * focus nears its end, page by page, up to `cap` items. A new first page (a
 * refetch, another layout) starts the row over.
 */
export function useRowPages(
  params: LibraryServiceListMediaItemsParams,
  firstPage: V1ListMediaItemsResponse | undefined,
  pageSize: number,
  cap: number,
): { items: V1MediaItem[]; loadMore: () => void } {
  const [extra, setExtra] = useState<V1MediaItem[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const loading = useRef(false);

  useEffect(() => {
    setExtra([]);
    setCursor(firstPage?.pagination?.nextCursor || undefined);
  }, [firstPage]);

  const items = useMemo(() => [...(firstPage?.items ?? []), ...extra].slice(0, cap), [firstPage, extra, cap]);

  const loadMore = useCallback(() => {
    if (loading.current || !cursor || items.length >= cap) return;
    loading.current = true;
    libraryServiceListMediaItems({
      ...params,
      "pagination.pageSize": Math.min(pageSize, cap - items.length),
      "pagination.cursor": cursor,
    })
      .then((page) => {
        setExtra((prev) => [...prev, ...(page.items ?? [])]);
        setCursor(page.pagination?.nextCursor || undefined);
      })
      // The row keeps what it has; the next approach of its end tries again.
      .catch(() => {})
      .finally(() => {
        loading.current = false;
      });
  }, [params, cursor, items.length, cap, pageSize]);

  return { items, loadMore };
}
