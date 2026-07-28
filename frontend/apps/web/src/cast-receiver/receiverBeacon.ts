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
