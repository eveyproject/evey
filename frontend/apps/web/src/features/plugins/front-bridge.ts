// Shared implementation lives in @evey/client (packages/client/src/plugins/bridgeScript.ts):
// one guest script for the iframe and the two WebViews. This module keeps the
// path PluginIframe and its tests import.
export { BRIDGE_SCRIPT } from "@evey/client";
export type { EveyBridge } from "@evey/client";
