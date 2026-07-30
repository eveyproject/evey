import { describe, it, expect } from "vitest";
import { vi } from "vitest";

// The mobile app's API requests used to reach the server as "okhttp/4.12.0":
// unreadable in the admin logs and rendered with a desktop icon on the online
// panel. The app now identifies itself with a product UA the panel and the
// future TV app (EveyTV) share the shape of.
//
// La version de build a ete ajoutee le 2026-07-28 : en pleine chasse au cast
// coupe, les logs prod montraient "EveyMobile (Android 13; TestPhone)" sans dire
// QUEL build parlait. Impossible de trancher entre "le correctif ne marche
// pas" et "le correctif n'est pas installe" sans aller le demander a
// l'utilisateur. Un diagnostic ne doit pas dependre d'une question posee.
let platformOS = "android";
let platformVersion: number | string = 34;
let platformConstants: Record<string, string> | undefined = { Release: "14", Model: "SM-G991B" };
let appVersion: string | undefined = "0.0.1-beta.14";

vi.mock("react-native", () => ({
  get Platform() {
    return { OS: platformOS, Version: platformVersion, constants: platformConstants };
  },
}));

// Le mock reproduit la FORME REELLE d'expo-constants 55 : la version vit dans
// expoConfig (embarque tel quel dans l'APK sous assets/app.config au build).
// Piege paye le 2026-07-28 : `Constants.nativeAppVersion` n'existe pas dans
// cette version, et le type se terminant par `Record<string, any>`, TypeScript
// l'accepte sans broncher - l'UA repartait sans version, en silence. Un mock
// qui invente un champ valide n'importe quel code : celui-ci ne doit exposer
// QUE ce que le paquet expose vraiment.
vi.mock("expo-constants", () => ({
  default: {
    get expoConfig() {
      return appVersion === undefined ? null : { version: appVersion };
    },
  },
}));

import { eveyClientUserAgent } from "./clientIdentity";

describe("eveyClientUserAgent", () => {
  it("builds an EveyMobile UA with build version, Android release and device model", () => {
    platformOS = "android";
    platformConstants = { Release: "14", Model: "SM-G991B" };
    appVersion = "0.0.1-beta.14";
    expect(eveyClientUserAgent()).toBe("EveyMobile/0.0.1-beta.14 (Android 14; SM-G991B)");
  });

  it("degrades gracefully when the platform constants are missing", () => {
    platformOS = "android";
    platformConstants = undefined;
    appVersion = "0.0.1-beta.14";
    expect(eveyClientUserAgent()).toBe("EveyMobile/0.0.1-beta.14 (Android)");
  });

  it("omet la version plutot que d'ecrire un mensonge quand elle est absente", () => {
    platformOS = "android";
    platformConstants = { Release: "14", Model: "SM-G991B" };
    appVersion = undefined;
    expect(eveyClientUserAgent()).toBe("EveyMobile (Android 14; SM-G991B)");
  });

  it("labels iOS with its version", () => {
    platformOS = "ios";
    platformVersion = "17.4";
    appVersion = "0.0.1-beta.14";
    expect(eveyClientUserAgent()).toBe("EveyMobile/0.0.1-beta.14 (iOS 17.4)");
  });

  it("garde le prefixe que le panneau admin teste (startsWith eveymobile)", () => {
    platformOS = "android";
    platformConstants = { Release: "14", Model: "SM-G991B" };
    appVersion = "0.0.1-beta.14";
    expect(eveyClientUserAgent().toLowerCase().startsWith("eveymobile")).toBe(true);
    // Le modele est lu entre "; " et ")" par onlineDevice.ts : la version
    // ajoutee avant la parenthese ne doit pas deplacer ce reperage.
    expect(/;\s*([^)]+)\)/.exec(eveyClientUserAgent())?.[1]).toBe("SM-G991B");
  });
});
