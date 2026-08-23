// receiverBeacon.ts - la couche transport de la telemetrie du recepteur Cast.
//
// Pourquoi elle est a part de main.ts : main.ts ne peut pas etre teste (il
// touche le global `cast` du CAF des l'import), et c'est precisement cette
// couche qui a echoue en silence deux fois. Le 2026-07-27, un arret PROPRE du
// cast (StopStream serveur a 19:10:37) n'a produit NI cast_receiver_finished NI
// cast_receiver_shutdown ; le 2026-07-28 la mort de la TV n'a laisse aucune
// trace non plus. Un fetch() lance pendant que la plateforme demonte la page
// perd la course : sendBeacon existe pour ca, l'agent utilisateur prend la
// requete en charge et la livre APRES la mort du document.
//
// Le battement periodique repond a l'autre moitie du probleme : une TV qui
// meurt sans rien envoyer reste invisible. Avec un battement, sa DERNIERE
// preuve de vie date sa mort a la demi-minute pres, sans dependre du fait
// qu'elle ait encore le temps de parler.

export interface PlayerEventPayload {
  type: string;
  details?: string;
  fatal?: boolean;
  code?: number;
  variant?: number;
  segment?: number;
  posSec?: number;
}

export interface BeaconTransport {
  // Signature de navigator.sendBeacon : false = l'agent a refuse la charge.
  sendBeacon?: (url: string, body: string) => boolean;
  fetchFn?: (url: string, init: RequestInit) => unknown;
}

export type BeaconOutcome = "beacon" | "fetch" | "dropped";

// Cadence du battement de vie. 30 s tient largement sous la limite de debit du
// serveur (playerEventRL = rafale 20, recharge 30/s) tout en datant une mort a
// la demi-minute pres, et reste discret dans les logs sur un film entier.
export const ALIVE_BEACON_MS = 30_000;

// postPlayerEvent envoie UNE balise. Le corps reste une chaine simple, sans
// en-tete Content-Type : c'est ce qui garde la requete "simple" au sens CORS
// (aucun preflight OPTIONS a gerer cote serveur), aussi bien pour sendBeacon
// que pour le repli fetch. L'auth est le token de flux porte par l'url.
export function postPlayerEvent(
  url: string | null,
  payload: PlayerEventPayload,
  transport: BeaconTransport,
): BeaconOutcome {
  if (!url) return "dropped";
  const body = JSON.stringify({
    type: payload.type,
    details: (payload.details ?? "").slice(0, 200),
    fatal: payload.fatal ?? false,
    code: payload.code ?? 0,
    variant: payload.variant ?? 0,
    segment: payload.segment ?? 0,
    posSec: payload.posSec ?? 0,
  });
  try {
    if (transport.sendBeacon?.(url, body)) return "beacon";
  } catch {
    /* agent sans sendBeacon utilisable : on tente le repli */
  }
  try {
    if (transport.fetchFn) {
      transport.fetchFn(url, { method: "POST", mode: "cors", credentials: "omit", keepalive: true, body });
      return "fetch";
    }
  } catch {
    /* une telemetrie ne casse jamais une lecture */
  }
  return "dropped";
}

export interface AliveHeartbeat {
  start(): void;
  stop(): void;
}

// postPlayerEventQuery envoie une balise via fetch et LIT la reponse : le
// serveur glisse la commande admin en attente ("pause", "play", "stop",
// "message:<txt>") dans le corps du battement alive - c'est ainsi que le
// panneau admin pilote une TV dont le telephone est gele. Null = pas de
// commande (204), erreur reseau, ou pas d'url. Reserve aux battements de VIE :
// les evenements de mort restent sur sendBeacon (postPlayerEvent), qui seul
// survit au demontage de la page.
export async function postPlayerEventQuery(
  url: string | null,
  payload: PlayerEventPayload,
  fetchFn: (url: string, init: RequestInit) => Promise<{ ok: boolean; status: number; json(): Promise<unknown> }>,
): Promise<string | null> {
  if (!url) return null;
  const body = JSON.stringify({
    type: payload.type,
    details: (payload.details ?? "").slice(0, 200),
    fatal: payload.fatal ?? false,
    code: payload.code ?? 0,
    variant: payload.variant ?? 0,
    segment: payload.segment ?? 0,
    posSec: payload.posSec ?? 0,
  });
  try {
    // Corps en chaine simple sans Content-Type : requete "simple" au sens
    // CORS, aucun preflight, comme postPlayerEvent.
    const resp = await fetchFn(url, { method: "POST", mode: "cors", credentials: "omit", body });
    if (!resp.ok || resp.status === 204) return null;
    const data = (await resp.json()) as { command?: unknown };
    return typeof data.command === "string" && data.command !== "" ? data.command : null;
  } catch {
    return null; // une telemetrie ne casse jamais une lecture
  }
}

