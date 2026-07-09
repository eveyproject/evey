// i18n dictionary for the settings panel. The scene itself shows no text
// (lumina is a pure visual wallpaper, no data to display) - every key here
// is settings.user.appearance UI copy. Preset names (Aurore/Néon/Braise/
// Océan) are intentionally NOT translated: stylized names, like a paint
// swatch, the same way weather-v2/cosmos never translate city names.

type Locale = "fr" | "en" | "es" | "de";

type LabelDict = Record<string, Record<Locale, string>>;

export const LABELS: LabelDict = {
  set_title: { en: "Ambient light", fr: "Lumière ambiante", es: "Luz ambiental", de: "Umgebungslicht" },
  set_desc: {
    en: "Palette, intensity and speed for the ambient light-art background.",
    fr: "Palette, intensité et vitesse du fond d'écran de lumière ambiante.",
    es: "Paleta, intensidad y velocidad del fondo de luz ambiental.",
    de: "Palette, Intensität und Geschwindigkeit des Umgebungslicht-Hintergrunds.",
  },
  set_palette: { en: "Palette", fr: "Palette", es: "Paleta", de: "Palette" },
  set_intensity: { en: "Intensity", fr: "Intensité", es: "Intensidad", de: "Intensität" },
  set_speed: { en: "Speed", fr: "Vitesse", es: "Velocidad", de: "Geschwindigkeit" },
  set_save: { en: "Save", fr: "Enregistrer", es: "Guardar", de: "Speichern" },
  set_saved: { en: "Lumina settings saved", fr: "Réglages Lumina enregistrés", es: "Ajustes de Lumina guardados", de: "Lumina-Einstellungen gespeichert" },
  set_error: { en: "Couldn't save", fr: "Enregistrement impossible", es: "No se pudo guardar", de: "Speichern fehlgeschlagen" },
};

export function translate(key: string, locale: string | undefined): string {
  const dict = LABELS[key];
  if (!dict) return key;
  const lang = (locale ?? "en").slice(0, 2).toLowerCase() as Locale;
  return dict[lang] ?? dict.en;
}
