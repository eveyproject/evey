package library

import (
	"sort"
	"strings"
)

// genre_aliases.go folds the many spellings of one genre onto a single label.
//
// Why it is needed. Genres are stored as free text, exactly as the provider
// returned them at scan time - and a library accumulates several providers and
// several languages over its life. A real library (2026-09-04) held 28 distinct
// genres for what is really 20, with eight straight duplicates:
//
//	Adventure/Aventure  Comedy/Comédie  Documentary/Documentaire  Drama/Drame
//	Fantasy/Fantastique Horror/Horreur  Mystery/Mystère           Sci-Fi/Science-Fiction
//
// plus IMDb-only spellings TMDB never emits ("Sci-Fi", "Short", "Supernatural").
// The filter panel listed all 28, so picking "Drame" silently hid every title
// tagged "Drama" - the filter looked broken, and the list looked absurd.
//
// This folds on READ. Nothing rewrites the stored fiches: a scan can still bring
// back an English label tomorrow, and re-tagging thousands of rows to chase a
// display problem would be a destructive answer to a cosmetic cause. Reading is
// also the only layer that knows which language to render in.
//
// Unknown genres pass through untouched. A provider inventing "Mecha" must show
// up as "Mecha", not vanish - an allowlist here would silently drop content.

// genreKey is the language-independent identity of a genre. The value is
// arbitrary; only its uniqueness matters.
type genreKey string

// genreAliases maps every spelling we have actually seen to its key. Keys are
// lowercased at lookup, so only one case form is needed here.
//
// Sources: TMDB movie genres (en + fr), TMDB TV genres (en + fr), and the
// IMDb/OMDb labels that reach us through the OMDB enricher.
var genreAliases = map[string]genreKey{
	// TMDB movie genres
	"action": "action", "aventure": "adventure", "adventure": "adventure",
	"animation": "animation", "comédie": "comedy", "comedie": "comedy", "comedy": "comedy",
	"crime": "crime", "documentaire": "documentary", "documentary": "documentary",
	"drame": "drama", "drama": "drama", "familial": "family", "family": "family",
	"fantastique": "fantasy", "fantasy": "fantasy", "histoire": "history", "history": "history",
	"horreur": "horror", "horror": "horror", "musique": "music", "music": "music",
	"mystère": "mystery", "mystere": "mystery", "mystery": "mystery",
	"romance": "romance",
	"science-fiction": "scifi", "science fiction": "scifi", "sci-fi": "scifi", "sciencefiction": "scifi",
	"téléfilm": "tvmovie", "telefilm": "tvmovie", "tv movie": "tvmovie",
	"thriller": "thriller", "guerre": "war", "war": "war", "western": "western",

	// TMDB TV genres
	"action & adventure": "action_adventure", "action & aventure": "action_adventure",
	"enfants": "kids", "kids": "kids",
	"actualités": "news", "actualites": "news", "news": "news",
	"réalité": "reality", "realite": "reality", "reality": "reality",
	"science-fiction & fantastique": "scifi_fantasy", "sci-fi & fantasy": "scifi_fantasy",
	"feuilleton": "soap", "soap": "soap",
	"talk": "talk", "talk-show": "talk", "talk show": "talk",
	"guerre & politique": "war_politics", "war & politics": "war_politics",

	// IMDb / OMDb only - TMDB never emits these
	"short": "short", "court-métrage": "short", "court-metrage": "short",
	"supernatural": "supernatural", "surnaturel": "supernatural",
	"film-noir": "film_noir", "biography": "biography", "biographie": "biography",
	"sport": "sport", "musical": "musical",
}

// genreLabels renders a key in a given language. French is the fallback for any
// language we do not translate: the alternative is showing a raw key.
var genreLabels = map[genreKey]map[string]string{
	"action":           {"fr": "Action", "en": "Action"},
	"adventure":        {"fr": "Aventure", "en": "Adventure"},
	"animation":        {"fr": "Animation", "en": "Animation"},
	"comedy":           {"fr": "Comédie", "en": "Comedy"},
	"crime":            {"fr": "Crime", "en": "Crime"},
	"documentary":      {"fr": "Documentaire", "en": "Documentary"},
	"drama":            {"fr": "Drame", "en": "Drama"},
	"family":           {"fr": "Familial", "en": "Family"},
	"fantasy":          {"fr": "Fantastique", "en": "Fantasy"},
	"history":          {"fr": "Histoire", "en": "History"},
	"horror":           {"fr": "Horreur", "en": "Horror"},
	"music":            {"fr": "Musique", "en": "Music"},
	"mystery":          {"fr": "Mystère", "en": "Mystery"},
	"romance":          {"fr": "Romance", "en": "Romance"},
	"scifi":            {"fr": "Science-Fiction", "en": "Science Fiction"},
	"tvmovie":          {"fr": "Téléfilm", "en": "TV Movie"},
	"thriller":         {"fr": "Thriller", "en": "Thriller"},
	"war":              {"fr": "Guerre", "en": "War"},
	"western":          {"fr": "Western", "en": "Western"},
	"action_adventure": {"fr": "Action & Aventure", "en": "Action & Adventure"},
	"kids":             {"fr": "Enfants", "en": "Kids"},
	"news":             {"fr": "Actualités", "en": "News"},
	"reality":          {"fr": "Réalité", "en": "Reality"},
	"scifi_fantasy":    {"fr": "Science-Fiction & Fantastique", "en": "Sci-Fi & Fantasy"},
	"soap":             {"fr": "Feuilleton", "en": "Soap"},
	"talk":             {"fr": "Talk", "en": "Talk"},
	"war_politics":     {"fr": "Guerre & Politique", "en": "War & Politics"},
	"short":            {"fr": "Court-métrage", "en": "Short"},
	"supernatural":     {"fr": "Surnaturel", "en": "Supernatural"},
	"film_noir":        {"fr": "Film noir", "en": "Film-Noir"},
	"biography":        {"fr": "Biographie", "en": "Biography"},
	"sport":            {"fr": "Sport", "en": "Sport"},
	"musical":          {"fr": "Musical", "en": "Musical"},
}

