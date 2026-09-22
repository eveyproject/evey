// The heartbeat lives in @evey/client (web/usePresenceHeartbeat.ts): the same
// 30s beat for the browser app and the browser television. This component
// keeps the mount point the authenticated layout already has.
import { usePresenceHeartbeat } from "@evey/client/web";

export function PresenceHeartbeat() {
  usePresenceHeartbeat();
  return null;
}
