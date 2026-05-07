// Per-server token storage with backward-compatible shadow fields.
//
// Design (Option A from spec):
//   - `tokensByServer` stores tokens keyed by serverId.
//   - Shadow fields (`accessToken`, `refreshToken`, `profileId`, `isAdmin`) mirror
//     the active server's entry so existing call sites need no changes.
//   - Every setter writes to BOTH the shadow field and `tokensByServer[activeServerId]`.
//   - `syncFromServer(id)` is called by `useServerListStore.setActiveServer` to update
//     the shadow fields when the active server changes.

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
  refreshToken: string | null;
  profileId: string | null;
  isAdmin: boolean;
}

const EMPTY_TOKENS: ServerTokens = {
  accessToken: null,
  refreshToken: null,
  profileId: null,
  isAdmin: false,
};

interface AuthState {
  // Internal per-server storage
  tokensByServer: Record<string, ServerTokens>;

  // Backward-compat shadow fields — always reflect the active server's tokens
  accessToken: string | null;
  refreshToken: string | null;
  profileId: string | null;
  isAdmin: boolean;

  // Setters — write to shadow fields AND to tokensByServer[activeServerId]
  setTokens: (access: string, refresh?: string) => void;
  setProfile: (profileId: string) => void;
  logout: () => void;
  logoutAll: () => void;

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
      refreshToken: null,
      profileId: null,
      isAdmin: false,

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

      setTokens: (access: string, refresh?: string) => {
        const claims = parseJwtPayload(access);
        const activeId = get()._getActiveServerId();

        const incoming: Partial<ServerTokens> = {
          accessToken: access,
          isAdmin: claims?.is_admin === true,
          profileId: typeof claims?.profile_id === "string" ? claims.profile_id : null,
        };
        if (refresh !== undefined) incoming.refreshToken = refresh;

        const shadow: Partial<AuthState> = {
          accessToken: incoming.accessToken,
          isAdmin: incoming.isAdmin,
          profileId: incoming.profileId,
        };
        if (refresh !== undefined) shadow.refreshToken = refresh;

        set((state) => {
          const prev = activeId ? (state.tokensByServer[activeId] ?? EMPTY_TOKENS) : EMPTY_TOKENS;
          const next: ServerTokens = { ...prev, ...incoming };
          return {
            ...shadow,
            tokensByServer: activeId
              ? { ...state.tokensByServer, [activeId]: next }
              : state.tokensByServer,
          };
        });

        if (typeof claims?.locale === "string" && claims.locale) {
          useLocaleStore.getState().setLocale(claims.locale);
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

      logout: () => {
        const activeId = get()._getActiveServerId();
        set((state) => ({
          ...EMPTY_TOKENS,
          tokensByServer: activeId
            ? { ...state.tokensByServer, [activeId]: { ...EMPTY_TOKENS } }
            : state.tokensByServer,
        }));
      },

      logoutAll: () => {
        set({ ...EMPTY_TOKENS, tokensByServer: {} });
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
        refreshToken: state.refreshToken,
        isAdmin: state.isAdmin,
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
