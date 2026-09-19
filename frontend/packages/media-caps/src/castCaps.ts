import type { ObservedReceiverCaps } from "./castUrl";

// The caps handshake between a sender and Evey's custom Cast receiver: what
// rides the channel and how a sender asks. SDK-free on purpose - the web
// sender (Cast Web Sender SDK) and the mobile sender (react-native-google-cast)
// each wrap their own session into a CapsTransport and run the SAME ask, so
// there is one place where the rules live: listener before the first ask,
// the receiver's unprompted READY broadcast accepted, a re-ask every retryMs
// within the timeoutMs budget (a cold app launch answers seconds after the
// session is up), the first valid answer wins, everything torn down after.
//
// The receiver side is apps/web cast-receiver (receiverLogic.ts: capsReport,
// isCapsRequest; main.ts: the listener and the READY broadcast).

export const CAPS_NAMESPACE = "urn:x-cast:org.eveyproject.caps";
export const CAPS_REQUEST = { type: "caps" } as const;
export const CAPS_TIMEOUT_MS = 5000;
export const CAPS_RETRY_MS = 500;

// CapsAnswer is what the receiver observed about itself; userAgent is
// diagnostic only (telemetry), never a negotiation input.
export interface CapsAnswer {
  caps: ObservedReceiverCaps;
  userAgent: string | null;
}

// parseCapsReport validates a raw message (an object, or the JSON string a
// sender SDK hands over) into a CapsAnswer, null for anything that is not a
// well-formed caps report. Optional fields a receiver may omit take the
// conservative value (stereo, level 4.0, no direct H.264 height probe).
export function parseCapsReport(raw: unknown): CapsAnswer | null {
  let data: unknown = raw;
  if (typeof raw === "string") {
    try {
      data = JSON.parse(raw);
    } catch {
      return null;
    }
  }
  if (typeof data !== "object" || data === null) return null;
  const d = data as { type?: unknown; caps?: unknown; userAgent?: unknown };
  if (d.type !== "caps" || typeof d.caps !== "object" || d.caps === null) return null;
  const c = d.caps as Partial<Record<keyof ObservedReceiverCaps, unknown>>;
  if (!Array.isArray(c.videoCodecs) || typeof c.maxHeight !== "number") return null;
  const videoCodecs: unknown[] = c.videoCodecs;
  return {
    caps: {
      videoCodecs: videoCodecs.filter((v): v is string => typeof v === "string"),
      maxHeight: c.maxHeight,
      hdr: c.hdr === true,
      nativeDolby: c.nativeDolby === true,
      maxAudioChannels: typeof c.maxAudioChannels === "number" && c.maxAudioChannels > 0 ? c.maxAudioChannels : 2,
      maxH264Level: typeof c.maxH264Level === "number" ? c.maxH264Level : 40,
      h264MaxHeight: typeof c.h264MaxHeight === "number" ? c.h264MaxHeight : undefined,
    },
    userAgent: typeof d.userAgent === "string" ? d.userAgent : null,
  };
}

// CapsTransport is the one thing a sender SDK must provide: a way to send
// the request on CAPS_NAMESPACE and a way to hear the receiver's messages on
// it. subscribe returns the unsubscribe; a message arrives raw (object or
// string) and is parsed here.
export interface CapsTransport {
  send(request: typeof CAPS_REQUEST): Promise<void>;
  subscribe(onMessage: (raw: unknown) => void): () => void;
}

export interface AskReceiverCapsOptions {
  timeoutMs?: number;
  retryMs?: number;
}

// askReceiverCaps runs the handshake on a transport bound to ONE session and
// resolves with the first valid answer, or null at the end of the budget (a
// receiver that does not speak the channel - an older build, the Default
// Media Receiver - a malformed reply, a send that keeps failing). The
// listener is registered before the first send so an unprompted READY
// broadcast is never missed; the retry timer and the listener are always
// torn down, and nothing is delivered after resolution - a late answer from
// a session the caller has moved on from lands on a dead listener.
export function askReceiverCaps(transport: CapsTransport, opts: AskReceiverCapsOptions = {}): Promise<CapsAnswer | null> {
  const timeoutMs = opts.timeoutMs ?? CAPS_TIMEOUT_MS;
  const retryMs = opts.retryMs ?? CAPS_RETRY_MS;
  return new Promise((resolve) => {
    let done = false;
    let unsubscribe: (() => void) | null = null;
    let retry: ReturnType<typeof setInterval> | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const finish = (value: CapsAnswer | null) => {
      if (done) return;
      done = true;
      if (timer !== null) clearTimeout(timer);
      if (retry !== null) clearInterval(retry);
      try { unsubscribe?.(); } catch { /* session gone */ }
      resolve(value);
    };
    const ask = () => {
      transport.send(CAPS_REQUEST).catch(() => { /* still launching: the next tick asks again */ });
    };
    try {
      unsubscribe = transport.subscribe((raw) => {
        if (done) return;
        const answer = parseCapsReport(raw);
        if (answer) finish(answer);
      });
    } catch {
      finish(null);
      return;
    }
    timer = setTimeout(() => finish(null), timeoutMs);
    retry = setInterval(() => { if (!done) ask(); }, retryMs);
    ask();
  });
}
