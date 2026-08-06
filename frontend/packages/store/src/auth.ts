// Per-server token storage with backward-compatible shadow fields.
//
// Design (Option A from spec):
//   - `tokensByServer` stores tokens keyed by serverId.
//   - Shadow fields (`accessToken`, `profileId`, `isAdmin`) mirror
//     the active server's entry so existing call sites need no changes.
//   - Every setter writes to BOTH the shadow field and `tokensByServer[activeServerId]`.
//   - `syncFromServer(id)` is called by `useServerListStore.setActiveServer` to update
//     the shadow fields when the active server changes.
//
// Refresh tokens are NOT held in this store - web uses HttpOnly cookies, mobile
// uses expo-secure-store via auth-refresh.ts.

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { getZustandStorage } from "./storage";
import { useLocaleStore } from "./locale";

function parseJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const payload = atob(parts[1].replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(payload) as Record<string, unknown>;
  } catch {
    return null;
  }
}

interface ServerTokens {
  accessToken: string | null;
  profileId: string | null;
  isAdmin: boolean;
}

const EMPTY_TOKENS: ServerTokens = {
  accessToken: null,
  profileId: null,
  isAdmin: false,
};

// Shape persisted to localStorage by persist().partialize below. Used by
// hydrateFromPersisted for faithful cross-tab sync (a storage event carries
// exactly this blob).
export interface PersistedAuthState {
  tokensByServer?: Record<string, ServerTokens>;
  accessToken?: string | null;
  profileId?: string | null;
  isAdmin?: boolean;
  impersonatedBy?: string | null;
  impersonatedUsername?: string | null;
  impersonationOrigin?: string | null;
}

interface AuthState {
  // Internal per-server storage
  tokensByServer: Record<string, ServerTokens>;

  // Backward-compat shadow fields - always reflect the active server's tokens
  accessToken: string | null;
  profileId: string | null;
  isAdmin: boolean;

  // Impersonation (admin "connect as"): impersonatedBy is the admin user id from
  // the active token's `imp_by` claim (null = a normal session). impersonationOrigin
  // holds the admin's own token so exitImpersonation can restore it seamlessly.
  impersonatedBy: string | null;
  impersonatedUsername: string | null;
  impersonationOrigin: string | null;

  // Setters - write to shadow fields AND to tokensByServer[activeServerId]
  setTokens: (access: string) => void;
  setProfile: (profileId: string) => void;
  impersonate: (access: string) => void;
  exitImpersonation: () => void;
  logout: () => void;
  logoutAll: () => void;

  // Cross-tab sync: apply a persisted auth blob (from another tab's storage
  // write) to the in-memory store, INCLUDING the impersonation fields that
  // setTokens cannot reconstruct from the JWT alone (impersonationOrigin).
  hydrateFromPersisted: (persisted: PersistedAuthState) => void;

  // Called by useServerListStore when active server changes
  syncFromServer: (id: string | null) => void;

  // Internal helper: returns the active server id from useServerListStore without
  // creating a circular import at module load time (lazy import)
  _getActiveServerId: () => string | null;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      tokensByServer: {},
      accessToken: null,
      profileId: null,
      isAdmin: false,
      impersonatedBy: null,
      impersonatedUsername: null,
      impersonationOrigin: null,

      _getActiveServerId: () => {
        try {
          // Lazy require to avoid circular dependency at module init time
          // eslint-disable-next-line @typescript-eslint/no-require-imports
          const { useServerListStore } = require("./servers") as typeof import("./servers");
          return useServerListStore.getState().activeServerId;
        } catch {
          return null;
        }
      },

