// Shared implementation lives in @evey/client (packages/client/src/hooks/useAuthenticatedImage.ts).
import { useAuthenticatedImage as useSharedAuthenticatedImage } from "@evey/client";

// The phone passes the token in the query too: expo-image on Android does not
// always forward the Authorization header across a redirect.
export function useAuthenticatedImage(uri: string | undefined) {
  return useSharedAuthenticatedImage(uri, true);
}
export { useAuthHeaders } from "@evey/client";
export type { AuthenticatedImageSource } from "@evey/client";
