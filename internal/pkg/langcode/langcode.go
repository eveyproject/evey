// Package langcode normalizes the many spellings of a language a media probe can
// emit (ISO 639-2/B bibliographic, 639-2/T terminological, BCP-47 macrolanguage
// tags, and full English names) to a single ISO 639-1 two-letter form.
//
// This matters because the same track can surface as "tur" (ffprobe, 639-2) or
// "tr" (the in-process mkvgo prober, BCP-47) — preferred-language matching must
// fold both to "tr". A naive 3→2 truncation is WRONG for the 639-2/B
// bibliographic codes whose two-letter form is not the first two letters
// (tur→tr but dut→nl, gre→el, per→fa, rum→ro, cze→cs, may→ms, …), which is the
// class this table exists to get right.
package langcode

import "strings"

// iso6392to1 maps ISO 639-2/B (bibliographic) and 639-2/T (terminological)
// three-letter codes to their ISO 639-1 two-letter form. Where /B and /T differ
// (e.g. ger/deu, fre/fra, dut/nld) both are listed. BCP-47 Chinese macrolanguage
// tags fold to "zh".
var iso6392to1 = map[string]string{
	// /B and /T variants that DIFFER (the dangerous ones for naive truncation)
	"alb": "sq", "sqi": "sq",
	"arm": "hy", "hye": "hy",
	"baq": "eu", "eus": "eu",
	"bur": "my", "mya": "my",
	"cze": "cs", "ces": "cs",
	"dut": "nl", "nld": "nl",
	"fre": "fr", "fra": "fr",
	"geo": "ka", "kat": "ka",
	"ger": "de", "deu": "de",
	"gre": "el", "ell": "el",
	"ice": "is", "isl": "is",
	"mac": "mk", "mkd": "mk",
	"mao": "mi", "mri": "mi",
	"may": "ms", "msa": "ms",
	"per": "fa", "fas": "fa",
	"rum": "ro", "ron": "ro",
	"slo": "sk", "slk": "sk",
	"tib": "bo", "bod": "bo",
	"wel": "cy", "cym": "cy",
	// Chinese (legacy 639-2 + BCP-47 macrolanguage tags)
	"chi": "zh", "zho": "zh", "cmn": "zh", "yue": "zh", "wuu": "zh", "hak": "zh", "nan": "zh",
	// Straightforward 639-2 → 639-1 (no /B vs /T split)
	"eng": "en", "spa": "es", "ita": "it", "por": "pt", "rus": "ru",
	"jpn": "ja", "kor": "ko", "ara": "ar", "heb": "he", "hin": "hi",
	"pol": "pl", "swe": "sv", "nor": "no", "nob": "nb", "nno": "nn",
	"dan": "da", "fin": "fi", "tur": "tr", "ukr": "uk", "vie": "vi",
	"tha": "th", "ind": "id", "hun": "hu", "hrv": "hr", "ben": "bn",
	"cat": "ca", "glg": "gl", "gle": "ga", "lat": "la", "epo": "eo",
	"est": "et", "lav": "lv", "lit": "lt", "slv": "sl", "srp": "sr",
	"bul": "bg", "bel": "be", "tam": "ta", "tel": "te",
	"urd": "ur", "pan": "pa", "guj": "gu", "kan": "kn", "mal": "ml",
	"mar": "mr", "nep": "ne", "sin": "si", "khm": "km", "lao": "lo",
	"amh": "am", "swa": "sw", "zul": "zu", "afr": "af", "fil": "tl",
	"tgl": "tl", "aze": "az", "kaz": "kk", "uzb": "uz",
}

// englishNames folds the full English language name (lower-cased) to ISO 639-1.
// Some metadata sources store "French"/"German" rather than a code.
var englishNames = map[string]string{
	"french": "fr", "english": "en", "spanish": "es", "german": "de",
	"italian": "it", "japanese": "ja", "chinese": "zh", "portuguese": "pt",
	"korean": "ko", "russian": "ru", "arabic": "ar", "dutch": "nl",
	"polish": "pl", "swedish": "sv", "turkish": "tr", "hindi": "hi",
	"greek": "el", "czech": "cs", "danish": "da", "finnish": "fi",
	"norwegian": "no", "hungarian": "hu", "hebrew": "he", "thai": "th",
	"romanian": "ro", "ukrainian": "uk", "vietnamese": "vi", "indonesian": "id",
}

// Normalize lower-cases and trims lang, then folds known ISO 639-2/B/T codes,
// BCP-47 macrolanguage tags and English names to ISO 639-1. Two-letter codes are
// already 639-1 and pass through; unknown exotic codes pass through unchanged so
// an exact match on the original tag still resolves.
func Normalize(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	// BCP-47: keep only the primary language subtag, dropping region/script/
	// variant (e.g. "fr-FR" -> "fr", "pt-BR" -> "pt", "cmn-Hans" -> "cmn"). The
	// in-process mkvgo prober surfaces these region-qualified tags where ffprobe
	// reports a bare 639-2 code, so both must collapse to the same primary lang.
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	if v, ok := englishNames[lang]; ok {
		return v
	}
	if v, ok := iso6392to1[lang]; ok {
		return v
	}
	return lang
}
