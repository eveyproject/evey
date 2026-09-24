import i18n, { type Resource } from "i18next";
import { initReactI18next } from "react-i18next";
import HttpBackend from "i18next-http-backend";
import { getLocales } from "expo-localization";
import { useAuthStore, useLocaleStore } from "@evey/store";

// The translation boot of the two native apps. The phone and the television
// kept a copy each until 2026-09-24; the television's had missed the phone's
// 2026-08-09 fix: its init returned at once when already initialised, so a
// boot whose one catalogue fetch failed (a sleeping NAS) or a switch to another
// server never loaded the server's translations for the rest of the session.
// Each app still brings its own offline floor (NativeI18nConfig).

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "de", label: "Deutsch" },
  { code: "es", label: "Español" },
  { code: "fr", label: "Français" },
  { code: "hi", label: "हिन्दी" },
  { code: "zh", label: "中文" },
] as const;

export function isSupportedLanguage(code?: string): boolean {
  if (!code) return false;
  return supportedLanguages.some((l) => l.code === code);
}

export function detectLanguage(): string {
  const { locale: stored, source } = useLocaleStore.getState();
  const storedLang = stored ? stored.split("-")[0] : undefined;
  const storedOk = isSupportedLanguage(storedLang);
  // Precedence (user decision 2026-08-05): profile > user pick > device >
  // server default. A stored profile claim or explicit pick wins outright;
  // a mere server-default echo ranks BELOW the device language.
  if (storedOk && (source === "profile" || source === "user")) return storedLang!;
  const deviceLang = getLocales()[0]?.languageCode ?? "en";
  if (isSupportedLanguage(deviceLang)) return deviceLang;
  if (storedOk) return storedLang!;
  return "en";
}

// The persisted stores rehydrate ASYNCHRONOUSLY (SecureStore), so reading
// them at boot races the storage: deciding before they land is how the server
// default used to beat a known profile. The timeout keeps a broken storage
// from hanging the boot forever.
export async function storeHydrated(store: {
  persist?: { hasHydrated: () => boolean; onFinishHydration: (cb: () => void) => () => void };
}): Promise<void> {
  const p = store.persist;
  if (!p || p.hasHydrated()) return;
  await new Promise<void>((resolve) => {
    let unsub: (() => void) | undefined;
    const timer = setTimeout(() => {
      unsub?.();
      resolve();
    }, 3000);
    unsub = p.onFinishHydration(() => {
      clearTimeout(timer);
      unsub?.();
      resolve();
    });
  });
}

// Translations actually IN MEMORY, not merely i18next constructed: a failed
// HTTP load leaves it "initialized" with an EMPTY catalogue.
export function i18nLoaded(): boolean {
  return i18n.isInitialized && i18n.hasResourceBundle(i18n.language, "translation");
}

export interface NativeI18nConfig {
  // Bundled strings merged UNDER the server catalogue (partialBundledLanguages):
  // an unreachable server degrades to these words instead of key names.
  resources?: Resource;
  // Called when the server catalogue fails to load for a language.
  onFailedLoading?: (lng: string) => void;
}

let config: NativeI18nConfig = {};

// Each app declares its offline floor once, at module load.
export function configureNativeI18n(next: NativeI18nConfig): void {
  config = next;
}

// Whether the SERVER catalogue landed in this session - not whether i18next
// holds a bundle (the bundled floor is one). It rides i18next's own events, so
// a boot whose single fetch failed is recognisable and retriable.
let catalogueArrived = false;

