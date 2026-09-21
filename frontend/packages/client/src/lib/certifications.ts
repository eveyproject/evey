export function certForLocale(
  certs: Record<string, string> | undefined,
  locale: string | undefined,
  fallback?: string,
): string | undefined {
  const info = certInfoForLocale(certs, locale, fallback);
  return info?.rating;
}

export function certInfoForLocale(
  certs: Record<string, string> | undefined,
  locale: string | undefined,
  fallback?: string,
  preferredCountry?: string,
): { country: string; rating: string } | undefined {
  if (!certs || Object.keys(certs).length === 0) {
    if (fallback) return { country: preferredCountry || "US", rating: fallback };
    return undefined;
  }
  if (preferredCountry && certs[preferredCountry]) {
    return { country: preferredCountry, rating: certs[preferredCountry] };
  }
  const country = countryFromLocale(locale);
  if (country && certs[country]) return { country, rating: certs[country] };
  if (certs.US) return { country: "US", rating: certs.US };
  const first = Object.keys(certs).sort()[0];
  if (certs[first]) return { country: first, rating: certs[first] };
  if (fallback) return { country: preferredCountry || "US", rating: fallback };
  return undefined;
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
