// Mirror of internal/pkg/region.DefaultForLocale. Used for UX hints only -
// the authoritative default lives server-side and is applied via
// ResolveRegion when a profile is used without an explicit region.
const DEFAULT_REGION_FOR_LOCALE: Record<string, string> = {
  en: "GB",
  fr: "FR",
  de: "DE",
  es: "ES",
  hi: "IN",
  zh: "CN",
};

export function defaultRegionForLocale(locale: string | undefined): string {
  if (!locale) return "GB";
  const parts = locale.replace("_", "-").split("-");
  if (parts.length >= 2 && /^[A-Za-z]{2}$/.test(parts[1])) {
    return parts[1].toUpperCase();
  }
  const bare = parts[0].toLowerCase();
  return DEFAULT_REGION_FOR_LOCALE[bare] ?? "GB";
}
