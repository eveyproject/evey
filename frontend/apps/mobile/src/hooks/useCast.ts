import { useState, useCallback } from "react";

// Cast support requires a dev build with react-native-google-cast.
// This module provides a stub that gracefully degrades in Expo Go.

let GoogleCast: any = null;
try {
  GoogleCast = require("react-native-google-cast");
} catch {
  // Not installed — cast not available
}

export function useCastAvailable(): boolean {
  return GoogleCast != null;
}

export function useCast() {
  const [casting, setCasting] = useState(false);
  const [deviceName, setDeviceName] = useState<string | null>(null);

  const startCast = useCallback(async (mediaUrl: string, title: string, posterUrl?: string) => {
    if (!GoogleCast) return;
    try {
      const client = GoogleCast.default?.getClient?.();
      if (!client) return;
      await client.loadMedia({
        mediaInfo: {
          contentUrl: mediaUrl,
          contentType: "video/mp4",
          metadata: {
            type: "movie",
            title,
            images: posterUrl ? [{ url: posterUrl }] : [],
          },
        },
      });
      setCasting(true);
    } catch {
      setCasting(false);
    }
  }, []);

  const stopCast = useCallback(() => {
    if (!GoogleCast) return;
    try {
      GoogleCast.default?.getClient?.()?.endSession();
    } catch {}
    setCasting(false);
    setDeviceName(null);
  }, []);

  return { casting, deviceName, startCast, stopCast, available: GoogleCast != null };
}
