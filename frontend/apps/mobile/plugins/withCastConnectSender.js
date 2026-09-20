const { withAndroidManifest, AndroidConfig } = require("expo/config-plugins");

// Cast Connect, cote SENDER : fait pointer le meta-data OptionsProvider du
// Cast SDK sur NOTRE classe (EveyCastOptionsProvider), qui ajoute
// setAndroidReceiverCompatible(true) aux options de react-native-google-cast.
//
// Le plugin du paquet ecrit ce meta-data avec sa propre classe ; le notre doit
// donc passer APRES lui dans app.json pour ecraser la valeur. Sans ca le
// telephone ne proposerait jamais l'app Evey TV et ouvrirait toujours le Web
// Receiver, meme sur une box qui a l'app installee.
const META = "com.google.android.gms.cast.framework.OPTIONS_PROVIDER_CLASS_NAME";
const PROVIDER = "com.evey.app.cast.EveyCastOptionsProvider";

function withCastConnectSender(config) {
  return withAndroidManifest(config, (cfg) => {
    const app = AndroidConfig.Manifest.getMainApplicationOrThrow(cfg.modResults);
    // addMetaDataItemToMainApplication remplace la valeur quand la cle existe
    // deja, ce qui est exactement le cas ici.
    AndroidConfig.Manifest.addMetaDataItemToMainApplication(app, META, PROVIDER);
    return cfg;
  });
}

module.exports = withCastConnectSender;
