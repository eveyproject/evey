// i18n dictionary for the cinephile widget's UI strings. Decade spans are
// rendered as language-neutral numeric ranges (see formatDecadeRange), so
// they don't need an entry here.

type Locale = "fr" | "en" | "es" | "de";

type LabelDict = Record<string, Record<Locale, string>>;

export const LABELS: LabelDict = {
  profileHeading: {
    en: "Your cinephile profile",
    fr: "Ton profil de cinéphile",
    es: "Tu perfil de cinéfilo",
    de: "Dein Cinephilen-Profil",
  },
  statTotal: { en: "Titles", fr: "Titres", es: "Títulos", de: "Titel" },
  statDominantDecade: { en: "Favorite decade", fr: "Décennie préférée", es: "Década favorita", de: "Lieblingsjahrzehnt" },
  statMovies: { en: "Movies", fr: "Films", es: "Películas", de: "Filme" },
  statSeries: { en: "Series", fr: "Séries", es: "Series", de: "Serien" },
  statSpan: { en: "Span", fr: "Étendue", es: "Rango", de: "Zeitraum" },
  otherDecade: { en: "Other", fr: "Autres", es: "Otros", de: "Weitere" },
  titlesSuffix: { en: "titles", fr: "titres", es: "títulos", de: "Titel" },
  empty: {
    en: "Add movies or shows to your library to reveal your taste galaxy.",
    fr: "Ajoute des films ou séries à ta bibliothèque pour révéler ta galaxie de goûts.",
    es: "Añade películas o series a tu biblioteca para revelar tu galaxia de gustos.",
    de: "Füge Filme oder Serien hinzu, um deine Geschmacksgalaxie zu enthüllen.",
  },
};

export function translate(key: string, locale: string | undefined): string {
  const dict = LABELS[key];
  if (!dict) return key;
  const lang = (locale ?? "en").slice(0, 2).toLowerCase() as Locale;
  return dict[lang] ?? dict.en;
}
