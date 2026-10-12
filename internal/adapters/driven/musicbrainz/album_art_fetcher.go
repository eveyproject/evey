// Package musicbrainz récupère les pochettes d'album via MusicBrainz (pour
// IDENTIFIER la sortie) puis Cover Art Archive (pour l'IMAGE).
//
// Pourquoi ce provider plutôt qu'une recherche plein-texte : MusicBrainz accepte
// une requête PAR CHAMPS (`artist:"..." AND releasegroup:"..."`) et renvoie un
// score. Une recherche plein-texte, elle, retombe sur l'album le plus populaire
// de l'artiste quand le titre demandé n'est pas indexé - c'est ce qui a collé la
// pochette de Mixed Signals sur deux albums Neon Harbor en prod (2026-07-25).
// Ici la mauvaise correspondance est écartée par construction, pas rattrapée.
package musicbrainz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // DecodeConfig lit l'en-tête JPEG sans décoder les pixels
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/pkg/titlematch"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"golang.org/x/time/rate"
)

const (
	defaultMusicBrainzURL = "https://musicbrainz.org/ws/2"
	defaultCoverArtURL    = "https://coverartarchive.org"

	// MusicBrainz demande explicitement un User-Agent identifiable et refuse les
	// requêtes anonymes.
	userAgent = "Evey/1.0 ( https://github.com/eveyproject/evey )"

	defaultTimeout = 15 * time.Second

	// Nombre d'éditions candidates testées auprès de Cover Art Archive avant de
	// se rabattre sur l'image du release-group. Borné : chaque essai est un aller
	// -retour réseau, et la cible est un NAS modeste.
	maxReleaseCandidates = 4

	// Une pochette d'album est carrée. Cover Art Archive sert des SCANS
	// d'éditions physiques : l'image « front » d'un release-group peut être une
	// cassette (mesuré : 320x500 pour From Nowhere), qui serait rognée dans une
	// grille carrée. On tolère l'imprécision d'un scan, pas un format qui n'est
	// pas une pochette.
	minAspect = 0.85
	maxAspect = 1.18
)

// preferredFormats classe les supports dont la pochette est carrée par nature.
// Les éditions numériques et CD portent l'artwork officiel ; vinyles et
// cassettes donnent des scans aux proportions arbitraires.
var preferredFormats = []string{"Digital Media", "CD"}

type AlbumArtFetcher struct {
	httpClient   *http.Client
	mbBaseURL    string
	caaBaseURL   string
	limiter      *rate.Limiter
	artSizeOrder []int
}

func NewAlbumArtFetcher(client *http.Client) *AlbumArtFetcher {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &AlbumArtFetcher{
		httpClient: client,
		mbBaseURL:  defaultMusicBrainzURL,
		caaBaseURL: defaultCoverArtURL,
		limiter:    rate.NewLimiter(rate.Every(time.Second), 1), // MusicBrainz demande <= 1 req/s
		artSizeOrder: []int{
			// 500 d'abord : suffisant pour une vignette comme pour la page album,
			// et bien plus léger que 1200 (mesuré sur un scan : 352 Ko contre 86).
			500, 1200,
		},
	}
}

// SetBaseURLsForTest redirige les deux APIs vers des serveurs de test.
func (f *AlbumArtFetcher) SetBaseURLsForTest(musicbrainzURL, coverArtURL string) {
	f.mbBaseURL = musicbrainzURL
	f.caaBaseURL = coverArtURL
	f.limiter = rate.NewLimiter(rate.Inf, 1) // pas d'attente d'une seconde en test
}

// Fetch renvoie les octets JPEG/PNG de la pochette de (artist, album), ou
// os.ErrNotExist quand aucune sortie ne correspond ou qu'aucune image n'existe.
// Ne renvoie JAMAIS la pochette d'un autre album : mieux vaut un placeholder.
func (f *AlbumArtFetcher) Fetch(ctx context.Context, artist, album string) ([]byte, string, error) {
	ctx, span := tracing.StartSpan(ctx, "musicbrainz.albumart.Fetch")
	defer span.End()

	if artist == "" || album == "" {
		return nil, "", os.ErrNotExist
	}

	groupID, err := f.findReleaseGroup(ctx, artist, album)
	if err != nil {
		return nil, "", err
	}

	// Éditions d'abord (pochette carrée officielle), release-group en repli.
	for _, releaseID := range f.candidateReleases(ctx, groupID) {
		if data, mime, ok := f.tryCoverArt(ctx, "release", releaseID); ok {
			return data, mime, nil
		}
	}
	if data, mime, ok := f.tryCoverArt(ctx, "release-group", groupID); ok {
		return data, mime, nil
	}
	return nil, "", os.ErrNotExist
}

// findReleaseGroup interroge MusicBrainz par CHAMPS et ne retient qu'un
// release-group dont l'artiste ET le titre correspondent vraiment.
func (f *AlbumArtFetcher) findReleaseGroup(ctx context.Context, artist, album string) (string, error) {
	// Requête LARGE, vérification STRICTE. Le titre interrogé est débarrassé de
	// son suffixe d'édition : une recherche par champs sur
	// `releasegroup:"Song Of The Valley (Remastered 2025)"` ne renvoie RIEN,
	// alors que le même album ressort avec un score de 100 sur
	// `releasegroup:"Song Of The Valley"` (mesuré contre l'API réelle). La
	// correspondance est ensuite contrôlée sur le titre COMPLET, donc élargir la
	// requête ne relâche pas la vérification.
	// Les guillemets délimitent la valeur dans la syntaxe de recherche : ceux
	// présents dans un titre casseraient la requête.
	q := fmt.Sprintf(`artist:%q AND releasegroup:%q`, sanitizeTerm(artist), sanitizeTerm(baseTitle(album)))
	params := url.Values{
		"query": {q},
		"fmt":   {"json"},
		"limit": {"10"},
	}

	body, err := f.getJSON(ctx, f.mbBaseURL+"/release-group?"+params.Encode())
	if err != nil {
		return "", fmt.Errorf("musicbrainz: search release-group: %w", err)
	}
	var resp releaseGroupSearch
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("musicbrainz: decode search: %w", err)
	}

	for _, rg := range resp.ReleaseGroups {
		if rg.ID == "" || !titlematch.Equivalent(rg.Title, album) {
			continue
		}
		if !slices.ContainsFunc(rg.ArtistCredit, func(ac artistCredit) bool {
			return titlematch.Equivalent(ac.Name, artist)
		}) {
			continue
		}
		return rg.ID, nil
	}
	return "", os.ErrNotExist
}

