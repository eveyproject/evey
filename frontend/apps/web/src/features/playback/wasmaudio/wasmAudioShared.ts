// Pure helpers for the wasm-audio path, shared between the web player (React
// hook) and the Cast Web Receiver (standalone page): keeping them here keeps
// the receiver bundle from pulling in React + hls.js via useWasmAudioPlayback.

// extractAudioURI pulls the out-of-band AC-3/E-AC-3 rendition URI the server
// named in the video-only master (#EVEY-AUDIO-URI:"..."). Absent means the
// server did NOT take the wasm-audio path for this request (e.g. it fell back to
// the muxed AAC master), so the caller must play normally instead of muting the
// video.
export function extractAudioURI(masterText: string): string | null {
  for (const raw of masterText.split("\n")) {
    const line = raw.trim();
    if (line.startsWith("#EVEY-AUDIO-URI:")) {
      const v = line.slice("#EVEY-AUDIO-URI:".length).trim();
      // The value is a quoted-string per our emitter (fmt %q).
      const m = v.match(/^"(.*)"$/);
      return m ? m[1] : v;
    }
  }
  return null;
}

// extractAacAudioURI pulls the AAC relay rendition URI (#EVEY-AUDIO-AAC-URI)
// out of the video-only master. Absent on servers whose audio-rendition
// feature is off, or predating the relay: the engine then simply waits for the
// wasm leg, the pre-relay behaviour.
export function extractAacAudioURI(masterText: string): string | null {
  for (const raw of masterText.split("\n")) {
    const line = raw.trim();
    if (line.startsWith("#EVEY-AUDIO-AAC-URI:")) {
      const v = line.slice("#EVEY-AUDIO-AAC-URI:".length).trim();
      const m = v.match(/^"(.*)"$/);
      return m ? m[1] : v;
    }
  }
  return null;
}

// injectToken replaces (or adds) the token query param on a URL with the current
// value, mirroring hls.js's xhrSetup so a long buffer-ahead can't 401 on a token
// that has since rotated. Segment URIs the audio playlist lists carry a baked
// token; this overrides it with the freshest one on every fetch.
export function injectToken(url: string, token: string): string {
  if (!token) return url;
  try {
    const u = new URL(url, window.location.origin);
    u.searchParams.set("token", token);
    return u.pathname + "?" + u.searchParams.toString();
  } catch {
    return url;
  }
}
