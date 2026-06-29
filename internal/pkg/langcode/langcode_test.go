package langcode

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		// already ISO 639-1 -> pass through
		"fr": "fr", "en": "en", "tr": "tr", "nl": "nl",
		// 639-2/B bibliographic whose 2-letter form is NOT a prefix (the bug class)
		"tur": "tr", "dut": "nl", "gre": "el", "per": "fa", "rum": "ro",
		"cze": "cs", "may": "ms", "slo": "sk", "ice": "is", "wel": "cy",
		"ger": "de", "fre": "fr",
		// 639-2/T terminological variants
		"deu": "de", "fra": "fr", "nld": "nl", "ces": "cs", "ell": "el",
		"fas": "fa", "ron": "ro", "msa": "ms",
		// straightforward 639-2 -> 639-1
		"eng": "en", "spa": "es", "jpn": "ja", "rus": "ru", "swe": "sv",
		"dan": "da", "fin": "fi", "ukr": "uk", "vie": "vi", "tha": "th",
		"ind": "id", "hun": "hu", "hrv": "hr", "ara": "ar", "heb": "he",
		// Chinese macrolanguage tags + legacy
		"chi": "zh", "zho": "zh", "cmn": "zh", "yue": "zh",
		// English names
		"french": "fr", "german": "de", "turkish": "tr", "dutch": "nl",
		// BCP-47 region/script subtags dropped to the primary language
		"fr-fr": "fr", "en-en": "en", "pt-br": "pt", "en-US": "en",
		"cmn-hans": "zh", "zh-hant": "zh", "es-419": "es",
		// case / whitespace insensitivity
		"FRE": "fr", " Eng ": "en", "TUR": "tr",
		// unknown exotic codes pass through unchanged
		"tlh": "tlh", "xx": "xx", "": "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
