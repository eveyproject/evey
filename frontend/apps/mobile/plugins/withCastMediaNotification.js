const { withAndroidManifest } = require("expo/config-plugins");

// Le Cast SDK reference MediaNotificationService (c'est lui qui porte la
// notification media, donc le service de premier plan qui empeche Android de
// geler le sender) mais ne le declare dans AUCUN de ses AAR : verifie sur
// play-services-cast-framework 22.3.1, dont le manifeste ne contient que
// MediaIntentReceiver et ReconnectionService. Sans declaration cote app, le
// service ne peut pas demarrer, l'app est gelee des qu'elle passe en
// arriere-plan, et le recepteur finit demonte par la box faute de sender.
// Depuis targetSdk 34 le type mediaPlayback et sa permission sont exiges.
const SERVICE = "com.google.android.gms.cast.framework.media.MediaNotificationService";
const PERMISSION = "android.permission.FOREGROUND_SERVICE_MEDIA_PLAYBACK";

module.exports = function withCastMediaNotification(config) {
  return withAndroidManifest(config, (cfg) => {
    const manifest = cfg.modResults.manifest;
    manifest["uses-permission"] = manifest["uses-permission"] ?? [];
    const has = manifest["uses-permission"].some((p) => p.$?.["android:name"] === PERMISSION);
    if (!has) manifest["uses-permission"].push({ $: { "android:name": PERMISSION } });

    const app = manifest.application?.[0];
    if (!app) return cfg;
    app.service = app.service ?? [];
    const declared = app.service.some((s) => s.$?.["android:name"] === SERVICE);
    if (!declared) {
      app.service.push({
        $: {
          "android:name": SERVICE,
          "android:exported": "false",
          "android:foregroundServiceType": "mediaPlayback",
        },
      });
    }
    return cfg;
  });
};
