import { useMemo } from "react";
import { useAuthStore } from "@evey/store";

interface AuthenticatedImageSource {
  uri: string;
  headers: { Authorization: string };
}

export function useAuthenticatedImage(uri: string | undefined): AuthenticatedImageSource | undefined {
  const token = useAuthStore((s) => s.accessToken);

  return useMemo(() => {
    if (!uri || !token) return undefined;
    return { uri, headers: { Authorization: `Bearer ${token}` } };
  }, [uri, token]);
}

export function useAuthHeaders(): { Authorization: string } | undefined {
  const token = useAuthStore((s) => s.accessToken);
  return useMemo(() => {
    if (!token) return undefined;
    return { Authorization: `Bearer ${token}` };
  }, [token]);
}
