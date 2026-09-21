// Shared implementation lives in @evey/client (packages/client/src/native/server-config.ts).
// This module keeps the path the screens and their tests import.
export { getServerUrl, resolveServerUrl, setServerUrl, clearServerUrl, pingServer, probeServer, probeServerHealth } from "@evey/client/native/server-config";
export type { ServerProbe } from "@evey/client/native/server-config";
