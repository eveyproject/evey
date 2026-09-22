// i18n key for a crew member's job, from the closed PERSON_TYPE_* enum the
// server sends alongside each person.
//
// The card used to print `person.role` instead, which for crew is the raw
// database role_type: every viewer, in every language, read "director" in
// lowercase English under Éric Toledano (seen on the living-room box,
// 2026-09-19). For the CAST that same field is the character name and must
// keep being shown as-is - this maps the job, nothing else.
const KEYS: Record<string, string> = {
  PERSON_TYPE_ACTOR: "media.person.actor",
  PERSON_TYPE_GUEST_STAR: "media.person.guestStar",
  PERSON_TYPE_DIRECTOR: "media.person.director",
  PERSON_TYPE_WRITER: "media.person.writer",
  PERSON_TYPE_PRODUCER: "media.person.producer",
  PERSON_TYPE_COMPOSER: "media.person.composer",
};

// Returns null for an unknown or missing type, so callers fall back to what
// the server gave them rather than showing a key.
export function personTypeLabelKey(type?: string): string | null {
  if (!type) return null;
  return KEYS[type] ?? null;
}
