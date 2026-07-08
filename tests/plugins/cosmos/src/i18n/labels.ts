// i18n dictionary for constellation names, planet names and UI strings.

type Locale = "fr" | "en" | "es" | "de";

type LabelDict = Record<string, Record<Locale, string>>;

export const LABELS: LabelDict = {
  con_ursa_major: { en: "Ursa Major", fr: "Grande Ourse", es: "Osa Mayor", de: "Grosser Bär" },
  con_ursa_minor: { en: "Ursa Minor", fr: "Petite Ourse", es: "Osa Menor", de: "Kleiner Bär" },
  con_orion: { en: "Orion", fr: "Orion", es: "Orión", de: "Orion" },
  con_cassiopeia: { en: "Cassiopeia", fr: "Cassiopée", es: "Casiopea", de: "Kassiopeia" },
  con_cygnus: { en: "Cygnus", fr: "Cygne", es: "Cisne", de: "Schwan" },
  con_leo: { en: "Leo", fr: "Lion", es: "Leo", de: "Löwe" },
  con_scorpius: { en: "Scorpius", fr: "Scorpion", es: "Escorpio", de: "Skorpion" },
  con_sagittarius: { en: "Sagittarius", fr: "Sagittaire", es: "Sagitario", de: "Schütze" },
  con_taurus: { en: "Taurus", fr: "Taureau", es: "Tauro", de: "Stier" },
  con_gemini: { en: "Gemini", fr: "Gémeaux", es: "Géminis", de: "Zwillinge" },
  con_canis_major: { en: "Canis Major", fr: "Grand Chien", es: "Can Mayor", de: "Grosser Hund" },
  con_crux: { en: "Southern Cross", fr: "Croix du Sud", es: "Cruz del Sur", de: "Kreuz des Südens" },
  con_centaurus: { en: "Centaurus", fr: "Centaure", es: "Centauro", de: "Zentaur" },
  con_bootes: { en: "Boötes", fr: "Bouvier", es: "Bootes", de: "Bootes" },
  con_lyra: { en: "Lyra", fr: "Lyre", es: "Lira", de: "Leier" },
  con_aquila: { en: "Aquila", fr: "Aigle", es: "Águila", de: "Adler" },
  con_andromeda: { en: "Andromeda", fr: "Andromède", es: "Andrómeda", de: "Andromeda" },
  con_perseus: { en: "Perseus", fr: "Persée", es: "Perseo", de: "Perseus" },

  planet_venus: { en: "Venus", fr: "Vénus", es: "Venus", de: "Venus" },
  planet_mars: { en: "Mars", fr: "Mars", es: "Marte", de: "Mars" },
  planet_jupiter: { en: "Jupiter", fr: "Jupiter", es: "Júpiter", de: "Jupiter" },
  planet_saturn: { en: "Saturn", fr: "Saturne", es: "Saturno", de: "Saturn" },

  phase_new: { en: "New moon", fr: "Nouvelle lune", es: "Luna nueva", de: "Neumond" },
  phase_waxing_crescent: { en: "Waxing crescent", fr: "Premier croissant", es: "Creciente", de: "Zunehmende Sichel" },
  phase_first_quarter: { en: "First quarter", fr: "Premier quartier", es: "Cuarto creciente", de: "Erstes Viertel" },
  phase_waxing_gibbous: { en: "Waxing gibbous", fr: "Gibbeuse croissante", es: "Gibosa creciente", de: "Zunehmender Mond" },
  phase_full: { en: "Full moon", fr: "Pleine lune", es: "Luna llena", de: "Vollmond" },
  phase_waning_gibbous: { en: "Waning gibbous", fr: "Gibbeuse décroissante", es: "Gibosa menguante", de: "Abnehmender Mond" },
  phase_last_quarter: { en: "Last quarter", fr: "Dernier quartier", es: "Cuarto menguante", de: "Letztes Viertel" },
  phase_waning_crescent: { en: "Waning crescent", fr: "Dernier croissant", es: "Menguante", de: "Abnehmende Sichel" },

  banner_tonight: { en: "Tonight", fr: "Ce soir", es: "Esta noche", de: "Heute Abend" },
  banner_planet_visible: { en: "visible", fr: "visible", es: "visible", de: "sichtbar" },
  banner_full_moon_in: { en: "Full moon in", fr: "Pleine lune dans", es: "Luna llena en", de: "Vollmond in" },
  banner_days_short: { en: "d", fr: "j", es: "d", de: "T" },
  banner_no_planet: { en: "No bright planet up right now", fr: "Aucune planète visible pour l'instant", es: "Ningún planeta brillante visible ahora", de: "Gerade kein heller Planet sichtbar" },

  loading: { en: "Loading sky...", fr: "Chargement du ciel...", es: "Cargando el cielo...", de: "Himmel wird geladen..." },

  // Settings panel (settings.user.appearance slot).
  set_title: { en: "Night sky", fr: "Ciel nocturne", es: "Cielo nocturno", de: "Nachthimmel" },
  set_desc: { en: "Your location for the real-time planetarium widget.", fr: "Ta position pour le planétarium temps réel.", es: "Tu ubicación para el planetario en tiempo real.", de: "Dein Standort für das Echtzeit-Planetarium." },
  set_city: { en: "City", fr: "Ville", es: "Ciudad", de: "Stadt" },
  set_city_ph: { en: "e.g. Paris", fr: "ex. Paris", es: "ej. París", de: "z. B. Paris" },
  set_save: { en: "Save", fr: "Enregistrer", es: "Guardar", de: "Speichern" },
  set_saved: { en: "Sky settings saved", fr: "Réglages du ciel enregistrés", es: "Ajustes del cielo guardados", de: "Himmel-Einstellungen gespeichert" },
  set_error: { en: "Couldn't save", fr: "Enregistrement impossible", es: "No se pudo guardar", de: "Speichern fehlgeschlagen" },
};

export function translate(key: string, locale: string | undefined): string {
  const dict = LABELS[key];
  if (!dict) return key;
  const lang = (locale ?? "en").slice(0, 2).toLowerCase() as Locale;
  return dict[lang] ?? dict.en;
}
