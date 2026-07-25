// Point d'entrée du Web Receiver Cast custom (page /cast-receiver.html,
// déclarée dans la Google Cast Console). Rôles :
//   1. re-négocier le master HLS avec les capacités RÉELLES du device
//      (probeReceiverCaps + rewriteCastMasterUrl - fini la table par modèle) ;
//   2. décoder l'AC-3/E-AC-3 SUR la TV via ac3go wasm quand le runtime cast ne
//      le fait pas nativement (master video-only + WasmAudioEngine, le même
//      moteur que le player web) ; repli automatique sur l'AAC transcodé si le
//      wasm ne démarre pas ;
//   3. afficher les erreurs de lecture À L'ÉCRAN - le Default Media Receiver
//      échoue en silence, c'est ce qui a rendu les incidents cast indiagnosticables.
import { WasmAudioEngine } from "@/features/playback/wasmaudio/WasmAudioEngine";
import { extractAudioURI } from "@/features/playback/wasmaudio/wasmAudioShared";
import { absoluteWithToken, probeReceiverCaps, rewriteCastMasterUrl, stripWasmAudio } from "./receiverLogic";

// Le SDK CAF est chargé par un <script> gstatic dans la page ; ses types ne
// sont pas packagés - accès volontairement non typé, borné à ce fichier.
/* eslint-disable @typescript-eslint/no-explicit-any */
declare const cast: any;

const context = cast.framework.CastReceiverContext.getInstance();
const playerManager = context.getPlayerManager();

const dbg = document.getElementById("dbg");
function showDebug(text: string): void {
  if (!dbg) return;
  dbg.textContent = text;
  dbg.style.display = text ? "block" : "none";
}

let engine: WasmAudioEngine | null = null;
let wasmDisabled = false;
let currentUrl = "";
let wasmRequested = false;
// Génération de LOAD : un nouveau LOAD invalide tout démarrage de moteur en
// cours (le start est asynchrone et parallèle au chargement CAF).
let loadGen = 0;
let engineStarting = false;

// Préchauffage : télécharger (cache HTTP) et COMPILER (cache de code V8, clé =
// URL, exploité par l'instantiateStreaming du worker) le décodeur pendant le
// splash - sur le CPU d'une TV, la compilation Go-wasm est le coût dominant du
// délai d'arrivée du son. Best-effort intégral : un échec ici ne change rien,
// le vrai chargement le repayera.
for (const asset of ["/wasm/ac3Worker.js", "/wasm/wasm_exec.js"]) {
  void fetch(asset).catch(() => {});
}
try {
  void WebAssembly.compileStreaming(fetch("/wasm/ac3go.wasm")).catch(() => {});
} catch {
  /* runtime sans compileStreaming : le repli bytes du worker s'en charge */
}

function destroyEngine(): void {
  engine?.destroy();
  engine = null;
}

// tokenFromUrl lit le token purpose=cast que le sender a baké dans l'URL - le
// receiver n'a aucune autre identité, ce token est TOUTE son autorisation.
function tokenFromUrl(rawUrl: string): string {
  try {
    return new URL(rawUrl).searchParams.get("token") ?? "";
  } catch {
    return "";
  }
}

// findMediaElement va chercher le <video> que le player CAF rend dans le
// shadow DOM de <cast-media-player>. Pas d'API publique pour ça : si Google
// referme le shadow root un jour, le retour null déclenche simplement le repli
// AAC - jamais un écran noir muet.
function findMediaElement(): HTMLVideoElement | null {
  const cmp = document.querySelector("cast-media-player");
  const inShadow = cmp?.shadowRoot?.querySelector("video");
  if (inShadow) return inShadow as HTMLVideoElement;
  return document.querySelector("video");
}

