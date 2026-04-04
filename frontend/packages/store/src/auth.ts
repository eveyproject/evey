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
  profileId: string | null;
  isAdmin: boolean;
  setTokens: (access: string) => void;
  setProfile: (profileId: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      profileId: null,
      isAdmin: false,
      setTokens: (access) => {
        const claims = parseJwtPayload(access);
        set({
          accessToken: access,
          isAdmin: claims?.is_admin === true,
          profileId: typeof claims?.profile_id === "string" ? claims.profile_id : null,
        });
        // Sync locale from JWT to localeStore
        if (typeof claims?.locale === "string" && claims.locale) {
          useLocaleStore.getState().setLocale(claims.locale);
        }
      },
      setProfile: (profileId) => set({ profileId }),
      logout: () => set({ accessToken: null, profileId: null, isAdmin: false }),
    }),
    {
      name: "evey-auth",
      storage: createJSONStorage(() => getZustandStorage()),
      partialize: (state) => ({ profileId: state.profileId, accessToken: state.accessToken, isAdmin: state.isAdmin }),
    }
  )
);
