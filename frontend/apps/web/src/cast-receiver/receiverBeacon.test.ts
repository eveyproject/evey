import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ALIVE_BEACON_MS, createAliveHeartbeat, postPlayerEvent, postPlayerEventQuery, readHeapMB } from "./receiverBeacon";

// Pourquoi ces tests existent : la telemetrie posee le 2026-07-26 devait nommer
// la cause des morts de cast cote TV. Elle ne l'a JAMAIS fait, deux fois de
// suite - le 07-27 un arret PROPRE (StopStream 19:10:37) n'a produit ni
// cast_receiver_finished ni cast_receiver_shutdown, et le 07-28 la mort de la
// TV n'a laisse aucune trace. Un fetch() perd la course au demontage de la page
// receveur : sendBeacon est justement concu pour survivre a ce demontage. Et
// sans battement periodique, une TV qui meurt sans rien dire reste invisible :
// la derniere preuve de vie devient l'horodatage de sa mort.

describe("postPlayerEvent - le beacon doit survivre au demontage de la page", () => {
  it("prefere sendBeacon quand il est disponible", () => {
    const sendBeacon = vi.fn(() => true);
    const fetchFn = vi.fn();
    const how = postPlayerEvent("https://srv/api/v1/media/m1/player-event?token=t", { type: "cast_receiver_shutdown" }, { sendBeacon, fetchFn });
    expect(how).toBe("beacon");
    expect(sendBeacon).toHaveBeenCalledTimes(1);
    expect(fetchFn).not.toHaveBeenCalled();
    const [url, body] = sendBeacon.mock.calls[0] as unknown as [string, string];
    expect(url).toContain("/player-event?token=t");
    expect(JSON.parse(body).type).toBe("cast_receiver_shutdown");
  });

  it("retombe sur fetch keepalive si sendBeacon refuse la charge", () => {
    const sendBeacon = vi.fn(() => false); // file pleine : la spec renvoie false
    const fetchFn = vi.fn(() => Promise.resolve());
    const how = postPlayerEvent("https://srv/api/v1/media/m1/player-event?token=t", { type: "cast_receiver_error" }, { sendBeacon, fetchFn });
    expect(how).toBe("fetch");
    expect(fetchFn).toHaveBeenCalledTimes(1);
    const init = fetchFn.mock.calls[0][1] as RequestInit;
    expect(init.keepalive).toBe(true);
    expect(init.credentials).toBe("omit");
    expect(init.method).toBe("POST");
  });

  it("retombe sur fetch quand sendBeacon n'existe pas", () => {
    const fetchFn = vi.fn(() => Promise.resolve());
    expect(postPlayerEvent("https://srv/x/player-event", { type: "t" }, { fetchFn })).toBe("fetch");
  });

  it("ne jette jamais : une telemetrie ne casse pas une lecture", () => {
    const sendBeacon = vi.fn(() => { throw new Error("boom"); });
    const fetchFn = vi.fn(() => { throw new Error("boom"); });
    expect(() => postPlayerEvent("https://srv/x/player-event", { type: "t" }, { sendBeacon, fetchFn })).not.toThrow();
    expect(postPlayerEvent("https://srv/x/player-event", { type: "t" }, { sendBeacon, fetchFn })).toBe("dropped");
  });

  it("sans url, rien n'est envoye", () => {
    const sendBeacon = vi.fn(() => true);
    expect(postPlayerEvent(null, { type: "t" }, { sendBeacon })).toBe("dropped");
    expect(sendBeacon).not.toHaveBeenCalled();
  });
});

describe("createAliveHeartbeat - la derniere preuve de vie date la mort de la TV", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("bat a la cadence annoncee tant qu'il tourne", () => {
    const send = vi.fn();
    const hb = createAliveHeartbeat(send);
    hb.start();
    expect(send).not.toHaveBeenCalled(); // pas de battement immediat : le LOAD vient d'en emettre un
    vi.advanceTimersByTime(ALIVE_BEACON_MS * 3);
    expect(send).toHaveBeenCalledTimes(3);
  });

  it("stop arrete net", () => {
    const send = vi.fn();
    const hb = createAliveHeartbeat(send);
    hb.start();
    vi.advanceTimersByTime(ALIVE_BEACON_MS);
    hb.stop();
    vi.advanceTimersByTime(ALIVE_BEACON_MS * 5);
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("un double start ne double pas la cadence", () => {
    const send = vi.fn();
    const hb = createAliveHeartbeat(send);
    hb.start();
    hb.start();
    vi.advanceTimersByTime(ALIVE_BEACON_MS * 2);
    expect(send).toHaveBeenCalledTimes(2);
  });

  it("reste sous la limite de debit du serveur (rafale 20, recharge 30/s)", () => {
    // playerEventRL = NewWithBurst(20, 30.0) cote serveur : un battement toutes
    // les 30 s laisse la rafale intacte meme sur un film entier.
    expect(ALIVE_BEACON_MS).toBeGreaterThanOrEqual(30_000);
  });
});

describe("readHeapMB - la taille du tas JS voyage sur chaque battement", () => {
  it("convertit usedJSHeapSize en MB entiers", () => {
    expect(readHeapMB({ memory: { usedJSHeapSize: 150 * 1024 * 1024 } })).toBe(150);
  });

  it("renvoie -1 quand le runtime n'expose pas performance.memory", () => {
    expect(readHeapMB({})).toBe(-1);
    expect(readHeapMB({ memory: {} })).toBe(-1);
    expect(readHeapMB({ memory: { usedJSHeapSize: Number.NaN } })).toBe(-1);
    expect(readHeapMB({ memory: { usedJSHeapSize: -5 } })).toBe(-1);
  });

  it("ne jette jamais, meme sur un objet hostile", () => {
    const trap = new Proxy({}, { get() { throw new Error("boom"); } });
    expect(readHeapMB(trap)).toBe(-1);
  });
});

describe("postPlayerEventQuery", () => {
  const mkResp = (status: number, body?: unknown) => ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  });

  it("returns the pending admin command carried by the response", async () => {
    const cmd = await postPlayerEventQuery(
      "https://srv/api/v1/media/m1/player-event?token=t",
      { type: "cast_receiver_alive", posSec: 42 },
      async () => mkResp(200, { command: "pause" }),
    );
    expect(cmd).toBe("pause");
  });

  it("returns null on the bare 204 (no pending command)", async () => {
    const cmd = await postPlayerEventQuery("https://srv/e", { type: "cast_receiver_alive" }, async () => mkResp(204));
    expect(cmd).toBeNull();
  });

  it("swallows network errors and malformed bodies - telemetry never breaks playback", async () => {
    expect(await postPlayerEventQuery("https://srv/e", { type: "cast_receiver_alive" }, async () => {
      throw new Error("net down");
    })).toBeNull();
    expect(await postPlayerEventQuery("https://srv/e", { type: "cast_receiver_alive" }, async () => mkResp(200, { command: 7 }))).toBeNull();
    expect(await postPlayerEventQuery(null, { type: "cast_receiver_alive" }, async () => mkResp(200, { command: "pause" }))).toBeNull();
  });

  it("keeps the CORS-simple shape: string body, no Content-Type header", async () => {
    let captured: RequestInit | undefined;
    await postPlayerEventQuery("https://srv/e", { type: "cast_receiver_alive" }, async (_u, init) => {
      captured = init;
      return mkResp(204);
    });
    expect(typeof captured?.body).toBe("string");
    expect(captured?.headers).toBeUndefined();
    expect(captured?.credentials).toBe("omit");
  });
});