// Repli : recharger le MÊME contenu sans wasmAudio - le serveur re-sert le
// master AAC muxé. wasmDisabled est un latch : on ne retente pas en boucle.
function fallbackToMuxedAudio(reason: string): void {
  if (!wasmRequested) return;
  wasmRequested = false;
  wasmDisabled = true;
  destroyEngine();
  showDebug(`wasm audio indisponible (${reason}) - repli AAC`);
  const url = stripWasmAudio(currentUrl);
  const request = new cast.framework.messages.LoadRequestData();
  request.media = new cast.framework.messages.MediaInformation();
  request.media.contentId = url;
  request.media.contentUrl = url;
  request.media.contentType = "application/x-mpegurl";
  request.media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
  request.media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
  request.currentTime = playerManager.getCurrentTimeSec() || 0;
  request.autoplay = true;
  playerManager.load(request).catch((e: unknown) => showDebug(`repli AAC impossible: ${String(e)}`));
}

// L'élément <video> de CAF n'existe qu'une fois le premier LOAD en cours de
// traitement : au démarrage anticipé (dès l'interception), on le guette.
async function findMediaElementWithRetry(timeoutMs: number): Promise<HTMLVideoElement | null> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const v = findMediaElement();
    if (v) return v;
    if (Date.now() > deadline) return null;
    await new Promise((r) => setTimeout(r, 250));
  }
}

// Démarré dès l'INTERCEPTION du LOAD (pas à PLAYER_LOAD_COMPLETE) : le fetch du
// master, l'attente du <video> et le boot du worker wasm courent en parallèle
// du chargement CAF - c'est ce qui absorbe les secondes de silence au départ.
// Sans risque de désynchro : le moteur ne joue que lorsque la vidéo AVANCE
// (stall gate côté engine), démarrer tôt ne fait que préparer.
async function startWasmEngine(): Promise<void> {
  if (engine || engineStarting) return;
  engineStarting = true;
  const gen = loadGen;
  const url = currentUrl;
  const token = tokenFromUrl(url);
  try {
    const [video, masterText] = await Promise.all([
      findMediaElementWithRetry(10_000),
      fetch(url, { mode: "cors" }).then((r) => {
        if (!r.ok) throw new Error(`master ${r.status}`);
        return r.text();
      }),
    ]);
    if (gen !== loadGen) return; // un autre LOAD est passé entre-temps
    if (!video) {
      fallbackToMuxedAudio("video element introuvable");
      return;
    }
    const audioUri = extractAudioURI(masterText);
    if (!audioUri) {
      // Le serveur n'a pas pris le chemin wasm (piste non-Dolby) : le master
      // servi est déjà l'AAC muxé, rien à faire.
      wasmRequested = false;
      return;
    }
    const playlistUrl = new URL(audioUri, url).toString();
    engine = new WasmAudioEngine(video, playlistUrl, {
      // Résolution ABSOLUE contre le serveur média : le receiver peut être
      // hébergé ailleurs (GitHub Pages, domaine projet). Et credentials omit :
      // le CORS token-auth du serveur ne répond pas Allow-Credentials.
      resolveUrl: (u) => absoluteWithToken(u, playlistUrl, token),
      credentials: "omit",
      // Compensation d'avance audio : le pipeline d'affichage d'une TV retarde
      // l'image bien plus que le 0,1 s desktop du moteur - mesuré ~0,5-0,6 s
      // d'avance audio sur la Mi TV (2026-07-25). Valeur à calibrer sur retour
      // utilisateur : augmenter si le son reste en avance, diminuer s'il est
      // passé en retard.
      leadCompensationSec: 0.65,
      onFatal: () => fallbackToMuxedAudio("moteur ac3go"),
    });
    await engine.start();
    showDebug("");
  } catch (err) {
    if (gen === loadGen) fallbackToMuxedAudio(String(err));
  } finally {
    engineStarting = false;
  }
}

