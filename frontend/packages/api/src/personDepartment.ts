// TMDB's known_for_department is a small closed set served in English
// ("Acting", "Costume & Make-Up", ...). Map each value to an i18n key so the
// person pages can localize the line; an unknown department yields null and
// the caller falls back to the raw English value instead of a key name.
const DEPARTMENT_SLUGS: Record<string, string> = {
  Acting: "acting",
  Actors: "acting",
  Directing: "directing",
  Writing: "writing",
  Production: "production",
  Sound: "sound",
  Camera: "camera",
  Editing: "editing",
  Art: "art",
  Crew: "crew",
  "Costume & Make-Up": "costumeMakeup",
  "Visual Effects": "visualEffects",
  Lighting: "lighting",
  Creator: "creator",
};

export function personDepartmentKey(department?: string | null): string | null {
  if (!department) return null;
  const slug = DEPARTMENT_SLUGS[department.trim()];
  return slug ? `media.person.department.${slug}` : null;
}
