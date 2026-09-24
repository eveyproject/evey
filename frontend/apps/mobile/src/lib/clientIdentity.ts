import { Platform } from "react-native";
import Constants from "expo-constants";
import { clientUserAgent } from "@evey/client";

// Every API request used to reach the server as "okhttp/4.12.0" - unreadable
// in the admin logs (the player-event handler logs the raw User-Agent as
// `device`) and rendered with a desktop icon on the online panel. This product
// UA rides on all fetcher requests via setExtraHeaders (okhttp and the iOS
// stack both honor a User-Agent header override). The TV app ships the same
// shape as "EveyTV (...)" so the panel can tell the three clients apart.
//
// La version de build est dans le jeton produit ("EveyMobile/0.0.1-beta.14"),
// avant la parenthese : le panneau admin reconnait le client par
// startsWith("eveymobile") et lit le modele entre "; " et ")", les deux
// continuent donc de marcher. Motivation (2026-07-28) : pendant la chasse au
// cast coupe, les logs prod ne disaient pas QUEL build parlait, ce qui rendait
// indecidable "le correctif ne marche pas" contre "le correctif n'est pas
// installe".
//
// La version vient d'expoConfig, embarque tel quel dans l'APK (assets/app.config)
// au moment du build. Premiere tentative RATEE le meme jour :
// `Constants.nativeAppVersion` n'existe pas dans expo-constants 55, et le type
// du module se terminant par `Record<string, any>`, TypeScript l'a accepte
// sans un mot - l'UA repartait sans version, en silence, et j'en ai tire la
// mauvaise conclusion sur le build installe. Toute lecture d'un champ de
// Constants doit etre verifiee contre le paquet, jamais supposee.
export function eveyClientUserAgent(): string {
  const version = Constants.expoConfig?.version;
  if (Platform.OS === "android") {
    const c = (Platform as unknown as { constants?: { Release?: string; Model?: string } }).constants;
    return clientUserAgent({ product: "EveyMobile", version, os: "Android", release: c?.Release, model: c?.Model });
  }
  return clientUserAgent({ product: "EveyMobile", version, os: "iOS", release: Platform.Version });
}