playerManager.setMessageInterceptor(
  cast.framework.messages.MessageType.LOAD,
  (request: any) => {
    try {
      return interceptLoad(request);
    } catch (err) {
      // Une exception ici ferait échouer le LOAD en silence (905 sans réseau).
      // On l'affiche À L'ÉCRAN et on laisse passer la requête INTACTE : l'URL
      // du sender mobile est déjà négociée AAC - le chemin qui marche.
      showDebug(`interceptor: ${String(err).slice(0, 160)}`);
      wasmRequested = false;
      currentUrl = request?.media?.contentUrl || request?.media?.contentId || "";
      return request;
    }
  },
);

function interceptLoad(request: any): any {
    loadGen++;
    destroyEngine();
    showDebug("");
    const media = request.media;
    if (!media) return request;
    const rawUrl: string = media.contentUrl || media.contentId || "";

    const deviceCaps = context.getDeviceCapabilities() ?? {};
    const systemHdr = deviceCaps[cast.framework.system.DeviceCapabilities.IS_HDR_SUPPORTED] === true;
    const probed = probeReceiverCaps(
      (mime, codec, w, h) => context.canDisplayType(mime, codec, w, h),
      systemHdr,
    );
    // v1 : ZÉRO passthrough Dolby, même si la sonde dit oui - position Jellyfin
    // (hasEAC3Support hardcodé false « problems getting it to work with HLS »),
    // confirmée le 25/07 au soir : la Mi TV se sonde Dolby-capable via EDID et
    // le player CAF échoue quand même le LOAD (code 905) sur le master ec-3.
    // L'audio passe donc par ac3go wasm (https) ou le transcode AAC serveur.
    const caps = { ...probed, nativeDolby: false };
    const wasmCapable = typeof WebAssembly !== "undefined" && !wasmDisabled;
    // window.location.protocol : une instance http: sur le LAN n'a pas droit
    // aux fetch JS (mixed content) - la décision wasm en tient compte.
    const rewritten = rewriteCastMasterUrl(rawUrl, caps, wasmCapable, window.location.protocol);
    if (rewritten) {
      media.contentId = rewritten.url;
      media.contentUrl = rewritten.url;
      media.contentType = "application/x-mpegurl";
      // fMP4 explicite : sans ces hints le receiver assume MPEG2-TS et charge
      // pour toujours sans image (même fix que le sender web/mobile).
      media.hlsSegmentFormat = cast.framework.messages.HlsSegmentFormat.FMP4;
      media.hlsVideoSegmentFormat = cast.framework.messages.HlsVideoSegmentFormat.FMP4;
      currentUrl = rewritten.url;
      wasmRequested = rewritten.wasmAudio;
      // Départ ANTICIPÉ du moteur audio, en parallèle du chargement CAF.
      if (wasmRequested) void startWasmEngine();
    } else {
      currentUrl = rawUrl;
      wasmRequested = false;
    }
    return request;
}

// Filet : si le démarrage anticipé n'a pas abouti (élément vidéo tardif...),
// on retente une fois le chargement CAF terminé - l'ancien déclencheur.
playerManager.addEventListener(cast.framework.events.EventType.PLAYER_LOAD_COMPLETE, () => {
  if (wasmRequested && !engine) void startWasmEngine();
});

// Les erreurs du player s'affichent À L'ÉCRAN : c'est le receiver de debug que
// le DMR n'a jamais été. detailedErrorCode + la raison brute + la décision de
// négociation : tout ce qu'il faut pour diagnostiquer sur une photo de TV.
playerManager.addEventListener(cast.framework.events.EventType.ERROR, (event: any) => {
  const code = event?.detailedErrorCode ?? "?";
  let extra = "";
  try {
    extra = JSON.stringify(event?.error ?? event?.reason ?? {}).slice(0, 140);
  } catch { /* l'écran reste le canal : jamais d'exception ici */ }
  const mode = wasmRequested ? "wasm" : "aac";
  const host = (() => { try { return new URL(currentUrl).host; } catch { return "?"; } })();
  showDebug(`Erreur lecture code=${code} mode=${mode} src=${host} ${extra}`);
});

context.addEventListener(cast.framework.system.EventType.SHUTDOWN, () => destroyEngine());

context.start();
