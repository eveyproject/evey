import { userServiceRefreshToken, type V1AuthResponse } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { getRefreshToken, setRefreshToken } from "./refresh-token";

// A profile switch is a refresh: the server rotates the refresh token on it.
// Send the stored one and write the rotated one back, so SecureStore stays in
// step. The silent refresh of this app carries no cookie (credentials omit),
// so the cookie jar the bare call fell back on held a token already consumed:
// the switch was refused, or it passed and the store kept the consumed token,
// which the next silent refresh replayed into a forced sign-out on a device
// with no keyboard. Same helper as the phone's, where it was fixed first.
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
