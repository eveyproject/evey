const { withAndroidManifest } = require("expo/config-plugins");

// Play compte les appareils compatibles a partir des features que le
// manifeste EXIGE - y compris celles qu'une permission implique sans qu'on
// l'ait ecrite. La release 22 (0.1.3) n'etait servie qu'a 12 appareils
// Android TV (Play Console, 20-09) : expo-audio declare RECORD_AUDIO, que
// Play traduit en « micro obligatoire », et la plupart des televiseurs n'en
// declarent pas. L'appli TV ne fait que LIRE de l'audio (TVMusicPlayer), elle
// n'enregistre jamais : la permission part, et les features qu'un televiseur
// peut ne pas avoir sont dites facultatives, comme la doc Android TV le
// demande (touchscreen, faketouch, microphone, camera, location, sensors,
// screen.portrait). leanback, lui, reste EXIGE (config-tv, androidTVRequired).
const REMOVED_PERMISSIONS = ["android.permission.RECORD_AUDIO"];
const OPTIONAL_FEATURES = [
  "android.hardware.touchscreen",
  "android.hardware.faketouch",
  "android.hardware.microphone",
  "android.hardware.camera",
  "android.hardware.camera.any",
  "android.hardware.location",
  "android.hardware.location.gps",
  "android.hardware.location.network",
  "android.hardware.sensor.accelerometer",
  "android.hardware.sensor.gyroscope",
  "android.hardware.screen.portrait",
  "android.hardware.telephony",
  "android.hardware.wifi",
  "android.hardware.bluetooth",
  "android.hardware.nfc",
];

function withTvHardwareFeatures(config) {
  return withAndroidManifest(config, (cfg) => {
    const manifest = cfg.modResults.manifest;
    manifest.$ = manifest.$ ?? {};
    manifest.$["xmlns:tools"] = manifest.$["xmlns:tools"] ?? "http://schemas.android.com/tools";

    manifest["uses-permission"] = manifest["uses-permission"] ?? [];
    for (const name of REMOVED_PERMISSIONS) {
      const existing = manifest["uses-permission"].find((p) => p.$?.["android:name"] === name);
      if (existing) existing.$["tools:node"] = "remove";
      else manifest["uses-permission"].push({ $: { "android:name": name, "tools:node": "remove" } });
    }

    manifest["uses-feature"] = manifest["uses-feature"] ?? [];
    for (const name of OPTIONAL_FEATURES) {
      const existing = manifest["uses-feature"].find((f) => f.$?.["android:name"] === name);
      if (existing) existing.$["android:required"] = "false";
      else manifest["uses-feature"].push({ $: { "android:name": name, "android:required": "false" } });
    }
    return cfg;
  });
}

module.exports = withTvHardwareFeatures;
