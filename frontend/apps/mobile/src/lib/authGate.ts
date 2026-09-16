export type RefreshState = "pending" | "refused" | "unavailable";
export type AuthGateDecision = "login" | "wait" | "profile-select" | "render";

export interface AuthGateInput {
  accessToken: string | null;
  expired: boolean;
  refreshState: RefreshState;
  online: boolean;
  profileId: string | null;
}

// Pure decision behind app/(auth)/_layout.tsx, kept out of the component so
// the offline contract is testable without the whole navigator.
//
// Only a REFUSED refresh ends the session. An unanswered one is the network,
// not the credential: /login cannot be completed offline, so bouncing there
// is a dead end that locks a viewer out of their own downloads. Offline the
// refresh cannot be refused at all, so the tree renders straight away
// instead of sitting blank until the attempt times out; the layout retries
// the refresh the moment connectivity returns.
export function authGateDecision({ accessToken, expired, refreshState, online, profileId }: AuthGateInput): AuthGateDecision {
  if (!accessToken) return "login";
  if (expired) {
    if (refreshState === "refused") return "login";
    if (refreshState === "pending" && online) return "wait";
  }
  if (!profileId) return "profile-select";
  return "render";
}
