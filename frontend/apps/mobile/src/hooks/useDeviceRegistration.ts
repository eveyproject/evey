import { useEffect, useRef } from "react";
import { AppState, Platform } from "react-native";
import * as SecureStore from "expo-secure-store";
import Constants from "expo-constants";
import { useRouter } from "expo-router";
import { useAuthStore } from "@evey/store";
import { useServerUrl } from "./useServerUrl";

// Stable per-install ID, mirroring the TV app's approach. Generated once and
// persisted so reconnects (app resume, network blip) are treated by the
// server as the SAME device rather than piling up duplicates in the user's
// Play On picker.
const DEVICE_ID_KEY = "evey-device-id";

async function getStableDeviceId(): Promise<string> {
  const existing = await SecureStore.getItemAsync(DEVICE_ID_KEY);
  if (existing) return existing;
  const id =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `dev-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
  await SecureStore.setItemAsync(DEVICE_ID_KEY, id);
  return id;
}

// Prefer the OS-reported device name (e.g. "Alice's iPhone") so the Play On
// picker can tell a user's own phones apart - falling back to a generic
// label when the platform doesn't expose one (no expo-device dependency
// available in this app; expo-constants already ships deviceName).
function deviceName(): string {
  if (Constants.deviceName) return Constants.deviceName;
  if (Platform.OS === "ios") return "iPhone";
  if (Platform.OS === "android") return "Android Phone";
  return "Mobile";
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
// registers this client as a controllable Play On target, mirroring the TV
// app's hook of the same name. A `loadAndPlay` command navigates to the
// player screen; play/pause/seek are intentionally NOT relayed - unlike TV,
// the mobile player instance isn't reachable from this hook (no shared
// player ref/store), so forwarding them would silently no-op deeper in the
// stack. Skipping them here keeps that limitation visible instead of hidden.
//
// Reconnects with capped exponential backoff. No-op when not authenticated
// or the server URL hasn't resolved yet.
export function useDeviceRegistration() {
  const router = useRouter();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  // Latest router/token/url in a ref so the reconnect loop's closures see
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
    let backgrounded = false;

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
        console.warn("[device] WebSocket construct failed:", err);
        scheduleReconnect();
        return;
      }

      ws.onopen = () => {
        attempt = 0;
        ws?.send(JSON.stringify({
          type: "register",
          deviceId,
          name: deviceName(),
          deviceType: "mobile",
        }));
      };

      ws.onmessage = (event) => {
        try {
          const cmd = JSON.parse(typeof event.data === "string" ? event.data : "") as DeviceCommand;
          if (cmd.type === "loadAndPlay" && cmd.mediaId) {
            // positionMs is forwarded as a query param for forward
            // compatibility with the TV contract; the mobile player screen
            // currently derives its resume position from the server-side
            // watch progress on its own, so this param is unused today.
            const params: Record<string, string> = {};
            if (typeof cmd.positionMs === "number" && cmd.positionMs > 0) {
              params.positionMs = String(cmd.positionMs);
            }
            stateRef.current.router.push({
              pathname: "/(auth)/player/[id]",
              params: { id: cmd.mediaId, ...params },
            });
          }
          // play / pause / seek: see hook-level doc comment above - not
          // relayed at this layer.
        } catch (err) {
          console.warn("[device] bad command frame:", err);
        }
      };

      ws.onerror = (err) => {
        console.warn("[device] ws error", err);
      };

      ws.onclose = () => {
        ws = null;
        scheduleReconnect();
      };
    }

    function scheduleReconnect() {
      if (cancelled || backgrounded) return;
      // Capped exponential backoff: 1s, 2s, 4s, … up to 30s.
      const delay = Math.min(30_000, 1000 * Math.pow(2, attempt));
      attempt += 1;
      reconnectTimer = setTimeout(() => { void connect(); }, delay);
    }

    // Clear handlers before close() so the deliberate teardown below never
    // trips onclose's own reconnect scheduling.
    function teardownSocket() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      if (ws) {
        ws.onopen = null;
        ws.onmessage = null;
        ws.onerror = null;
        ws.onclose = null;
        try { ws.close(); } catch { /* already closed */ }
        ws = null;
      }
    }

    // Unlike TV, a phone spends most of its time backgrounded. Closing the
    // socket on background instead of leaving it dangling lets the server
    // drop the Play On registration promptly rather than waiting out its own
    // liveness timeout for a connection the OS may silently kill anyway.
    const appStateSub = AppState.addEventListener("change", (next) => {
      if (next === "active") {
        if (backgrounded) {
          backgrounded = false;
          attempt = 0;
          void connect();
        }
      } else if (!backgrounded) {
        backgrounded = true;
        teardownSocket();
      }
    });

    void connect();

    return () => {
      cancelled = true;
      appStateSub.remove();
      teardownSocket();
    };
  }, [accessToken, serverUrl]);
}
