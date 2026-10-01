// ISO 639-2 -> 639-1 language code table.
//
// It lived in apps/mobile with a note saying it was copied verbatim from the
// web player and had to be kept in sync BY HAND. It now lives here so mobile,
// TV and anything else compare language tags the same way - a shared rule that
// pickDefaultSubtitle depends on: a "fr" preference has to match a "fre"
// track, and a reduced copy of this table silently stopped matching (caught by
// the TV suite, 2026-09-05).
//
// A plain slice(0,2) of the 639-2 code is NOT a safe shortcut: it happens to
// work for some ("fre"/"fra" -> "fr"), but produces the WRONG 639-1 code for
// others ("ger" -> "ge" instead of "de", "por" -> "po" instead of "pt"), and
// for a few it silently COLLIDES with an unrelated language's real code
// ("slo" (Slovak) -> "sl", which is Slovenian's actual 639-1 code). Every
// language below is therefore listed explicitly.
const ISO_639_2_TO_1: Record<string, string> = {
  afr: "af", ara: "ar", bul: "bg", ben: "bn", cat: "ca",
  cze: "cs", ces: "cs", wel: "cy", cym: "cy", dan: "da",
  ger: "de", deu: "de", gre: "el", ell: "el", eng: "en",
  spa: "es", est: "et", baq: "eu", eus: "eu", per: "fa", fas: "fa",
  fin: "fi", fil: "fil", fre: "fr", fra: "fr", gle: "ga", glg: "gl",
  guj: "gu", heb: "he", hin: "hi", hrv: "hr", hun: "hu",
  arm: "hy", hye: "hy", ind: "id", ice: "is", isl: "is", ita: "it",
  jpn: "ja", geo: "ka", kat: "ka", kan: "kn", kor: "ko",
  lit: "lt", lav: "lv", mac: "mk", mkd: "mk", mal: "ml", mar: "mr",
  may: "ms", msa: "ms", mlt: "mt", dut: "nl", nld: "nl", nor: "no",
  pan: "pa", pol: "pl", por: "pt", rum: "ro", ron: "ro", rus: "ru",
  slo: "sk", slk: "sk", slv: "sl", alb: "sq", sqi: "sq", srp: "sr",
  swe: "sv", swa: "sw", tam: "ta", tel: "te", tha: "th", tur: "tr",
  ukr: "uk", urd: "ur", vie: "vi", chi: "zh", zho: "zh",
};

// normalizeLangCode reduces a language tag to a comparable ISO 639-1 code:
// strips a BCP-47 region/script subtag ("fr-FR" -> "fr"), then maps a known
// ISO 639-2 code to 639-1. Codes outside the table above fall back to a
// 2-letter slice - the best guess available.
export function normalizeLangCode(code: string | undefined): string {
  const primary = (code ?? "").toLowerCase().trim().split("-")[0];
  if (!primary) return "";
  return ISO_639_2_TO_1[primary] ?? (primary.length > 2 ? primary.slice(0, 2) : primary);
}

/**
 * The first stream whose language matches one of the profile's preferred
 * codes (a CSV of ISO 639-1 codes, in order of preference), or null. The
 * same rule picks the audio track on every player; a plain slice of the
 * container's 639-2 code is not a match (see normalizeLangCode).
 */
export function pickPreferredStreamIndex(
  streams: { index: number; language?: string }[],
  preferredLangs: string[],
): number | null {
  for (const lang of preferredLangs) {
    if (!lang) continue;
    const want = normalizeLangCode(lang);
    if (!want) continue;
    const match = streams.find((s) => normalizeLangCode(s.language) === want);
    if (match) return match.index;
  }
  return null;
}

// Names for the codes a library actually carries, used when the runtime has no
// Intl language data (Hermes on the phone and the TV ships without it unless
// built against it) or does not know the code. Endonyms, so the fallback reads
// right whatever the UI language.
const FALLBACK_LANGUAGE_NAMES: Record<string, string> = {
  ar: "العربية", bg: "Български", ca: "Català", cs: "Čeština", da: "Dansk",
  de: "Deutsch", el: "Ελληνικά", en: "English", es: "Español", et: "Eesti",
  fa: "فارسی", fi: "Suomi", fr: "Français", he: "עברית", hi: "हिन्दी",
  hr: "Hrvatski", hu: "Magyar", id: "Bahasa Indonesia", is: "Íslenska",
  it: "Italiano", ja: "日本語", ko: "한국어", lt: "Lietuvių", lv: "Latviešu",
  nl: "Nederlands", no: "Norsk", pl: "Polski", pt: "Português", ro: "Română",
  ru: "Русский", sk: "Slovenčina", sl: "Slovenščina", sr: "Српски", sv: "Svenska",
  th: "ไทย", tr: "Türkçe", uk: "Українська", vi: "Tiếng Việt", zh: "中文",
};

// Tags that name no language: undetermined, multiple, no linguistic content.
const NO_LANGUAGE = new Set(["und", "mul", "zxx", "mis"]);

const displayNamesByLocale = new Map<string, Intl.DisplayNames | null>();

function languageNames(locale: string): Intl.DisplayNames | null {
  if (!displayNamesByLocale.has(locale)) {
    let names: Intl.DisplayNames | null = null;
    try {
      names = new Intl.DisplayNames([locale], { type: "language" });
    } catch {
      names = null; // no Intl language data on this runtime
    }
    displayNamesByLocale.set(locale, names);
  }
  return displayNamesByLocale.get(locale) ?? null;
}

/**
 * The readable name of a track's language tag in the UI language: "ger",
 * "deu", "de" and "de-DE" all read "German" ("Allemand" in French). Null for
 * an empty tag or one that names no language ("und"), so the caller decides
 * whether to say "Unknown" or show nothing. A code nothing knows comes back
 * as written: a poor label, never a blank one.
 */
export function languageDisplayName(code: string | undefined, locale: string): string | null {
  const raw = (code ?? "").trim();
  if (!raw) return null;
  const [primary, ...rest] = raw.toLowerCase().split(/[-_]/);
  if (!primary || NO_LANGUAGE.has(primary)) return null;
  const base = ISO_639_2_TO_1[primary] ?? primary;
  const tag = [base, ...rest.map((r) => (r.length === 2 ? r.toUpperCase() : r))].join("-");
  let name: string | undefined;
  try {
    name = languageNames(locale)?.of(tag);
  } catch {
    name = undefined; // a tag Intl rejects as malformed
  }
  // Intl hands an unknown code back unchanged: that is not a name.
  if (!name || name.toLowerCase() === tag.toLowerCase()) name = FALLBACK_LANGUAGE_NAMES[base];
  if (!name) return raw;
  return name.charAt(0).toLocaleUpperCase(locale) + name.slice(1);
}
