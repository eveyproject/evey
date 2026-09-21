// The refresh flow itself is ../lib/authRefreshCore (shared with the browser
// TV client); this module binds it to the device's secure store, where the
// phone and the television keep their refresh token.
import { createAuthRefresh } from "../lib/authRefreshCore";
import { getServerUrl } from "./server-config";
import { getRefreshToken, setRefreshToken, clearRefreshToken } from "./refresh-token";

export { isAccessExpired } from "../lib/authRefreshCore";
export type { AccessTokenState, RefreshOutcome } from "../lib/authRefreshCore";

const refresh = createAuthRefresh({ getRefreshToken, setRefreshToken, clearRefreshToken, getServerUrl });

export const ensureAccessToken = refresh.ensureAccessToken;
export const refreshAccessTokenIfNeeded = refresh.refreshAccessTokenIfNeeded;
export const forceRefresh = refresh.forceRefresh;
