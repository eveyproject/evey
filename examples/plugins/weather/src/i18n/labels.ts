// i18n dictionary for weather condition labels and UI strings.
// Keys are stable identifiers; EN values match the original English labels exactly.

type Locale = "fr" | "en" | "es" | "de";

type LabelDict = Record<string, Record<Locale, string>>;

export const LABELS: LabelDict = {
  // Sky conditions
  clear_day: {
    en: "Clear",
    fr: "Ciel dégagé",
    es: "Despejado",
    de: "Klar",
  },
  clear_night: {
    en: "Clear night",
    fr: "Nuit claire",
    es: "Noche despejada",
    de: "Klare Nacht",
  },
  mainly_clear: {
    en: "Mainly clear",
    fr: "Principalement dégagé",
    es: "Mayormente despejado",
    de: "Überwiegend klar",
  },
  partly_cloudy: {
    en: "Partly cloudy",
    fr: "Partiellement nuageux",
    es: "Parcialmente nublado",
    de: "Teilweise bewölkt",
  },
  overcast: {
    en: "Overcast",
    fr: "Couvert",
    es: "Nublado",
    de: "Bedeckt",
  },

  // Fog
  fog: {
    en: "Fog",
    fr: "Brouillard",
    es: "Niebla",
    de: "Nebel",
  },
  rime_fog: {
    en: "Rime fog",
    fr: "Brouillard givrant",
    es: "Niebla engelante",
    de: "Gefrierender Nebel",
  },

  // Drizzle
  light_drizzle: {
    en: "Light drizzle",
    fr: "Bruine légère",
    es: "Llovizna ligera",
    de: "Leichter Nieselregen",
  },
  drizzle: {
    en: "Drizzle",
    fr: "Bruine",
    es: "Llovizna",
    de: "Nieselregen",
  },
  dense_drizzle: {
    en: "Dense drizzle",
    fr: "Bruine dense",
    es: "Llovizna intensa",
    de: "Starker Nieselregen",
  },

  // Freezing drizzle
  light_freezing_drizzle: {
    en: "Light freezing drizzle",
    fr: "Bruine verglaçante légère",
    es: "Llovizna engelante ligera",
    de: "Leichter gefrierender Nieselregen",
  },
  freezing_drizzle: {
    en: "Freezing drizzle",
    fr: "Bruine verglaçante",
    es: "Llovizna engelante",
    de: "Gefrierender Nieselregen",
  },

  // Rain
  light_rain: {
    en: "Light rain",
    fr: "Pluie légère",
    es: "Lluvia ligera",
    de: "Leichter Regen",
  },
  rain: {
    en: "Moderate rain",
    fr: "Pluie modérée",
    es: "Lluvia moderada",
    de: "Mäßiger Regen",
  },
  heavy_rain: {
    en: "Heavy rain",
    fr: "Pluie forte",
    es: "Lluvia fuerte",
    de: "Starker Regen",
  },

  // Freezing rain
  light_freezing_rain: {
    en: "Light freezing rain",
    fr: "Pluie verglaçante légère",
    es: "Lluvia engelante ligera",
    de: "Leichter Gefrierregen",
  },
  freezing_rain: {
    en: "Freezing rain",
    fr: "Pluie verglaçante",
    es: "Lluvia engelante",
    de: "Gefrierregen",
  },

  // Snow
  slight_snow: {
    en: "Light snow",
    fr: "Légère neige",
    es: "Nevada ligera",
    de: "Leichter Schneefall",
  },
  snow: {
    en: "Moderate snow",
    fr: "Neige modérée",
    es: "Nevada moderada",
    de: "Mäßiger Schneefall",
  },
  heavy_snow: {
    en: "Heavy snow",
    fr: "Neige abondante",
    es: "Nevada intensa",
    de: "Starker Schneefall",
  },
  snow_grains: {
    en: "Snow grains",
    fr: "Grésil",
    es: "Granos de nieve",
    de: "Schneekörner",
  },

  // Showers
  light_showers: {
    en: "Light showers",
    fr: "Averses légères",
    es: "Chubascos ligeros",
    de: "Leichte Schauer",
  },
  showers: {
    en: "Showers",
    fr: "Averses",
    es: "Chubascos",
    de: "Schauer",
  },
  violent_showers: {
    en: "Heavy showers",
    fr: "Averses fortes",
    es: "Chubascos fuertes",
    de: "Starke Schauer",
  },

  // Snow showers
  light_snow_showers: {
    en: "Light snow showers",
    fr: "Averses de neige légères",
    es: "Chubascos de nieve ligeros",
    de: "Leichte Schneeschauer",
  },
  heavy_snow_showers: {
    en: "Heavy snow showers",
    fr: "Averses de neige fortes",
    es: "Chubascos de nieve fuertes",
    de: "Starke Schneeschauer",
  },

  // Thunderstorms
  thunderstorm: {
    en: "Thunderstorm",
    fr: "Orage",
    es: "Tormenta",
    de: "Gewitter",
  },
  thunderstorm_hail: {
    en: "Thunderstorm with hail",
    fr: "Orage avec grêle",
    es: "Tormenta con granizo",
    de: "Gewitter mit Hagel",
  },
  violent_thunderstorm_hail: {
    en: "Heavy thunderstorm with hail",
    fr: "Violent orage avec grêle",
    es: "Tormenta violenta con granizo",
    de: "Schweres Gewitter mit Hagel",
  },

  // Fallback
  unknown: {
    en: "Unknown",
    fr: "Inconnu",
    es: "Desconocido",
    de: "Unbekannt",
  },

  // UI strings
  loading: {
    en: "Loading…",
    fr: "Chargement…",
    es: "Cargando…",
    de: "Lädt…",
  },
  unavailable: {
    en: "Unavailable",
    fr: "Indisponible",
    es: "No disponible",
    de: "Nicht verfügbar",
  },
};

export function translate(key: string, locale: string | undefined): string {
  const dict = LABELS[key];
  if (!dict) return key;
  const lang = ((locale ?? "en").slice(0, 2).toLowerCase()) as Locale;
  return dict[lang] ?? dict.en;
}
