const { withAndroidManifest } = require("expo/config-plugins");

// Evey n'utilise AUCUN identifiant publicitaire, et doit le dire de facon
// verifiable.
//
// Play exige une declaration « identifiant publicitaire » pour toute appli
// qui cible Android 13+ (l'avertissement de la Play Console, 19-09). La
// declaration se remplit dans la Console, mais elle doit rester VRAIE : la
// permission com.google.android.gms.permission.AD_ID est declaree par
// plusieurs bibliotheques Google (play-services-ads-identifier, et ses
// consommateurs transitifs) et le merge de manifeste l'ajoute alors sans que
// personne ne l'ait demande. Une mise a jour d'une dependance suffirait donc
// a transformer la declaration en mensonge, et Play rejette pour ca.
//
// Ce plugin retire la permission au merge. Verifie le 19-09 : le manifeste
// fusionne (debug et release) n'en contenait aucune, Cast Connect inclus -
// c'est donc une garde pour l'avenir, pas une correction d'un manque actuel.
// Si un jour l'appli DOIT utiliser l'identifiant publicitaire, supprimer ce
// plugin ET changer la declaration dans la Console, dans cet ordre.
const AD_ID = "com.google.android.gms.permission.AD_ID";

function withNoAdId(config) {
  return withAndroidManifest(config, (cfg) => {
    const manifest = cfg.modResults.manifest;
    manifest.$ = manifest.$ ?? {};
    // tools: n'est pas declare par le template - sans ce namespace,
    // tools:node est ignore en silence (et aapt refuse le build).
    manifest.$["xmlns:tools"] = manifest.$["xmlns:tools"] ?? "http://schemas.android.com/tools";

    manifest["uses-permission"] = manifest["uses-permission"] ?? [];
    const existing = manifest["uses-permission"].find((p) => p.$?.["android:name"] === AD_ID);
    if (existing) {
      existing.$["tools:node"] = "remove";
      return cfg;
    }
    manifest["uses-permission"].push({ $: { "android:name": AD_ID, "tools:node": "remove" } });
    return cfg;
  });
}

module.exports = withNoAdId;
