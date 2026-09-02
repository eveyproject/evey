// Who is behind an id in a log line. The server logs user_id / profile_id
// (the only stable handle it has); the operator reads names. Resolving them
// in the explorer, from the admin's user and profile listings, is what makes
// a line like `user_id=usr-ddcb76cd…` readable without a SQL query - which
// is what debugging a beta tester's report used to take (2026-09-03).

interface IdentityUser {
  id?: string;
  username?: string;
}

interface IdentityProfile {
  id?: string;
  userId?: string;
  name?: string;
}

// IdentityIndex maps every known user AND profile id to a display label. A
// profile carries its owner too ("Kids (gravity)"): the profile name alone is
// ambiguous across accounts.
export type IdentityIndex = ReadonlyMap<string, string>;

export const EMPTY_IDENTITIES: IdentityIndex = new Map();

export function buildIdentityIndex(users: IdentityUser[], profiles: IdentityProfile[]): IdentityIndex {
  const idx = new Map<string, string>();
  for (const u of users) {
    if (u.id && u.username) idx.set(u.id, u.username);
  }
  for (const p of profiles) {
    if (!p.id || !p.name) continue;
    const owner = p.userId ? idx.get(p.userId) : undefined;
    idx.set(p.id, owner ? `${p.name} (${owner})` : p.name);
  }
  return idx;
}

// labelFor decorates a value that IS a known id ("gravity (usr-…)"); any other
// value comes back untouched. Applied to every string field of a line, so it
// catches every key spelling (user_id, userID, target_user, impersonated_by…)
// without the explorer enumerating them.
export function labelFor(idx: IdentityIndex, value: string): string {
  const name = idx.get(value);
  return name ? `${name} (${value})` : value;
}

// resolveUserFilter lets the operator type a username in the user filter: an
// exact (case-insensitive) match on a known account becomes that account's id,
// which is what the server-side search and the live matcher key on. Anything
// else is passed through as an id.
export function resolveUserFilter(users: IdentityUser[], raw: string): string {
  const q = raw.trim();
  if (!q) return "";
  const lower = q.toLowerCase();
  const hit = users.find((u) => u.username?.toLowerCase() === lower);
  return hit?.id ?? q;
}
