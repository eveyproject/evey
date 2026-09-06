// The little that the profile picker needs to draw itself, kept on the device.
//
// The picker is shown at cold start, which is exactly the moment the profile
// list has not arrived yet - and, on a plane, never will. Rendering it from a
// network response would make it either a spinner or a dead end. So the four
// fields it draws are written down every time the real list lands, and the
// picker reads them instead.
//
// `isKids` rides along because it decides whether the picker is shown at all.
// It is a DISPLAY hint and nothing more: the actual gate lives in the server's
// RefreshToken, which re-reads the profiles itself and demands a PIN proof to
// leave a restricted profile. A tampered cache buys a different-looking list,
// not a switch the server would have refused.
import * as SecureStore from "expo-secure-store";

const KEY = "evey:profiles_min";

export interface CachedProfile {
  id: string;
  name: string;
  avatarPath?: string;
  isKids: boolean;
}

/** Shape of the live V1Profile fields this cache keeps. */
interface ProfileLike {
  id?: string;
  name?: string;
  avatarPath?: string;
  isKids?: boolean;
}

export async function saveProfileCache(profiles: ProfileLike[] | undefined): Promise<void> {
  // An empty list is not a valid answer to cache: it is what a failed or
  // not-yet-loaded query looks like, and writing it would erase a good cache
  // and send the next cold start straight past the picker.
  if (!profiles || profiles.length === 0) return;
  const minimal: CachedProfile[] = profiles
    .filter((p): p is ProfileLike & { id: string } => !!p.id)
    .map((p) => ({
      id: p.id,
      name: p.name ?? "",
      ...(p.avatarPath ? { avatarPath: p.avatarPath } : {}),
      isKids: p.isKids === true,
    }));
  if (minimal.length === 0) return;
  try {
    await SecureStore.setItemAsync(KEY, JSON.stringify(minimal));
  } catch {
    // Best effort: losing the cache costs a spinner, never a broken screen.
  }
}

export async function loadProfileCache(): Promise<CachedProfile[]> {
  try {
    const raw = await SecureStore.getItemAsync(KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (p): p is CachedProfile => !!p && typeof (p as CachedProfile).id === "string",
    );
  } catch {
    return [];
  }
}

/** Dropped on logout and on a server switch - it describes one account on one server. */
export async function clearProfileCache(): Promise<void> {
  try {
    await SecureStore.deleteItemAsync(KEY);
  } catch {
    /* best effort */
  }
}

/**
 * Whether a cold start should stop at the picker.
 *
 * Only when somebody could pick WRONG: a household with a kids profile, and
 * more than one profile to choose from. A single-profile account, or one with
 * no kids profile at all, has nothing to protect and gets no screen - that was
 * the whole point of asking.
 */
export function shouldPickProfileOnColdStart(profiles: CachedProfile[]): boolean {
  return profiles.length >= 2 && profiles.some((p) => p.isKids);
}
