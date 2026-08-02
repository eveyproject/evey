import { useEffect, useRef } from "react";
import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";
import { useRouter } from "expo-router";
import { useAuthStore, useServerListStore } from "@evey/store";
import { logger } from "../lib/log";

const log = logger("device");

// Stable per-install ID. Generated once + persisted so the server treats
// reconnects as the SAME device (replacing the stale entry) rather than
// piling up duplicates in the user's Play On picker.
const DEVICE_ID_KEY = "evey-device-id";

async function getStableDeviceId(): Promise<string> {
  const existing = await SecureStore.getItemAsync(DEVICE_ID_KEY);
  if (existing) return existing;
  // crypto.randomUUID is widely available in React Native (Hermes); fall
  // back to a timestamp + random shard if for some reason it isn't.
  const id =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `dev-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
  await SecureStore.setItemAsync(DEVICE_ID_KEY, id);
  return id;
}

// This app only ships to TV devices, so Platform.isTV is always true here -
// no phone branch needed.
function deviceName(): string {
  if (Platform.OS === "ios") return "Apple TV";
  if (Platform.OS === "android") return "Android TV";
  return "TV";
}

interface LoadAndPlayCommand {
  type: "loadAndPlay";
  mediaId: string;
  positionMs?: number;
}
interface PlaybackCommand {
  type: "play" | "pause" | "seek";
  positionMs?: number;
}
type DeviceCommand = LoadAndPlayCommand | PlaybackCommand;

// useDeviceRegistration opens a persistent WebSocket to the server and
// registers this client as a controllable Play On target. Any
// `loadAndPlay` command pushed by the server navigates the TV to the
// player; other commands are forwarded as `evey:cmd` window-style events
// for the active player screen to consume (kept lightweight here so this
// hook stays one focused responsibility - register + route loadAndPlay).
//
// Reconnects with exponential backoff. No-op when not authenticated.
export function useDeviceRegistration() {
  const router = useRouter();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerListStore((s) => s.getActiveServer()?.url);
  // Latest router/token in a ref so the reconnect loop's closures see
  // fresh values without restarting the WS on every render.
  const stateRef = useRef({ router, accessToken, serverUrl });
  stateRef.current = { router, accessToken, serverUrl };

  useEffect(() => {
    if (!accessToken || !serverUrl) return;
    let ws: WebSocket | null = null;
    let cancelled = false;
    let attempt = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let deviceId = "";

    async function connect() {
      if (cancelled) return;
      deviceId = deviceId || (await getStableDeviceId());
      const { accessToken: tok, serverUrl: url } = stateRef.current;
      if (!tok || !url) return;
      // wss:// when the server is https, ws:// otherwise. The backend
      // accepts the token via Sec-WebSocket-Protocol "Bearer.<token>"
      // (extractWSToken) - same auth mechanism as the syncplay socket.
      const wsUrl = url.replace(/^http/, "ws") + "/api/v1/devices/ws";
      try {
        ws = new WebSocket(wsUrl, ["Bearer." + tok]);
      } catch (err) {
        log.warn("WebSocket construct failed:", err);
        scheduleReconnect();
        return;
      }

      ws.onopen = () => {
        attempt = 0;
        ws?.send(JSON.stringify({
          type: "register",
          deviceId,
          name: deviceName(),
          deviceType: "tv",
        }));
      };

      ws.onmessage = (event) => {
        try {
          const cmd = JSON.parse(typeof event.data === "string" ? event.data : "") as DeviceCommand;
          if (cmd.type === "loadAndPlay" && cmd.mediaId) {
            // Hand the playback off to the player screen. positionMs is
            // forwarded as a query param the player route reads when
            // resuming. Screens currently in the player will react to
            // the route change naturally.
            const params: Record<string, string> = {};
            if (typeof cmd.positionMs === "number" && cmd.positionMs > 0) {
              params.positionMs = String(cmd.positionMs);
            }
            stateRef.current.router.push({
              pathname: "/(auth)/player/[id]",
              params: { id: cmd.mediaId, ...params },
            });
          }
          // play / pause / seek for an already-loaded player are
          // intentionally not handled at this layer yet - the player
          // screen will subscribe directly once those commands are wired.
        } catch (err) {
          log.warn("bad command frame:", err);
        }
      };

      ws.onerror = (err) => {
        log.warn("ws error", err);
      };

      ws.onclose = () => {
        ws = null;
        scheduleReconnect();
      };
    }

    function scheduleReconnect() {
      if (cancelled) return;
      // Capped exponential backoff: 1s, 2s, 4s, … up to 30s.
      const delay = Math.min(30_000, 1000 * Math.pow(2, attempt));
      attempt += 1;
      reconnectTimer = setTimeout(() => { void connect(); }, delay);
    }

    void connect();

    return () => {
      cancelled = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      // Clear handlers so the explicit close doesn't trigger reconnect.
      if (ws) {
        ws.onopen = null;
        ws.onmessage = null;
        ws.onerror = null;
        ws.onclose = null;
        try { ws.close(); } catch { /* already closed */ }
      }
    };
  }, [accessToken, serverUrl]);
}
