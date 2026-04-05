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

interface AuthState {
  accessToken: string | null;
  refreshToken: string | null;
  profileId: string | null;
  isAdmin: boolean;
  setTokens: (access: string, refresh?: string) => void;
  setProfile: (profileId: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      refreshToken: null,
      profileId: null,
      isAdmin: false,
      setTokens: (access, refresh) => {
        const claims = parseJwtPayload(access);
        const update: Partial<AuthState> = {
          accessToken: access,
          isAdmin: claims?.is_admin === true,
          profileId: typeof claims?.profile_id === "string" ? claims.profile_id : null,
        };
        if (refresh !== undefined) update.refreshToken = refresh;
        set(update);
        if (typeof claims?.locale === "string" && claims.locale) {
          useLocaleStore.getState().setLocale(claims.locale);
        }
      },
      setProfile: (profileId) => set({ profileId }),
      logout: () => set({ accessToken: null, refreshToken: null, profileId: null, isAdmin: false }),
    }),
    {
      name: "evey-auth",
      storage: createJSONStorage(() => getZustandStorage()),
      partialize: (state) => ({
        profileId: state.profileId,
        accessToken: state.accessToken,
        refreshToken: state.refreshToken,
        isAdmin: state.isAdmin,
      }),
    }
  )
);
