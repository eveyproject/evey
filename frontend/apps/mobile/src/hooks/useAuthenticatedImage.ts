// Shared implementation lives in @evey/client (packages/client/src/hooks/useAuthenticatedImage.ts).
import { useAuthenticatedImage as useSharedAuthenticatedImage } from "@evey/client";

// Posters only (MediaCard, history), and the artwork route is public: the
// token rides the header, never the query. In the query it reached every
// proxy log, and since it changes at each refresh it also changed the poster's
// URL every 15 minutes, so the image cache missed and posters downloaded again.
// The route's redirect to a remote poster needs no auth either.
export function useAuthenticatedImage(uri: string | undefined) {
  return useSharedAuthenticatedImage(uri, false);
}
export { useAuthHeaders } from "@evey/client";
export type { AuthenticatedImageSource } from "@evey/client";
