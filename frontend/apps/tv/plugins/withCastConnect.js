const { withAndroidManifest, AndroidConfig } = require("expo/config-plugins");

// Cast Connect : déclare l'app TV comme cible d'un cast, pour que « caster »
// depuis le web ou le téléphone OUVRE Evey TV au lieu du Web Receiver, et que
// la télécommande pilote alors le vrai lecteur Evey.
//
// apps/tv/android/ est gitignoré (régénéré par prebuild) : tout ce qui doit
// survivre passe donc par ici, jamais par un fichier posé dans android/.
//
// Deux déclarations, et l'ordre des noms compte :
//   - l'intent-filter LAUNCH (com.google.android.gms.cast.tv.action.LAUNCH)
//     est ce que la plateforme envoie pour DÉMARRER l'app sur un cast. Ce
//     n'est pas action.LOAD : le LOAD arrive ensuite, au MediaManager, une
//     fois l'app lancée et le LaunchRequestChecker passé.
//   - le meta-data RECEIVER_OPTIONS_PROVIDER_CLASS_NAME nomme la classe qui
//     fournit les ReceiverOptions, dont notre LaunchRequestChecker. Sans lui
//     le SDK prend ses options par défaut et accepte tout, y compris un cast
//     venu d'une autre instance Evey.
const LAUNCH_ACTION = "com.google.android.gms.cast.tv.action.LAUNCH";
const OPTIONS_PROVIDER_META = "com.google.android.gms.cast.tv.RECEIVER_OPTIONS_PROVIDER_CLASS_NAME";
const OPTIONS_PROVIDER_CLASS = "com.evey.casttv.EveyCastReceiverOptionsProvider";

function withCastConnect(config) {
  return withAndroidManifest(config, (cfg) => {
    const app = AndroidConfig.Manifest.getMainApplicationOrThrow(cfg.modResults);
    const activity = AndroidConfig.Manifest.getMainActivityOrThrow(cfg.modResults);

    activity["intent-filter"] = activity["intent-filter"] ?? [];
    const hasLaunch = activity["intent-filter"].some((f) =>
      (f.action ?? []).some((a) => a.$?.["android:name"] === LAUNCH_ACTION),
    );
    if (!hasLaunch) {
      activity["intent-filter"].push({
        action: [{ $: { "android:name": LAUNCH_ACTION } }],
        category: [{ $: { "android:name": "android.intent.category.DEFAULT" } }],
      });
    }

    // singleTask, pas le launchMode par défaut : un cast reçu pendant que
    // l'app tourne déjà doit atterrir dans l'instance existante (avec sa
    // session et son lecteur), pas en empiler une seconde par-dessus.
    activity.$["android:launchMode"] = "singleTask";

    AndroidConfig.Manifest.addMetaDataItemToMainApplication(
      app,
      OPTIONS_PROVIDER_META,
      OPTIONS_PROVIDER_CLASS,
    );

    return cfg;
  });
}

module.exports = withCastConnect;