// readHeapMB reports the JS heap in MB, or -1 when the runtime does not
// expose it. Chrome's non-standard performance.memory is exactly what a cast
// device runs. Motivation (2026-07-28): the receiver died 3-4 minutes into
// every cast with NO event at all - the signature of a platform memory kill.
// Riding the heap size on each alive beacon turns the next ordinary cast into
// the experiment: a heap that climbs toward the death is the proof, a flat
// one is the refutation.
export function readHeapMB(perf?: unknown): number {
  try {
    const p = (perf ?? (typeof performance !== "undefined" ? performance : undefined)) as
      | { memory?: { usedJSHeapSize?: number } }
      | undefined;
    const used = p?.memory?.usedJSHeapSize;
    if (typeof used !== "number" || !isFinite(used) || used < 0) return -1;
    return Math.round(used / (1024 * 1024));
  } catch {
    return -1;
  }
}

// createAliveHeartbeat ne bat PAS immediatement au demarrage : le
// cast_receiver_loaded qui vient de partir est deja la preuve de vie de
// l'instant zero, un battement colle dessus ne dirait rien de plus.
export function createAliveHeartbeat(send: () => void, intervalMs = ALIVE_BEACON_MS): AliveHeartbeat {
  let timer: ReturnType<typeof setInterval> | null = null;
  return {
    start() {
      if (timer !== null) return; // un double start ne doit pas doubler la cadence
      timer = setInterval(send, intervalMs);
    },
    stop() {
      if (timer === null) return;
      clearInterval(timer);
      timer = null;
    },
  };
}

// FrameQuality - ce que le DECODEUR de la box a fait des octets qu'on lui a
// servis, et non ce que le reseau a livre. Cumuls depuis le debut de
// l'element, plus le delta depuis le battement precedent. -1 partout = le
// runtime n'expose rien (aucune conclusion possible, surtout pas "0 perdue").
export interface FrameQuality {
  dropped: number;
  total: number;
  droppedDelta: number;
  totalDelta: number;
}

export interface FrameDropTracker {
  sample(video: unknown): FrameQuality;
}

const BLIND_QUALITY: FrameQuality = { dropped: -1, total: -1, droppedDelta: -1, totalDelta: -1 };

// createFrameDropTracker suit getVideoPlaybackQuality() d'un battement a
// l'autre. Motivation (incident cast du 2026-08-23) : un cast a gele a
// tampon PLEIN, position a 1,000x, 100 % de HTTP 200 - toutes les mesures
// serveur exonerent la livraison, et la seule qui aurait tranche manquait.
// Des frames perdues qui montent pendant que la position avance nomment un
// decodeur qui decroche ; un compteur plat l'exonere.
export function createFrameDropTracker(): FrameDropTracker {
  let lastDropped: number | null = null;
  let lastTotal: number | null = null;
  return {
    sample(video: unknown): FrameQuality {
      let dropped: number;
      let total: number;
      try {
        const q = (video as { getVideoPlaybackQuality?: () => unknown } | null)?.getVideoPlaybackQuality?.() as
          | { droppedVideoFrames?: number; totalVideoFrames?: number }
          | undefined;
        dropped = Number(q?.droppedVideoFrames);
        total = Number(q?.totalVideoFrames);
      } catch {
        return BLIND_QUALITY; // element arrache en plein demontage
      }
      if (!isFinite(dropped) || !isFinite(total) || dropped < 0 || total < 0) return BLIND_QUALITY;
      // Un compteur qui RECULE = nouvel element (load suivant) : la ligne de
      // base repart de zero plutot que de rendre un delta negatif.
      const baseDropped = lastDropped !== null && lastDropped <= dropped ? lastDropped : 0;
      const baseTotal = lastTotal !== null && lastTotal <= total ? lastTotal : 0;
      lastDropped = dropped;
      lastTotal = total;
      return { dropped, total, droppedDelta: dropped - baseDropped, totalDelta: total - baseTotal };
    },
  };
}

// formatFrameQuality - le delta d'abord (c'est lui qui date la souffrance),
// le cumul ensuite. Le champ details du serveur est borne a 200 caracteres.
export function formatFrameQuality(q: FrameQuality): string {
  if (q.total < 0) return "drop=-";
  return `drop=${q.droppedDelta}/${q.totalDelta} dropTot=${q.dropped}`;
}
