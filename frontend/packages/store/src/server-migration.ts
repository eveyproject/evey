// One-time migration from single-server storage to multi-server store.
//
// Accepts the legacy URL reader as a parameter so that mobile (expo-secure-store)
// and TV can each inject their own storage accessor — no hard dep on expo-secure-store here.
//
// Idempotent: if servers is already populated or the migration has run before, it no-ops.

import { useServerListStore } from "./servers";
import { useAuthStore } from "./auth";

export interface MigrateFromSingleServerOpts {
  /** Read the legacy "evey-server-url" from the platform's secure store */
  readLegacyServerUrl: () => Promise<string | null>;
}

export interface MigrateResult {
  migrated: boolean;
  serverId: string | null;
}

export async function migrateFromSingleServer(
  opts: MigrateFromSingleServerOpts
): Promise<MigrateResult> {
  const serverStore = useServerListStore.getState();

  // Already migrated: servers map is non-empty or an active server is set
  if (
    Object.keys(serverStore.servers).length > 0 ||
    serverStore.activeServerId !== null
  ) {
    return { migrated: false, serverId: serverStore.activeServerId };
  }

  let legacyUrl: string | null = null;
  try {
    legacyUrl = await opts.readLegacyServerUrl();
  } catch {
    // Storage read failed — not a fatal error
    return { migrated: false, serverId: null };
  }

  if (!legacyUrl) {
    return { migrated: false, serverId: null };
  }

  // Add the legacy server URL to the store
  const server = useServerListStore.getState().addServer(legacyUrl);

  // Pull existing flat tokens from auth store and assign them to the new server slot
  const authState = useAuthStore.getState();
  const existingTokens = {
    accessToken: authState.accessToken,
    profileId: authState.profileId,
    isAdmin: authState.isAdmin,
  };

  if (existingTokens.accessToken) {
    useAuthStore.setState((state) => ({
      tokensByServer: {
        ...state.tokensByServer,
        [server.id]: existingTokens,
      },
    }));
  }

  // Mark as the active server (also bumps lastUsedAt)
  useServerListStore.getState().setActiveServer(server.id);

  return { migrated: true, serverId: server.id };
}
