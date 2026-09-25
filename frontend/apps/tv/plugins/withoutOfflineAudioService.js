const { withAndroidManifest } = require("expo/config-plugins");

// La TV embarque @evey/offline-audio pour son decodeur ac3go (le lecteur
// decode l'AC-3/E-AC-3 quand la sortie HDMI ne prend pas le Dolby tel quel),
// mais PAS la conversion audio des telechargements, qui n'existe que sur le
// telephone. Le manifeste du module declare le service de premier plan de
// cette conversion (type dataSync) et sa permission : les garder sur la TV
// imposerait une declaration de service de premier plan dans la Play Console
// pour une fonction absente. Ce plugin les retire au merge.
//
// FOREGROUND_SERVICE et WAKE_LOCK restent : la lecture (service media
// d'expo-video) en a besoin, les retirer casserait autre chose.
const SERVICE = "com.evey.offlineaudio.OfflineAudioService";
const DATA_SYNC = "android.permission.FOREGROUND_SERVICE_DATA_SYNC";

function markRemoved(list, name) {
  const existing = list.find((e) => e.$?.["android:name"] === name);
  if (existing) {
    existing.$["tools:node"] = "remove";
    return;
  }
  list.push({ $: { "android:name": name, "tools:node": "remove" } });
}

function withoutOfflineAudioService(config) {
  return withAndroidManifest(config, (cfg) => {
    const manifest = cfg.modResults.manifest;
    manifest.$ = manifest.$ ?? {};
    // Meme garde que withNoAdId : sans ce namespace, tools:node est ignore.
    manifest.$["xmlns:tools"] = manifest.$["xmlns:tools"] ?? "http://schemas.android.com/tools";

    manifest["uses-permission"] = manifest["uses-permission"] ?? [];
    markRemoved(manifest["uses-permission"], DATA_SYNC);

    const app = manifest.application?.[0];
    if (app) {
      app.service = app.service ?? [];
      markRemoved(app.service, SERVICE);
    }
    return cfg;
  });
}

module.exports = withoutOfflineAudioService;
