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

async function startWasmEngine(): Promise<void> {
  const video = findMediaElement();
  if (!video) {
    fallbackToMuxedAudio("video element introuvable");
    return;
  }
  const token = tokenFromUrl(currentUrl);
  try {
    const resp = await fetch(currentUrl, { mode: "cors" });
    if (!resp.ok) throw new Error(`master ${resp.status}`);
    const audioUri = extractAudioURI(await resp.text());
    if (!audioUri) {
      // Le serveur n'a pas pris le chemin wasm (piste non-Dolby) : le master
      // servi est déjà l'AAC muxé, rien à faire.
      wasmRequested = false;
      return;
    }
    const playlistUrl = new URL(audioUri, currentUrl).toString();
    engine = new WasmAudioEngine(video, playlistUrl, {
      // Résolution ABSOLUE contre le serveur média : le receiver peut être
      // hébergé ailleurs (GitHub Pages, domaine projet). Et credentials omit :
      // le CORS token-auth du serveur ne répond pas Allow-Credentials.
      resolveUrl: (u) => absoluteWithToken(u, playlistUrl, token),
      credentials: "omit",
      onFatal: () => fallbackToMuxedAudio("moteur ac3go"),
    });
    await engine.start();
    showDebug("");
  } catch (err) {
    fallbackToMuxedAudio(String(err));
  }
}

playerManager.setMessageInterceptor(
  cast.framework.messages.MessageType.LOAD,
  (request: any) => {
    destroyEngine();
    showDebug("");
    const media = request.media;
    if (!media) return request;
    const rawUrl: string = media.contentUrl || media.contentId || "";

    const deviceCaps = context.getDeviceCapabilities() ?? {};
    const systemHdr = deviceCaps[cast.framework.system.DeviceCapabilities.IS_HDR_SUPPORTED] === true;
    const caps = probeReceiverCaps(
      (mime, codec, w, h) => context.canDisplayType(mime, codec, w, h),
      systemHdr,
    );
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
    } else {
      currentUrl = rawUrl;
      wasmRequested = false;
    }
    return request;
  },
);

playerManager.addEventListener(cast.framework.events.EventType.PLAYER_LOAD_COMPLETE, () => {
  if (wasmRequested) void startWasmEngine();
});

// Les erreurs du player s'affichent À L'ÉCRAN : c'est le receiver de debug que
// le DMR n'a jamais été. detailedErrorCode est la donnée qui manquait à chaque
// incident cast passé.
playerManager.addEventListener(cast.framework.events.EventType.ERROR, (event: any) => {
  const code = event?.detailedErrorCode ?? "?";
  showDebug(`Erreur lecture (code ${code})`);
});

context.addEventListener(cast.framework.system.EventType.SHUTDOWN, () => destroyEngine());

context.start();
