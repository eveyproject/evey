// i18n dictionary for the marquee widget's UI strings.

type Locale = "fr" | "en" | "es" | "de";

type LabelDict = Record<string, Record<Locale, string>>;

export const LABELS: LabelDict = {
  cta: {
    en: "Tonight, what are we watching?",
    fr: "Ce soir, on regarde ?",
    es: "¿Qué vemos esta noche?",
    de: "Was schauen wir heute?",
  },
  spinning: { en: "Picking something…", fr: "Sélection en cours…", es: "Eligiendo algo…", de: "Wähle etwas aus…" },
  play_hint: { en: "Tap to watch", fr: "Toucher pour regarder", es: "Toca para ver", de: "Zum Ansehen tippen" },
  retry: { en: "Spin again", fr: "Relancer", es: "Girar de nuevo", de: "Nochmal drehen" },
  watch_now: { en: "Watch now", fr: "Regarder maintenant", es: "Ver ahora", de: "Jetzt ansehen" },
  // {s} is substituted with the live remaining whole seconds.
  opening_in: { en: "Opening in {s}s", fr: "Ouverture dans {s} s", es: "Abriendo en {s} s", de: "Öffnet in {s} s" },
  empty: {
    en: "Add movies or shows to your library to light up the wall.",
    fr: "Ajoute des films ou séries à ta bibliothèque pour allumer le mur.",
    es: "Añade películas o series a tu biblioteca para encender el mural.",
    de: "Füge Filme oder Serien hinzu, um die Wand zu erleuchten.",
  },
};

export function translate(key: string, locale: string | undefined): string {
  const dict = LABELS[key];
  if (!dict) return key;
  const lang = (locale ?? "en").slice(0, 2).toLowerCase() as Locale;
  return dict[lang] ?? dict.en;
}
