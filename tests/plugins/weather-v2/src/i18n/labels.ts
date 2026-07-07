// i18n dictionary for weather condition labels and UI strings.
// Keys are stable identifiers shared with conditionToScene labelKey values.

type Locale = "fr" | "en" | "es" | "de";

type LabelDict = Record<string, Record<Locale, string>>;

export const LABELS: LabelDict = {
  clear_day:    { en: "Clear", fr: "Ciel dégagé", es: "Despejado", de: "Klar" },
  clear_night:  { en: "Clear night", fr: "Nuit claire", es: "Noche despejada", de: "Klare Nacht" },
  mainly_clear: { en: "Mainly clear", fr: "Principalement dégagé", es: "Mayormente despejado", de: "Überwiegend klar" },
  partly_cloudy: { en: "Partly cloudy", fr: "Partiellement nuageux", es: "Parcialmente nublado", de: "Teilweise bewölkt" },
  overcast:     { en: "Overcast", fr: "Couvert", es: "Nublado", de: "Bedeckt" },

  fog:      { en: "Fog", fr: "Brouillard", es: "Niebla", de: "Nebel" },
  rime_fog: { en: "Rime fog", fr: "Brouillard givrant", es: "Niebla engelante", de: "Gefrierender Nebel" },

  light_drizzle: { en: "Light drizzle", fr: "Bruine légère", es: "Llovizna ligera", de: "Leichter Nieselregen" },
  drizzle:       { en: "Drizzle", fr: "Bruine", es: "Llovizna", de: "Nieselregen" },
  dense_drizzle: { en: "Dense drizzle", fr: "Bruine dense", es: "Llovizna intensa", de: "Starker Nieselregen" },

  light_freezing_drizzle: { en: "Light freezing drizzle", fr: "Bruine verglaçante légère", es: "Llovizna engelante ligera", de: "Leichter gefrierender Nieselregen" },
  freezing_drizzle:       { en: "Freezing drizzle", fr: "Bruine verglaçante", es: "Llovizna engelante", de: "Gefrierender Nieselregen" },

  light_rain: { en: "Light rain", fr: "Pluie légère", es: "Lluvia ligera", de: "Leichter Regen" },
  rain:       { en: "Moderate rain", fr: "Pluie modérée", es: "Lluvia moderada", de: "Mäßiger Regen" },
  heavy_rain: { en: "Heavy rain", fr: "Pluie forte", es: "Lluvia fuerte", de: "Starker Regen" },

  light_freezing_rain: { en: "Light freezing rain", fr: "Pluie verglaçante légère", es: "Lluvia engelante ligera", de: "Leichter Gefrierregen" },
  freezing_rain:       { en: "Freezing rain", fr: "Pluie verglaçante", es: "Lluvia engelante", de: "Gefrierregen" },

  slight_snow: { en: "Light snow", fr: "Légère neige", es: "Nevada ligera", de: "Leichter Schneefall" },
  snow:        { en: "Moderate snow", fr: "Neige modérée", es: "Nevada moderada", de: "Mäßiger Schneefall" },
  heavy_snow:  { en: "Heavy snow", fr: "Neige abondante", es: "Nevada intensa", de: "Starker Schneefall" },
  snow_grains: { en: "Snow grains", fr: "Grésil", es: "Granos de nieve", de: "Schneekörner" },

  light_showers:   { en: "Light showers", fr: "Averses légères", es: "Chubascos ligeros", de: "Leichte Schauer" },
  showers:         { en: "Showers", fr: "Averses", es: "Chubascos", de: "Schauer" },
  violent_showers: { en: "Heavy showers", fr: "Averses fortes", es: "Chubascos fuertes", de: "Starke Schauer" },

  light_snow_showers: { en: "Light snow showers", fr: "Averses de neige légères", es: "Chubascos de nieve ligeros", de: "Leichte Schneeschauer" },
  heavy_snow_showers: { en: "Heavy snow showers", fr: "Averses de neige fortes", es: "Chubascos de nieve fuertes", de: "Starke Schneeschauer" },

  thunderstorm:              { en: "Thunderstorm", fr: "Orage", es: "Tormenta", de: "Gewitter" },
  thunderstorm_hail:         { en: "Thunderstorm with hail", fr: "Orage avec grêle", es: "Tormenta con granizo", de: "Gewitter mit Hagel" },
  violent_thunderstorm_hail: { en: "Heavy thunderstorm with hail", fr: "Violent orage avec grêle", es: "Tormenta violenta con granizo", de: "Schweres Gewitter mit Hagel" },

  unknown: { en: "Unknown", fr: "Inconnu", es: "Desconocido", de: "Unbekannt" },

  loading:     { en: "Loading…", fr: "Chargement…", es: "Cargando…", de: "Lädt…" },
  unavailable: { en: "Unavailable", fr: "Indisponible", es: "No disponible", de: "Nicht verfügbar" },

  // Settings panel (settings.user.appearance slot).
  set_title:   { en: "Weather", fr: "Météo", es: "Clima", de: "Wetter" },
  set_desc:    { en: "Your home city and temperature unit for the weather widget.", fr: "Ta ville de résidence et l'unité de température du widget météo.", es: "Tu ciudad y la unidad de temperatura del widget del clima.", de: "Deine Stadt und Temperatureinheit für das Wetter-Widget." },
  set_city:    { en: "City", fr: "Ville", es: "Ciudad", de: "Stadt" },
  set_city_ph: { en: "e.g. Paris", fr: "ex. Paris", es: "ej. París", de: "z. B. Paris" },
  set_units:   { en: "Temperature unit", fr: "Unité de température", es: "Unidad de temperatura", de: "Temperatureinheit" },
  set_celsius: { en: "Celsius (°C)", fr: "Celsius (°C)", es: "Celsius (°C)", de: "Celsius (°C)" },
  set_fahrenheit: { en: "Fahrenheit (°F)", fr: "Fahrenheit (°F)", es: "Fahrenheit (°F)", de: "Fahrenheit (°F)" },
  set_save:    { en: "Save", fr: "Enregistrer", es: "Guardar", de: "Speichern" },
  set_saved:   { en: "Weather settings saved", fr: "Réglages météo enregistrés", es: "Ajustes del clima guardados", de: "Wetter-Einstellungen gespeichert" },
  set_error:   { en: "Couldn't save", fr: "Enregistrement impossible", es: "No se pudo guardar", de: "Speichern fehlgeschlagen" },
};

export function translate(key: string, locale: string | undefined): string {
  const dict = LABELS[key];
  if (!dict) return key;
  const lang = (locale ?? "en").slice(0, 2).toLowerCase() as Locale;
  return dict[lang] ?? dict.en;
}
