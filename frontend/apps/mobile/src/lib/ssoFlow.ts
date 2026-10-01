// The poll secret of the SSO sign-in in progress. Held in memory only: it is
// half of what redeems the tokens (the other half arrives through the
// evey://oidc-callback deep link), and it is taken exactly once.
let pendingPollToken: string | null = null;

export function setSsoPollToken(token: string | null): void {
  pendingPollToken = token;
}

export function takeSsoPollToken(): string | null {
  const token = pendingPollToken;
  pendingPollToken = null;
  return token;
}
