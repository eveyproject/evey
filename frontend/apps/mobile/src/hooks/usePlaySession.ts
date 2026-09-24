import * as Network from "expo-network";
import type { V1StreamInfo } from "@evey/api";
import {
  buildStreamUrl as buildSharedStreamUrl,
  usePlaySession as useSharedPlaySession,
  type StreamUrlOptions,
  type PlaySessionDevice,
  type PlaySessionInitial,
  type PlaySessionResult,
} from "@evey/client/native/playSession";
import { getServerUrl } from "../lib/server-config";
import { getDeviceHdrSupport } from "../lib/deviceCodecs";
import { probeMaxOutputChannels } from "../lib/outputChannels";

// Shared implementation lives in @evey/client (packages/client/src/native/playSession.ts).
export { fetchStreamToken } from "@evey/client/native/playSession";
export type { PlaySessionInitial, PlaySessionResult } from "@evey/client/native/playSession";

// The phone's own HDR answer unless the caller gives one: the guest page
// builds its URL here too and always carried it.
export function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
  opts: StreamUrlOptions = {},
): string {
  return buildSharedStreamUrl(serverUrl, mediaId, streamInfo, token, { supportsHdr: getDeviceHdrSupport(), ...opts });
}

// Anything that isn't explicitly cellular is reported as "wifi": wired and
// unknown don't deserve the profile's cellular cap, and the API has two values.
async function detectNetworkType(): Promise<"wifi" | "cellular"> {
  try {
    const state = await Network.getNetworkStateAsync();
    return state.type === Network.NetworkStateType.CELLULAR ? "cellular" : "wifi";
  } catch {
    return "wifi";
  }
}

// No screenHeight: the phone never trimmed the ladder by its screen.
const PHONE: PlaySessionDevice = {
  id: "evey-mobile",
  name: "Evey Mobile",
  getServerUrl,
  probeMaxOutputChannels,
  probeHdr: getDeviceHdrSupport,
  networkType: detectNetworkType,
};

export function usePlaySession(mediaId: string, initial?: PlaySessionInitial): PlaySessionResult {
  const session = useSharedPlaySession(mediaId, PHONE, initial);
  // The phone's player has no sign-in-again screen: an expired session keeps
  // showing as the stream error it showed before the hook was shared.
  return session.isAuthError ? { ...session, isError: true } : session;
}