// Re-entrant: the first launch runs it with no server, the server screen with
// the chosen one, and each app gives it a second chance once signed in. It
// re-points and reloads when the path changed or the catalogue never arrived.
export async function initI18n(serverUrl: string) {
  if (i18n.isInitialized) {
    const wanted = serverUrl ? `${serverUrl}/api/v1/i18n/{{lng}}` : "";
    const backend = i18n.services.backendConnector.backend as { options?: { loadPath?: string } } | undefined;
    if (wanted && backend?.options && (backend.options.loadPath !== wanted || !catalogueArrived)) {
      backend.options.loadPath = wanted;
      try {
        await i18n.reloadResources();
      } catch {
        /* still unreachable: the bundled floor keeps the UI readable */
      }
    }
    return i18n;
  }

  i18n.on?.("loaded", () => { catalogueArrived = true; });
  i18n.on?.("failedLoading", (lng: string) => {
    catalogueArrived = false;
    config.onFailedLoading?.(lng);
  });
  await i18n
    .use(HttpBackend)
    .use(initReactI18next)
    .init({
      backend: {
        loadPath: `${serverUrl}/api/v1/i18n/{{lng}}`,
      },
      lng: detectLanguage(),
      fallbackLng: "en",
      ns: ["translation"],
      defaultNS: "translation",
      keySeparator: false,
      interpolation: { escapeValue: false },
      ...(config.resources ? { partialBundledLanguages: true, resources: config.resources } : {}),
    });

  return i18n;
}

// Resolve the boot language once the persisted stores have landed, then - and
// only as a LAST resort - apply the server's default so a fresh install
// renders login/setup in the admin-picked language. Deliberately a raw fetch:
// it probes a CANDIDATE server that may differ from the configured baseURL.
export async function applyServerLanguage(serverUrl: string): Promise<void> {
  await Promise.all([storeHydrated(useAuthStore), storeHydrated(useLocaleStore)]);
  // Re-run detection now that the persisted locale/source are readable.
  const resolved = detectLanguage();
  if (resolved !== i18n.language) {
    try {
      await i18n.changeLanguage(resolved);
    } catch {
      /* offline: the bundled floor keeps the UI readable */
    }
  }
  // A profile is known (JWT locale claim reigns), or the stored locale came
  // from a profile/user pick: the server default is outranked, stop here.
  if (useAuthStore.getState().accessToken) return;
  const { source } = useLocaleStore.getState();
  if (source === "profile" || source === "user") return;
  // Device language outranks the server default too.
  if (isSupportedLanguage(getLocales()[0]?.languageCode ?? undefined)) return;
  try {
    const res = await fetch(`${serverUrl}/api/v1/setup/status`);
    if (!res.ok) return;
    const info = (await res.json()) as { serverLanguage?: string };
    const lang = info.serverLanguage?.split("-")[0];
    if (!isSupportedLanguage(lang)) return;
    // Re-check AFTER the fetch: an auto-login or a profile switch can land
    // while setup/status is in flight, and must not be overwritten.
    if (useAuthStore.getState().accessToken) return;
    const latest = useLocaleStore.getState();
    if (latest.source === "profile" || latest.source === "user") return;
    if (latest.locale === lang) return;
    useLocaleStore.getState().setLocale(lang!, "server");
    await i18n.changeLanguage(lang!);
  } catch {
    // setup/status is best-effort; an offline boot keeps the device default.
  }
}

// A profile EDIT that changes the locale must show at once when it touches the
// profile the session runs as - the JWT claim only reapplies it at the next
// login or profile switch (reported 2026-08-28).
export async function applyEditedProfileLocale(
  profile: { id?: string; isDefault?: boolean },
  locale: string,
): Promise<void> {
  const activeId = useAuthStore.getState().profileId;
  const isActive = activeId ? profile.id === activeId : !!profile.isDefault;
  if (!isActive) return;
  const lang = locale ? locale.split("-")[0] : "";
  if (lang && isSupportedLanguage(lang)) {
    useLocaleStore.getState().setLocale(lang, "profile");
    try {
      await i18n.changeLanguage(lang);
    } catch {
      /* offline: the store is set, the next boot applies it */
    }
    return;
  }
  // "" = Automatic: fall back to the device language, ranked "server" so a
  // later profile claim or user pick can reclaim precedence.
  const deviceLang = getLocales()[0]?.languageCode ?? "en";
  const resolved = isSupportedLanguage(deviceLang) ? deviceLang : "en";
  useLocaleStore.getState().setLocale(resolved, "server");
  try {
    await i18n.changeLanguage(resolved);
  } catch {
    /* offline: same as above */
  }
}

export default i18n;
