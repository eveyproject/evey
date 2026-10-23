import { describe, it, expect } from "vitest";
import { readFileSync } from "fs";
import { resolve } from "path";

// Le service de premier plan du Cast SDK n'est declare par AUCUN AAR Google
// (verifie sur play-services-cast-framework 22.3.1 : son manifeste ne contient
// que MediaIntentReceiver et ReconnectionService). C'est a l'application de le
// declarer, avec son foregroundServiceType, sinon Android refuse de le demarrer
// et la notification media Cast n'existe jamais - l'app est alors gelee des
// qu'elle passe en arriere-plan, le sender meurt, et le recepteur finit par
// etre demonte par la box (incident prod 2026-07-28 18:40 UTC : ReportProgress
// s'arrete net 21 s apres le debut du cast, puis reprend a la reactivation).
// beta.12 avait ajoute POST_NOTIFICATIONS pour un service qui n'etait pas la.
const MANIFEST = resolve(__dirname, "../../android/app/src/main/AndroidManifest.xml");

describe("AndroidManifest - service de premier plan Cast", () => {
  const xml = readFileSync(MANIFEST, "utf8");

  it("declare MediaNotificationService", () => {
    expect(xml).toContain("com.google.android.gms.cast.framework.media.MediaNotificationService");
  });

  it("type mediaPlayback sur ce service (obligatoire des targetSdk 34)", () => {
    const svc = xml.match(/<service[^>]*MediaNotificationService[^>]*\/>/s)?.[0] ?? "";
    expect(svc).toContain('android:foregroundServiceType="mediaPlayback"');
    expect(svc).toContain('android:exported="false"');
  });

  it("demande la permission FOREGROUND_SERVICE_MEDIA_PLAYBACK", () => {
    expect(xml).toContain("android.permission.FOREGROUND_SERVICE_MEDIA_PLAYBACK");
  });

  it("garde POST_NOTIFICATIONS, sans quoi la notification reste invisible", () => {
    expect(xml).toContain("android.permission.POST_NOTIFICATIONS");
  });

  it("declare l'activite du controleur etendu que le SDK reference sans condition", () => {
    // GoogleCastOptionsProvider appelle setExpandedControllerActivityClassName()
    // en dur : sans cette declaration la pile de notification du SDK vise un
    // composant absent du manifeste.
    expect(xml).toContain("com.reactnative.googlecast.RNGCExpandedControllerActivity");
  });
});

// Le verrou CPU est la piece qui manquait a beta.15 : un service de premier
// plan empeche Android de GELER le processus, il n'empeche pas le CPU de se
// suspendre ecran eteint. Sans wakelock les PONG du heartbeat CASTV2 cessent,
// la box ferme la socket sender puis demonte le recepteur - mesure prod du
// 2026-07-28 : mort de la TV 3 min 35 puis 3 min 45 apres le silence du
// telephone. Un client Android Cast tres repandu tient exactement ce verrou ; RNGC 4.9.1 n'en
// tient aucun.
describe("verrou CPU/Wi-Fi de la session Cast", () => {
  const xml = readFileSync(MANIFEST, "utf8");
  const mainApp = readFileSync(
    resolve(__dirname, "../../android/app/src/main/java/com/evey/app/MainApplication.kt"),
    "utf8",
  );
  const keepAlive = readFileSync(
    resolve(__dirname, "../../android/app/src/main/java/com/evey/app/cast/CastKeepAlive.kt"),
    "utf8",
  );

  it("demande WAKE_LOCK et ACCESS_WIFI_STATE", () => {
    expect(xml).toContain("android.permission.WAKE_LOCK");
    expect(xml).toContain("android.permission.ACCESS_WIFI_STATE");
  });

  it("est branche au demarrage de l'application", () => {
    expect(mainApp).toContain("import com.evey.app.cast.CastKeepAlive");
    expect(mainApp).toContain("CastKeepAlive.install(this)");
  });

  it("suit la session Cast, sans passer par le JS qui meurt avec l'ecran", () => {
    expect(keepAlive).toContain("SessionManagerListener<CastSession>");
    expect(keepAlive).toContain("addSessionManagerListener");
    expect(keepAlive).toContain("PARTIAL_WAKE_LOCK");
    // Acquisition au demarrage ET a la reprise de session, liberation sur les
    // trois fins possibles : une session qui finit ne doit pas laisser le CPU
    // retenu, et un echec de demarrage non plus.
    expect(keepAlive).toContain("onSessionStarted");
    expect(keepAlive).toContain("onSessionResumed");
    expect(keepAlive).toContain("onSessionEnded");
    expect(keepAlive).toContain("onSessionStartFailed");
    expect(keepAlive).toContain("onSessionResumeFailed");
  });

  it("n'utilise PAS le mode Wi-Fi LOW_LATENCY, inactif ecran eteint", () => {
    // WIFI_MODE_FULL_LOW_LATENCY n'est actif que si l'app est au premier plan
    // ET l'ecran allume : il serait sans effet dans le seul cas qu'on repare.
    expect(keepAlive).not.toContain("WIFI_MODE_FULL_LOW_LATENCY");
    expect(keepAlive).toContain("WIFI_MODE_FULL_HIGH_PERF");
  });
});
