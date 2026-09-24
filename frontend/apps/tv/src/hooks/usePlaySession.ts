import { Dimensions } from "react-native";
import {
  usePlaySession as useSharedPlaySession,
  type PlaySessionDevice,
  type PlaySessionInitial,
  type PlaySessionResult,
} from "@evey/client/native/playSession";
import { getServerUrl } from "../lib/server-config";
import { probeMaxOutputChannels } from "../lib/outputChannels";
import { probeDisplayHdr } from "../lib/displayHdr";
import { logger } from "../lib/log";

// Shared implementation lives in @evey/client (packages/client/src/native/playSession.ts).
export { buildStreamUrl } from "@evey/client/native/playSession";
export type { PlaySessionInitial, PlaySessionResult } from "@evey/client/native/playSession";

const log = logger("play");

// The panel's short side in pixels: it trims the ladder, never the decision.
function panelHeight(): number {
  const screen = Dimensions.get("screen");
  const scale = screen.scale ?? 1;
  return Math.min(Math.round(screen.height * scale), Math.round(screen.width * scale));
}

const TELEVISION: PlaySessionDevice = {
  id: "evey-tv",
  name: "Evey TV",
  getServerUrl,
  probeMaxOutputChannels,
  // HDR capability of the PANEL, not of the decoder - see lib/displayHdr.
  probeHdr: probeDisplayHdr,
  screenHeight: panelHeight,
  warn: (...args) => log.warn(...args),
};

export function usePlaySession(mediaId: string, initial?: PlaySessionInitial): PlaySessionResult {
  return useSharedPlaySession(mediaId, TELEVISION, initial);
}
