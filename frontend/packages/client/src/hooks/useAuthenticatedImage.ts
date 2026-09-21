import { useMemo } from "react";
import { useAuthStore } from "@evey/store";

export interface AuthenticatedImageSource {
  uri: string;
  headers: { Authorization: string };
}

/**
 * An image source carrying the session's bearer token.
 *
 * `tokenInQuery` also appends the token to the URL: expo-image on Android
 * does not always forward the Authorization header across a redirect, so the
 * phone asks for it. The television does not - a token in a URL ends up in
 * logs and screenshots, and its image loader keeps the header.
 */
export function useAuthenticatedImage(
  uri: string | undefined,
  tokenInQuery = false,
): AuthenticatedImageSource | undefined {
  const token = useAuthStore((s) => s.accessToken);
  return useMemo(() => {
    if (!uri || !token) return undefined;
    const headers = { Authorization: `Bearer ${token}` };
    if (!tokenInQuery) return { uri, headers };
    const sep = uri.includes("?") ? "&" : "?";
    return { uri: `${uri}${sep}token=${token}`, headers };
  }, [uri, token, tokenInQuery]);
}

export function useAuthHeaders(): { Authorization: string } | undefined {
  const token = useAuthStore((s) => s.accessToken);
  return useMemo(() => {
    if (!token) return undefined;
    return { Authorization: `Bearer ${token}` };
  }, [token]);
}
