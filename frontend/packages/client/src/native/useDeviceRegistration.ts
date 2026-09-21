import { useEffect, useRef } from "react";
import { AppState } from "react-native";
import * as SecureStore from "expo-secure-store";
import { useAuthStore } from "@evey/store";
import { clientLogger } from "../log";

const log = clientLogger("device");

// Stable per-install ID. Generated once and persisted so reconnects (app
// resume, network blip) are treated by the server as the SAME device rather
// than piling up duplicates in the user's Play On picker.
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

export interface DeviceRegistrationOptions {
  /** What the server files this client as: "mobile" or "tv". */
  deviceType: "mobile" | "tv";
  /** The label the Play On picker shows, resolved at registration time. */
  deviceName: () => string;
  /** The server to register with; nothing happens until it is known. */
  serverUrl: string | null | undefined;
  /**
   * Close the socket while the app is in the background and reconnect on
   * return. A phone spends most of its time backgrounded: closing lets the
   * server drop the Play On registration promptly rather than waiting out its
   * own liveness timeout for a connection the OS may silently kill anyway. A
   * television stays in the foreground, so it keeps the socket.
   */
  pauseInBackground: boolean;
  /** Open the player on this media - the app owns its router. */
  onLoadAndPlay: (mediaId: string, positionMs?: number) => void;
}

// useDeviceRegistration opens a persistent WebSocket to the server and
// registers this client as a controllable Play On target. A `loadAndPlay`
// command navigates to the player screen; play/pause/seek are intentionally
// NOT relayed - the player instance isn't reachable from this hook (no shared
// player ref/store), so forwarding them would silently no-op deeper in the
// stack. Skipping them here keeps that limitation visible instead of hidden.
//
// Reconnects with capped exponential backoff. No-op when not authenticated
// or the server URL hasn't resolved yet.
export function useDeviceRegistration(options: DeviceRegistrationOptions) {
  const accessToken = useAuthStore((s) => s.accessToken);
  const { serverUrl, deviceType, deviceName, pauseInBackground, onLoadAndPlay } = options;
  // Latest router/token/url/name in a ref so the reconnect loop's closures see
  // fresh values without restarting the WS on every render.
  const stateRef = useRef({ accessToken, serverUrl, deviceName, onLoadAndPlay });
  stateRef.current = { accessToken, serverUrl, deviceName, onLoadAndPlay };

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
        log.warn("WebSocket construct failed:", err);
        scheduleReconnect();
        return;
      }

      ws.onopen = () => {
        attempt = 0;
        ws?.send(JSON.stringify({
          type: "register",
          deviceId,
          name: stateRef.current.deviceName(),
          deviceType,
        }));
      };

      ws.onmessage = (event) => {
        try {
          const cmd = JSON.parse(typeof event.data === "string" ? event.data : "") as DeviceCommand;
          if (cmd.type === "loadAndPlay" && cmd.mediaId) {
            stateRef.current.onLoadAndPlay(
              cmd.mediaId,
              typeof cmd.positionMs === "number" && cmd.positionMs > 0 ? cmd.positionMs : undefined,
            );
          }
          // play / pause / seek: see hook-level doc comment above - not
          // relayed at this layer.
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

    const appStateSub = pauseInBackground
      ? AppState.addEventListener("change", (next) => {
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
        })
      : null;

    void connect();

    return () => {
      cancelled = true;
      appStateSub?.remove();
      teardownSocket();
    };
  }, [accessToken, serverUrl, deviceType, pauseInBackground]);
}
