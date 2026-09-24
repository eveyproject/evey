// Shared implementation lives in @evey/client (packages/client/src/hooks/useSyncplay.ts);
// this module gives it the television's server URL.
import { useSyncplay as useSharedSyncplay, type UseSyncplayOptions } from "@evey/client";
import { useServerUrl } from "./useServerUrl";

export { joinSyncplayRoomByCode } from "@evey/client";
export type { SyncplayJoinResponse, SyncplayMember, SyncplayState } from "@evey/client";

export function useSyncplay(opts: Omit<UseSyncplayOptions, "serverUrl">) {
  const serverUrl = useServerUrl();
  return useSharedSyncplay({ ...opts, serverUrl });
}
