import { useState, useCallback, useRef } from "react";
import { eveyFetch } from "@evey/api";

let GoogleCast: any = null;
try {
  GoogleCast = require("react-native-google-cast");
} catch { /* not installed */ }

export interface CastDevice {
  id: string;
  name: string;
  type: "chromecast" | "dlna";
}

interface DLNARenderer {
  id: string;
  name: string;
  location: string;
  controlUrl: string;
}

export function useCast() {
  const [casting, setCasting] = useState(false);
  const [deviceName, setDeviceName] = useState<string | null>(null);
  const activeMode = useRef<"chromecast" | "dlna" | null>(null);
  const activeDlnaId = useRef<string | null>(null);

  const discover = useCallback(async (): Promise<CastDevice[]> => {
    const devices: CastDevice[] = [];

    // Chromecast: use useDevices hook data or discovery manager
    if (GoogleCast) {
      try {
        const mgr = GoogleCast.DiscoveryManager;
        const castDevices = mgr?.getDevices?.() ?? [];
        for (const d of castDevices) {
          devices.push({
            id: `chromecast:${d.deviceId ?? d.id ?? "default"}`,
            name: d.friendlyName ?? d.name ?? "Chromecast",
            type: "chromecast",
          });
        }
      } catch { /* ignore */ }

      // Always offer native cast dialog as option
      if (devices.filter((d) => d.type === "chromecast").length === 0) {
        devices.push({ id: "chromecast:native", name: "Chromecast", type: "chromecast" });
      }
    }

    // DLNA fallback
    try {
      const renderers = (await eveyFetch<DLNARenderer[]>({
        url: "/api/v1/dlna/renderers",
        method: "GET",
      })) ?? [];
      for (const r of renderers) {
        devices.push({ id: `dlna:${r.id}`, name: r.name, type: "dlna" });
      }
    } catch { /* no dlna */ }

    return devices;
  }, []);

  const castTo = useCallback(async (device: CastDevice, mediaUrl: string, title: string) => {
    if (device.type === "chromecast" && GoogleCast) {
      try {
        const CastContext = GoogleCast.default ?? GoogleCast.CastContext ?? GoogleCast;
        // Show native Google Cast dialog — user picks device there
        await CastContext.showCastDialog();

        // Wait a moment for session to establish
        await new Promise((r) => setTimeout(r, 2000));

        const client = await CastContext.getClient?.();
        if (client) {
          const isHls = mediaUrl.includes("m3u8");
          await client.loadMedia({
            mediaInfo: {
              contentUrl: mediaUrl,
              contentType: isHls ? "application/x-mpegurl" : "video/mp4",
              metadata: { type: "movie", title, images: [] },
            },
          });
          activeMode.current = "chromecast";
          setCasting(true);
          setDeviceName(device.name);
          return;
        }
      } catch { /* cast failed */ }
    }

    if (device.type === "dlna") {
      const dlnaId = device.id.replace("dlna:", "");
      try {
        await eveyFetch({
          url: "/api/v1/dlna/play",
          method: "POST",
          data: { rendererId: dlnaId, mediaUrl, title },
        });
        activeMode.current = "dlna";
        activeDlnaId.current = dlnaId;
        setCasting(true);
        setDeviceName(device.name);
      } catch {
        setCasting(false);
      }
    }
  }, []);

  const stopCast = useCallback(async () => {
    if (activeMode.current === "chromecast" && GoogleCast) {
      try {
        const CastContext = GoogleCast.default ?? GoogleCast.CastContext ?? GoogleCast;
        const session = await CastContext.getSessionManager?.()?.getCurrentCastSession?.();
        session?.end?.();
      } catch { /* ignore */ }
    } else if (activeMode.current === "dlna" && activeDlnaId.current) {
      try {
        await eveyFetch({
          url: "/api/v1/dlna/stop",
          method: "POST",
          data: { rendererId: activeDlnaId.current },
        });
      } catch { /* best effort */ }
      activeDlnaId.current = null;
    }
    activeMode.current = null;
    setCasting(false);
    setDeviceName(null);
  }, []);

  return { casting, deviceName, discover, castTo, stopCast };
}
