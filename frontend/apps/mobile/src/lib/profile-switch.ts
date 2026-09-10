import { userServiceRefreshToken, type V1AuthResponse } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { getRefreshToken, setRefreshToken } from "./refresh-token";

// A profile switch is a refresh: the server rotates the refresh token on it.
// Send the stored one and write the rotated one back, so SecureStore stays in
// step - relying on the cookie jar alone left a stale token behind, and the
// next silent refresh replayed it into a forced logout.
export async function switchProfileToken(profileId: string, pinToken?: string): Promise<V1AuthResponse> {
  const stored = await getRefreshToken();
  const resp = await userServiceRefreshToken({
    ...(stored ? { refreshToken: stored } : {}),
    profileId,
    ...(pinToken ? { pinToken } : {}),
  });
  if (resp.refreshToken) await setRefreshToken(resp.refreshToken);
  if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
  return resp;
}
