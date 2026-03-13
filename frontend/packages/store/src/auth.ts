import { create } from "zustand";
import { persist } from "zustand/middleware";

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
        });
      },
      setProfile: (profileId) => set({ profileId }),
      logout: () => set({ accessToken: null, profileId: null, isAdmin: false }),
    }),
    {
      name: "evey-auth",
      partialize: (state) => ({ profileId: state.profileId }),
    }
  )
);
