import { useEffect, useState } from "react";
import { parseHlsMasterResolutions, type RungSize } from "@evey/api";

// The quality rungs this transcode actually offers.
//
// The web reads them off hls.js (MANIFEST_PARSED). expo-video drives HLS
// itself and exposes no level list, so the mobile quality menu shipped a FIXED
// ladder - 1080/720/480/360 - and offered 1080p on a 720p source. The manifest
// says what exists; here it is simply read.
//
// One fetch, of the MASTER only. That request is the same one the shed
// preflight already makes (see usePlaySession): a small text body, idempotent,
// no ffmpeg behind it - unlike /transcode, which spawns one process per GET.
// Direct play has no manifest and no rungs to choose from, so nothing is
// fetched at all.
// Each rung keeps its width: the menu names it like the title page names the file.
export function useHlsRungs(streamUrl: string | null, streamToken?: string | null): RungSize[] {
  const [heights, setHeights] = useState<RungSize[]>([]);

  useEffect(() => {
    if (!streamUrl || !streamUrl.includes("master.m3u8")) {
      setHeights([]);
      return;
    }
    const ctrl = new AbortController();
    let alive = true;
    void (async () => {
      try {
        const res = await fetch(streamUrl, {
          signal: ctrl.signal,
          ...(streamToken ? { headers: { Authorization: `Bearer ${streamToken}` } } : {}),
        });
        if (!res.ok) return;
        const text = await res.text();
        if (alive) setHeights(parseHlsMasterResolutions(text));
      } catch {
        // Offline, aborted, a shed 503: the menu keeps its static ladder,
        // which is exactly what it had before. A quality list is not worth
        // an error on screen.
      }
    })();
    return () => {
      alive = false;
      ctrl.abort();
    };
  }, [streamUrl, streamToken]);

  return heights;
}
