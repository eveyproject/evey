// Errors thrown by packages/api/src/fetcher.ts (eveyFetch/eveyFetchRaw) carry
// `httpStatus` once a response actually came back from the server (see
// fetchWithRefresh). Its absence means the request never reached the server at
// all - fetch() itself rejected (no network, DNS/TLS failure, CORS) - so that
// case is classified as "offline" instead of being merged into one opaque
// "something went wrong" bucket that hides whether retrying can help.
export type ErrorKind = "not-found" | "server" | "offline" | "unknown";

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== "object" || error === null) return undefined;
  const status = (error as { httpStatus?: unknown }).httpStatus;
  return typeof status === "number" ? status : undefined;
}

export function classifyError(error: unknown): ErrorKind {
  const httpStatus = getHttpStatus(error);
  if (httpStatus === undefined) return "offline";
  if (httpStatus === 404) return "not-found";
  if (httpStatus >= 500) return "server";
  return "unknown";
}
