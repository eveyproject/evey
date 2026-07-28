const { withAndroidManifest, withMainApplication } = require("expo/config-plugins");

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
// Verrous tenus par cast/CastKeepAlive.kt pendant une session Cast : le
// service de premier plan empeche le gel du processus, pas la suspension du
// CPU ecran eteint - sans wakelock les PONG du heartbeat CASTV2 cessent et la
// box demonte le recepteur.
const KEEPALIVE_PERMISSIONS = [
  "android.permission.WAKE_LOCK",
  "android.permission.ACCESS_WIFI_STATE",
];

// L'activite du controleur etendu : GoogleCastOptionsProvider appelle
// setExpandedControllerActivityClassName() sans condition, alors que le plugin
// du paquet ne la declare que si on lui passe expandedController: true. La
// notification media du SDK visait donc un composant absent du manifeste.
const EXPANDED_CONTROLLER = "com.reactnative.googlecast.RNGCExpandedControllerActivity";

// Le verrou CPU/Wi-Fi de la session Cast vit en natif (cast/CastKeepAlive.kt)
// et doit etre branche au demarrage de l'application. Un prebuild --clean
// regenererait MainApplication.kt sans cette ligne, et la panne reviendrait en
// silence : ce plugin la reinjecte.
const INSTALL_LINE = "CastKeepAlive.install(this)";
const INSTALL_IMPORT = "import com.evey.app.cast.CastKeepAlive";

function withCastKeepAliveInstall(config) {
  return withMainApplication(config, (cfg) => {
    let src = cfg.modResults.contents;
    if (!src.includes(INSTALL_IMPORT)) {
      src = src.replace(/^(package .+)$/m, `$1\n\n${INSTALL_IMPORT}`);
    }
    if (!src.includes(INSTALL_LINE)) {
      src = src.replace(/(loadReactNative\(this\))/, `$1\n    ${INSTALL_LINE}`);
    }
    cfg.modResults.contents = src;
    return cfg;
  });
}

module.exports = function withCastMediaNotification(config) {
  config = withCastKeepAliveInstall(config);
  return withAndroidManifest(config, (cfg) => {
    const manifest = cfg.modResults.manifest;
    manifest["uses-permission"] = manifest["uses-permission"] ?? [];
    for (const perm of [PERMISSION, ...KEEPALIVE_PERMISSIONS]) {
      const has = manifest["uses-permission"].some((p) => p.$?.["android:name"] === perm);
      if (!has) manifest["uses-permission"].push({ $: { "android:name": perm } });
    }

    const app = manifest.application?.[0];
    if (!app) return cfg;
    app.activity = app.activity ?? [];
    if (!app.activity.some((a) => a.$?.["android:name"] === EXPANDED_CONTROLLER)) {
      app.activity.push({ $: { "android:name": EXPANDED_CONTROLLER, "android:exported": "false" } });
    }
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
