import { memo, useEffect, useState } from "react";
import {
  eveyFetchRaw,
  findActiveCueIndex,
  findActivePGSCue,
  isPGSBlob,
  parsePGSBlob,
  parseVttCues,
  pgsCueDataURI,
  type PGSTrack,
  type SubtitleCue,
} from "@evey/api";

interface Props {
  mediaId: string;
  // Subtitle-relative ordinal the server addresses (/subtitle/{track}), or
  // null for none. Text and PGS tracks are both fetched and drawn here; a
  // bitmap the player cannot draw is burnt in server side instead.
  subtitleIndex: number | null;
  currentTimeSec: number;
}

// One fetch per (media, track) per app run, like the Android TV overlay.
const textCache = new Map<string, SubtitleCue[]>();
const pgsCache = new Map<string, { track: PGSTrack; uris: string[] }>();

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec }: Props) {
  const [cues, setCues] = useState<SubtitleCue[]>([]);
  const [pgs, setPgs] = useState<{ track: PGSTrack; uris: string[] } | null>(null);

  useEffect(() => {
    if (subtitleIndex === null) {
      setCues([]);
      setPgs(null);
      return;
    }
    const key = `${mediaId}:${subtitleIndex}`;
    const cachedPgs = pgsCache.get(key);
    if (cachedPgs) { setPgs(cachedPgs); setCues([]); return; }
    const cachedText = textCache.get(key);
    if (cachedText) { setCues(cachedText); setPgs(null); return; }
    setCues([]);
    setPgs(null);
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${encodeURIComponent(mediaId)}/subtitle/${subtitleIndex}`, method: "GET" });
        const buf = new Uint8Array(await res.arrayBuffer());
        if (cancelled) return;
        if (isPGSBlob(buf)) {
          const track = parsePGSBlob(buf);
          const entry = { track, uris: track.cues.map(pgsCueDataURI) };
          pgsCache.set(key, entry);
          setPgs(entry);
          return;
        }
        const parsed = parseVttCues(new TextDecoder().decode(buf));
        textCache.set(key, parsed);
        setCues(parsed);
      } catch {
        // No subtitles rather than a broken player.
      }
    })();
    return () => { cancelled = true; };
  }, [mediaId, subtitleIndex]);

  if (pgs) {
    const { track, uris } = pgs;
    const idx = findActivePGSCue(track.cues, currentTimeSec * 1000);
    if (idx === -1 || track.screenW <= 0 || track.screenH <= 0) return null;
    const cue = track.cues[idx];
    // Positioned in percent of the source frame: the picture is letterboxed
    // by object-fit, so the overlay covers the video box, not the screen.
    return (
      <div className="subs-pgs">
        <img
          src={uris[idx]}
          alt=""
          style={{
            left: `${(cue.x / track.screenW) * 100}%`,
            top: `${(cue.y / track.screenH) * 100}%`,
            width: `${(cue.w / track.screenW) * 100}%`,
            height: `${(cue.h / track.screenH) * 100}%`,
          }}
        />
      </div>
    );
  }

  const active = findActiveCueIndex(cues, currentTimeSec);
  if (active === -1) return null;
  return (
    <div className="subs-text">
      {cues[active].text.split("\n").map((line, i) => <span key={i}>{line}</span>)}
    </div>
  );
}

export const SubtitleOverlay = memo(SubtitleOverlayImpl);
