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
});
