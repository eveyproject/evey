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