      setTokens: (access: string) => {
        const claims = parseJwtPayload(access);
        const activeId = get()._getActiveServerId();
        const impBy = typeof claims?.imp_by === "string" && claims.imp_by ? claims.imp_by : null;
        const username = typeof claims?.username === "string" ? claims.username : null;

        const incoming: ServerTokens = {
          accessToken: access,
          isAdmin: claims?.is_admin === true,
          profileId: typeof claims?.profile_id === "string" ? claims.profile_id : null,
        };

        set((state) => {
          const prev = activeId ? (state.tokensByServer[activeId] ?? EMPTY_TOKENS) : EMPTY_TOKENS;
          const next: ServerTokens = { ...prev, ...incoming };
          return {
            ...incoming,
            impersonatedBy: impBy,
            impersonatedUsername: impBy ? username : null,
            tokensByServer: activeId
              ? { ...state.tokensByServer, [activeId]: next }
              : state.tokensByServer,
          };
        });

        if (typeof claims?.locale === "string" && claims.locale) {
          useLocaleStore.getState().setLocale(claims.locale, "profile");
        }
      },

      setProfile: (profileId: string) => {
        const activeId = get()._getActiveServerId();
        set((state) => {
          const prev = activeId ? (state.tokensByServer[activeId] ?? EMPTY_TOKENS) : EMPTY_TOKENS;
          return {
            profileId,
            tokensByServer: activeId
              ? { ...state.tokensByServer, [activeId]: { ...prev, profileId } }
              : state.tokensByServer,
          };
        });
      },

      impersonate: (access: string) => {
        // Stash the admin's own token so exitImpersonation can restore it without
        // a re-login. setTokens then swaps to the impersonation token (whose imp_by
        // claim drives impersonatedBy) and activates the target's profile/locale.
        set({ impersonationOrigin: get().accessToken });
        get().setTokens(access);
      },

      exitImpersonation: () => {
        const origin = get().impersonationOrigin;
        set({ impersonationOrigin: null });
        if (origin) get().setTokens(origin);
      },

      logout: () => {
        const activeId = get()._getActiveServerId();
        set((state) => ({
          ...EMPTY_TOKENS,
          impersonatedBy: null,
          impersonatedUsername: null,
          impersonationOrigin: null,
          tokensByServer: activeId
            ? { ...state.tokensByServer, [activeId]: { ...EMPTY_TOKENS } }
            : state.tokensByServer,
        }));
      },

      logoutAll: () => {
        set({ ...EMPTY_TOKENS, impersonatedBy: null, impersonatedUsername: null, impersonationOrigin: null, tokensByServer: {} });
      },

      hydrateFromPersisted: (persisted) => {
        set((state) => ({
          accessToken: persisted.accessToken ?? null,
          profileId: persisted.profileId ?? null,
          isAdmin: persisted.isAdmin ?? false,
          impersonatedBy: persisted.impersonatedBy ?? null,
          impersonatedUsername: persisted.impersonatedUsername ?? null,
          impersonationOrigin: persisted.impersonationOrigin ?? null,
          tokensByServer: persisted.tokensByServer ?? state.tokensByServer,
        }));
      },

      syncFromServer: (id: string | null) => {
        set((state) => {
          const tokens = id ? (state.tokensByServer[id] ?? EMPTY_TOKENS) : EMPTY_TOKENS;
          return { ...tokens };
        });
      },
    }),
    {
      name: "evey-auth",
      storage: createJSONStorage(() => getZustandStorage()),
      partialize: (state) => ({
        tokensByServer: state.tokensByServer,
        profileId: state.profileId,
        accessToken: state.accessToken,
        isAdmin: state.isAdmin,
        impersonatedBy: state.impersonatedBy,
        impersonatedUsername: state.impersonatedUsername,
        impersonationOrigin: state.impersonationOrigin,
      }),
    }
  )
);

// Wire up: when useServerListStore activates a different server, sync shadow fields.
// This import is deferred to break the circular dependency at module evaluation time.
// The subscription is set up once when the auth store module is first imported.
setTimeout(() => {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { useServerListStore } = require("./servers") as typeof import("./servers");
    // Register the sync callback on the server store
    useServerListStore.setState({
      _syncActiveToAuth: () => {
        const id = useServerListStore.getState().activeServerId;
        useAuthStore.getState().syncFromServer(id);
      },
    });
  } catch {
    // no-op: server store not yet available (test environment)
  }
}, 0);
