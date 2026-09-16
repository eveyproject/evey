import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { getRefreshToken, clearRefreshToken } from "./refresh-token";

// Ends THIS device's session: the server deletes the presented refresh token
// only (other devices keep theirs), then the token leaves the disk. The access
// token is captured up front: the fetcher reads it after an await, by which
// time the caller's logout() has emptied the store and the request used to go
// out unauthenticated - the server never heard of any sign-out.
export async function signOutDevice(): Promise<void> {
  const access = useAuthStore.getState().accessToken;
  let stored: string | null = null;
  try {
    stored = await getRefreshToken();
  } catch {
    stored = null;
  }
  if (access) {
    void eveyFetch({
      url: "/api/v1/auth/logout",
      method: "POST",
      data: stored ? { refreshToken: stored } : {},
      headers: { Authorization: `Bearer ${access}` },
    }).catch((err) => console.warn("logout revoke failed", err));
  }
  await clearRefreshToken();
}
