import { create } from "zustand";
import { persist } from "zustand/middleware";

interface AuthState {
  accessToken: string | null;
  refreshToken: string | null;
  profileId: string | null;
  setTokens: (access: string, refresh: string) => void;
  setProfile: (profileId: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      refreshToken: null,
      profileId: null,
      setTokens: (access, refresh) => set({ accessToken: access, refreshToken: refresh }),
      setProfile: (profileId) => set({ profileId }),
      logout: () => set({ accessToken: null, refreshToken: null, profileId: null }),
    }),
    { name: "evey-auth" }
  )
);
