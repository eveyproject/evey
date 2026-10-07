import { useEffect, useRef } from "react";
import { useAuthStore } from "@evey/store";
import { clientLogger } from "../log";

const log = clientLogger("device");

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
  /**
   * The stable per-install id, from wherever this platform keeps one: the
   * phone and the television use the secure store, the browser television
   * uses localStorage. The server treats a reconnect under the same id as the
   * SAME device instead of piling up duplicates in the Play On picker.
   */
  getDeviceId: () => Promise<string>;
  /**
   * Tells the core when the app goes to the background and comes back. Only
   * consulted when pauseInBackground is on; the caller owns the subscription
   * (AppState on the phone, visibilitychange in a browser).
   */
  subscribeBackground?: (onChange: (backgrounded: boolean) => void) => () => void;
}

/**
 * The Play On registration: one WebSocket that tells the server this client
 * can be told to play something. Reconnects with capped exponential backoff.
 * Nothing happens while signed out or before the server url is known.
 *
 * play / pause / seek are deliberately NOT relayed: the player instance is
 * not reachable from here, so forwarding them would silently do nothing.
 */
export function useDeviceRegistrationCore(options: DeviceRegistrationOptions) {
  const accessToken = useAuthStore((s) => s.accessToken);
  const { serverUrl, deviceType, deviceName, pauseInBackground, onLoadAndPlay, getDeviceId, subscribeBackground } = options;
  // Latest router/token/url/name in a ref so the reconnect loop's closures see
  // fresh values without restarting the WS on every render.
  const stateRef = useRef({ accessToken, serverUrl, deviceName, onLoadAndPlay, getDeviceId, subscribeBackground });
  stateRef.current = { accessToken, serverUrl, deviceName, onLoadAndPlay, getDeviceId, subscribeBackground };

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
      deviceId = deviceId || (await stateRef.current.getDeviceId());
      // The effect may have been torn down while the id was being read
      // (StrictMode's double mount, a sign-out): opening a socket now would
      // leave a zombie nothing closes.
      if (cancelled) return;
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

    // Read through the ref: the caller passes a fresh arrow function each
    // render, and adding it to the effect deps would restart the socket then.
    const unsubscribe = pauseInBackground
      ? stateRef.current.subscribeBackground?.((isBackground) => {
          if (!isBackground) {
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
      : undefined;

    void connect();

    return () => {
      cancelled = true;
      unsubscribe?.();
      teardownSocket();
    };
  }, [accessToken, serverUrl, deviceType, pauseInBackground]);
}
