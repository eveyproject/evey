// The reasons the server puts in ?oidc_error= after a failed SSO sign-in
// (httpapi oidcErrorReason). Shared by the web and mobile login screens so
// both say the same thing; anything else falls back to the generic message.
export const SSO_ERROR_REASONS = [
  "domain_not_allowed",
  "signup_disabled",
  "account_disabled",
  "account_exists",
  "unavailable",
  "expired",
  "cancelled",
] as const;

export type SsoErrorReason = (typeof SSO_ERROR_REASONS)[number];

export function ssoErrorKey(reason: string | null | undefined): string {
  return reason && (SSO_ERROR_REASONS as readonly string[]).includes(reason)
    ? `auth.login.ssoError.${reason}`
    : "auth.login.ssoFailed";
}
