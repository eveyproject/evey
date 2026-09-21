// Shared implementation lives in @evey/client (packages/client/src/web/codec.ts).
// This module keeps the path the callers and their tests import.
export { canPlayHevc, canDisplayHdr, canWasmDecodeAC3, isWasmDecodableAudioCodec, getBrowserCapabilities } from "@evey/client/web/codec";
export type { BrowserCapabilities } from "@evey/client/web/codec";
