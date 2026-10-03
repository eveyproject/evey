const { withGradleProperties, withDangerousMod } = require("expo/config-plugins");
const fs = require("node:fs");
const path = require("node:path");

// R8 sur la TV, comme sur le telephone (android.enableMinifyInReleaseBuilds
// dans apps/mobile/android/gradle.properties, qui est suivi par git ; celui de
// la TV est un artefact de prebuild, ignore, donc le reglage vit ici).
//
// Pourquoi : l'AAB TV n'avait ni r8.json ni mapping, d'ou l'avertissement
// « aucun fichier de desobscurcissement » de la Play Console (mesure le
// 03-10 : le mobile embarque proguard.map, la TV non), et 4 dex de ~30 Mo.
//
// Les regles ne sont que celles qu'un build ACTIVE a deja demandees au mobile
// (apps/mobile/android/app/proguard-rules.pro) et qui concernent aussi la TV :
// la TV embarque expo-secure-store et le module offline-audio (ses propres
// regles, consumer-rules.pro, gardent go.** et com.evey.offlineaudio.** : le
// pont JNI de gomobile et le lecteur ac3go construit par reflexion).
const MARK = "# --- evey: R8 rules (withR8Minify) ---";
const RULES = `${MARK}
# expo Record conversion finds these via kotlin-reflect; R8 -optimize strips their
# kotlin.Metadata unless kept (SecureStoreOptions cast NPE at boot, expo/expo#28010).
# expo-secure-store holds the TV's session and settings.
-keep @interface expo.modules.kotlin.records.Field { *; }
-keep @interface expo.modules.kotlin.records.Required { *; }
-keep class com.facebook.react.turbomodule.** { *; }
`;

function setProperty(props, key, value) {
  const existing = props.find((p) => p.type === "property" && p.key === key);
  if (existing) existing.value = value;
  else props.push({ type: "property", key, value });
}

function withR8Minify(config) {
  config = withGradleProperties(config, (cfg) => {
    setProperty(cfg.modResults, "android.enableMinifyInReleaseBuilds", "true");
    setProperty(cfg.modResults, "android.enableShrinkResourcesInReleaseBuilds", "true");
    return cfg;
  });
  return withDangerousMod(config, [
    "android",
    async (cfg) => {
      const file = path.join(cfg.modRequest.platformProjectRoot, "app", "proguard-rules.pro");
      const current = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
      if (!current.includes(MARK)) fs.writeFileSync(file, `${current.replace(/\n*$/, "\n")}\n${RULES}`);
      return cfg;
    },
  ]);
}

module.exports = withR8Minify;
module.exports.MARK = MARK;
