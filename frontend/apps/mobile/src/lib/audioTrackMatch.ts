// Shared implementation lives in @evey/client (packages/client/src/hooks/useFileAudioSelection.ts).
// This module keeps the path the screens and their tests import.
export { pickNativeAudioTrack, nativeTrackToApply, type NativeAudioTrack } from "@evey/client";

// The profile-language pick is the one every player uses (@evey/api).
export { pickPreferredStreamIndex } from "@evey/api";
