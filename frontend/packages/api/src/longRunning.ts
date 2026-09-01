import { eveyFetch, NO_TIMEOUT } from "./fetcher";
import type { V1DeleteLibraryResponse } from "./generated/library.schemas";
import type { V1ClearTranscodeCacheResponse } from "./generated/transcode_cache.schemas";

// The generated clients of these two RPCs take no `signal` argument (no body,
// no query), so a caller cannot opt out of the fetcher's default timeout
// through them - and both do their work INSIDE the request: clearing the
// transcode cache removes every segment file on the volume, deleting a
// library cascades over all its rows. On a NAS either can outlast 60 s while
// the server keeps going; the UI would report "no answer" for a job that then
// completes. Same URL and method as the generated functions, unbounded.

export function clearTranscodeCacheUnbounded(): Promise<V1ClearTranscodeCacheResponse> {
  return eveyFetch<V1ClearTranscodeCacheResponse>({ url: "/api/v1/admin/transcode-cache", method: "DELETE", signal: NO_TIMEOUT });
}

export function deleteLibraryUnbounded(id: string): Promise<V1DeleteLibraryResponse> {
  return eveyFetch<V1DeleteLibraryResponse>({ url: `/api/v1/libraries/${encodeURIComponent(id)}`, method: "DELETE", signal: NO_TIMEOUT });
}
