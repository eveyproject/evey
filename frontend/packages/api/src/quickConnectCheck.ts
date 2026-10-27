import { eveyFetch } from "./fetcher";
import type { V1CheckQuickConnectResponse } from "./generated/user.schemas";

// The pairing secret travels as a header, never in the URL, so it stays out
// of every access log between the device and the server. Only the browser's
// event stream, which cannot set a header, passes it in the query.
export const QUICK_CONNECT_SECRET_HEADER = "X-Quick-Connect-Secret";

export function checkQuickConnect(code: string, secret?: string | null): Promise<V1CheckQuickConnectResponse> {
  return eveyFetch<V1CheckQuickConnectResponse>({
    url: `/api/v1/auth/quick-connect/${encodeURIComponent(code)}`,
    method: "GET",
    headers: secret ? { [QUICK_CONNECT_SECRET_HEADER]: secret } : undefined,
  });
}