func lookupKey(genre string) (genreKey, bool) {
	k, ok := genreAliases[strings.ToLower(strings.TrimSpace(genre))]
	return k, ok
}

// CanonicalGenre renders one stored genre in lang ("fr", "en"). A spelling we
// do not know comes back unchanged - never empty, never dropped.
//
// An EMPTY lang means "do not translate": the stored spelling is returned as-is.
// That is the honest default for a server whose language nobody configured -
// translating to a language picked by the developer would be worse than showing
// what the provider actually wrote.
func CanonicalGenre(genre, lang string) string {
	genre = strings.TrimSpace(genre)
	if lang == "" {
		return genre
	}
	key, ok := lookupKey(genre)
	if !ok {
		return genre
	}
	labels, ok := genreLabels[key]
	if !ok {
		return genre
	}
	if l, ok := labels[lang]; ok {
		return l
	}
	// A language we hold no labels for: keep the stored spelling rather than
	// substituting an arbitrary one.
	return genre
}

// CanonicalGenres folds a stored list into the display list: one entry per
// genre, sorted. This is what turns 28 rows into 20.
//
// With a lang, entries are rendered in it. WITHOUT one, duplicates are still
// folded - onto the first spelling encountered, in sorted order, so the result
// is deterministic - but nothing is translated. Deduplicating is always right;
// choosing a language is not ours to do unasked.
func CanonicalGenres(stored []string, lang string) []string {
	ordered := make([]string, len(stored))
	copy(ordered, stored)
	sort.Strings(ordered) // determinism: which spelling wins must not depend on row order

	seenKey := make(map[genreKey]struct{}, len(ordered))
	seenLabel := make(map[string]struct{}, len(ordered))
	out := make([]string, 0, len(ordered))
	for _, g := range ordered {
		label := CanonicalGenre(g, lang)
		if label == "" {
			continue
		}
		// Fold by identity when we know it, so "Drama" and "Drame" collapse even
		// when we are not translating.
		if key, ok := lookupKey(g); ok {
			if _, dup := seenKey[key]; dup {
				continue
			}
			seenKey[key] = struct{}{}
		}
		if _, dup := seenLabel[label]; dup {
			continue
		}
		seenLabel[label] = struct{}{}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

// ExpandGenreAliases turns the genres a client asked for into every spelling
// that means the same thing, so the SQL filter matches rows tagged in the other
// language or by the other provider.
//
// Without this the fold above would make things WORSE, not better: the panel
// would offer a single "Drame" while half the library is tagged "Drama", and
// selecting it would hide those titles - a filter that lies rather than a list
// that repeats.
//
// ⚠️ Everything comes back LOWERCASED, and the caller must compare lowercased
// too. Stored genres carry whatever case the provider used ("Sci-Fi",
// "Science-Fiction", "SCI-FI"), and enumerating case variants here would be a
// losing game; the query does `lower(element) = ANY(...)` instead. See
// buildWhereClause in the postgres adapter - the two MUST move together.
func ExpandGenreAliases(requested []string) []string {
	seen := make(map[string]struct{}, len(requested)*2)
	out := make([]string, 0, len(requested)*2)
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, g := range requested {
		// The request itself, even when unknown: an invented genre must still
		// filter on itself rather than match nothing.
		add(g)
		key, ok := lookupKey(g)
		if !ok {
			continue
		}
		// Every stored spelling that folds onto the same key.
		for alias, k := range genreAliases {
			if k == key {
				add(alias)
			}
		}
		// And the rendered labels, which are what a client usually sends back.
		for _, label := range genreLabels[key] {
			add(label)
		}
	}
	sort.Strings(out)
	return out
}
