export function certForLocale(
  certs: Record<string, string> | undefined,
  locale: string | undefined,
  fallback?: string,
): string | undefined {
  if (!certs || Object.keys(certs).length === 0) {
    return fallback || undefined;
  }
  const country = countryFromLocale(locale);
  if (country && certs[country]) return certs[country];
  if (certs.US) return certs.US;
  const first = Object.keys(certs).sort()[0];
  return certs[first] || fallback;
}

function countryFromLocale(locale: string | undefined): string | undefined {
  if (!locale) return undefined;
  const parts = locale.replace("_", "-").split("-");
  if (parts.length >= 2 && parts[1].length === 2) {
    return parts[1].toUpperCase();
  }
  const defaults: Record<string, string> = {
    en: "US",
    fr: "FR",
    es: "ES",
    de: "DE",
    hi: "IN",
    zh: "CN",
  };
  return defaults[parts[0].toLowerCase()];
}