// candidateReleases renvoie les éditions du release-group dont le support porte
// une pochette carrée, les formats préférés en tête. Une erreur ici n'est pas
// fatale : l'appelant se rabat sur l'image du release-group.
func (f *AlbumArtFetcher) candidateReleases(ctx context.Context, groupID string) []string {
	params := url.Values{
		"release-group": {groupID},
		"fmt":           {"json"},
		"inc":           {"media"},
		"limit":         {"50"},
	}
	body, err := f.getJSON(ctx, f.mbBaseURL+"/release?"+params.Encode())
	if err != nil {
		return nil
	}
	var resp releaseList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}

	var out []string
	for _, want := range preferredFormats {
		for _, r := range resp.Releases {
			if r.ID == "" || slices.Contains(out, r.ID) {
				continue
			}
			if slices.ContainsFunc(r.Media, func(m medium) bool { return m.Format == want }) {
				out = append(out, r.ID)
			}
			if len(out) >= maxReleaseCandidates {
				return out
			}
		}
	}
	return out
}

// tryCoverArt télécharge la pochette d'une entité Cover Art Archive et rejette
// une image dont les proportions ne sont pas celles d'une pochette.
func (f *AlbumArtFetcher) tryCoverArt(ctx context.Context, entity, id string) ([]byte, string, bool) {
	for _, size := range f.artSizeOrder {
		u := fmt.Sprintf("%s/%s/%s/front-%d", f.caaBaseURL, entity, url.PathEscape(id), size)
		data, mime, err := f.getImage(ctx, u)
		if err != nil {
			continue // 404 = pas d'image pour cette entité/taille, on continue
		}
		if !plausibleCoverAspect(data) {
			continue
		}
		return data, mime, true
	}
	return nil, "", false
}

func plausibleCoverAspect(data []byte) bool {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Height == 0 {
		// En-tête illisible : on ACCEPTE plutôt que de rejeter. Le rôle de ce
		// contrôle est d'écarter un scan de cassette, pas de faire office de
		// validateur d'image - refuser sur un format inconnu ferait perdre des
		// pochettes valides pour une raison qui n'est pas la bonne.
		return true
	}
	ratio := float64(cfg.Width) / float64(cfg.Height)
	return ratio >= minAspect && ratio <= maxAspect
}

func (f *AlbumArtFetcher) getJSON(ctx context.Context, rawURL string) ([]byte, error) {
	if err := f.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return f.do(ctx, rawURL, "application/json")
}

func (f *AlbumArtFetcher) getImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	// Cover Art Archive est un autre hôte que MusicBrainz et n'impose pas la
	// même cadence : pas d'attente d'une seconde par image.
	data, err := f.do(ctx, rawURL, "image/*")
	if err != nil {
		return nil, "", err
	}
	return data, "image/jpeg", nil
}

func (f *AlbumArtFetcher) do(ctx context.Context, rawURL, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d for %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

// baseTitle retire les groupes entre parenthèses ou crochets en FIN de titre,
// là où vivent les mentions d'édition ("(Remastered 2025)", "(Deluxe Edition)",
// "[Explicit]"). Sert uniquement à élargir la requête ; ne jamais s'en servir
// pour comparer deux titres, sous peine de confondre un album avec sa suite
// ("Volume (1)" et "Volume (2)" donnent la même base).
func baseTitle(album string) string {
	out := strings.TrimSpace(album)
	for {
		trimmed := false
		for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}} {
			if !strings.HasSuffix(out, pair[1]) {
				continue
			}
			open := strings.LastIndex(out, pair[0])
			// Un titre entièrement parenthésé n'a pas de base : on le garde tel quel.
			if open <= 0 {
				continue
			}
			out = strings.TrimSpace(out[:open])
			trimmed = true
		}
		if !trimmed || out == "" {
			break
		}
	}
	if out == "" {
		return strings.TrimSpace(album)
	}
	return out
}

// sanitizeTerm retire les caractères qui ont un sens dans la syntaxe de
// recherche Lucene de MusicBrainz, pour qu'un titre ne puisse pas altérer la
// structure de la requête.
func sanitizeTerm(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`"\+-!(){}[]^~*?:/`, r) {
			return ' '
		}
		return r
	}, s)
}

type releaseGroupSearch struct {
	ReleaseGroups []struct {
		ID           string         `json:"id"`
		Title        string         `json:"title"`
		ArtistCredit []artistCredit `json:"artist-credit"`
	} `json:"release-groups"`
}

type artistCredit struct {
	Name string `json:"name"`
}

type releaseList struct {
	Releases []struct {
		ID    string   `json:"id"`
		Media []medium `json:"media"`
	} `json:"releases"`
}

type medium struct {
	Format string `json:"format"`
}
